package agent

import (
	"context"
	"errors"
	devopsv1 "silent-devops/api/devops/v1"
	"time"
)

type MetricsPublisher struct {
	Interval time.Duration
	Collect  func(context.Context) (*devopsv1.MetricsSnapshot, error)
	Send     func(*devopsv1.AgentMessage) error
}

func (p MetricsPublisher) Run(ctx context.Context) error {
	if p.Interval <= 0 || p.Collect == nil || p.Send == nil {
		return errors.New("metrics publisher configuration required")
	}
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		snapshot, err := p.Collect(ctx)
		if err == nil {
			err = p.Send(&devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_Metrics{Metrics: snapshot}})
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
