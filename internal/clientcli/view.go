package clientcli

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	devopsv1 "silent-devops/api/devops/v1"
)

func (d *Dashboard) View() string {
	if d.login {
		return d.loginView()
	}
	header := d.headerView()
	tabs := d.tabsView()
	sidebar := d.th.sidebar.Render(d.sidebar())
	main := d.th.panel.Render(d.panelView())

	var content string
	if d.width >= 90 {
		content = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(34).Render(sidebar),
			lipgloss.NewStyle().Width(max(40, d.width-40)).Render(main),
		)
	} else {
		content = sidebar + "\n" + main
	}

	footer := d.help.View(d.keys)
	if d.err != nil {
		footer = d.th.errText.Render("Error: " + d.err.Error())
	}
	return header + "\n" + tabs + "\n\n" + content + "\n" + footer
}

func (d *Dashboard) headerView() string {
	online := 0
	for _, a := range d.agents {
		if a.Online {
			online++
		}
	}
	pill := func(label string) string { return d.th.statPill.Render(label) }
	stats := strings.Join([]string{
		pill(fmt.Sprintf("fleet %d", len(d.agents))),
		pill(fmt.Sprintf("online %d", online)),
		pill(fmt.Sprintf("offline %d", len(d.agents)-online)),
	}, " ")
	title := d.th.title.Render("● Silent DevOps")
	gap := d.width - lipgloss.Width(title) - lipgloss.Width(stats)
	if gap < 1 {
		gap = 1
	}
	return title + strings.Repeat(" ", gap) + stats
}

func (d *Dashboard) tabsView() string {
	cells := make([]string, len(panels))
	for i, p := range panels {
		if i == d.panel {
			cells[i] = d.th.tabActive.Render(p)
		} else {
			cells[i] = d.th.tab.Render(p)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, cells...)
}

func (d *Dashboard) sidebar() string {
	rows := []string{d.th.heading.Render("AGENTS"), ""}
	for i, a := range d.agents {
		dot := d.th.offDot.Render("○")
		if a.Online {
			dot = d.th.okDot.Render("●")
		}
		name := cut(a.Hostname, 20)
		if i == d.selected {
			rows = append(rows, d.th.cursor.Render("▸ ")+dot+" "+d.th.value.Render(name))
		} else {
			rows = append(rows, "  "+dot+" "+d.th.subtle.Render(name))
		}
	}
	if len(d.agents) == 0 {
		rows = append(rows, d.th.subtle.Render("No agents"))
	}
	return strings.Join(rows, "\n")
}

func (d *Dashboard) panelView() string {
	if d.loading {
		return d.spinner.View() + d.th.subtle.Render("loading…")
	}
	if len(d.agents) == 0 {
		return d.th.subtle.Render("No fleet data")
	}
	a := d.agents[d.selected]
	switch d.panel {
	case 0:
		return d.overview(a)
	case 1:
		return d.metrics()
	case 2, 3:
		head := d.th.heading.Render(strings.ToUpper(panels[d.panel]))
		pct := d.th.subtle.Render(fmt.Sprintf("%3.0f%%", d.viewport.ScrollPercent()*100))
		return head + "  " + pct + "\n\n" + d.viewport.View()
	case 4, 5, 6:
		head := d.th.heading.Render(strings.ToUpper(panels[d.panel]))
		count := d.th.subtle.Render(fmt.Sprintf("%d rows", len(d.table.Rows())))
		return head + "  " + count + "\n\n" + d.table.View()
	case 7:
		d.help.ShowAll = true
		return d.th.heading.Render("KEY BINDINGS") + "\n\n" + d.help.View(d.keys) +
			"\n\n" + d.th.subtle.Render("Live dashboard refreshes every 15 seconds.")
	default:
		return d.th.heading.Render(strings.ToUpper(panels[d.panel])) + "\n\n" + fmt.Sprint(d.data)
	}
}

func (d *Dashboard) overview(a *devopsv1.Agent) string {
	status := d.th.offDot.Render("offline")
	if a.Online {
		status = d.th.okDot.Render("online")
	}
	row := func(l, v string) string {
		return d.th.label.Render(fmt.Sprintf("%-10s", l)) + d.th.value.Render(v)
	}
	return d.th.heading.Render("HOST OVERVIEW") + "\n\n" +
		row("Hostname", a.Hostname) + "\n" +
		row("ID", a.Id) + "\n" +
		d.th.label.Render(fmt.Sprintf("%-10s", "Status")) + status + "\n" +
		row("Last seen", formatTime(a.LastSeenUnixMs))
}

func (d *Dashboard) metrics() string {
	r, ok := d.data.(*devopsv1.GetMetricsResponse)
	if !ok || len(r.Snapshots) == 0 {
		return d.th.heading.Render("METRICS") + "\n\n" + d.th.subtle.Render("Waiting for first sample…")
	}
	s := r.Snapshots[len(r.Snapshots)-1]
	v := map[string]float64{}
	for _, m := range s.Metrics {
		v[m.Name] = m.Value
	}
	cpu := percent(v["cpu_total_ticks"]-v["cpu_idle_ticks"], v["cpu_total_ticks"])
	mem := percent(v["memory_used_bytes"], v["memory_total_bytes"])
	row := func(l string, bar string, p float64) string {
		return d.th.label.Render(fmt.Sprintf("%-8s", l)) + bar + d.th.value.Render(fmt.Sprintf(" %5.1f%%", p))
	}
	return d.th.heading.Render("METRICS") + "  " + d.th.subtle.Render(formatTime(s.SampledUnixMs)) + "\n\n" +
		row("CPU", d.cpu.ViewAs(cpu/100), cpu) + "\n" +
		row("Memory", d.mem.ViewAs(mem/100), mem) + "\n\n" +
		d.th.label.Render(fmt.Sprintf("%-8s", "Load")) + d.th.value.Render(fmt.Sprintf("%.2f  %.2f  %.2f", v["load_1"], v["load_5"], v["load_15"])) + "\n" +
		d.th.label.Render(fmt.Sprintf("%-8s", "Uptime")) + d.th.value.Render(duration(v["uptime_seconds"]))
}

func (d *Dashboard) loginView() string {
	title := d.th.title.Render("● SILENT DEVOPS")
	prompt := d.th.subtle.Render("Session expired. Sign in to continue.")
	body := title + "\n\n" + prompt + "\n\n" +
		d.th.label.Render("Username") + "\n" + d.user.View() + "\n\n" +
		d.th.label.Render("Password") + "\n" + d.pass.View() + "\n\n" +
		d.th.footer.Render("tab field · enter connect · esc quit")
	w := max(48, d.width/2)
	card := d.th.panel.Width(w).Padding(1, 3).Render(body)
	if d.width > 0 && d.height > 0 {
		return lipgloss.Place(d.width, d.height, lipgloss.Center, lipgloss.Center, card)
	}
	return card
}

func percent(n, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Max(0, math.Min(100, n/total*100))
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
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
