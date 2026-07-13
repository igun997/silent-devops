package auth

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type claimsKey struct{}

func ContextWithClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(Claims)
	return claims, ok
}
func AuthenticateContext(ctx context.Context, issuer *Issuer, policy CIDRPolicy, now time.Time) (context.Context, error) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return ctx, errors.New("peer address missing")
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		host = p.Addr.String()
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !policy.Allows(addr) {
		return ctx, errors.New("peer address denied")
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx, errors.New("authorization metadata missing")
	}
	values := md.Get("authorization")
	if len(values) != 1 {
		return ctx, errors.New("authorization metadata invalid")
	}
	token, err := ParseBearer(values[0])
	if err != nil {
		return ctx, err
	}
	claims, err := issuer.Verify(token, now)
	if err != nil {
		return ctx, err
	}
	return ContextWithClaims(ctx, claims), nil
}
func AuthorizeMethod(ctx context.Context, method string) error {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "authentication required")
	}
	if !MethodAllowed(method, claims.Role) {
		return status.Error(codes.PermissionDenied, "role denied")
	}
	return nil
}
func UnaryInterceptor(issuer *Issuer, policy CIDRPolicy, now func() time.Time) grpc.UnaryServerInterceptor {
	if now == nil {
		now = time.Now
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		authenticated, err := AuthenticateContext(ctx, issuer, policy, now())
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "authentication failed")
		}
		if err := AuthorizeMethod(authenticated, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(authenticated, req)
	}
}
