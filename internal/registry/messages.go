package registry

import (
	"context"
	"database/sql"
	"errors"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/metrics"
)

type SSHReadyHandler interface {
	Ready(context.Context, string, uint32, []byte, []byte) (*devopsv1.SshSession, error)
}
type MessageHandler struct {
	DB      *sql.DB
	Metrics *metrics.Repository
	SSH     SSHReadyHandler
}

func (h MessageHandler) Handle(ctx context.Context, agentID string, message *devopsv1.AgentMessage) error {
	if message == nil {
		return errors.New("message required")
	}
	switch payload := message.Payload.(type) {
	case *devopsv1.AgentMessage_Heartbeat:
		return nil
	case *devopsv1.AgentMessage_Metrics:
		if h.Metrics == nil {
			return errors.New("metrics unavailable")
		}
		bounded := metrics.Bound(payload.Metrics, int(devopsv1.DefaultLimits().MaxMetrics), int(devopsv1.DefaultLimits().MaxLabelsPerMetric))
		return h.Metrics.Store(ctx, agentID, bounded)
	case *devopsv1.AgentMessage_SshReady:
		if h.SSH == nil {
			return errors.New("SSH unavailable")
		}
		ready := payload.SshReady
		_, err := h.SSH.Ready(ctx, ready.SessionId, ready.ValidatorLoopbackPort, ready.BindingToken, ready.HostKey)
		return err
	case *devopsv1.AgentMessage_JobResult:
		if h.DB == nil {
			return errors.New("storage unavailable")
		}
		result := payload.JobResult
		updated, err := h.DB.ExecContext(ctx, "UPDATE jobs SET state=?,output=?,output_truncated=? WHERE id=? AND agent_id=? AND dispatch_id=? AND attempt=? AND state IN (2,3)", result.State, result.Output, result.OutputTruncated, result.JobId, agentID, result.DispatchId, result.Attempt)
		if err != nil {
			return err
		}
		count, _ := updated.RowsAffected()
		if count != 1 {
			return errors.New("unknown or stale job result")
		}
		return nil
	default:
		return nil
	}
}
