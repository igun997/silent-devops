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
		out := []Project{}
		for name := range f.projects {
			out = append(out, Project{Name: name})
		}
		writeJSON(w, 200, map[string]any{"json": out})
	})
	mux.HandleFunc("/api/trpc/projects.inspectProject", func(w http.ResponseWriter, r *http.Request) {
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

func inputString(r *http.Request, key string) string {
	var env struct {
		JSON map[string]any `json:"json"`
	}
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &env)
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
