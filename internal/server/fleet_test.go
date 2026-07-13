package server_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"silent-devops/internal/pki"
	"silent-devops/internal/server"
	"silent-devops/internal/store"
)

func TestFleetReadsAndAdminManagement(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	hash, _ := auth.HashPassword("correct horse battery staple")
	s.DB().Exec("INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES('admin','admin',?,3,?)", []byte(hash), now.UnixMilli())
	s.DB().Exec("INSERT INTO agents(id,hostname,created_unix_ms) VALUES('a','host',?)", now.UnixMilli())
	fleet := server.Fleet{DB: s.DB(), Tokens: pki.NewEnrollmentManager(s.DB()), Now: func() time.Time { return now }}
	agents, err := fleet.ListAgents(context.Background(), &devopsv1.ListAgentsRequest{PageSize: 10})
	if err != nil || len(agents.Agents) != 1 {
		t.Fatalf("agents=%v err=%v", agents, err)
	}
	agent, err := fleet.GetAgent(context.Background(), &devopsv1.GetAgentRequest{AgentId: "a"})
	if err != nil || agent.Hostname != "host" {
		t.Fatalf("agent=%v err=%v", agent, err)
	}
	token, err := fleet.CreateEnrollmentToken(auth.ContextWithClaims(context.Background(), auth.Claims{Subject: "admin", Role: devopsv1.Role_ROLE_ADMIN}), &devopsv1.CreateEnrollmentTokenRequest{TtlSeconds: 60})
	if err != nil || token.Token == "" {
		t.Fatalf("token=%v err=%v", token, err)
	}
	user, err := fleet.CreateUser(context.Background(), &devopsv1.CreateUserRequest{Username: "viewer", Password: "another correct password", Role: devopsv1.Role_ROLE_VIEWER})
	if err != nil || user.Username != "viewer" {
		t.Fatalf("user=%v err=%v", user, err)
	}
}
