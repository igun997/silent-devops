package pki_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"silent-devops/internal/pki"
	"silent-devops/internal/store"
)

func TestRegisterRenewAndRevokeAgent(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	registry := pki.NewIdentityRegistry(s.DB())
	now := time.Now()
	if err := registry.Register(context.Background(), "agent-1", "serial-1", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := registry.AuthorizeRenewal(context.Background(), "agent-1", now); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateCertificate(context.Background(), "agent-1", "serial-2", now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := registry.Revoke(context.Background(), "agent-1", now); err != nil {
		t.Fatal(err)
	}
	if err := registry.AuthorizeRenewal(context.Background(), "agent-1", now); err == nil {
		t.Fatal("revoked agent renewed")
	}
	if err := registry.AuthorizeConnection(context.Background(), "agent-1", "serial-2", now); err == nil {
		t.Fatal("revoked agent connected")
	}
}

func TestBootstrapAdminOnlyOnceConcurrent(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if pki.BootstrapAdmin(context.Background(), s.DB(), "admin", "correct horse battery staple", time.Now()) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("bootstrap successes=%d", successes.Load())
	}
	var count int
	if err := s.DB().QueryRow("SELECT count(*) FROM users WHERE role=3").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("admin count=%d", count)
	}
}
