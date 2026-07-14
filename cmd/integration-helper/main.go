//go:build integration

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/pki"
	"strings"
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
	case "ssh-e2e":
		err = sshE2E(os.Args[2], os.Args[3])
	case "ssh-exec":
		err = sshExec(os.Args[2], os.Args[3])
	case "ssh-pipe":
		err = sshPipe(os.Args[2], os.Args[3])
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
	for _, agent := range agents.Agents {
		metrics, err := fleet.GetMetrics(ctx, &devopsv1.GetMetricsRequest{AgentId: agent.Id})
		if err != nil || len(metrics.Snapshots) == 0 {
			return fmt.Errorf("metrics %s: %v", agent.Id, err)
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
	jobCtx := &devopsv1.JobRequestContext{AgentId: agents.Agents[0].Id, Reason: "e2e process list", TimeoutSeconds: 30, IdempotencyKey: "e2e-process"}
	job, err := fleet.ListProcesses(opCtx, &devopsv1.ProcessListJobRequest{Context: jobCtx, Request: &devopsv1.ProcessListRequest{Limit: 10}})
	if err != nil {
		return err
	}
	_ = job
	if _, err := fleet.Exec(opCtx, &devopsv1.ExecJobRequest{Context: &devopsv1.JobRequestContext{AgentId: agents.Agents[0].Id, Reason: "denied", TimeoutSeconds: 30, IdempotencyKey: "operator-exec", Confirmed: true}, Request: &devopsv1.ArbitraryCommand{Command: "true"}}); err == nil {
		return fmt.Errorf("operator exec accepted")
	}
	adminJob, err := fleet.Exec(ctx, &devopsv1.ExecJobRequest{Context: &devopsv1.JobRequestContext{AgentId: agents.Agents[0].Id, Reason: "e2e admin", TimeoutSeconds: 30, IdempotencyKey: "admin-exec", Confirmed: true}, Request: &devopsv1.ArbitraryCommand{Command: "printf e2e", CaptureOutput: true}})
	if err != nil {
		return err
	}
	_ = adminJob
	if _, err := fleet.ListAgents(context.Background(), &devopsv1.ListAgentsRequest{}); err == nil {
		return fmt.Errorf("missing bearer accepted")
	}
	return nil
}
func sshE2E(address, target string) error {
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
	agents, err := fleet.ListAgents(ctx, &devopsv1.ListAgentsRequest{})
	if err != nil {
		return err
	}
	if target == "" {
		return errors.New("SSH target missing")
	}
	found := false
	for _, a := range agents.Agents {
		if a.Id == target {
			found = true
		}
	}
	if !found {
		return errors.New("SSH target missing")
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	public, err := ssh.NewPublicKey(private.Public())
	if err != nil {
		return err
	}
	session, err := fleet.PrepareSsh(ctx, &devopsv1.PrepareSshRequest{AgentId: target, PublicKey: ssh.MarshalAuthorizedKey(public), Reason: "e2e SSH", TtlSeconds: 60})
	if err != nil {
		return err
	}
	defer fleet.CloseSsh(ctx, &devopsv1.CloseSshRequest{SessionId: session.Id, Reason: "e2e done"})
	for i := 0; i < 50 && session.State == devopsv1.SshSessionState_SSH_SESSION_STATE_PREPARING; i++ {
		time.Sleep(100 * time.Millisecond)
		session, err = fleet.GetSshSession(ctx, &devopsv1.GetSshSessionRequest{SessionId: session.Id})
		if err != nil {
			return err
		}
	}
	if session.State != devopsv1.SshSessionState_SSH_SESSION_STATE_READY {
		return errors.New("SSH not ready")
	}
	fmt.Println("ssh-ready", session.Id)
	return nil
}

// sshExec exercises real interactive-style SSH traffic end to end: it prepares
// a session, waits for READY, then launches native OpenSSH through the
// validator ProxyJump to run `id; hostname` on the agent, asserting output and
// pinning both the jump and target host keys (no TOFU).
func sshExec(address, target string) error {
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
	if target == "" {
		return errors.New("SSH target missing")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	signerPub, err := ssh.NewPublicKey(public)
	if err != nil {
		return err
	}
	session, err := fleet.PrepareSsh(ctx, &devopsv1.PrepareSshRequest{AgentId: target, PublicKey: ssh.MarshalAuthorizedKey(signerPub), Reason: "e2e SSH exec", TtlSeconds: 60})
	if err != nil {
		return err
	}
	defer fleet.CloseSsh(ctx, &devopsv1.CloseSshRequest{SessionId: session.Id, Reason: "e2e exec done"})
	for i := 0; i < 50 && session.State == devopsv1.SshSessionState_SSH_SESSION_STATE_PREPARING; i++ {
		time.Sleep(100 * time.Millisecond)
		session, err = fleet.GetSshSession(ctx, &devopsv1.GetSshSessionRequest{SessionId: session.Id})
		if err != nil {
			return err
		}
	}
	if session.State != devopsv1.SshSessionState_SSH_SESSION_STATE_READY {
		return errors.New("SSH not ready")
	}
	if len(session.HostKey) == 0 {
		return fmt.Errorf("incomplete session: hostKey=%d", len(session.HostKey))
	}
	dir, err := os.MkdirTemp("", "ssh-exec")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	keyPath := filepath.Join(dir, "id")
	signer, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(signer), 0600); err != nil {
		return err
	}
	known := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(known, []byte(fmt.Sprintf("%s %s\n", session.Id, session.HostKey)), 0600); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	proxy := fmt.Sprintf("ProxyCommand=%s ssh-pipe %s %s", exe, address, session.Id)
	cmd := exec.Command("ssh", "-i", keyPath, "-o", "IdentitiesOnly=yes", "-o", "UserKnownHostsFile="+known, "-o", "StrictHostKeyChecking=yes", "-o", "PasswordAuthentication=no", "-o", "BatchMode=yes", "-o", proxy, "silent-devops@"+session.Id, "id; hostname")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh exec failed: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "silent-devops") {
		return fmt.Errorf("unexpected ssh output: %s", out)
	}
	fmt.Printf("ssh-exec-ok %s %s", session.Id, out)
	return nil
}

// sshPipe is invoked by OpenSSH as a ProxyCommand: it logs in, opens a
// BridgeSsh stream for the session, and relays stdin/stdout.
func sshPipe(address, sessionID string) error {
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
	stream, err := devopsv1.NewFleetServiceClient(conn).BridgeSsh(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&devopsv1.TunnelFrame{SessionId: sessionID}); err != nil {
		return err
	}
	errc := make(chan error, 2)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := os.Stdin.Read(buf)
			if n > 0 {
				if serr := stream.Send(&devopsv1.TunnelFrame{Data: buf[:n]}); serr != nil {
					errc <- serr
					return
				}
			}
			if rerr != nil {
				_ = stream.Send(&devopsv1.TunnelFrame{Close: true})
				_ = stream.CloseSend()
				errc <- nil
				return
			}
		}
	}()
	go func() {
		for {
			frame, rerr := stream.Recv()
			if rerr == io.EOF {
				errc <- nil
				return
			}
			if rerr != nil {
				errc <- rerr
				return
			}
			if len(frame.Data) > 0 {
				if _, werr := os.Stdout.Write(frame.Data); werr != nil {
					errc <- werr
					return
				}
			}
			if frame.Close {
				errc <- nil
				return
			}
		}
	}()
	return <-errc
}
func enroll(address, token, agentID, dir string) error {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
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
	return pki.SaveAgentCredentials(dir, pki.AgentCredentials{AgentID: response.AgentId, PrivateKey: key, CertificatePEM: response.CertificatePem, CAPEM: ca})
}
