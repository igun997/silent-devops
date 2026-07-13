package agent_test

import (
	"context"
	devopsv1 "silent-devops/api/devops/v1"
	agentstream "silent-devops/internal/agent"
	"testing"
	"time"
)

func TestMetricsPublisherSendsImmediatelyAndPeriodically(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan *devopsv1.AgentMessage, 2)
	p := agentstream.MetricsPublisher{Interval: 10 * time.Millisecond, Collect: func(context.Context) (*devopsv1.MetricsSnapshot, error) {
		return &devopsv1.MetricsSnapshot{SampledUnixMs: 1}, nil
	}, Send: func(m *devopsv1.AgentMessage) error {
		sent <- m
		if len(sent) == 2 {
			cancel()
		}
		return nil
	}}
	_ = p.Run(ctx)
	if len(sent) < 2 {
		t.Fatalf("sent=%d", len(sent))
	}
}
