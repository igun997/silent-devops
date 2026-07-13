package pki

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/argon2"
)

type IdentityRegistry struct{ db *sql.DB }

func NewIdentityRegistry(db *sql.DB) *IdentityRegistry { return &IdentityRegistry{db: db} }
func (r *IdentityRegistry) SetMetadata(ctx context.Context, id, hostname string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE agents SET hostname=? WHERE id=?", hostname, id)
	return err
}
func (r *IdentityRegistry) Register(ctx context.Context, id, serial string, expires time.Time) error {
	if id == "" || serial == "" {
		return errors.New("agent identity and serial required")
	}
	_, err := r.db.ExecContext(ctx, "INSERT INTO agents(id,certificate_serial,certificate_expires_unix_ms,created_unix_ms) VALUES(?,?,?,?)", id, serial, expires.UnixMilli(), time.Now().UnixMilli())
	return err
}
func (r *IdentityRegistry) AuthorizeRenewal(ctx context.Context, id string, now time.Time) error {
	var revoked sql.NullInt64
	var expires int64
	err := r.db.QueryRowContext(ctx, "SELECT revoked_unix_ms,certificate_expires_unix_ms FROM agents WHERE id=?", id).Scan(&revoked, &expires)
	if err != nil {
		return err
	}
	if revoked.Valid {
		return errors.New("agent revoked")
	}
	if expires < now.UnixMilli() {
		return errors.New("agent certificate expired")
	}
	return nil
}
func (r *IdentityRegistry) UpdateCertificate(ctx context.Context, id, serial string, expires time.Time) error {
	result, err := r.db.ExecContext(ctx, "UPDATE agents SET certificate_serial=?,certificate_expires_unix_ms=? WHERE id=? AND revoked_unix_ms IS NULL", serial, expires.UnixMilli(), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("agent not active")
	}
	return nil
}
func (r *IdentityRegistry) Revoke(ctx context.Context, id string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, "UPDATE agents SET revoked_unix_ms=? WHERE id=? AND revoked_unix_ms IS NULL", now.UnixMilli(), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("agent not active")
	}
	return nil
}
func (r *IdentityRegistry) AuthorizeConnection(ctx context.Context, id, serial string, now time.Time) error {
	var stored string
	var expires int64
	var revoked sql.NullInt64
	err := r.db.QueryRowContext(ctx, "SELECT certificate_serial,certificate_expires_unix_ms,revoked_unix_ms FROM agents WHERE id=?", id).Scan(&stored, &expires, &revoked)
	if err != nil {
		return err
	}
	if revoked.Valid || stored != serial || expires < now.UnixMilli() {
		return errors.New("agent identity unauthorized")
	}
	return nil
}

func BootstrapAdmin(ctx context.Context, db *sql.DB, username, password string, now time.Time) error {
	if username == "" || len(password) < 16 {
		return errors.New("bootstrap admin requires username and password of at least 16 bytes")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("bootstrap already completed")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	encoded := fmt.Sprintf("argon2id$v=19$m=65536,t=3,p=2$%s$%s", hex.EncodeToString(salt), hex.EncodeToString(hash))
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES(?,?,?,?,?)", hex.EncodeToString(id), username, []byte(encoded), 3, now.UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}
