# SUMMARY — T4: documentation contract for automatic Issue evaluation

Task: `task_01M3P9EWCRRSBEK0XH8F9Y83D9` · Worktree `impl-docs` · Base `5d7d489`
· Commit `e2c28da`

## What shipped
The documentation contract for the automatic linked-Issue evaluation (T1–T3),
with no production-code change:

- `docs/trackers.md` — new "Automatic Issue evaluation on Workspace completion"
  section (trigger, actor, outcomes table, stale-digest guard, no-tracker-write).
- `ARCHITECTURE.md` — narrow `authorizeProjectActor` exception for the completed
  Workspace's own orchestrator and the `issue_evaluation` Workspace state.
- `PRODUCT.md` — scope bullet and Issue-section paragraph.
- `README.md` — `issue-evaluation record` reference and
  `complete --no-issue-evaluation-start`, plus a corrected Issue-authority
  sentence.
- `docs/operations.md` — post-commit launch key and the two-file, idempotent,
  crash-convergent Issue/Workspace write contract.
- `docs/runtime.md` — the conversation-only Run exception for
  `issue-evaluation record`.
- `internal/core/templates/dispatcher.AGENTS.md.tmpl` — the Dispatcher must not
  close an Issue whose linked Workspace has a pending evaluation.
- `TODO.md` — checked item for the shipped feature.

## Acceptance
All four task acceptance criteria are met: every listed document describes the
implemented contract, the docs state that the external tracker is never written
and describe the outcomes behavior and stale-digest guard, the repo doc link
check passes (`checked=34 broken=0`), and `go build ./...` passes with the work
committed in the task worktree.

## Checks
- `python3 work-products/check-doc-links.py` — exit 0, `checked=34 broken=0`.
- `go build ./...` — exit 0.
- `go vet ./...` — exit 0.
- `go test ./internal/core/ -run 'TestSkill|TestPrime|TestDispatcher|TestHelp'`
  — exit 0.

## Risks
- Docs only; behavior is unchanged and owned by T1–T3.
- The `issue_evaluation` field keeps the same strict-YAML downgrade trade-off as
  `autonomy` for a linked Workspace completed by this binary; documented and
  `omitempty` for unlinked Workspaces.
