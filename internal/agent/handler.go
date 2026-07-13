package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/maintenance"
)

type Handler struct {
	AgentID    string
	Dispatcher maintenance.Dispatcher
	Send       func(*devopsv1.AgentMessage) error
	mu         sync.Mutex
	seen       map[string]struct{}
}

func (h *Handler) Handle(ctx context.Context, message *devopsv1.ValidatorMessage) error {
	job := message.GetJob()
	if job == nil {
		return nil
	}
	if h.AgentID != "" && job.AgentId != h.AgentID {
		return errors.New("job target mismatch")
	}
	if job.Id == "" || job.DispatchId == "" || job.Attempt == 0 {
		return errors.New("malformed job")
	}
	if time.Now().UnixMilli() > job.DeadlineUnixMs {
		return errors.New("job expired")
	}
	h.mu.Lock()
	if h.seen == nil {
		h.seen = make(map[string]struct{})
	}
	key := job.Id + "\x00" + job.DispatchId
	if _, ok := h.seen[key]; ok {
		h.mu.Unlock()
		return errors.New("duplicate job")
	}
	h.seen[key] = struct{}{}
	h.mu.Unlock()
	result := h.Dispatcher.Dispatch(ctx, job)
	if h.Send == nil {
		return errors.New("agent sender unavailable")
	}
	return h.Send(&devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_JobResult{JobResult: result}})
}
