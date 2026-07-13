package agentjoin_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/agentjoin"
	"silent-devops/internal/pki"
	"testing"
	"time"
)

func TestJoinRequiresPinBeforeEnrollment(t *testing.T) {
	called := false
	err := agentjoin.Join(context.Background(), agentjoin.Options{Address: "validator:8443", Token: "secret", CredentialDir: t.TempDir(), Probe: func(context.Context, string) ([]byte, error) { return []byte("certificate"), nil }, Enroll: func(context.Context, string, *devopsv1.EnrollRequest) (*devopsv1.EnrollResponse, error) {
		called = true
		return nil, nil
	}})
	if err == nil || called {
		t.Fatal("missing pin sent enrollment")
	}
}
func TestJoinGeneratesKeyAndStoresValidatorAssignedIdentity(t *testing.T) {
	dir := t.TempDir()
	certDER, certPEM, caPEM := certificate(t, "assigned-id")
	pin := pki.CertificatePin([]byte("validator"))
	err := agentjoin.Join(context.Background(), agentjoin.Options{Address: "validator:8443", Token: "secret", Pin: pin, CredentialDir: dir, Hostname: "host", NoStart: true, Probe: func(context.Context, string) ([]byte, error) { return []byte("validator"), nil }, Enroll: func(_ context.Context, _ string, r *devopsv1.EnrollRequest) (*devopsv1.EnrollResponse, error) {
		csr, err := x509.ParseCertificateRequest(r.CsrDer)
		if err != nil || csr.Subject.CommonName != "" || r.Token != "secret" {
			t.Fatal("bad enrollment request")
		}
		return &devopsv1.EnrollResponse{AgentId: "assigned-id", CertificatePem: certPEM, CaCertificatePem: caPEM, ExpiresUnixMs: time.Now().Add(time.Hour).UnixMilli()}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := pki.LoadAgentCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.AgentID != "assigned-id" || len(credentials.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatal("credentials incomplete")
	}
	block, _ := pem.Decode(credentials.CertificatePEM)
	if string(block.Bytes) != string(certDER) {
		t.Fatal("certificate mismatch")
	}
	if info, _ := os.Stat(filepath.Join(dir, "agent.key")); info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	if err := agentjoin.Join(context.Background(), agentjoin.Options{Address: "x", Token: "x", Pin: pin, CredentialDir: dir}); err == nil {
		t.Fatal("existing credentials overwritten")
	}
}
func certificate(t *testing.T, id string) ([]byte, []byte, []byte) {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	template := &x509.Certificate{SerialNumber: new(big.Int).SetInt64(2), Subject: pkix.Name{CommonName: id}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return der, encoded, encoded
}
