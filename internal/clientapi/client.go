package clientapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

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
	RedeemClientInvitation(context.Context, *devopsv1.RedeemClientInvitationRequest, ...grpc.CallOption) (*devopsv1.LoginResponse, error)
}
type Adapter struct {
	conn  *grpc.ClientConn
	auth  AuthClient
	fleet devopsv1.FleetServiceClient
	store Store
}

func (a *Adapter) Redeem(ctx context.Context, secret, password string) (*devopsv1.LoginResponse, error) {
	r, err := a.auth.RedeemClientInvitation(ctx, &devopsv1.RedeemClientInvitationRequest{Secret: secret, Password: password})
	if err != nil {
		return nil, err
	}
	if err = a.store.Save(r.AccessToken); err != nil {
		return nil, err
	}
	return r, nil
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

// Pipe relays stdin/stdout over a BridgeSsh gRPC stream for use as an OpenSSH
// ProxyCommand. The first frame carries the session id; bearer auth rides in
// the outgoing metadata.
func (a *Adapter) Pipe(ctx context.Context, sessionID string, in io.Reader, out io.Writer) error {
	token, err := a.store.Load()
	if err != nil {
		return err
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	stream, err := a.fleet.BridgeSsh(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&devopsv1.TunnelFrame{SessionId: sessionID}); err != nil {
		return err
	}
	errc := make(chan error, 2)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := in.Read(buf)
			if n > 0 {
				if serr := stream.Send(&devopsv1.TunnelFrame{Data: buf[:n]}); serr != nil {
					errc <- serr
					return
				}
			}
			if rerr != nil {
				_ = stream.Send(&devopsv1.TunnelFrame{Close: true})
				_ = stream.CloseSend()
				errc <- nil
				return
			}
		}
	}()
	go func() {
		for {
			frame, rerr := stream.Recv()
			if rerr == io.EOF {
				errc <- nil
				return
			}
			if rerr != nil {
				errc <- rerr
				return
			}
			if len(frame.Data) > 0 {
				if _, werr := out.Write(frame.Data); werr != nil {
					errc <- werr
					return
				}
			}
			if frame.Close {
				errc <- nil
				return
			}
		}
	}()
	return <-errc
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
		if len(args) == 2 && args[0] == "show" {
			return a.fleet.GetAgent(ctx, &devopsv1.GetAgentRequest{AgentId: args[1]})
		}
		return a.fleet.ListAgents(ctx, &devopsv1.ListAgentsRequest{PageSize: 100})
	case "stats":
		if len(args) != 1 {
			return nil, errors.New("agent ID required")
		}
		return a.fleet.GetMetrics(ctx, &devopsv1.GetMetricsRequest{AgentId: args[0], SinceUnixMs: time.Now().Add(-time.Hour).UnixMilli(), PageSize: 100})
	case "services":
		return a.services(ctx, args)
	case "logs":
		if len(args) != 2 {
			return nil, errors.New("agent ID and unit required")
		}
		return a.fleet.ReadLogs(ctx, &devopsv1.JournalJobRequest{Context: jobContext(args[0], "read logs", false), Request: &devopsv1.JournalReadRequest{Unit: args[1], LineLimit: 200}})
	case "cleanup":
		if len(args) < 3 {
			return nil, errors.New("cleanup action, agent ID, and value required")
		}
		if args[0] == "preview" {
			return a.fleet.PreviewCleanup(ctx, &devopsv1.CleanupPreviewJobRequest{Context: jobContext(args[1], "cleanup preview", false), Request: &devopsv1.CleanupPreviewRequest{AllowedPaths: args[2:]}})
		}
		return nil, errors.New("cleanup run requires preview metadata")
	case "reboot":
		return a.fleet.Reboot(ctx, &devopsv1.RebootJobRequest{Context: jobContext(args[0], "reboot", true), Request: &devopsv1.RebootRequest{TargetAgentId: args[0], Confirmation: args[0], ConfirmationExpiresUnixMs: time.Now().Add(time.Minute).UnixMilli()}})
	case "exec":
		return a.fleet.Exec(ctx, &devopsv1.ExecJobRequest{Context: jobContext(args[0], "admin exec", true), Request: &devopsv1.ArbitraryCommand{Command: strings.Join(args[1:], " "), CaptureOutput: true}})
	case "enroll-token":
		if len(args) == 0 || args[0] == "create" {
			return a.fleet.CreateEnrollmentToken(ctx, &devopsv1.CreateEnrollmentTokenRequest{TtlSeconds: 300})
		}
		if args[0] == "list" {
			return a.fleet.ListEnrollmentTokens(ctx, &devopsv1.ListEnrollmentTokensRequest{PageSize: 100})
		}
		if len(args) == 2 && args[0] == "revoke" {
			return a.fleet.RevokeEnrollmentToken(ctx, &devopsv1.RevokeEnrollmentTokenRequest{Id: args[1]})
		}
		return nil, errors.New("invalid enroll-token action")
	case "users":
		if len(args) == 0 || args[0] == "list" {
			return a.fleet.ListUsers(ctx, &devopsv1.ListUsersRequest{PageSize: 100})
		}
		return nil, errors.New("invalid users action")
	case "ssh-keys":
		if len(args) == 0 || args[0] == "list" {
			user := ""
			if len(args) > 1 {
				user = args[1]
			}
			return a.fleet.ListSshKeys(ctx, &devopsv1.ListSshKeysRequest{UserId: user, PageSize: 100})
		}
		if len(args) == 3 && args[0] == "add" {
			key, err := os.ReadFile(args[1])
			if err != nil {
				return nil, err
			}
			return a.fleet.AddSshKey(ctx, &devopsv1.AddSshKeyRequest{PublicKey: key, Label: args[2]})
		}
		if len(args) == 2 && args[0] == "delete" {
			return a.fleet.DeleteSshKey(ctx, &devopsv1.DeleteSshKeyRequest{KeyId: args[1]})
		}
		return nil, errors.New("invalid ssh-keys action")
	case "audit":
		return a.fleet.ListAudit(ctx, &devopsv1.ListAuditRequest{PageSize: 100})
	case "ssh":
		if len(args) == 2 && args[0] == "status" {
			return a.fleet.GetSshSession(ctx, &devopsv1.GetSshSessionRequest{SessionId: args[1]})
		}
		if len(args) == 2 && args[0] == "close" {
			return a.fleet.CloseSsh(ctx, &devopsv1.CloseSshRequest{SessionId: args[1], Reason: "client closed"})
		}
		if len(args) != 2 {
			return nil, errors.New("agent ID and public key required")
		}
		key, err := os.ReadFile(args[1])
		if err != nil {
			return nil, err
		}
		return a.fleet.PrepareSsh(ctx, &devopsv1.PrepareSshRequest{AgentId: args[0], PublicKey: key, Reason: "interactive SSH", TtlSeconds: 900})
	default:
		return nil, fmt.Errorf("unsupported command %q", command)
	}
}
func (a *Adapter) JobOutput(ctx context.Context, id string) (string, error) {
	token, err := a.store.Load()
	if err != nil {
		return "", errors.New("login required")
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	stream, err := a.fleet.StreamJobOutput(ctx, &devopsv1.StreamJobOutputRequest{JobId: id})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		out.Write(chunk.Data)
		if chunk.Final {
			break
		}
	}
	return out.String(), nil
}
func jobContext(agent, reason string, confirmed bool) *devopsv1.JobRequestContext {
	return &devopsv1.JobRequestContext{AgentId: agent, Reason: reason, TimeoutSeconds: 30, IdempotencyKey: strconv.FormatInt(time.Now().UnixNano(), 36), Confirmed: confirmed}
}
func (a *Adapter) services(ctx context.Context, args []string) (any, error) {
	if len(args) < 2 {
		return nil, errors.New("service action and agent ID required")
	}
	c := jobContext(args[1], "service "+args[0], args[0] != "list" && args[0] != "status")
	switch args[0] {
	case "list":
		return a.fleet.ListServices(ctx, &devopsv1.ServiceListJobRequest{Context: c, Request: &devopsv1.ServiceListRequest{Limit: 200}})
	case "status", "start", "stop", "restart":
		if len(args) != 3 {
			return nil, errors.New("unit required")
		}
		r := &devopsv1.ServiceJobRequest{Context: c, Request: &devopsv1.ServiceRequest{Unit: args[2]}}
		switch args[0] {
		case "status":
			return a.fleet.GetService(ctx, r)
		case "start":
			return a.fleet.StartService(ctx, r)
		case "stop":
			return a.fleet.StopService(ctx, r)
		default:
			return a.fleet.RestartService(ctx, r)
		}
	}
	return nil, errors.New("invalid service action")
}
