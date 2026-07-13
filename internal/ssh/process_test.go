package ssh_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"silent-devops/internal/ssh"
)

func TestTunnelProcessStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p, err := ssh.StartTunnel(ctx, "/bin/sh", []string{"-c", "sleep 30"})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-p.Done():
		if err == nil {
			t.Fatal("killed process returned nil")
		}
	case <-time.After(time.Second):
		t.Fatal("process did not stop")
	}
}
func TestClientArgsUseHostKeyAndLoopback(t *testing.T) {
	dir := t.TempDir()
	args, cleanup, err := ssh.ClientArgs(dir, 22000, "root", []byte("ssh-ed25519 AAAAhostkey"))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	joined := strings.Join(args, " ")
	for _, want := range []string{"127.0.0.1", "Port=22000", "StrictHostKeyChecking=yes", "UserKnownHostsFile="} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("files=%v", entries)
	}
	info, _ := os.Stat(filepath.Join(dir, entries[0].Name()))
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}
