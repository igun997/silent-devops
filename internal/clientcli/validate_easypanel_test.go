package clientcli

import "testing"

func TestValidateEasypanelActions(t *testing.T) {
	ok := [][]string{
		{"easypanel", "AGENT", "detect"},
		{"easypanel", "AGENT", "projects"},
		{"easypanel", "AGENT", "token"},
		{"easypanel", "AGENT", "migrate", "--to-agent", "DST"},
		{"easypanel", "AGENT", "job", "JOB_ID"},
	}
	for _, a := range ok {
		if err := Validate(a); err != nil {
			t.Errorf("Validate(%v) = %v, want nil", a, err)
		}
	}
	bad := [][]string{
		{"easypanel", "AGENT"},          // no action
		{"easypanel", "AGENT", "job"},   // job needs id
		{"easypanel", "AGENT", "bogus"}, // unknown action
	}
	for _, a := range bad {
		if err := Validate(a); err == nil {
			t.Errorf("Validate(%v) = nil, want error", a)
		}
	}
}
