package ssh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"silent-devops/internal/ssh"
)

func TestTemporaryAuthorizedKeyInstallRemoveAndReconcile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authorized_keys")
	k := ssh.KeyStore{Path: path}
	expires := time.Now().Add(time.Minute)
	if err := k.Install("session-1", []byte("ssh-ed25519 AAA user@host"), expires); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, "restrict,port-forwarding") || strings.Contains(text, "user@host") {
		t.Fatalf("key line=%q", text)
	}
	if err := k.Remove("session-1"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if len(data) != 0 {
		t.Fatalf("key remains=%q", data)
	}
	k.Install("expired", []byte("ssh-ed25519 BBB"), time.Now().Add(-time.Second))
	k.Install("live", []byte("ssh-ed25519 CCC"), time.Now().Add(time.Minute))
	if err := k.Reconcile(time.Now()); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "expired") || !strings.Contains(string(data), "live") {
		t.Fatalf("reconciled=%q", data)
	}
}
func TestReverseTunnelArgvHardened(t *testing.T) {
	args, err := ssh.ReverseTunnelArgs("validator.example", 22000, "agent", "/keys/tunnel", "known_hosts")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"ExitOnForwardFailure=yes", "StrictHostKeyChecking=yes", "PasswordAuthentication=no", "RequestTTY=no", "-R 127.0.0.1:22000:127.0.0.1:22", "-N"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}
