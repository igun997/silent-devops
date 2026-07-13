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

func peerContext(ip string) context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP(ip), Port: 443}})
}
func TestEndpointPoliciesSeparateNetworks(t *testing.T) {
	enroll, _ := auth.ParseCIDRs([]string{"10.0.0.0/8"})
	agent, _ := auth.ParseCIDRs([]string{"172.16.0.0/12"})
	client, _ := auth.ParseCIDRs([]string{"192.0.2.0/24"})
	p := auth.EndpointPolicies{Enrollment: enroll, Agent: agent, Client: client}
	cases := []struct {
		method, ip string
		want       bool
	}{{"/devops.v1.EnrollmentService/Enroll", "10.1.1.1", true}, {"/devops.v1.AgentService/Connect", "172.16.2.2", true}, {"/devops.v1.AuthService/Login", "192.0.2.2", true}, {"/devops.v1.FleetService/ListAgents", "192.0.2.3", true}, {"/devops.v1.AgentService/Connect", "10.1.1.1", false}}
	for _, tc := range cases {
		err := p.AuthorizePeer(peerContext(tc.ip), tc.method)
		if (err == nil) != tc.want {
			t.Errorf("%s %s err=%v", tc.method, tc.ip, err)
		}
	}
}
func TestLoginMethodNeedsCIDRButNoBearer(t *testing.T) {
	client, _ := auth.ParseCIDRs([]string{"192.0.2.0/24"})
	p := auth.EndpointPolicies{Client: client}
	if err := p.AuthorizePeer(peerContext("192.0.2.1"), "/devops.v1.AuthService/Login"); err != nil {
		t.Fatal(err)
	}
}
func TestFleetAuthentication(t *testing.T) {
	issuer, _ := auth.NewIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	token, _ := issuer.Issue("u", devopsv1.Role_ROLE_VIEWER, time.Now())
	ctx := metadata.NewIncomingContext(peerContext("192.0.2.1"), metadata.Pairs("authorization", "Bearer "+token))
	client, _ := auth.ParseCIDRs([]string{"192.0.2.0/24"})
	p := auth.EndpointPolicies{Client: client}
	authenticated, err := auth.AuthenticateFleet(ctx, issuer, p, time.Now(), "/devops.v1.FleetService/ListAgents")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := auth.ClaimsFromContext(authenticated); !ok {
		t.Fatal("claims missing")
	}
}
