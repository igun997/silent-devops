package server

import (
	"context"
	"database/sql"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"silent-devops/internal/metrics"
)

func (s Fleet) GetMetrics(ctx context.Context, r *devopsv1.GetMetricsRequest) (*devopsv1.GetMetricsResponse, error) {
	if r.GetAgentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "agent ID required")
	}
	snapshot, err := metrics.NewRepository(s.DB).Current(ctx, r.AgentId)
	if err == sql.ErrNoRows {
		return &devopsv1.GetMetricsResponse{}, nil
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "metrics unavailable")
	}
	return &devopsv1.GetMetricsResponse{Snapshots: []*devopsv1.MetricsSnapshot{snapshot}}, nil
}
func (s Fleet) ListAudit(ctx context.Context, r *devopsv1.ListAuditRequest) (*devopsv1.ListAuditResponse, error) {
	query := "SELECT id,actor_id,COALESCE(agent_id,''),action,reason,occurred_unix_ms FROM audit_events"
	args := []any{}
	if r.AgentId != "" {
		query += " WHERE agent_id=?"
		args = append(args, r.AgentId)
	}
	query += " ORDER BY occurred_unix_ms DESC LIMIT ?"
	args = append(args, pageSize(r.PageSize))
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Error(codes.Internal, "audit unavailable")
	}
	defer rows.Close()
	out := &devopsv1.ListAuditResponse{}
	for rows.Next() {
		event := &devopsv1.AuditEvent{}
		if err := rows.Scan(&event.Id, &event.ActorId, &event.AgentId, &event.Action, &event.Reason, &event.OccurredUnixMs); err != nil {
			return nil, status.Error(codes.Internal, "audit unavailable")
		}
		out.Events = append(out.Events, event)
	}
	return out, rows.Err()
}
func (s Fleet) SetUserRole(ctx context.Context, r *devopsv1.SetUserRoleRequest) (*devopsv1.User, error) {
	if r.UserId == "" || r.Role < devopsv1.Role_ROLE_VIEWER || r.Role > devopsv1.Role_ROLE_ADMIN {
		return nil, status.Error(codes.InvalidArgument, "user and valid role required")
	}
	result, err := s.DB.ExecContext(ctx, "UPDATE users SET role=? WHERE id=?", r.Role, r.UserId)
	if err != nil {
		return nil, status.Error(codes.Internal, "update failed")
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	var user devopsv1.User
	if err := s.DB.QueryRowContext(ctx, "SELECT id,username,role,disabled FROM users WHERE id=?", r.UserId).Scan(&user.Id, &user.Username, &user.Role, &user.Disabled); err != nil {
		return nil, status.Error(codes.Internal, "read failed")
	}
	return &user, nil
}
func (s Fleet) AddSshKey(ctx context.Context, r *devopsv1.AddSshKeyRequest) (*devopsv1.SshKey, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if len(r.PublicKey) == 0 || len(r.PublicKey) > 16384 || r.Label == "" || len(r.Label) > 200 {
		return nil, status.Error(codes.InvalidArgument, "public key and label required")
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	if _, err := s.DB.ExecContext(ctx, "INSERT INTO ssh_keys(id,user_id,public_key,label,created_unix_ms) VALUES(?,?,?,?,?)", id, claims.Subject, r.PublicKey, r.Label, s.now().UnixMilli()); err != nil {
		return nil, status.Error(codes.AlreadyExists, "key exists")
	}
	return &devopsv1.SshKey{Id: id, UserId: claims.Subject, PublicKey: r.PublicKey, Label: r.Label}, nil
}
func (s Fleet) ListSshKeys(ctx context.Context, r *devopsv1.ListSshKeysRequest) (*devopsv1.ListSshKeysResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	userID := r.UserId
	if userID == "" {
		userID = claims.Subject
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT id,user_id,public_key,label FROM ssh_keys WHERE user_id=? ORDER BY created_unix_ms LIMIT ?", userID, pageSize(r.PageSize))
	if err != nil {
		return nil, status.Error(codes.Internal, "list failed")
	}
	defer rows.Close()
	out := &devopsv1.ListSshKeysResponse{}
	for rows.Next() {
		key := &devopsv1.SshKey{}
		if err := rows.Scan(&key.Id, &key.UserId, &key.PublicKey, &key.Label); err != nil {
			return nil, status.Error(codes.Internal, "list failed")
		}
		out.Keys = append(out.Keys, key)
	}
	return out, rows.Err()
}
func (s Fleet) DeleteSshKey(ctx context.Context, r *devopsv1.DeleteSshKeyRequest) (*devopsv1.DeleteSshKeyResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	result, err := s.DB.ExecContext(ctx, "DELETE FROM ssh_keys WHERE id=? AND user_id=?", r.KeyId, claims.Subject)
	if err != nil {
		return nil, status.Error(codes.Internal, "delete failed")
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return nil, status.Error(codes.NotFound, "key not found")
	}
	return &devopsv1.DeleteSshKeyResponse{}, nil
}
func pageSize(value uint32) int {
	if value == 0 || value > 100 {
		return 100
	}
	return int(value)
}
func (s Fleet) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
