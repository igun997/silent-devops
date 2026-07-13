package server

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"time"
)

func (s Fleet) PrepareSsh(ctx context.Context, r *devopsv1.PrepareSshRequest) (*devopsv1.SshSession, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if claims.Role != devopsv1.Role_ROLE_ADMIN {
		return nil, status.Error(codes.PermissionDenied, "admin required")
	}
	if s.SSH == nil || s.Registry == nil {
		return nil, status.Error(codes.Unavailable, "SSH unavailable")
	}
	session, prepare, err := s.SSH.Create(ctx, claims.Subject, r.AgentId, r.PublicKey, r.Reason, time.Duration(r.TtlSeconds)*time.Second)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	message := &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_PrepareSsh{PrepareSsh: prepare}}
	if err := s.Registry.Dispatch(ctx, r.AgentId, message); err != nil {
		_, _ = s.SSH.Close(ctx, session.Id, "dispatch failed")
		return nil, status.Error(codes.Unavailable, "agent offline or backpressured")
	}
	return session, nil
}
func (s Fleet) CloseSsh(ctx context.Context, r *devopsv1.CloseSshRequest) (*devopsv1.SshSession, error) {
	if s.SSH == nil {
		return nil, status.Error(codes.Unavailable, "SSH unavailable")
	}
	session, err := s.SSH.Close(ctx, r.SessionId, r.Reason)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if s.Registry != nil {
		_ = s.Registry.Dispatch(ctx, session.AgentId, &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_CloseSsh{CloseSsh: &devopsv1.CloseSsh{SessionId: session.Id}}})
	}
	return session, nil
}
