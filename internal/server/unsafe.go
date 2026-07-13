package server

import (
	"context"
	"encoding/json"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
)

func (s Fleet) PreviewCleanup(ctx context.Context, r *devopsv1.CleanupPreviewJobRequest) (*devopsv1.Job, error) {
	return s.submitSafe(ctx, r.GetContext(), &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_CleanupPreview{CleanupPreview: r.GetRequest()}}, "cleanup_preview")
}
func (s Fleet) RunCleanup(ctx context.Context, r *devopsv1.CleanupRunJobRequest) (*devopsv1.Job, error) {
	if r.GetRequest() == nil || r.Request.PreviewId == "" || len(r.Request.PreviewHash) == 0 || r.Request.PreviewExpiresUnixMs < s.now().UnixMilli() {
		return nil, status.Error(codes.InvalidArgument, "matching unexpired preview required")
	}
	return s.submitUnsafe(ctx, r.Context, &devopsv1.UnsafeOperation{Operation: &devopsv1.UnsafeOperation_CleanupRun{CleanupRun: r.Request}}, "cleanup_run", false)
}
func (s Fleet) Reboot(ctx context.Context, r *devopsv1.RebootJobRequest) (*devopsv1.Job, error) {
	if r.GetRequest() == nil || r.Request.TargetAgentId != r.GetContext().GetAgentId() || r.Request.Confirmation == "" || r.Request.ConfirmationExpiresUnixMs < s.now().UnixMilli() {
		return nil, status.Error(codes.InvalidArgument, "target-bound unexpired confirmation required")
	}
	return s.submitUnsafe(ctx, r.Context, &devopsv1.UnsafeOperation{Operation: &devopsv1.UnsafeOperation_Reboot{Reboot: r.Request}}, "reboot", false)
}
func (s Fleet) Exec(ctx context.Context, r *devopsv1.ExecJobRequest) (*devopsv1.Job, error) {
	if r.GetRequest() == nil || r.Request.Command == "" || !r.GetContext().GetConfirmed() {
		return nil, status.Error(codes.InvalidArgument, "command and confirmation required")
	}
	return s.submitUnsafe(ctx, r.Context, &devopsv1.UnsafeOperation{Operation: &devopsv1.UnsafeOperation_ArbitraryCommand{ArbitraryCommand: r.Request}}, "exec", true)
}
func (s Fleet) submitUnsafe(ctx context.Context, r *devopsv1.JobRequestContext, operation *devopsv1.UnsafeOperation, kind string, adminOnly bool) (*devopsv1.Job, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if adminOnly && claims.Role != devopsv1.Role_ROLE_ADMIN {
		return nil, status.Error(codes.PermissionDenied, "admin required")
	}
	if r == nil || r.AgentId == "" || r.Reason == "" || r.IdempotencyKey == "" || r.TimeoutSeconds == 0 || r.TimeoutSeconds > 3600 || !r.Confirmed {
		return nil, status.Error(codes.InvalidArgument, "confirmed bounded job context required")
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	dispatch, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	auditID, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	now := s.now()
	job := &devopsv1.Job{Id: id, ActorId: claims.Subject, AgentId: r.AgentId, CreatedUnixMs: now.UnixMilli(), DeadlineUnixMs: now.Add(time.Duration(r.TimeoutSeconds) * time.Second).UnixMilli(), Authorization: &devopsv1.AuthorizationContext{Role: claims.Role, Reason: r.Reason, Confirmed: true}, Operation: &devopsv1.Job_UnsafeOperation{UnsafeOperation: operation}, State: devopsv1.JobState_JOB_STATE_DISPATCHED, Attempt: 1, DispatchId: dispatch, IdempotencyKey: r.IdempotencyKey}
	metadata, _ := json.Marshal(map[string]any{"job_id": id, "kind": kind, "dispatch_id": dispatch})
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "storage unavailable")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO jobs(id,idempotency_key,actor_id,agent_id,kind,state,reason,created_unix_ms,deadline_unix_ms,attempt,dispatch_id) VALUES(?,?,?,?,?,?,?,?,?,?,?)", id, r.IdempotencyKey, claims.Subject, r.AgentId, kind, job.State, r.Reason, job.CreatedUnixMs, job.DeadlineUnixMs, 1, dispatch); err != nil {
		return nil, status.Error(codes.AlreadyExists, "duplicate job")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit_events(id,actor_id,agent_id,action,reason,occurred_unix_ms,metadata) VALUES(?,?,?,?,?,?,?)", auditID, claims.Subject, r.AgentId, kind, r.Reason, job.CreatedUnixMs, metadata); err != nil {
		return nil, status.Error(codes.Unavailable, "audit unavailable")
	}
	if err := tx.Commit(); err != nil {
		return nil, status.Error(codes.Unavailable, "storage unavailable")
	}
	if err := s.Registry.Dispatch(ctx, r.AgentId, &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Job{Job: job}}); err != nil {
		_, _ = s.DB.ExecContext(ctx, "UPDATE jobs SET state=? WHERE id=?", devopsv1.JobState_JOB_STATE_UNKNOWN_RESULT, id)
		return nil, status.Error(codes.Unavailable, "agent offline or backpressured")
	}
	return job, nil
}
