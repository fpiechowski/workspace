# SUMMARY — T5: Evaluation state in project overview link and TUI Issue detail

Task `task_01M3P9E8MAVA0KN5WP767JJ5FB` implemented on top of T1 in the `impl-tui`
worktree. Commit `9958a07` (base `d0bb317`).

## What changed
- `IssueWorkspaceLink` and `WorkspaceSummary` now carry `EvaluationState` and
  `EvaluationOutcome`, populated from the Workspace `issue_evaluation`.
- `ProjectOverview` fills the workspace row and the per-Issue linked-workspace entries.
- The TUI Issue detail renders `· evaluation <state>` (and `(<outcome>)` when recorded)
  on each linked Workspace bullet; an un-evaluated Workspace is unchanged.

## Verification
- `gofmt -l ./cmd ./internal`: clean.
- `go vet ./...`: pass.
- Targeted new tests (`TestReadModelsCarryLinkedEvaluationState`,
  `TestIssueDetailShowsLinkedWorkspaceEvaluation`): pass.
- `go test ./... -count=1 -timeout 570s`: pass.

See `IMPLEMENTATION.md` and `CHECKS-T5-EVAL.yaml`; evidence files:
`evidence-t5-eval-gofmt.txt`, `evidence-t5-eval-vet.txt`,
`evidence-t5-eval-targeted.txt`, `evidence-t5-eval-go-test-all.txt`.
