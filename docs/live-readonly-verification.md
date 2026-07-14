# Live read-only verification

Date: 2026-07-13

Targets: validator `175.176.161.234:8443`, agent `f6d8ba62dabb826272054504e55f2764`, client workstation. Secrets omitted.

## Passed

- Remote login: exit 0
- `agents list --json`: exit 0, enrolled agent returned
- `agents show … --json`: exit 0, expected hostname/ID returned
- `stats … --json`: exit 0
- `users list --json`: exit 0, admin inventory returned
- `ssh-keys list --json`: exit 0
- `audit --json`: exit 0, read requests audited
- Invalid/expired prior token: rejected with `Unauthenticated`

## Blocked by live runtime state

Agent systemd process is active, but validator registry reports it offline after validator restart. Consequently service list/status, logs, and cleanup preview fail closed with `Unavailable: agent offline or backpressured`. No service restart was performed because live verification is read-only. This exposes missing agent reconnect behavior and must be fixed before production completion.

## Explicitly not executed

No reboot, deletion, cleanup run, service start/stop/restart, arbitrary exec, SSH session, key/user mutation, agent revocation, or token revocation was performed.
