package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	devopsv1 "silent-devops/api/devops/v1"
)

type Service struct {
	db      *sql.DB
	issuer  *Issuer
	limiter *RateLimiter
	now     func() time.Time
}

func NewService(db *sql.DB, issuer *Issuer, limiter *RateLimiter, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, issuer: issuer, limiter: limiter, now: now}
}
func (s *Service) Issue(userID string, role devopsv1.Role) (*devopsv1.LoginResponse, error) {
	token, err := s.issuer.Issue(userID, role, s.now())
	if err != nil {
		return nil, err
	}
	claims, err := s.issuer.Verify(token, s.now())
	if err != nil {
		return nil, err
	}
	return &devopsv1.LoginResponse{AccessToken: token, ExpiresUnixMs: claims.Expires * 1000, Role: claims.Role}, nil
}
func (s *Service) Login(ctx context.Context, source, username, password string) (*devopsv1.LoginResponse, error) {
	if !s.limiter.Allow(source, s.now()) {
		return nil, errors.New("login rate limited")
	}
	var id string
	var hash []byte
	var role int32
	var disabled bool
	err := s.db.QueryRowContext(ctx, "SELECT id,password_hash,role,disabled FROM users WHERE username=?", username).Scan(&id, &hash, &role, &disabled)
	if err != nil || disabled || !VerifyPassword(string(hash), password) {
		return nil, errors.New("invalid credentials")
	}
	return s.Issue(id, devopsv1.Role(role))
}

var methodActions = map[string]Action{
	"/devops.v1.FleetService/ListAgents": ActionRead, "/devops.v1.FleetService/GetAgent": ActionRead, "/devops.v1.FleetService/GetMetrics": ActionRead, "/devops.v1.FleetService/StreamJobOutput": ActionRead, "/devops.v1.FleetService/ListAudit": ActionRead, "/devops.v1.FleetService/ListProcesses": ActionRead, "/devops.v1.FleetService/ListServices": ActionRead, "/devops.v1.FleetService/GetService": ActionRead, "/devops.v1.FleetService/ReadLogs": ActionRead,
	"/devops.v1.FleetService/StartService": ActionOperate, "/devops.v1.FleetService/StopService": ActionOperate, "/devops.v1.FleetService/RestartService": ActionOperate, "/devops.v1.FleetService/PreviewCleanup": ActionOperate, "/devops.v1.FleetService/RunCleanup": ActionOperate, "/devops.v1.FleetService/Reboot": ActionOperate, "/devops.v1.FleetService/CancelJob": ActionOperate, "/devops.v1.FleetService/PrepareSsh": ActionOperate, "/devops.v1.FleetService/GetSshSession": ActionOperate, "/devops.v1.FleetService/CloseSsh": ActionOperate, "/devops.v1.FleetService/BridgeSsh": ActionOperate,
	"/devops.v1.FleetService/Exec": ActionAdmin, "/devops.v1.FleetService/CreateClientInvitation": ActionAdmin, "/devops.v1.FleetService/ListClientInvitations": ActionAdmin, "/devops.v1.FleetService/RevokeClientInvitation": ActionAdmin, "/devops.v1.FleetService/CreateEnrollmentToken": ActionAdmin, "/devops.v1.FleetService/ListEnrollmentTokens": ActionAdmin, "/devops.v1.FleetService/RevokeEnrollmentToken": ActionAdmin, "/devops.v1.FleetService/RevokeAgent": ActionAdmin, "/devops.v1.FleetService/CreateUser": ActionAdmin, "/devops.v1.FleetService/ListUsers": ActionAdmin, "/devops.v1.FleetService/SetUserRole": ActionAdmin, "/devops.v1.FleetService/AddSshKey": ActionAdmin, "/devops.v1.FleetService/ListSshKeys": ActionAdmin, "/devops.v1.FleetService/DeleteSshKey": ActionAdmin,
}

func MethodAllowed(method string, role devopsv1.Role) bool {
	action, ok := methodActions[method]
	return ok && Allowed(role, action)
}
