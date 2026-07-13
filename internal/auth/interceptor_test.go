package auth_test

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
)

func TestAuthenticateContextAndCIDR(t *testing.T) {
	issuer, _ := auth.NewIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	now := time.Now()
	token, _ := issuer.Issue("u", devopsv1.Role_ROLE_OPERATOR, now)
	policy, _ := auth.ParseCIDRs([]string{"192.0.2.0/24"})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.4"), Port: 1234}})
	ctx, err := auth.AuthenticateContext(ctx, issuer, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims.Subject != "u" {
		t.Fatal("claims missing")
	}
}
func TestAuthenticateRejectsOutsideCIDRAndMissingToken(t *testing.T) {
	issuer, _ := auth.NewIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	policy, _ := auth.ParseCIDRs([]string{"10.0.0.0/8"})
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.4"), Port: 1}})
	if _, err := auth.AuthenticateContext(ctx, issuer, policy, time.Now()); err == nil {
		t.Fatal("outside CIDR accepted")
	}
}
func TestAuthorizeMethod(t *testing.T) {
	ctx := auth.ContextWithClaims(context.Background(), auth.Claims{Subject: "u", Role: devopsv1.Role_ROLE_VIEWER})
	if err := auth.AuthorizeMethod(ctx, "/devops.v1.FleetService/ListAgents"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AuthorizeMethod(ctx, "/devops.v1.FleetService/Exec"); err == nil {
		t.Fatal("viewer exec accepted")
	}
}
