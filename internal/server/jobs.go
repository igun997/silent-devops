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

func (s Fleet) ListProcesses(ctx context.Context, request *devopsv1.ProcessListJobRequest) (*devopsv1.Job, error) {
	return s.submitSafe(ctx, request.GetContext(), &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_ProcessList{ProcessList: request.GetRequest()}}, "process_list")
}
func (s Fleet) ListServices(ctx context.Context, request *devopsv1.ServiceListJobRequest) (*devopsv1.Job, error) {
	return s.submitSafe(ctx, request.GetContext(), &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_ServiceList{ServiceList: request.GetRequest()}}, "service_list")
}
func (s Fleet) GetService(ctx context.Context, request *devopsv1.ServiceJobRequest) (*devopsv1.Job, error) {
	return s.submitSafe(ctx, request.GetContext(), &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_ServiceStatus{ServiceStatus: request.GetRequest()}}, "service_status")
}
func (s Fleet) StartService(ctx context.Context, request *devopsv1.ServiceJobRequest) (*devopsv1.Job, error) {
	return s.submitSafe(ctx, request.GetContext(), &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_ServiceStart{ServiceStart: request.GetRequest()}}, "service_start")
}
func (s Fleet) StopService(ctx context.Context, request *devopsv1.ServiceJobRequest) (*devopsv1.Job, error) {
	return s.submitSafe(ctx, request.GetContext(), &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_ServiceStop{ServiceStop: request.GetRequest()}}, "service_stop")
}
func (s Fleet) RestartService(ctx context.Context, request *devopsv1.ServiceJobRequest) (*devopsv1.Job, error) {
	return s.submitSafe(ctx, request.GetContext(), &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_ServiceRestart{ServiceRestart: request.GetRequest()}}, "service_restart")
}
func (s Fleet) ReadLogs(ctx context.Context, request *devopsv1.JournalJobRequest) (*devopsv1.Job, error) {
	return s.submitSafe(ctx, request.GetContext(), &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_JournalRead{JournalRead: request.GetRequest()}}, "journal_read")
}
func (s Fleet) submitSafe(ctx context.Context, request *devopsv1.JobRequestContext, operation *devopsv1.TypedOperation, kind string) (*devopsv1.Job, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if request == nil || request.AgentId == "" || request.Reason == "" || request.IdempotencyKey == "" || request.TimeoutSeconds == 0 || request.TimeoutSeconds > 3600 {
		return nil, status.Error(codes.InvalidArgument, "invalid job context")
	}
	if s.Registry == nil {
		return nil, status.Error(codes.Unavailable, "agent registry unavailable")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	dispatch, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	job := &devopsv1.Job{Id: id, ActorId: claims.Subject, AgentId: request.AgentId, CreatedUnixMs: now().UnixMilli(), DeadlineUnixMs: now().Add(time.Duration(request.TimeoutSeconds) * time.Second).UnixMilli(), Authorization: &devopsv1.AuthorizationContext{Role: claims.Role, Reason: request.Reason, Confirmed: request.Confirmed}, Operation: &devopsv1.Job_SafeOperation{SafeOperation: &devopsv1.SafeOperation{Operation: operation, RetryPolicy: devopsv1.RetryPolicy_RETRY_POLICY_SAFE_BEFORE_START}}, State: devopsv1.JobState_JOB_STATE_DISPATCHED, Attempt: 1, DispatchId: dispatch, IdempotencyKey: request.IdempotencyKey}
	payload, _ := json.Marshal(map[string]any{"job_id": id, "kind": kind, "dispatch_id": dispatch})
	auditID, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "storage unavailable")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO jobs(id,idempotency_key,actor_id,agent_id,kind,state,reason,created_unix_ms,deadline_unix_ms,attempt,dispatch_id) VALUES(?,?,?,?,?,?,?,?,?,?,?)", id, request.IdempotencyKey, claims.Subject, request.AgentId, kind, job.State, request.Reason, job.CreatedUnixMs, job.DeadlineUnixMs, 1, dispatch); err != nil {
		return nil, status.Error(codes.AlreadyExists, "duplicate job")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit_events(id,actor_id,agent_id,action,reason,occurred_unix_ms,metadata) VALUES(?,?,?,?,?,?,?)", auditID, claims.Subject, request.AgentId, kind, request.Reason, job.CreatedUnixMs, payload); err != nil {
		return nil, status.Error(codes.Unavailable, "audit unavailable")
	}
	if err := tx.Commit(); err != nil {
		return nil, status.Error(codes.Unavailable, "storage unavailable")
	}
	if err := s.Registry.Dispatch(ctx, request.AgentId, &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Job{Job: job}}); err != nil {
		_, _ = s.DB.ExecContext(ctx, "UPDATE jobs SET state=? WHERE id=?", devopsv1.JobState_JOB_STATE_UNKNOWN_RESULT, id)
		return nil, status.Error(codes.Unavailable, "agent offline or backpressured")
	}
	return job, nil
}
