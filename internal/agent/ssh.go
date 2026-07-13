package agent

import (
	"context"
	"errors"
	devopsv1 "silent-devops/api/devops/v1"
	sshmanager "silent-devops/internal/ssh"
	"sync"
	"time"
)

type SSHHandler struct {
	KeyStore                           sshmanager.KeyStore
	Host, User, PrivateKey, KnownHosts string
	Send                               func(*devopsv1.AgentMessage) error
	mu                                 sync.Mutex
	sessions                           map[string]*sshmanager.TunnelProcess
}

func (h *SSHHandler) Prepare(ctx context.Context, p *devopsv1.PrepareSsh) error {
	if p == nil || p.SessionId == "" || time.Now().UnixMilli() >= p.ExpiresUnixMs {
		return errors.New("invalid SSH preparation")
	}
	if err := h.KeyStore.Install(p.SessionId, p.PublicKey, time.UnixMilli(p.ExpiresUnixMs)); err != nil {
		return err
	}
	args, err := sshmanager.ReverseTunnelArgs(h.Host, p.LoopbackPort, h.User, h.PrivateKey, h.KnownHosts)
	if err != nil {
		_ = h.KeyStore.Remove(p.SessionId)
		return err
	}
	sessionCtx, cancel := context.WithDeadline(ctx, time.UnixMilli(p.ExpiresUnixMs))
	process, err := sshmanager.StartTunnel(sessionCtx, "ssh", args)
	if err != nil {
		cancel()
		_ = h.KeyStore.Remove(p.SessionId)
		return err
	}
	h.mu.Lock()
	if h.sessions == nil {
		h.sessions = make(map[string]*sshmanager.TunnelProcess)
	}
	h.sessions[p.SessionId] = process
	h.mu.Unlock()
	go func() { <-process.Done(); cancel(); _ = h.Close(p.SessionId) }()
	if h.Send == nil {
		return errors.New("sender unavailable")
	}
	return h.Send(&devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_SshReady{SshReady: &devopsv1.SshReady{SessionId: p.SessionId, ValidatorLoopbackPort: p.LoopbackPort, BindingToken: p.BindingToken}}})
}
func (h *SSHHandler) Close(id string) error {
	h.mu.Lock()
	process := h.sessions[id]
	delete(h.sessions, id)
	h.mu.Unlock()
	if process != nil {
		_ = process.Stop()
	}
	return h.KeyStore.Remove(id)
}
func (h *SSHHandler) Handle(ctx context.Context, message *devopsv1.ValidatorMessage) error {
	if p := message.GetPrepareSsh(); p != nil {
		return h.Prepare(ctx, p)
	}
	if c := message.GetCloseSsh(); c != nil {
		return h.Close(c.SessionId)
	}
	return nil
}
