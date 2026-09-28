# T4 base merge

Combined base for task `t4-auto-migration` (task_01M3KQ1W22HP8PPBKN10HHCJVJ,
worktree `t4-migration`).

## Source commits

- T2 landing/completion: `701530da3d4108b7ab9903763941ac318ba3792e`
- T3 IR removal / plan-first default: `cfe679b39aab8beae1b301e3a4091a54b793fd7f`

Both were merged into this branch with `--no-ff`, starting from the frozen base
`3708751814501221f40b6b54667f6259f302aac3`.

Merge commits:

| Order | Merge commit | Parents (base, merged head) |
|---|---|---|
| 1 | `bc277f0fe99214eb356c97c07e6ea2d096de5ba0` | `3708751`, `701530d` |
| 2 | `142da5aa908c75f8e7687ad77e5cecd72edd439b` | `bc277f0`, `cfe679b` |

Both merges were clean at the git level: **no merge conflicts** were reported.

## Semantic (non-git) conflict

Combining the two accepted branches exposes one contract conflict in a test
expectation, not in a production file.

`TestCompleteNotApplicableToWorkflowWorkspaces`
(`internal/core/manual_completion_test.go:184`) created a second workspace with
`Create(ctx, CreateOptions{Title: "Pending", Input: "Choose later"})` and
asserted `operation_not_applicable` for `CompleteWorkspace`.

- T2 introduced `completeLanding`: a workspace whose snapshot declares the
  `landing` capability is refused by `workflow_gate` ("workspace must be in the
  integration phase to complete") rather than `operation_not_applicable`.
- T3 made plan-first the default for every create without
  `--workflow`/`--no-workflow`, and new plan-first selections snapshot the v2
  capability set, which includes `landing`.

The combination therefore turns the former "pending selection" workspace into a
plan-first v2 workspace that hits the new gate. Observed before the fix:

```text
--- FAIL: TestCompleteNotApplicableToWorkflowWorkspaces (0.15s)
    manual_completion_test.go:201: wanted operation_not_applicable, got workflow_gate: workspace must be in the integration phase to complete
FAIL
FAIL	workspace/internal/core	0.154s
```

Evidence: `evidence-before-fix.txt`.

### Resolution

Only the test expectations were adjusted (`internal/core/manual_completion_test.go`),
no production behavior. The test now asserts the combined contract explicitly:

1. an extended/custom workflow fixture (the shared `fixture`, which declares
   `change_request`/`release`) -> `operation_not_applicable`;
2. a new plan-first v2 selection (default create, snapshot contains `landing`)
   outside the integration phase -> `workflow_gate`;
3. a v1 plan-first snapshot forced to `legacySnapshotCapabilities("plan-first")`
   (no `landing`) -> `operation_not_applicable`;
4. the archive check is unchanged: a workflow that declares `release` still
   fails archive with `release_required`.

Evidence: `evidence-after-fix.txt`.

## Verification

```text
git merge-base --is-ancestor 701530da3d4108b7ab9903763941ac318ba3792e HEAD  # exit 0
git merge-base --is-ancestor cfe679b39aab8beae1b301e3a4091a54b793fd7f HEAD  # exit 0
gofmt -l ./cmd ./internal                                                    # exit 0, empty
go vet ./...                                                                 # exit 0
env -u WORKSPACE_AGENT_ID -u WORKSPACE_SESSION_ID -u WORKSPACE_RUN_ID \
  go test -count=1 ./... -timeout 570s                                        # exit 0
```

Evidence files: `evidence-merge-base.txt`, `evidence-gofmt.txt`,
`evidence-vet.txt`, `evidence-go-test-all.txt`, `evidence-before-fix.txt`,
`evidence-after-fix.txt`, `CHECKS.log`, `CHECKS.yaml`.

## Notes

- The work-product directories from the merged branches are preserved unchanged
  under `work-products/` (T0/T1 reports under `t0-tests/` and `t1-core/`). This
  task's products live under `work-products/t4-base/` and do not overwrite them.
- `git diff` against the last merge commit (`142da5a`) touches only
  `internal/core/manual_completion_test.go`.
