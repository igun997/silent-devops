#!/bin/sh
set -eu
case "${ROLE:-}" in
  validator)
    mkdir -p /state /shared
    if [ ! -s /shared/server.crt ]; then integration-helper init /shared; fi
    exec validator
    ;;
  agent)
    mkdir -p "$SILENT_DEVOPS_CREDENTIAL_DIR"
    if [ -s "$SILENT_DEVOPS_CREDENTIAL_DIR/agent.crt" ]; then exec agent; fi
    until [ -f /shared/server.crt ] && integration-helper enroll "$SILENT_DEVOPS_VALIDATOR" "$(cat "$ENROLL_TOKEN_FILE")" "$AGENT_ID" "$SILENT_DEVOPS_CREDENTIAL_DIR"; do sleep 1; done
    exec agent
    ;;
  client) exec tail -f /dev/null ;;
  sshd-check)
    useradd --system --home-dir /var/lib/silent-devops/tunnel --shell /usr/sbin/nologin silent-tunnel 2>/dev/null || true
    mkdir -p /var/lib/silent-devops/tunnel /run/sshd
    touch /var/lib/silent-devops/tunnel/authorized_keys
    chmod 0600 /var/lib/silent-devops/tunnel/authorized_keys
    exec /usr/sbin/sshd -D -e -f /etc/ssh/sshd_config.silent-devops
    ;;
  *) echo "unknown ROLE" >&2; exit 2 ;;
esac
