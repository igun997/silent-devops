package server_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/peer"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"silent-devops/internal/server"
	"silent-devops/internal/store"
)

func TestAuthServerLoginUsesPeerAddress(t *testing.T) {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	hash, _ := auth.HashPassword("correct horse battery staple")
	s.DB().Exec("INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES('u','alice',?,2,?)", []byte(hash), time.Now().UnixMilli())
	issuer, _ := auth.NewIssuer([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	service := auth.NewService(s.DB(), issuer, auth.NewRateLimiter(5, time.Minute), time.Now)
	srv := server.Auth{Service: service}
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 123}})
	response, err := srv.Login(ctx, &devopsv1.LoginRequest{Username: "alice", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	if response.AccessToken == "" {
		t.Fatal("token missing")
	}
}
