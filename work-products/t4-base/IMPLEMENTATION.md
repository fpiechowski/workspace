# Implementation — t4-base-prep

Task: `task_01M3KQ1W22HP8PPBKN10HHCJVJ` (prepare combined base for T4).
Worktree: `t4-migration`. Role: implementer.

## Goal

Merge the accepted T2 head `701530da3d4108b7ab9903763941ac318ba3792e`
(landing/completion) and T3 head `cfe679b39aab8beae1b301e3a4091a54b793fd7f`
(IR removal, plan-first default) into this branch, then resolve the one semantic
test conflict exposed by combining them.

## What changed

| File | Change |
|---|---|
| merge commit `bc277f0f` | `--no-ff` merge of T2 `701530d` into base `3708751` (clean, no conflicts) |
| merge commit `142da5aa` | `--no-ff` merge of T3 `cfe679b` on top (clean, no conflicts) |
| `internal/core/manual_completion_test.go` | updated `TestCompleteNotApplicableToWorkflowWorkspaces` to the post-T2/T3 combined contract (test expectation only) |

No production (non-test) file was modified by my change. `git diff` against the
last merge commit touches only `internal/core/manual_completion_test.go`.

### Test conflict and resolution

Combining T2 (`completeLanding` refuses `workflow_gate` for snapshots that
declare `landing`) with T3 (plan-first is the default for every create and new
plan-first selections snapshot the v2 `landing` capability) turned the test's
former "pending selection" workspace into a plan-first v2 workspace, so
`CompleteWorkspace` returned `workflow_gate` instead of
`operation_not_applicable`. The old test is not valid on the combined base.

The test now asserts the combined contract:

1. extended/custom workflow fixture (`change_request`/`release`) ->
   `operation_not_applicable`;
2. new plan-first v2 selection outside the integration phase -> `workflow_gate`;
3. v1 plan-first snapshot (`legacySnapshotCapabilities("plan-first")`, no
   `landing`) -> `operation_not_applicable`;
4. archive of a `release`-declaring workflow still fails with
   `release_required` (unchanged).

## Acceptance criteria

| Criterion | Status | Evidence |
|---|---|---|
| HEAD contains T2 `701530d` and T3 `cfe679b` as ancestors | met | `evidence-merge-base.txt` (both exit 0) |
| `go test -count=1 ./... -timeout 570s` passes | met | `evidence-go-test-all.txt` (exit 0) |
| `gofmt -l ./cmd ./internal` empty | met | `evidence-gofmt.txt` (exit 0, empty) |
| `go vet ./...` passes | met | `evidence-vet.txt` (exit 0) |
| Only test expectations adjusted; no production behavior changed | met | `git diff` vs merge head touches only `internal/core/manual_completion_test.go`; `CHECKS.log` (`git diff --check` exit 0) |
| BASE-MERGE.md documents every change with evidence, receipts exit 0 | met | `BASE-MERGE.md`, `CHECKS.yaml`, `CHECKS.log` |

## Commands run

```text
git merge --no-ff --no-edit 701530da3d4108b7ab9903763941ac318ba3792e   # exit 0, clean
git merge --no-ff --no-edit cfe679b39aab8beae1b301e3a4091a54b793fd7f   # exit 0, clean
git merge-base --is-ancestor <T2> HEAD                                  # exit 0
git merge-base --is-ancestor <T3> HEAD                                  # exit 0
gofmt -l ./cmd ./internal                                               # exit 0, empty
go vet ./...                                                            # exit 0
env -u WORKSPACE_AGENT_ID -u WORKSPACE_SESSION_ID -u WORKSPACE_RUN_ID \
  go test -count=1 ./... -timeout 570s                                  # exit 0
git diff --check                                                        # exit 0
```

Pre-fix reproduction of the semantic conflict (`evidence-before-fix.txt`, exit 1)
and post-fix targeted run (`evidence-after-fix.txt`, exit 0) are included.

## Deviations and risks

- Task products are stored under `work-products/t4-base/` so the merged
  work-product directories from T0/T1/T2/T3 are not overwritten
  (`work-products/IMPLEMENTATION.md` etc. already exist from the merged
  branches). The required artifact is submitted as `BASE-MERGE.md`.
- `WORKSPACE_TMUX_TEST=1` was not required by this task spec (require_checks:
  false; prepare-merge only) and was not run.
- No production behavior was changed; the only risk is the adjusted test
  expectation, which is verified by the full suite passing on the combined base.
