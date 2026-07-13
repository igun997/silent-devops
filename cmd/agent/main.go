package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"silent-devops/internal/app"
	runtimeapp "silent-devops/internal/runtime"
)

func main() {
	os.Exit(app.Run(context.Background(), "agent", os.Args[1:], os.Stdout, os.Stderr, func(context.Context) error {
		cfg, err := runtimeapp.ParseAgentConfig(os.Getenv)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		err = runtimeapp.RunAgent(ctx, cfg)
		if err == context.Canceled {
			return nil
		}
		return err
	}))
}
