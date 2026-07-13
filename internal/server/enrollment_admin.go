package server

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"time"
)

func (s Fleet) ListEnrollmentTokens(ctx context.Context, r *devopsv1.ListEnrollmentTokensRequest) (*devopsv1.ListEnrollmentTokensResponse, error) {
	items, err := s.Tokens.List(ctx, int(r.PageSize))
	if err != nil {
		return nil, status.Error(codes.Internal, "join codes unavailable")
	}
	out := &devopsv1.ListEnrollmentTokensResponse{}
	for _, item := range items {
		out.Tokens = append(out.Tokens, &devopsv1.EnrollmentToken{Id: item.ID, ExpiresUnixMs: item.Expires, Consumed: item.Consumed, Revoked: item.Revoked})
	}
	return out, nil
}
func (s Fleet) RevokeEnrollmentToken(ctx context.Context, r *devopsv1.RevokeEnrollmentTokenRequest) (*devopsv1.EnrollmentToken, error) {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	if err := s.Tokens.Revoke(ctx, r.Id, now); err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &devopsv1.EnrollmentToken{Id: r.Id, Revoked: true}, nil
}
func (s Fleet) RevokeAgent(ctx context.Context, r *devopsv1.RevokeAgentRequest) (*devopsv1.Agent, error) {
	if r.AgentId == "" || r.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "agent and reason required")
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	result, err := s.DB.ExecContext(ctx, "UPDATE agents SET revoked_unix_ms=? WHERE id=? AND revoked_unix_ms IS NULL", now.UnixMilli(), r.AgentId)
	if err != nil {
		return nil, status.Error(codes.Internal, "revoke failed")
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return nil, status.Error(codes.NotFound, "active agent not found")
	}
	if s.Registry != nil {
		s.Registry.Disconnect(r.AgentId, now)
	}
	return s.GetAgent(ctx, &devopsv1.GetAgentRequest{AgentId: r.AgentId})
}
