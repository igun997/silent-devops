package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/auth"
	"silent-devops/internal/pki"
	"silent-devops/internal/registry"
	sshmanager "silent-devops/internal/ssh"
)

type Fleet struct {
	devopsv1.UnimplementedFleetServiceServer
	DB       *sql.DB
	Tokens   *pki.EnrollmentManager
	Registry *registry.Registry
	SSH      *sshmanager.Manager
	Now      func() time.Time
}

func (s Fleet) ListAgents(ctx context.Context, request *devopsv1.ListAgentsRequest) (*devopsv1.ListAgentsResponse, error) {
	limit := request.GetPageSize()
	if limit == 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT a.id,a.hostname,a.revoked_unix_ms,c.state,COALESCE(c.connected_unix_ms,c.disconnected_unix_ms,0) FROM agents a LEFT JOIN connections c ON c.agent_id=a.id ORDER BY a.id LIMIT ?", limit)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "storage unavailable")
	}
	defer rows.Close()
	response := &devopsv1.ListAgentsResponse{}
	for rows.Next() {
		agent := &devopsv1.Agent{}
		var revoked sql.NullInt64
		var connection sql.NullString
		if err := rows.Scan(&agent.Id, &agent.Hostname, &revoked, &connection, &agent.LastSeenUnixMs); err != nil {
			return nil, err
		}
		agent.Revoked = revoked.Valid
		agent.Online = connection.String == "online"
		response.Agents = append(response.Agents, agent)
	}
	return response, rows.Err()
}
func (s Fleet) GetAgent(ctx context.Context, request *devopsv1.GetAgentRequest) (*devopsv1.Agent, error) {
	agent := &devopsv1.Agent{Id: request.GetAgentId()}
	var revoked sql.NullInt64
	var connection sql.NullString
	if err := s.DB.QueryRowContext(ctx, "SELECT a.hostname,a.revoked_unix_ms,c.state,COALESCE(c.connected_unix_ms,c.disconnected_unix_ms,0) FROM agents a LEFT JOIN connections c ON c.agent_id=a.id WHERE a.id=?", agent.Id).Scan(&agent.Hostname, &revoked, &connection, &agent.LastSeenUnixMs); err != nil {
		return nil, status.Error(codes.NotFound, "agent not found")
	}
	agent.Revoked = revoked.Valid
	agent.Online = connection.String == "online"
	return agent, nil
}
func (s Fleet) CreateEnrollmentToken(ctx context.Context, request *devopsv1.CreateEnrollmentTokenRequest) (*devopsv1.EnrollmentToken, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	ttl := time.Duration(request.GetTtlSeconds()) * time.Second
	if ttl <= 0 || ttl > time.Hour {
		return nil, status.Error(codes.InvalidArgument, "TTL out of range")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	token, err := s.Tokens.CreateToken(ctx, claims.Subject, now(), ttl)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "token creation failed")
	}
	idRaw, _ := hex.DecodeString(token)
	sum := sha256.Sum256(idRaw)
	return &devopsv1.EnrollmentToken{Token: token, Id: hex.EncodeToString(sum[:16]), ExpiresUnixMs: now().Add(ttl).UnixMilli()}, nil
}
func (s Fleet) CreateUser(ctx context.Context, request *devopsv1.CreateUserRequest) (*devopsv1.User, error) {
	if request.GetUsername() == "" || request.GetRole() < devopsv1.Role_ROLE_VIEWER || request.GetRole() > devopsv1.Role_ROLE_ADMIN {
		return nil, status.Error(codes.InvalidArgument, "invalid user")
	}
	hash, err := auth.HashPassword(request.GetPassword())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid password")
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, status.Error(codes.Internal, "entropy unavailable")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if _, err := s.DB.ExecContext(ctx, "INSERT INTO users(id,username,password_hash,role,created_unix_ms) VALUES(?,?,?,?,?)", id, request.GetUsername(), []byte(hash), request.GetRole(), now().UnixMilli()); err != nil {
		return nil, status.Error(codes.AlreadyExists, "user exists")
	}
	return &devopsv1.User{Id: id, Username: request.GetUsername(), Role: request.GetRole()}, nil
}
func (s Fleet) ListUsers(ctx context.Context, request *devopsv1.ListUsersRequest) (*devopsv1.ListUsersResponse, error) {
	limit := request.GetPageSize()
	if limit == 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT id,username,role,disabled FROM users ORDER BY username LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &devopsv1.ListUsersResponse{}
	for rows.Next() {
		user := &devopsv1.User{}
		var role int32
		if err := rows.Scan(&user.Id, &user.Username, &role, &user.Disabled); err != nil {
			return nil, err
		}
		user.Role = devopsv1.Role(role)
		out.Users = append(out.Users, user)
	}
	return out, rows.Err()
}
func randomHex(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("invalid random length")
	}
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
