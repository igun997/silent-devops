package easypanel

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Runner executes a command and returns combined trimmed stdout. It is injected
// so detection/extraction can be unit-tested without a real Docker daemon.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// ExecRunner runs commands via os/exec.
type ExecRunner struct{}

// Run executes name+args and returns trimmed stdout, mapping non-zero exits to
// an error that includes any stderr for diagnosis.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", errors.New(strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Detection reports what was found about a local EasyPanel install.
type Detection struct {
	Present   bool
	Container string // running panel container name (easypanel.1.*)
	BaseURL   string // http://127.0.0.1:3000
}

// lmdbTokenScript reads the admin apiToken directly from EasyPanel's LMDB using
// the panel container's own lmdb module. Panel records are stored as JSON text
// with a short binary length/version prefix, so the store is opened with binary
// encoding and each value is decoded by skipping to the first '{' and parsing
// the remainder. It accepts an already-decoded {json:{...}} shape too, and
// prints only the token — never the bcrypt password hash on the same record.
const lmdbTokenScript = `const {open}=require("/app/node_modules/lmdb");` +
	`const db=open({path:"/etc/easypanel/data/data.mdb",readOnly:true,encoding:"binary"});` +
	`function dec(value){` +
	`if(value&&typeof value==="object"&&value.json&&typeof value.json==="object")return value.json;` +
	`try{const b=Buffer.from(value);const i=b.indexOf(0x7b);if(i<0)return null;` +
	`const o=JSON.parse(b.slice(i).toString("utf8"));return o&&o.json?o.json:o;}catch(e){return null;}}` +
	`let t="";for(const {key,value} of db.getRange()){` +
	`const k=String(key);if(k.indexOf("users:")!==0)continue;const v=dec(value);` +
	`if(v&&v.apiToken){if(v.admin){t=v.apiToken;break;}if(!t)t=v.apiToken;}}` +
	`process.stdout.write(t);`

// localURL returns the panel HTTP base, honoring an EASYPANEL_URL override for
// test topologies where the panel is not on host loopback.
func localURL() string {
	if u := strings.TrimSpace(os.Getenv("EASYPANEL_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://127.0.0.1:3000"
}

// Detect looks for a running EasyPanel container. It checks the Swarm service /
// container list; a panel container is named "easypanel.1.<id>".
func Detect(ctx context.Context, r Runner) (Detection, error) {
	if r == nil {
		r = ExecRunner{}
	}
	// An explicit override pins the local panel container. On a real host there
	// is exactly one panel; the override is mainly for multi-panel test setups.
	if pinned := strings.TrimSpace(os.Getenv("EASYPANEL_CONTAINER")); pinned != "" {
		return Detection{Present: true, Container: pinned, BaseURL: localURL()}, nil
	}
	names, err := r.Run(ctx, "docker", "ps", "--format", "{{.Names}}")
	if err != nil {
		return Detection{}, err
	}
	container := panelContainer(names)
	if container == "" {
		return Detection{Present: false}, nil
	}
	return Detection{Present: true, Container: container, BaseURL: localURL()}, nil
}

// panelContainer returns the first line naming a panel container.
func panelContainer(psOutput string) string {
	for _, line := range strings.Split(psOutput, "\n") {
		name := strings.TrimSpace(line)
		if strings.HasPrefix(name, "easypanel.1.") ||
			name == "easypanel" || name == "easypanel-app" {
			return name
		}
	}
	return ""
}

// ExtractToken reads the admin API token from the panel container's LMDB store.
func ExtractToken(ctx context.Context, r Runner, container string) (string, error) {
	if r == nil {
		r = ExecRunner{}
	}
	if container == "" {
		return "", errors.New("easypanel: no panel container")
	}
	out, err := r.Run(ctx, "docker", "exec", container, "node", "-e", lmdbTokenScript)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(out)
	if token == "" {
		return "", errors.New("easypanel: no admin apiToken found in store")
	}
	return token, nil
}

// DetectAndToken is the convenience path an agent uses: find the local panel and
// pull its API token, returning a ready-to-use local Client.
func DetectAndToken(ctx context.Context, r Runner) (*Client, Detection, error) {
	det, err := Detect(ctx, r)
	if err != nil {
		return nil, det, err
	}
	if !det.Present {
		return nil, det, errors.New("easypanel: not detected on host")
	}
	token, err := ExtractToken(ctx, r, det.Container)
	if err != nil {
		return nil, det, err
	}
	return New(det.BaseURL, token), det, nil
}
