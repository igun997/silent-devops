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
func TestDashboardQuit(t *testing.T) {
	d := NewDashboard(dashboardAPI{}, true)
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit command missing")
	}
}
