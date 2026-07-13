package clientcli

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	devopsv1 "silent-devops/api/devops/v1"
)

type resultMsg struct {
	panel int
	value any
	err   error
}
type tickMsg time.Time

var panels = []string{"Overview", "Metrics", "Services", "Logs", "Users", "SSH Keys", "Audit", "Help"}

type Dashboard struct {
	API                            API
	width, height, selected, panel int
	agents                         []*devopsv1.Agent
	data                           any
	loading                        bool
	err                            error
	noColor                        bool
}

func NewDashboard(api API, noColor bool) *Dashboard {
	return &Dashboard{API: api, loading: true, noColor: noColor}
}
func (d *Dashboard) Init() tea.Cmd { return tea.Batch(d.loadAgents(), tick()) }
func tick() tea.Cmd                { return tea.Tick(15*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }
func (d *Dashboard) loadAgents() tea.Cmd {
	return func() tea.Msg {
		v, e := d.API.Call(context.Background(), "agents", []string{"list"})
		return resultMsg{panel: -1, value: v, err: e}
	}
}
func (d *Dashboard) loadPanel() tea.Cmd {
	if len(d.agents) == 0 {
		return nil
	}
	id := d.agents[d.selected].Id
	p := d.panel
	return func() tea.Msg {
		var c string
		var a []string
		switch p {
		case 1:
			c = "stats"
			a = []string{id}
		case 2:
			c = "services"
			a = []string{"list", id}
		case 3:
			c = "logs"
			a = []string{id, "silent-devops-agent.service"}
		case 4:
			c = "users"
			a = []string{"list"}
		case 5:
			c = "ssh-keys"
			a = []string{"list"}
		case 6:
			c = "audit"
		default:
			return resultMsg{panel: p}
		}
		v, e := d.API.Call(context.Background(), c, a)
		return resultMsg{panel: p, value: v, err: e}
	}
}
func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = m.Width, m.Height
	case tickMsg:
		return d, tea.Batch(d.loadAgents(), tick())
	case resultMsg:
		d.loading = false
		d.err = m.err
		if m.panel < 0 {
			if r, ok := m.value.(*devopsv1.ListAgentsResponse); ok && r != nil {
				d.agents = r.Agents
				if d.selected >= len(d.agents) {
					d.selected = 0
				}
			}
		} else if m.panel == d.panel {
			d.data = m.value
		}
	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c":
			return d, tea.Quit
		case "up", "k":
			if d.selected > 0 {
				d.selected--
				d.data = nil
				return d, d.loadPanel()
			}
		case "down", "j":
			if d.selected+1 < len(d.agents) {
				d.selected++
				d.data = nil
				return d, d.loadPanel()
			}
		case "tab", "right", "l":
			d.panel = (d.panel + 1) % len(panels)
			d.loading = true
			d.data = nil
			return d, d.loadPanel()
		case "shift+tab", "left", "h":
			d.panel = (d.panel + len(panels) - 1) % len(panels)
			d.loading = true
			d.data = nil
			return d, d.loadPanel()
		case "r":
			d.loading = true
			return d, tea.Batch(d.loadAgents(), d.loadPanel())
		}
	}
	return d, nil
}
func (d *Dashboard) View() string {
	title := d.style("Silent DevOps", true, "42")
	online := 0
	for _, a := range d.agents {
		if a.Online {
			online++
		}
	}
	header := fmt.Sprintf("%s  fleet %d  online %d  offline %d", title, len(d.agents), online, len(d.agents)-online)
	tabs := make([]string, len(panels))
	for i, p := range panels {
		if i == d.panel {
			tabs[i] = d.style("["+p+"]", true, "42")
		} else {
			tabs[i] = " " + p + " "
		}
	}
	sidebar := d.sidebar()
	main := d.panelView()
	content := sidebar + "\n\n" + main
	if d.width >= 90 {
		content = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(36).Render(sidebar), lipgloss.NewStyle().Width(max(40, d.width-38)).Render(main))
	}
	status := "Tab panels  ↑/↓ agents  r refresh  q quit"
	if d.err != nil {
		status = "Error: " + d.err.Error()
	}
	return header + "\n" + strings.Join(tabs, " ") + "\n\n" + content + "\n\n" + status + "\n"
}
func (d *Dashboard) sidebar() string {
	rows := []string{d.style("AGENTS", true, "245")}
	for i, a := range d.agents {
		mark := " "
		if i == d.selected {
			mark = ">"
		}
		state := "○"
		if a.Online {
			state = "●"
		}
		rows = append(rows, fmt.Sprintf("%s %s %-20s", mark, state, cut(a.Hostname, 20)))
	}
	if len(d.agents) == 0 {
		rows = append(rows, "No agents")
	}
	return strings.Join(rows, "\n")
}
func (d *Dashboard) panelView() string {
	if d.loading {
		return "Loading…"
	}
	if len(d.agents) == 0 {
		return "No fleet data"
	}
	a := d.agents[d.selected]
	switch d.panel {
	case 0:
		return fmt.Sprintf("%s\n\nHostname  %s\nID        %s\nStatus    %s\nLast seen %s", d.style("HOST OVERVIEW", true, "245"), a.Hostname, a.Id, map[bool]string{true: "online", false: "offline"}[a.Online], formatTime(a.LastSeenUnixMs))
	case 1:
		return d.metrics()
	case 7:
		return "KEYBOARD\n\nTab / Shift+Tab  change panel\n↑ ↓ or j k        select agent\nr                 refresh\nq                 quit\n\nLive dashboard refreshes every 15 seconds."
	default:
		return d.style(strings.ToUpper(panels[d.panel]), true, "245") + "\n\n" + fmt.Sprint(d.data)
	}
}
func (d *Dashboard) metrics() string {
	r, ok := d.data.(*devopsv1.GetMetricsResponse)
	if !ok || len(r.Snapshots) == 0 {
		return "METRICS\n\nWaiting for first sample…"
	}
	s := r.Snapshots[len(r.Snapshots)-1]
	v := map[string]float64{}
	for _, m := range s.Metrics {
		v[m.Name] = m.Value
	}
	cpu := percent(v["cpu_total_ticks"]-v["cpu_idle_ticks"], v["cpu_total_ticks"])
	mem := percent(v["memory_used_bytes"], v["memory_total_bytes"])
	return fmt.Sprintf("METRICS  %s\n\nCPU     %s %5.1f%%\nMemory  %s %5.1f%%\nLoad    %.2f  %.2f  %.2f\nUptime  %s", formatTime(s.SampledUnixMs), bar(cpu, 24), cpu, bar(mem, 24), mem, v["load_1"], v["load_5"], v["load_15"], duration(v["uptime_seconds"]))
}
func (d *Dashboard) style(s string, bold bool, color string) string {
	if d.noColor {
		return s
	}
	return lipgloss.NewStyle().Bold(bold).Foreground(lipgloss.Color(color)).Render(s)
}
func RunTUI(api API, noColor bool) error {
	_, err := tea.NewProgram(NewDashboard(api, noColor), tea.WithAltScreen()).Run()
	return err
}
func percent(n, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Max(0, math.Min(100, n/total*100))
}
func bar(p float64, w int) string {
	n := int(math.Round(p / 100 * float64(w)))
	return "[" + strings.Repeat("█", n) + strings.Repeat("░", w-n) + "]"
}
func formatTime(ms int64) string {
	if ms <= 0 {
		return "never"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}
func duration(seconds float64) string {
	return (time.Duration(seconds) * time.Second).Round(time.Minute).String()
}
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
