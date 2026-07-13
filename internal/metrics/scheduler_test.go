package metrics_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"silent-devops/internal/metrics"
)

func TestSchedulerContinuesAfterCollectionFailure(t *testing.T) {
	var calls atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()
	err := metrics.Run(ctx, 10*time.Millisecond, func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("temporary")
		}
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if calls.Load() < 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
}
