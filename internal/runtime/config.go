package runtime

import (
	"errors"
	"strings"
	"time"

	"silent-devops/internal/auth"
)

type ValidatorConfig struct {
	Listen, DB, TLSCert, TLSKey, ClientCA, AgentCA, AgentCAPassphrase string
	TokenKey                                                          []byte
	Policies                                                          auth.EndpointPolicies
	BootstrapUser, BootstrapPassword                                  string
}

func ParseValidatorConfig(getenv func(string) string) (ValidatorConfig, error) {
	cfg := ValidatorConfig{Listen: getenv("SILENT_DEVOPS_LISTEN"), DB: getenv("SILENT_DEVOPS_DB"), TLSCert: getenv("SILENT_DEVOPS_TLS_CERT"), TLSKey: getenv("SILENT_DEVOPS_TLS_KEY"), ClientCA: getenv("SILENT_DEVOPS_CLIENT_CA"), AgentCA: getenv("SILENT_DEVOPS_AGENT_CA"), AgentCAPassphrase: getenv("SILENT_DEVOPS_AGENT_CA_PASSPHRASE"), TokenKey: []byte(getenv("SILENT_DEVOPS_TOKEN_KEY")), BootstrapUser: getenv("SILENT_DEVOPS_BOOTSTRAP_USER"), BootstrapPassword: getenv("SILENT_DEVOPS_BOOTSTRAP_PASSWORD")}
	if cfg.Listen == "" || cfg.DB == "" || cfg.TLSCert == "" || cfg.TLSKey == "" || cfg.ClientCA == "" || cfg.AgentCA == "" || len(cfg.AgentCAPassphrase) < 16 || len(cfg.TokenKey) < 32 {
		return cfg, errors.New("validator listen, DB, TLS certificate/key, client CA, encrypted agent CA/passphrase, and 32-byte token key required")
	}
	var err error
	if cfg.Policies.Enrollment, err = auth.ParseCIDRs(split(getenv("SILENT_DEVOPS_ENROLL_CIDRS"))); err != nil {
		return cfg, err
	}
	if cfg.Policies.Agent, err = auth.ParseCIDRs(split(getenv("SILENT_DEVOPS_AGENT_CIDRS"))); err != nil {
		return cfg, err
	}
	if cfg.Policies.Client, err = auth.ParseCIDRs(split(getenv("SILENT_DEVOPS_CLIENT_CIDRS"))); err != nil {
		return cfg, err
	}
	return cfg, nil
}

type AgentConfig struct {
	Validator, CredentialDir                                            string
	TunnelHost, TunnelUser, TunnelKey, TunnelKnownHosts, AuthorizedKeys string
	Heartbeat                                                           time.Duration
}

func ParseAgentConfig(getenv func(string) string) (AgentConfig, error) {
	cfg := AgentConfig{Validator: getenv("SILENT_DEVOPS_VALIDATOR"), CredentialDir: getenv("SILENT_DEVOPS_CREDENTIAL_DIR"), TunnelHost: getenv("SILENT_DEVOPS_TUNNEL_HOST"), TunnelUser: getenv("SILENT_DEVOPS_TUNNEL_USER"), TunnelKey: getenv("SILENT_DEVOPS_TUNNEL_KEY"), TunnelKnownHosts: getenv("SILENT_DEVOPS_TUNNEL_KNOWN_HOSTS"), AuthorizedKeys: getenv("SILENT_DEVOPS_AUTHORIZED_KEYS"), Heartbeat: 15 * time.Second}
	if cfg.Validator == "" || cfg.CredentialDir == "" {
		return cfg, errors.New("validator address and credential directory required")
	}
	return cfg, nil
}
func split(value string) []string {
	var out []string
	for _, v := range strings.Split(value, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
