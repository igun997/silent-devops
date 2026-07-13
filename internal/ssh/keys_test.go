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
	if !strings.Contains(text, "restrict,pty") || strings.Contains(text, "user@host") {
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
