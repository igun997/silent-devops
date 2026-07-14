package clientapi

import "testing"

func TestExtractFlag(t *testing.T) {
	v, rest, ok := extractFlag([]string{"migrate", "--to-agent", "dst", "--x"}, "--to-agent")
	if !ok || v != "dst" {
		t.Fatalf("value=%q ok=%v", v, ok)
	}
	if len(rest) != 2 || rest[0] != "migrate" || rest[1] != "--x" {
		t.Fatalf("rest=%v", rest)
	}
	if _, _, ok := extractFlag([]string{"a", "b"}, "--to-agent"); ok {
		t.Fatal("absent flag should return ok=false")
	}
	// Trailing flag without a value is not consumed.
	if _, rest, ok := extractFlag([]string{"a", "--to-agent"}, "--to-agent"); ok || len(rest) != 2 {
		t.Fatalf("dangling flag mishandled ok=%v rest=%v", ok, rest)
	}
}

func TestParseDetectURL(t *testing.T) {
	got := parseDetectURL("easypanel: detected container=easypanel.1.x url=http://127.0.0.1:3000")
	if got != "http://127.0.0.1:3000" {
		t.Fatalf("got %q", got)
	}
	if parseDetectURL("easypanel: not detected") != "" {
		t.Fatal("expected empty for no url field")
	}
}

func TestExtractBoolFlag(t *testing.T) {
	rest, ok := extractBoolFlag([]string{"migrate", "--detach", "--x"}, "--detach")
	if !ok {
		t.Fatal("expected --detach present")
	}
	if len(rest) != 2 || rest[0] != "migrate" || rest[1] != "--x" {
		t.Fatalf("rest=%v", rest)
	}
	if _, ok := extractBoolFlag([]string{"a", "b"}, "--detach"); ok {
		t.Fatal("absent bool flag should return ok=false")
	}
}

func TestParseDetectField(t *testing.T) {
	line := "easypanel: detected container=easypanel.1.x url=http://127.0.0.1:3000 version=2.32.2 public_url=https://panel.example.com"
	if got := parseDetectField(line, "public_url="); got != "https://panel.example.com" {
		t.Fatalf("public_url=%q", got)
	}
	if got := parseDetectField(line, "url="); got != "http://127.0.0.1:3000" {
		t.Fatalf("url=%q", got)
	}
	// "unknown" sentinel is treated as absent.
	if got := parseDetectField("x public_url=unknown", "public_url="); got != "" {
		t.Fatalf("unknown should be empty, got %q", got)
	}
	if got := parseDetectField("x version=2.30.1", "public_url="); got != "" {
		t.Fatalf("absent field should be empty, got %q", got)
	}
}
