package pki_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"silent-devops/internal/pki"
	"silent-devops/internal/store"
)

func TestEncryptedCAAndAgentCertificate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-ca.pem")
	ca, err := pki.CreateCA(path, []byte("long test passphrase"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if string(data) == "" || !contains(data, []byte("ENCRYPTED PRIVATE KEY")) {
		t.Fatal("CA key not encrypted")
	}
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	certPEM, serial, err := ca.SignAgentCSR(csrDER, "agent-1", time.Now(), time.Hour)
	if err != nil || serial == "" {
		t.Fatal(err)
	}
	cert := parseCert(t, certPEM)
	if cert.Subject.CommonName != "agent-1" {
		t.Fatalf("CN=%q", cert.Subject.CommonName)
	}
}

func TestLoadCARejectsInsecurePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if _, err := pki.CreateCA(path, []byte("long test passphrase"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := pki.LoadCA(path, []byte("long test passphrase")); err == nil {
		t.Fatal("insecure permissions accepted")
	}
}

func TestEnrollmentTokenSingleUseConcurrent(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mgr := pki.NewEnrollmentManager(s.DB())
	token, err := mgr.CreateToken(context.Background(), "", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if mgr.ConsumeToken(context.Background(), token, time.Now()) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successes=%d", successes.Load())
	}
}

func contains(haystack, needle []byte) bool { return bytes.Contains(haystack, needle) }
func pkixName(commonName string) pkix.Name  { return pkix.Name{CommonName: commonName} }
func parseCert(t *testing.T, data []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("missing certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestEnrollmentTokenExpiry(t *testing.T) {
	s, _ := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	defer s.Close()
	mgr := pki.NewEnrollmentManager(s.DB())
	token, _ := mgr.CreateToken(context.Background(), "", time.Now(), time.Second)
	if err := mgr.ConsumeToken(context.Background(), token, time.Now().Add(2*time.Second)); err == nil {
		t.Fatal("expired token accepted")
	}
}
