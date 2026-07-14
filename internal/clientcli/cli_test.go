package clientcli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"silent-devops/internal/clientcli"
)

type fakeAPI struct {
	called  string
	command string
	result  any
}

func (f *fakeAPI) Call(_ context.Context, command string, args []string) (any, error) {
	f.called = command + " " + strings.Join(args, " ")
	f.command = command
	if f.result != nil {
		return f.result, nil
	}
	return map[string]any{"ok": true}, nil
}
func TestCommandsAndStableJSON(t *testing.T) {
	api := &fakeAPI{}
	var out, errOut bytes.Buffer
	code := clientcli.Run(context.Background(), []string{"agents", "list", "--json"}, &out, &errOut, api, false)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
	if out.String() != "{\"ok\":true}\n" {
		t.Fatalf("out=%q", out.String())
	}
	if api.called != "agents list" {
		t.Fatalf("called=%q", api.called)
	}
}
func TestDestructiveRequiresConfirmation(t *testing.T) {
	api := &fakeAPI{}
	var out, errOut bytes.Buffer
	code := clientcli.Run(context.Background(), []string{"reboot", "agent-1"}, &out, &errOut, api, false)
	if code == 0 || api.called != "" {
		t.Fatal("reboot ran without confirmation")
	}
	code = clientcli.Run(context.Background(), []string{"reboot", "agent-1", "--yes"}, &out, &errOut, api, false)
	if code != 0 {
		t.Fatalf("confirmed code=%d", code)
	}
}
func TestNoANSIOnNonTTYAndSecretsRedacted(t *testing.T) {
	api := &fakeAPI{}
	var out, errOut bytes.Buffer
	code := clientcli.Run(context.Background(), []string{"login", "alice", "secret"}, &out, &errOut, api, false)
	if code != 0 {
		t.Fatal(code)
	}
	if strings.Contains(out.String(), "\x1b[") || strings.Contains(out.String(), "secret") {
		t.Fatalf("unsafe output=%q", out.String())
	}
}
func TestCommandCoverage(t *testing.T) {
	for _, command := range []string{"login", "logout", "agents", "stats", "services", "logs", "cleanup", "reboot", "exec", "ssh", "enroll-token", "users", "ssh-keys", "audit", "easypanel", "tui"} {
		if !clientcli.Known(command) {
			t.Errorf("missing %s", command)
		}
	}
}

// TestEasypanelCommandPrintsCapturedOutput verifies the easypanel command
// prints the captured job output (the map's "output" field), not the raw map.
func TestEasypanelCommandPrintsCapturedOutput(t *testing.T) {
	api := &fakeAPI{result: map[string]any{"job_id": "j1", "output": "easypanel: detected\n"}}
	var out, errOut bytes.Buffer
	code := clientcli.Run(context.Background(),
		[]string{"easypanel", "agent-1", "detect", "--yes"}, &out, &errOut, api, false)
	if code != 0 {
		t.Fatalf("exit %d err=%q", code, errOut.String())
	}
	if got := out.String(); got != "easypanel: detected\n" {
		t.Fatalf("unexpected output %q", got)
	}
	if api.command != "easypanel" {
		t.Fatalf("command routed as %q", api.command)
	}
}
