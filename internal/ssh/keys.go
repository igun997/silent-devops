package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type KeyStore struct{ Path string }

func (k KeyStore) Install(sessionID string, publicKey []byte, expires time.Time) error {
	if sessionID == "" || expires.IsZero() {
		return errors.New("session and expiry required")
	}
	fields := strings.Fields(string(publicKey))
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "ssh-") {
		return errors.New("invalid SSH public key")
	}
	line := fmt.Sprintf("restrict,port-forwarding,command=\"/bin/false\" %s %s silent-devops:%s:%d\n", fields[0], fields[1], sessionID, expires.Unix())
	return k.update(func(lines []string) []string {
		return append(removeSession(lines, sessionID), strings.TrimSuffix(line, "\n"))
	})
}
func (k KeyStore) Remove(sessionID string) error {
	return k.update(func(lines []string) []string { return removeSession(lines, sessionID) })
}
func (k KeyStore) Reconcile(now time.Time) error {
	return k.update(func(lines []string) []string {
		out := lines[:0]
		for _, line := range lines {
			marker := marker(line)
			if marker == "" {
				out = append(out, line)
				continue
			}
			parts := strings.Split(marker, ":")
			if len(parts) != 3 {
				continue
			}
			expiry, err := strconv.ParseInt(parts[2], 10, 64)
			if err == nil && expiry > now.Unix() {
				out = append(out, line)
			}
		}
		return out
	})
}
func (k KeyStore) update(change func([]string) []string) error {
	if k.Path == "" {
		return errors.New("authorized keys path required")
	}
	var lines []string
	file, err := os.Open(k.Path)
	if err == nil {
		s := bufio.NewScanner(file)
		for s.Scan() {
			lines = append(lines, s.Text())
		}
		file.Close()
		if err := s.Err(); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	lines = change(lines)
	if err := os.MkdirAll(filepath.Dir(k.Path), 0700); err != nil {
		return err
	}
	data := []byte{}
	if len(lines) > 0 {
		data = []byte(strings.Join(lines, "\n") + "\n")
	}
	return atomicWrite(k.Path, data)
}
func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".authorized-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
func removeSession(lines []string, id string) []string {
	out := lines[:0]
	needle := "silent-devops:" + id + ":"
	for _, line := range lines {
		if !strings.Contains(line, needle) {
			out = append(out, line)
		}
	}
	return out
}
func marker(line string) string {
	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, "silent-devops:") {
			return field
		}
	}
	return ""
}
func ReverseTunnelArgs(host string, port uint32, user, key, knownHosts string) ([]string, error) {
	if host == "" || port == 0 || user == "" || key == "" || knownHosts == "" {
		return nil, errors.New("complete SSH tunnel configuration required")
	}
	remote := fmt.Sprintf("127.0.0.1:%d:127.0.0.1:22", port)
	return []string{"-N", "-T", "-i", key, "-o", "BatchMode=yes", "-o", "ExitOnForwardFailure=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + knownHosts, "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no", "-o", "RequestTTY=no", "-o", "ForwardAgent=no", "-o", "ForwardX11=no", "-o", "ClearAllForwardings=yes", "-R", remote, user + "@" + host}, nil
}
