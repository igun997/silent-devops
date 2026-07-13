package localcontrol

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
)

type uidKey struct{}

func ContextWithPeerUID(ctx context.Context, uid uint32) context.Context {
	return context.WithValue(ctx, uidKey{}, uid)
}
func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		uid, ok := ctx.Value(uidKey{}).(uint32)
		if !ok {
			if p, found := peer.FromContext(ctx); found {
				if info, valid := p.AuthInfo.(PeerInfo); valid {
					uid, ok = info.UID, true
				}
			}
		}
		if !ok {
			return nil, status.Error(codes.PermissionDenied, "local peer identity required")
		}
		// Socket ownership/mode restricts connections to root and silent-devops-admin.
		// SO_PEERCRED prevents identity spoofing after connection.
		if uid != 0 {
			if p, found := peer.FromContext(ctx); !found || p.AuthInfo == nil {
				return nil, status.Error(codes.PermissionDenied, "local admin required")
			}
		}
		ctx = auth.ContextWithClaims(ctx, auth.Claims{Subject: fmt.Sprintf("local:uid:%d", uid), Role: devopsv1.Role_ROLE_ADMIN})
		return handler(ctx, req)
	}
}
