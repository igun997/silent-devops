package server_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"path/filepath"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/pki"
	"silent-devops/internal/server"
	"silent-devops/internal/store"
)

func TestEnrollConsumesTokenAndRegistersIdentity(t *testing.T) {
	now := time.Now()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ca, err := pki.CreateCA(filepath.Join(t.TempDir(), "ca.pem"), []byte("long test passphrase"), now)
	if err != nil {
		t.Fatal(err)
	}
	tokens := pki.NewEnrollmentManager(s.DB())
	token, err := tokens.CreateToken(context.Background(), "", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "agent-1"}}, key)
	srv := server.Enrollment{CA: ca, Tokens: tokens, Identities: pki.NewIdentityRegistry(s.DB()), Now: func() time.Time { return now }, Validity: time.Hour}
	response, err := srv.Enroll(context.Background(), &devopsv1.EnrollRequest{Token: token, CsrDer: csr, Hostname: "host"})
	if err != nil {
		t.Fatal(err)
	}
	if response.AgentId != "agent-1" || len(response.CertificatePem) == 0 {
		t.Fatal("bad response")
	}
	if _, err := srv.Enroll(context.Background(), &devopsv1.EnrollRequest{Token: token, CsrDer: csr}); err == nil {
		t.Fatal("token reused")
	}
}
