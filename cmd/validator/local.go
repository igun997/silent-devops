package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/localcontrol"
	"strconv"
)

func runLocal(ctx context.Context, args []string, out io.Writer, socket string) error {
	if len(args) == 0 {
		return errors.New("local command required")
	}
	conn, err := localcontrol.Dial(ctx, socket)
	if err != nil {
		return err
	}
	defer conn.Close()
	fleet := devopsv1.NewFleetServiceClient(conn)
	var result any
	switch {
	case len(args) == 2 && args[0] == "agents" && args[1] == "list":
		result, err = fleet.ListAgents(ctx, &devopsv1.ListAgentsRequest{PageSize: 100})
	case len(args) == 3 && args[0] == "agents" && args[1] == "show":
		result, err = fleet.GetAgent(ctx, &devopsv1.GetAgentRequest{AgentId: args[2]})
	case len(args) == 2 && args[0] == "metrics":
		result, err = fleet.GetMetrics(ctx, &devopsv1.GetMetricsRequest{AgentId: args[1]})
	case len(args) == 2 && args[0] == "join-code" && args[1] == "list":
		result, err = fleet.ListEnrollmentTokens(ctx, &devopsv1.ListEnrollmentTokensRequest{PageSize: 100})
	case len(args) == 3 && args[0] == "join-code" && args[1] == "revoke":
		result, err = fleet.RevokeEnrollmentToken(ctx, &devopsv1.RevokeEnrollmentTokenRequest{Id: args[2]})
	case len(args) == 4 && args[0] == "agents" && args[1] == "revoke":
		result, err = fleet.RevokeAgent(ctx, &devopsv1.RevokeAgentRequest{AgentId: args[2], Reason: args[3]})
	case len(args) >= 2 && (args[0] == "enroll-token" || args[0] == "join-code") && args[1] == "create":
		ttl := uint32(600)
		if len(args) == 3 {
			v, e := strconv.ParseUint(args[2], 10, 32)
			if e != nil {
				return e
			}
			ttl = uint32(v)
		}
		result, err = fleet.CreateEnrollmentToken(ctx, &devopsv1.CreateEnrollmentTokenRequest{TtlSeconds: ttl})
	case len(args) == 2 && args[0] == "audit" && args[1] == "list":
		result, err = fleet.ListAudit(ctx, &devopsv1.ListAuditRequest{PageSize: 100})
	case len(args) == 4 && args[0] == "users" && args[1] == "create":
		role, roleErr := parseRole(args[3])
		if roleErr != nil {
			return roleErr
		}
		password, passwordErr := generatedPassword()
		if passwordErr != nil {
			return passwordErr
		}
		user, createErr := fleet.CreateUser(ctx, &devopsv1.CreateUserRequest{Username: args[2], Password: password, Role: role})
		if createErr != nil {
			return createErr
		}
		result = map[string]any{"user": user, "password": password}
	default:
		return fmt.Errorf("unknown local command")
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
func parseRole(value string) (devopsv1.Role, error) {
	switch value {
	case "viewer":
		return devopsv1.Role_ROLE_VIEWER, nil
	case "operator":
		return devopsv1.Role_ROLE_OPERATOR, nil
	case "admin":
		return devopsv1.Role_ROLE_ADMIN, nil
	default:
		return 0, errors.New("role must be viewer, operator, or admin")
	}
}
func generatedPassword() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
