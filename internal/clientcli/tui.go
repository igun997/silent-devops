package clientcli

import (
	"sort"
	"strings"

	devopsv1 "silent-devops/api/devops/v1"
)

type View int

const (
	ViewLogin View = iota
	ViewFleet
	ViewHost
	ViewMetrics
	ViewServices
	ViewLogs
	ViewActions
	ViewSSH
	ViewAudit
	ViewHelp
)

type Model struct {
	Role           devopsv1.Role
	NoColor        bool
	agents         []string
	filter, dialog string
}

func NewModel(role devopsv1.Role, noColor bool) *Model { return &Model{Role: role, NoColor: noColor} }
func (m *Model) CanView(view View) bool {
	if view == ViewAudit {
		return m.Role >= devopsv1.Role_ROLE_OPERATOR
	}
	if view == ViewActions || view == ViewSSH {
		return m.Role >= devopsv1.Role_ROLE_OPERATOR
	}
	return true
}
func (m *Model) SetAgents(agents []string) { m.agents = append([]string(nil), agents...) }
func (m *Model) Filter(value string)       { m.filter = strings.ToLower(value) }
func (m *Model) VisibleAgents() []string {
	var out []string
	for _, agent := range m.agents {
		if m.filter == "" || strings.Contains(strings.ToLower(agent), m.filter) {
			out = append(out, agent)
		}
	}
	sort.Strings(out)
	return out
}
func (m *Model) Confirm(action string) { m.dialog = action }
func (m *Model) DialogOpen() bool      { return m.dialog != "" }
func (m *Model) AcceptDialog() string  { action := m.dialog; m.dialog = ""; return action }
func (m *Model) CancelDialog()         { m.dialog = "" }
