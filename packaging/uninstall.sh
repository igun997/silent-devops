#!/bin/sh
set -eu
prefix=${PREFIX:-/usr/local}
systemctl disable --now silent-devops-agent.service silent-devops-validator.service 2>/dev/null || true
rm -f /etc/systemd/system/silent-devops-agent.service /etc/systemd/system/silent-devops-validator.service
rm -f "$prefix/sbin/silent-devops-agent" "$prefix/sbin/silent-devops-validator" "$prefix/bin/silent-devops-client"
systemctl daemon-reload
printf '%s\n' 'State and accounts retained. Revoke fleet, verify sessions/keys/firewall, then remove /var/lib/silent-devops and service accounts manually.'
