package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"silent-devops/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "validator.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAppliesMigrationsAndPragmas(t *testing.T) {
	s := openStore(t)
	for pragma, want := range map[string]int{"foreign_keys": 1, "busy_timeout": 5000} {
		var got int
		if err := s.DB().QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("PRAGMA %s = %d, want %d", pragma, got, want)
		}
	}
	var mode string
	if err := s.DB().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
	var count int
	if err := s.DB().QueryRow("SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no migrations applied")
	}
}

func TestSchemaRejectsPlaintextSecretsAndDuplicateTokens(t *testing.T) {
	s := openStore(t)
	columns, err := s.DB().Query("PRAGMA table_info(users)")
	if err != nil {
		t.Fatal(err)
	}
	defer columns.Close()
	for columns.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue sql.NullString
		if err := columns.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "password" {
			t.Fatal("plaintext password column exists")
		}
	}
	expires := time.Now().Add(time.Hour).UnixMilli()
	if _, err := s.DB().Exec("INSERT INTO enrollment_tokens(id, token_hash, expires_unix_ms) VALUES('1', ?, ?)", []byte("hash"), expires); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec("INSERT INTO enrollment_tokens(id, token_hash, expires_unix_ms) VALUES('2', ?, ?)", []byte("hash"), expires); err == nil {
		t.Fatal("duplicate token hash accepted")
	}
}

func TestRestartAndRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "validator.db")
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
	if _, err := s.DB().Exec("INSERT INTO metrics_minute(agent_id, bucket_unix_ms, payload) VALUES('a', ?, '{}')", old); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Cleanup(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB().QueryRow("SELECT count(*) FROM metrics_minute").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("old metric count = %d", count)
	}
	if err := s.IntegrityCheck(context.Background()); err != nil {
		t.Fatal(err)
	}
}
