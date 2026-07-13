package main

import "testing"

func TestStripFlags(t *testing.T) {
	got := stripFlags([]string{"reboot", "a", "--yes", "--json"})
	if len(got) != 2 || got[1] != "a" {
		t.Fatalf("got=%v", got)
	}
}
