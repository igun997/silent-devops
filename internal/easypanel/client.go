// Package easypanel talks to a local EasyPanel instance over its tRPC HTTP API
// and provides host-side detection plus API-token extraction so an agent can
// migrate ("snapshot/transfer") a service between two panels without any
// operator-supplied credentials.
//
// The tRPC surface used here was mapped against a live EasyPanel 2.32.x server.
// All procedures are POST with a JSON envelope {"json": <input>}. Queries and
// mutations share that shape. Responses wrap payloads as {"json": <payload>}.
package easypanel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal EasyPanel tRPC client scoped to the procedures needed for
// project/service inspection and service migration.
type Client struct {
	BaseURL string       // e.g. http://127.0.0.1:3000
	Token   string       // scoped API token (Bearer)
	HTTP    *http.Client // optional; defaults applied by New
}

// New builds a Client with a bounded default HTTP client.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
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

// trpc issues a POST to /api/trpc/<proc> with {"json": input} and decodes the
// {"json": ...} response payload into out. A nil out skips decoding.
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
	var env struct {
		JSON json.RawMessage `json:"json"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("easypanel: decode %s: %w", proc, err)
	}
	payload := env.JSON
	if len(payload) == 0 {
		payload = raw // some mutations return a bare value
	}
	return json.Unmarshal(payload, out)
}

func decodeAPIError(status int, raw []byte) error {
	// tRPC error: {"json":{"code":"NOT_FOUND","status":404,"message":"..."}}
	var env struct {
		JSON struct {
			Code    string `json:"code"`
			Status  int    `json:"status"`
			Message string `json:"message"`
		} `json:"json"`
		// REST-style: {"message":"...","success":false} or {"error":"Not found"}
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal(raw, &env)
	ae := &APIError{Status: status, Code: env.JSON.Code, Message: env.JSON.Message}
	if ae.Message == "" {
		if env.Message != "" {
			ae.Message = env.Message
		} else if env.Error != "" {
			ae.Message = env.Error
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
	if err := c.trpc(ctx, "projects.listProjects", struct{}{}, &out); err != nil {
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
	if err := c.trpc(ctx, "projects.inspectProject",
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
