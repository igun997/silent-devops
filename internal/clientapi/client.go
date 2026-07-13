package clientapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	devopsv1 "silent-devops/api/devops/v1"
)

type Store interface {
	Save(string) error
	Load() (string, error)
	Clear() error
}
type AuthClient interface {
	Login(context.Context, *devopsv1.LoginRequest, ...grpc.CallOption) (*devopsv1.LoginResponse, error)
}
type Adapter struct {
	conn  *grpc.ClientConn
	auth  AuthClient
	fleet devopsv1.FleetServiceClient
	store Store
}

func Dial(address, caPath, serverName string, store Store) (*Adapter, error) {
	if address == "" || caPath == "" || store == nil {
		return nil, errors.New("address, CA, and credential store required")
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid validator CA")
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: pool, ServerName: serverName, MinVersion: tls.VersionTLS13})))
	if err != nil {
		return nil, err
	}
	return &Adapter{conn: conn, auth: devopsv1.NewAuthServiceClient(conn), fleet: devopsv1.NewFleetServiceClient(conn), store: store}, nil
}
func NewForTest(auth AuthClient, fleet devopsv1.FleetServiceClient, store Store) *Adapter {
	return &Adapter{auth: auth, fleet: fleet, store: store}
}
func (a *Adapter) Close() error {
	if a.conn == nil {
		return nil
	}
	return a.conn.Close()
}
func (a *Adapter) Call(ctx context.Context, command string, args []string) (any, error) {
	switch command {
	case "login":
		if len(args) != 2 {
			return nil, errors.New("username and password required")
		}
		response, err := a.auth.Login(ctx, &devopsv1.LoginRequest{Username: args[0], Password: args[1]})
		if err != nil {
			return nil, err
		}
		if err := a.store.Save(response.AccessToken); err != nil {
			return nil, err
		}
		return map[string]any{"access_token": nil, "role": response.Role.String()}, nil
	case "logout":
		return map[string]any{"ok": true}, a.store.Clear()
	}
	token, err := a.store.Load()
	if err != nil {
		return nil, errors.New("login required")
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	switch command {
	case "agents":
		if len(args) > 0 && args[0] == "show" {
			if len(args) != 2 {
				return nil, errors.New("agent ID required")
			}
			return a.fleet.GetAgent(ctx, &devopsv1.GetAgentRequest{AgentId: args[1]})
		}
		return a.fleet.ListAgents(ctx, &devopsv1.ListAgentsRequest{PageSize: 100})
	case "enroll-token":
		return a.fleet.CreateEnrollmentToken(ctx, &devopsv1.CreateEnrollmentTokenRequest{TtlSeconds: 300})
	case "users":
		return a.fleet.ListUsers(ctx, &devopsv1.ListUsersRequest{PageSize: 100})
	default:
		return nil, errors.New("command not wired")
	}
}
