package easypanel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePanel is an in-memory EasyPanel tRPC/REST stand-in for unit tests.
type fakePanel struct {
	projects map[string][]Service // projectName -> services
	migrated []MigrateInput
	token    string
}

func newFakePanel(token string) *fakePanel {
	return &fakePanel{projects: map[string][]Service{}, token: token}
}

func (f *fakePanel) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/trpc/projects.listProjects", func(w http.ResponseWriter, r *http.Request) {
		// Queries must be GET; a POST would 405 on real panels.
		if r.Method != http.MethodGet {
			writeJSON(w, 405, map[string]any{"message": "Method Not Allowed"})
			return
		}
		out := []Project{}
		for name := range f.projects {
			out = append(out, Project{Name: name})
		}
		// Respond with the tRPC v10 wrapper to exercise unwrapPayload.
		writeJSON(w, 200, map[string]any{"result": map[string]any{"data": map[string]any{"json": out}}})
	})
	mux.HandleFunc("/api/trpc/projects.inspectProject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, 405, map[string]any{"message": "Method Not Allowed"})
			return
		}
		name := inputString(r, "projectName")
		svcs, ok := f.projects[name]
		if !ok {
			writeJSON(w, 404, map[string]any{"json": map[string]any{
				"code": "NOT_FOUND", "status": 404, "message": "Project not found."}})
			return
		}
		writeJSON(w, 200, map[string]any{"json": InspectResult{
			Project: Project{Name: name}, Services: svcs}})
	})
	mux.HandleFunc("/api/trpc/projects.createProject", func(w http.ResponseWriter, r *http.Request) {
		name := inputString(r, "name")
		if _, ok := f.projects[name]; !ok {
			f.projects[name] = []Service{}
		}
		writeJSON(w, 200, map[string]any{"json": map[string]any{}})
	})
	mux.HandleFunc("/api/migrate-service", func(w http.ResponseWriter, r *http.Request) {
		var in MigrateInput
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &in)
		f.migrated = append(f.migrated, in)
		writeJSON(w, 200, map[string]any{"success": true})
	})
	return mux
}

// inputString reads a tRPC input field from either the ?input= query param
// (GET queries) or the request body (POST mutations), mirroring the real panel.
func inputString(r *http.Request, key string) string {
	var env struct {
		JSON map[string]any `json:"json"`
	}
	if raw := r.URL.Query().Get("input"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &env)
	} else {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &env)
	}
	if v, ok := env.JSON[key].(string); ok {
		return v
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestProjectExists(t *testing.T) {
	f := newFakePanel("tok")
	f.projects["tests"] = []Service{{Name: "flux", Type: "app", ProjectName: "tests"}}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := New(srv.URL, "tok")

	ok, err := c.ProjectExists(context.Background(), "tests")
	if err != nil || !ok {
		t.Fatalf("tests should exist: ok=%v err=%v", ok, err)
	}
	ok, err = c.ProjectExists(context.Background(), "nope")
	if err != nil || ok {
		t.Fatalf("nope should be absent cleanly: ok=%v err=%v", ok, err)
	}
}

func TestInspectNotFoundClassified(t *testing.T) {
	f := newFakePanel("tok")
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	c := New(srv.URL, "tok")
	_, err := c.InspectProject(context.Background(), "ghost")
	if !NotFound(err) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestPreflightHappyPath(t *testing.T) {
	src := newFakePanel("s")
	src.projects["staging"] = []Service{{Name: "flux-be", Type: "app", ProjectName: "staging"}}
	dst := newFakePanel("d")
	dst.projects["tests"] = []Service{} // exists, empty
	ssrv := httptest.NewServer(src.handler())
	dsrv := httptest.NewServer(dst.handler())
	defer ssrv.Close()
	defer dsrv.Close()

	local := New(ssrv.URL, "s")
	remote := New(dsrv.URL, "d")
	in := MigrateInput{
		LocalProjectName: "staging", LocalServiceName: "flux-be",
		RemoteAPIToken: "d", RemoteEasypanelURL: dsrv.URL,
		RemoteProjectName: "tests", RemoteServiceName: "flux",
	}
	plan, err := Preflight(context.Background(), local, remote, in, PreflightOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CreatedProject {
		t.Fatal("should not create existing project")
	}
	if err := plan.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(src.migrated) != 1 || src.migrated[0].RemoteServiceName != "flux" {
		t.Fatalf("migrate not invoked on source: %+v", src.migrated)
	}
}

func TestPreflightMissingRemoteProjectFailsClosed(t *testing.T) {
	src := newFakePanel("s")
	src.projects["staging"] = []Service{{Name: "flux-be", ProjectName: "staging"}}
	dst := newFakePanel("d") // empty: no tests project
	ssrv := httptest.NewServer(src.handler())
	dsrv := httptest.NewServer(dst.handler())
	defer ssrv.Close()
	defer dsrv.Close()
	in := MigrateInput{
		LocalProjectName: "staging", LocalServiceName: "flux-be",
		RemoteProjectName: "tests", RemoteServiceName: "flux", RemoteEasypanelURL: dsrv.URL,
	}
	_, err := Preflight(context.Background(), New(ssrv.URL, "s"), New(dsrv.URL, "d"), in, PreflightOptions{})
	if err == nil || !strings.Contains(err.Error(), "remote project \"tests\" not found") {
		t.Fatalf("expected fail-closed on missing remote project, got %v", err)
	}
	if len(src.migrated) != 0 {
		t.Fatal("migrate must not run when preflight fails")
	}
}

func TestPreflightAutoCreatesRemoteProject(t *testing.T) {
	src := newFakePanel("s")
	src.projects["staging"] = []Service{{Name: "flux-be", ProjectName: "staging"}}
	dst := newFakePanel("d")
	ssrv := httptest.NewServer(src.handler())
	dsrv := httptest.NewServer(dst.handler())
	defer ssrv.Close()
	defer dsrv.Close()
	remote := New(dsrv.URL, "d")
	in := MigrateInput{
		LocalProjectName: "staging", LocalServiceName: "flux-be",
		RemoteProjectName: "tests", RemoteServiceName: "flux", RemoteEasypanelURL: dsrv.URL,
	}
	plan, err := Preflight(context.Background(), New(ssrv.URL, "s"), remote, in,
		PreflightOptions{CreateRemoteProject: true})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CreatedProject {
		t.Fatal("expected CreatedProject=true")
	}
	if ok, _ := remote.ProjectExists(context.Background(), "tests"); !ok {
		t.Fatal("remote project not created")
	}
}

func TestPreflightServiceCollision(t *testing.T) {
	src := newFakePanel("s")
	src.projects["staging"] = []Service{{Name: "flux-be", ProjectName: "staging"}}
	dst := newFakePanel("d")
	dst.projects["tests"] = []Service{{Name: "flux", ProjectName: "tests"}}
	ssrv := httptest.NewServer(src.handler())
	dsrv := httptest.NewServer(dst.handler())
	defer ssrv.Close()
	defer dsrv.Close()
	in := MigrateInput{
		LocalProjectName: "staging", LocalServiceName: "flux-be",
		RemoteProjectName: "tests", RemoteServiceName: "flux", RemoteEasypanelURL: dsrv.URL,
	}
	_, err := Preflight(context.Background(), New(ssrv.URL, "s"), New(dsrv.URL, "d"), in, PreflightOptions{})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected collision error, got %v", err)
	}
	// overwrite allowed -> ok
	if _, err := Preflight(context.Background(), New(ssrv.URL, "s"), New(dsrv.URL, "d"), in,
		PreflightOptions{OverwriteRemoteService: true}); err != nil {
		t.Fatalf("overwrite should pass: %v", err)
	}
}

// TestListProjectsUsesGETAndV10Envelope locks in that queries are sent as GET
// (POST would 405 on real panels) and that the tRPC v10 response wrapper
// {"result":{"data":{"json":...}}} is decoded.
func TestListProjectsUsesGETAndV10Envelope(t *testing.T) {
	f := newFakePanel("tok")
	f.projects["cds"] = []Service{{Name: "site", ProjectName: "cds"}}
	f.projects["ai"] = []Service{}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	got, err := New(srv.URL, "tok").ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	names := map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
	}
	if !names["cds"] || !names["ai"] || len(got) != 2 {
		t.Fatalf("projects=%v", got)
	}
}

// TestListProjectsFallsBackToPOSTOn405 covers panel builds that reject GET
// queries with 405 and require POST; the client must negotiate the method.
func TestListProjectsFallsBackToPOSTOn405(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/trpc/projects.listProjects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]any{"json": map[string]any{
				"code": "METHOD_NOT_SUPPORTED", "status": 405, "message": "Method Not Supported"}})
			return
		}
		writeJSON(w, 200, map[string]any{"json": []Project{{Name: "only-post"}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := New(srv.URL, "tok")
	got, err := c.ListProjects(context.Background())
	if err != nil || len(got) != 1 || got[0].Name != "only-post" {
		t.Fatalf("post-fallback failed: got=%v err=%v", got, err)
	}
	// Second call should reuse the negotiated POST method and still succeed.
	if _, err := c.ListProjects(context.Background()); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestPreferPOSTQueryByVersion(t *testing.T) {
	cases := map[string]bool{
		"2.30.1": false, "2.31.9": false, "2.32.0": true, "2.32.2": true,
		"3.0.0": true, "1.99.0": false, "v2.32.1": true, "": false, "garbage": false,
	}
	for v, want := range cases {
		if got := preferPOSTQuery(v); got != want {
			t.Errorf("preferPOSTQuery(%q)=%v want %v", v, got, want)
		}
	}
}

// TestNewForVersionPicksPOSTFirst verifies a 2.32.x panel is queried with POST
// on the first attempt (no GET round-trip). The fake 500s on GET so any GET
// attempt would surface as an error.
func TestNewForVersionPicksPOSTFirst(t *testing.T) {
	var sawGET bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/trpc/projects.listProjects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			sawGET = true
			writeJSON(w, 500, map[string]any{"json": map[string]any{"message": "should not GET"}})
			return
		}
		writeJSON(w, 200, map[string]any{"json": []Project{{Name: "ok"}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	got, err := NewForVersion(srv.URL, "tok", "2.32.2").ListProjects(context.Background())
	if err != nil || len(got) != 1 || got[0].Name != "ok" {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if sawGET {
		t.Fatal("2.32.x panel should be queried with POST first, not GET")
	}
}
