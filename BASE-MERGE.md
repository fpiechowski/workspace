# T3 base merge

## Source commits

- T0 test decoupling: `ba86a61609851a5279f97322ef1a2849d26cb486`
- T1 plan-first v2 core: `1d168c49bd2fc8369fd2f22409263b548285331a`

Both commits were merged into this worktree with `--no-ff`, preserving each
accepted branch as an ancestor.

## Conflict resolution

- `internal/core/routing_test.go`: kept the T0 version of the two overlapping
  profile tests, including the `extended` custom workflow fixture and its
  workflow selection. The T1 integrator fallback coverage remains in
  `internal/core/plan_first_v2_test.go`, where it was added as a focused test
  and does not overlap the T0 test setup.
- `work-products/CHECKS.log`, `work-products/IMPLEMENTATION.md`, and
  `work-products/SUMMARY.md`: retained both reports under
  `work-products/t0-tests/` and `work-products/t1-core/`, respectively. These
  are worker artifacts only; no source file was selected or discarded for this
  conflict.
- The accepted branches also contain `work-products/CHECKS.yaml` and
  `work-products/checks.yaml`. This checkout is case-insensitive, so the
  lower-case duplicate was removed after the merge to avoid one physical file
  representing two tracked paths; the upper-case manifest remains.

No other merge conflicts occurred. The T1 evidence files and the T0/T1
check-manifest variants were retained.

## Verification

Commands were run from the merged worktree:

```text
git merge-base --is-ancestor ba86a61609851a5279f97322ef1a2849d26cb486 HEAD  # exit 0
git merge-base --is-ancestor 1d168c49bd2fc8369fd2f22409263b548285331a HEAD  # exit 0
gofmt -l ./cmd ./internal                                         # exit 0, empty
go vet ./...                                                       # exit 0
env -u WORKSPACE_AGENT_ID -u WORKSPACE_SESSION_ID -u WORKSPACE_RUN_ID \
  go test -count=1 ./internal/core -timeout 300s                    # exit 0
```

Evidence files are stored under `work-products/evidence-*-base.txt`.
