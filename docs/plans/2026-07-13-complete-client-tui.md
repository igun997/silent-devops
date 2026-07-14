# Complete Client CLI and TUI Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** Complete advertised client operations and build default Bubble Tea fleet dashboard with safe live read-only verification.

**Architecture:** Extend shared `clientapi.Adapter`; CLI and TUI call same typed methods. Add protocol fields/RPCs only for missing data. Bubble Tea performs async calls and Lip Gloss renders responsive panels.

**Tech Stack:** Go, gRPC/protobuf, Bubble Tea, Bubbles, Lip Gloss, SQLite, Docker Compose.

---

### Task 1: Audit command-to-RPC coverage

**Files:**
- Modify: `internal/clientapi/client_test.go`
- Modify: `internal/clientcli/credentials_test.go`
- Modify: `docs/plans/2026-07-13-complete-client-tui.md`

1. Table-test every command documented in `clientcli.Usage` against validation and adapter behavior.
2. Run `go test ./internal/clientapi ./internal/clientcli`; confirm failures identify every unwired command.
3. Record exact existing RPC coverage and smallest missing protocol surface in this plan.
4. Commit: `test: expose incomplete client command coverage`.

### Task 2: Complete read-only CLI operations

**Files:**
- Modify: `internal/clientapi/client.go`
- Modify: `internal/clientapi/client_test.go`
- Modify: `internal/clientcli/credentials.go`
- Modify: `internal/clientcli/cli.go`
- Modify: `internal/clientcli/*_test.go`
- Modify if required: `api/devops/v1/devops.proto`
- Modify if required: `internal/server/read.go`, `internal/server/fleet.go`

1. Write failing tests for stats, service list/status, bounded logs, cleanup preview, users, SSH-key inventory, and audit.
2. Run focused tests and confirm `command not wired` or missing RPC failures.
3. Implement minimum adapter mappings and strict argument validation.
4. Produce stable JSON and compact human output; preserve non-zero failures and redaction.
5. Regenerate protobuf only if required.
6. Run focused tests, generation check, vet.
7. Commit: `feat: complete read-only client commands`.

### Task 3: Complete mutating CLI operations

**Files:**
- Modify: `internal/clientapi/client.go`
- Modify: `internal/clientapi/client_test.go`
- Modify: `internal/clientcli/credentials.go`
- Modify: `internal/clientcli/cli.go`
- Modify: `internal/clientcli/*_test.go`
- Modify if required: `api/devops/v1/devops.proto`, `internal/server/*.go`

1. Write failing tests for services start/stop/restart, cleanup run preview binding, reboot confirmation, bounded admin exec, enrollment token lifecycle, user management, SSH public-key lifecycle, and SSH launch preparation.
2. Verify confirmation, role denial, target/reason/timeout validation, and no automatic retry.
3. Implement smallest complete RPC mappings and native OpenSSH invocation.
4. Run focused tests and race tests.
5. Commit: `feat: complete privileged client commands`.

### Task 4: Default TUI entrypoint and dependencies

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `cmd/client/main.go`, `cmd/client/main_test.go`
- Replace: `internal/clientcli/tui.go`
- Modify: `internal/clientcli/tui_test.go`

1. Add failing tests: no args launches TUI; explicit CLI still dispatches unchanged; `tui --no-color` works.
2. Add Bubble Tea, Bubbles, Lip Gloss dependencies.
3. Build minimal model with login/fleet/help and safe terminal exit.
4. Run focused tests.
5. Commit: `feat: launch interactive client by default`.

### Task 5: Responsive fleet dashboard

**Files:**
- Create: `internal/clientcli/tui_model.go`
- Create: `internal/clientcli/tui_view.go`
- Create: `internal/clientcli/tui_update.go`
- Modify: `internal/clientcli/tui_test.go`

1. Write failing model tests for resize, sidebar selection, filter/sort, tabs, refresh, empty/offline/error states, and no-color rendering.
2. Implement header, tabs, searchable fleet sidebar, responsive detail panel, status bar, and help.
3. Keep calls async and cancellation-aware.
4. Run focused tests and race tests.
5. Commit: `feat: add responsive fleet dashboard`.

### Task 6: Complete TUI panels and actions

**Files:**
- Modify: `internal/clientcli/tui_*.go`
- Modify: `internal/clientcli/tui_test.go`

1. Write failing tests for Host, Metrics, Services, Logs, Users, SSH Keys, Audit, Actions, and login expiry.
2. Add lazy panel loading, bounded refresh, pagination, and visible errors.
3. Add role-aware actions and destructive confirmation dialogs using shared adapter calls.
4. Ensure passwords clear after attempts and secrets never render.
5. Run focused tests and race tests.
6. Commit: `feat: complete client TUI panels`.

### Task 7: Real Docker client acceptance

**Files:**
- Modify: `integration/docker-compose.yml`
- Modify: `integration/e2e_test.go`
- Modify: `integration/entrypoint.sh`

1. Add failing E2E tests invoking real client CLI for every advertised command.
2. Exercise destructive actions only against disposable fixtures.
3. Add TUI model/program smoke test using deterministic input/output; no idle topology substitute.
4. Verify denied CIDR, viewer/operator/admin behavior, token expiry, errors, and terminal cleanup.
5. Run `make test-e2e`.
6. Commit: `test: cover complete client lifecycle`.

### Task 8: Documentation and live read-only verification

**Files:**
- Modify: `README.md`
- Modify: `docs/installation.md`
- Modify: `docs/operations.md`
- Create: `docs/live-readonly-verification.md`

1. Document default TUI, keys, panels, CLI equivalents, client environment, and safety prompts.
2. Build/install current binaries on approved validator, agent, and client hosts.
3. Run only read operations: login/logout, agents, stats, services list/status, logs, cleanup preview, users, SSH-key list, audit, and TUI navigation.
4. Explicitly skip reboot, deletion, cleanup run, service mutation, exec, SSH sessions, mutation, and revocation.
5. Store sanitized command/status evidence; no secrets.
6. Commit: `docs: document and verify client operations`.

### Task 9: Release gate

1. Run `make generate-check`.
2. Run `make fmt-check`.
3. Run `make vet`.
4. Run `make test`.
5. Run `make test-race`.
6. Run `make build`.
7. Run `make build-linux`.
8. Run `make test-e2e`.
9. Search help commands and assert no `command not wired`, placeholder, or fake success remains.
10. Commit final corrections, push branch, open PR, verify CI.
