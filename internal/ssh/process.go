package ssh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

type TunnelProcess struct {
	cmd  *exec.Cmd
	done chan error
}

func StartTunnel(ctx context.Context, binary string, args []string) (*TunnelProcess, error) {
	if binary == "" {
		return nil, errors.New("SSH binary required")
	}
	cmd := exec.Command(binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &TunnelProcess{cmd: cmd, done: make(chan error, 1)}
	go func() { p.done <- cmd.Wait(); close(p.done) }()
	go func() {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		case <-p.done:
		}
	}()
	return p, nil
}
func (p *TunnelProcess) Done() <-chan error { return p.done }
func (p *TunnelProcess) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
}
func ClientArgs(dir string, port uint32, user string, hostKey []byte) ([]string, func(), error) {
	if dir == "" || port == 0 || user == "" || len(hostKey) == 0 {
		return nil, nil, errors.New("complete SSH client configuration required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	file, err := os.CreateTemp(dir, "known-hosts-*")
	if err != nil {
		return nil, nil, err
	}
	path := file.Name()
	cleanup := func() { file.Close(); os.Remove(path) }
	if err := file.Chmod(0600); err != nil {
		cleanup()
		return nil, nil, err
	}
	line := fmt.Sprintf("[127.0.0.1]:%d %s\n", port, hostKey)
	if _, err := file.WriteString(line); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return nil, nil, err
	}
	args := []string{"-o", "Port=" + strconv.FormatUint(uint64(port), 10), "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + filepath.Clean(path), "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no", "-o", "ForwardAgent=no", "-o", "ForwardX11=no", user + "@127.0.0.1"}
	return args, cleanup, nil
}
