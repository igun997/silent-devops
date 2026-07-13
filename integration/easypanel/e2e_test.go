//go:build easypanel_e2e

package easypanel_e2e_test

import (
	"os/exec"
	"strings"
	"testing"
)

// compose runs a docker compose subcommand against this suite's compose file.
func compose(t *testing.T, args ...string) string {
	t.Helper()
	full := append([]string{"compose", "-f", "docker-compose.yml"}, args...)
	cmd := exec.Command("docker", full...)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(full, " "), err, out)
	}
	return string(out)
}

// runner executes a command inside the runner container.
func runner(t *testing.T, args ...string) string {
	t.Helper()
	return compose(t, append([]string{"exec", "-T", "runner"}, args...)...)
}

// TestDetectExtractTokenAndMigrate drives the full path against two fake panels:
// host detection, LMDB token extraction via docker exec, fail-closed preflight,
// and a real migrate that lands the service on the target panel.
func TestDetectExtractTokenAndMigrate(t *testing.T) {
	// 1. Detect the source panel from host Docker.
	out := runner(t, "easypanel-migrate", "detect")
	if !strings.Contains(out, "easypanel.1.source") {
		t.Fatalf("source panel not detected: %q", out)
	}

	// 2. Extract the source panel's API token straight from its LMDB store.
	tok := strings.TrimSpace(runner(t, "easypanel-migrate", "token"))
	if !strings.HasPrefix(tok, "source-api-token") {
		t.Fatalf("unexpected extracted token: %q", tok)
	}

	// 3. Fail-closed preflight: migrating into a MISSING remote project must
	//    refuse before calling migrate.
	out = failing(t, "easypanel-migrate", "migrate",
		"--local-project", "staging", "--local-service", "flux-be",
		"--remote-url", "http://easypanel-target:3000",
		"--remote-container", "easypanel.1.target",
		"--remote-project", "ghost", "--remote-service", "flux")
	if !strings.Contains(out, "remote project \"ghost\" not found") {
		t.Fatalf("expected fail-closed on missing remote project, got: %q", out)
	}

	// 4. Real migrate into the existing "tests" project; token for the remote is
	//    extracted from the target container by the CLI.
	out = runner(t, "easypanel-migrate", "migrate",
		"--local-project", "staging", "--local-service", "flux-be",
		"--remote-url", "http://easypanel-target:3000",
		"--remote-container", "easypanel.1.target",
		"--remote-project", "tests", "--remote-service", "flux")
	if !strings.Contains(out, "migrate: ok") {
		t.Fatalf("migrate failed: %q", out)
	}

	// 5. Assert the service now exists on the target panel.
	projs := runner(t, "easypanel-migrate", "projects",
		"--remote-url", "http://easypanel-target:3000",
		"--remote-container", "easypanel.1.target")
	if !strings.Contains(projs, "tests") {
		t.Fatalf("target missing tests project: %q", projs)
	}
	// Inspect the target project via the target panel's own node runtime
	// (loopback), asserting the migrated service landed there.
	inspect := compose(t, "exec", "-T", "easypanel-target", "node", "-e",
		`const http=require("http");const d=JSON.stringify({json:{projectName:"tests"}});`+
			`const r=http.request({host:"127.0.0.1",port:3000,path:"/api/trpc/projects.inspectProject",method:"POST",headers:{"content-type":"application/json"}},`+
			`s=>{let b="";s.on("data",c=>b+=c);s.on("end",()=>{process.stdout.write(b)})});r.write(d);r.end();`)
	if !strings.Contains(inspect, `"flux"`) {
		t.Fatalf("migrated service not present on target: %q", inspect)
	}
}

// failing runs a runner command expected to exit non-zero and returns output.
func failing(t *testing.T, args ...string) string {
	t.Helper()
	full := append([]string{"compose", "-f", "docker-compose.yml", "exec", "-T", "runner"}, args...)
	cmd := exec.Command("docker", full...)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure but succeeded: %s", out)
	}
	return string(out)
}
