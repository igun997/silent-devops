package maintenance

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func ValidateUnit(unit string) error {
	if unit == "" || len(unit) > 256 || strings.ContainsAny(unit, "/;|&`$\\\n\r\t ") {
		return errors.New("invalid systemd unit")
	}
	for _, r := range unit {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.@:-", r)) {
			return errors.New("invalid systemd unit")
		}
	}
	return nil
}
func ValidatePath(path string, allowed []string) (string, error) {
	clean, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	clean = filepath.Clean(clean)
	for _, root := range allowed {
		root, err = filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(filepath.Clean(root), clean)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return clean, nil
		}
	}
	return "", errors.New("path outside allowlist")
}

type Result struct {
	Output    []byte
	ExitCode  int
	Truncated bool
	Err       error
}
type Runner struct{ MaxOutputBytes int }
type limitedBuffer struct {
	mu        sync.Mutex
	data      []byte
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(p)
	remaining := b.max - len(b.data)
	if remaining <= 0 {
		b.truncated = true
		return original, nil
	}
	if len(p) > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	b.data = append(b.data, p...)
	return original, nil
}

func (b *limitedBuffer) snapshot() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...), b.truncated
}
func (r Runner) Run(ctx context.Context, timeout time.Duration, name string, args ...string) Result {
	if r.MaxOutputBytes <= 0 {
		return Result{Err: errors.New("output limit required")}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output := &limitedBuffer{max: r.MaxOutputBytes}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return Result{Err: err}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		code := 0
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		data, truncated := output.snapshot()
		return Result{Output: data, ExitCode: code, Truncated: truncated, Err: err}
	case <-runCtx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		data, truncated := output.snapshot()
		return Result{Output: data, ExitCode: -1, Truncated: truncated, Err: runCtx.Err()}
	}
}

type Preview struct {
	ID      string
	Hash    []byte
	Expires time.Time
	paths   []string
}
type CleanupManager struct {
	mu       sync.Mutex
	allowed  []string
	ttl      time.Duration
	previews map[string]Preview
}

func NewCleanupManager(allowed []string, ttl time.Duration) *CleanupManager {
	return &CleanupManager{allowed: allowed, ttl: ttl, previews: make(map[string]Preview)}
}
func (m *CleanupManager) Preview(paths []string, now time.Time) (Preview, error) {
	if len(paths) == 0 {
		return Preview{}, errors.New("cleanup paths required")
	}
	clean := make([]string, 0, len(paths))
	for _, path := range paths {
		validated, err := ValidatePath(path, m.allowed)
		if err != nil {
			return Preview{}, err
		}
		clean = append(clean, validated)
	}
	idRaw := make([]byte, 16)
	rand.Read(idRaw)
	id := hex.EncodeToString(idRaw)
	hash := sha256.Sum256([]byte(strings.Join(clean, "\x00")))
	preview := Preview{ID: id, Hash: hash[:], Expires: now.Add(m.ttl), paths: clean}
	m.mu.Lock()
	m.previews[id] = preview
	m.mu.Unlock()
	return preview, nil
}
func (m *CleanupManager) Execute(id string, hash []byte, now time.Time) error {
	m.mu.Lock()
	preview, ok := m.previews[id]
	if ok {
		delete(m.previews, id)
	}
	m.mu.Unlock()
	if !ok || now.After(preview.Expires) || !bytes.Equal(hash, preview.Hash) {
		return errors.New("invalid cleanup preview")
	}
	for _, path := range preview.paths {
		if _, err := ValidatePath(path, m.allowed); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}
