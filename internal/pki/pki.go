package pki

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	"golang.org/x/crypto/argon2"
)

type CA struct {
	cert    *x509.Certificate
	key     ed25519.PrivateKey
	certPEM []byte
}

func (ca *CA) CertificatePEM() []byte { return append([]byte(nil), ca.certPEM...) }

func CreateCA(path string, passphrase []byte, now time.Time) (*CA, error) {
	if len(passphrase) < 16 {
		return nil, errors.New("CA passphrase must be at least 16 bytes")
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{SerialNumber: randomSerial(), Subject: pkix.Name{CommonName: "Silent DevOps Agent CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, key)
	if err != nil {
		return nil, err
	}
	cert, _ := x509.ParseCertificate(der)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	encrypted, err := x509.EncryptPEMBlock(rand.Reader, "ENCRYPTED PRIVATE KEY", keyDER, passphrase, x509.PEMCipherAES256)
	if err != nil {
		return nil, err
	}
	contents := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(encrypted)...)
	if err := os.WriteFile(path, contents, 0600); err != nil {
		return nil, err
	}
	return &CA{cert: cert, key: key, certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, nil
}

func LoadCA(path string, passphrase []byte) (*CA, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("CA key permissions %o: require 0600 or stricter", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	certBlock, rest := pem.Decode(data)
	keyBlock, _ := pem.Decode(rest)
	if certBlock == nil || keyBlock == nil || keyBlock.Type != "ENCRYPTED PRIVATE KEY" {
		return nil, errors.New("invalid encrypted CA file")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.DecryptPEMBlock(keyBlock, passphrase)
	if err != nil {
		return nil, err
	}
	raw, err := x509.ParsePKCS8PrivateKey(keyDER)
	if err != nil {
		return nil, err
	}
	key, ok := raw.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("unsupported CA key")
	}
	return &CA{cert: cert, key: key, certPEM: pem.EncodeToMemory(certBlock)}, nil
}

func (ca *CA) SignAgentCSR(csrDER []byte, agentID string, now time.Time, validity time.Duration) ([]byte, string, error) {
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, "", err
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, "", err
	}
	if agentID == "" || csr.Subject.CommonName != agentID {
		return nil, "", errors.New("CSR identity mismatch")
	}
	serial := randomSerial()
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: agentID}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(validity), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		return nil, "", err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), serial.Text(16), nil
}
func randomSerial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	return n
}

type EnrollmentManager struct{ db *sql.DB }

func NewEnrollmentManager(db *sql.DB) *EnrollmentManager { return &EnrollmentManager{db: db} }
func (m *EnrollmentManager) CreateToken(ctx context.Context, creator string, now time.Time, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		return "", errors.New("TTL must be positive")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	hash := argon2.IDKey([]byte(token), raw[:16], 1, 32*1024, 1, 32)
	idRaw := sha256.Sum256(raw)
	var creatorValue any
	if creator != "" {
		creatorValue = creator
	}
	_, err := m.db.ExecContext(ctx, "INSERT INTO enrollment_tokens(id,token_hash,expires_unix_ms,created_by) VALUES(?,?,?,?)", hex.EncodeToString(idRaw[:16]), hash, now.Add(ttl).UnixMilli(), creatorValue)
	return token, err
}
func (m *EnrollmentManager) ConsumeToken(ctx context.Context, token string, now time.Time) error {
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return errors.New("invalid enrollment token")
	}
	hash := argon2.IDKey([]byte(token), raw[:16], 1, 32*1024, 1, 32)
	result, err := m.db.ExecContext(ctx, "UPDATE enrollment_tokens SET consumed_unix_ms=? WHERE token_hash=? AND consumed_unix_ms IS NULL AND expires_unix_ms>=?", now.UnixMilli(), hash, now.UnixMilli())
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("invalid or expired enrollment token")
	}
	return nil
}
