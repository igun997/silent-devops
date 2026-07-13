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
	if len(args) > 0 && args[0] != "help" && args[0] != "version" && args[0] != "-h" && args[0] != "--help" && args[0] != "-v" && args[0] != "--version" {
		socket := os.Getenv("SILENT_DEVOPS_LOCAL_SOCKET")
		if socket == "" {
			socket = "/run/silent-devops/validator.sock"
		}
		if err := runLocal(context.Background(), args, os.Stdout, socket); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
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
