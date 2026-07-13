package server

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/pki"
)

type Enrollment struct {
	devopsv1.UnimplementedEnrollmentServiceServer
	CA         *pki.CA
	Tokens     *pki.EnrollmentManager
	Identities *pki.IdentityRegistry
	Now        func() time.Time
	Validity   time.Duration
}

func (s Enrollment) Enroll(ctx context.Context, request *devopsv1.EnrollRequest) (*devopsv1.EnrollResponse, error) {
	if s.CA == nil || s.Tokens == nil || s.Identities == nil {
		return nil, status.Error(codes.FailedPrecondition, "enrollment unavailable")
	}
	csr, err := x509.ParseCertificateRequest(request.GetCsrDer())
	if err != nil || csr.Subject.CommonName == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid CSR")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if err := s.Tokens.ConsumeToken(ctx, request.GetToken(), now()); err != nil {
		return nil, status.Error(codes.PermissionDenied, "invalid enrollment token")
	}
	validity := s.Validity
	if validity <= 0 {
		validity = 24 * time.Hour
	}
	certificate, serial, err := s.CA.SignAgentCSR(request.GetCsrDer(), csr.Subject.CommonName, now(), validity)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid CSR")
	}
	if err := s.Identities.Register(ctx, csr.Subject.CommonName, serial, now().Add(validity)); err != nil {
		return nil, status.Error(codes.AlreadyExists, "agent identity exists")
	}
	return &devopsv1.EnrollResponse{AgentId: csr.Subject.CommonName, CertificatePem: certificate, CaCertificatePem: s.CA.CertificatePEM(), ExpiresUnixMs: now().Add(validity).UnixMilli()}, nil
}
func (s Enrollment) Renew(ctx context.Context, request *devopsv1.RenewRequest) (*devopsv1.RenewResponse, error) {
	id, err := mtlsID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "mTLS identity required")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if err := s.Identities.AuthorizeRenewal(ctx, id, now()); err != nil {
		return nil, status.Error(codes.PermissionDenied, "renewal denied")
	}
	validity := s.Validity
	if validity <= 0 {
		validity = 24 * time.Hour
	}
	certificate, serial, err := s.CA.SignAgentCSR(request.GetCsrDer(), id, now(), validity)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid CSR")
	}
	if err := s.Identities.UpdateCertificate(ctx, id, serial, now().Add(validity)); err != nil {
		return nil, status.Error(codes.PermissionDenied, "renewal denied")
	}
	return &devopsv1.RenewResponse{CertificatePem: certificate, ExpiresUnixMs: now().Add(validity).UnixMilli()}, nil
}
func mtlsID(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", errors.New("peer missing")
	}
	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.PeerCertificates) == 0 {
		return "", errors.New("certificate missing")
	}
	id := info.State.PeerCertificates[0].Subject.CommonName
	if id == "" {
		return "", errors.New("identity missing")
	}
	return id, nil
}
func parseCertificate(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}
