# Implementation report — task_01M3KJ6ZDVJF6R62YRRJDQP5KC

## Commit

- `8414095` — `feat(core): migrate legacy workflows on load`

## Changes

- `internal/core/workflow_migration.go`: added idempotent `issue-resolution` to
  `plan-first` migration. It maps legacy phases, installs the v2 capabilities,
  preserves task/integration/history state, defaults integration target and
  strategy, supersedes pending live-testing decisions, and stages the old
  snapshots plus a migration manifest through `PendingFiles`. Active workspaces
  receive plan-first snapshots using project templates with embedded fallback;
  completed and archived workspaces only receive the state rewrite.
- `internal/core/files.go`: invokes the migration during document load and saves
  it through the existing write-ahead recovery path.
- `internal/core/workflow_migration_test.go`: covers direct legacy fixture
  migration, state preservation, history staging, decision supersession, and
  second-load idempotence.

## Acceptance criteria

- Legacy workflow state is rewritten to plan-first v2 and mapped to the
  integration phase where required.
- Integration defaults, preserved history, cleared change-request mode, and
  superseded live-testing decisions are covered.
- Snapshot/history writes use `PendingFiles`, so existing `pending.json`
  recovery remains applicable; template failures do not prevent state rewrite.
- The second migration call is a no-op.

## Checks

| Command | Exit | Evidence |
|---|---:|---|
| `gofmt -l ./cmd ./internal` | 0 | `evidence-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-vet.txt` |
| `env -u WORKSPACE_AGENT_ID -u WORKSPACE_SESSION_ID -u WORKSPACE_RUN_ID go test ./... -count=1 -timeout 570s` | 0 | `evidence-go-test-all.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race -count=1 ./internal/core -run 'TestTmux|TestMigrateLegacyWorkflow' -timeout 90s` | 0 | `evidence-tmux-core.txt` |
| `go test ./internal/core -run TestMigrateLegacyWorkflowRewritesStateAndSnapshotsHistory -count=1` | 0 | terminal check |

## Risks and deviations

- The migration deliberately leaves completed and archived workspaces without
  snapshot/history file changes, as required by the terminal-state rule.
- The full repository test command was run with worker identity variables unset,
  matching the repository's existing CLI test requirement.
