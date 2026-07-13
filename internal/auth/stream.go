package auth

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type contextStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s contextStream) Context() context.Context { return s.ctx }
func StreamInterceptor(issuer *Issuer, policies EndpointPolicies, now func() time.Time) grpc.StreamServerInterceptor {
	if now == nil {
		now = time.Now
	}
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := policies.AuthorizePeer(stream.Context(), info.FullMethod); err != nil {
			return status.Error(codes.PermissionDenied, "peer address denied")
		}
		if info.FullMethod == "/devops.v1.AgentService/Connect" {
			return handler(srv, stream)
		}
		authenticated, err := AuthenticateFleet(stream.Context(), issuer, policies, now(), info.FullMethod)
		if err != nil {
			return status.Error(codes.Unauthenticated, "authentication failed")
		}
		return handler(srv, contextStream{ServerStream: stream, ctx: authenticated})
	}
}
