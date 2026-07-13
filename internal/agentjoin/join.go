package agentjoin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/pki"
)

type Options struct {
	Address, Token, Pin, CredentialDir, Hostname string
	NoStart                                      bool
	Probe                                        func(context.Context, string) ([]byte, error)
	Enroll                                       func(context.Context, string, *devopsv1.EnrollRequest) (*devopsv1.EnrollResponse, error)
}

func DefaultTransport(pin string) (func(context.Context, string) ([]byte, error), func(context.Context, string, *devopsv1.EnrollRequest) (*devopsv1.EnrollResponse, error)) {
	var conn *grpc.ClientConn
	probe := func(ctx context.Context, address string) ([]byte, error) {
		var presented []byte
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true, VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("validator certificate missing")
			}
			presented = append([]byte(nil), state.PeerCertificates[0].Raw...)
			return pki.VerifyCertificatePin(presented, pin)
		}}
		var err error
		conn, err = grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		if err != nil {
			return nil, err
		}
		if err := conn.Invoke(ctx, "/devops.v1.AuthService/Login", &devopsv1.LoginRequest{}, &devopsv1.LoginResponse{}); err != nil && len(presented) == 0 {
			conn.Close()
			return nil, err
		}
		return presented, nil
	}
	enroll := func(ctx context.Context, _ string, r *devopsv1.EnrollRequest) (*devopsv1.EnrollResponse, error) {
		if conn == nil {
			return nil, errors.New("validator not probed")
		}
		defer conn.Close()
		return devopsv1.NewEnrollmentServiceClient(conn).Enroll(ctx, r)
	}
	return probe, enroll
}

func Join(ctx context.Context, o Options) error {
	if o.Address == "" || o.Token == "" || o.Pin == "" || o.CredentialDir == "" {
		return errors.New("validator, token, pin, and credential directory required")
	}
	if _, err := os.Stat(filepath.Join(o.CredentialDir, "agent.key")); err == nil {
		return errors.New("agent credentials already exist")
	} else if !os.IsNotExist(err) {
		return err
	}
	if o.Probe == nil || o.Enroll == nil {
		return errors.New("join transport unavailable")
	}
	certificate, err := o.Probe(ctx, o.Address)
	if err != nil {
		return err
	}
	if err := pki.VerifyCertificatePin(certificate, o.Pin); err != nil {
		return err
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{}}, key)
	if err != nil {
		return err
	}
	response, err := o.Enroll(ctx, o.Address, &devopsv1.EnrollRequest{Token: o.Token, CsrDer: csr, ValidatorPin: o.Pin, Hostname: o.Hostname})
	if err != nil {
		return err
	}
	if err := validateResponse(response); err != nil {
		return err
	}
	return pki.SaveAgentCredentials(o.CredentialDir, pki.AgentCredentials{AgentID: response.AgentId, PrivateKey: key, CertificatePEM: response.CertificatePem, CAPEM: response.CaCertificatePem})
}
func validateResponse(r *devopsv1.EnrollResponse) error {
	if r == nil || r.AgentId == "" {
		return errors.New("invalid enrollment response")
	}
	block, _ := pem.Decode(r.CertificatePem)
	if block == nil {
		return errors.New("invalid agent certificate")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	if certificate.Subject.CommonName != r.AgentId {
		return errors.New("agent certificate identity mismatch")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(r.CaCertificatePem) {
		return errors.New("invalid agent CA")
	}
	_, err = certificate.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	return err
}
