package metrics

import (
	"context"
	"errors"
	"time"
)

func Run(ctx context.Context, interval time.Duration, collect func(context.Context) error) error {
	if interval <= 0 {
		return errors.New("collection interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_ = collect(ctx)
		}
	}
}
