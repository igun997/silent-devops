# Detailed installation guide

Silent DevOps supports three install roles. Managed hosts need agent binary only. Validator host gets daemon plus built-in local control CLI. Remote client remains optional.

## Download

```sh
arch=$(dpkg --print-architecture) # amd64 or arm64
version=v0.1.0
curl -fLO "https://github.com/igun997/silent-devops/releases/download/${version}/silent-devops-linux-${arch}.tar.gz"
curl -fLO "https://github.com/igun997/silent-devops/releases/download/${version}/silent-devops-linux-${arch}.tar.gz.sha256"
sha256sum -c "silent-devops-linux-${arch}.tar.gz.sha256"
tar -xzf "silent-devops-linux-${arch}.tar.gz"
cd "silent-devops-linux-${arch}"
```

## Validator host

Install validator role:

```sh
sudo ./install.sh validator
```

Installer creates:

- `silent-devops` service user
- `silent-devops-admin` local admin group
- `/usr/local/sbin/silent-devops-validator`
- protected `/run/silent-devops/validator.sock`
- validator systemd unit

Configure TLS server cert/key, client CA, encrypted agent-signing CA, SQLite path, token key, CIDR policies, and first bootstrap admin using protected systemd environment/credentials. Required keys remain listed in [operations guide](operations.md).

```sh
sudo systemctl edit silent-devops-validator
sudo systemctl enable --now silent-devops-validator
sudo journalctl -u silent-devops-validator -f
```

Remove bootstrap password after first successful start.

Local control uses Unix socket. Root allowed:

```sh
sudo silent-devops-validator agents list
sudo silent-devops-validator metrics AGENT_ID
sudo silent-devops-validator audit list
```

Grant operator local access by adding user to admin group, then start new login session:

```sh
sudo usermod -aG silent-devops-admin alice
```

Create enrollment token:

```sh
sudo silent-devops-validator enroll-token create 600
```

Validator certificate pin can be calculated from certificate DER:

```sh
openssl x509 -in /path/to/server.crt -outform DER |
  openssl dgst -sha256 -binary |
  openssl base64 -A
```

Prefix output with `sha256/`.

## Managed agent host

Install agent role only:

```sh
sudo ./install.sh agent
```

Join with one command copied from validator administrator:

```sh
sudo silent-devops-agent join \
  validator.example.com:8443 \
  ONE_TIME_TOKEN \
  --pin 'sha256/BASE64_FINGERPRINT'
```

Join performs:

1. Opens TLS connection without sending token.
2. Verifies validator certificate pin.
3. Generates Ed25519 private key locally.
4. Sends CSR, hostname metadata, and one-time token.
5. Receives validator-assigned stable random agent ID.
6. Validates returned certificate identity and CA chain.
7. Writes credentials atomically under `/var/lib/silent-devops/agent` with `0600` files.
8. Enables and starts agent service.

Use `--no-start` for image preparation or manual service review. Use `--credential-dir PATH` only when systemd configuration matches.

Pin mismatch, used/expired token, invalid response, or existing credentials abort join. Token is never stored.

Check:

```sh
sudo systemctl status silent-devops-agent
sudo journalctl -u silent-devops-agent -f
sudo ls -l /var/lib/silent-devops/agent
```

Agent initiates outbound mTLS connection. Do not expose inbound agent control port.

### EasyPanel support (optional)

To manage a host's [EasyPanel](https://easypanel.io) from the client
(`easypanel AGENT detect|projects|token|migrate`), install the standalone
`easypanel-migrate` helper on that agent host. It is **not** bundled by
`install.sh agent`; install it per host:

```sh
GOOS=linux GOARCH=amd64 go build -o bin/easypanel-migrate-linux-amd64 ./cmd/easypanel-migrate
scp bin/easypanel-migrate-linux-amd64 HOST:/tmp/em
ssh HOST 'sudo install -m 0755 /tmp/em /usr/local/bin/easypanel-migrate'
silent-devops-client easypanel AGENT_ID detect   # verify
```

Cross-agent migration routes to the **target panel's own public URL** (its
`customPanelDomain`/`defaultDomain`/`serverIp`, surfaced by `detect` as
`public_url=`), not the agent hostname, so it works across networks. See
[EasyPanel service migration](easypanel-migrate.md).

## Optional remote client

```sh
sudo ./install.sh client
export SILENT_DEVOPS_VALIDATOR=validator.example.com:8443
export SILENT_DEVOPS_VALIDATOR_CA=$HOME/.config/silent-devops/validator-ca.crt
export SILENT_DEVOPS_SERVER_NAME=validator.example.com
silent-devops-client login admin
silent-devops-client agents list --json
```

Remote client still follows CIDR, password, token, and RBAC policy. Validator-local CLI needs no bearer token and uses OS peer identity.

## Verify installation

```sh
sudo silent-devops-validator agents list
sudo silent-devops-validator metrics AGENT_ID
```

Verify agent online, metrics current, viewer/operator/admin boundaries, audit metadata, revocation, reconnect, and no destructive replay. Run [VM smoke test](vm-smoke-test.md).

## Upgrade

1. Back up SQLite and encrypted agent CA separately.
2. Verify release checksum.
3. Stop target service.
4. Run role-specific installer from new bundle.
5. Start service.
6. Verify socket permissions, agent reconnect, metrics, audit, and no destructive replay.

## Rollback

Restore previous binaries. Restore DB only when schema compatibility requires it. Never replace agent credential directory during normal rollback. Confirm validator CA and server identity remain unchanged.

## Troubleshooting

- `validator certificate pin mismatch`: verify address and recompute pin from active server certificate.
- `invalid enrollment token`: create new token; tokens are short-lived and single-use.
- `agent credentials already exist`: inspect existing identity; do not overwrite without intentional decommission/re-enrollment.
- local socket permission denied: use root or start new session after joining `silent-devops-admin`.
- agent offline: inspect agent journal, validator CIDR policy, DNS, TLS time validity, and revocation state.

## Uninstall

```sh
sudo ./uninstall.sh
```

Preserve validator DB/CA and agent credentials unless data destruction or re-enrollment is intentional.
