package maintenance_test

import (
	"context"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/maintenance"
)

func TestDispatchRejectsArbitraryAndMapsExitStates(t *testing.T) {
	d := maintenance.Dispatcher{Runner: maintenance.Runner{MaxOutputBytes: 1024}, Timeout: time.Second}
	job := &devopsv1.Job{Id: "j", Operation: &devopsv1.Job_UnsafeOperation{UnsafeOperation: &devopsv1.UnsafeOperation{Operation: &devopsv1.UnsafeOperation_ArbitraryCommand{ArbitraryCommand: &devopsv1.ArbitraryCommand{Command: "id"}}}}}
	result := d.Dispatch(context.Background(), job)
	if result.State != devopsv1.JobState_JOB_STATE_REJECTED {
		t.Fatalf("state=%v", result.State)
	}
	job.Operation = &devopsv1.Job_SafeOperation{SafeOperation: &devopsv1.SafeOperation{Operation: &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_ServiceStatus{ServiceStatus: &devopsv1.ServiceRequest{Unit: "../bad"}}}}}
	result = d.Dispatch(context.Background(), job)
	if result.State != devopsv1.JobState_JOB_STATE_REJECTED {
		t.Fatalf("state=%v", result.State)
	}
}
func TestDispatchCancellationBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := maintenance.Dispatcher{Runner: maintenance.Runner{MaxOutputBytes: 10}, Timeout: time.Second}
	job := &devopsv1.Job{Id: "j", Operation: &devopsv1.Job_SafeOperation{SafeOperation: &devopsv1.SafeOperation{Operation: &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_ProcessList{ProcessList: &devopsv1.ProcessListRequest{Limit: 1}}}}}}
	result := d.Dispatch(ctx, job)
	if result.State != devopsv1.JobState_JOB_STATE_CANCELLED {
		t.Fatalf("state=%v", result.State)
	}
}
