# Implementation: Auto-close completed worker Sessions

Task: task_01M3P785FNW7WPVAQ0NJKVHCX0
Plan: art_01M3P2E8D7AHJE7MHE36Q5CQPK (PLAN.md, planner head 59fa949)
Base: 3031ccd17917f9586d907382a5eb94efc5d2d61d
Commit: 248ae63 `feat: auto-close accepted worker sessions`
Worktree: `.workspace/ws_01M3P1NVNKTKBC53CQ2EGBYTVN/worktrees/impl-autoclose`

## 1. What changed

Accepting a worker handoff now records a durable close intent on the submitting
worker Session in the same atomic commit as the acceptance. After the commit a
best-effort settle step verifies pane ownership, stops the owned Run, and marks
the Session `closed`. Settle failures never fail the acceptance: they are stored
in `close_error` and retried by `Reconcile`, every supervisor tick, and the next
`StartSession`. A pane that is not verified as owned is never stopped. The
documented consultation resume of an accepted worker is preserved only for
Sessions kept with the new `--keep-session` opt-out.

Follows design D from PLAN.md sections 3-5 (T1 core, T2 CLI/help, T3 docs in one
worktree). No new dependencies, no unrelated refactors, stock prompt templates
untouched.

### Core (`internal/core`)

- `model.go`: added `Session.CloseRequestedAt *time.Time` and
  `Session.CloseError string` (both `omitempty`, additive; no schema bump).
- `session.go`:
  - `requestWorkerSessionClose(d, sessionID, reason, now)` records the intent.
    No-op for a missing/deleted/closed/orchestrator Session, for the acting
    Session itself, and when an intent is already pending.
  - `settleClosingSessionsLocked(ctx, d)` converges every Session with a pending
    intent and returns whether the document changed:
    - idle (no current Run or a terminated Run): clear `current_run_id`, set
      `closed_at`, keep `close_reason`, clear `close_error`, `syncSession`;
    - active Run with a recorded pane: `Inspect`; `pane_missing`/dead marks the
      Run `exited` (no "without a handoff" error) and closes; an owned pane calls
      `Runtime.Stop`, marks the Run `stopped` with `finished_at`, then closes; a
      pane that is not owned, a `launch_uncertain` Run without a pane, or any
      other inspect/stop error records `close_error` and leaves the intent
      pending;
  - `settleClosingSessions(ctx, selector)` is the `With` wrapper.
  - Wired into `StartSession` (before ownership and parallel-limit checks) and
    `Reconcile` (after the Runs loop, before save).
  - Resume validation treats a pending intent as closed
    (`session_closed`), the parent-Session guard does the same, and `ResumeAgent`
    clears `ResumeSession` so a closing latest Session starts fresh.
  - `CloseSession` clears a pending intent/error when a manual close converges.
- `handoff.go`: `ReviewHandoffAudited` gained a `keepSession bool` parameter. On
  the accept branch (unless keep) it records the intent before the commit; after
  the commit (including a replayed receipt) it calls `settleClosingSessions` and
  ignores its error. The keep flag joins the idempotency payload only when true,
  so existing receipts keep their digests.

### CLI (`internal/cli`)

- `work.go`: `--keep-session` on `handoff accept` only (not `reject`), passed
  through to `ReviewHandoffAudited`.
- `help.go`: accept long help and the flag description map updated.

### Docs

- `docs/runtime.md`: Session projection table gains
  `close_requested_at`/`close_error`; a new auto-close convergence paragraph; the
  consultation paragraph now requires `--keep-session`.
- `README.md`: one sentence each near `handoff accept` and `session close`.
- `PRODUCT.md`: one sentence in the session-lifecycle paragraph.
- `ARCHITECTURE.md`: durable close-intent sentence in the Session projections
  paragraph.
- Stock prompt templates (`prompts/*.tmpl`,
  `internal/core/templates/**/*.tmpl`) are untouched.

## 2. Acceptance criteria

| Criterion | Where |
|---|---|
| Accept closes the worker Session (`closed`, `closed_at`, `close_reason`), stops the owned Run, frees pane and slot, no manual stop | `TestAutoCloseAcceptClosesLiveWorker`, `TestAutoCloseReleasesParallelSlot` |
| Archive/autonomy/next start not blocked after last worker accepted | `TestAutoCloseUnblocksArchiveAfterLastWorkerAccepted`, `TestAutoCloseUnblocksAutonomyReport`, `TestAutoCloseReleasesParallelSlot` |
| Runtime failures never fail acceptance; durable `close_requested_at`/`close_error`; converge via Reconcile; unowned pane never stopped | `TestAutoCloseRuntimeFailureIsDurableAndConverges`, `TestAutoCloseNeverKillsUnownedPane` |
| Idempotent under replay and repeated settle; `ClosedAt` not rewritten | `TestAutoCloseIsIdempotent` |
| Rejected handoffs, orchestrator Sessions, `--keep-session` unaffected; kept Sessions still support consultation resume | `TestAutoCloseRejectLeavesSessionOpen`, `TestAutoCloseKeepSessionOptOut` |
| Dead pane closes with Run `exited` and task stays `accepted` | `TestAutoCloseDeadPaneExitsRunWithoutBlockingTask` |
| Closing Session cannot resume; `ResumeAgent` starts fresh | `TestAutoCloseClosingSessionCannotResume`, `TestAutoCloseResumeAgentStartsFreshSession` |
| Review message to a closed Session stays queued and is not rerouted | `TestAutoCloseReviewMessageToClosedWorkerIsNotRerouted` |
| CLI flag/help coverage | `TestHandoffAcceptHasKeepSessionFlagOnly` + existing help-map tests |
| Existing tests updated for the new `ReviewHandoffAudited` shape | `autonomy_gates_test.go`, `autonomy_e2e_test.go` |

New test file: `internal/core/session_autoclose_test.go` (PLAN section 5
scenarios 1-11, 13). Existing `handoff_test.go`, `reopen_test.go`,
`autonomy*_test.go`, `plan_first_v2_test.go`, `workflow_test.go` and the full
suite pass unchanged.

## 3. Checks (real commands and outcomes)

Evidence files are submitted with the handoff.

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l ./internal ./cmd` | 0 | `evidence-autoclose-gofmt.txt` |
| `env -i PATH HOME go vet ./...` | 0 | `evidence-autoclose-vet.txt` |
| `env -i PATH HOME go test ./...` | 0 | `evidence-autoclose-test-all.txt` |
| `env -i PATH HOME go test -race ./... -timeout 300s` | 0 | `evidence-autoclose-race.txt` |
| `env -i PATH HOME WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 300s` | 1 | `evidence-autoclose-race-tmux.txt` |
| Same tmux test on clean base `3031ccd` | 1 | `evidence-autoclose-base-tmux.txt` |
| `python3 work-products/check-doc-links.py` | 0 | `evidence-autoclose-doc-links.txt` |

Tests were run with a clean environment (`env -i`) because this worktree exports
`WORKSPACE_SESSION_ID`, which would otherwise make `TestWorkerProcess` execute in
the parent test binary.

## 4. Risks and limitations

- **tmux integration suite is red in this sandbox.** `WORKSPACE_TMUX_TEST=1`
  fails with `launch_uncertain: invalid tmux pane: "%0_@0"` in
  `TestTmuxEndToEnd`, `TestDispatcherTmuxEndToEnd`, `TestTmuxLandAndCompletePlanFirst`,
  `TestTmuxCompleteIssueWorkflow` and
  `TestNavigatorRealPTYAndClientSelection`. This reproduces unchanged on the
  clean base commit `3031ccd` (see `evidence-autoclose-base-tmux.txt`), so it is a
  pre-existing tmux 3.7c/environment incompatibility, not caused by this change.
  The race suite without the tmux opt-in passes, and the auto-close logic is
  covered through the fake runtime.
- **Consultation contract change.** Accepted workers are no longer resumable for
  consultation unless `--keep-session` is used. This is the accepted plan's
  decision and is documented in docs/runtime.md, README.md and PRODUCT.md.
- **Downgrade.** An older binary ignores `close_requested_at` on load and drops
  it on save, leaving a live worker; this is the same manual situation as before.
- **Lock hold time.** `Runtime.Stop` runs under the workspace lock, matching the
  existing `stopSession` pattern.
