// Package easypanel talks to a local EasyPanel instance over its tRPC HTTP API
// and provides host-side detection plus API-token extraction so an agent can
// migrate ("snapshot/transfer") a service between two panels without any
// operator-supplied credentials.
//
// The tRPC surface used here was mapped against live EasyPanel servers. Per the
// tRPC HTTP contract, queries are GET with the input url-encoded in ?input=
// {"json":<input>} and mutations are POST with a body of {"json":<input>}.
// (Some builds tolerate POST for queries; others reject it with 405, so queries
// must use GET.) Responses come back either as tRPC v10 wrappers
// {"result":{"data":{"json":<payload>}}} or as a bare {"json":<payload>}; both
// are decoded here.
package easypanel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"
)

// Client is a minimal EasyPanel tRPC client scoped to the procedures needed for
// project/service inspection and service migration.
type Client struct {
	BaseURL string       // e.g. http://127.0.0.1:3000
	Token   string       // scoped API token (Bearer)
	HTTP    *http.Client // optional; defaults applied by New

	// queryMethod caches the HTTP method (GET or POST) this panel accepts for
	// tRPC queries, negotiated on first use via 405 fallback.
	queryMethod string
}

// New builds a Client with a bounded default HTTP client.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// NewForVersion builds a Client that uses the panel semver to pick the initial
// tRPC query HTTP method, avoiding a wasted 405 round-trip on the first query.
// Observed: EasyPanel 2.30.x uses GET queries (v10 response envelope); 2.32.x
// uses POST queries (bare envelope). The 405 fallback still corrects a wrong
// guess (e.g. an untested build at the boundary), so an empty/unparseable
// version simply defaults to GET-first.
func NewForVersion(baseURL, token, version string) *Client {
	c := New(baseURL, token)
	if preferPOSTQuery(version) {
		c.queryMethod = http.MethodPost
	}
	return c
}

// preferPOSTQuery reports whether the panel version wants POST for tRPC queries
// (true for >= 2.32.0). Unparseable versions return false (GET-first).
func preferPOSTQuery(version string) bool {
	major, minor, ok := parseMajorMinor(version)
	if !ok {
		return false
	}
	if major != 2 {
		return major > 2
	}
	return minor >= 32
}

// parseMajorMinor parses the leading "MAJOR.MINOR" of a semver string.
func parseMajorMinor(version string) (major, minor int, ok bool) {
	v := strings.TrimSpace(version)
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return maj, min, true
}

// Project is an EasyPanel project record.
type Project struct {
	Name string `json:"name"`
}

// Service is an EasyPanel service record within a project.
type Service struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	ProjectName string `json:"projectName"`
}

// APIError carries the EasyPanel error envelope for a non-2xx tRPC response.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("easypanel %d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("easypanel http %d", e.Status)
}

// NotFound reports whether err is an EasyPanel NOT_FOUND (missing project/service).
func NotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && (ae.Status == http.StatusNotFound || ae.Code == "NOT_FOUND")
}

// trpcQuery reads a tRPC query. Panel builds disagree on the HTTP method for
// queries: the tRPC HTTP contract uses GET (input url-encoded in ?input=), but
// some EasyPanel builds only accept POST and 405 on GET, while others 405 on
// POST. So it tries GET first and falls back to POST on a 405, and vice versa,
// caching the winning method on the client for subsequent calls.
func (c *Client) trpcQuery(ctx context.Context, proc string, input any, out any) error {
	if c.BaseURL == "" {
		return errors.New("easypanel: base url required")
	}
	first := c.queryMethod
	if first == "" {
		first = http.MethodGet
	}
	err := c.queryVia(ctx, first, proc, input, out)
	if !methodNotAllowed(err) {
		if err == nil {
			c.queryMethod = first
		}
		return err
	}
	alt := http.MethodPost
	if first == http.MethodPost {
		alt = http.MethodGet
	}
	if err := c.queryVia(ctx, alt, proc, input, out); err != nil {
		return err
	}
	c.queryMethod = alt
	return nil
}

// queryVia performs a single query attempt with the given HTTP method.
func (c *Client) queryVia(ctx context.Context, method, proc string, input any, out any) error {
	enc, err := json.Marshal(map[string]any{"json": input})
	if err != nil {
		return err
	}
	if method == http.MethodPost {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/api/trpc/"+proc, bytes.NewReader(enc))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		return c.do(req, proc, out)
	}
	url := c.BaseURL + "/api/trpc/" + proc + "?input=" + neturl.QueryEscape(string(enc))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return c.do(req, proc, out)
}

// methodNotAllowed reports whether err is an HTTP 405 from the panel.
func methodNotAllowed(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusMethodNotAllowed
}

// trpc issues a POST to /api/trpc/<proc> with a {"json": input} body (mutations
// are POST) and decodes the response payload into out. A nil out skips decoding.
func (c *Client) trpc(ctx context.Context, proc string, input any, out any) error {
	if c.BaseURL == "" {
		return errors.New("easypanel: base url required")
	}
	body, err := json.Marshal(map[string]any{"json": input})
	if err != nil {
		return err
	}
	url := c.BaseURL + "/api/trpc/" + proc
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, proc, out)
}

// do sends req (adding the bearer token), maps non-2xx to an APIError, and
// decodes the tRPC payload into out, accepting both the v10
// {"result":{"data":{"json":...}}} wrapper and a bare {"json":...} envelope.
func (c *Client) do(req *http.Request, proc string, out any) error {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	payload := unwrapPayload(raw)
	return json.Unmarshal(payload, out)
}

// unwrapPayload extracts the inner payload from either the tRPC v10 wrapper
// {"result":{"data":{"json":<payload>}}} or a bare {"json":<payload>}, falling
// back to the raw body for mutations that return a bare value.
func unwrapPayload(raw []byte) json.RawMessage {
	var v10 struct {
		Result struct {
			Data struct {
				JSON json.RawMessage `json:"json"`
			} `json:"data"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &v10) == nil && len(v10.Result.Data.JSON) > 0 {
		return v10.Result.Data.JSON
	}
	var bare struct {
		JSON json.RawMessage `json:"json"`
	}
	if json.Unmarshal(raw, &bare) == nil && len(bare.JSON) > 0 {
		return bare.JSON
	}
	return raw
}

func decodeAPIError(status int, raw []byte) error {
	// tRPC v10 error: {"error":{"json":{"code":"NOT_FOUND","data":{"httpStatus":404},"message":"..."}}}
	// legacy tRPC error: {"json":{"code":"NOT_FOUND","status":404,"message":"..."}}
	type trpcErr struct {
		Code    string `json:"code"`
		Status  int    `json:"status"`
		Message string `json:"message"`
		Data    struct {
			HTTPStatus int    `json:"httpStatus"`
			Code       string `json:"code"`
		} `json:"data"`
	}
	var env struct {
		Error struct {
			JSON trpcErr `json:"json"`
		} `json:"error"`
		JSON trpcErr `json:"json"`
		// REST-style: {"message":"...","success":false}
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &env)
	te := env.Error.JSON
	if te.Code == "" && te.Message == "" {
		te = env.JSON // legacy shape
	}
	code := te.Code
	if code == "" {
		code = te.Data.Code
	}
	ae := &APIError{Status: status, Code: code, Message: te.Message}
	if ae.Message == "" && env.Message != "" {
		ae.Message = env.Message
	}
	// REST-style {"error":"Not found"} — "error" is a string here, decoded
	// separately since the tRPC shape above uses "error" as an object.
	if ae.Message == "" {
		var restErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &restErr) == nil {
			ae.Message = restErr.Error
		}
	}
	if ae.Code == "" && status == http.StatusNotFound {
		ae.Code = "NOT_FOUND"
	}
	return ae
}

// ListProjects returns all projects visible to the token.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var out []Project
	if err := c.trpcQuery(ctx, "projects.listProjects", struct{}{}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// InspectResult is the payload of projects.inspectProject.
type InspectResult struct {
	Project  Project   `json:"project"`
	Services []Service `json:"services"`
}

// InspectProject returns a project's detail and services. A missing project
// surfaces as an APIError for which NotFound(err) is true.
func (c *Client) InspectProject(ctx context.Context, name string) (*InspectResult, error) {
	var out InspectResult
	if err := c.trpcQuery(ctx, "projects.inspectProject",
		map[string]string{"projectName": name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ProjectExists reports whether a project exists, translating NOT_FOUND to a
// clean (false, nil).
func (c *Client) ProjectExists(ctx context.Context, name string) (bool, error) {
	_, err := c.InspectProject(ctx, name)
	if err == nil {
		return true, nil
	}
	if NotFound(err) {
		return false, nil
	}
	return false, err
}

// CreateProject creates an empty project.
func (c *Client) CreateProject(ctx context.Context, name string) error {
	return c.trpc(ctx, "projects.createProject", map[string]string{"name": name}, nil)
}

// MigrateInput describes a service snapshot/transfer to a remote panel.
type MigrateInput struct {
	LocalProjectName   string `json:"localProjectName"`
	LocalServiceName   string `json:"localServiceName"`
	RemoteAPIToken     string `json:"remoteApiToken"`
	RemoteEasypanelURL string `json:"remoteEasypanelUrl"`
	RemoteProjectName  string `json:"remoteProjectName"`
	RemoteServiceName  string `json:"remoteServiceName"`
}

// Migrate invokes the REST /api/migrate-service endpoint on the source panel,
// which pushes the service definition to the remote panel.
func (c *Client) Migrate(ctx context.Context, in MigrateInput) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	url := c.BaseURL + "/api/migrate-service"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.StatusCode, raw)
	}
	// success envelope may be {"success":true} or {}
	var env struct {
		Success *bool `json:"success"`
	}
	_ = json.Unmarshal(raw, &env)
	if env.Success != nil && !*env.Success {
		return decodeAPIError(http.StatusBadGateway, raw)
	}
	return nil
}
