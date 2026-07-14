# EasyPanel service migration (`easypanel-migrate`)

`easypanel-migrate` detects a local [EasyPanel](https://easypanel.io) install on
an agent host, reads its API token straight from the panel's LMDB store, and
migrates ("snapshots and transfers") a service to a remote EasyPanel — with a
fail-closed preflight that refuses to run when the target project is missing.

## Why

EasyPanel's built-in snapshot/transfer (`POST /api/migrate-service`) pushes a
service definition from one panel to another. If the remote project does not
exist, the panel returns an opaque `500 {"message":"Service not found."}`. This
tool verifies both sides first and fails with a precise error instead.

## How detection + token work (zero credentials)

The agent runs on the host with Docker access:

1. **Detect** — a running panel container is named `easypanel.1.*`
   (`docker ps`). Override with `EASYPANEL_CONTAINER` when needed. Detect also
   reads the panel **version** from `/app/package.json`, printed as
   `version=<semver>` (used to pick the tRPC query method, see below), and the
   panel's own externally reachable **public URL** from its LMDB settings,
   printed as `public_url=<url>` (used to route a cross-agent migrate, see
   [Cross-agent migration](#cross-agent-migration-and-remote-url-resolution)):

   ```
   easypanel: detected container=easypanel.1.x url=http://127.0.0.1:3000 version=2.32.2 public_url=https://panel.example.com
   ```
2. **Token** — the admin `apiToken` is stored in the panel's LMDB at
   `/etc/easypanel/data/data.mdb` under key `users:<id>`. The tool runs a small
   read-only script via `docker exec <panel> node -e ...` using the panel's own
   `lmdb` module and prints **only** the token (never the bcrypt password hash).
   The record sits behind a short binary prefix whose bytes vary by build
   (including a literal spurious `{`), so the extractor scans every `{` offset
   until the JSON parses rather than stopping at the first `{`.

The panel HTTP base defaults to `http://127.0.0.1:3000`; override with
`EASYPANEL_URL`.

## Installing on agents

The `easypanel-migrate` binary is **not** bundled by the agent installer
(`install.sh agent`). Install it on every agent host whose EasyPanel you want to
manage — the client `easypanel AGENT …` verbs shell out to it over admin exec,
and a missing binary surfaces as `/bin/sh: 1: easypanel-migrate: not found`.

Build for Linux and install to `/usr/local/bin` (matching the agent `PATH`):

```sh
# from a checkout, build the linux/amd64 (or arm64) binary
GOOS=linux GOARCH=amd64 go build -o bin/easypanel-migrate-linux-amd64 ./cmd/easypanel-migrate

# copy to the agent host and install
scp bin/easypanel-migrate-linux-amd64 HOST:/tmp/em
ssh HOST 'sudo install -m 0755 /tmp/em /usr/local/bin/easypanel-migrate'

# verify
silent-devops-client easypanel AGENT_ID detect
```

The host needs Docker access (the binary reads the panel via `docker ps` /
`docker exec`); the agent runs as root or a Docker-group member, so no extra
credentials are required.

## API surface used

| Procedure | Kind | Input | Purpose |
| --- | --- | --- | --- |
| `projects.listProjects` | query | `{}` | list projects |
| `projects.inspectProject` | query | `{projectName}` | exists? (200 / 404) + services |
| `projects.createProject` | mutation | `{name}` | auto-create remote project |
| `/api/migrate-service` (REST) | POST | migrate fields | push service to remote |

### Version-driven query method

EasyPanel builds disagree on the HTTP method for tRPC **queries** and on the
response envelope:

| Version | Query method | Response envelope |
| --- | --- | --- |
| `2.30.x` (and older) | `GET` `?input={"json":input}` | `{"result":{"data":{"json":…}}}` |
| `2.32.x` (and newer) | `POST` body `{"json":input}` | `{"json":…}` |

The client picks the initial query method from the detected version
(`>= 2.32.0` → POST, else GET) to avoid a wasted round-trip, and still falls
back on an HTTP `405` if the guess is wrong (e.g. an untested boundary build),
caching the winning method. Both response envelopes are decoded. **Mutations**
are always `POST`.

## Preflight (fail-closed)

Before calling migrate the tool verifies:

1. local project exists,
2. local service exists in it,
3. remote project exists (or is auto-created with `--create-remote-project`),
4. remote service name is free (unless `--overwrite-remote-service`).

## Usage

```sh
# is EasyPanel here?
easypanel-migrate detect

# print the local panel API token (from LMDB)
easypanel-migrate token

# list local (or remote) projects
easypanel-migrate projects
easypanel-migrate projects --remote-url http://REMOTE:3000 --remote-token TOK

# migrate a service between two panels
easypanel-migrate migrate \
  --local-project staging --local-service flux-be \
  --remote-url http://REMOTE:3000 --remote-token TOK \
  --remote-project tests --remote-service flux \
  [--create-remote-project] [--overwrite-remote-service]
```

`--remote-container NAME` extracts the remote token from a locally reachable
panel container instead of `--remote-token`.

## From the Silent DevOps client

The binary is installed on the agent host and driven remotely through admin
exec, so it works from the client CLI and TUI without SSH.

### CLI

```sh
# read-only actions (no confirmation)
silent-devops-client easypanel AGENT_ID detect
silent-devops-client easypanel AGENT_ID projects
silent-devops-client easypanel AGENT_ID token

# migrate mutates a remote panel and requires --yes
silent-devops-client easypanel AGENT_ID migrate --yes \
  --local-project staging --local-service flux-be \
  --remote-url http://REMOTE:3000 --remote-token TOK \
  --remote-project tests --remote-service flux

# migrate between two agents: --to-agent resolves the target panel URL
# (from the target panel's own public_url, or --remote-url) and token
# automatically.
silent-devops-client easypanel SRC_AGENT_ID migrate --yes \
  --to-agent DST_AGENT_ID \
  --local-project staging --local-service flux-be \
  --remote-project tests --remote-service flux \
  --create-remote-project

# long migrate: raise the timeout (minutes, default 30) and/or run detached
silent-devops-client easypanel SRC_AGENT_ID migrate --yes \
  --to-agent DST_AGENT_ID --timeout 60 --detach \
  --local-project staging --local-service flux-be \
  --remote-project tests --remote-service flux --create-remote-project

# check a dispatched (e.g. detached) migrate job's status + output
silent-devops-client easypanel SRC_AGENT_ID job JOB_ID
```

### Cross-agent migration and remote-url resolution

With `--to-agent`, the client runs `detect` + `token` on the target agent to
resolve its panel token, resolves the target panel's **reachable URL**, and
passes both to the migrate job on the source agent. The source agent must be
able to reach that URL over the network.

The reachable URL is resolved in this priority order:

1. an explicit `--remote-url` (or the TUI "remote url" field), if given;
2. the target panel's own configured address, read from its LMDB settings and
   printed by `detect` as `public_url=` (see the detect example above):
   `customPanelDomain` (HTTPS) → `defaultDomain` `*.easypanel.host` (HTTPS) →
   `serverIp` (`http://IP:3000`);
3. as a last resort, `http://<target agent hostname>:3000`.

The agent **hostname** is mutable metadata and is usually **not resolvable**
from the source agent across networks (symptom:
`dial tcp: lookup HOSTNAME ... server misbehaving`), which is why the panel's
own `public_url` is preferred.

The client dispatches an admin-exec job that runs `easypanel-migrate` on the
agent and streams back the captured stdout.

**Long-running migrations.** A migrate is a panel snapshot+transfer that can
take minutes. The job runs on the agent and is persisted on the validator, so
it is observable independently of the client:

- `--timeout MINUTES` (migrate only, default **30**) sets the job deadline; the
  agent kills the job past this bound and the client follows output up to the
  same deadline (no fixed short ceiling). Set generously for large services.
- `--detach` (migrate only) dispatches the job and returns its `job_id`
  immediately instead of blocking on output.
- `easypanel AGENT job JOB_ID` reports whether the job is still `running` or
  `terminal`, printing the captured output once terminal. Re-run to poll. This
  works whether or not the original command detached, since the validator holds
  the job and its output.

### TUI

The dashboard has an **EasyPanel** panel (tab across to it) that runs `detect`
and `projects` on the selected agent and shows the captured output in a
scrollable view.

Press **`m`** on the EasyPanel panel to open the **migrate form**: the source
agent is preset to the selected agent, you pick a target agent and name the
local/remote projects and services, set the **timeout (min)** (default 30),
toggle create/overwrite and **detach (run in background)**, and confirm. The
client resolves the target panel URL + token automatically and streams the
migrate output back into the panel. Migration is destructive and requires an
explicit second Enter to confirm.

## Tests

- Unit: `go test ./internal/easypanel/` (client, preflight, detection, token).
- E2E: `make test-e2e-easypanel` spins up two fake EasyPanel panels (real
  `lmdb`-seeded LMDB store, tRPC/REST surface) and drives the real binary
  through detect → token extract → fail-closed preflight → migrate → verify.
