package clientcli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"silent-devops/internal/clientcli"
)

type fakeAPI struct{ called string }

func (f *fakeAPI) Call(_ context.Context, command string, args []string) (any, error) {
	f.called = command + " " + strings.Join(args, " ")
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
	for _, command := range []string{"login", "logout", "agents", "stats", "services", "logs", "cleanup", "reboot", "exec", "ssh", "enroll-token", "users", "ssh-keys", "audit", "tui"} {
		if !clientcli.Known(command) {
			t.Errorf("missing %s", command)
		}
	}
}
