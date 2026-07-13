package registry_test

import (
	"context"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/registry"
)

func TestDispatchToActiveAgent(t *testing.T) {
	r := registry.New(1, devopsv1.DefaultLimits(), time.Minute)
	session, _, err := r.Acquire("a", "a", hello("a"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Release(time.Now())
	message := &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Job{Job: &devopsv1.Job{Id: "j"}}}
	if err := r.Dispatch(context.Background(), "a", message); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-session.Messages():
		if got.GetJob().GetId() != "j" {
			t.Fatal("wrong job")
		}
	case <-time.After(time.Second):
		t.Fatal("job not delivered")
	}
}
func TestDispatchRejectsOffline(t *testing.T) {
	r := registry.New(1, devopsv1.DefaultLimits(), time.Minute)
	if err := r.Dispatch(context.Background(), "missing", &devopsv1.ValidatorMessage{}); err != registry.ErrAgentOffline {
		t.Fatalf("err=%v", err)
	}
}
