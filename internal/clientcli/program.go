package clientcli

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	devopsv1 "silent-devops/api/devops/v1"
)

type resultMsg struct {
	value any
	err   error
}
type Dashboard struct {
	API           API
	model         *Model
	width, height int
	selected      int
	agents        []*devopsv1.Agent
	loading       bool
	err           error
	noColor       bool
}

func NewDashboard(api API, noColor bool) *Dashboard {
	return &Dashboard{API: api, model: NewModel(devopsv1.Role_ROLE_ADMIN, noColor), loading: true, noColor: noColor}
}
func (d *Dashboard) Init() tea.Cmd { return d.loadAgents() }
func (d *Dashboard) loadAgents() tea.Cmd {
	return func() tea.Msg {
		v, e := d.API.Call(context.Background(), "agents", []string{"list"})
		return resultMsg{v, e}
	}
}
func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = m.Width, m.Height
	case resultMsg:
		d.loading = false
		d.err = m.err
		if r, ok := m.value.(*devopsv1.ListAgentsResponse); ok {
			d.agents = r.Agents
		}
	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c":
			return d, tea.Quit
		case "up", "k":
			if d.selected > 0 {
				d.selected--
			}
		case "down", "j":
			if d.selected+1 < len(d.agents) {
				d.selected++
			}
		case "r":
			d.loading = true
			return d, d.loadAgents()
		case "?":
			d.model.Confirm("↑/↓ navigate • r refresh • q quit")
		}
	}
	return d, nil
}
func (d *Dashboard) View() string {
	title := "Silent DevOps"
	if !d.noColor {
		title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")).Render(title)
	}
	var rows []string
	for i, a := range d.agents {
		mark := "  "
		if i == d.selected {
			mark = "> "
		}
		state := "offline"
		if a.Online {
			state = "online"
		}
		rows = append(rows, fmt.Sprintf("%s%-20s %-8s %s", mark, a.Hostname, state, a.Id))
	}
	body := strings.Join(rows, "\n")
	if d.loading {
		body = "Loading fleet…"
	}
	if d.err != nil {
		body = "Error: " + d.err.Error()
	}
	if body == "" {
		body = "No agents"
	}
	detail := "Select agent with ↑/↓"
	if len(d.agents) > 0 {
		a := d.agents[d.selected]
		detail = fmt.Sprintf("Host: %s\nID: %s\nOnline: %t\nLast seen: %d", a.Hostname, a.Id, a.Online, a.LastSeenUnixMs)
	}
	content := body + "\n\n" + detail
	if d.width >= 90 {
		content = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(48).Render(body), lipgloss.NewStyle().Width(d.width-50).Render(detail))
	}
	help := "↑/↓ navigate  r refresh  ? help  q quit"
	if d.model.DialogOpen() {
		help = d.model.AcceptDialog()
	}
	return title + "\n\n" + content + "\n\n" + help + "\n"
}
func RunTUI(api API, noColor bool) error {
	_, err := tea.NewProgram(NewDashboard(api, noColor), tea.WithAltScreen()).Run()
	return err
}
