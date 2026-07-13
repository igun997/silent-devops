package registry_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"silent-devops/internal/registry"
	"silent-devops/internal/store"
)

func TestPersistentConnectionLifecycleAndRestartRecovery(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB().Exec("INSERT INTO agents(id,hostname,created_unix_ms) VALUES('a','host',?)", time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	p := registry.NewPersistence(s.DB())
	now := time.Now()
	if err := p.Online(context.Background(), "a", "stream-1", hello("a"), now); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := s.DB().QueryRow("SELECT state FROM connections WHERE agent_id='a'").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "online" {
		t.Fatalf("state=%q", state)
	}
	if err := p.MarkAllOffline(context.Background(), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().QueryRow("SELECT state FROM connections WHERE agent_id='a'").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "offline" {
		t.Fatalf("state=%q", state)
	}
}
