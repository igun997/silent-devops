package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"silent-devops/internal/app"
	runtimeapp "silent-devops/internal/runtime"
)

func main() {
	args := os.Args[1:]
	code := app.Run(context.Background(), "validator", args, os.Stdout, os.Stderr, func(context.Context) error {
		cfg, err := runtimeapp.ParseValidatorConfig(os.Getenv)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		err = runtimeapp.RunValidator(ctx, cfg)
		if err == context.Canceled {
			return nil
		}
		return err
	})
	if code != 0 {
		fmt.Fprint(os.Stderr, "")
	}
	os.Exit(code)
}
