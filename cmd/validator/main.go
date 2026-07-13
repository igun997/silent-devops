package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"silent-devops/internal/app"
	runtimeapp "silent-devops/internal/runtime"
	"silent-devops/internal/validatorinit"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "init" {
		flags := flag.NewFlagSet("init", flag.ExitOnError)
		address := flags.String("public-address", "", "public validator address")
		dir := flags.String("dir", "/etc/silent-devops", "configuration directory")
		cidrs := flags.String("agent-cidrs", "0.0.0.0/0,::/0", "agent CIDRs")
		_ = flags.Parse(args[1:])
		result, err := validatorinit.Init(validatorinit.Options{PublicAddress: *address, Dir: *dir, AgentCIDRs: *cidrs})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("validator initialized; certificate pin:", result.Pin)
		_ = exec.Command("systemctl", "enable", "--now", "silent-devops-validator").Run()
		return
	}
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
