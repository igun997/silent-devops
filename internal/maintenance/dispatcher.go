package maintenance

import (
	"context"
	"errors"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
)

type Dispatcher struct {
	Runner     Runner
	Operations Operations
	Timeout    time.Duration
}

func (d Dispatcher) Dispatch(ctx context.Context, job *devopsv1.Job) *devopsv1.JobResult {
	result := &devopsv1.JobResult{JobId: job.GetId(), State: devopsv1.JobState_JOB_STATE_REJECTED, DispatchId: job.GetDispatchId(), Attempt: job.GetAttempt()}
	if err := ctx.Err(); err != nil {
		result.State = devopsv1.JobState_JOB_STATE_CANCELLED
		result.Error = protocolError(devopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, err.Error(), false)
		return result
	}
	var argv []string
	if unsafe := job.GetUnsafeOperation(); unsafe != nil {
		switch req := unsafe.Operation.(type) {
		case *devopsv1.UnsafeOperation_ArbitraryCommand:
			if req.ArbitraryCommand.Command == "" || !job.GetAuthorization().GetConfirmed() {
				result.Error = protocolError(devopsv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "confirmed command required", false)
				return result
			}
			argv = []string{"/bin/sh", "-c", req.ArbitraryCommand.Command}
		case *devopsv1.UnsafeOperation_Reboot:
			if req.Reboot.TargetAgentId != job.AgentId || req.Reboot.Confirmation == "" {
				result.Error = protocolError(devopsv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "target confirmation required", false)
				return result
			}
			argv = []string{"systemctl", "reboot"}
		default:
			result.Error = protocolError(devopsv1.ErrorCode_ERROR_CODE_UNSUPPORTED, "unsupported unsafe operation", false)
			return result
		}
	} else if safe := job.GetSafeOperation(); safe != nil && safe.Operation != nil {
		switch req := safe.Operation.Request.(type) {
		case *devopsv1.TypedOperation_ProcessList:
			argv = d.Operations.Processes(req.ProcessList.Limit)
		case *devopsv1.TypedOperation_ServiceList:
			argv = d.Operations.ListServices(req.ServiceList.Limit)
		case *devopsv1.TypedOperation_ServiceStatus:
			argv = d.Operations.Service("status", req.ServiceStatus.Unit)
		case *devopsv1.TypedOperation_ServiceStart:
			argv = d.Operations.Service("start", req.ServiceStart.Unit)
		case *devopsv1.TypedOperation_ServiceStop:
			argv = d.Operations.Service("stop", req.ServiceStop.Unit)
		case *devopsv1.TypedOperation_ServiceRestart:
			argv = d.Operations.Service("restart", req.ServiceRestart.Unit)
		case *devopsv1.TypedOperation_JournalRead:
			argv = d.Operations.Journal(req.JournalRead.Unit, time.UnixMilli(req.JournalRead.SinceUnixMs), time.UnixMilli(req.JournalRead.UntilUnixMs), req.JournalRead.LineLimit)
		default:
			result.Error = protocolError(devopsv1.ErrorCode_ERROR_CODE_UNSUPPORTED, "unsupported typed operation", false)
			return result
		}
	} else {
		result.Error = protocolError(devopsv1.ErrorCode_ERROR_CODE_UNSUPPORTED, "operation required", false)
		return result
	}
	if len(argv) == 0 {
		result.Error = protocolError(devopsv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "invalid operation arguments", false)
		return result
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	run := d.Runner.Run(ctx, timeout, argv[0], argv[1:]...)
	result.ExitCode = int32(run.ExitCode)
	result.OutputTruncated = run.Truncated
	result.Output = run.Output
	if run.Err == nil {
		result.State = devopsv1.JobState_JOB_STATE_SUCCEEDED
		return result
	}
	switch {
	case errors.Is(run.Err, context.Canceled):
		result.State = devopsv1.JobState_JOB_STATE_CANCELLED
	case errors.Is(run.Err, context.DeadlineExceeded):
		result.State = devopsv1.JobState_JOB_STATE_TIMED_OUT
		result.Error = protocolError(devopsv1.ErrorCode_ERROR_CODE_DEADLINE_EXCEEDED, run.Err.Error(), false)
	default:
		result.State = devopsv1.JobState_JOB_STATE_FAILED
		result.Error = protocolError(devopsv1.ErrorCode_ERROR_CODE_INTERNAL, run.Err.Error(), false)
	}
	return result
}
func protocolError(code devopsv1.ErrorCode, message string, retryable bool) *devopsv1.ProtocolError {
	return &devopsv1.ProtocolError{Code: code, Message: message, Details: []*devopsv1.ErrorDetail{{Retryable: retryable}}}
}
