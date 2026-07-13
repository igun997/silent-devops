package clientcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type CredentialStore struct{ Path string }

func (s CredentialStore) Save(token string) error {
	if s.Path == "" || token == "" || strings.ContainsAny(token, "\r\n") {
		return errors.New("valid credential path and token required")
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".token-*")
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
	if _, err := tmp.WriteString(token); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.Path); err != nil {
		return err
	}
	ok = true
	return nil
}
func (s CredentialStore) Load() (string, error) {
	info, err := os.Stat(s.Path)
	if err != nil {
		return "", err
	}
	if info.Mode().Perm()&0077 != 0 {
		return "", errors.New("credential file permissions too broad")
	}
	data, err := os.ReadFile(s.Path)
	return string(data), err
}
func (s CredentialStore) Clear() error {
	err := os.Remove(s.Path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func Validate(args []string) error {
	if len(args) == 0 || !Known(args[0]) {
		return errors.New("unknown command")
	}
	need := func(n int) error {
		if len(args) != n {
			return errors.New("invalid arguments")
		}
		return nil
	}
	switch args[0] {
	case "login":
		return need(3)
	case "logout", "audit", "tui":
		return need(1)
	case "stats":
		return need(2)
	case "enroll-token":
		if len(args) == 1 {
			return nil
		}
		if len(args) == 2 && (args[1] == "create" || args[1] == "list") {
			return nil
		}
		if len(args) == 3 && args[1] == "revoke" {
			return nil
		}
		return errors.New("usage: enroll-token create|list|revoke ID")
	case "agents":
		if len(args) == 2 && args[1] == "list" {
			return nil
		}
		if len(args) == 3 && args[1] == "show" {
			return nil
		}
		return errors.New("usage: agents list | agents show AGENT_ID")
	case "services":
		if len(args) < 2 {
			return errors.New("service action required")
		}
		switch args[1] {
		case "list":
			return need(3)
		case "status", "start", "stop", "restart":
			return need(4)
		default:
			return errors.New("invalid service action")
		}
	case "logs":
		return need(3)
	case "cleanup":
		if len(args) < 2 {
			return errors.New("cleanup action required")
		}
		if args[1] == "preview" {
			if len(args) < 4 {
				return errors.New("cleanup paths required")
			}
			return nil
		}
		if args[1] == "run" {
			return need(4)
		}
		return errors.New("invalid cleanup action")
	case "reboot":
		return need(2)
	case "exec":
		if len(args) < 3 {
			return errors.New("target and command required")
		}
	case "ssh":
		return need(3)
	case "users":
		if len(args) == 1 || len(args) == 2 && args[1] == "list" {
			return nil
		}
		return errors.New("usage: users list")
	case "ssh-keys":
		if len(args) == 1 || len(args) >= 2 {
			return nil
		}
		return errors.New("invalid ssh-keys action")
	}
	return nil
}
