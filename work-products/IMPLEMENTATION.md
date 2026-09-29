# IMPLEMENTATION — T1: Core Issue helper refactor and Issue-evaluation domain

Task: `task_01M3P9C2JNHFQC04ASAP33ZQ7T`
Plan: `art_01M3P7H0BN7BZGZBK0YRK8YDXD/PLAN.md` (§4.1, §4.2 pending marker, §4.4 core,
§4.5, §4.7)
Worktree: `impl-core` · Base: `3031ccd` · Commit: `87bc342`

No launch logic, CLI or menu was added in this task (those are T3).

## Changes

### 1. Durable Issue-evaluation state (`internal/core/model.go`, §4.1)
- Added `Workspace.IssueEvaluation *IssueEvaluation` (`issue_evaluation`, omitempty),
  nil for every unlinked Workspace. Because `Status` embeds `Workspace`, it is visible
  through `workspace status --json` with no further work.
- Added the `IssueEvaluation` struct with `ID`, `IssueID`, `State`
  (`pending`/`recorded`), `RequestedAt`, `Outcome`, `Reason`, `IssueAction`,
  `IssueRevision`, `EvaluatedBy`, `RunID`, `EvaluatedAt` and `LaunchError`.

### 2. Pending marker on completion (`internal/core/completion.go`, §4.2)
- `markIssueEvaluationPending` is called by both `completeManual` and
  `completeLanding` immediately before the single `saveDocument`. It is a no-op when
  `input.issue_id` is empty.
- When linked it writes a fresh `ieval_...` pending evaluation and a
  `## Issue evaluation` body note in the **same revision** as `status: completed`.
  The mutate receipt replays it idempotently.

### 3. `UpdateIssue` refactor (`internal/core/issues.go`, §4.5)
- `UpdateIssue` keeps its validation and `authorizeProjectActor` call, then takes the
  project lock and delegates to the new
  `updateIssueLocked(selector, opt) (Issue, error)`.
- `updateIssueLocked` contains the original lock body (WAL recovery, receipt
  replay/reconcile, revision guard, write, receipt) and performs **no** authorization.
  Public behavior and the existing tests are unchanged.

### 4. `RecordIssueEvaluation` (`internal/core/issue_evaluation.go`, §4.4 core only)
- `IssueEvaluationOptions{Outcome, Reason, ExpectedRevision}` and
  `RecordIssueEvaluation(ctx, selector, opt, key)`.
- Runs through `mutate(..., request, &out, authorize, fn)` and therefore takes the
  project lock once. Authorization is `authorizeIssueEvaluation`:
  `requireWorkspaceScope()` (project-scope actors `forbidden`) then `requireOrchestrator`
  (workspace orchestrator, including a conversation-only Run, or the user terminal).
  `rejectAutonomousAttestation` is intentionally not applied and no
  `--user-confirmed` is required.
- Outcomes table implemented in `issueEvaluationTransition`:
  - `open` + delivered -> `closed`/`closed`; + not_delivered -> `open`/`left_open`.
  - `deferred` + delivered -> `closed`/`closed`; + not_delivered -> `open`/`left_open`.
  - `closed` by this Workspace (its own `Delivered by workspace <ws> (evaluation ...)`
    marker in `status_reason`) + delivered -> `unchanged_closed`; + not_delivered ->
    `open`/`reopened`.
  - `closed` by anyone else -> `unchanged_closed` for both; the Issue is not touched.
- Stale-digest guard: delivered is refused with `issue_revised` when
  `Issue.Digest != input.issue_digest`; not_delivered is still allowed.
- Reason stored on the Issue is prefixed and bounded (2000 bytes, rune-safe):
  `<Delivered|Not delivered> by workspace <ws> (evaluation <ieval>): <reason>`.
- Writes: the Issue goes through `updateIssueLocked` with
  `ExpectedRevision = current.Revision` and the stable receipt key
  `issue-evaluation:<ws>:<ieval>`. The Workspace `IssueEvaluation` is set to
  `recorded` and saved by the same mutation flush (order: Issue write, then Workspace).
- Refusals: `workspace_not_completed`, `workspace_archived`,
  `operation_not_applicable` (unlinked / no pending evaluation), `invalid_outcome`,
  `reason_required`, `revision_conflict`, `issue_evaluation_recorded` (different
  recorded outcome/reason); same outcome+reason returns the current status idempotently.

### 5. Reopen clears the evaluation (`internal/core/reopen.go`, §4.7)
- `ReopenWorkspace` sets `IssueEvaluation = nil` with the other derived completion
  state. The Issue is not changed. The next completion creates a new pending `ieval` ID,
  so an earlier delivered closure can be reopened by a later not_delivered.

## Acceptance criteria mapping

| Criterion | Evidence |
|---|---|
| `UpdateIssue` behavior/tests unchanged; orchestrator still `forbidden` on `UpdateIssue` | `TestUpdateIssueAuthorityUnchangedForOrchestrator`; existing `TestFirstClassIssueIntakeRevisionAndFrozenWorkspace`, `TestIssueStatusGuardAndDamagedSiblingTolerance` pass |
| Linked manual / plan-first (nothing-to-integrate and landed) completion sets `pending` in the same revision; unlinked gets none | `TestCompletionMarksPendingEvaluation` (manual, unlinked, plan-first nothing-to-integrate, plan-first landed) |
| Outcomes table, stale-digest refusal, validation, not-completed/unlinked/missing/archived/revision/already-recorded | `TestRecordIssueEvaluationOutcomesAndAuthorization`, `TestRecordIssueEvaluationRefusals`, `TestRecordIssueEvaluationStaleDigestGuard` |
| Authorization: orchestrator incl. conversation-only succeeds, worker/project forbidden, stale `stale_actor`, user succeeds | `TestRecordIssueEvaluationOutcomesAndAuthorization` subtests |
| Idempotency: replay no new Issue revision, different payload `operation_conflict`, crash converges | `TestRecordIssueEvaluationIdempotencyAndConvergence` |
| Reopen clears; re-completion new pending ID; not_delivered after earlier delivered reopens | `TestIssueEvaluationReopenCycle` |
| No deadlock under the 10 s lock timeout | `TestRecordIssueEvaluationIdempotencyAndConvergence/no_deadlock_under_the_lock_timeout` (bounded 5 s context) |
| gofmt, `go test ./...`, `go vet ./...` | checks below |

## Checks

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l ./cmd ./internal` | 0 (clean) | `evidence-t1-eval-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-t1-eval-vet.txt` |
| targeted core tests (new + existing Issue/completion tests) | 0 | `evidence-t1-eval-targeted.txt` |
| `go test ./... -count=1 -timeout 570s` | 0 | `evidence-t1-eval-go-test-all.txt` |

The workspace orchestration exports `WORKSPACE_SESSION_ID`/`WORKSPACE_AGENT_ID`, which
the suite's in-process helper (`TestWorkerProcess`) interprets as a child-process
invocation. The commands therefore unset the `WORKSPACE_*` variables (as earlier
`work-products/CHECKS-*.yaml` do); this is an environment artifact, not a code issue.

## Risks / notes

- The evaluation is recorded by the completed Workspace's orchestrator (or the user
  terminal as recovery). It deliberately does not require `--user-confirmed`, per the
  accepted plan assumptions.
- A Workspace completed before this change has no `issue_evaluation` and
  `RecordIssueEvaluation` returns `operation_not_applicable`; the existing manual
  `issue update` path still works for it. No migration is performed.
- "Closed by this Workspace" is detected from the Workspace link written into the
  Issue `status_reason`; a manual edit of that reason makes the closure look like it
  belongs to someone else, which is the same safety outcome as a revision mismatch.
- Older binaries cannot decode a Workspace carrying `issue_evaluation` (strict YAML);
  this is the same trade-off as earlier optional fields. The Issue schema is untouched.
- The outcome reason is truncated to 2000 bytes on a rune boundary so frontmatter stays
  small.
