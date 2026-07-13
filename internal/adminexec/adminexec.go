package adminexec

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/maintenance"
)

type Request struct {
	ActorID                  string
	Role                     devopsv1.Role
	AgentID, Reason, Command string
	Timeout                  time.Duration
	Confirmed, Capture       bool
}

func (r Request) Validate() error {
	switch {
	case r.Role != devopsv1.Role_ROLE_ADMIN:
		return errors.New("admin role required")
	case r.ActorID == "" || r.AgentID == "":
		return errors.New("actor and target required")
	case r.Reason == "" || len(r.Reason) > 2048:
		return errors.New("bounded reason required")
	case r.Command == "" || len(r.Command) > 64<<10:
		return errors.New("bounded command required")
	case r.Timeout <= 0 || r.Timeout > time.Hour:
		return errors.New("bounded timeout required")
	case !r.Confirmed:
		return errors.New("confirmation required")
	}
	return nil
}

type Executor struct {
	DB     *sql.DB
	Runner maintenance.Runner
	Shell  string
	Now    func() time.Time
}

func (e Executor) Execute(ctx context.Context, request Request) (maintenance.Result, error) {
	if err := request.Validate(); err != nil {
		return maintenance.Result{}, err
	}
	shell := e.Shell
	if shell == "" {
		shell = "/bin/sh"
	}
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	id, err := randomID()
	if err != nil {
		return maintenance.Result{}, err
	}
	created := now()
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return maintenance.Result{}, err
	}
	metadata, _ := json.Marshal(map[string]any{"command": request.Command, "capture": request.Capture, "timeout_ms": request.Timeout.Milliseconds()})
	if _, err := tx.ExecContext(ctx, "INSERT INTO jobs(id,idempotency_key,actor_id,agent_id,kind,state,reason,created_unix_ms,deadline_unix_ms,capture_output) VALUES(?,?,?,?,?,?,?,?,?,?)", id, id, request.ActorID, request.AgentID, "arbitrary", devopsv1.JobState_JOB_STATE_RUNNING, request.Reason, created.UnixMilli(), created.Add(request.Timeout).UnixMilli(), request.Capture); err != nil {
		tx.Rollback()
		return maintenance.Result{}, err
	}
	auditID, err := randomID()
	if err != nil {
		return maintenance.Result{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit_events(id,actor_id,agent_id,action,reason,occurred_unix_ms,metadata) VALUES(?,?,?,?,?,?,?)", auditID, request.ActorID, request.AgentID, "arbitrary_command", request.Reason, created.UnixMilli(), metadata); err != nil {
		tx.Rollback()
		return maintenance.Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return maintenance.Result{}, err
	}
	result := e.Runner.Run(ctx, request.Timeout, shell, "-c", request.Command)
	state := devopsv1.JobState_JOB_STATE_FAILED
	if result.Err == nil {
		state = devopsv1.JobState_JOB_STATE_SUCCEEDED
	} else if errors.Is(result.Err, context.DeadlineExceeded) {
		state = devopsv1.JobState_JOB_STATE_TIMED_OUT
	} else if errors.Is(result.Err, context.Canceled) {
		state = devopsv1.JobState_JOB_STATE_CANCELLED
	}
	var output any
	if request.Capture {
		output = result.Output
	}
	_, err = e.DB.ExecContext(ctx, "UPDATE jobs SET state=?,output=?,output_truncated=? WHERE id=?", state, output, request.Capture && result.Truncated, id)
	return result, err
}
func randomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
