package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"silent-devops/internal/store"
)

func TestLoginAndDisabledUser(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	hash, _ := auth.HashPassword("correct horse battery staple")
	if _, err := s.DB().Exec("INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES('u','alice',?,2,?)", []byte(hash), time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	issuer, _ := auth.NewIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	svc := auth.NewService(s.DB(), issuer, auth.NewRateLimiter(2, time.Minute), time.Now)
	response, err := svc.Login(context.Background(), "192.0.2.1", "alice", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if response.Role != devopsv1.Role_ROLE_OPERATOR || response.AccessToken == "" {
		t.Fatal("bad login response")
	}
	if _, err := s.DB().Exec("UPDATE users SET disabled=1 WHERE id='u'"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(context.Background(), "192.0.2.1", "alice", "correct horse battery staple"); err == nil {
		t.Fatal("disabled user logged in")
	}
}

func TestMethodRoleMatrixDefaultsDeny(t *testing.T) {
	cases := []struct {
		method string
		role   devopsv1.Role
		want   bool
	}{{"/devops.v1.FleetService/ListAgents", devopsv1.Role_ROLE_VIEWER, true}, {"/devops.v1.FleetService/StartService", devopsv1.Role_ROLE_VIEWER, false}, {"/devops.v1.FleetService/StartService", devopsv1.Role_ROLE_OPERATOR, true}, {"/devops.v1.FleetService/Exec", devopsv1.Role_ROLE_OPERATOR, false}, {"/devops.v1.FleetService/Exec", devopsv1.Role_ROLE_ADMIN, true}, {"/unknown", devopsv1.Role_ROLE_ADMIN, false}}
	for _, tc := range cases {
		if got := auth.MethodAllowed(tc.method, tc.role); got != tc.want {
			t.Errorf("%s role=%v got=%v", tc.method, tc.role, got)
		}
	}
}
