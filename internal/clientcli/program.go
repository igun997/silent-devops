package clientcli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

var panels = []string{"Overview", "Metrics", "Services", "Logs", "Users", "SSH Keys", "Audit", "EasyPanel", "Help"}

const easypanelPanel = 7

// scrollPanel reports whether the given panel routes keys to the viewport.
func scrollPanel(p int) bool { return p == 2 || p == 3 || p == easypanelPanel }

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
	th                             theme
	keys                           keyMap
	help                           help.Model
	spinner                        spinner.Model
	cpu                            progress.Model
	mem                            progress.Model
	viewport                       viewport.Model
	table                          table.Model
	user                           textinput.Model
	pass                           textinput.Model
	search                         textinput.Model
	searching                      bool
	filter                         string
	migrating                      bool
	form                           *migrateForm
}

func NewDashboard(api API, noColor bool) *Dashboard {
	th := newTheme(noColor)
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	if !noColor {
		sp.Style = th.title
	}
	barOpts := []progress.Option{progress.WithoutPercentage()}
	if noColor {
		barOpts = append(barOpts, progress.WithSolidFill(colInk))
	} else {
		barOpts = append(barOpts, progress.WithScaledGradient("#00d75f", "#5fffaf"))
	}
	hlp := help.New()
	hlp.ShowAll = false

	user := textinput.New()
	user.Placeholder = "username"
	user.Prompt = "› "
	user.CharLimit = 64
	user.Focus()
	pass := textinput.New()
	pass.Placeholder = "password"
	pass.Prompt = "› "
	pass.CharLimit = 128
	pass.EchoMode = textinput.EchoPassword
	pass.EchoCharacter = '•'
	search := textinput.New()
	search.Placeholder = "filter…"
	search.Prompt = ""
	search.CharLimit = 64

	d := &Dashboard{
		API:      api,
		loading:  true,
		noColor:  noColor,
		th:       th,
		keys:     defaultKeys(),
		help:     hlp,
		spinner:  sp,
		cpu:      progress.New(barOpts...),
		mem:      progress.New(barOpts...),
		viewport: viewport.New(80, 20),
		user:     user,
		pass:     pass,
		search:   search,
	}
	d.table = d.newTable()
	return d
}

func (d *Dashboard) Init() tea.Cmd { return tea.Batch(d.loadAgents(), tick(), d.spinner.Tick) }
func tick() tea.Cmd {
	return tea.Tick(15*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

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
	if p == easypanelPanel {
		return d.loadEasypanel(id)
	}
	return func() tea.Msg {
		var c string
		var a []string
		switch p {
		case 1:
			c, a = "stats", []string{id}
		case 2:
			c, a = "services", []string{"list", id}
		case 3:
			c, a = "logs", []string{id, "silent-devops-agent.service"}
		case 4:
			c, a = "users", []string{"list"}
		case 5:
			c, a = "ssh-keys", []string{"list"}
		case 6:
			c = "audit"
		default:
			return resultMsg{panel: p}
		}
		v, e := d.API.Call(context.Background(), c, a)
		if e == nil && scrollPanel(p) {
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

// loadEasypanel runs the read-only easypanel detect + projects actions on the
// selected agent and renders their captured output in the EasyPanel panel.
func (d *Dashboard) loadEasypanel(id string) tea.Cmd {
	return func() tea.Msg {
		var b strings.Builder
		for _, action := range []string{"detect", "projects"} {
			v, e := d.API.Call(context.Background(), "easypanel", []string{id, action})
			if e != nil {
				return resultMsg{panel: easypanelPanel, err: e}
			}
			b.WriteString("$ easypanel-migrate " + action + "\n")
			b.WriteString(easypanelOutput(v))
			b.WriteString("\n")
		}
		return resultMsg{panel: easypanelPanel, value: b.String()}
	}
}

// easypanelOutput pulls the captured stdout (or error) from an easypanel result
// map produced by the client adapter.
func easypanelOutput(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Sprint(v)
	}
	if oe, ok := m["output_error"].(string); ok && oe != "" {
		return "error: " + oe + "\n"
	}
	if out, ok := m["output"].(string); ok {
		if out == "" {
			return "(no output)\n"
		}
		return out
	}
	return fmt.Sprint(v)
}

func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = m.Width, m.Height
		d.viewport.Width = max(20, m.Width-46)
		d.viewport.Height = max(5, m.Height-12)
		barW := max(10, min(30, m.Width-60))
		d.cpu.Width, d.mem.Width = barW, barW
		d.help.Width = m.Width
	case spinner.TickMsg:
		var cmd tea.Cmd
		d.spinner, cmd = d.spinner.Update(m)
		return d, cmd
	case tickMsg:
		return d, tea.Batch(d.loadAgents(), tick())
	case resultMsg:
		d.loading = false
		d.err = m.err
		if status.Code(m.err) == codes.Unauthenticated {
			d.login = true
			d.loginField = 0
			d.user.Focus()
			d.pass.Blur()
			d.pass.SetValue("")
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
			if tablePanel(d.panel) {
				d.fillTable()
			}
		}
	case tea.KeyMsg:
		if d.login {
			return d, d.updateLogin(m)
		}
		if d.migrating {
			return d, d.updateMigrate(m)
		}
		if d.searching {
			return d, d.updateSearch(m)
		}
		return d.handleKey(m)
	}
	return d, nil
}

// updateSearch drives the table filter input while search mode is active.
func (d *Dashboard) updateSearch(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		d.searching = false
		d.search.SetValue("")
		d.filter = ""
		d.search.Blur()
		d.fillTable()
		return nil
	case "enter":
		d.searching = false
		d.filter = strings.TrimSpace(d.search.Value())
		d.search.Blur()
		d.fillTable()
		return nil
	}
	var cmd tea.Cmd
	d.search, cmd = d.search.Update(k)
	d.filter = strings.TrimSpace(d.search.Value())
	d.fillTable()
	return cmd
}

func (d *Dashboard) handleKey(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	// On tabular panels arrow/j/k move the table cursor (navigate inside
	// content). Use [/] to change the selected agent there instead.
	if tablePanel(d.panel) && (key.Matches(m, d.keys.Up) || key.Matches(m, d.keys.Down)) {
		var cmd tea.Cmd
		d.table, cmd = d.table.Update(m)
		return d, cmd
	}
	switch {
	case key.Matches(m, d.keys.Quit):
		return d, tea.Quit
	case key.Matches(m, d.keys.AgentPrev):
		if d.selected > 0 {
			d.selected--
			d.data = nil
			return d, d.loadPanel()
		}
	case key.Matches(m, d.keys.AgentNext):
		if d.selected+1 < len(d.agents) {
			d.selected++
			d.data = nil
			return d, d.loadPanel()
		}
	case key.Matches(m, d.keys.Up):
		if !tablePanel(d.panel) && d.selected > 0 {
			d.selected--
			d.data = nil
			return d, d.loadPanel()
		}
	case key.Matches(m, d.keys.Down):
		if !tablePanel(d.panel) && d.selected+1 < len(d.agents) {
			d.selected++
			d.data = nil
			return d, d.loadPanel()
		}
	case key.Matches(m, d.keys.Next):
		d.panel = (d.panel + 1) % len(panels)
		d.loading, d.data = true, nil
		return d, d.loadPanel()
	case key.Matches(m, d.keys.Prev):
		d.panel = (d.panel + len(panels) - 1) % len(panels)
		d.loading, d.data = true, nil
		return d, d.loadPanel()
	case key.Matches(m, d.keys.Refresh):
		d.loading = true
		return d, tea.Batch(d.loadAgents(), d.loadPanel())
	case key.Matches(m, d.keys.SSH):
		return d.startSSH()
	case key.Matches(m, d.keys.Migrate):
		if d.panel == easypanelPanel {
			d.openMigrate()
			return d, nil
		}
	}
	if tablePanel(d.panel) && m.String() == "/" {
		d.searching = true
		d.search.SetValue(d.filter)
		d.search.CursorEnd()
		d.search.Focus()
		return d, nil
	}
	if scrollPanel(d.panel) {
		switch m.String() {
		case "pgup":
			d.viewport.HalfViewUp()
		case "pgdown":
			d.viewport.HalfViewDown()
		case "g":
			d.viewport.GotoTop()
		case "G":
			d.viewport.GotoBottom()
		}
	}
	if tablePanel(d.panel) {
		var cmd tea.Cmd
		d.table, cmd = d.table.Update(m)
		return d, cmd
	}
	return d, nil
}

func (d *Dashboard) startSSH() (tea.Model, tea.Cmd) {
	if len(d.agents) == 0 {
		return d, nil
	}
	launch, err := PrepareNativeSSH(context.Background(), d.API, d.agents[d.selected].Id)
	if err != nil {
		d.err = err
		return d, nil
	}
	return d, tea.ExecProcess(launch.Command, func(err error) tea.Msg {
		launch.Cleanup()
		return resultMsg{panel: d.panel, err: err}
	})
}

func (d *Dashboard) updateLogin(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "ctrl+c", "esc":
		return tea.Quit
	case "tab", "down", "shift+tab", "up":
		d.loginField = (d.loginField + 1) % 2
		if d.loginField == 0 {
			d.user.Focus()
			d.pass.Blur()
		} else {
			d.pass.Focus()
			d.user.Blur()
		}
		return nil
	case "enter":
		if d.user.Value() != "" && d.pass.Value() != "" {
			u, p := d.user.Value(), d.pass.Value()
			d.loading = true
			return func() tea.Msg {
				if _, e := d.API.Call(context.Background(), "login", []string{u, p}); e != nil {
					return resultMsg{panel: -1, err: e}
				}
				d.login = false
				d.pass.SetValue("")
				v, e := d.API.Call(context.Background(), "agents", []string{"list"})
				return resultMsg{panel: -1, value: v, err: e}
			}
		}
		return nil
	}
	var cmd tea.Cmd
	if d.loginField == 0 {
		d.user, cmd = d.user.Update(k)
	} else {
		d.pass, cmd = d.pass.Update(k)
	}
	return cmd
}

func RunTUI(api API, noColor bool) error {
	_, err := tea.NewProgram(NewDashboard(api, noColor), tea.WithAltScreen()).Run()
	return err
}
