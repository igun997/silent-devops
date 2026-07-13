package server

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
)

type Auth struct {
	devopsv1.UnimplementedAuthServiceServer
	Service *auth.Service
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

var errUnavailable = errors.New("service unavailable")
