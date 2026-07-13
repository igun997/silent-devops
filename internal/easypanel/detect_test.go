package easypanel

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// scriptRunner returns canned output per (name+args) prefix match.
type scriptRunner struct {
	responses map[string]string
	errs      map[string]error
	calls     []string
}

func (s *scriptRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	s.calls = append(s.calls, key)
	for prefix, err := range s.errs {
		if strings.Contains(key, prefix) {
			return "", err
		}
	}
	for prefix, out := range s.responses {
		if strings.Contains(key, prefix) {
			return out, nil
		}
	}
	return "", errors.New("unexpected: " + key)
}

func TestDetectFindsPanelContainer(t *testing.T) {
	r := &scriptRunner{responses: map[string]string{
		"docker ps": "easypanel-traefik.1.abc\neasypanel.1.n45yu3don2n3psobhp5au56dz\nother",
	}}
	det, err := Detect(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !det.Present || det.Container != "easypanel.1.n45yu3don2n3psobhp5au56dz" {
		t.Fatalf("bad detection: %+v", det)
	}
	if det.BaseURL != "http://127.0.0.1:3000" {
		t.Fatalf("bad base url: %s", det.BaseURL)
	}
}

func TestDetectContainerOverride(t *testing.T) {
	t.Setenv("EASYPANEL_CONTAINER", "easypanel.1.source")
	r := &scriptRunner{responses: map[string]string{"docker ps": "easypanel.1.target"}}
	det, err := Detect(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if det.Container != "easypanel.1.source" {
		t.Fatalf("override ignored: %+v", det)
	}
	if len(r.calls) != 0 {
		t.Fatalf("override should skip docker ps, calls=%v", r.calls)
	}
}

func TestDetectAbsent(t *testing.T) {
	r := &scriptRunner{responses: map[string]string{"docker ps": "nginx\nredis"}}
	det, err := Detect(context.Background(), r)
	if err != nil || det.Present {
		t.Fatalf("should be absent: %+v err=%v", det, err)
	}
}

func TestExtractTokenReadsLMDB(t *testing.T) {
	r := &scriptRunner{responses: map[string]string{
		"docker exec easypanel.1.abc node -e": "6174dcfa460ea411e4b45fcd02a0d9e10613a585f9742b9aeaff9f9a2659238d\n",
	}}
	tok, err := ExtractToken(context.Background(), r, "easypanel.1.abc")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "6174dcfa460ea411e4b45fcd02a0d9e10613a585f9742b9aeaff9f9a2659238d" {
		t.Fatalf("bad token: %q", tok)
	}
}

func TestExtractTokenEmptyFails(t *testing.T) {
	r := &scriptRunner{responses: map[string]string{"docker exec": ""}}
	if _, err := ExtractToken(context.Background(), r, "easypanel.1.abc"); err == nil {
		t.Fatal("expected error on empty token")
	}
}

func TestDetectAndTokenWiresClient(t *testing.T) {
	r := &scriptRunner{responses: map[string]string{
		"docker ps":   "easypanel.1.xyz",
		"docker exec": "tok-123",
	}}
	c, det, err := DetectAndToken(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !det.Present || c.Token != "tok-123" || c.BaseURL != "http://127.0.0.1:3000" {
		t.Fatalf("bad wiring: det=%+v client=%+v", det, c)
	}
}

func TestDetectAndTokenAbsentErrors(t *testing.T) {
	r := &scriptRunner{responses: map[string]string{"docker ps": "nginx"}}
	if _, _, err := DetectAndToken(context.Background(), r); err == nil {
		t.Fatal("expected not-detected error")
	}
}
