# Implementation report — task_01M3MJYR3CFG2SYPQ3Q4A39YS1 (I1: Autonomy state, creation, enable/disable, status)

## Commit

- `3af359c` — `feat: add workspace autonomy state and lifecycle`

## Changes

### Core state model (`internal/core/model.go`, `internal/core/workflow_model.go`)

- Added `Workspace.Autonomy *Autonomy` (`yaml|json:"autonomy,omitempty"`), so a
  non-autonomous document serializes exactly as before.
- Added `Autonomy` (`mode`, `state`, `enabled_at`, `enabled_revision`, `source`,
  `disabled_at`, `disabled_reason`, `report`) and `AutonomyReport` (`outcome`,
  `recommendation`, `artifact_ids`, `phase`, `integration_head`, `pending`,
  `revision`, `created_at`) types per PLAN §3.1.
- Added the omitempty `Decision` audit fields (`resolved_by`, `autonomous`,
  `subject`, `evidence`, `session_id`, `run_id`, `decided_at`) and a
  `Workspace.AutonomyRunning()` helper. The minimal Decision extension landed
  here so `autonomy.enabled`/`autonomy.disabled` can record their resolver; I2
  extends the gate mutations that consume the same fields.

### Creation (`internal/core/project.go`, `internal/core/issue_workspace.go`, `internal/core/issues.go`)

- `CreateOptions.Autonomous bool` (`json:",omitempty"`) joins the idempotency
  payload without changing existing digests.
- `createWorkspaceLegacy` refuses autonomous creation with
  `autonomy_unsupported` when the resolved orchestrator profile has no route
  whose client declares the `deliver` capability, then persists
  `autonomy.state=running`, `source=create`, `mode=autonomous` for both
  plan-first and `--no-workflow` creation.
- `IssueDispatchOptions.Autonomous` is threaded into the linked workspace
  creation. `DispatchIssue` refuses a project-scoped (Dispatcher) actor with
  `forbidden` before `dispatchIssue` clears the actor; a user dispatch succeeds.

### Lifecycle (`internal/core/autonomy.go`, `internal/core/reopen.go`)

- New `EnableAutonomy`: `requireUser` (agents always get
  `user_decision_required`), refuses `needs_workflow`, completed and archived
  workspaces and an existing `PendingDecision`, enforces the expected revision,
  requires a deliver-capable orchestrator route, then appends an
  `autonomy.enabled` decision in the same mutation.
- New `DisableAutonomy`: user or orchestrator with `--user-confirmed`,
  requires an autonomy record, a reason and the expected revision; records
  `disabled_at`/`disabled_reason` and appends an `autonomy.disabled` decision.
- Shared `resolvedOrchestratorProfile`/`requireAutonomySupport` helpers.
- `ReopenWorkspace` now clears `Autonomy`; the full pre-reopen document
  (including the autonomy record) is already preserved under
  `history/reopen_ID/WORKSPACE.md`.

### Interfaces (`internal/cli/*`, `internal/tui/*`)

- `workspace create --autonomous`, `workspace issue dispatch --autonomous`, and
  new `workspace autonomy enable|disable` commands (with `--reason`,
  `--expected-revision`, `--user-confirmed`) plus help/flag documentation.
- TUI create form gained an `Autonomous run` confirm; `ActionCall.Autonomous`
  is translated by `createOptions` into `CreateOptions.Autonomous`.
- `status --json` and the workspace document expose autonomy automatically via
  the `Workspace` projection.

## Acceptance criteria

- Autonomy types exist as designed; a non-autonomous `CreateOptions`, `Workspace`
  and `Decision` keep their exact JSON/YAML shape and a created non-autonomous
  `WORKSPACE.md` contains no `autonomy:` front matter (targeted test).
- `create --autonomous` persists `state=running`, `source=create` for plan-first
  and manual; replays idempotently under the same key; a changed flag under the
  same key returns `operation_conflict`. The TUI form passes `Autonomous`.
- Dispatcher dispatch with `Autonomous` returns `forbidden`; a user dispatch
  creates a running autonomous workspace.
- Creation and enable return `autonomy_unsupported` without a deliver-capable
  orchestrator route.
- `autonomy enable` refuses agent actors, `needs_workflow`, completed, archived
  and pending-decision workspaces; revision guard and receipt replay work; it
  appends `autonomy.enabled`.
- `autonomy disable` works for the user and attesting orchestrator, records the
  reason, replays idempotently and appends `autonomy.disabled`.
- `reopen` clears autonomy while the history copy retains `state: running`;
  `status --json` exposes autonomy.

## Checks

See `work-products/CHECKS.yaml`. All commands exited 0: gofmt clean,
`go vet ./...`, the two targeted autonomy test runs, and the full
`go test ./... -count=1 -timeout 570s`.

## Risks and deviations

- Documentation (README/PRODUCT/ARCHITECTURE/docs) is intentionally deferred to
  I4 per PLAN §5, whose file list owns those documents; I1 changes no doc file.
- The `Decision` audit fields landed in I1 (allowed by the task) so the lifecycle
  decisions can record `ResolvedBy`; I2 should reuse them rather than duplicate.
- `autonomy report`, the safety boundary (`autonomy_excluded`) and the menu are
  I3 scope and are not implemented here.
- The full tmux race suite was not required for I1; the complete non-race
  package suite was run instead.
