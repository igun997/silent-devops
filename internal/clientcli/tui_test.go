package clientcli_test

import (
	"testing"

	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/clientcli"
)

func TestTUIViewsFilterSortRoleAndDestructiveDialog(t *testing.T) {
	m := clientcli.NewModel(devopsv1.Role_ROLE_OPERATOR, true)
	for _, view := range []clientcli.View{clientcli.ViewLogin, clientcli.ViewFleet, clientcli.ViewHost, clientcli.ViewMetrics, clientcli.ViewServices, clientcli.ViewLogs, clientcli.ViewActions, clientcli.ViewSSH, clientcli.ViewAudit, clientcli.ViewHelp} {
		if !m.CanView(view) {
			t.Errorf("missing view %v", view)
		}
	}
	m.SetAgents([]string{"zeta", "alpha", "beta"})
	m.Filter("a")
	got := m.VisibleAgents()
	if len(got) != 3 || got[0] != "alpha" {
		t.Fatalf("agents=%v", got)
	}
	m.Confirm("reboot agent alpha")
	if !m.DialogOpen() || m.AcceptDialog() != "reboot agent alpha" {
		t.Fatal("dialog failed")
	}
	if !m.NoColor {
		t.Fatal("no-color lost")
	}
}
