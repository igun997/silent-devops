package main

import (
	"testing"
)

func TestParseJoin(t *testing.T) {
	options, ok, err := parseJoin([]string{"join", "validator:8443", "token", "--pin", "sha256/pin", "--credential-dir", "/tmp/creds", "--no-start"})
	if err != nil || !ok || options.Address != "validator:8443" || options.Token != "token" || options.Pin != "sha256/pin" || !options.NoStart {
		t.Fatalf("options=%+v ok=%v err=%v", options, ok, err)
	}
}
func TestParseJoinRequiresPin(t *testing.T) {
	_, _, err := parseJoin([]string{"join", "validator:8443", "token"})
	if err == nil {
		t.Fatal("missing pin accepted")
	}
}
