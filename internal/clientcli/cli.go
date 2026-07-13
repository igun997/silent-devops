package clientcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type API interface {
	Call(context.Context, string, []string) (any, error)
}

var commands = map[string]bool{"login": true, "logout": true, "agents": true, "stats": true, "services": true, "logs": true, "cleanup": true, "reboot": true, "exec": true, "ssh": true, "enroll-token": true, "users": true, "ssh-keys": true, "audit": true, "tui": true}

func Known(command string) bool { return commands[command] }
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, api API, isTTY bool) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "command required")
		return 2
	}
	command := args[0]
	if !Known(command) {
		fmt.Fprintln(stderr, "unknown command:", command)
		return 2
	}
	jsonOutput := removeFlag(&args, "--json")
	yes := removeFlag(&args, "--yes")
	if destructive(command, args) && !yes {
		fmt.Fprintln(stderr, "confirmation required; pass --yes for non-interactive use")
		return 2
	}
	if api == nil {
		fmt.Fprintln(stderr, "client API unavailable")
		return 1
	}
	result, err := api.Call(ctx, command, args[1:])
	if err != nil {
		fmt.Fprintln(stderr, redact(err.Error(), args))
		return 1
	}
	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if command == "login" {
		fmt.Fprintln(stdout, "login succeeded")
		return 0
	}
	if command == "logout" {
		fmt.Fprintln(stdout, "logged out")
		return 0
	}
	text := fmt.Sprint(result)
	if !isTTY {
		text = strings.ReplaceAll(text, "\x1b[", "")
	}
	fmt.Fprintln(stdout, redact(text, args))
	return 0
}
func removeFlag(args *[]string, flag string) bool {
	out := (*args)[:0]
	found := false
	for _, arg := range *args {
		if arg == flag {
			found = true
			continue
		}
		out = append(out, arg)
	}
	*args = out
	return found
}
func destructive(command string, args []string) bool {
	if command == "reboot" || command == "exec" {
		return true
	}
	if command == "cleanup" && len(args) > 1 && args[1] == "run" {
		return true
	}
	if command == "services" && len(args) > 1 {
		return args[1] == "start" || args[1] == "stop" || args[1] == "restart"
	}
	return false
}
func redact(text string, args []string) string {
	for i, arg := range args {
		if i > 0 && (args[i-1] == "login" || strings.Contains(strings.ToLower(args[i-1]), "password") || strings.Contains(strings.ToLower(args[i-1]), "token") || strings.Contains(strings.ToLower(args[i-1]), "private")) {
			text = strings.ReplaceAll(text, arg, "[REDACTED]")
		}
	}
	return text
}

const Usage = `Usage: client <command> [arguments] [--json] [--yes] [--no-color]
Commands: login logout agents stats services logs cleanup reboot exec ssh enroll-token users ssh-keys audit tui`

var ErrNotImplemented = errors.New("client API not configured")
