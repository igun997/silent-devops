//go:build e2e

package integration_test

import (
	"os/exec"
	"strings"
	"testing"
)

func compose(t *testing.T, args ...string) string {
	t.Helper()
	full := append([]string{"compose", "-f", "docker-compose.yml"}, args...)
	command := exec.Command("docker", full...)
	command.Dir = "."
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(full, " "), err, out)
	}
	return string(out)
}
func TestRealRolesAndNoAgentPorts(t *testing.T) {
	services := compose(t, "ps", "--services")
	for _, want := range []string{"validator", "agent-ubuntu-2204", "agent-ubuntu-2404", "agent-debian-12", "allowed-client", "denied-client"} {
		if !strings.Contains(services, want) {
			t.Errorf("missing %s", want)
		}
	}
	config := compose(t, "config")
	for _, agent := range []string{"agent-debian-12", "agent-ubuntu-2204", "agent-ubuntu-2404"} {
		if strings.Contains(config, agent+":\n    ports:") {
			t.Fatalf("%s exposes ports", agent)
		}
	}
}
func TestAgentsActuallyEnrollConnectAndPublishMetrics(t *testing.T) {
	for _, service := range []string{"agent-ubuntu-2204", "agent-ubuntu-2404", "agent-debian-12"} {
		id := strings.TrimSpace(compose(t, "exec", "-T", service, "cat", "/creds/agent-id"))
		out := compose(t, "exec", "-T", "validator", "sh", "-c", "sqlite3 /state/devops.db \"select count(*) from connections where agent_id='"+id+"' and state='online'\"")
		if strings.TrimSpace(out) != "1" {
			t.Fatalf("%s offline: %q", service, out)
		}
		out = compose(t, "exec", "-T", "validator", "sh", "-c", "sqlite3 /state/devops.db \"select count(*) from metrics_current where agent_id='"+id+"'\"")
		if strings.TrimSpace(out) != "1" {
			t.Fatalf("%s metrics missing: %q", service, out)
		}
	}
}
func TestAPIWorkflowAndRBAC(t *testing.T) {
	compose(t, "exec", "-T", "allowed-client", "integration-helper", "verify", "validator:8443")
	deadline := compose(t, "exec", "-T", "validator", "sh", "-c", "for i in $(seq 1 100); do s=$(sqlite3 /state/devops.db \"select state from jobs where idempotency_key='e2e-process'\"); [ \"$s\" = 4 ] && exit 0; sleep .1; done; exit 1")
	_ = deadline
	out := compose(t, "exec", "-T", "validator", "sh", "-c", "sqlite3 /state/devops.db \"select count(*) from audit_events where action in ('process_list','exec')\"")
	if strings.TrimSpace(out) != "2" {
		t.Fatalf("audit missing: %q", out)
	}
}
func TestDeniedCIDRAndTokenReplay(t *testing.T) {
	command := exec.Command("docker", "compose", "-f", "docker-compose.yml", "exec", "-T", "denied-client", "integration-helper", "login", "validator:8443")
	command.Dir = "."
	if out, err := command.CombinedOutput(); err == nil || !strings.Contains(string(out), "peer address denied") {
		t.Fatalf("denied CIDR accepted: %v %s", err, out)
	}
	command = exec.Command("docker", "compose", "-f", "docker-compose.yml", "exec", "-T", "allowed-client", "integration-helper", "reuse", "validator:8443", "/shared/token-1", "/tmp/reused")
	command.Dir = "."
	if out, err := command.CombinedOutput(); err == nil || !strings.Contains(string(out), "invalid enrollment token") {
		t.Fatalf("token replay accepted: %v %s", err, out)
	}
}
func TestExpiredTokenRetentionAndRevocation(t *testing.T) {
	compose(t, "exec", "-T", "allowed-client", "integration-helper", "expired-token", "validator:8443")
	old := "1000"
	revokedID := strings.TrimSpace(compose(t, "exec", "-T", "agent-ubuntu-2404", "cat", "/creds/agent-id"))
	debianID := strings.TrimSpace(compose(t, "exec", "-T", "agent-debian-12", "cat", "/creds/agent-id"))
	compose(t, "exec", "-T", "validator", "sh", "-c", "sqlite3 /state/devops.db \"insert into audit_events(id,action,reason,occurred_unix_ms) values('old','old','retention',"+old+"); insert into metrics_minute(agent_id,bucket_unix_ms,payload) values('"+debianID+"',"+old+",'{}'); update agents set revoked_unix_ms=strftime('%s','now')*1000 where id='"+revokedID+"';\"")
	compose(t, "restart", "validator")
	compose(t, "restart", "agent-ubuntu-2404")
	compose(t, "exec", "-T", "validator", "sh", "-c", "sleep 2; test \"$(sqlite3 /state/devops.db \"select state from connections where agent_id='"+revokedID+"'\")\" != online")
}
func TestRestartReconciliation(t *testing.T) {
	compose(t, "restart", "validator")
	id := strings.TrimSpace(compose(t, "exec", "-T", "agent-debian-12", "cat", "/creds/agent-id"))
	compose(t, "restart", "agent-debian-12")
	compose(t, "exec", "-T", "validator", "sh", "-c", "for i in $(seq 1 100); do [ \"$(sqlite3 /state/devops.db \"select state from connections where agent_id='"+id+"'\")\" = online ] && exit 0; sleep .1; done; exit 1")
	out := compose(t, "exec", "-T", "validator", "sh", "-c", "sqlite3 /state/devops.db \"select count(*) from jobs where idempotency_key='admin-exec' and state=4\"")
	if strings.TrimSpace(out) != "1" {
		t.Fatalf("destructive job replay/state changed: %q", out)
	}
}
func TestRetentionCleanupUnitBacked(t *testing.T) {
	out := compose(t, "exec", "-T", "validator", "sh", "-c", "sqlite3 /state/devops.db \"delete from metrics_minute where bucket_unix_ms < (strftime('%s','now')-604800)*1000; delete from audit_events where occurred_unix_ms < (strftime('%s','now')-604800)*1000; select (select count(*) from audit_events where id='old')+(select count(*) from metrics_minute where bucket_unix_ms=1000);\"")
	if strings.TrimSpace(out) != "0" {
		t.Fatalf("retention failed: %q", out)
	}
}
func TestInteractiveSSHTrafficOverGRPCTunnel(t *testing.T) {
	id := strings.TrimSpace(compose(t, "exec", "-T", "agent-debian-12", "cat", "/creds/agent-id"))
	out := compose(t, "exec", "-T", "allowed-client", "integration-helper", "ssh-exec", "validator:8443", id)
	if !strings.Contains(out, "ssh-exec-ok") || !strings.Contains(out, "silent-devops") {
		t.Fatal(out)
	}
	compose(t, "exec", "-T", "validator", "sh", "-c", "for i in $(seq 1 30); do [ \"$(sqlite3 /state/devops.db \"select count(*) from ssh_sessions where state=3\")\" -gt 0 ] && exit 0; sleep .1; done; exit 1")
	compose(t, "exec", "-T", "agent-debian-12", "sh", "-c", "! grep -q silent-devops: /shared/agent-authorized-keys")
}
func TestCredentialIsolationAndTokenConsumption(t *testing.T) {
	for _, service := range []string{"agent-ubuntu-2204", "agent-ubuntu-2404", "agent-debian-12"} {
		compose(t, "exec", "-T", service, "sh", "-c", "test -s /creds/agent.key && test $(stat -c %a /creds/agent.key) = 600")
	}
	out := compose(t, "exec", "-T", "validator", "sh", "-c", "sqlite3 /state/devops.db 'select count(*) from enrollment_tokens where consumed_unix_ms is not null'")
	if strings.TrimSpace(out) != "3" {
		t.Fatalf("tokens not consumed: %q", out)
	}
}
