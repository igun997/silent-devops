# Silent DevOps MVP Implementation Plan

> **REQUIRED SUB-SKILL:** Use executing-plans skill to implement this plan milestone-by-milestone.

## Title

Secure Root-Agent Fleet Management MVP

## Goal

Build one Go module containing three Linux applications:

- `agent`: root daemon installed on managed Ubuntu/Debian hosts.
- `validator`: central control plane for identity, authorization, agent streams, operations, metrics, audit records, and SSH sessions.
- `client`: operator CLI with interactive TUI and scriptable commands.

Agents initiate persistent outbound mTLS gRPC connections to validator. Validator sends authorized jobs over those streams. Interactive access uses short-lived reverse SSH tunnels through dedicated restricted OpenSSH service on validator.

## Non-goals / out of scope

- Containers as managed resources; Docker is test infrastructure only.
- Package installation or upgrades.
- Firewall, OS user, cron, or systemd timer management.
- Permanent SSH-key deployment.
- File upload/download.
- Scheduled jobs, playbooks, multi-host batch execution, or rolling deployments.
- Validator HA, PostgreSQL, external metrics storage, OIDC, KMS, or HSM.
- Browser/web UI.
- SSH session recording.
- Generic non-systemd Linux or RHEL-family support.
- Agent inbound control port.
- Validator storage of client SSH private keys.
- Custom Go SSH server.
- Arbitrary commands for `operator` users.

## Relevant context and files to inspect

Repository was greenfield except `.git` when plan was created. Recheck before implementation:

```bash
git status --short
git log --oneline --decorate -10
find . -maxdepth 3 -type f
go version
protoc --version
docker version
docker compose version
ssh -V
```

Expected structure, adjustable after inspection:

```text
go.mod
go.sum
Makefile
README.md
api/devops/v1/
cmd/agent/
cmd/validator/
cmd/client/
internal/
migrations/
packaging/systemd/
packaging/ssh/
integration/
```

## Constraints and repository conventions

### Repository

- Single Go module.
- Three commands: `cmd/agent`, `cmd/validator`, `cmd/client`.
- Shared protobuf contracts with deterministic generation.
- Minimal dependencies; standard library and native Linux/OpenSSH/systemd first.
- Avoid speculative abstractions and plugin systems.
- Keep security-critical APIs typed and explicit.
- Use context cancellation and bounded timeouts.
- TDD for non-trivial and security-sensitive behavior.

### Supported systems

Managed agents:

- Ubuntu 22.04 and 24.04.
- Debian 12.
- amd64 and arm64.
- systemd, journald, and OpenSSH.
- VM or bare-metal host only.

Validator:

- Single Linux systemd host.
- SQLite persistence.
- Dedicated restricted OpenSSH tunnel service/account.

### Security invariants

- Root agent is intentional; validator compromise is fleet-root threat.
- Agent opens outbound connection and exposes no inbound control listener.
- Use mTLS for established agent control streams.
- Agent generates private key locally; key never leaves host.
- Pin validator trust during enrollment.
- Enrollment tokens are random, hashed, short-lived, single-use, and concurrency-safe.
- Rotate agent certificates before expiry; revoked agents cannot renew or reconnect.
- Separate validator server identity and agent-signing CA.
- CA key stays encrypted in root-only file outside SQLite.
- Load CA secret through systemd credential or protected prompt/file, not CLI argument.
- Validator ingress applies configurable IPv4/IPv6 CIDR allowlist before authentication.
- Source-IP policy uses socket peer address, never untrusted forwarded headers.
- Local passwords use Argon2id with random salts and documented resource limits.
- Login is rate-limited with generic failure responses.
- Access tokens are short-lived, signed, audience-bound, and never logged.
- Roles: `viewer`, `operator`, `admin`; authorization denies by default.
- Operators use typed maintenance actions only.
- Arbitrary commands and interactive SSH require admin.
- Typed operations invoke direct argv and never build shell strings.
- Arbitrary commands require target, reason, bounded timeout, and confirmation.
- Every job has unique ID, actor, target, deadline, operation kind, and authorization context.
- Agent rejects duplicate, expired, malformed, and unsupported jobs.
- Validator is authorization authority; agent still enforces operation shape and hard limits.
- Bound all messages, outputs, logs, queues, subprocesses, and retention.
- Do not log passwords, tokens, private keys, authorization headers, CA secrets, or full environments.
- Command output persistence is opt-in and size-capped.
- SSH content is not recorded.
- Validator stores public SSH keys only; private keys remain client-side or in SSH agent.
- Temporary SSH authorization is removed on timeout, disconnect, and restart reconciliation.
- If SQLite cannot persist authorization or audit state, validator rejects new privileged operations.

### Reliability

- Agent reconnects with exponential backoff and jitter.
- Stable agent ID is independent from hostname.
- Hostname and boot ID are mutable metadata.
- One active stream per agent identity with explicit duplicate-stream policy.
- Commands have deadlines and cancellation semantics.
- Unsafe operations use at-most-once dispatch semantics.
- Unknown completion is reported as `unknown`, never retried automatically.
- SQLite enables WAL, foreign keys, busy timeout, migrations, and bounded retention.
- Metrics ingestion cannot block command/control traffic.
- Agent offline metrics buffer is bounded and may discard stale samples.

## Assumptions confirmed with user

- Root agent model.
- Hybrid typed gRPC operations and SSH.
- Outbound persistent agent connections.
- Reverse SSH through restricted validator OpenSSH service.
- Loopback-only reverse tunnel listeners.
- One-time enrollment token and local agent key generation.
- Local validator users and RBAC.
- SQLite persistence.
- Ubuntu/Debian systemd hosts only.
- No managed-container support.
- Validator ingress CIDR whitelist.
- TUI plus scriptable commands with JSON output.
- Operators use structured actions; admins get arbitrary commands and SSH.
- Metadata-first audit; output capture opt-in and bounded.
- Metrics sampled every 15 seconds, aggregated per minute, retained seven days.
- User-owned SSH private keys; validator stores public keys only.
- Local encrypted agent-signing CA.
- Docker Compose E2E simulation.
- Additional production-hardening controls are deferred and documented, not added to MVP.

## RBAC matrix

| Capability | viewer | operator | admin |
|---|---:|---:|---:|
| View fleet/metrics | Yes | Yes | Yes |
| View services/logs | Yes | Yes | Yes |
| Structured maintenance | No | Yes | Yes |
| Disk cleanup execution | No | Yes | Yes |
| Reboot | No | Yes | Yes |
| Arbitrary command | No | No | Yes |
| Interactive SSH | No | No | Yes |
| User/enrollment management | No | No | Yes |

## Milestones with concrete subtasks

### 1. Buildable repository foundation

- Create Go module and three command entry points.
- Add build, format, vet, test, race-test, protobuf-generation, and cross-build targets.
- Add config loading, structured secret-safe logging, and build/version metadata.
- Add minimal README and tests proving help/version parsing has no side effects.

Deliverable: three buildable binaries with no privileged behavior.

### 2. Protocol contracts

Define protobuf messages/services for:

- Enrollment and certificate renewal.
- Bidirectional agent control stream.
- Agent hello, heartbeat, capabilities, and connection state.
- Metrics snapshots.
- Typed operations and results.
- Arbitrary commands and results.
- Cancellation.
- SSH session prepare/readiness/close.
- Client authentication and fleet APIs.
- Stable machine-readable error codes.

Specify limits, IDs, deadlines, version negotiation, duplicate stream/job policy, unknown-result semantics, and protobuf compatibility rules. Add deterministic generation and round-trip tests.

### 3. SQLite foundation

Add migrations and storage for:

- Schema versions, users, roles, public SSH keys.
- Enrollment tokens.
- Agents and certificate/revocation state.
- Connection state.
- Jobs and audit events.
- SSH sessions.
- Current metrics and one-minute aggregates.

Enable WAL, foreign keys, busy timeout, transactions, uniqueness constraints, cleanup, and seven-day retention. Add migration, restart, integrity, and backup/restore tests/docs. Never store private keys, CA secrets, plaintext passwords/tokens, or command output by default.

### 4. PKI and enrollment

- Generate/import separate server identity and agent-signing CA.
- Encrypt CA key outside SQLite and reject insecure permissions.
- Safely create one bootstrap admin once.
- Create hashed one-time enrollment tokens.
- Agent generates local keypair and CSR.
- Enrollment checks CIDR, token, expiry, pin, and CSR.
- Consume token and issue identity atomically where practical.
- Persist agent key/cert atomically with root-only permissions.
- Renew over existing mTLS identity; support revocation.

Test token reuse, expiry, concurrent redemption, wrong pin, malformed CSR, unauthorized renewal, expiry, and revocation.

### 5. Agent stream and registry

- Implement outbound bidirectional mTLS stream.
- Add reconnect backoff/jitter, hello metadata, heartbeat, online/offline state, and capability/version negotiation.
- Enforce certificate identity matching and one active stream per agent.
- Handle validator/agent restart, duplicate streams, revocation, timeout, and backpressure.

### 6. Authentication, CIDR policy, and RBAC

- Parse validated IPv4/IPv6 CIDRs and reject invalid startup config.
- Apply peer-address policy to enrollment, agent, and client endpoints.
- Add Argon2id local users, login rate limiting, short-lived access tokens, and secure client credential handling.
- Enforce endpoint-by-endpoint role matrix with table-driven negative tests.

### 7. Metrics and inventory

Collect every 15 seconds:

- CPU, memory, load averages.
- Filesystem totals/usage.
- Network counters/rates.
- Uptime, OS, kernel, hostname, architecture, boot ID.

Use procfs/sysfs and standard Linux interfaces where practical. Handle counter resets, reboot, disappearing mounts, pseudo-filesystems, overflow, cardinality bounds, collection failure, offline buffering, minute aggregation, and seven-day retention.

### 8. Typed maintenance

Implement:

- Process listing.
- systemd service list/status/start/stop/restart.
- Bounded journald viewing.
- Safe disk-cleanup preview.
- Preview-ID/hash-bound cleanup execution.
- Reboot with target-bound short-lived confirmation.

Validate unit names and paths. Use direct argv, subprocess timeouts, process-group cancellation, bounded output, and distinct failure/timeout/transport/unknown states. Do not retry cleanup or reboot automatically.

### 9. Admin arbitrary commands

- Require admin, target, reason, bounded timeout, and confirmation.
- Use explicit arbitrary-command shell path only; never route typed operations through it.
- Bound live output.
- Persist output only by explicit opt-in with cap and truncation marker.
- Audit actor, target, reason, exact command, timing, result, and capture choice.
- Return `unknown` after unprovable completion; never auto-retry.

### 10. Reverse SSH lifecycle

Use native OpenSSH:

- Dedicated validator tunnel account/service.
- No shell, password auth, TTY, X11, agent forwarding, or unrestricted forwarding.
- Remote forwarding only; listeners bind loopback only.
- Restrict per-agent tunnel keys.

Session flow:

1. Admin requests session with agent, public key, reason, and TTL.
2. Validator authorizes and creates unpredictable session ID.
3. Agent installs temporary public-key authorization.
4. Agent opens reverse tunnel with per-agent tunnel credential.
5. Validator verifies readiness and session binding.
6. Client invokes local OpenSSH using local private key/agent and verifies host key.
7. Session closes on disconnect, TTL, cancellation, agent loss, or validator shutdown.
8. Agent removes temporary authorization.
9. Validator records metadata-only audit event.

Avoid port-allocation races. Reconcile expired keys and stale sessions after restarts. Test using isolated temporary `sshd`; never modify developer host SSH configuration.

### 11. Client CLI and TUI

Commands should cover:

```text
client login/logout
client agents list/show
client stats
client services list/status/start/stop/restart
client logs
client cleanup preview/run
client reboot
client exec
client ssh
client enroll-token
client users
client ssh-keys
client audit
```

Requirements:

- Human output and stable `--json`.
- Reliable non-zero failure exit codes.
- No ANSI on non-TTY unless forced.
- Interactive confirmations; explicit non-interactive confirmation flag.
- Never print access tokens/private keys.
- Delegate terminal behavior to installed OpenSSH.

TUI views: login, fleet, search/filter/sort, host detail, metrics history, services, logs, actions, SSH launch, role-appropriate audit, keyboard help, no-color mode, and destructive-action dialogs. Keep TUI thin over shared client API.

### 12. Packaging and operations

Provide:

- Agent and validator systemd units.
- Restricted tunnel `sshd` configuration.
- Account setup and permission matrix.
- CIDR/firewall examples.
- Enrollment, rotation, revocation, recovery, backup/restore, integrity, logging, upgrade, and uninstall docs.
- Security model and explicit validator-compromise warning.
- Compatibility matrix and troubleshooting.

Verify systemd hardening against root agent requirements; do not copy incompatible blanket restrictions.

### 13. Docker Compose E2E harness

- Multi-stage images for agent, validator, and client runner.
- Ubuntu 22.04, Ubuntu 24.04, and Debian 12 simulated agent hosts.
- systemd/OpenSSH where needed.
- Separate networks for outbound agents, validator, allowed client, and denied client.
- No exposed agent control ports.
- Dedicated restricted validator `sshd`.
- Ephemeral test CA, certificates, passwords, tokens, SSH keys, and SQLite DB per run.
- Health checks and deterministic waits; no arbitrary sleeps.
- Trap-based cleanup and failure log collection.

E2E flow:

1. Bootstrap validator/admin and CIDR policy.
2. Create enrollment tokens and enroll all agents.
3. Confirm outbound mTLS and no reachable agent control port.
4. Verify inventory and metrics.
5. Verify viewer/operator/admin matrix.
6. Exercise safe typed service/log/cleanup fixtures.
7. Run bounded admin command and reject operator command.
8. Open reverse SSH and verify local private-key use.
9. Verify loopback listener and temporary-key/tunnel cleanup.
10. Revoke agent and reject reconnect.
11. Restart agent/validator and prove no destructive replay.
12. Reject expired/reused token and denied CIDR.
13. Verify retention and audit metadata.
14. Collect diagnostics and destroy environment.

Docker cannot fully reproduce VM kernel, cgroup, firewall, reboot, or networking behavior. Keep documented VM smoke-test checklist before production.

### 14. Full verification and release gate

Run unit, integration, negative security, race, restart, timeout, duplicate, revocation, E2E, and amd64/arm64 build checks. Validate supported distro behavior in disposable environments where available.

## Acceptance criteria and verification

Repository should expose equivalent stable targets:

```bash
make generate
make generate-check
make fmt-check
make vet
make test
make test-race
make build
make build-linux
make test-e2e
```

Direct baseline:

```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/agent
go build ./cmd/validator
go build ./cmd/client
GOOS=linux GOARCH=amd64 go build ./cmd/...
GOOS=linux GOARCH=arm64 go build ./cmd/...
```

E2E lifecycle should be available through `make test-e2e`, with equivalent underlying commands:

```bash
docker compose -f integration/docker-compose.yml build
docker compose -f integration/docker-compose.yml up -d --wait
go test -tags=e2e ./integration/... -count=1 -v
docker compose -f integration/docker-compose.yml down -v --remove-orphans
```

Acceptance gate:

- Three binaries build from clean checkout.
- Protobuf generation is reproducible.
- Agent exposes no inbound control port.
- Invalid/revoked mTLS identities fail.
- Enrollment token reuse fails.
- Agent private key never leaves host.
- Client SSH private key never reaches validator/application API.
- CIDR policy rejects disallowed peers; forwarded headers cannot bypass it.
- Complete RBAC matrix passes.
- Arbitrary commands require admin, reason, timeout, and confirmation.
- Every privileged action has persisted audit metadata before execution proceeds.
- Output persistence is opt-in, bounded, and truncation-marked.
- Duplicate/expired jobs fail.
- Unsafe jobs never retry after uncertain completion.
- Typed maintenance contains no shell interpolation.
- Logs, processes, messages, and queues are bounded.
- Cleanup execution requires matching unexpired preview.
- Reboot requires target-bound short-lived confirmation.
- Metrics sampling, aggregation, retention, and offline memory bounds pass tests.
- Reverse SSH is loopback-only, no-shell, TTL-bound, and cleanup-tested.
- Restart does not replay uncertain destructive work.
- SQLite migration, integrity, backup/restore, and disk-failure behavior are tested.
- `go test -race ./...` passes.
- Tests do not modify host OpenSSH/systemd configuration.
- Documentation includes threat, recovery, and production-readiness warnings.

## Risks, edge cases, and rollback/cleanup

### Primary risks

- Validator compromise may grant fleet root.
- Root operation injection or unsafe replay.
- Reverse tunnel breakout.
- PKI key loss/compromise.
- SQLite disk exhaustion.
- Credential cloning after machine image/snapshot duplication.

### Required edge-case handling

- NAT/public-IP changes.
- Duplicate hostnames.
- Cloned stable agent identity.
- VM snapshot restores stale state.
- Clock skew and clock jumps.
- Duplicate/stale agent streams.
- Reboot disconnect before acknowledgement.
- Successful systemd operation with lost response.
- journald absence/difference.
- Mount/network counter reset during metrics collection.
- SQLite disk full or retention failure.
- Client disconnect during command output.
- SSH client exit without clean API closure.
- Temporary key editing races.
- Validator restart with allocated reverse ports.
- Missing CA passphrase during unattended restart.

### Rollback and cleanup

- Avoid destructive migrations in MVP.
- Keep previous binaries/config recoverable during deployment.
- Never silently regenerate missing CA.
- Agent uninstall removes service, certificates, tunnel key, temporary keys, and state only after explicit confirmation.
- Validator uninstall does not delete CA, DB, or audit data by default.
- SSH cleanup runs on exit, timeout, disconnect, and startup reconciliation.
- Never rollback by replaying root operations.
- Mark uncertain operations `unknown` for human inspection.
- E2E cleanup must execute even after failure and remove volumes/orphans.

## Deferred production-hardening gaps

User chose to retain minimal MVP scope. Document, do not implement now:

- No immediate revocation for already-issued client access tokens; short expiry limits exposure.
- Cloned agent credentials may impersonate same agent until manually detected.
- No signed release artifact workflow.
- Audit records are access-controlled but not tamper-evident.
- No security-alert delivery pipeline beyond audit views.
- No advanced fleet-wide quota policy.
- Certificate revocation uses validator state, not CRL/OCSP.
- Clock skew may disrupt enrollment, auth, and rotation.
- Single validator remains availability and control-plane failure point.

Required warning in documentation:

> MVP is suitable for controlled evaluation and limited internal deployment. It is not production-hardened for hostile multi-tenant environments. Before broad production rollout, add immediate session revocation, clone detection, signed releases, tamper-evident audit storage, security alerting, tested disk-pressure behavior, and formal PKI revocation.

## Ready-to-run implementation prompt

```text
Execute plan in `docs/plans/2026-07-13-silent-devops-mvp.md` from `/home/nst/GolandProjects/silent-devops`.

Create and maintain a `todo_write` list before editing. Use plan milestones as top-level todos and keep one item active. Use `code_intelligence_search` before broad exploration for non-trivial work. Use `code_intelligence_impact` before changing protobuf contracts, exported/shared APIs, schemas, migrations, authentication, RBAC, command dispatch, or SSH lifecycle code. If repository remains greenfield, confirm git/toolchain state and proceed.

Implement milestone-by-milestone using TDD for non-trivial and security-sensitive behavior. Keep one Go module with `cmd/agent`, `cmd/validator`, and `cmd/client`. Prefer standard library and native Linux/OpenSSH/systemd behavior. Preserve every security invariant, scope exclusion, and fail-closed rule in plan. Do not broaden MVP or add speculative abstractions.

Docker is E2E infrastructure only. Build isolated Compose simulation with validator, client runner, Ubuntu 22.04, Ubuntu 24.04, and Debian 12 agents. Use ephemeral credentials, deterministic readiness, failure diagnostics, and trap-based cleanup. Do not expose agent control ports.

Run milestone checks continuously and final checks with fresh output:
- deterministic protobuf generation
- formatting
- `go vet ./...`
- `go test ./...`
- `go test -race ./...`
- all three binary builds
- Linux amd64/arm64 cross-builds
- `make test-e2e`
- negative security, restart, replay, revocation, retention, and cleanup tests

Stop only when all MVP acceptance criteria pass. If environment blocks checks, report exact command, exact error, affected criteria, and reproducible manual verification. Never weaken security controls to make tests pass. Never claim completion without fresh verification evidence.
```
