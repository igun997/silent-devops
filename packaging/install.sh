#!/bin/sh
set -eu
role=${1:-all}
prefix=${PREFIX:-/usr/local}
state=${STATE_DIR:-/var/lib/silent-devops}
arch=$(dpkg --print-architecture)
case "$role" in
  agent) install -d -m 0700 "$state/agent"; install -m 0755 "bin/agent-linux-$arch" "$prefix/sbin/silent-devops-agent"; install -m 0644 packaging/systemd/silent-devops-agent.service /etc/systemd/system/ ;;
  validator) getent group silent-devops-admin >/dev/null || groupadd --system silent-devops-admin; id silent-devops >/dev/null 2>&1 || useradd --system --home-dir "$state" --shell /usr/sbin/nologin silent-devops; install -d -o silent-devops -g silent-devops -m 0700 "$state"; install -d -o silent-devops -g silent-devops-admin -m 0750 /run/silent-devops; install -m 0755 "bin/validator-linux-$arch" "$prefix/sbin/silent-devops-validator"; install -m 0644 packaging/systemd/silent-devops-validator.service /etc/systemd/system/ ;;
  client) install -m 0755 "bin/client-linux-$arch" "$prefix/bin/silent-devops-client" ;;
  all) "$0" validator; "$0" agent; "$0" client; exit 0 ;;
  *) echo "usage: $0 [validator|agent|client|all]" >&2; exit 2 ;;
esac
systemctl daemon-reload
