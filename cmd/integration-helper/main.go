//go:build integration

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"math/big"
	"os"
	"path/filepath"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/pki"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		panic("init or enroll")
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = initPKI(os.Args[2])
	case "enroll":
		err = enroll(os.Args[2], os.Args[3], os.Args[4], os.Args[5])
	case "tokens":
		err = tokens(os.Args[2], os.Args[3])
	case "verify":
		err = verify(os.Args[2])
	case "login":
		err = login(os.Args[2])
	case "reuse":
		err = enroll(os.Args[2], string(mustRead(os.Args[3])), "reused", os.Args[4])
	case "expired-token":
		err = expiredToken(os.Args[2])
	default:
		err = fmt.Errorf("unknown action")
	}
	if err != nil {
		panic(err)
	}
}
func initPKI(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if _, err := pki.CreateCA(filepath.Join(dir, "agent-ca.key"), []byte("integration-agent-ca-passphrase"), time.Now()); err != nil {
		return err
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "validator"}, DNSNames: []string{"validator"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		return err
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	raw, _ := x509.MarshalPKCS8PrivateKey(key)
	for name, data := range map[string][]byte{"server.crt": cert, "client-ca.crt": cert, "server.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
	}
	return nil
}
func expiredToken(address string) error {
	conn, err := dial(address)
	if err != nil {
		return err
	}
	defer conn.Close()
	login, err := devopsv1.NewAuthServiceClient(conn).Login(context.Background(), &devopsv1.LoginRequest{Username: "admin", Password: "integration-admin-password"})
	if err != nil {
		return err
	}
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+login.AccessToken))
	_, err = devopsv1.NewFleetServiceClient(conn).CreateEnrollmentToken(ctx, &devopsv1.CreateEnrollmentTokenRequest{TtlSeconds: 0})
	if err == nil {
		return fmt.Errorf("zero TTL token accepted")
	}
	return nil
}
func mustRead(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return b
}
func login(address string) error {
	conn, err := dial(address)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = devopsv1.NewAuthServiceClient(conn).Login(context.Background(), &devopsv1.LoginRequest{Username: "admin", Password: "integration-admin-password"})
	return err
}
func dial(address string) (*grpc.ClientConn, error) {
	pool := x509.NewCertPool()
	ca, err := os.ReadFile("/shared/server.crt")
	if err != nil {
		return nil, err
	}
	pool.AppendCertsFromPEM(ca)
	return grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(pool, "validator")))
}
func tokens(address, dir string) error {
	conn, err := dial(address)
	if err != nil {
		return err
	}
	defer conn.Close()
	auth, err := devopsv1.NewAuthServiceClient(conn).Login(context.Background(), &devopsv1.LoginRequest{Username: "admin", Password: "integration-admin-password"})
	if err != nil {
		return err
	}
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+auth.AccessToken))
	fleet := devopsv1.NewFleetServiceClient(conn)
	for i := 1; i <= 3; i++ {
		token, err := fleet.CreateEnrollmentToken(ctx, &devopsv1.CreateEnrollmentTokenRequest{TtlSeconds: 600})
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("token-%d", i)), []byte(token.Token), 0600); err != nil {
			return err
		}
	}
	return nil
}
func verify(address string) error {
	conn, err := dial(address)
	if err != nil {
		return err
	}
	defer conn.Close()
	login, err := devopsv1.NewAuthServiceClient(conn).Login(context.Background(), &devopsv1.LoginRequest{Username: "admin", Password: "integration-admin-password"})
	if err != nil {
		return err
	}
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+login.AccessToken))
	fleet := devopsv1.NewFleetServiceClient(conn)
	agents, err := fleet.ListAgents(ctx, &devopsv1.ListAgentsRequest{PageSize: 10})
	if err != nil || len(agents.Agents) != 3 {
		return fmt.Errorf("agents=%d err=%v", len(agents.GetAgents()), err)
	}
	for _, id := range []string{"ubuntu-2204", "ubuntu-2404", "debian-12"} {
		metrics, err := fleet.GetMetrics(ctx, &devopsv1.GetMetricsRequest{AgentId: id})
		if err != nil || len(metrics.Snapshots) == 0 {
			return fmt.Errorf("metrics %s: %v", id, err)
		}
	}
	viewer, err := fleet.CreateUser(ctx, &devopsv1.CreateUserRequest{Username: "viewer", Password: "viewer-password-strong", Role: devopsv1.Role_ROLE_VIEWER})
	if err != nil {
		return err
	}
	_ = viewer
	operator, err := fleet.CreateUser(ctx, &devopsv1.CreateUserRequest{Username: "operator", Password: "operator-password-strong", Role: devopsv1.Role_ROLE_OPERATOR})
	if err != nil {
		return err
	}
	_ = operator
	viewerLogin, err := devopsv1.NewAuthServiceClient(conn).Login(context.Background(), &devopsv1.LoginRequest{Username: "viewer", Password: "viewer-password-strong"})
	if err != nil {
		return err
	}
	viewerCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+viewerLogin.AccessToken))
	if _, err := fleet.ListAgents(viewerCtx, &devopsv1.ListAgentsRequest{}); err != nil {
		return fmt.Errorf("viewer read: %w", err)
	}
	if _, err := fleet.Exec(viewerCtx, &devopsv1.ExecJobRequest{}); err == nil {
		return fmt.Errorf("viewer exec accepted")
	}
	opLogin, err := devopsv1.NewAuthServiceClient(conn).Login(context.Background(), &devopsv1.LoginRequest{Username: "operator", Password: "operator-password-strong"})
	if err != nil {
		return err
	}
	opCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+opLogin.AccessToken))
	jobCtx := &devopsv1.JobRequestContext{AgentId: "debian-12", Reason: "e2e process list", TimeoutSeconds: 30, IdempotencyKey: "e2e-process"}
	job, err := fleet.ListProcesses(opCtx, &devopsv1.ProcessListJobRequest{Context: jobCtx, Request: &devopsv1.ProcessListRequest{Limit: 10}})
	if err != nil {
		return err
	}
	_ = job
	if _, err := fleet.Exec(opCtx, &devopsv1.ExecJobRequest{Context: &devopsv1.JobRequestContext{AgentId: "debian-12", Reason: "denied", TimeoutSeconds: 30, IdempotencyKey: "operator-exec", Confirmed: true}, Request: &devopsv1.ArbitraryCommand{Command: "true"}}); err == nil {
		return fmt.Errorf("operator exec accepted")
	}
	adminJob, err := fleet.Exec(ctx, &devopsv1.ExecJobRequest{Context: &devopsv1.JobRequestContext{AgentId: "debian-12", Reason: "e2e admin", TimeoutSeconds: 30, IdempotencyKey: "admin-exec", Confirmed: true}, Request: &devopsv1.ArbitraryCommand{Command: "printf e2e", CaptureOutput: true}})
	if err != nil {
		return err
	}
	_ = adminJob
	if _, err := fleet.ListAgents(context.Background(), &devopsv1.ListAgentsRequest{}); err == nil {
		return fmt.Errorf("missing bearer accepted")
	}
	return nil
}
func enroll(address, token, agentID, dir string) error {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: agentID}}, key)
	if err != nil {
		return err
	}
	ca, err := os.ReadFile("/shared/server.crt")
	if err != nil {
		return err
	}
	conn, err := dial(address)
	if err != nil {
		return err
	}
	defer conn.Close()
	response, err := devopsv1.NewEnrollmentServiceClient(conn).Enroll(context.Background(), &devopsv1.EnrollRequest{Token: token, CsrDer: csr, Hostname: agentID})
	if err != nil {
		return err
	}
	return pki.SaveAgentCredentials(dir, pki.AgentCredentials{AgentID: agentID, PrivateKey: key, CertificatePEM: response.CertificatePem, CAPEM: ca})
}
