package clientcli

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
)

type dashboardAPI struct{ err error }

func (a dashboardAPI) Call(context.Context, string, []string) (any, error) {
	if a.err != nil {
		return nil, a.err
	}
	return &devopsv1.ListAgentsResponse{Agents: []*devopsv1.Agent{{Id: "a1", Hostname: "alpha", Online: true}}}, nil
}
func TestDashboardLoadsNavigatesResizesAndShowsErrors(t *testing.T) {
	d := NewDashboard(dashboardAPI{}, true)
	d.Update(d.loadAgents()())
	d.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := d.View()
	for _, want := range []string{"Silent DevOps", "alpha", "online", "a1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in %q", want, view)
		}
	}
	d2 := NewDashboard(dashboardAPI{err: errors.New("offline")}, true)
	d2.Update(d2.Init()())
	if !strings.Contains(d2.View(), "offline") {
		t.Fatal(d2.View())
	}
}
func TestDashboardPermissionDeniedPreservesSession(t *testing.T) {
	d := NewDashboard(nil, true)
	d.Update(resultMsg{panel: 4, err: status.Error(codes.PermissionDenied, "authentication failed")})
	view := d.View()
	if !strings.Contains(view, "Access denied") || !strings.Contains(view, "Admin role required") || strings.Contains(view, "Session expired") {
		t.Fatalf("view=%q", view)
	}
}

func TestDashboardHandlesNilFleetResponse(t *testing.T) {
	d := NewDashboard(dashboardAPI{}, true)
	d.Update(resultMsg{panel: -1, value: (*devopsv1.ListAgentsResponse)(nil)})
	if !strings.Contains(d.View(), "No agents") {
		t.Fatal(d.View())
	}
}
func TestTablePanelSwitchNoPanic(t *testing.T) {
	d := NewDashboard(dashboardAPI{}, true)
	d.Update(d.loadAgents()())
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	d.loading = false
	// SSH Keys (3 cols) then Users (4 cols) then Audit (4 cols): switching
	// column count with stale rows must not panic in renderRow.
	d.panel = 5
	d.data = &devopsv1.ListSshKeysResponse{Keys: []*devopsv1.SshKey{{Id: "k1", UserId: "u1", Label: "laptop", PublicKey: []byte("ssh-ed25519 AAAA test")}}}
	d.fillTable()
	_ = d.View()
	d.panel = 4
	d.data = &devopsv1.ListUsersResponse{Users: []*devopsv1.User{{Id: "u1", Username: "live-admin", Role: devopsv1.Role_ROLE_ADMIN}}}
	d.fillTable()
	if v := d.View(); !strings.Contains(v, "live-admin") || !strings.Contains(v, "admin") {
		t.Fatalf("users table view=%q", v)
	}
	d.panel = 6
	d.data = &devopsv1.ListAuditResponse{Events: []*devopsv1.AuditEvent{{Id: "e1", ActorId: "u1", Action: "login", Reason: "cli", OccurredUnixMs: 1}}}
	d.fillTable()
	if v := d.View(); !strings.Contains(v, "login") {
		t.Fatalf("audit table view=%q", v)
	}
}
func TestTableSearchFiltersRows(t *testing.T) {
	d := NewDashboard(dashboardAPI{}, true)
	d.Update(d.loadAgents()())
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	d.loading = false
	d.panel = 4
	d.data = &devopsv1.ListUsersResponse{Users: []*devopsv1.User{
		{Id: "u1", Username: "live-admin", Role: devopsv1.Role_ROLE_ADMIN},
		{Id: "u2", Username: "bob", Role: devopsv1.Role_ROLE_VIEWER},
	}}
	d.fillTable()
	if got := len(d.table.Rows()); got != 2 {
		t.Fatalf("pre-filter rows=%d", got)
	}
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !d.searching {
		t.Fatal("search not opened by /")
	}
	for _, r := range []rune("bob") {
		d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.searching {
		t.Fatal("search still open after enter")
	}
	if got := len(d.table.Rows()); got != 1 {
		t.Fatalf("filtered rows=%d want 1", got)
	}
	if v := d.View(); !strings.Contains(v, "bob") || strings.Contains(v, "live-admin") {
		t.Fatalf("view=%q", v)
	}
	// Esc clears the filter and restores all rows.
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := len(d.table.Rows()); got != 2 {
		t.Fatalf("post-clear rows=%d want 2", got)
	}
}

// easypanelAPI answers agents + easypanel commands so the EasyPanel panel can
// be exercised end to end in the TUI.
type easypanelAPI struct{}

func (easypanelAPI) Call(_ context.Context, command string, args []string) (any, error) {
	switch command {
	case "agents":
		return &devopsv1.ListAgentsResponse{Agents: []*devopsv1.Agent{{Id: "a1", Hostname: "alpha", Online: true}}}, nil
	case "easypanel":
		if len(args) >= 2 && args[1] == "detect" {
			return map[string]any{"job_id": "j1", "output": "easypanel: detected container=easypanel.1.x\n"}, nil
		}
		return map[string]any{"job_id": "j2", "output": "tests\n"}, nil
	}
	return nil, nil
}

func TestDashboardEasypanelPanelShowsOutput(t *testing.T) {
	d := NewDashboard(easypanelAPI{}, true)
	d.Update(d.loadAgents()())
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	d.panel = easypanelPanel
	msg := d.loadEasypanel("a1")()
	d.Update(msg)
	view := d.View()
	for _, want := range []string{"EasyPanel", "detected container=easypanel.1.x", "tests"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in view:\n%s", want, view)
		}
	}
}

// captureAPI records the last easypanel call so migrate dispatch can be
// asserted.
type captureAPI struct {
	cmd  string
	args []string
}

func (c *captureAPI) Call(_ context.Context, command string, args []string) (any, error) {
	if command == "agents" {
		return &devopsv1.ListAgentsResponse{Agents: []*devopsv1.Agent{
			{Id: "src", Hostname: "alpha", Online: true},
			{Id: "dst", Hostname: "beta", Online: true},
		}}, nil
	}
	c.cmd, c.args = command, args
	return map[string]any{"job_id": "j1", "output": "migrate: ok\n"}, nil
}

func TestMigrateFormDispatchesCommand(t *testing.T) {
	api := &captureAPI{}
	d := NewDashboard(api, true)
	d.Update(d.loadAgents()())
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	d.panel = easypanelPanel
	// Open the migrate form on the EasyPanel panel.
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if !d.migrating || d.form == nil {
		t.Fatalf("migrate form did not open (migrating=%v)", d.migrating)
	}
	// Fill the four fields.
	for i, val := range []string{"staging", "flux-be", "tests", "flux"} {
		d.form.setFocus(i)
		for _, r := range val {
			d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}
	// Jump to submit, first Enter arms confirmation, second Enter dispatches.
	d.form.setFocus(mfSubmit)
	d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !d.form.confirm {
		t.Fatal("first enter should arm confirmation")
	}
	cmd := d.updateMigrate(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("second enter should dispatch")
	}
	d.Update(cmd())
	if api.cmd != "easypanel" {
		t.Fatalf("expected easypanel command, got %q", api.cmd)
	}
	joined := strings.Join(api.args, " ")
	for _, want := range []string{
		"src migrate --to-agent dst",
		"--local-project staging", "--local-service flux-be",
		"--remote-project tests", "--remote-service flux",
		"--create-remote-project",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %q", want, joined)
		}
	}
	if d.migrating {
		t.Fatal("form should close after dispatch")
	}
}

func TestMigrateFormValidationRequiresFields(t *testing.T) {
	api := &captureAPI{}
	d := NewDashboard(api, true)
	d.Update(d.loadAgents()())
	d.panel = easypanelPanel
	d.openMigrate()
	d.form.setFocus(mfSubmit)
	d.updateMigrate(tea.KeyMsg{Type: tea.KeyEnter})
	if d.err == nil {
		t.Fatal("expected validation error for empty fields")
	}
	if api.cmd == "easypanel" {
		t.Fatal("must not dispatch with empty fields")
	}
}

func TestDashboardQuit(t *testing.T) {
	d := NewDashboard(dashboardAPI{}, true)
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit command missing")
	}
}
