package maintenance_test

import (
	"testing"
	"time"

	"silent-devops/internal/maintenance"
)

func TestTypedArgv(t *testing.T) {
	ops := maintenance.Operations{}
	cases := []struct {
		name  string
		args  []string
		want0 string
	}{{"services", ops.ListServices(10), "systemctl"}, {"status", ops.Service("status", "sshd.service"), "systemctl"}, {"logs", ops.Journal("sshd.service", time.Unix(1, 0), time.Unix(2, 0), 50), "journalctl"}, {"processes", ops.Processes(20), "ps"}}
	for _, tc := range cases {
		if len(tc.args) == 0 || tc.args[0] != tc.want0 {
			t.Errorf("%s args=%v", tc.name, tc.args)
		}
	}
	if args := ops.Service("invalid", "sshd.service"); args != nil {
		t.Fatalf("invalid action args=%v", args)
	}
	if args := ops.Journal("../bad", time.Time{}, time.Time{}, 1); args != nil {
		t.Fatalf("invalid unit args=%v", args)
	}
}
func TestRebootConfirmationBindingExpiryAndSingleUse(t *testing.T) {
	m := maintenance.NewRebootManager(time.Minute)
	token, err := m.Confirm("agent-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Consume("agent-2", token, time.Now()); err == nil {
		t.Fatal("wrong target accepted")
	}
	if err := m.Consume("agent-1", token, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := m.Consume("agent-1", token, time.Now()); err == nil {
		t.Fatal("token reused")
	}
	expired, _ := m.Confirm("agent-1", time.Now())
	if err := m.Consume("agent-1", expired, time.Now().Add(2*time.Minute)); err == nil {
		t.Fatal("expired token accepted")
	}
}
