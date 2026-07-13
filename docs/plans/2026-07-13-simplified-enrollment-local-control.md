# Simplified Enrollment and Validator-Local Control Implementation Plan

> **REQUIRED SUB-SKILL:** Use executing-plans skill to implement this plan task-by-task.

**Goal:** Add pinned one-command agent enrollment and protected validator-local control while keeping remote client optional.

**Architecture:** `agent join` verifies validator certificate pin before sending secrets, generates local Ed25519 key, enrolls with validator-assigned stable random ID, atomically stores credentials/config, and optionally starts systemd service. Validator exposes same Fleet gRPC API over protected Unix socket; local peer authorization maps root or configured admin-group members to local admin claims and preserves audit path.

**Tech Stack:** Go, gRPC, Unix domain sockets, Linux `SO_PEERCRED`, systemd, protobuf, SQLite.

---

### Task 1: Validator-assigned enrollment identity

**Files:** `api/devops/v1/devops.proto`, generated files, `internal/server/enrollment.go`, tests, integration helper.

1. Add failing contract/server tests for CSR without identity and validator-generated ID.
2. Generate random stable ID server-side; sign CSR certificate using ID; store hostname metadata separately.
3. Regenerate protobuf files and run enrollment/contract tests.

### Task 2: Pinned agent join

**Files:** `internal/agentjoin/**`, `cmd/agent/main.go`, tests.

1. Add failing tests for missing pin, pin mismatch before request, local key generation, response identity validation, atomic credential/config writes, overwrite refusal.
2. Implement TLS certificate probing and constant-time SHA-256 pin verification.
3. Implement enrollment client and `agent join VALIDATOR TOKEN --pin ...` command.
4. Add `--credential-dir`, `--hostname`, `--no-start`; never persist/log token.

### Task 3: Protected validator Unix socket

**Files:** `internal/localcontrol/**`, `internal/runtime/config.go`, `internal/runtime/validator.go`, `cmd/validator/main.go`, tests.

1. Add failing tests for socket mode, root/admin-group access, denied peers, local claims, and CLI calls.
2. Serve FleetService on Unix socket with peer-credential transport credentials/interceptor.
3. Add local validator commands for enrollment tokens, agents, metrics, and audit.
4. Ensure commands use service APIs, not direct SQLite.

### Task 4: Packaging and detailed installation

**Files:** `packaging/install.sh`, systemd units, uninstall script, `README.md`, `docs/installation.md`.

1. Support role-specific installation (`agent`, `validator`, optional `client`).
2. Create `silent-devops` service user and `silent-devops-admin` group; runtime dir/socket permissions.
3. Document full validator bootstrap, certificate pin retrieval, ready-to-copy token command, one-command agent join, service start, local control, remote client, upgrade, rollback, verification, and troubleshooting.

### Task 5: Real lifecycle E2E and release gate

**Files:** integration Compose/helper/tests, CI if required.

1. Replace integration enrollment helper path with actual `agent join` command.
2. Exercise validator-local CLI through Unix socket.
3. Verify duplicate hostname gets distinct stable IDs, pin mismatch sends no token, credential overwrite refusal, restart/reconnect, and existing security lifecycle.
4. Run `make generate-check fmt-check vet test test-race build build-linux test-e2e`.
5. Commit and push feature branch only after fresh gate passes.
