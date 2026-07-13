package auth_test

import (
	"net/netip"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
)

func TestCIDRPolicyIPv4IPv6AndInvalid(t *testing.T) {
	p, err := auth.ParseCIDRs([]string{"10.0.0.0/8", "2001:db8::/32"})
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"10.2.3.4", "2001:db8::1"} {
		if !p.Allows(netip.MustParseAddr(ip)) {
			t.Fatalf("%s rejected", ip)
		}
	}
	if p.Allows(netip.MustParseAddr("192.0.2.1")) {
		t.Fatal("outside address accepted")
	}
	if _, err := auth.ParseCIDRs([]string{"bad"}); err == nil {
		t.Fatal("invalid CIDR accepted")
	}
	if _, err := auth.ParseCIDRs(nil); err == nil {
		t.Fatal("empty CIDR policy accepted")
	}
}

func TestPasswordHashAndTokenLifecycle(t *testing.T) {
	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("correct password rejected")
	}
	if auth.VerifyPassword(hash, "wrong") {
		t.Fatal("wrong password accepted")
	}
	issuer, err := auth.NewIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token, err := issuer.Issue("user-1", devopsv1.Role_ROLE_OPERATOR, now)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := issuer.Verify(token, now.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" || claims.Role != devopsv1.Role_ROLE_OPERATOR {
		t.Fatal("claims mismatch")
	}
	if _, err := issuer.Verify(token, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestRateLimiterAndRBAC(t *testing.T) {
	l := auth.NewRateLimiter(2, time.Minute)
	now := time.Now()
	if !l.Allow("ip", now) || !l.Allow("ip", now) || l.Allow("ip", now) {
		t.Fatal("rate limit mismatch")
	}
	if !l.Allow("ip", now.Add(time.Minute)) {
		t.Fatal("rate limit did not reset")
	}
	cases := []struct {
		role   devopsv1.Role
		action auth.Action
		want   bool
	}{{devopsv1.Role_ROLE_VIEWER, auth.ActionRead, true}, {devopsv1.Role_ROLE_VIEWER, auth.ActionOperate, false}, {devopsv1.Role_ROLE_OPERATOR, auth.ActionOperate, true}, {devopsv1.Role_ROLE_OPERATOR, auth.ActionAdmin, false}, {devopsv1.Role_ROLE_ADMIN, auth.ActionAdmin, true}}
	for _, tc := range cases {
		if got := auth.Allowed(tc.role, tc.action); got != tc.want {
			t.Errorf("role=%v action=%v got=%v", tc.role, tc.action, got)
		}
	}
}
