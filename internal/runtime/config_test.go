package runtime_test

import (
	"testing"

	"silent-devops/internal/runtime"
)

func TestValidatorConfigRequiresSecureInputs(t *testing.T) {
	env := map[string]string{"SILENT_DEVOPS_LISTEN": "127.0.0.1:8443", "SILENT_DEVOPS_DB": "/tmp/db", "SILENT_DEVOPS_TLS_CERT": "cert", "SILENT_DEVOPS_TLS_KEY": "key", "SILENT_DEVOPS_CLIENT_CA": "ca", "SILENT_DEVOPS_AGENT_CA": "agent-ca", "SILENT_DEVOPS_AGENT_CA_PASSPHRASE": "long test passphrase", "SILENT_DEVOPS_TOKEN_KEY": "0123456789abcdef0123456789abcdef", "SILENT_DEVOPS_ENROLL_CIDRS": "10.0.0.0/8", "SILENT_DEVOPS_AGENT_CIDRS": "10.0.0.0/8", "SILENT_DEVOPS_CLIENT_CIDRS": "127.0.0.0/8"}
	cfg, err := runtime.ParseValidatorConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:8443" {
		t.Fatalf("listen=%q", cfg.Listen)
	}
	delete(env, "SILENT_DEVOPS_CLIENT_CA")
	if _, err := runtime.ParseValidatorConfig(func(k string) string { return env[k] }); err == nil {
		t.Fatal("missing client CA accepted")
	}
}
func TestAgentConfigRequiresCredentials(t *testing.T) {
	env := map[string]string{"SILENT_DEVOPS_VALIDATOR": "validator:8443", "SILENT_DEVOPS_CREDENTIAL_DIR": "/state"}
	if _, err := runtime.ParseAgentConfig(func(k string) string { return env[k] }); err != nil {
		t.Fatal(err)
	}
	delete(env, "SILENT_DEVOPS_CREDENTIAL_DIR")
	if _, err := runtime.ParseAgentConfig(func(k string) string { return env[k] }); err == nil {
		t.Fatal("missing credential dir accepted")
	}
}
