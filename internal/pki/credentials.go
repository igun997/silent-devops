package pki

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type AgentCredentials struct {
	AgentID               string
	PrivateKey            ed25519.PrivateKey
	CertificatePEM, CAPEM []byte
}

func SaveAgentCredentials(dir string, credentials AgentCredentials) error {
	if credentials.AgentID == "" || len(credentials.PrivateKey) != ed25519.PrivateKeySize || len(credentials.CertificatePEM) == 0 || len(credentials.CAPEM) == 0 {
		return errors.New("incomplete agent credentials")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(credentials.PrivateKey)
	if err != nil {
		return err
	}
	files := map[string][]byte{"agent.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), "agent.crt": credentials.CertificatePEM, "validator-ca.crt": credentials.CAPEM, "agent-id": []byte(credentials.AgentID + "\n")}
	for name, data := range files {
		if err := atomicWrite(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
	}
	return nil
}
func LoadAgentCredentials(dir string) (AgentCredentials, error) {
	var result AgentCredentials
	id, err := os.ReadFile(filepath.Join(dir, "agent-id"))
	if err != nil {
		return result, err
	}
	result.AgentID = string(id)
	if len(result.AgentID) > 0 && result.AgentID[len(result.AgentID)-1] == '\n' {
		result.AgentID = result.AgentID[:len(result.AgentID)-1]
	}
	keyPEM, err := os.ReadFile(filepath.Join(dir, "agent.key"))
	if err != nil {
		return result, err
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return result, errors.New("invalid private key")
	}
	raw, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return result, err
	}
	key, ok := raw.(ed25519.PrivateKey)
	if !ok {
		return result, errors.New("unsupported private key")
	}
	result.PrivateKey = key
	result.CertificatePEM, err = os.ReadFile(filepath.Join(dir, "agent.crt"))
	if err != nil {
		return result, err
	}
	result.CAPEM, err = os.ReadFile(filepath.Join(dir, "validator-ca.crt"))
	return result, err
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credential-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
func CertificatePin(certificateDER []byte) string {
	sum := sha256.Sum256(certificateDER)
	return "sha256/" + base64.RawStdEncoding.EncodeToString(sum[:])
}
func VerifyCertificatePin(certificateDER []byte, pin string) error {
	got := CertificatePin(certificateDER)
	if len(got) != len(pin) || subtle.ConstantTimeCompare([]byte(got), []byte(pin)) != 1 {
		return fmt.Errorf("validator certificate pin mismatch")
	}
	return nil
}
