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
func TestDashboardQuit(t *testing.T) {
	d := NewDashboard(dashboardAPI{}, true)
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit command missing")
	}
}
