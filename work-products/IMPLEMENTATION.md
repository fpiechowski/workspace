# IMPLEMENTATION — T4: documentation contract for automatic Issue evaluation

Task: `task_01M3P9EWCRRSBEK0XH8F9Y83D9`
Plan: `art_01M3P7H0BN7BZGZBK0YRK8YDXD/PLAN.md` (§4.8 Documentation contract updates)
Worktree: `impl-docs` · Base: `5d7d489` (includes T1 `87bc342`, T2 `7dbd583`,
T3 `046b33b`/`5d7d489`)
Commit: `e2c28da` (`docs: describe automatic linked Issue evaluation on completion`)

Scope: the documentation contract for the automatic linked-Issue evaluation
implemented by T1–T3. No production code or tests changed. T2 already owns the
prompt notice, the orchestrator/workflow templates and the skill text; this task
adds the `docs/` and top-level contract documents plus the Dispatcher
instruction.

## Changes

| File | Change |
|---|---|
| `docs/trackers.md` | New section "Automatic Issue evaluation on Workspace completion": trigger and pending marker, the auto-started conversation-only orchestrator Run, `--no-issue-evaluation-start`, the actor and its narrow scope, the outcomes table, the reason prefix/bound, the stale-digest guard, recorded-evaluation immutability, reopen, legacy workspaces, and the explicit "never writes the external tracker" statement. |
| `ARCHITECTURE.md` | "Project Issue and Dispatcher state" gains the narrow authority exception (own linked Issue only, orchestrator/conversation-only Run or user terminal, `authorizeProjectActor` unchanged, Dispatcher leaves a pending evaluation alone). The `WORKSPACE.md` state description gains `issue_evaluation` (pending → recorded fields, `omitempty`). |
| `PRODUCT.md` | New Product Scope bullet for automatic linked-Issue evaluation; a paragraph in the project-scope Issue section describing the automatic judgement, durability/idempotency/revision guard, stale-digest refusal, no-tracker-write, and the Dispatcher's restraint. |
| `README.md` | Issue-operations paragraph corrected (the orchestrator/user evaluate the linked Issue; workers cannot mutate project Issues) with a `docs/trackers.md` link; completion section documents `complete --no-issue-evaluation-start` and the `workspace issue-evaluation record` reference with `--outcome`, `--reason`, `--expected-revision` and `--operation-key`. |
| `docs/operations.md` | `issue-evaluation record` added to the workspace-mutation list; new "Automatic Issue evaluation" section describing the post-commit launch key `issue-evaluation:<ieval id>` and the two-file, single-critical-section write with the derived Issue receipt key `issue-evaluation:<ws_id>:<ieval_id>`, replay/conflict behavior, crash convergence, recorded immutability, stale-digest refusal, and no external effect. |
| `docs/runtime.md` | The conversation-only continuation paragraph now records the one additional allowed operation (`issue-evaluation record`) for a pending evaluation, the automatic start when no orchestrator is active, the supervisor's no-auto-restart rule, and the byte-for-byte unchanged prompt when there is no pending evaluation. |
| `internal/core/templates/dispatcher.AGENTS.md.tmpl` | Instruction text: the Dispatcher may still close local Issues but must not close an Issue whose linked Workspace has a pending evaluation. |
| `TODO.md` | New checked `[x]` item recording the shipped feature. |

## Acceptance criteria mapping

| Criterion | Where satisfied |
|---|---|
| Each document describes the implemented contract as in plan §4.8; docs describe the current contract, not a one-time state | All eight files above; contract statements verified against `internal/core/issue_evaluation.go`, `completion.go`, `session.go`, `workflow.go`, `internal/cli/cli.go` and the Dispatcher template. |
| Docs make no claim that the external tracker is written; the stale-digest guard and outcomes behavior are described | `docs/trackers.md` ("No step of this flow writes to the external tracker"; outcomes table; `issue_revised` guard), `ARCHITECTURE.md`, `PRODUCT.md`, `README.md`, `docs/operations.md`, `TODO.md` all state the external tracker is untouched. |
| All document links valid (run the doc link check used by the repo) | `python3 work-products/check-doc-links.py` → `checked=34 broken=0`, exit 0 (`evidence-t4-doc-links.txt`). |
| gofmt not applicable; `go build ./...` passes; commit work in the task worktree | No Go source changed; `go build ./...` exit 0 (`evidence-t4-build.txt`); commit `e2c28da`. |

## Checks

| Command | Exit | Evidence |
|---|---|---|
| `python3 work-products/check-doc-links.py` | 0 | `evidence-t4-doc-links.txt` |
| `go build ./...` | 0 | `evidence-t4-build.txt` |
| `go vet ./...` | 0 | `evidence-t4-vet.txt` |
| `go test ./internal/core/ -run 'TestSkill\|TestPrime\|TestDispatcher\|TestHelp' -count=1` | 0 | `evidence-t4-tests.txt` |

The `go` commands unset the exported `WORKSPACE_*` variables, which the suite's
in-process child-process helper would otherwise interpret as a nested invocation;
this matches the T1–T3 reports and is an environment artifact, not a code issue.

## Risks / notes

- Documentation only. The feature behavior and all code paths were implemented by
  T1–T3 and are consumed unchanged.
- `issue_evaluation` makes a completed linked Workspace unreadable by an older
  strict-YAML binary, the same trade-off as the earlier optional `autonomy` field.
  The docs state the `omitempty` behavior and the no-migration rule for legacy
  Workspaces; the Issue schema itself is untouched.
- English only; no links to external trackers were added.
