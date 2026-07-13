package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
)

type Persistence struct{ db *sql.DB }

func NewPersistence(db *sql.DB) *Persistence { return &Persistence{db: db} }
func (p *Persistence) Online(ctx context.Context, agentID, streamID string, hello *devopsv1.AgentHello, now time.Time) error {
	metadata, err := json.Marshal(hello)
	if err != nil {
		return err
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE agents SET hostname=?,boot_id=?,metadata=? WHERE id=?", hello.Hostname, hello.BootId, metadata, agentID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO connections(agent_id,connected_unix_ms,stream_id,state) VALUES(?,?,?,'online') ON CONFLICT(agent_id) DO UPDATE SET connected_unix_ms=excluded.connected_unix_ms,disconnected_unix_ms=NULL,stream_id=excluded.stream_id,state='online'", agentID, now.UnixMilli(), streamID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (p *Persistence) Offline(ctx context.Context, agentID, streamID string, now time.Time) error {
	_, err := p.db.ExecContext(ctx, "UPDATE connections SET disconnected_unix_ms=?,state='offline' WHERE agent_id=? AND stream_id=?", now.UnixMilli(), agentID, streamID)
	return err
}
func (p *Persistence) MarkAllOffline(ctx context.Context, now time.Time) error {
	_, err := p.db.ExecContext(ctx, "UPDATE connections SET disconnected_unix_ms=?,state='offline' WHERE state='online'", now.UnixMilli())
	return err
}
