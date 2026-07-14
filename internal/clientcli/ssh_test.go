package clientcli

import (
	"context"
	"os"
	"strings"
	"testing"

	devopsv1 "silent-devops/api/devops/v1"
)

type sshAPI struct{ session *devopsv1.SshSession }

func (a sshAPI) Call(_ context.Context, command string, args []string) (any, error) {
	if command == "ssh" && len(args) == 2 && args[0] == "close" {
		return a.session, nil
	}
	return a.session, nil
}

func readKnownHosts(t *testing.T, args []string) string {
	t.Helper()
	for _, a := range args {
		if strings.HasPrefix(a, "UserKnownHostsFile=") {
			data, err := os.ReadFile(strings.TrimPrefix(a, "UserKnownHostsFile="))
			if err != nil {
				t.Fatal(err)
			}
			return string(data)
		}
	}
	t.Fatal("no UserKnownHostsFile arg")
	return ""
}

func TestPrepareNativeSSHUsesProxyCommandAndPinsAgentHostKey(t *testing.T) {
	session := &devopsv1.SshSession{
		Id:      "sess",
		AgentId: "agent",
		State:   devopsv1.SshSessionState_SSH_SESSION_STATE_READY,
		HostKey: []byte("ssh-ed25519 AAAAtarget agent"),
	}
	launch, err := PrepareNativeSSH(context.Background(), sshAPI{session: session}, "agent")
	if err != nil {
		t.Fatal(err)
	}
	defer launch.Cleanup()
	args := launch.Command.Args
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "ProxyCommand=") || !strings.Contains(joined, "ssh-pipe sess") {
		t.Fatalf("missing ProxyCommand: %q", joined)
	}
	if !strings.Contains(joined, "silent-devops@sess") {
		t.Fatalf("missing target: %q", joined)
	}
	if strings.Contains(joined, "-J ") || strings.Contains(joined, "127.0.0.1") {
		t.Fatalf("unexpected jump/loopback: %q", joined)
	}
	for _, want := range []string{"StrictHostKeyChecking=yes", "IdentitiesOnly=yes", "PasswordAuthentication=no"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	known := readKnownHosts(t, args)
	if !strings.Contains(known, "sess ssh-ed25519 AAAAtarget agent") {
		t.Fatalf("agent host key not pinned under session id: %q", known)
	}
}

func TestPrepareNativeSSHRejectsSessionWithoutHostKey(t *testing.T) {
	session := &devopsv1.SshSession{
		Id:      "sess",
		AgentId: "agent",
		State:   devopsv1.SshSessionState_SSH_SESSION_STATE_READY,
	}
	if _, err := PrepareNativeSSH(context.Background(), sshAPI{session: session}, "agent"); err == nil {
		t.Fatal("expected error for missing host key")
	}
}
