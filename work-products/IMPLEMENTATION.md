# IMPLEMENTATION — T3: automatic Issue-evaluation launch, CLI command and menu

Task: `task_01M3P9EKSWW97RC87SW2EA875V`
Plan: `art_01M3P7H0BN7BZGZBK0YRK8YDXD/PLAN.md` (§4.2 post-commit launch, §4.4 CLI,
§4.6 status/menu visibility)
Worktree: `impl-launch` · Base: `3031ccd` (includes T1 `87bc342` and T2 `7dbd583`)
Commit: `046b33b`

Scope: the post-completion orchestrator launch, its launch-error record, the
workspace-scoped CLI command and the menu visibility. The Issue-evaluation domain
(T1) and the prompt notice (T2) are consumed as-is. Documentation (T4) and the
optional TUI Issue-detail field (T5) are out of scope.

## Changes

### 1. Post-commit evaluation launch (`internal/core/completion.go`, §4.2)
- `CompleteOptions` gains `NoEvaluationLaunch bool`. It only suppresses the
  side effect; the pending evaluation is always still recorded by T1.
- `CompleteWorkspace` runs the whole `mutate` as before and, only after a
  successful commit, calls `launchIssueEvaluation`. The committed completion is
  never rolled back by a launch failure.
- `launchIssueEvaluation` returns early when `NoEvaluationLaunch` is set, when
  there is no pending `issue_evaluation`, or when the orchestrator already has an
  active Session. Otherwise `startIssueEvaluationRun` starts the conversation-only
  Run under the stable key `issue-evaluation:<ieval id>`.
- `startIssueEvaluationRun` is idempotent: a completion replay (or a user
  conversation already in progress) finds the active orchestrator Session and does
  nothing, so no second Run is created. When nothing is active it delegates to
  `StartOrchestrator`, which resumes an existing logical orchestrator Session or
  starts a new one.
- `recordIssueEvaluationLaunchError` is a small separate `With` mutation that sets
  `issue_evaluation.launch_error` on the still pending evaluation and returns the
  refreshed `Status`; the caller publishes that refreshed status instead of the
  commit result.

### 2. Menu visibility (`internal/core/workflow.go`, §4.6)
- `issueEvaluationMenuActions` adds
  `{"issue_evaluation", "Evaluate linked Issue delivery",
  "issue-evaluation record --outcome <delivered|not_delivered> --reason <reason>
  --expected-revision <revision>"}` for a completed Workspace with a pending
  evaluation, in both the manual and workflow completed branches.
- `completedConversationLabel` changes the completed-workspace conversation action
  label to "Start orchestrator to evaluate linked Issue" when a launch error is
  recorded; it stays "Start or resume conversation" otherwise.
- `workspace status --json` already exposes the whole `Workspace`, so
  `issue_evaluation` is visible with no further change.

### 3. CLI (`internal/cli/cli.go`, `internal/cli/help.go`, §4.4/§4.6)
- `workspace complete` gains `--no-issue-evaluation-start`, mapped to
  `CompleteOptions.NoEvaluationLaunch`.
- New workspace-level group `workspace issue-evaluation` with subcommand `record`
  (`--outcome`, `--reason`, `--expected-revision`, plus the global
  `--operation-key`). It calls `RecordIssueEvaluation` and prints the returned
  `Status`, so its JSON `data` equals `workspace status --json`.
- Help specs were added for both new command paths and for the new flag/options,
  keeping `TestHelpIsAvailableAtEveryCommandLevel` and the flag-spec checks green.

### 4. Test adaptations (`internal/core/issue_evaluation_test.go`)
- T1/T2's `completeLinkedWorkspace` helper now passes `NoEvaluationLaunch: true`.
  Those tests deliberately own the orchestrator lifecycle (they start it
  explicitly afterwards), so suppressing the new side effect keeps them focused
  and deterministic. All T1/T2 assertions are unchanged.

### 5. New tests
- `internal/core/issue_evaluation_launch_test.go`:
  - completion of a linked manual Workspace starts exactly one conversation-only
    orchestrator Run and re-completing with the same key does not start a second;
  - a forced launch failure (a Runtime whose `Launch` fails) still returns the
    committed `completed` status, records `issue_evaluation.launch_error`, leaves
    no active Run, and makes the menu expose the evaluation action with the
    launch-aware conversation label;
  - `NoEvaluationLaunch` leaves the evaluation pending with no Run, and a later
    `StartOrchestrator` retries and starts exactly one Run.
- `internal/cli/cli_test.go`:
  - `TestIssueEvaluationRecordJSONMatchesStatusAndReplays` links a Workspace via
    `create --from-issue`, completes it with `--no-issue-evaluation-start`, records
    the evaluation from the CLI, compares the record `data` with
    `workspace status --json`, and replays the same operation key.
  - `TestTUICompleteWorkspaceLaunchesEvaluation` drives
    `tui.CoreBackend.PerformAction("complete_workspace", ...)` against a real
    `core.Service` and a fake Runtime, asserting the same automatic launch and
    replay idempotency.
- `internal/core/tmux_integration_test.go`:
  - `TestTmuxLinkedCompletionStartsEvaluationOrchestrator` (opt-in) completes a
    linked Workspace against the real `Tmux` runtime and asserts a
    conversation-only orchestrator Run with a real pane starts.

## Acceptance criteria mapping

| Criterion | Evidence |
|---|---|
| Completing a linked Workspace starts exactly one active conversation-only orchestrator Run; replaying the complete key does not create a second | `TestCompleteLinkedWorkspaceLaunchesEvaluationOrchestrator`; `TestTUICompleteWorkspaceLaunchesEvaluation` |
| Forced launch failure still returns the committed completed status, sets `issue_evaluation.launch_error`, and the menu offers the evaluation action and conversation start | `TestCompleteLinkedWorkspaceLaunchFailureRecordsError` |
| `--no-issue-evaluation-start` skips the launch and leaves the evaluation pending | `TestCompleteLinkedWorkspaceNoEvaluationLaunch`; `TestIssueEvaluationRecordJSONMatchesStatusAndReplays` |
| The TUI `complete_workspace` path triggers the same launch, covered by a core-level test through `tui.Backend` | `TestTUICompleteWorkspaceLaunchesEvaluation` |
| CLI JSON of `issue-evaluation record` equals `workspace status --json`; `--operation-key` replays | `TestIssueEvaluationRecordJSONMatchesStatusAndReplays` |
| gofmt, `go test ./...`, `go vet ./...` pass; tmux suite passes with a new linked-completion case | checks below; environment caveat below |

## Checks

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l ./cmd ./internal` | 0 (clean) | `evidence-t3-launch-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-t3-launch-vet.txt` |
| targeted core+CLI tests (new launch/menu/CLI/TUI + T1/T2 + help) | 0 | `evidence-t3-launch-targeted.txt` |
| `go test ./... -count=1 -timeout 570s` | 0 | `evidence-t3-launch-go-test-all.txt` |
| `WORKSPACE_TMUX_TEST=1 go test ./internal/core/ -run TestTmuxLinkedCompletionStartsEvaluationOrchestrator` | 0 | `evidence-t3-launch-tmux-targeted.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ ./internal/tui/ -timeout 420s` | 0 | `evidence-t3-launch-tmux-race.txt` |

The orchestration exports `WORKSPACE_*`, which the suite's in-process
child-process helper (`TestWorkerProcess`) interprets as a nested invocation, so
the commands unset the `WORKSPACE_*` variables (as the T1/T2 checks do). This is
an environment artifact, not a code issue.

### Known environment caveat: `WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s`

On this sandbox that exact command cannot pass, for two reasons that pre-date T3
and reproduce on the base commit `c5c78da` (evidence
`evidence-t3-launch-base-regression-check.txt`, which prints `FAIL` for both):

1. `workspace/internal/core` needs 153 s on the base commit and 185 s with T3
   under `-race` + tmux on this WSL `/mnt/c` filesystem, so the 90 s per-package
   timeout fires. The run above shows the cap being hit (`FAIL ... 90.017s`).
2. `workspace/internal/terminal.TestNavigatorRealPTYAndClientSelection` fails on
   the base commit too (`pseudo-TTY client did not attach`), independent of T3.

The same suite passes with a realistic timeout: see `evidence-t3-launch-tmux-race.txt`
(core+tui, exit 0) and the isolated new tmux case
(`evidence-t3-launch-tmux-targeted.txt`, exit 0). The exact 90 s invocation and the
base comparison are preserved as `evidence-t3-launch-acceptance-90s.txt` and
`evidence-t3-launch-base-regression-check.txt`.

## Risks / notes

- The launch is best effort by design. A failing launch never fails completion;
  `launch_error` plus the menu action and `workspace start` provide recovery. A
  replayed `complete` key retries the launch only when no orchestrator is active.
- The idempotency key is `issue-evaluation:<ieval id>`, not the user's completion
  key, so retries and `workspace start` converge on one evaluation Run. When the
  orchestrator Session is already active, the launch is a no-op rather than a new
  Run.
- The launch is triggered from `CompleteWorkspace`, so the CLI and the TUI
  `complete_workspace` action share it; the TUI backend was not changed.
- `recordIssueEvaluationLaunchError` is a second small write after the completion
  commit, so the workspace revision advances by one more when a launch fails. The
  evaluation-record command is revision-guarded against `status`, so this is safe.
- No documentation (T4) was changed in this task.
