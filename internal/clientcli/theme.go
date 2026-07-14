package clientcli

import "github.com/charmbracelet/lipgloss"

// theme centralizes the dashboard palette and reusable lip gloss styles so the
// whole TUI shares one visual language.
type theme struct {
	noColor bool

	base      lipgloss.Style
	title     lipgloss.Style
	subtle    lipgloss.Style
	statPill  lipgloss.Style
	tab       lipgloss.Style
	tabActive lipgloss.Style
	panel     lipgloss.Style
	sidebar   lipgloss.Style
	heading   lipgloss.Style
	footer    lipgloss.Style
	errText   lipgloss.Style
	okDot     lipgloss.Style
	offDot    lipgloss.Style
	cursor    lipgloss.Style
	label     lipgloss.Style
	value     lipgloss.Style
}

const (
	colAccent  = "42"  // green
	colAccent2 = "48"  // brighter green
	colSubtle  = "245" // grey
	colFaint   = "240" // border grey
	colError   = "203" // red
	colWarn    = "214" // amber
	colInk     = "231" // near white
	colBg      = "236" // dark panel fill
)

func newTheme(noColor bool) theme {
	if noColor {
		// Plain styles keep the same layout without any ANSI color so the
		// dashboard stays legible on dumb terminals and in test snapshots.
		plain := lipgloss.NewStyle()
		return theme{
			noColor:   true,
			base:      plain,
			title:     plain.Bold(true),
			subtle:    plain,
			statPill:  plain,
			tab:       plain,
			tabActive: plain.Bold(true),
			panel:     plain,
			sidebar:   plain,
			heading:   plain.Bold(true),
			footer:    plain,
			errText:   plain.Bold(true),
			okDot:     plain,
			offDot:    plain,
			cursor:    plain.Bold(true),
			label:     plain,
			value:     plain,
		}
	}
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }
	return theme{
		base:      lipgloss.NewStyle(),
		title:     lipgloss.NewStyle().Bold(true).Foreground(c(colAccent2)),
		subtle:    lipgloss.NewStyle().Foreground(c(colSubtle)),
		statPill:  lipgloss.NewStyle().Foreground(c(colInk)).Background(c(colFaint)).Padding(0, 1),
		tab:       lipgloss.NewStyle().Foreground(c(colSubtle)).Padding(0, 2),
		tabActive: lipgloss.NewStyle().Bold(true).Foreground(c(colInk)).Background(c(colAccent)).Padding(0, 2),
		panel:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c(colFaint)).Padding(0, 1),
		sidebar:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c(colFaint)).Padding(0, 1),
		heading:   lipgloss.NewStyle().Bold(true).Foreground(c(colSubtle)),
		footer:    lipgloss.NewStyle().Foreground(c(colSubtle)),
		errText:   lipgloss.NewStyle().Bold(true).Foreground(c(colError)),
		okDot:     lipgloss.NewStyle().Foreground(c(colAccent2)),
		offDot:    lipgloss.NewStyle().Foreground(c(colFaint)),
		cursor:    lipgloss.NewStyle().Bold(true).Foreground(c(colAccent2)),
		label:     lipgloss.NewStyle().Foreground(c(colSubtle)),
		value:     lipgloss.NewStyle().Foreground(c(colInk)),
	}
}
