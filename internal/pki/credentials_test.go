package pki_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"silent-devops/internal/pki"
)

func TestAgentCredentialsStayRootOnlyAndReload(t *testing.T) {
	dir := t.TempDir()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	want := pki.AgentCredentials{AgentID: "agent-1", PrivateKey: key, CertificatePEM: []byte("certificate"), CAPEM: []byte("ca")}
	if err := pki.SaveAgentCredentials(dir, want); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"agent.key", "agent.crt", "validator-ca.crt", "agent-id"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("%s mode=%o", name, info.Mode().Perm())
		}
	}
	got, err := pki.LoadAgentCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID != want.AgentID || string(got.PrivateKey) != string(want.PrivateKey) {
		t.Fatal("credential round trip mismatch")
	}
}

func TestValidatorPin(t *testing.T) {
	pin := pki.CertificatePin([]byte("validator certificate DER"))
	if err := pki.VerifyCertificatePin([]byte("validator certificate DER"), pin); err != nil {
		t.Fatal(err)
	}
	if err := pki.VerifyCertificatePin([]byte("other"), pin); err == nil {
		t.Fatal("wrong pin accepted")
	}
}
