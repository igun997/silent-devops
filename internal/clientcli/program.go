package clientcli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	login                          bool
	loginField                     int
	username, password             string
	viewport                       viewport.Model
}

func NewDashboard(api API, noColor bool) *Dashboard {
	v := viewport.New(80, 20)
	return &Dashboard{API: api, loading: true, noColor: noColor, viewport: v}
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
		if e == nil && (p == 2 || p == 3) {
			if job, ok := v.(*devopsv1.Job); ok {
				if outputAPI, ok := d.API.(interface {
					JobOutput(context.Context, string) (string, error)
				}); ok {
					for i := 0; i < 50; i++ {
						text, oe := outputAPI.JobOutput(context.Background(), job.Id)
						if oe == nil {
							return resultMsg{panel: p, value: text}
						}
						time.Sleep(100 * time.Millisecond)
					}
					return resultMsg{panel: p, err: errors.New("job output unavailable")}
				}
			}
		}
		return resultMsg{panel: p, value: v, err: e}
	}
}
func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = m.Width, m.Height
		d.viewport.Width, d.viewport.Height = max(20, m.Width-42), max(5, m.Height-10)
	case tickMsg:
		return d, tea.Batch(d.loadAgents(), tick())
	case resultMsg:
		d.loading = false
		d.err = m.err
		if status.Code(m.err) == codes.Unauthenticated {
			d.login = true
			d.password = ""
		} else if status.Code(m.err) == codes.PermissionDenied {
			d.err = errors.New("Access denied: Admin role required")
		}
		if m.panel < 0 {
			if r, ok := m.value.(*devopsv1.ListAgentsResponse); ok && r != nil {
				d.agents = r.Agents
				if d.selected >= len(d.agents) {
					d.selected = 0
				}
			}
		} else if m.panel == d.panel {
			d.data = m.value
			if text, ok := m.value.(string); ok {
				d.viewport.SetContent(text)
				d.viewport.GotoTop()
			}
		}
	case tea.KeyMsg:
		if d.login {
			return d, d.updateLogin(m)
		}
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
		case "pgup":
			if d.panel == 2 || d.panel == 3 {
				d.viewport.HalfViewUp()
				return d, nil
			}
		case "pgdown":
			if d.panel == 2 || d.panel == 3 {
				d.viewport.HalfViewDown()
				return d, nil
			}
		case "g":
			if d.panel == 2 || d.panel == 3 {
				d.viewport.GotoTop()
				return d, nil
			}
		case "G":
			if d.panel == 2 || d.panel == 3 {
				d.viewport.GotoBottom()
				return d, nil
			}
		case "s":
			if len(d.agents) == 0 {
				return d, nil
			}
			launch, err := PrepareNativeSSH(context.Background(), d.API, d.agents[d.selected].Id)
			if err != nil {
				d.err = err
				return d, nil
			}
			return d, tea.ExecProcess(launch.Command, func(err error) tea.Msg { launch.Cleanup(); return resultMsg{panel: d.panel, err: err} })
		}
	}
	return d, nil
}
func (d *Dashboard) updateLogin(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "ctrl+c", "esc":
		return tea.Quit
	case "tab", "down", "shift+tab", "up":
		d.loginField = (d.loginField + 1) % 2
	case "backspace":
		if d.loginField == 0 && len(d.username) > 0 {
			d.username = d.username[:len(d.username)-1]
		}
		if d.loginField == 1 && len(d.password) > 0 {
			d.password = d.password[:len(d.password)-1]
		}
	case "enter":
		if d.username != "" && d.password != "" {
			u, p := d.username, d.password
			d.loading = true
			return func() tea.Msg {
				_, e := d.API.Call(context.Background(), "login", []string{u, p})
				if e != nil {
					return resultMsg{panel: -1, err: e}
				}
				d.login = false
				d.password = ""
				v, e := d.API.Call(context.Background(), "agents", []string{"list"})
				return resultMsg{panel: -1, value: v, err: e}
			}
		}
	default:
		if len(k.Runes) > 0 {
			if d.loginField == 0 {
				d.username += string(k.Runes)
			} else {
				d.password += string(k.Runes)
			}
		}
	}
	return nil
}
func (d *Dashboard) View() string {
	if d.login {
		return d.loginView()
	}
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
	status := "Tab panels  ↑/↓ agents  s SSH  r refresh  q quit"
	if d.err != nil {
		status = "Error: " + d.err.Error()
	}
	inner := header + "\n" + strings.Join(tabs, " ") + "\n\n" + content + "\n\n" + status
	w, h := d.width-4, d.height-2
	if w < 50 {
		w = 50
	}
	if h < 14 {
		h = 14
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w).Height(h).Padding(0, 1).Render(inner)
}
func (d *Dashboard) loginView() string {
	user, pass := d.username, strings.Repeat("•", len(d.password))
	um, pm := " ", " "
	if d.loginField == 0 {
		um = ">"
	} else {
		pm = ">"
	}
	body := fmt.Sprintf("%s\n\nSession expired. Sign in to continue.\n\n%s Username  %s\n%s Password  %s\n\nTab field  Enter connect  Esc quit", d.style("SILENT DEVOPS", true, "42"), um, user, pm, pass)
	w := d.width - 8
	if w < 48 {
		w = 48
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w).Padding(2, 3).Render(body)
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
	case 2, 3:
		return d.style(strings.ToUpper(panels[d.panel]), true, "245") + fmt.Sprintf("  %3.0f%%\n\n", d.viewport.ScrollPercent()*100) + d.viewport.View()
	case 7:
		return "KEYBOARD\n\nTab / Shift+Tab  change panel\n↑ ↓ or j k        select agent\nPgUp/PgDn, g/G    scroll output\nr                 refresh\ns                 native SSH\nq                 quit\n\nLive dashboard refreshes every 15 seconds."
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
