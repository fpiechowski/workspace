# Implementation report — task_01M3KJ6AW6M5TA821V5CFTJGW0 (T1 core plan-first v2)

## Commit

- `a76b8a6` — `feat(core): add plan-first v2 integration state machine` (all
  files below; work-products are committed separately).

Base: `3708751`.

## Changes per file

Production:

- `internal/core/workflow_capabilities.go`
  - Added `capLanding = "landing"`.
  - Split the former `defaultWorkflowCapabilities` into
    `legacySnapshotCapabilities(id)` (v1 fallback for persisted snapshots with an
    empty capability list; keeps issue-resolution's extended set) and
    `builtinWorkflowCapabilities(id)` (plan-first v2 set: `tasks`, `phases`,
    `tasks.role.planner`, `tasks.role.implementer`, `tasks.role.integrator`,
    `planner_dependency`, `integration`, `landing`).
  - `workflowConfigCapabilities`: for `plan-first` returns
    `union(configured, builtin)` so a project config listing the v1 set cannot
    drop the mandatory integration/landing stage; other ids keep
    "configured, else nothing". Added `unionCapabilities`.
  - `knownWorkflowCapability` accepts `landing`.
- `internal/core/workflow_model.go`
  - `Integration` gained `Target`, `Strategy`, `Landing`; added the `Landing`
    receipt type (`State`, `Target`, `Before`, `After`, `UserConfirmed`,
    `LandedAt`). Landing logic itself is T2.
- `internal/core/workflow.go`
  - `workflowPhases` includes `integration`; added `integrationPhase(d)`
    (`integration` for landing workflows, `integrating` otherwise).
  - Added `advancePlanFirstIntegrated`: planning → plan_review → implementing →
    integration; plan_review keeps the v1 dependency/retirement gate; implementing
    advances to `integration` once every live implementer is accepted (or all
    implementers are retired); `integration` returns `user_decision_required`
    naming the next steps; `completed` is rejected.
  - `advance` now dispatches three ways by capability: `landing` → the new
    machine, no `integration` → compact v1 machine, else the extended machine.
- `internal/core/integration.go`
  - `IntegrationOptions` gained `Target`.
  - Preparation phase check uses `integrationPhase(d)`.
  - Landing workflows default the base to the current tip of the target branch
    (default target = `Base.Ref`), validate the target with
    `git check-ref-format --branch` (leading `-` → `invalid_target`), record
    `Target`/`Strategy=merge`, and include both in the prepare request digest and
    manifest. `--base` still overrides.
- `internal/core/task.go`
  - Integrator retry resets the phase via `integrationPhase(d)`.
- `internal/core/routing.go`
  - `workflowProfile` falls back to the workflow's `implementation` profile for
    the integrator role when no `integration` profile is mapped.
- `internal/core/project.go`
  - `ValidateConfig`: `landing` requires `integration` and
    `tasks.role.integrator` and is rejected together with any of
    `change_request`, `live_test`, `release` (`invalid_config`).
  - A `workflows.issue-resolution` entry is validated as before and then deleted
    from the normalized config, so listing/selection/routing/capability lookup
    cannot observe it; the file on disk is untouched.
- `internal/cli/workflow.go`, `internal/cli/help.go`
  - `integration prepare` gained `--target` and updated help text/flag docs.

Tests:

- `internal/core/plan_first_v2_test.go` (new): v2 snapshot union with a v1
  config; implementing→integration gated on accepted live implementers;
  `advance` in integration → `user_decision_required`; v1 snapshots (explicit
  and empty capabilities) still complete after implementation; integration
  prepare defaults base to the target tip, honors `--target`, rejects invalid
  targets and records `Target`/`Strategy`; integrator acceptance records
  `Integration.HeadCommit`; landing config validation and hidden
  issue-resolution entry; capability union and integrator profile fallback.
- `internal/core/workflow_requirements_test.go`: plan-first now declares the
  integration capability (prepare gate is `workflow_gate`); the retired
  implementer test now advances to `integration` instead of `completed`.
- `internal/core/routing_test.go`: the profile/parallel-limit and orchestrator
  override tests use a plan-first workspace/config (the issue-resolution config
  entry is now hidden by `ValidateConfig`). T0 also rewrites these tests to the
  custom `extended` workflow; this is a known, intentional overlap.

## Acceptance criteria

1. New plan-first selection snapshots the v2 set even with a v1 config;
   v1 snapshots (explicit or empty) still complete on the implementing advance.
   Covered by `TestPlanFirstV2SnapshotsCapabilitiesAndGates`,
   `TestV1PlanFirstSnapshotStillCompletesAfterImplementation`,
   `TestWorkflowConfigCapabilitiesUnionsPlanFirstV2`.
2. plan-first v2 advances implementing → integration only after every live
   implementer is accepted; advance in `integration` returns
   `user_decision_required`. Covered by
   `TestPlanFirstV2SnapshotsCapabilitiesAndGates` and
   `TestPlanFirstIntegratorAcceptanceRecordsHead`.
3. `integration prepare` works in phase `integration`, defaults base to the
   target tip, accepts `--target`, rejects invalid targets; integrator
   acceptance records `Integration.HeadCommit`; extended-machine tests
   unchanged. Covered by `TestIntegrationPrepareTargetsTipForLandingWorkflow`,
   `TestPlanFirstIntegratorAcceptanceRecordsHead`; extended tests
   (`internal/core/workflow_test.go`) still pass.
4. Landing config combinations validated; an issue-resolution entry loads and is
   invisible. Covered by `TestValidateConfigLandingRulesAndHidesIssueResolution`.
5. gofmt clean; `go vet ./...` and `go test ./...` pass; the targeted tmux race
   suite passes. See checks below.

## Checks (see work-products/CHECKS.log and evidence-*.txt)

- `gofmt -l ./cmd ./internal` — exit 0, empty.
- `go vet ./...` — exit 0.
- `go test -count=1 ./...` — exit 0 (worker `WORKSPACE_*` identity variables
  unset; they are inherited from the agent session and otherwise trip the
  child-process guard tests).
- `WORKSPACE_TMUX_TEST=1 go test -race -count=1 ./internal/core -run 'Workflow|Integration' -timeout 90s`
  — exit 0.

## Risks and deviations

- Intermediate template gap: the dedicated plan-first `prompts/integration.md.tmpl`
  is delivered by T3, so before T0/T3 merge an integrator session must use an
  existing prompt template. The v2 integrator acceptance test does this
  explicitly; production behavior is otherwise unchanged.
- `ValidateConfig` now hides `issue-resolution`. The legacy built-in listing /
  selection special case in `project.go` (`workflowAvailable`, `WorkflowNames`)
  is still present and is removed by T3, so `workflow list` can still surface the
  workflow while the on-disk template exists. The config entry itself is no
  longer observable by any consumer.
- Pre-existing environment failure, unrelated to this change: under
  `WORKSPACE_TMUX_TEST=1` on this machine,
  `internal/terminal.TestNavigatorRealPTYAndClientSelection` fails
  ("pseudo-TTY client did not attach"). It reproduces at the frozen base
  `3708751` in a clean checkout, so the full `WORKSPACE_TMUX_TEST=1 go test
  -race ./...` command is not green here; every other package passes.
- `routing_test.go` overlap with T0 (both touch the same lines). T0 merges first
  per the plan; the conflict is limited to the workflow name used by two tests.
