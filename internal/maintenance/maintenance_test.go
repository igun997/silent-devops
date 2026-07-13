package maintenance_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"silent-devops/internal/maintenance"
)

func TestUnitAndPathValidation(t *testing.T) {
	for _, unit := range []string{"sshd.service", "nginx@blue.service"} {
		if err := maintenance.ValidateUnit(unit); err != nil {
			t.Fatal(err)
		}
	}
	for _, unit := range []string{"", "../x.service", "x;reboot", "x.service/evil"} {
		if maintenance.ValidateUnit(unit) == nil {
			t.Fatalf("accepted %q", unit)
		}
	}
	root := t.TempDir()
	allowed := filepath.Join(root, "cache")
	os.MkdirAll(allowed, 0755)
	if _, err := maintenance.ValidatePath(filepath.Join(allowed, "file"), []string{allowed}); err != nil {
		t.Fatal(err)
	}
	if _, err := maintenance.ValidatePath(filepath.Join(root, "other"), []string{allowed}); err == nil {
		t.Fatal("outside path accepted")
	}
}
func TestRunArgvTimeoutAndOutputBound(t *testing.T) {
	r := maintenance.Runner{MaxOutputBytes: 4}
	result := r.Run(context.Background(), time.Second, "/bin/sh", "-c", "printf 123456")
	if string(result.Output) != "1234" || !result.Truncated {
		t.Fatalf("result=%+v", result)
	}
	result = r.Run(context.Background(), 10*time.Millisecond, "/bin/sh", "-c", "sleep 1")
	if !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", result.Err)
	}
}
func TestCleanupPreviewBinding(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "old")
	os.WriteFile(file, []byte("x"), 0600)
	manager := maintenance.NewCleanupManager([]string{dir}, time.Minute)
	preview, err := manager.Preview([]string{file}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Execute(preview.ID, preview.Hash, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("file not removed")
	}
	if err := manager.Execute(preview.ID, preview.Hash, time.Now()); err == nil {
		t.Fatal("preview reused")
	}
}
