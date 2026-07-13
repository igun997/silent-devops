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
   (`docker ps`). Override with `EASYPANEL_CONTAINER` when needed.
2. **Token** — the admin `apiToken` is stored in the panel's LMDB at
   `/etc/easypanel/data/data.mdb` under key `users:<id>`. The tool runs a small
   read-only script via `docker exec <panel> node -e ...` using the panel's own
   `lmdb` module and prints **only** the token (never the bcrypt password hash).

The panel HTTP base defaults to `http://127.0.0.1:3000`; override with
`EASYPANEL_URL`.

## API surface used

All tRPC procedures are `POST` with a `{"json": <input>}` envelope:

| Procedure | Input | Purpose |
| --- | --- | --- |
| `projects.listProjects` | `{}` | list projects |
| `projects.inspectProject` | `{projectName}` | exists? (200 / 404) + services |
| `projects.createProject` | `{name}` | auto-create remote project |
| `/api/migrate-service` (REST) | migrate fields | push service to remote |

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
```

The client dispatches an admin-exec job that runs `easypanel-migrate` on the
agent and streams back the captured stdout (polling until the job reaches a
terminal state).

### TUI

The dashboard has an **EasyPanel** panel (tab across to it) that runs `detect`
and `projects` on the selected agent and shows the captured output in a
scrollable view.

## Tests

- Unit: `go test ./internal/easypanel/` (client, preflight, detection, token).
- E2E: `make test-e2e-easypanel` spins up two fake EasyPanel panels (real
  `lmdb`-seeded LMDB store, tRPC/REST surface) and drives the real binary
  through detect → token extract → fail-closed preflight → migrate → verify.
