package adminexec_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/adminexec"
	"silent-devops/internal/maintenance"
	"silent-devops/internal/store"
)

func TestValidateRequiresAdminTargetReasonConfirmation(t *testing.T) {
	base := adminexec.Request{ActorID: "u", Role: devopsv1.Role_ROLE_ADMIN, AgentID: "a", Reason: "incident", Command: "printf ok", Timeout: time.Second, Confirmed: true}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []adminexec.Request{base, base, base, base}
	tests[0].Role = devopsv1.Role_ROLE_OPERATOR
	tests[1].AgentID = ""
	tests[2].Reason = ""
	tests[3].Confirmed = false
	for _, request := range tests {
		if request.Validate() == nil {
			t.Fatalf("accepted %+v", request)
		}
	}
}
func TestExecuteAuditAndExplicitCapture(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	s.DB().Exec("INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES('u','admin','x',3,?)", now.UnixMilli())
	s.DB().Exec("INSERT INTO agents(id,created_unix_ms) VALUES('a',?)", now.UnixMilli())
	executor := adminexec.Executor{DB: s.DB(), Runner: maintenance.Runner{MaxOutputBytes: 4}, Shell: "/bin/sh", Now: func() time.Time { return now }}
	request := adminexec.Request{ActorID: "u", Role: devopsv1.Role_ROLE_ADMIN, AgentID: "a", Reason: "incident", Command: "printf 123456", Timeout: time.Second, Confirmed: true, Capture: true}
	result, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Output) != "1234" || !result.Truncated {
		t.Fatalf("result=%+v", result)
	}
	var output []byte
	var truncated bool
	if err := s.DB().QueryRow("SELECT output,output_truncated FROM jobs").Scan(&output, &truncated); err != nil {
		t.Fatal(err)
	}
	if string(output) != "1234" || !truncated {
		t.Fatal("capture not persisted")
	}
	var count int
	s.DB().QueryRow("SELECT count(*) FROM audit_events WHERE actor_id='u' AND agent_id='a' AND reason='incident'").Scan(&count)
	if count != 1 {
		t.Fatalf("audit count=%d", count)
	}
}
