# Complete Client CLI and TUI Design

## Goal

Complete every advertised client command and provide a Bubble Tea terminal dashboard so routine fleet operation needs no command memorization.

## UX

Running `silent-devops-client` without arguments launches TUI. Explicit `tui` remains supported. Scriptable CLI and stable JSON remain available.

Desktop layout has header, panel tabs, searchable fleet sidebar, selected panel, and keyboard status bar. Panels: Fleet, Host, Metrics, Services, Logs, Users, SSH Keys, Audit, Actions, Help. Narrow terminals use one column. Keys: arrows navigate, Enter selects, `/` searches, Tab changes panel, `r` refreshes, `?` opens help, Esc goes back, `q` quits.

Login appears when credentials are absent or expired. Password input is masked and cleared after each attempt. Errors remain visible without replacing last successful data. No-color mode preserves hierarchy.

## Architecture

One `clientapi.Adapter` powers CLI and TUI. Bubble Tea commands perform asynchronous RPCs and return typed messages. Global model owns auth, terminal dimensions, selection, navigation, theme, and status. Panels own loading, last data, sanitized errors, timestamps, and pagination.

Existing Fleet RPCs are reused. Missing protocol calls are added only where required. Every command shown in help must work; placeholders and `command not wired` responses are forbidden.

## Safety

Server RBAC remains authority. Destructive actions require target, reason, impact display, and explicit confirmation. Uncertain destructive operations are never retried. Secrets never enter logs or rendered errors. SSH delegates to native OpenSSH; private keys remain local.

## Testing

Unit tests cover CLI validation, RPC mapping, JSON, redaction, TUI navigation, filtering, resize, no-color, loading/errors, role visibility, dialogs, and terminal exit. Docker E2E uses real roles and disposable fixtures.

Live-server verification is read-only: login/logout, inventory, metrics, service list/status, bounded logs, cleanup preview, users, SSH-key inventory, audit, and TUI navigation. Never execute live reboot, delete, cleanup run, service mutation, arbitrary exec, key/user mutation, revocation, or SSH session creation.

Completion requires full Makefile gate, no advertised placeholder, and sanitized live evidence.
