package validatorinit

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"silent-devops/internal/pki"
	"strings"
	"time"
)

type Options struct{ PublicAddress, Dir, AgentCIDRs string }
type Result struct{ Pin string }

func Init(o Options) (Result, error) {
	if o.PublicAddress == "" || o.Dir == "" {
		return Result{}, errors.New("public address and directory required")
	}
	if _, err := os.Stat(filepath.Join(o.Dir, "validator.env")); err == nil {
		return Result{}, errors.New("validator already initialized")
	}
	if o.AgentCIDRs == "" {
		o.AgentCIDRs = "0.0.0.0/0,::/0"
	}
	if err := os.MkdirAll(o.Dir, 0700); err != nil {
		return Result{}, err
	}
	host, port, err := net.SplitHostPort(o.PublicAddress)
	if err != nil {
		return Result{}, err
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Result{}, err
	}
	now := time.Now()
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if ip := net.ParseIP(host); ip != nil {
		cert.IPAddresses = []net.IP{ip}
	} else {
		cert.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, pub, key)
	if err != nil {
		return Result{}, err
	}
	raw, _ := x509.MarshalPKCS8PrivateKey(key)
	caPass, err := random(32)
	if err != nil {
		return Result{}, err
	}
	tokenKey, err := random(32)
	if err != nil {
		return Result{}, err
	}
	files := map[string][]byte{"server.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), "server.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(o.Dir, name), data, 0600); err != nil {
			return Result{}, err
		}
	}
	if _, err := pki.CreateCA(filepath.Join(o.Dir, "agent-ca.key"), []byte(caPass), now); err != nil {
		return Result{}, err
	}
	env := strings.Join([]string{"SILENT_DEVOPS_LISTEN=0.0.0.0:" + port, "SILENT_DEVOPS_DB=/var/lib/silent-devops/devops.db", "SILENT_DEVOPS_TLS_CERT=" + filepath.Join(o.Dir, "server.crt"), "SILENT_DEVOPS_TLS_KEY=" + filepath.Join(o.Dir, "server.key"), "SILENT_DEVOPS_CLIENT_CA=" + filepath.Join(o.Dir, "server.crt"), "SILENT_DEVOPS_AGENT_CA=" + filepath.Join(o.Dir, "agent-ca.key"), "SILENT_DEVOPS_AGENT_CA_PASSPHRASE=" + caPass, "SILENT_DEVOPS_TOKEN_KEY=" + tokenKey, "SILENT_DEVOPS_ENROLL_CIDRS=" + o.AgentCIDRs, "SILENT_DEVOPS_AGENT_CIDRS=" + o.AgentCIDRs, "SILENT_DEVOPS_CLIENT_CIDRS=127.0.0.1/32,::1/128"}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(o.Dir, "validator.env"), []byte(env), 0600); err != nil {
		return Result{}, err
	}
	return Result{Pin: pki.CertificatePin(der)}, nil
}
func random(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
