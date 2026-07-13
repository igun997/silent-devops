package clientcli

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
func TestDashboardQuit(t *testing.T) {
	d := NewDashboard(dashboardAPI{}, true)
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit command missing")
	}
}
