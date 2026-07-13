# Operations

## Security warning

Validator compromise is fleet compromise. Validator can authorize root-level typed work, arbitrary admin commands, and temporary SSH access. Isolate host, restrict client/agent/enrollment networks, encrypt backups, protect CA and token-signing keys outside SQLite, and review audit events externally.

## Accounts and permissions

| Path/process | Owner | Mode/policy |
|---|---|---|
| agent process | root:root | root required for systemd, logs, cleanup, reboot, temporary SSH keys |
| validator process | silent-devops:silent-devops | no login shell |
| tunnel account | silent-tunnel:silent-tunnel | locked password, `/usr/sbin/nologin`, no shell/TTY |
| `/var/lib/silent-devops` | service owner | `0700` |
| private keys, DB, tokens | service owner | `0600` |
| tunnel `authorized_keys` | silent-tunnel | `0600` |

Create accounts:

```sh
useradd --system --home /var/lib/silent-devops --shell /usr/sbin/nologin silent-devops
useradd --system --home /var/lib/silent-devops/tunnel --shell /usr/sbin/nologin silent-tunnel
install -d -m 0700 -o silent-devops -g silent-devops /var/lib/silent-devops
install -d -m 0700 -o silent-tunnel -g silent-tunnel /var/lib/silent-devops/tunnel
```

## Network policy

Use distinct enrollment, agent, and client CIDR allowlists. Example: enrollment `10.10.0.0/24`, outbound agent `10.20.0.0/16`, client `10.30.0.0/24`. Permit agents to initiate validator mTLS and SSH tunnel connections. Expose no agent listener. Firewall validator client API and enrollment API to respective CIDRs. Tunnel remote listeners remain `127.0.0.1`.

## Lifecycle

- Enrollment: create short-lived one-time token, verify validator certificate pin out-of-band, enroll once, delete token material.
- Rotation: renew before expiry over authenticated mTLS; CSR identity must equal transport identity.
- Revocation: mark agent revoked, terminate active stream/tunnel, reject reconnect and renewal.
- Recovery: preserve DB, CA, signing key, config, logs. Run SQLite integrity check before restart. Never regenerate CA over existing fleet state.
- Upgrade: back up and integrity-check DB, install binaries atomically, run migrations on startup, restart validator then agents, verify versions and streams.
- Uninstall: revoke agents, close SSH sessions, stop/disable units, remove binaries/config/state, remove service accounts, verify firewall rules and temporary keys are gone.

## Storage, logging, health

See [storage.md](storage.md) for backup/restore and integrity. Services log structured JSON to stderr/journald. Alert on restart loops, storage failures, rejected certificates, duplicate streams, rate limits, and stale agents. Never log passwords, enrollment tokens, access tokens, private keys, or SSH binding tokens.

## Compatibility

| OS | amd64 | arm64 | Notes |
|---|---:|---:|---|
| Ubuntu 22.04 | yes | yes | systemd/OpenSSH |
| Ubuntu 24.04 | yes | yes | systemd/OpenSSH |
| Debian 12 | yes | yes | systemd/OpenSSH |

Go binaries are built for Linux. Agent expects procfs, systemd, journald, and OpenSSH. Containers cannot validate reboot, host cgroups, kernel firewall, or real VM networking; run VM smoke tests before production.

## Troubleshooting

- Agent offline: verify time, CA chain, certificate serial/revocation, outbound CIDR/firewall, DNS, and duplicate stream.
- Enrollment rejected: verify one-time token age/use, source CIDR, CSR, and certificate pin.
- Metrics absent: inspect procfs permissions, cardinality bounds, stream backpressure, and SQLite health.
- SSH not ready: verify tunnel key restriction, port availability, loopback binding, host key, session token, TTL, and dedicated sshd logs.
- Job unknown: do not retry cleanup, reboot, or arbitrary commands automatically. Inspect target and audit records.
