package ssh

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
)

type Manager struct {
	db       *sql.DB
	min, max uint32
	now      func() time.Time
	mu       sync.Mutex
	tokens   map[string][]byte
}

func NewManager(db *sql.DB, min, max uint32, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{db: db, min: min, max: max, now: now, tokens: make(map[string][]byte)}
}
func (m *Manager) Create(ctx context.Context, actor, agent string, key []byte, reason string, ttl time.Duration) (*devopsv1.SshSession, *devopsv1.PrepareSsh, error) {
	if actor == "" || agent == "" || len(key) == 0 || reason == "" || ttl <= 0 || ttl > time.Hour {
		return nil, nil, errors.New("invalid SSH session request")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	port, err := m.allocatePort(ctx, tx, now)
	if err != nil {
		return nil, nil, err
	}
	id, err := random(16)
	if err != nil {
		return nil, nil, err
	}
	token, err := randomBytes(32)
	if err != nil {
		return nil, nil, err
	}
	expires := now.Add(ttl)
	if _, err := tx.ExecContext(ctx, "INSERT INTO ssh_sessions(id,actor_id,agent_id,public_key,state,loopback_port,expires_unix_ms,created_unix_ms) VALUES(?,?,?,?,?,?,?,?)", id, actor, agent, key, devopsv1.SshSessionState_SSH_SESSION_STATE_PREPARING, port, expires.UnixMilli(), now.UnixMilli()); err != nil {
		return nil, nil, err
	}
	metadata, _ := json.Marshal(map[string]any{"session_id": id, "port": port, "expires_unix_ms": expires.UnixMilli()})
	auditID, err := random(16)
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit_events(id,actor_id,agent_id,action,reason,occurred_unix_ms,metadata) VALUES(?,?,?,?,?,?,?)", auditID, actor, agent, "ssh_session", reason, now.UnixMilli(), metadata); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	m.tokens[id] = token
	session := &devopsv1.SshSession{Id: id, AgentId: agent, State: devopsv1.SshSessionState_SSH_SESSION_STATE_PREPARING, ValidatorLoopbackPort: port, ExpiresUnixMs: expires.UnixMilli()}
	return session, &devopsv1.PrepareSsh{SessionId: id, PublicKey: key, ExpiresUnixMs: expires.UnixMilli(), LoopbackPort: port, BindingToken: token}, nil
}
func (m *Manager) allocatePort(ctx context.Context, tx *sql.Tx, now time.Time) (uint32, error) {
	for port := m.min; port <= m.max; port++ {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM ssh_sessions WHERE loopback_port=? AND state IN (1,2) AND expires_unix_ms>?", port, now.UnixMilli()).Scan(&count); err != nil {
			return 0, err
		}
		if count == 0 {
			return port, nil
		}
		if port == ^uint32(0) {
			break
		}
	}
	return 0, errors.New("no SSH loopback ports available")
}
func (m *Manager) Ready(ctx context.Context, id string, port uint32, token, hostKey []byte) (*devopsv1.SshSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want, ok := m.tokens[id]
	if !ok || !equal(want, token) {
		return nil, errors.New("SSH session binding mismatch")
	}
	result, err := m.db.ExecContext(ctx, "UPDATE ssh_sessions SET state=2 WHERE id=? AND loopback_port=? AND state=1 AND expires_unix_ms>?", id, port, m.now().UnixMilli())
	if err != nil {
		return nil, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return nil, errors.New("SSH session not ready")
	}
	delete(m.tokens, id)
	return m.get(ctx, id, hostKey)
}
func (m *Manager) Close(ctx context.Context, id, reason string) (*devopsv1.SshSession, error) {
	if reason == "" {
		return nil, errors.New("close reason required")
	}
	m.mu.Lock()
	delete(m.tokens, id)
	m.mu.Unlock()
	if _, err := m.db.ExecContext(ctx, "UPDATE ssh_sessions SET state=3 WHERE id=? AND state IN (1,2)", id); err != nil {
		return nil, err
	}
	return m.get(ctx, id, nil)
}
func (m *Manager) Reconcile(ctx context.Context, now time.Time) error {
	m.mu.Lock()
	m.tokens = make(map[string][]byte)
	m.mu.Unlock()
	_, err := m.db.ExecContext(ctx, "UPDATE ssh_sessions SET state=4 WHERE state IN (1,2) AND expires_unix_ms<=?", now.UnixMilli())
	return err
}
func (m *Manager) get(ctx context.Context, id string, hostKey []byte) (*devopsv1.SshSession, error) {
	s := &devopsv1.SshSession{HostKey: hostKey}
	var state int32
	if err := m.db.QueryRowContext(ctx, "SELECT id,agent_id,state,loopback_port,expires_unix_ms FROM ssh_sessions WHERE id=?", id).Scan(&s.Id, &s.AgentId, &state, &s.ValidatorLoopbackPort, &s.ExpiresUnixMs); err != nil {
		return nil, err
	}
	s.State = devopsv1.SshSessionState(state)
	return s, nil
}
func random(n int) (string, error)      { b, err := randomBytes(n); return hex.EncodeToString(b), err }
func randomBytes(n int) ([]byte, error) { b := make([]byte, n); _, err := rand.Read(b); return b, err }
func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
