package auth_test

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"silent-devops/internal/auth"
)

func TestUnaryEndpointRules(t *testing.T) {
	enroll, _ := auth.ParseCIDRs([]string{"10.0.0.0/8"})
	client, _ := auth.ParseCIDRs([]string{"192.0.2.0/24"})
	issuer, _ := auth.NewIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	policies := auth.EndpointPolicies{Enrollment: enroll, Client: client}
	interceptor := auth.EndpointUnaryInterceptor(issuer, policies, time.Now)
	ctx := func(ip string) context.Context {
		return peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP(ip), Port: 1}})
	}
	called := false
	handler := func(context.Context, any) (any, error) { called = true; return "ok", nil }
	if _, err := interceptor(ctx("192.0.2.1"), nil, &grpc.UnaryServerInfo{FullMethod: "/devops.v1.AuthService/Login"}, handler); err != nil || !called {
		t.Fatal(err)
	}
	called = false
	if _, err := interceptor(ctx("10.1.1.1"), nil, &grpc.UnaryServerInfo{FullMethod: "/devops.v1.EnrollmentService/Enroll"}, handler); err != nil || !called {
		t.Fatal(err)
	}
	called = false
	if _, err := interceptor(ctx("192.0.2.1"), nil, &grpc.UnaryServerInfo{FullMethod: "/devops.v1.FleetService/ListAgents"}, handler); err == nil || called {
		t.Fatal("fleet accepted without bearer")
	}
}
