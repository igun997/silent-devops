// Package tunnel relays an authenticated SSH byte stream between a client
// (FleetService.BridgeSsh) and an agent (AgentService.OpenTunnel) joined by
// session id. The validator moves only ciphertext; SSH runs end-to-end.
package tunnel

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"io"
	"sync"

	devopsv1 "silent-devops/api/devops/v1"
)

// FrameStream is the common surface of both gRPC bidi tunnel streams.
type FrameStream interface {
	Send(*devopsv1.TunnelFrame) error
	Recv() (*devopsv1.TunnelFrame, error)
}

type pending struct {
	token    []byte
	clientCh chan FrameStream
	done     chan struct{}
}

// Relay coordinates client/agent tunnel stream rendezvous by session id.
type Relay struct {
	mu      sync.Mutex
	pending map[string]*pending
}

func NewRelay() *Relay { return &Relay{pending: make(map[string]*pending)} }

// Register reserves a session and returns the binding token the validator
// must dispatch to the agent. The agent echoes it back on OpenTunnel.
func (r *Relay) Register(sessionID string) ([]byte, error) {
	if sessionID == "" {
		return nil, errors.New("session id required")
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		r.pending = make(map[string]*pending)
	}
	if _, ok := r.pending[sessionID]; ok {
		return nil, errors.New("tunnel already registered")
	}
	r.pending[sessionID] = &pending{token: token, clientCh: make(chan FrameStream, 1), done: make(chan struct{})}
	return token, nil
}

func (r *Relay) get(sessionID string) *pending {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending[sessionID]
}

func (r *Relay) remove(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pending, sessionID)
}

// Cancel drops a registration when dispatch to the agent fails.
func (r *Relay) Cancel(sessionID string) { r.remove(sessionID) }

// Serve runs the client end: hands the client stream to the pending rendezvous
// and blocks until the splice completes or the context is cancelled.
func (r *Relay) Serve(ctx context.Context, sessionID string, client FrameStream) error {
	p := r.get(sessionID)
	if p == nil {
		return errors.New("unknown tunnel session")
	}
	select {
	case p.clientCh <- client:
	default:
		return errors.New("tunnel client already attached")
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		r.remove(sessionID)
		return ctx.Err()
	}
}

// Attach runs the agent end: validates the binding token, waits for the client
// stream, and splices the two streams until either side closes.
func (r *Relay) Attach(ctx context.Context, sessionID string, token []byte, agent FrameStream) error {
	p := r.get(sessionID)
	if p == nil {
		return errors.New("unknown tunnel session")
	}
	if len(token) == 0 || subtle.ConstantTimeCompare(p.token, token) != 1 {
		return errors.New("tunnel binding mismatch")
	}
	var client FrameStream
	select {
	case client = <-p.clientCh:
	case <-ctx.Done():
		r.remove(sessionID)
		return ctx.Err()
	}
	defer func() { close(p.done); r.remove(sessionID) }()
	return splice(client, agent)
}

func splice(a, b FrameStream) error {
	errc := make(chan error, 2)
	go func() { errc <- pump(a, b) }()
	go func() { errc <- pump(b, a) }()
	return <-errc
}

func pump(src, dst FrameStream) error {
	for {
		frame, err := src.Recv()
		if err == io.EOF {
			_ = dst.Send(&devopsv1.TunnelFrame{Close: true})
			return nil
		}
		if err != nil {
			return err
		}
		if len(frame.Data) > 0 {
			if err := dst.Send(&devopsv1.TunnelFrame{Data: frame.Data}); err != nil {
				return err
			}
		}
		if frame.Close {
			_ = dst.Send(&devopsv1.TunnelFrame{Close: true})
			return nil
		}
	}
}
