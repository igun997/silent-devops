package registry_test

import (
	"context"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/registry"
)

func hello(id string) *devopsv1.AgentHello {
	return &devopsv1.AgentHello{AgentId: id, Protocol: &devopsv1.VersionRange{Minimum: 1, Maximum: 1}, Limits: devopsv1.DefaultLimits()}
}

func TestOneActiveStreamAndOfflineOnRelease(t *testing.T) {
	r := registry.New(1, devopsv1.DefaultLimits(), time.Minute)
	s, state, err := r.Acquire("agent-1", "agent-1", hello("agent-1"), time.Now())
	if err != nil || !state.Accepted {
		t.Fatal(err)
	}
	if _, _, err := r.Acquire("agent-1", "agent-1", hello("agent-1"), time.Now()); err != registry.ErrDuplicateStream {
		t.Fatalf("duplicate err=%v", err)
	}
	if !r.Snapshot("agent-1").Online {
		t.Fatal("agent offline")
	}
	s.Release(time.Now())
	if r.Snapshot("agent-1").Online {
		t.Fatal("agent stayed online")
	}
}

func TestIdentityVersionAndCapabilityValidation(t *testing.T) {
	r := registry.New(2, devopsv1.DefaultLimits(), time.Minute)
	if _, _, err := r.Acquire("cert-id", "hello-id", hello("hello-id"), time.Now()); err != registry.ErrIdentityMismatch {
		t.Fatalf("identity err=%v", err)
	}
	h := hello("a")
	h.Protocol = &devopsv1.VersionRange{Minimum: 3, Maximum: 4}
	if _, _, err := r.Acquire("a", "a", h, time.Now()); err != registry.ErrVersionMismatch {
		t.Fatalf("version err=%v", err)
	}
	h = hello("a")
	h.Protocol = &devopsv1.VersionRange{Minimum: 1, Maximum: 2}
	h.Capabilities = make([]*devopsv1.Capability, devopsv1.DefaultLimits().MaxCapabilities+1)
	if _, _, err := r.Acquire("a", "a", h, time.Now()); err != registry.ErrLimitsExceeded {
		t.Fatalf("limits err=%v", err)
	}
}

func TestBackpressureHeartbeatAndStaleRelease(t *testing.T) {
	r := registry.New(1, &devopsv1.ProtocolLimits{MaxQueueDepth: 1, MaxCapabilities: 8, MaxMessageBytes: 1024}, time.Second)
	s, _, err := r.Acquire("a", "a", hello("a"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue(context.Background(), &devopsv1.ValidatorMessage{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := s.Enqueue(ctx, &devopsv1.ValidatorMessage{}); err == nil {
		t.Fatal("full queue accepted")
	}
	now := time.Now()
	s.Heartbeat(now)
	r.Expire(now.Add(2 * time.Second))
	if r.Snapshot("a").Online {
		t.Fatal("stale agent online")
	}
	s.Release(now.Add(3 * time.Second))
}

func TestBackoffBoundsAndReset(t *testing.T) {
	b := registry.NewBackoff(time.Second, time.Minute, 1)
	for i := 0; i < 20; i++ {
		d := b.Next()
		if d < 0 || d > time.Minute {
			t.Fatalf("delay=%v", d)
		}
	}
	b.Reset()
	if d := b.Next(); d > time.Second {
		t.Fatalf("reset delay=%v", d)
	}
}
