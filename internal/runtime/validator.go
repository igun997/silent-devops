package runtime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"silent-devops/internal/localcontrol"
	"silent-devops/internal/metrics"
	"silent-devops/internal/pki"
	"silent-devops/internal/registry"
	serverapi "silent-devops/internal/server"
	sshmanager "silent-devops/internal/ssh"
	"silent-devops/internal/store"
	"silent-devops/internal/tunnel"
)

func RunValidator(ctx context.Context, cfg ValidatorConfig) error {
	s, err := store.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer s.Close()
	if cfg.BootstrapUser != "" || cfg.BootstrapPassword != "" {
		if err := pki.BootstrapAdmin(ctx, s.DB(), cfg.BootstrapUser, cfg.BootstrapPassword, time.Now()); err != nil && err.Error() != "bootstrap already completed" {
			return err
		}
	}
	persistence := registry.NewPersistence(s.DB())
	if err := persistence.MarkAllOffline(ctx, time.Now()); err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
	if err != nil {
		return fmt.Errorf("load validator TLS identity: %w", err)
	}
	caPEM, err := os.ReadFile(cfg.ClientCA)
	if err != nil {
		return fmt.Errorf("read client CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return errors.New("invalid client CA")
	}
	agentCA, err := pki.LoadCA(cfg.AgentCA, []byte(cfg.AgentCAPassphrase))
	if err != nil {
		return fmt.Errorf("load agent CA: %w", err)
	}
	if !pool.AppendCertsFromPEM(agentCA.CertificatePEM()) {
		return errors.New("invalid agent CA")
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}, ClientCAs: pool, ClientAuth: tls.VerifyClientCertIfGiven, MinVersion: tls.VersionTLS13}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	identities := pki.NewIdentityRegistry(s.DB())
	issuer, err := auth.NewIssuer(cfg.TokenKey, 15*time.Minute)
	if err != nil {
		return err
	}
	authService := auth.NewService(s.DB(), issuer, auth.NewRateLimiter(5, time.Minute), time.Now)
	serverOptions := []grpc.ServerOption{grpc.Creds(credentials.NewTLS(tlsConfig)), grpc.UnaryInterceptor(auth.EndpointUnaryInterceptor(issuer, cfg.Policies, time.Now)), grpc.StreamInterceptor(auth.StreamInterceptor(issuer, cfg.Policies, time.Now))}
	server := grpc.NewServer(serverOptions...)
	devopsv1.RegisterAuthServiceServer(server, serverapi.Auth{Service: authService, DB: s.DB()})
	tokens := pki.NewEnrollmentManager(s.DB())
	devopsv1.RegisterEnrollmentServiceServer(server, serverapi.Enrollment{CA: agentCA, Tokens: tokens, Identities: identities, Validity: 24 * time.Hour})
	agentRegistry := registry.New(1, devopsv1.DefaultLimits(), 45*time.Second)
	sshSessions := sshmanager.NewManager(s.DB(), 22000, 22999, time.Now)
	if err := sshSessions.Reconcile(ctx, time.Now()); err != nil {
		return err
	}
	sshRelay := tunnel.NewRelay()
	fleet := serverapi.Fleet{DB: s.DB(), Tokens: tokens, Registry: agentRegistry, SSH: sshSessions, Relay: sshRelay}
	devopsv1.RegisterFleetServiceServer(server, fleet)
	if err := os.MkdirAll(filepath.Dir(cfg.LocalSocket), 0750); err != nil {
		return err
	}
	_ = os.Remove(cfg.LocalSocket)
	localListener, err := net.Listen("unix", cfg.LocalSocket)
	if err != nil {
		return err
	}
	defer localListener.Close()
	group, err := user.LookupGroup("silent-devops-admin")
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return err
	}
	if err := os.Chown(cfg.LocalSocket, -1, gid); err != nil {
		return err
	}
	if err := os.Chmod(cfg.LocalSocket, 0660); err != nil {
		return err
	}
	localServer := grpc.NewServer(grpc.Creds(localcontrol.Credentials{}), grpc.UnaryInterceptor(localcontrol.UnaryInterceptor()))
	devopsv1.RegisterFleetServiceServer(localServer, fleet)
	go func() { <-ctx.Done(); localServer.GracefulStop() }()
	go func() {
		if err := localServer.Serve(localListener); err != nil && err != grpc.ErrServerStopped {
			log.Printf("local control server stopped: %v", err)
		}
	}()
	messageHandler := registry.MessageHandler{DB: s.DB(), Metrics: metrics.NewRepository(s.DB()), SSH: sshSessions}
	devopsv1.RegisterAgentServiceServer(server, &registry.AgentServer{Registry: agentRegistry, Persistence: persistence, Relay: sshRelay, Authorize: identities.AuthorizeConnection, Handle: messageHandler.Handle})
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			server.GracefulStop()
		case <-stopped:
		}
	}()
	err = server.Serve(listener)
	close(stopped)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
