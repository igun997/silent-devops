package auth

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func EndpointUnaryInterceptor(issuer *Issuer, policies EndpointPolicies, now func() time.Time) grpc.UnaryServerInterceptor {
	if now == nil {
		now = time.Now
	}
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := policies.AuthorizePeer(ctx, info.FullMethod); err != nil {
			return nil, status.Error(codes.PermissionDenied, "peer address denied")
		}
		switch {
		case info.FullMethod == "/devops.v1.AuthService/Login", info.FullMethod == "/devops.v1.EnrollmentService/Enroll":
			return handler(ctx, request)
		case info.FullMethod == "/devops.v1.EnrollmentService/Renew":
			return handler(ctx, request)
		case strings.HasPrefix(info.FullMethod, "/devops.v1.FleetService/"):
			authenticated, err := AuthenticateFleet(ctx, issuer, policies, now(), info.FullMethod)
			if err != nil {
				return nil, status.Error(codes.Unauthenticated, "authentication failed")
			}
			return handler(authenticated, request)
		default:
			return nil, status.Error(codes.PermissionDenied, "endpoint denied")
		}
	}
}
