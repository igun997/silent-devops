package clientcli

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	devopsv1 "silent-devops/api/devops/v1"
)

// tablePanel reports whether the panel renders a bubbles table.
func tablePanel(p int) bool { return p == 4 || p == 5 || p == 6 }

// newTable builds a styled, scrollable bubbles table used by the tabular panels.
func (d *Dashboard) newTable() table.Model {
	t := table.New(table.WithFocused(true))
	st := table.DefaultStyles()
	if d.noColor {
		st.Header = lipgloss.NewStyle().Bold(true)
		st.Selected = lipgloss.NewStyle()
	} else {
		st.Header = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colSubtle)).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colFaint)).
			BorderBottom(true)
		st.Cell = lipgloss.NewStyle().Foreground(lipgloss.Color(colInk))
		st.Selected = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colInk)).
			Background(lipgloss.Color(colAccent))
	}
	t.SetStyles(st)
	return t
}

// fillTable populates the dashboard table for the active tabular panel from the
// current response payload, choosing columns per panel.
func (d *Dashboard) fillTable() {
	w := max(30, d.viewport.Width)
	var cols []table.Column
	var rows []table.Row
	switch d.panel {
	case 4:
		cols = []table.Column{
			{Title: "Username", Width: colw(w, 0.35, 12)},
			{Title: "Role", Width: colw(w, 0.25, 8)},
			{Title: "Status", Width: colw(w, 0.2, 8)},
			{Title: "ID", Width: colw(w, 0.2, 8)},
		}
		if r, ok := d.data.(*devopsv1.ListUsersResponse); ok {
			for _, u := range r.Users {
				rows = append(rows, table.Row{u.Username, roleLabel(u.Role), enabledLabel(u.Disabled), cut(u.Id, 12)})
			}
		}
	case 5:
		cols = []table.Column{
			{Title: "Label", Width: colw(w, 0.3, 10)},
			{Title: "User", Width: colw(w, 0.25, 10)},
			{Title: "Fingerprint", Width: colw(w, 0.45, 16)},
		}
		if r, ok := d.data.(*devopsv1.ListSshKeysResponse); ok {
			for _, k := range r.Keys {
				rows = append(rows, table.Row{orDash(k.Label), cut(k.UserId, 12), fingerprint(k.PublicKey)})
			}
		}
	case 6:
		cols = []table.Column{
			{Title: "Time", Width: colw(w, 0.24, 16)},
			{Title: "Actor", Width: colw(w, 0.18, 8)},
			{Title: "Action", Width: colw(w, 0.28, 10)},
			{Title: "Reason", Width: colw(w, 0.3, 8)},
		}
		if r, ok := d.data.(*devopsv1.ListAuditResponse); ok {
			for _, e := range r.Events {
				rows = append(rows, table.Row{auditTime(e.OccurredUnixMs), cut(e.ActorId, 10), cut(e.Action, 18), cut(e.Reason, 20)})
			}
		}
	}
	rows = filterRows(rows, d.filter)
	// Clear rows before swapping columns: SetColumns re-renders existing rows,
	// and a stale row from a prior panel can have fewer cells than the new
	// column set, causing an index-out-of-range panic in renderRow.
	d.table.SetRows(nil)
	d.table.SetColumns(cols)
	d.table.SetRows(rows)
	d.table.SetWidth(w)
	d.table.SetHeight(max(4, d.viewport.Height))
	d.table.GotoTop()
}

// filterRows keeps rows whose any cell contains the query (case-insensitive).
func filterRows(rows []table.Row, query string) []table.Row {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return rows
	}
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		for _, cell := range r {
			if strings.Contains(strings.ToLower(cell), q) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

func colw(total int, frac float64, min int) int {
	w := int(float64(total) * frac)
	if w < min {
		return min
	}
	return w
}

func roleLabel(r devopsv1.Role) string {
	return strings.ToLower(strings.TrimPrefix(r.String(), "ROLE_"))
}

func enabledLabel(disabled bool) string {
	if disabled {
		return "disabled"
	}
	return "active"
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func fingerprint(pub []byte) string {
	if len(pub) == 0 {
		return "—"
	}
	// OpenSSH-style SHA256 fingerprint over the raw authorized_keys line.
	fields := strings.Fields(string(pub))
	blob := pub
	if len(fields) >= 2 {
		if b, err := base64.StdEncoding.DecodeString(fields[1]); err == nil {
			blob = b
		}
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + strings.TrimRight(base64.StdEncoding.EncodeToString(sum[:]), "=")
}

func auditTime(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	return time.UnixMilli(ms).Format("01-02 15:04:05")
}
