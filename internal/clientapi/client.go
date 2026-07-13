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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
	case "easypanel":
		// Sugar over admin exec: run the easypanel-migrate binary on the agent
		// host. args = [AGENT_ID, action, extra...].
		if len(args) < 2 {
			return nil, errors.New("agent ID and action required")
		}
		rest := args[1:]
		// `easypanel AGENT job JOB_ID` reports a dispatched job's status + output.
		// This keeps long/detached migrations observable on the validator.
		if rest[0] == "job" {
			if len(rest) < 2 {
				return nil, errors.New("job id required")
			}
			return a.jobStatus(ctx, rest[1])
		}
		isMigrate := rest[0] == "migrate"
		// Migrate is long-running: allow a configurable timeout (minutes, default
		// 30) and an optional --detach that returns the job id immediately.
		timeoutSec := uint32(30)
		detach := false
		if isMigrate {
			if mins, remain, ok := extractFlag(rest, "--timeout"); ok {
				m, perr := strconv.Atoi(strings.TrimSpace(mins))
				if perr != nil || m <= 0 {
					return nil, fmt.Errorf("--timeout must be a positive number of minutes")
				}
				timeoutSec = uint32(m * 60)
				rest = remain
			} else {
				timeoutSec = 30 * 60
			}
			if remain, ok := extractBoolFlag(rest, "--detach"); ok {
				detach = true
				rest = remain
			}
		}
		// Cross-agent migrate: --to-agent DST resolves the target panel URL+token
		// by running detect/token on DST, then feeds them to the migrate job on
		// the source agent (args[0]).
		if to, remain, ok := extractFlag(rest, "--to-agent"); ok {
			// Honor an explicit reachable URL; otherwise derive from the target
			// agent hostname. The token is always read from the target panel.
			url, remain2, hasURL := extractFlag(remain, "--remote-url")
			resolvedURL, tok, rerr := a.resolveRemotePanel(ctx, to, url)
			if rerr != nil {
				return nil, rerr
			}
			_ = hasURL
			rest = append(remain2, "--remote-url", resolvedURL, "--remote-token", tok)
		}
		cmd := append([]string{"easypanel-migrate"}, rest...)
		return a.execCaptureT(ctx, args[0], "easypanel "+rest[0], cmd, timeoutSec, detach)
	case "exec":
		return a.execCapture(ctx, args[0], "admin exec", args[1:])
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

// extractFlag removes "name VALUE" from args and returns the value plus the
// remaining args. ok is false when the flag is absent.
func extractFlag(args []string, name string) (value string, remaining []string, ok bool) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			value, ok = args[i+1], true
			i++
			continue
		}
		out = append(out, args[i])
	}
	return value, out, ok
}

// extractBoolFlag removes a valueless flag (e.g. --detach) from args, reporting
// whether it was present.
func extractBoolFlag(args []string, name string) (remaining []string, ok bool) {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == name {
			ok = true
			continue
		}
		out = append(out, a)
	}
	return out, ok
}

// jobStatus reports whether a dispatched job is still running or terminal, and
// includes captured output once terminal. There is no dedicated job-read RPC;
// StreamJobOutput returns FailedPrecondition ("still running") until the job is
// terminal, then returns the captured output, so we use that as the signal.
func (a *Adapter) jobStatus(ctx context.Context, id string) (any, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("job id required")
	}
	token, err := a.store.Load()
	if err != nil {
		return nil, errors.New("login required")
	}
	octx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	out, oerr := a.streamJobOutput(octx, id)
	if oerr == nil {
		return map[string]any{"job_id": id, "state": "terminal", "output": out}, nil
	}
	if status.Code(oerr) == codes.FailedPrecondition {
		return map[string]any{"job_id": id, "state": "running",
			"output": fmt.Sprintf("job %s is still running; re-run this command to poll", id)}, nil
	}
	return nil, oerr
}

// easypanelExec runs one easypanel-migrate action on an agent and returns its
// trimmed captured stdout.
func (a *Adapter) easypanelExec(ctx context.Context, agentID string, argv ...string) (string, error) {
	res, err := a.execCapture(ctx, agentID, "easypanel "+argv[0], append([]string{"easypanel-migrate"}, argv...))
	if err != nil {
		return "", err
	}
	m, _ := res.(map[string]any)
	if oe, ok := m["output_error"].(string); ok && oe != "" {
		return "", errors.New(oe)
	}
	out, _ := m["output"].(string)
	return strings.TrimSpace(out), nil
}

// resolveRemotePanel confirms the target agent runs EasyPanel and reads its API
// token from the target panel's LMDB store. The reachable base URL is either the
// operator-supplied explicitURL or derived from the target agent hostname (the
// panel's own detect reports a loopback URL the SOURCE host cannot reach).
func (a *Adapter) resolveRemotePanel(ctx context.Context, agentID, explicitURL string) (url, token string, err error) {
	det, err := a.easypanelExec(ctx, agentID, "detect")
	if err != nil {
		return "", "", fmt.Errorf("detect target panel: %w", err)
	}
	if parseDetectURL(det) == "" {
		return "", "", fmt.Errorf("target agent has no EasyPanel: %s", det)
	}
	url = strings.TrimSpace(explicitURL)
	if url == "" {
		// Prefer the target panel's OWN configured reachable URL (its
		// customPanelDomain over HTTPS, else serverIp:3000), which detect reads
		// from the panel's LMDB settings and prints as public_url=. This is the
		// address the panel serves itself on, so it is routable from the source
		// agent across networks.
		url = parseDetectField(det, "public_url=")
	}
	if url == "" {
		// Last resort: the target agent's reported hostname. This is mutable
		// metadata and is often NOT resolvable from the source agent (different
		// network / no shared DNS), so it only works co-located or with shared
		// DNS. Prefer public_url or an explicit --remote-url.
		host, herr := a.agentHostname(ctx, agentID)
		if herr != nil {
			return "", "", herr
		}
		url = "http://" + host + ":3000"
	}
	token, err = a.easypanelExec(ctx, agentID, "token")
	if err != nil {
		return "", "", fmt.Errorf("read target panel token: %w", err)
	}
	if token == "" {
		return "", "", errors.New("target panel token empty")
	}
	return url, token, nil
}

// agentHostname resolves an agent ID to its reported hostname via the fleet.
func (a *Adapter) agentHostname(ctx context.Context, agentID string) (string, error) {
	token, err := a.store.Load()
	if err != nil {
		return "", errors.New("login required")
	}
	lctx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	resp, err := a.fleet.ListAgents(lctx, &devopsv1.ListAgentsRequest{PageSize: 200})
	if err != nil {
		return "", err
	}
	for _, ag := range resp.GetAgents() {
		if ag.GetId() == agentID {
			if ag.GetHostname() == "" {
				return "", fmt.Errorf("target agent %s has no hostname; pass --remote-url", agentID)
			}
			return ag.GetHostname(), nil
		}
	}
	return "", fmt.Errorf("target agent %s not found", agentID)
}

// parseDetectURL extracts the url=... field from a detect line such as
// "easypanel: detected container=... url=http://127.0.0.1:3000".
func parseDetectURL(detect string) string {
	return parseDetectField(detect, "url=")
}

// parseDetectField returns the value of a `key=value` token in a detect line,
// or "" if absent or the literal "unknown".
func parseDetectField(detect, prefix string) string {
	for _, f := range strings.Fields(detect) {
		if strings.HasPrefix(f, prefix) {
			v := strings.TrimPrefix(f, prefix)
			if v == "unknown" {
				return ""
			}
			return v
		}
	}
	return ""
}

// execCapture dispatches an admin exec job on the agent and follows its
// captured stdout, returning a {job_id, output} (or output_error) result shared
// by the exec and easypanel commands. It uses the default short 30s job timeout
// and blocks until the job is terminal.
func (a *Adapter) execCapture(ctx context.Context, agentID, reason string, argv []string) (any, error) {
	return a.execCaptureT(ctx, agentID, reason, argv, 30, false)
}

// execCaptureT is execCapture with a configurable job timeout (seconds) and an
// optional detach mode. When detach is true it dispatches the job and returns
// immediately with the job_id (no output follow) so long-running work such as an
// EasyPanel migrate keeps running on the validator and stays observable via the
// `easypanel AGENT job JOB_ID` command. When detach is false it polls the
// captured output up to the job's own deadline (not a fixed short ceiling).
func (a *Adapter) execCaptureT(ctx context.Context, agentID, reason string, argv []string, timeoutSec uint32, detach bool) (any, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	job, err := a.fleet.Exec(ctx, &devopsv1.ExecJobRequest{
		Context: jobContextT(agentID, reason, true, timeoutSec),
		Request: &devopsv1.ArbitraryCommand{Command: strings.Join(argv, " "), CaptureOutput: true},
	})
	if err != nil {
		return nil, err
	}
	if detach {
		return map[string]any{"job_id": job.GetId(), "detached": true,
			"output": fmt.Sprintf("dispatched job %s (detached); check with: easypanel %s job %s",
				job.GetId(), agentID, job.GetId())}, nil
	}
	// Follow output up to the job deadline plus a small margin for the agent to
	// flush the final chunk.
	followCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		followCtx, cancel = context.WithTimeout(ctx,
			time.Duration(timeoutSec)*time.Second+10*time.Second)
		defer cancel()
	}
	out, oerr := a.jobOutputUntil(followCtx, job.GetId(),
		time.Duration(timeoutSec)*time.Second+10*time.Second)
	if oerr != nil {
		return map[string]any{"job_id": job.GetId(), "output": nil, "output_error": oerr.Error()}, nil
	}
	return map[string]any{"job_id": job.GetId(), "output": out}, nil
}

func (a *Adapter) JobOutput(ctx context.Context, id string) (string, error) {
	return a.jobOutputUntil(ctx, id, 40*time.Second)
}

// jobOutputUntil polls captured job output until the job is terminal or the
// given wait budget elapses. The server returns FailedPrecondition while the
// job is still running; we retry with a 500ms backoff up to the budget so
// long-running jobs (e.g. a large EasyPanel migrate) are not truncated by a
// fixed short ceiling.
func (a *Adapter) jobOutputUntil(ctx context.Context, id string, wait time.Duration) (string, error) {
	token, err := a.store.Load()
	if err != nil {
		return "", errors.New("login required")
	}
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	if wait <= 0 {
		wait = 40 * time.Second
	}
	deadline := time.Now().Add(wait)
	for {
		out, err := a.streamJobOutput(ctx, id)
		if err == nil {
			return out, nil
		}
		if status.Code(err) != codes.FailedPrecondition || time.Now().After(deadline) {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// streamJobOutput performs a single StreamJobOutput attempt and concatenates
// the captured chunks.
func (a *Adapter) streamJobOutput(ctx context.Context, id string) (string, error) {
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
	return jobContextT(agent, reason, confirmed, 30)
}

// jobContextT builds a job context with a configurable timeout (seconds). The
// agent kills the job when this deadline passes, so long-running operations
// must set a generous value.
func jobContextT(agent, reason string, confirmed bool, timeoutSec uint32) *devopsv1.JobRequestContext {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &devopsv1.JobRequestContext{AgentId: agent, Reason: reason, TimeoutSeconds: timeoutSec, IdempotencyKey: strconv.FormatInt(time.Now().UnixNano(), 36), Confirmed: confirmed}
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
