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
	Version   string // panel semver from /app/package.json, e.g. "2.32.2" (may be "")
	// PublicURL is the panel's own externally reachable base URL, read from its
	// LMDB settings (customPanelDomain over HTTPS, else serverIp:3000). Empty
	// when the panel has neither configured. Used to route a cross-agent migrate
	// to the target panel without relying on the mutable, often-unresolvable
	// agent hostname.
	PublicURL string
}

// lmdbTokenScript reads the admin apiToken directly from EasyPanel's LMDB using
// the panel container's own lmdb module. Panel records are stored as JSON text
// behind a short binary/length/version prefix whose bytes vary by panel build
// (observed: raw length bytes, and a literal spurious '{' before the real
// {"json":...} payload). The store is opened with binary encoding and each
// value is decoded by trying every '{' offset (preferring the {"json" marker)
// until one parses as JSON — a naive first-'{' skip mis-parses the doubled-'{'
// case and silently yields an empty token. It accepts an already-decoded
// {json:{...}} shape too, and prints only the token — never the bcrypt
// password hash on the same record.
const lmdbTokenScript = `const {open}=require("/app/node_modules/lmdb");` +
	`const db=open({path:"/etc/easypanel/data/data.mdb",readOnly:true,encoding:"binary"});` +
	`function dec(value){` +
	`if(value&&typeof value==="object"&&value.json&&typeof value.json==="object")return value.json;` +
	`let b;try{b=Buffer.from(value);}catch(e){return null;}` +
	`let i=b.indexOf(Buffer.from('{"json"'));if(i<0)i=b.indexOf(0x7b);` +
	`for(;i>=0&&i<b.length;i=b.indexOf(0x7b,i+1)){` +
	`try{const o=JSON.parse(b.slice(i).toString("utf8"));return o&&o.json?o.json:o;}catch(e){}}` +
	`return null;}` +
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
	version, _ := PanelVersion(ctx, r, container)     // best-effort; empty is fine
	publicURL, _ := PanelPublicURL(ctx, r, container) // best-effort; empty is fine
	return Detection{Present: true, Container: container, BaseURL: localURL(), Version: version, PublicURL: publicURL}, nil
}

// PanelVersion reads the panel's semver from /app/package.json inside the
// container. It is best-effort: an empty string with a nil error means the
// version could not be determined (the client then negotiates the query method
// via 405 fallback instead of using the version hint).
func PanelVersion(ctx context.Context, r Runner, container string) (string, error) {
	if r == nil {
		r = ExecRunner{}
	}
	if container == "" {
		return "", errors.New("easypanel: no panel container")
	}
	out, err := r.Run(ctx, "docker", "exec", container, "node", "-e",
		`try{process.stdout.write(String(require("/app/package.json").version||""))}catch(e){}`)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// lmdbSettingsScript reads the panel's own reachable base URL from EasyPanel's
// LMDB settings, trying three sources in priority order:
//  1. customPanelDomain — an operator-set custom panel domain (HTTPS)
//  2. defaultDomain     — the panel's own *.easypanel.host subdomain (HTTPS)
//  3. serverIp          — the host public IP on the default HTTP port :3000
//
// Domains are served over HTTPS by the panel's reverse proxy and are publicly
// routable, so they are preferred over the raw IP. Values share the same
// prefixed-JSON encoding as other records, so it reuses the scan-every-'{'
// decode. It prints ONLY the resolved URL (or empty) — never other settings
// such as tokens.
const lmdbSettingsScript = `const {open}=require("/app/node_modules/lmdb");` +
	`const db=open({path:"/etc/easypanel/data/data.mdb",readOnly:true,encoding:"binary"});` +
	`function dec(value){` +
	`if(value&&typeof value==="object"&&value.json!==undefined)return value.json;` +
	`let b;try{b=Buffer.from(value);}catch(e){return undefined;}` +
	`let i=b.indexOf(Buffer.from('{"json"'));if(i<0)i=b.indexOf(0x7b);` +
	`for(;i>=0&&i<b.length;i=b.indexOf(0x7b,i+1)){` +
	`try{const o=JSON.parse(b.slice(i).toString("utf8"));return o&&o.json!==undefined?o.json:o;}catch(e){}}` +
	`return undefined;}` +
	`function get(k){try{const v=dec(db.get(k));return typeof v==="string"?v.trim():"";}catch(e){return "";}}` +
	`const dom=get("settings:customPanelDomain");const def=get("settings:defaultDomain");const ip=get("settings:serverIp");` +
	`let u="";if(dom)u="https://"+dom;else if(def)u="https://"+def;else if(ip)u="http://"+ip+":3000";` +
	`process.stdout.write(u);`

// PanelPublicURL reads the panel's own externally reachable base URL from its
// LMDB settings (customPanelDomain over HTTPS, else serverIp:3000). Best-effort:
// an empty string with a nil error means neither was configured.
func PanelPublicURL(ctx context.Context, r Runner, container string) (string, error) {
	if r == nil {
		r = ExecRunner{}
	}
	if container == "" {
		return "", errors.New("easypanel: no panel container")
	}
	out, err := r.Run(ctx, "docker", "exec", container, "node", "-e", lmdbSettingsScript)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
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
	return NewForVersion(det.BaseURL, token, det.Version), det, nil
}
