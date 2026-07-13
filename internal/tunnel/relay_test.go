package tunnel_test

import (
	"context"
	"io"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/tunnel"
)

type fakeStream struct {
	in  chan *devopsv1.TunnelFrame
	out chan *devopsv1.TunnelFrame
}

func newFake() *fakeStream {
	return &fakeStream{in: make(chan *devopsv1.TunnelFrame, 16), out: make(chan *devopsv1.TunnelFrame, 16)}
}
func (f *fakeStream) Send(frame *devopsv1.TunnelFrame) error { f.out <- frame; return nil }
func (f *fakeStream) Recv() (*devopsv1.TunnelFrame, error) {
	frame, ok := <-f.in
	if !ok {
		return nil, io.EOF
	}
	return frame, nil
}

func TestRelayJoinsAndSplices(t *testing.T) {
	r := tunnel.NewRelay()
	token, err := r.Register("s1")
	if err != nil {
		t.Fatal(err)
	}
	client, agent := newFake(), newFake()
	go func() { _ = r.Serve(context.Background(), "s1", client) }()
	go func() { _ = r.Attach(context.Background(), "s1", token, agent) }()

	client.in <- &devopsv1.TunnelFrame{Data: []byte("hello-agent")}
	select {
	case got := <-agent.out:
		if string(got.Data) != "hello-agent" {
			t.Fatalf("agent got %q", got.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("agent never received client data")
	}

	agent.in <- &devopsv1.TunnelFrame{Data: []byte("hello-client")}
	select {
	case got := <-client.out:
		if string(got.Data) != "hello-client" {
			t.Fatalf("client got %q", got.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("client never received agent data")
	}
}

func TestRelayRejectsBadBindingToken(t *testing.T) {
	r := tunnel.NewRelay()
	if _, err := r.Register("s2"); err != nil {
		t.Fatal(err)
	}
	agent := newFake()
	err := r.Attach(context.Background(), "s2", []byte("wrong-token-000000000000000000000"), agent)
	if err == nil {
		t.Fatal("expected binding mismatch")
	}
}

func TestRelayUnknownSession(t *testing.T) {
	r := tunnel.NewRelay()
	if err := r.Serve(context.Background(), "nope", newFake()); err == nil {
		t.Fatal("expected unknown session error")
	}
	if err := r.Attach(context.Background(), "nope", []byte("t"), newFake()); err == nil {
		t.Fatal("expected unknown session error")
	}
}

func TestRelayDuplicateRegister(t *testing.T) {
	r := tunnel.NewRelay()
	if _, err := r.Register("s3"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register("s3"); err == nil {
		t.Fatal("expected duplicate register error")
	}
}
