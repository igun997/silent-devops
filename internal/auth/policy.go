package auth

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type EndpointPolicies struct{ Enrollment, Agent, Client CIDRPolicy }

func (p EndpointPolicies) AuthorizePeer(ctx context.Context, method string) error {
	var policy CIDRPolicy
	switch {
	case strings.HasPrefix(method, "/devops.v1.EnrollmentService/"):
		policy = p.Enrollment
	case strings.HasPrefix(method, "/devops.v1.AgentService/"):
		policy = p.Agent
	case strings.HasPrefix(method, "/devops.v1.AuthService/"), strings.HasPrefix(method, "/devops.v1.FleetService/"):
		policy = p.Client
	default:
		return errors.New("unknown endpoint")
	}
	addr, err := peerAddr(ctx)
	if err != nil {
		return err
	}
	if !policy.Allows(addr) {
		return errors.New("peer address denied")
	}
	return nil
}
func peerAddr(ctx context.Context) (netip.Addr, error) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return netip.Addr{}, errors.New("peer address missing")
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		host = p.Addr.String()
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, errors.New("invalid peer address")
	}
	return addr.Unmap(), nil
}
func AuthenticateFleet(ctx context.Context, issuer *Issuer, policies EndpointPolicies, now time.Time, method string) (context.Context, error) {
	if err := policies.AuthorizePeer(ctx, method); err != nil {
		return ctx, err
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
	authenticated := ContextWithClaims(ctx, claims)
	if err := AuthorizeMethod(authenticated, method); err != nil {
		return ctx, err
	}
	return authenticated, nil
}
