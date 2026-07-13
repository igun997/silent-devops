package agent

import (
	"context"
	"errors"
	"math/rand"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
)

type Stream interface {
	Send(*devopsv1.AgentMessage) error
	Recv() (*devopsv1.ValidatorMessage, error)
	CloseSend() error
}

func RunConnection(ctx context.Context, stream Stream, hello *devopsv1.AgentHello, heartbeatInterval time.Duration, handle func(*devopsv1.ValidatorMessage)) error {
	if hello == nil || hello.AgentId == "" {
		return errors.New("agent hello required")
	}
	if heartbeatInterval <= 0 {
		return errors.New("heartbeat interval must be positive")
	}
	defer stream.CloseSend()
	if err := stream.Send(&devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_Hello{Hello: hello}}); err != nil {
		return err
	}
	errCh := make(chan error, 2)
	go func() {
		for {
			m, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}
			if handle != nil {
				handle(m)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case now := <-ticker.C:
				if err := stream.Send(&devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_Heartbeat{Heartbeat: &devopsv1.Heartbeat{SentUnixMs: now.UnixMilli()}}}); err != nil {
					errCh <- err
					return
				}
			}
		}
	}()
	return <-errCh
}

func Reconnect(ctx context.Context, min, max time.Duration, connect func(context.Context) error) error {
	if min <= 0 || max < min {
		return errors.New("invalid reconnect bounds")
	}
	delay := min
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := connect(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			delay = min
		}
		wait := time.Duration(rng.Int63n(int64(delay) + 1))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < max {
			delay *= 2
			if delay > max {
				delay = max
			}
		}
	}
}
