package app_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"silent-devops/internal/app"
)

func TestRunVersionHasNoSideEffects(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false

	code := app.Run(context.Background(), "agent", []string{"version"}, &stdout, &stderr, func(context.Context) error {
		called = true
		return nil
	})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if called {
		t.Fatal("service started while printing version")
	}
	if !strings.Contains(stdout.String(), "agent") {
		t.Fatalf("stdout %q does not identify binary", stdout.String())
	}
}

func TestRunHelpHasNoSideEffects(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false

	code := app.Run(context.Background(), "validator", []string{"--help"}, &stdout, &stderr, func(context.Context) error {
		called = true
		return nil
	})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if called {
		t.Fatal("service started while printing help")
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("stdout %q does not contain usage", stdout.String())
	}
}

func TestRunRejectsUnknownArgumentsWithoutStarting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false

	code := app.Run(context.Background(), "client", []string{"--unknown"}, &stdout, &stderr, func(context.Context) error {
		called = true
		return nil
	})

	if code == 0 {
		t.Fatal("exit code = 0, want failure")
	}
	if called {
		t.Fatal("service started for invalid arguments")
	}
	if !strings.Contains(stderr.String(), "unknown argument") {
		t.Fatalf("stderr %q does not explain failure", stderr.String())
	}
}
