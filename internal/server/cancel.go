package server

import (
	"context"
	"database/sql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
)

func (s Fleet) CancelJob(ctx context.Context, r *devopsv1.CancelJobRequest) (*devopsv1.Job, error) {
	if r.JobId == "" || r.Reason == "" || r.DeadlineUnixMs <= s.now().UnixMilli() {
		return nil, status.Error(codes.InvalidArgument, "job, reason, and future deadline required")
	}
	var job devopsv1.Job
	var state int32
	if err := s.DB.QueryRowContext(ctx, "SELECT id,agent_id,state,dispatch_id,attempt,deadline_unix_ms FROM jobs WHERE id=?", r.JobId).Scan(&job.Id, &job.AgentId, &state, &job.DispatchId, &job.Attempt, &job.DeadlineUnixMs); err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "job not found")
	} else if err != nil {
		return nil, status.Error(codes.Internal, "job unavailable")
	}
	job.State = devopsv1.JobState(state)
	switch job.State {
	case devopsv1.JobState_JOB_STATE_SUCCEEDED, devopsv1.JobState_JOB_STATE_FAILED, devopsv1.JobState_JOB_STATE_CANCELLED, devopsv1.JobState_JOB_STATE_TIMED_OUT:
		return &job, nil
	}
	if s.Registry == nil {
		return nil, status.Error(codes.Unavailable, "registry unavailable")
	}
	message := &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Cancel{Cancel: &devopsv1.CancelJob{JobId: job.Id, Reason: r.Reason, DeadlineUnixMs: r.DeadlineUnixMs, RequestId: job.DispatchId}}}
	if err := s.Registry.Dispatch(ctx, job.AgentId, message); err != nil {
		return nil, status.Error(codes.Unavailable, "agent offline")
	}
	_, err := s.DB.ExecContext(ctx, "UPDATE jobs SET state=? WHERE id=? AND state IN (2,3)", devopsv1.JobState_JOB_STATE_CANCELLED, job.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "cancel persistence failed")
	}
	job.State = devopsv1.JobState_JOB_STATE_CANCELLED
	return &job, nil
}
