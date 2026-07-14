package clientcli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	devopsv1 "silent-devops/api/devops/v1"
)

// migrateForm collects inputs for an EasyPanel service migration between two
// agents. The source agent is preset to the currently selected agent; the
// target agent is chosen from the fleet. The target panel URL and token are
// resolved automatically by the client adapter (--to-agent), so the operator
// only names projects/services and toggles create/overwrite.
type migrateForm struct {
	fields    []textinput.Model // localProject, localService, remoteProject, remoteService, timeout(min)
	focus     int               // index into fields, then target, create, overwrite, detach, submit
	targets   []*devopsv1.Agent // candidate target agents (excludes source)
	targetIdx int
	create    bool
	overwrite bool
	detach    bool
	source    *devopsv1.Agent
	confirm   bool // second Enter confirms the destructive run
}

// migrateField labels align with the fields slice order. "remote url" is
// optional (blank = derive http://<target hostname>:3000, which only works when
// that host is resolvable from the source agent). "timeout (min)" bounds the
// long-running migrate job; the agent kills the job past this deadline.
var migrateLabels = []string{"local project", "local service", "remote project", "remote service", "remote url (opt)", "timeout (min)"}

// optionalMigrateFields lists field indices that may be left blank.
var optionalMigrateFields = map[int]bool{mfRemoteURL: true}

// focus stops: 0-5 fields (incl. remote url + timeout), 6 target, 7 create,
// 8 overwrite, 9 detach, 10 submit.
const (
	mfRemoteURL = 4
	mfTimeout   = 5
	mfTarget    = 6
	mfCreate    = 7
	mfOverwrite = 8
	mfDetach    = 9
	mfSubmit    = 10
	mfStops     = 11
)

func newMigrateForm(source *devopsv1.Agent, targets []*devopsv1.Agent) *migrateForm {
	f := &migrateForm{source: source, targets: targets}
	for i := range migrateLabels {
		in := textinput.New()
		in.Prompt = "› "
		in.CharLimit = 128
		in.Placeholder = migrateLabels[i]
		if i == 0 {
			in.Focus()
		}
		f.fields = append(f.fields, in)
	}
	f.create = true                    // default to auto-creating the remote project
	f.fields[mfTimeout].SetValue("30") // default 30-minute migrate budget
	return f
}

// openMigrate initializes the migrate form on the EasyPanel panel when at least
// one other agent exists as a target.
func (d *Dashboard) openMigrate() {
	if len(d.agents) == 0 {
		return
	}
	source := d.agents[d.selected]
	var targets []*devopsv1.Agent
	for _, a := range d.agents {
		if a.GetId() != source.GetId() {
			targets = append(targets, a)
		}
	}
	if len(targets) == 0 {
		d.err = fmt.Errorf("need a second agent to migrate to")
		return
	}
	d.form = newMigrateForm(source, targets)
	d.migrating = true
	d.err = nil
}

func (d *Dashboard) closeMigrate() {
	d.migrating = false
	d.form = nil
}

// updateMigrate drives the migrate form. Enter on the submit stop requires a
// second confirming Enter before dispatching the (destructive) migration.
func (d *Dashboard) updateMigrate(k tea.KeyMsg) tea.Cmd {
	f := d.form
	if f == nil {
		d.migrating = false
		return nil
	}
	switch k.String() {
	case "esc":
		d.closeMigrate()
		return nil
	case "tab", "down":
		f.setFocus((f.focus + 1) % mfStops)
		f.confirm = false
		return nil
	case "shift+tab", "up":
		f.setFocus((f.focus + mfStops - 1) % mfStops)
		f.confirm = false
		return nil
	case "left", "right", " ":
		switch f.focus {
		case mfTarget:
			if len(f.targets) > 0 {
				if k.String() == "left" {
					f.targetIdx = (f.targetIdx + len(f.targets) - 1) % len(f.targets)
				} else {
					f.targetIdx = (f.targetIdx + 1) % len(f.targets)
				}
			}
		case mfCreate:
			f.create = !f.create
		case mfOverwrite:
			f.overwrite = !f.overwrite
		case mfDetach:
			f.detach = !f.detach
		}
		return nil
	case "enter":
		if f.focus != mfSubmit {
			f.setFocus((f.focus + 1) % mfStops)
			return nil
		}
		if err := f.validate(); err != nil {
			d.err = err
			return nil
		}
		if !f.confirm {
			f.confirm = true // ask for a second Enter to confirm
			return nil
		}
		return d.dispatchMigrate()
	}
	// Text input while a field is focused.
	if f.focus < len(f.fields) {
		var cmd tea.Cmd
		f.fields[f.focus], cmd = f.fields[f.focus].Update(k)
		return cmd
	}
	return nil
}

func (f *migrateForm) setFocus(i int) {
	f.focus = i
	for j := range f.fields {
		if j == i {
			f.fields[j].Focus()
		} else {
			f.fields[j].Blur()
		}
	}
}

func (f *migrateForm) validate() error {
	for i := range f.fields {
		if optionalMigrateFields[i] {
			continue
		}
		if strings.TrimSpace(f.fields[i].Value()) == "" {
			return fmt.Errorf("%s is required", migrateLabels[i])
		}
	}
	if m, err := strconv.Atoi(strings.TrimSpace(f.fields[mfTimeout].Value())); err != nil || m <= 0 {
		return fmt.Errorf("timeout (min) must be a positive number")
	}
	if len(f.targets) == 0 {
		return fmt.Errorf("no target agent")
	}
	return nil
}

// dispatchMigrate builds and runs the easypanel migrate command through the
// client API and shows its captured output in the EasyPanel panel.
func (d *Dashboard) dispatchMigrate() tea.Cmd {
	f := d.form
	src := f.source.GetId()
	dst := f.targets[f.targetIdx].GetId()
	args := []string{
		src, "migrate", "--to-agent", dst,
		"--local-project", strings.TrimSpace(f.fields[0].Value()),
		"--local-service", strings.TrimSpace(f.fields[1].Value()),
		"--remote-project", strings.TrimSpace(f.fields[2].Value()),
		"--remote-service", strings.TrimSpace(f.fields[3].Value()),
	}
	if ru := strings.TrimSpace(f.fields[mfRemoteURL].Value()); ru != "" {
		args = append(args, "--remote-url", ru)
	}
	args = append(args, "--timeout", strings.TrimSpace(f.fields[mfTimeout].Value()))
	if f.create {
		args = append(args, "--create-remote-project")
	}
	if f.overwrite {
		args = append(args, "--overwrite-remote-service")
	}
	if f.detach {
		args = append(args, "--detach")
	}
	d.closeMigrate()
	d.loading = true
	d.panel = easypanelPanel
	return func() tea.Msg {
		v, e := d.API.Call(context.Background(), "easypanel", args)
		if e != nil {
			return resultMsg{panel: easypanelPanel, err: e}
		}
		return resultMsg{panel: easypanelPanel, value: "$ easypanel migrate\n" + easypanelOutput(v)}
	}
}

// migrateView renders the migrate form.
func (d *Dashboard) migrateView() string {
	f := d.form
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(d.th.heading.Render("MIGRATE EASYPANEL SERVICE") + "\n\n")
	b.WriteString(d.th.label.Render("source agent  ") + d.th.value.Render(agentName(f.source)) + "\n\n")
	for i := range f.fields {
		cursor := "  "
		if f.focus == i {
			cursor = d.th.cursor.Render("▸ ")
		}
		b.WriteString(cursor + d.th.label.Render(fmt.Sprintf("%-15s", migrateLabels[i])) + f.fields[i].View() + "\n")
	}
	b.WriteString("\n")
	b.WriteString(d.toggleLine(f.focus == mfTarget, "target agent", agentName(f.targets[f.targetIdx])+"  (←/→)") + "\n")
	b.WriteString(d.toggleLine(f.focus == mfCreate, "create remote project", boolText(f.create)+"  (space)") + "\n")
	b.WriteString(d.toggleLine(f.focus == mfOverwrite, "overwrite remote svc", boolText(f.overwrite)+"  (space)") + "\n")
	b.WriteString(d.toggleLine(f.focus == mfDetach, "detach (run in background)", boolText(f.detach)+"  (space)") + "\n\n")
	submit := "  [ run migrate ]"
	if f.focus == mfSubmit {
		submit = d.th.cursor.Render("▸ ") + d.th.value.Render("[ run migrate ]")
	}
	b.WriteString(submit + "\n")
	if f.confirm {
		b.WriteString("\n" + d.th.offDot.Render("Press Enter again to CONFIRM this migration.") + "\n")
	}
	b.WriteString("\n" + d.th.subtle.Render("tab/↑↓ move · ←/→/space toggle · enter next/confirm · esc cancel") + "\n")
	return b.String()
}

func (d *Dashboard) toggleLine(focused bool, label, value string) string {
	cursor := "  "
	if focused {
		cursor = d.th.cursor.Render("▸ ")
	}
	return cursor + d.th.label.Render(fmt.Sprintf("%-22s", label)) + d.th.value.Render(value)
}

func boolText(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func agentName(a *devopsv1.Agent) string {
	if a == nil {
		return "?"
	}
	name := a.GetHostname()
	if name == "" {
		name = a.GetId()
	}
	return name + " (" + a.GetId() + ")"
}
