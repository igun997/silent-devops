package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"time"
)

type Auth struct {
	devopsv1.UnimplementedAuthServiceServer
	Service *auth.Service
	DB      *sql.DB
	Now     func() time.Time
}

func (s Auth) Login(ctx context.Context, request *devopsv1.LoginRequest) (*devopsv1.LoginResponse, error) {
	if s.Service == nil {
		return nil, status.Error(codes.FailedPrecondition, "auth unavailable")
	}
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return nil, status.Error(codes.Unauthenticated, "peer unavailable")
	}
	response, err := s.Service.Login(ctx, p.Addr.String(), request.GetUsername(), request.GetPassword())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	return response, nil
}

func (s Auth) RedeemClientInvitation(ctx context.Context, r *devopsv1.RedeemClientInvitationRequest) (*devopsv1.LoginResponse, error) {
	if s.Service == nil || s.DB == nil {
		return nil, status.Error(codes.FailedPrecondition, "auth unavailable")
	}
	secret, err := hex.DecodeString(r.Secret)
	if err != nil || len(secret) != 32 {
		return nil, status.Error(codes.Unauthenticated, "invalid invitation")
	}
	hash, err := auth.HashPassword(r.Password)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid password")
	}
	sum := sha256.Sum256(secret)
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "storage unavailable")
	}
	defer tx.Rollback()
	var id, username string
	var role int32
	err = tx.QueryRowContext(ctx, "SELECT id,username,role FROM client_invitations WHERE secret_hash=? AND consumed_unix_ms IS NULL AND revoked_unix_ms IS NULL AND expires_unix_ms>?", sum[:], now.UnixMilli()).Scan(&id, &username, &role)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid invitation")
	}
	userID, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES(?,?,?,?,?)", userID, username, []byte(hash), role, now.UnixMilli()); err != nil {
		return nil, status.Error(codes.AlreadyExists, "user exists")
	}
	res, err := tx.ExecContext(ctx, "UPDATE client_invitations SET consumed_unix_ms=? WHERE id=? AND consumed_unix_ms IS NULL", now.UnixMilli(), id)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "storage unavailable")
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, status.Error(codes.Unauthenticated, "invalid invitation")
	}
	if err = tx.Commit(); err != nil {
		return nil, status.Error(codes.Unavailable, "storage unavailable")
	}
	return s.Service.Issue(userID, devopsv1.Role(role))
}

var errUnavailable = errors.New("service unavailable")
