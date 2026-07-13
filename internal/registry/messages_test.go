package registry_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/metrics"
	"silent-devops/internal/registry"
	"silent-devops/internal/store"
)

func TestMessageHandlerPersistsMetricsAndJobResult(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	s.DB().Exec("INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES('u','x','x',3,?)", now.UnixMilli())
	s.DB().Exec("INSERT INTO agents(id,created_unix_ms) VALUES('a',?)", now.UnixMilli())
	s.DB().Exec("INSERT INTO jobs(id,idempotency_key,actor_id,agent_id,kind,state,reason,created_unix_ms,deadline_unix_ms,attempt,dispatch_id) VALUES('j','i','u','a','typed',2,'r',?,?,1,'d')", now.UnixMilli(), now.Add(time.Minute).UnixMilli())
	h := registry.MessageHandler{Metrics: metrics.NewRepository(s.DB()), DB: s.DB()}
	snapshot := &devopsv1.MetricsSnapshot{SampledUnixMs: now.UnixMilli(), Metrics: []*devopsv1.Metric{{Name: "cpu", Value: 1}}}
	if err := h.Handle(context.Background(), "a", &devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_Metrics{Metrics: snapshot}}); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), "a", &devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_JobResult{JobResult: &devopsv1.JobResult{JobId: "j", DispatchId: "d", Attempt: 1, State: devopsv1.JobState_JOB_STATE_SUCCEEDED}}}); err != nil {
		t.Fatal(err)
	}
	var state int
	if err := s.DB().QueryRow("SELECT state FROM jobs WHERE id='j'").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != 4 {
		t.Fatalf("state=%d", state)
	}
}
func TestMessageHandlerRejectsWrongDispatch(t *testing.T) {
	s, _ := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	defer s.Close()
	h := registry.MessageHandler{DB: s.DB()}
	if err := h.Handle(context.Background(), "a", &devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_JobResult{JobResult: &devopsv1.JobResult{JobId: "missing"}}}); err == nil {
		t.Fatal("unknown job accepted")
	}
}
