package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
)

func (s Fleet) CreateClientInvitation(ctx context.Context, r *devopsv1.CreateClientInvitationRequest) (*devopsv1.ClientInvitation, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if r.Username == "" || r.Role < devopsv1.Role_ROLE_VIEWER || r.Role > devopsv1.Role_ROLE_ADMIN || r.TtlSeconds == 0 || r.TtlSeconds > 3600 || r.ValidatorAddress == "" || r.ValidatorPin == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid invitation")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	sum := sha256.Sum256(secret)
	now := time.Now()
	expires := now.Add(time.Duration(r.TtlSeconds) * time.Second)
	var createdBy any
	if !strings.HasPrefix(claims.Subject, "local:uid:") {
		createdBy = claims.Subject
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO client_invitations(id,secret_hash,username,role,validator_address,validator_pin,expires_unix_ms,created_by,created_unix_ms) VALUES(?,?,?,?,?,?,?,?,?)", id, sum[:], r.Username, r.Role, r.ValidatorAddress, r.ValidatorPin, expires.UnixMilli(), createdBy, now.UnixMilli())
	if err != nil {
		return nil, status.Error(codes.AlreadyExists, "invitation exists")
	}
	return &devopsv1.ClientInvitation{Id: id, Secret: hex.EncodeToString(secret), Username: r.Username, Role: r.Role, ExpiresUnixMs: expires.UnixMilli(), ValidatorAddress: r.ValidatorAddress, ValidatorPin: r.ValidatorPin}, nil
}
func (s Fleet) ListClientInvitations(ctx context.Context, r *devopsv1.ListClientInvitationsRequest) (*devopsv1.ListClientInvitationsResponse, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,username,role,validator_address,validator_pin,expires_unix_ms,consumed_unix_ms IS NOT NULL,revoked_unix_ms IS NOT NULL FROM client_invitations ORDER BY created_unix_ms DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &devopsv1.ListClientInvitationsResponse{}
	for rows.Next() {
		v := &devopsv1.ClientInvitation{}
		if err := rows.Scan(&v.Id, &v.Username, &v.Role, &v.ValidatorAddress, &v.ValidatorPin, &v.ExpiresUnixMs, &v.Consumed, &v.Revoked); err != nil {
			return nil, err
		}
		out.Invitations = append(out.Invitations, v)
	}
	return out, rows.Err()
}
func (s Fleet) RevokeClientInvitation(ctx context.Context, r *devopsv1.RevokeClientInvitationRequest) (*devopsv1.ClientInvitation, error) {
	now := time.Now().UnixMilli()
	res, err := s.DB.ExecContext(ctx, "UPDATE client_invitations SET revoked_unix_ms=? WHERE id=? AND consumed_unix_ms IS NULL AND revoked_unix_ms IS NULL", now, r.Id)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, status.Error(codes.NotFound, "active invitation not found")
	}
	return &devopsv1.ClientInvitation{Id: r.Id, Revoked: true}, nil
}
