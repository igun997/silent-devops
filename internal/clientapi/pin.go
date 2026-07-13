package clientapi

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func FetchPinnedCertificate(ctx context.Context, address, pin, path string) error {
	if address == "" || !strings.HasPrefix(pin, "sha256/") {
		return errors.New("address and sha256 pin required")
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer raw.Close()
	conn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13})
	if err = conn.HandshakeContext(ctx); err != nil {
		return err
	}
	cert := conn.ConnectionState().PeerCertificates[0]
	sum := sha256.Sum256(cert.Raw)
	if "sha256/"+base64.StdEncoding.EncodeToString(sum[:]) != pin {
		return errors.New("validator certificate pin mismatch")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, certPEM(cert.Raw), 0600)
}
func certPEM(der []byte) []byte {
	return append(append([]byte("-----BEGIN CERTIFICATE-----\n"), []byte(base64.StdEncoding.EncodeToString(der))...), []byte("\n-----END CERTIFICATE-----\n")...)
}
