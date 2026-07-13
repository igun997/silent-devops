package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"silent-devops/internal/agentjoin"
	"silent-devops/internal/app"
	runtimeapp "silent-devops/internal/runtime"
)

func parseJoin(args []string) (agentjoin.Options, bool, error) {
	if len(args) == 0 || args[0] != "join" {
		return agentjoin.Options{}, false, nil
	}
	if len(args) < 3 {
		return agentjoin.Options{}, true, errors.New("usage: agent join VALIDATOR TOKEN --pin sha256/...")
	}
	options := agentjoin.Options{Address: args[1], Token: args[2], CredentialDir: "/var/lib/silent-devops/agent"}
	flags := flag.NewFlagSet("join", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&options.Pin, "pin", "", "validator certificate pin")
	flags.StringVar(&options.CredentialDir, "credential-dir", options.CredentialDir, "credential directory")
	flags.StringVar(&options.Hostname, "hostname", "", "hostname metadata")
	flags.BoolVar(&options.NoStart, "no-start", false, "do not start service")
	if err := flags.Parse(args[3:]); err != nil {
		return options, true, err
	}
	if options.Pin == "" {
		return options, true, errors.New("--pin required")
	}
	return options, true, nil
}
func main() {
	if options, join, err := parseJoin(os.Args[1:]); join {
		if err == nil {
			if options.Hostname == "" {
				options.Hostname, _ = os.Hostname()
			}
			options.Probe, options.Enroll = agentjoin.DefaultTransport(options.Pin)
			err = agentjoin.Join(context.Background(), options)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !options.NoStart {
			startCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err = exec.CommandContext(startCtx, "systemctl", "enable", "--now", "silent-devops-agent").Run()
			cancel()
			if err != nil {
				fmt.Fprintln(os.Stderr, "credentials stored, but service start failed:", err)
				os.Exit(1)
			}
		}
		fmt.Fprintln(os.Stdout, "agent joined; credentials stored in", options.CredentialDir)
		return
	}
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
