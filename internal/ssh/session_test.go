package ssh_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"silent-devops/internal/ssh"
	"silent-devops/internal/store"
)

func TestCreateReadyCloseBindingAndAudit(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	s.DB().Exec("INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES('u','admin','x',3,?)", now.UnixMilli())
	s.DB().Exec("INSERT INTO agents(id,created_unix_ms) VALUES('a',?)", now.UnixMilli())
	m := ssh.NewManager(s.DB(), 22000, 22001, func() time.Time { return now })
	session, prepare, err := m.Create(context.Background(), "u", "a", []byte("ssh-ed25519 AAA"), "incident", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if session.ValidatorLoopbackPort < 22000 || session.ValidatorLoopbackPort > 22001 || len(prepare.BindingToken) != 32 {
		t.Fatalf("session=%+v prepare=%+v", session, prepare)
	}
	if _, err := m.Ready(context.Background(), "wrong", prepare.LoopbackPort, prepare.BindingToken, []byte("host")); err == nil {
		t.Fatal("wrong session accepted")
	}
	ready, err := m.Ready(context.Background(), session.Id, prepare.LoopbackPort, prepare.BindingToken, []byte("host"))
	if err != nil {
		t.Fatal(err)
	}
	if ready.State.String() != "SSH_SESSION_STATE_READY" {
		t.Fatalf("state=%v", ready.State)
	}
	fetched, err := m.Get(context.Background(), session.Id)
	if err != nil {
		t.Fatal(err)
	}
	if string(fetched.HostKey) != "host" {
		t.Fatalf("persisted host key=%q", fetched.HostKey)
	}
	if _, err := m.Close(context.Background(), session.Id, "done"); err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB().QueryRow("SELECT count(*) FROM audit_events WHERE action='ssh_session'").Scan(&count)
	if count != 1 {
		t.Fatalf("audit count=%d", count)
	}
}
func TestClosedSessionReleasesPort(t *testing.T) {
	s, _ := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	defer s.Close()
	now := time.Now()
	s.DB().Exec("INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES('u','admin','x',3,?)", now.UnixMilli())
	s.DB().Exec("INSERT INTO agents(id,created_unix_ms) VALUES('a',?)", now.UnixMilli())
	m := ssh.NewManager(s.DB(), 24000, 24000, func() time.Time { return now })
	first, _, err := m.Create(context.Background(), "u", "a", []byte("ssh-ed25519 AAA"), "one", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Close(context.Background(), first.Id, "done"); err != nil {
		t.Fatal(err)
	}
	second, _, err := m.Create(context.Background(), "u", "a", []byte("ssh-ed25519 BBB"), "two", time.Minute)
	if err != nil {
		t.Fatalf("reuse after close: %v", err)
	}
	if second.ValidatorLoopbackPort != 24000 {
		t.Fatalf("port=%d", second.ValidatorLoopbackPort)
	}
}
func TestExpireAndRestartReconcile(t *testing.T) {
	s, _ := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	defer s.Close()
	now := time.Now()
	s.DB().Exec("INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES('u','admin','x',3,?)", now.UnixMilli())
	s.DB().Exec("INSERT INTO agents(id,created_unix_ms) VALUES('a',?)", now.UnixMilli())
	m := ssh.NewManager(s.DB(), 23000, 23000, func() time.Time { return now })
	session, _, _ := m.Create(context.Background(), "u", "a", []byte("key"), "reason", time.Second)
	if err := m.Reconcile(context.Background(), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	var state int
	s.DB().QueryRow("SELECT state FROM ssh_sessions WHERE id=?", session.Id).Scan(&state)
	if state != 4 {
		t.Fatalf("state=%d", state)
	}
}
