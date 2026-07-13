package clientcli

import "github.com/charmbracelet/bubbles/key"

// keyMap defines the dashboard bindings and feeds the bubbles help component so
// the footer legend stays in sync with the actual handlers.
type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	AgentPrev key.Binding
	AgentNext key.Binding
	Next      key.Binding
	Prev      key.Binding
	Scroll    key.Binding
	Search    key.Binding
	SSH       key.Binding
	Refresh   key.Binding
	Quit      key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		AgentPrev: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "agent prev")),
		AgentNext: key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "agent next")),
		Next:      key.NewBinding(key.WithKeys("tab", "right", "l"), key.WithHelp("tab", "next panel")),
		Prev:      key.NewBinding(key.WithKeys("shift+tab", "left", "h"), key.WithHelp("shift+tab", "prev panel")),
		Scroll:    key.NewBinding(key.WithKeys("pgup", "pgdown", "g", "G"), key.WithHelp("pgup/pgdn", "scroll")),
		Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		SSH:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "ssh")),
		Refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Next, k.Up, k.Down, k.Search, k.SSH, k.Refresh, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Next, k.Prev},
		{k.Up, k.Down},
		{k.AgentPrev, k.AgentNext},
		{k.Scroll, k.Search},
		{k.SSH, k.Refresh, k.Quit},
	}
}
