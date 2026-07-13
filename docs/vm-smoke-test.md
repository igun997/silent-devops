# Production VM smoke test

Run on disposable Ubuntu 22.04, Ubuntu 24.04, and Debian 12 VMs for amd64 and arm64:

1. Install units and verify `systemd-analyze security` output; confirm root agent still accesses systemd, journal, cleanup allowlist, reboot fixture, and temporary SSH key path.
2. Enroll through allowed CIDR; reject denied CIDR, expired token, reused token, wrong pin, wrong CSR identity.
3. Verify agent has no listening control port and reconnects outbound after validator restart.
4. Collect inventory, CPU, memory, load, filesystem, network, uptime; reboot and verify counter reset/boot ID behavior.
5. Exercise viewer/operator/admin negative matrix.
6. Run process, service, journal, cleanup preview/run fixtures. Confirm cleanup cannot escape allowlist.
7. Interrupt service action, cleanup, reboot, and arbitrary command at each state; prove no destructive replay.
8. Open SSH session using client-held private key. Verify validator listener is loopback-only; expire/cancel/disconnect and confirm process/key/port cleanup.
9. Revoke agent and prove active disconnect plus renewal/reconnect failure.
10. Backup/restore DB, run integrity check, verify audit retention and secret absence.
11. Upgrade binaries, restart, verify migration compatibility and no destructive replay.
12. Uninstall and confirm units, users, firewall rules, keys, sessions, ports, and state removal.
