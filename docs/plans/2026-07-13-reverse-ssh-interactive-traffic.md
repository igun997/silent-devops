# Reverse SSH end-to-end interactive traffic

## Problem

Reverse-SSH readiness works, but real interactive traffic has never flowed end
to end, and cannot from a remote workstation:

1. Agent `SshReady` omits the agent sshd host key (`internal/agent/ssh.go`),
   so `SshReady.HostKey` is always empty. Even with host-key persistence
   (migration `005`), the stored value is empty, so the client cannot pin the
   target and refuses to launch (`internal/clientcli/ssh.go` requires
   `len(session.HostKey) > 0`).
2. The reverse tunnel binds `127.0.0.1:PORT` **on the validator host**
   (`ReverseTunnelArgs` `-R 127.0.0.1:PORT:127.0.0.1:22`). The client launches
   `ssh -p PORT silent-devops@127.0.0.1`, which only works when the client is
   co-located on the validator. A remote workstation has no route to the
   validator loopback.
3. E2E `TestReverseSSHReadyAndCleanup` only polls for `READY` and checks
   authorized-key cleanup. It never launches OpenSSH through the tunnel, so
   real traffic is unverified.

## Goal

Native-OpenSSH interactive session from a remote workstation client, reaching
the target host through the validator, with fail-closed pinning at every hop.
"Like normal ssh": use OpenSSH `ProxyJump`, not a custom relay.

## Topology

```
workstation client                validator host                 agent host
------------------                --------------                 ----------
ssh -J silent-client@VAL:2222     tunnel sshd :2222              sshd :22
   -p PORT silent-devops@127.0.0.1   |  reverse tunnel bind:        ^
        |  (ProxyJump / -W)          |  127.0.0.1:PORT ------------>| agent -R
        +--------------------------->+  (silent-tunnel, -R)
                                     |  jump direct-tcpip to
                                     |  127.0.0.1:PORT
                                     |  (silent-client, -W)
```

- Agent opens reverse tunnel to `silent-tunnel@validator:2222`
  (`-R 127.0.0.1:PORT:127.0.0.1:22`). Existing.
- Client jumps through `silent-client@validator:2222`, which local-forwards
  (`-W 127.0.0.1:PORT`) to the loopback-bound reverse tunnel, landing on the
  agent sshd.

## Trust / pinning

Every hop pinned, no TOFU:

- Jump host key: validator tunnel sshd host key, delivered to the client over
  the already-pinned authenticated gRPC channel in `SshSession`.
- Target host key: agent sshd host key, sent by the agent in `SshReady`,
  persisted by the validator (migration `005`), returned in `SshSession`.
- Client identity: one ephemeral Ed25519 key per session, authorized on BOTH:
  - validator `silent-client` user, scoped
    `restrict,permitopen="127.0.0.1:PORT",command="/bin/false"`; and
  - agent `silent-devops` user (existing `KeyStore.Install`).
- Session key + both authorized-key entries carry the session TTL marker and
  are removed on close/expiry/reconcile.

## Security properties

- `silent-client` uses `AllowTcpForwarding local` + `PermitOpen` restricted to
  the validator loopback port range. `-W` opens only a direct-tcpip channel
  (no session channel), so no shell/exec; `PermitTTY no` and
  `ForceCommand /bin/false` remain as belt-and-suspenders.
- Per-session `permitopen` means a client key can only forward to its own
  session's port.
- Tunnel sshd port firewalled to agent + client source IPs only.
- All OpenSSH invocations keep `StrictHostKeyChecking=yes`,
  `PasswordAuthentication=no`, `KbdInteractiveAuthentication=no`,
  `IdentitiesOnly=yes`, `ForwardAgent=no`, `ForwardX11=no`.

## Tasks

### Task 0 — proto: jump fields on SshSession
Add to `SshSession`: `jump_host` (7), `jump_user` (8), `jump_host_key` (9).
Regenerate. Update contracts test.

### Task 1 — agent sends its host key
- `AgentConfig.HostKeyPath` from `SILENT_DEVOPS_HOST_KEY` (default
  `/etc/ssh/ssh_host_ed25519_key.pub`).
- `SSHHandler.HostKey` loaded once; `Prepare` sends it in `SshReady.HostKey`.
- Unit test: `SshReady` carries the configured host key.

### Task 2 — validator jump-key install + SshSession jump fields
- Validator config: `SILENT_DEVOPS_TUNNEL_JUMP_ADDR`,
  `SILENT_DEVOPS_TUNNEL_JUMP_USER` (default `silent-client`),
  `SILENT_DEVOPS_TUNNEL_JUMP_HOST_KEY` (path to validator tunnel sshd public
  host key), `SILENT_DEVOPS_TUNNEL_CLIENT_AUTHORIZED_KEYS` (path to
  `silent-client` authorized_keys).
- `ssh.Manager` gains a client `KeyStore`-like installer that writes
  `restrict,permitopen="127.0.0.1:PORT",command="/bin/false"` entries scoped by
  session marker; install on `Create`, remove on `Close`/`Reconcile`.
- `Fleet.PrepareSsh`/`GetSshSession`/`Ready` populate `SshSession` jump fields
  from config.
- Unit tests: client authorized_keys install/remove, permitopen scoping,
  reconcile expiry.

### Task 3 — client ProxyJump launch
- `PrepareNativeSSH` builds `ssh -J JUMPUSER@JUMPADDR ... -p PORT
  silent-devops@127.0.0.1` with a pinned known-hosts file containing BOTH the
  jump host key (`[JUMPADDR]`) and the target host key (`[127.0.0.1]:PORT`).
- Fall back to direct loopback only when jump fields are empty (co-located
  client), preserving current behavior.
- Unit test on argument assembly (no network).

### Task 4 — Docker E2E real traffic
- `integration/entrypoint.sh` validator role: create `silent-client` user,
  write `sshd_config.silent-devops` with the `silent-client` Match block,
  publish the jump host key + jump address to `/shared`.
- Agent role: run login sshd, publish agent host key.
- `cmd/integration-helper` `ssh-exec ADDRESS TARGET_ID`: login, PrepareSsh,
  poll READY, assemble ProxyJump `ssh` with pinned known-hosts, run
  `id; hostname`, capture output, close, assert authorized-key cleanup on both
  hosts.
- `integration/e2e_test.go`: assert command output contains target hostname and
  both authorized-key files no longer contain the session marker.

### Task 5 — live provisioning + read-only verification
- Provision validator tunnel sshd (`silent-tunnel` + `silent-client`), agent
  env (`SILENT_DEVOPS_TUNNEL_*`, `SILENT_DEVOPS_AUTHORIZED_KEYS`,
  `SILENT_DEVOPS_HOST_KEY`), firewall 2222 to agent + client IPs.
- Run approved read-only command `id; hostname; exit`, verify cleanup.

## Non-goals
- No password auth anywhere.
- No persistent client shell key; ephemeral per session only.
- No change to job/metrics/enrollment paths.
