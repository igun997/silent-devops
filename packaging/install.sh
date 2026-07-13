#!/bin/sh
set -eu
prefix=${PREFIX:-/usr/local}
state=${STATE_DIR:-/var/lib/silent-devops}
install -d -m 0700 "$state"
arch=$(dpkg --print-architecture)
install -m 0755 "bin/agent-linux-$arch" "$prefix/sbin/silent-devops-agent"
install -m 0755 "bin/validator-linux-$arch" "$prefix/sbin/silent-devops-validator"
install -m 0755 "bin/client-linux-$arch" "$prefix/bin/silent-devops-client"
install -m 0644 packaging/systemd/silent-devops-agent.service /etc/systemd/system/
install -m 0644 packaging/systemd/silent-devops-validator.service /etc/systemd/system/
systemctl daemon-reload
