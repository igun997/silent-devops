package server_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"silent-devops/internal/registry"
	"silent-devops/internal/server"
	"silent-devops/internal/store"
)

func TestSubmitTypedJobPersistsBeforeDispatch(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	s.DB().Exec("INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES('u','op','x',2,?)", now.UnixMilli())
	s.DB().Exec("INSERT INTO agents(id,created_unix_ms) VALUES('a',?)", now.UnixMilli())
	r := registry.New(1, devopsv1.DefaultLimits(), time.Minute)
	session, _, _ := r.Acquire("a", "a", &devopsv1.AgentHello{AgentId: "a", Protocol: &devopsv1.VersionRange{Minimum: 1, Maximum: 1}, Limits: devopsv1.DefaultLimits()}, now)
	defer session.Release(now)
	fleet := server.Fleet{DB: s.DB(), Registry: r, Now: func() time.Time { return now }}
	ctx := auth.ContextWithClaims(context.Background(), auth.Claims{Subject: "u", Role: devopsv1.Role_ROLE_OPERATOR})
	job, err := fleet.ListProcesses(ctx, &devopsv1.ProcessListJobRequest{Context: &devopsv1.JobRequestContext{AgentId: "a", Reason: "inspect", TimeoutSeconds: 30, IdempotencyKey: "idem"}, Request: &devopsv1.ProcessListRequest{Limit: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != devopsv1.JobState_JOB_STATE_DISPATCHED {
		t.Fatalf("state=%v", job.State)
	}
	select {
	case message := <-session.Messages():
		if message.GetJob().Id != job.Id {
			t.Fatal("wrong dispatch")
		}
	case <-time.After(time.Second):
		t.Fatal("not dispatched")
	}
	var count int
	s.DB().QueryRow("SELECT count(*) FROM audit_events WHERE actor_id='u' AND agent_id='a'").Scan(&count)
	if count != 1 {
		t.Fatalf("audit=%d", count)
	}
	if _, err := fleet.ListProcesses(ctx, &devopsv1.ProcessListJobRequest{Context: &devopsv1.JobRequestContext{AgentId: "a", Reason: "inspect", TimeoutSeconds: 30, IdempotencyKey: "idem"}, Request: &devopsv1.ProcessListRequest{Limit: 10}}); err == nil {
		t.Fatal("duplicate accepted")
	}
}
