package registry

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/tunnel"
)

type AgentServer struct {
	devopsv1.UnimplementedAgentServiceServer
	Registry    *Registry
	Persistence *Persistence
	Relay       *tunnel.Relay
	Authorize   func(context.Context, string, string, time.Time) error
	Handle      func(context.Context, string, *devopsv1.AgentMessage) error
	Now         func() time.Time
}

// OpenTunnel is the agent end of an SSH byte relay. The agent dials its local
// sshd, sends a first frame carrying session id + binding token, and the
// validator splices this stream to the waiting client stream.
func (s *AgentServer) OpenTunnel(stream grpc.BidiStreamingServer[devopsv1.TunnelFrame, devopsv1.TunnelFrame]) error {
	if s.Relay == nil {
		return status.Error(codes.FailedPrecondition, "relay unavailable")
	}
	if _, _, err := peerCertificateIdentity(stream.Context()); err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if first.SessionId == "" || len(first.BindingToken) == 0 {
		return status.Error(codes.InvalidArgument, "first frame must carry session id and token")
	}
	if err := s.Relay.Attach(stream.Context(), first.SessionId, first.BindingToken, stream); err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	return nil
}

func (s *AgentServer) Connect(stream grpc.BidiStreamingServer[devopsv1.AgentMessage, devopsv1.ValidatorMessage]) error {
	if s.Registry == nil {
		return status.Error(codes.FailedPrecondition, "registry unavailable")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	certID, serial, err := peerCertificateIdentity(stream.Context())
	if err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	if s.Authorize != nil {
		if err := s.Authorize(stream.Context(), certID, serial, now()); err != nil {
			return status.Error(codes.PermissionDenied, "agent certificate rejected")
		}
	}
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	hello := first.GetHello()
	if hello == nil {
		return status.Error(codes.InvalidArgument, "first message must be hello")
	}
	session, state, err := s.Registry.Acquire(certID, hello.AgentId, hello, now())
	if err != nil {
		return registryStatus(err)
	}
	defer session.Release(now())
	streamID := fmt.Sprintf("%s-%d", certID, now().UnixNano())
	if s.Persistence != nil {
		if err := s.Persistence.Online(stream.Context(), certID, streamID, hello, now()); err != nil {
			return status.Error(codes.Unavailable, "persist connection")
		}
		defer s.Persistence.Offline(stream.Context(), certID, streamID, now())
	}
	if err := stream.Send(&devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Connection{Connection: state}}); err != nil {
		return err
	}
	errCh := make(chan error, 2)
	go func() {
		for m := range session.Messages() {
			if err := stream.Send(m); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()
	go func() {
		for {
			m, err := stream.Recv()
			if err == io.EOF {
				errCh <- nil
				return
			}
			if err != nil {
				errCh <- err
				return
			}
			if m.GetHeartbeat() != nil {
				session.Heartbeat(now())
			}
			if s.Handle != nil {
				if err := s.Handle(stream.Context(), certID, m); err != nil {
					errCh <- err
					return
				}
			}
		}
	}()
	return <-errCh
}
func peerCertificateIdentity(ctx context.Context) (string, string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", "", errors.New("missing peer")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
		return "", "", errors.New("mTLS certificate required")
	}
	cert := tlsInfo.State.PeerCertificates[0]
	id, err := certificateIdentity(cert)
	if err != nil {
		return "", "", err
	}
	return id, cert.SerialNumber.Text(16), nil
}
func certificateIdentity(cert *x509.Certificate) (string, error) {
	if cert.Subject.CommonName == "" {
		return "", errors.New("certificate identity missing")
	}
	return cert.Subject.CommonName, nil
}
func registryStatus(err error) error {
	switch err {
	case ErrDuplicateStream:
		return status.Error(codes.AlreadyExists, err.Error())
	case ErrIdentityMismatch:
		return status.Error(codes.Unauthenticated, err.Error())
	case ErrVersionMismatch:
		return status.Error(codes.FailedPrecondition, err.Error())
	case ErrLimitsExceeded:
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
