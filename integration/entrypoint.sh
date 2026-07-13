#!/bin/sh
set -eu
case "${ROLE:-}" in
  validator)
    getent group silent-devops-admin >/dev/null 2>&1 || groupadd --system silent-devops-admin
    mkdir -p /state /shared
    if [ ! -s /shared/server.crt ]; then integration-helper init /shared; fi
    exec validator
    ;;
  agent)
    mkdir -p "$SILENT_DEVOPS_CREDENTIAL_DIR"
    if [ -n "${SILENT_DEVOPS_AUTHORIZED_KEYS:-}" ]; then
      ssh-keygen -A
      mkdir -p /run/sshd
      useradd --create-home --shell /bin/sh silent-devops 2>/dev/null || true
      : > "${SILENT_DEVOPS_AUTHORIZED_KEYS}"
      printf 'StrictModes no\nAuthorizedKeysFile %s\nPubkeyAuthentication yes\nPasswordAuthentication no\nLogLevel DEBUG3\n' "${SILENT_DEVOPS_AUTHORIZED_KEYS}" > /etc/ssh/sshd_config.d/silent-devops.conf
      /usr/sbin/sshd -e
    fi
    if [ -s "$SILENT_DEVOPS_CREDENTIAL_DIR/agent.crt" ]; then exec agent; fi
    until [ -f /shared/server.crt ] && integration-helper enroll "$SILENT_DEVOPS_VALIDATOR" "$(cat "$ENROLL_TOKEN_FILE")" "$AGENT_ID" "$SILENT_DEVOPS_CREDENTIAL_DIR"; do sleep 1; done
    exec agent
    ;;
  client) exec tail -f /dev/null ;;
  *) echo "unknown ROLE" >&2; exit 2 ;;
esac
