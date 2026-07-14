package runtime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	devopsv1 "silent-devops/api/devops/v1"
	agentstream "silent-devops/internal/agent"
	"silent-devops/internal/maintenance"
	"silent-devops/internal/metrics"
	"silent-devops/internal/pki"
	sshmanager "silent-devops/internal/ssh"
)

type lockedStream struct {
	devopsv1.AgentService_ConnectClient
	mu sync.Mutex
}

func (s *lockedStream) Send(message *devopsv1.AgentMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.AgentService_ConnectClient.Send(message)
}

func pemPrivateKey(key any) []byte {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func RunAgent(ctx context.Context, cfg AgentConfig) error {
	credentialsData, err := pki.LoadAgentCredentials(cfg.CredentialDir)
	if err != nil {
		return err
	}
	cert, err := tls.X509KeyPair(credentialsData.CertificatePEM, pemPrivateKey(credentialsData.PrivateKey))
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(credentialsData.CAPEM) {
		return errors.New("invalid validator CA")
	}
	hostname, _ := os.Hostname()
	hostKey, _ := os.ReadFile(cfg.HostKeyPath)
	hello := &devopsv1.AgentHello{AgentId: credentialsData.AgentID, Hostname: hostname, Architecture: runtime.GOARCH, Protocol: &devopsv1.VersionRange{Minimum: 1, Maximum: 1}, Limits: devopsv1.DefaultLimits()}
	return agentstream.Reconnect(ctx, time.Second, time.Minute, func(connectCtx context.Context) error {
		conn, err := grpc.NewClient(cfg.Validator, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}, RootCAs: pool, MinVersion: tls.VersionTLS13})))
		if err != nil {
			return err
		}
		defer conn.Close()
		client := devopsv1.NewAgentServiceClient(conn)
		stream, err := client.Connect(connectCtx)
		if err != nil {
			return err
		}
		locked := &lockedStream{AgentService_ConnectClient: stream}
		handler := &agentstream.Handler{AgentID: credentialsData.AgentID, Dispatcher: maintenance.Dispatcher{Runner: maintenance.Runner{MaxOutputBytes: int(devopsv1.DefaultLimits().MaxOutputBytes)}, Timeout: time.Hour}, Send: locked.Send}
		sshHandler := &agentstream.SSHHandler{KeyStore: sshmanager.KeyStore{Path: cfg.AuthorizedKeys}, HostKey: hostKey, LocalAddr: cfg.SSHLocalAddr, Open: func(c context.Context) (devopsv1.AgentService_OpenTunnelClient, error) {
			return client.OpenTunnel(c)
		}, Send: locked.Send}
		if cfg.AuthorizedKeys != "" {
			_ = sshHandler.KeyStore.Reconcile(time.Now())
		}
		publisherCtx, cancelPublisher := context.WithCancel(connectCtx)
		defer cancelPublisher()
		publisher := agentstream.MetricsPublisher{Interval: time.Minute, Collect: func(ctx context.Context) (*devopsv1.MetricsSnapshot, error) {
			snapshot, _, err := (metrics.Collector{MaxInterfaces: 64}).Collect(ctx)
			return snapshot, err
		}, Send: locked.Send}
		go func() { _ = publisher.Run(publisherCtx) }()
		return agentstream.RunConnection(connectCtx, locked, hello, cfg.Heartbeat, func(message *devopsv1.ValidatorMessage) {
			if message.GetJob() != nil {
				_ = handler.Handle(connectCtx, message)
			} else {
				_ = sshHandler.Handle(connectCtx, message)
			}
		})
	})
}
