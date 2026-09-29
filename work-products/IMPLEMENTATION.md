# IMPLEMENTATION — T5: Evaluation state in project overview link and TUI Issue detail

Task: `task_01M3P9E8MAVA0KN5WP767JJ5FB`
Plan: `art_01M3P7H0BN7BZGZBK0YRK8YDXD/PLAN.md` (§4.6 optional part, T5)
Worktree: `impl-tui` · Base: `d0bb317` · Commit: `9958a07`

Depends on T1 (`87bc342`, core `IssueEvaluation`) which is already in this worktree.

## Changes

### 1. Read models carry the evaluation (`internal/core/issues.go`, `query.go`, `model.go`)
- `IssueWorkspaceLink` gained `EvaluationState` and `EvaluationOutcome`
  (`evaluation_state` / `evaluation_outcome`, `omitempty`). Both are empty when the
  linked Workspace has no `issue_evaluation`.
- `WorkspaceSummary` (the project overview row) gained the same two fields, so the
  overview carries the evaluation without a second Workspace read.
- `ProjectOverview` populates `WorkspaceSummary.EvaluationState/Outcome` from
  `status.Workspace.IssueEvaluation` and copies them into each
  `IssueSummary.LinkedWorkspaces` entry.
- `linkedWorkspaces` (`issues.go`) copies the row fields into `IssueWorkspaceLink`, so
  `issue show` / `IssueDetail` carries them too.
- New unexported helper `linkedIssueEvaluation(*IssueEvaluation) (state, outcome string)`
  in `model.go`: a nil evaluation yields two empty strings, making "no evaluation"
  indistinguishable from Workspaces that predate the field (strict-YAML backward
  compatibility is preserved because the JSON/YAML shape only gains omitempty keys).

### 2. TUI Issue detail rendering (`internal/tui/detail.go`)
- Each "Linked workspaces" bullet now appends `· evaluation <state>` and, when an
  outcome exists, `(<outcome>)`, e.g. `Delivery · completed · revision 2 · evaluation
  pending` or `Shipped · completed · revision 2 · evaluation recorded (delivered)`.
- A linked Workspace with no evaluation renders the exact previous string, so the
  change is invisible for un-evaluated/legacy Workspaces.

## Acceptance criteria mapping

| Criterion | Evidence |
|---|---|
| `IssueWorkspaceLink` and project overview rows carry state and outcome | `TestReadModelsCarryLinkedEvaluationState` asserts pending then recorded/delivered on both `ProjectOverview.Workspaces` and `ShowIssue.LinkedWorkspaces`; empty before completion |
| TUI Issue detail renders the evaluation state for a linked completed Workspace; Workspace without `issue_evaluation` is unchanged | `TestIssueDetailShowsLinkedWorkspaceEvaluation` (pending, recorded+delivered, and an un-evaluated "Legacy" row) |
| Tests follow existing TUI/overview conventions | Core test reuses `fixture`/`linkedManualWorkspace`/`completeLinkedWorkspace`/`startEvaluationOrchestrator`; TUI test follows `project_scope_test.go` |
| gofmt on changed files; `go test ./...` and `go vet ./...` pass; commit in the task worktree | checks below; commit `9958a07` |

## Checks

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l ./cmd ./internal` | 0 (clean) | `evidence-t5-eval-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-t5-eval-vet.txt` |
| targeted `internal/core` + `internal/tui` tests | 0 | `evidence-t5-eval-targeted.txt` |
| `go test ./... -count=1 -timeout 570s` | 0 | `evidence-t5-eval-go-test-all.txt` |

The workspace orchestration exports `WORKSPACE_*` variables, which the suite's
in-process helper (`TestWorkerProcess`) interprets as a child-process invocation. The
commands therefore unset them, as earlier `work-products/CHECKS-*.yaml` do.

## Risks / notes

- The change is read-model only; no new mutation, schema-strictness or authority
  behavior is introduced. `IssueEvaluation` itself was added by T1.
- `evaluation_outcome` is rendered only when a recorded evaluation sets it; a pending
  evaluation shows just the state.
- Scope was held to the required surfaces (project overview row + TUI Issue detail).
  The Issues collection subtitle and the Workspace detail were intentionally left
  unchanged.
- `work-products/IMPLEMENTATION.md` and `SUMMARY.md` are tracked leftovers from the T1
  report; they are overwritten here as this task's reports, consistent with plan
  assumption 6.
