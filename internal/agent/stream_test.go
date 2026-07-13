package agent_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/agent"
)

type fakeConn struct {
	sends  atomic.Int32
	recv   chan *devopsv1.ValidatorMessage
	closed chan struct{}
}

func (f *fakeConn) Send(*devopsv1.AgentMessage) error { f.sends.Add(1); return nil }
func (f *fakeConn) Recv() (*devopsv1.ValidatorMessage, error) {
	select {
	case m := <-f.recv:
		return m, nil
	case <-f.closed:
		return nil, errors.New("closed")
	}
}
func (f *fakeConn) CloseSend() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	return nil
}

func TestRunSendsHelloHeartbeatAndReceives(t *testing.T) {
	conn := &fakeConn{recv: make(chan *devopsv1.ValidatorMessage, 1), closed: make(chan struct{})}
	conn.recv <- &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Connection{Connection: &devopsv1.ConnectionState{Accepted: true}}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	received := make(chan struct{}, 1)
	err := agent.RunConnection(ctx, conn, &devopsv1.AgentHello{AgentId: "a"}, 5*time.Millisecond, func(*devopsv1.ValidatorMessage) { received <- struct{}{} })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if conn.sends.Load() < 2 {
		t.Fatalf("sends=%d", conn.sends.Load())
	}
	select {
	case <-received:
	default:
		t.Fatal("message not handled")
	}
}

func TestReconnectRetriesAndResetsAfterConnection(t *testing.T) {
	var attempts atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	err := agent.Reconnect(ctx, time.Millisecond, 5*time.Millisecond, func(context.Context) error {
		if attempts.Add(1) < 3 {
			return errors.New("dial")
		}
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if attempts.Load() < 3 {
		t.Fatalf("attempts=%d", attempts.Load())
	}
}
