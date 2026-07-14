package clientcli

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	devopsv1 "silent-devops/api/devops/v1"
)

type SSHLaunch struct {
	Command *exec.Cmd
	Cleanup func()
}

func PrepareNativeSSH(ctx context.Context, api API, agentID string) (SSHLaunch, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return SSHLaunch{}, err
	}
	public, err := ssh.NewPublicKey(private.Public())
	if err != nil {
		return SSHLaunch{}, err
	}
	dir, err := os.MkdirTemp("", "silent-devops-ssh-")
	if err != nil {
		return SSHLaunch{}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	keyDER, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		cleanup()
		return SSHLaunch{}, err
	}
	keyPath := filepath.Join(dir, "id_ed25519")
	if err = os.WriteFile(keyPath, pem.EncodeToMemory(keyDER), 0600); err != nil {
		cleanup()
		return SSHLaunch{}, err
	}
	value, err := api.Call(ctx, "ssh", []string{agentID, writePublicKey(dir, ssh.MarshalAuthorizedKey(public))})
	if err != nil {
		cleanup()
		return SSHLaunch{}, err
	}
	session, ok := value.(*devopsv1.SshSession)
	if !ok || session.Id == "" {
		cleanup()
		return SSHLaunch{}, errors.New("invalid SSH session")
	}
	for session.State == devopsv1.SshSessionState_SSH_SESSION_STATE_PREPARING {
		select {
		case <-ctx.Done():
			cleanup()
			return SSHLaunch{}, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		value, err = api.Call(ctx, "ssh", []string{"status", session.Id})
		if err != nil {
			cleanup()
			return SSHLaunch{}, err
		}
		session, ok = value.(*devopsv1.SshSession)
		if !ok {
			cleanup()
			return SSHLaunch{}, errors.New("invalid SSH session")
		}
	}
	if session.State != devopsv1.SshSessionState_SSH_SESSION_STATE_READY || len(session.HostKey) == 0 {
		cleanup()
		return SSHLaunch{}, errors.New("SSH session not ready")
	}
	baseCleanup := cleanup
	cleanup = func() { _, _ = api.Call(context.Background(), "ssh", []string{"close", session.Id}); baseCleanup() }
	known := filepath.Join(dir, "known_hosts")
	// Pin the agent host key under the session id, which is also the ssh target
	// hostname. Transport is a ProxyCommand relaying stdio over gRPC BridgeSsh,
	// so there is no loopback port and no jump host.
	if err = os.WriteFile(known, []byte(fmt.Sprintf("%s %s\n", session.Id, session.HostKey)), 0600); err != nil {
		cleanup()
		return SSHLaunch{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		cleanup()
		return SSHLaunch{}, err
	}
	proxy := fmt.Sprintf("ProxyCommand=%q ssh-pipe %s", exe, session.Id)
	args := []string{"-i", keyPath, "-o", "IdentitiesOnly=yes", "-o", "UserKnownHostsFile=" + known, "-o", "StrictHostKeyChecking=yes", "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no", "-o", "ForwardAgent=no", "-o", "ForwardX11=no", "-o", proxy, "silent-devops@" + session.Id}
	cmd := exec.Command("ssh", args...)
	return SSHLaunch{Command: cmd, Cleanup: cleanup}, nil
}

func writePublicKey(dir string, data []byte) string {
	path := filepath.Join(dir, "id_ed25519.pub")
	_ = os.WriteFile(path, data, 0600)
	return path
}
