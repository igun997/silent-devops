package agent

import (
	"context"
	"errors"
	"io"
	"net"
	devopsv1 "silent-devops/api/devops/v1"
	sshmanager "silent-devops/internal/ssh"
	"time"
)

// TunnelOpener opens an OpenTunnel bidi stream to the validator.
type TunnelOpener func(context.Context) (devopsv1.AgentService_OpenTunnelClient, error)

// SSHHandler authorizes ephemeral client keys on the agent's login user and
// bridges SSH byte streams from the validator to the local sshd. No inbound
// port and no reverse ssh -R: the agent dials 127.0.0.1:22 on demand.
type SSHHandler struct {
	KeyStore  sshmanager.KeyStore
	HostKey   []byte
	LocalAddr string
	Open      TunnelOpener
	Send      func(*devopsv1.AgentMessage) error
}

func (h *SSHHandler) Prepare(ctx context.Context, p *devopsv1.PrepareSsh) error {
	if p == nil || p.SessionId == "" || time.Now().UnixMilli() >= p.ExpiresUnixMs {
		return errors.New("invalid SSH preparation")
	}
	if err := h.KeyStore.Install(p.SessionId, p.PublicKey, time.UnixMilli(p.ExpiresUnixMs)); err != nil {
		return err
	}
	if h.Send == nil {
		_ = h.KeyStore.Remove(p.SessionId)
		return errors.New("sender unavailable")
	}
	return h.Send(&devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_SshReady{SshReady: &devopsv1.SshReady{SessionId: p.SessionId, HostKey: h.HostKey, BindingToken: p.BindingToken}}})
}

func (h *SSHHandler) Close(id string) error {
	return h.KeyStore.Remove(id)
}

// tunnel opens the validator stream, dials the local sshd, and splices bytes.
func (h *SSHHandler) tunnel(ctx context.Context, o *devopsv1.OpenSshTunnel) error {
	if o == nil || o.SessionId == "" || len(o.BindingToken) == 0 || h.Open == nil {
		return errors.New("invalid tunnel open")
	}
	addr := h.LocalAddr
	if addr == "" {
		addr = "127.0.0.1:22"
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	stream, err := h.Open(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&devopsv1.TunnelFrame{SessionId: o.SessionId, BindingToken: o.BindingToken}); err != nil {
		return err
	}
	return splice(conn, stream)
}

// splice copies bytes between the local sshd connection and the gRPC stream.
func splice(conn net.Conn, stream devopsv1.AgentService_OpenTunnelClient) error {
	errc := make(chan error, 2)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				if sendErr := stream.Send(&devopsv1.TunnelFrame{Data: buf[:n]}); sendErr != nil {
					errc <- sendErr
					return
				}
			}
			if err != nil {
				_ = stream.Send(&devopsv1.TunnelFrame{Close: true})
				errc <- nil
				return
			}
		}
	}()
	go func() {
		for {
			frame, err := stream.Recv()
			if err == io.EOF {
				errc <- nil
				return
			}
			if err != nil {
				errc <- err
				return
			}
			if len(frame.Data) > 0 {
				if _, werr := conn.Write(frame.Data); werr != nil {
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

func (h *SSHHandler) Handle(ctx context.Context, message *devopsv1.ValidatorMessage) error {
	if p := message.GetPrepareSsh(); p != nil {
		return h.Prepare(ctx, p)
	}
	if o := message.GetOpenTunnel(); o != nil {
		go func() { _ = h.tunnel(ctx, o) }()
		return nil
	}
	if c := message.GetCloseSsh(); c != nil {
		return h.Close(c.SessionId)
	}
	return nil
}
