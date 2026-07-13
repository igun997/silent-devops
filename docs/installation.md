# Installation and instance setup

## 1. Download release

Choose target architecture:

```sh
arch=$(dpkg --print-architecture) # amd64 or arm64
version=v0.1.0
curl -fLO "https://github.com/igun997/silent-devops/releases/download/${version}/silent-devops-linux-${arch}.tar.gz"
tar -xzf "silent-devops-linux-${arch}.tar.gz"
cd "silent-devops-linux-${arch}"
```

Replace version with published release tag.

## 2. Install binaries and units

```sh
sudo ./install.sh
```

Installer copies binaries and systemd units, then runs `systemctl daemon-reload`. It does not enable services because credentials and CIDR policy must exist first.

## 3. Validator instance

Use dedicated, restricted instance. Allow validator API only from configured client and agent networks. Keep agent-signing CA encrypted outside SQLite.

Required environment keys:

```text
SILENT_DEVOPS_LISTEN
SILENT_DEVOPS_DB
SILENT_DEVOPS_TLS_CERT
SILENT_DEVOPS_TLS_KEY
SILENT_DEVOPS_CLIENT_CA
SILENT_DEVOPS_AGENT_CA
SILENT_DEVOPS_AGENT_CA_PASSPHRASE
SILENT_DEVOPS_TOKEN_KEY
SILENT_DEVOPS_ENROLL_CIDRS
SILENT_DEVOPS_AGENT_CIDRS
SILENT_DEVOPS_CLIENT_CIDRS
SILENT_DEVOPS_BOOTSTRAP_USER
SILENT_DEVOPS_BOOTSTRAP_PASSWORD
```

Create protected systemd override or use systemd credentials for secrets:

```sh
sudo systemctl edit silent-devops-validator
sudo systemctl enable --now silent-devops-validator
sudo systemctl status silent-devops-validator
```

Remove bootstrap password configuration after first successful bootstrap. Back up SQLite and encrypted CA separately. Test restore procedure.

## 4. Client workstation

Configure:

```sh
export SILENT_DEVOPS_VALIDATOR=validator.example.com:8443
export SILENT_DEVOPS_VALIDATOR_CA=$HOME/.config/silent-devops/validator-ca.crt
export SILENT_DEVOPS_SERVER_NAME=validator.example.com
```

Login token is stored under `$HOME/.config/silent-devops/` with restricted permissions.

## 5. Agent instance

Create enrollment token as admin. Transfer only token and validator trust material to target host. Agent generates private key locally; never copy private key from validator.

Required agent environment keys:

```text
SILENT_DEVOPS_VALIDATOR
SILENT_DEVOPS_CREDENTIAL_DIR
```

Reverse SSH additionally needs:

```text
SILENT_DEVOPS_TUNNEL_HOST
SILENT_DEVOPS_TUNNEL_USER
SILENT_DEVOPS_TUNNEL_KEY
SILENT_DEVOPS_TUNNEL_KNOWN_HOSTS
SILENT_DEVOPS_AUTHORIZED_KEYS
```

After enrollment credentials exist:

```sh
sudo systemctl edit silent-devops-agent
sudo systemctl enable --now silent-devops-agent
sudo systemctl status silent-devops-agent
```

Agent requires root for intended maintenance operations. It opens outbound connections only. Do not add inbound agent control rules.

## 6. Verify

```sh
silent-devops-client login ADMIN
silent-devops-client agents list --json
silent-devops-client agents show AGENT_ID --json
```

Then verify:

- agent appears online
- metrics update
- viewer cannot operate
- operator can run typed maintenance but not arbitrary command
- admin action records audit metadata
- revoked agent cannot reconnect
- restart does not replay uncertain destructive work

Follow [VM smoke test](vm-smoke-test.md) before deployment beyond evaluation.

## Upgrade

1. Back up SQLite and encrypted CA.
2. Download new release for architecture.
3. Stop affected service.
4. Replace binaries with `install.sh`.
5. Start service and inspect logs.
6. Verify agents reconnect and no destructive job replays.

## Uninstall

```sh
sudo ./uninstall.sh
```

Review script before use. Preserve state and CA backups unless intentional data destruction is confirmed.
