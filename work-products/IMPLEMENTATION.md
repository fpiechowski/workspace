# Implementation report — task_01M3P2JFEFSWE8CD2VHS5PXBQW

**Task:** Implement `tolerate the orchestrator's own session in workspace completion` from
`artifacts/art_01M3P2A2Q8TC0RF38KEH0SATFN/PLAN.md`.

## Commit

- `ccbd7525b4256a3977cd3bcfcf358c298af9e533` — `feat: tolerate the orchestrator's own session when completing`
- Branch/worktree: `workspace/ws_01M3P1N1MBMTHGFQM8HSN1SMCE/impl-complete-orchestrator-exemption`
- Base: `3031ccd17917f9586d907382a5eb94efc5d2d61d`
- One product commit (code, tests, docs). A second `work-products/` evidence is committed separately and changes no product code.

## Changes

### T1 — `internal/core/completion.go`

- `completionRuntimeQuiet` now skips active Sessions that belong to the workspace
  orchestrator, using the same predicate as `reopen.go:77`:
  `session.AgentID == d.State.OrchestratorAgentID || session.AgentSnapshot.Role == "orchestrator"`.
  Worker Sessions and services are still rejected. The error text is now
  `stop worker session %s before completing`; the code stays `session_active`.
- The `CompleteWorkspace` doc comment contract now states that no active service,
  active **worker** session or non-accepted task may remain, and that the
  orchestrator's own running Session is tolerated and is not stopped.
- The orchestrator Session is only tolerated; it is not stopped, closed, rewritten
  or flagged `conversation_only`, and `archive` still requires every Session to be
  stopped (unchanged `internal/core/lifecycle.go`). The predicate was inlined so
  `reopen.go` is untouched (behavior-preserving, per plan).

### T2 — tests

- `internal/core/manual_completion_test.go`
  - `TestManualCompletionRequiresUserAttestation`: the final block now expects
    **success** with `UserConfirmed: true`; asserts `Status == "completed"` and that the
    orchestrator Session is still `Active()` after completion. The earlier
    `user_decision_required` assertion is retained.
  - New `TestManualCompletionToleratesOrchestratorButNotWorkers`: an active worker
    Session yields `session_active` naming the worker Session ID; after stopping the
    worker the orchestrator actor completes while its own Session stays active; `Archive`
    is refused with `session_active` while the orchestrator runs and succeeds after
    `StopSession`; the orchestrator Session is active throughout completion.
  - New `TestManualCompletionUserActorToleratesOrchestrator`: the user terminal (empty
    actor) completes while the orchestrator Session runs.
- `internal/core/landing_test.go`
  - New `TestLandCompleteToleratesOrchestratorButNotWorkers` (plan-first v2, the
    reported path): `planFirstFixture` + `integratePlanFirst` + `LandIntegration`, then
    `StartOrchestrator`; an active worker still yields `session_active` naming it, and an
    orchestrator actor with `UserConfirmed` completes (`Status == "completed"`,
    `Workflow.Phase == "completed"`) once the worker is stopped.
- Autonomy tests (`autonomy_gates_test.go`, `autonomy_boundary_test.go`) were **not**
  changed and pass under the focused filter. The service guard test stays unchanged.

### T3 — documentation

- `docs/runtime.md` (~230-236): `workspace complete` requires no active worker Sessions
  or services; the orchestrator's own running Session is tolerated and not stopped; a
  Codex orchestrator Run exits after its current turn; stop remaining Sessions before
  `archive`.
- `ARCHITECTURE.md` Manual Mode (~499-502): completion requires "no active worker sessions
  (the orchestrator's own Session is tolerated), services, or unaccepted tasks".
- `README.md` (~589-592): distinguishes `complete` (worker sessions/services; orchestrator
  session tolerated and not stopped) from `archive` (every session and service stopped).
- `docs/operations.md` Completed workspaces (~117-122): one sentence stating completion
  tolerates the running orchestrator Session, that Run does not become `conversation_only`,
  cannot create work (status guards), and must be stopped before `archive`.
- `PRODUCT.md` and `TODO.md` unchanged (per plan).

## Acceptance criteria

1. `completionRuntimeQuiet` skips active orchestrator Sessions in both manual and landing
   paths; workers/services still fail with `session_active` / `service_active` naming the
   blocking ID. — Covered by T1 + T2 tests.
2. The orchestrator Session is not stopped/closed/modified; `archive` still refuses while
   active and succeeds after it is stopped. — `TestManualCompletionToleratesOrchestratorButNotWorkers`.
3. All T2 tests exist/updated, including the landed plan-first orchestrator-actor test and
   the negative worker case; autonomy tests unchanged and pass. — Verified.
4. The four T3 docs consistently describe the new precondition. — Verified; doc-link check
   `checked=30 broken=0`.
5. `gofmt -l` clean, `go vet ./...` clean, focused core tests pass, `go test ./...` passes
   with real evidence committed. — `work-products/CHECKS-completion.yaml`.

## Check results

| Check | Command | Exit | Evidence |
| --- | --- | --- | --- |
| Format | `gofmt -l ./cmd ./internal` | 0 | `evidence-completion-gofmt.txt` |
| Vet | `go vet ./...` | 0 | `evidence-completion-vet.txt` |
| Focused core | `go test ./internal/core -run 'Complet\|Landing\|Land\|Reopen\|Autonomy\|Archive' -count=1` | 0 (`ok ... 24.878s`) | `evidence-completion-core-targeted.txt` |
| Full suite | `env -u WORKSPACE_* go test ./... -count=1` | 0 (all packages `ok`) | `evidence-completion-go-test-all.txt` |
| Doc links | `python3 work-products/check-doc-links.py` | 0 (`checked=30 broken=0`) | `evidence-completion-doc-links.txt` |

Notes:

- The focused command passed both with and without the sandbox actor environment; the
  recorded run used the literal acceptance command.
- The full suite must be run with the orchestrator/worker `WORKSPACE_*` actor variables
  cleared. Inside a worker/implementer session those variables are exported and leak into
  the harness: `TestWorkerProcess` (core) then reads its own test flag as a prompt path,
  and CLI tests treat the invocation as an agent actor. Those four CLI failures and the one
  core failure reproduce on the **unmodified base** `3031ccd` and are not caused by this
  change (verified with `git stash`). With the variables cleared, `go test ./...` is green.

## Risks / follow-ups

- **Residual copy, out of T3 scope:** `internal/cli/help.go` and `internal/tui/forms.go`
  (and `docs/tui.md:125`) still describe `complete` as requiring "no active Runs"; the
  accepted plan bounded T3 to the four named documents, so they were intentionally left
  unchanged. They are wording-only and do not affect behavior. Recommend a follow-up to
  align them.
- The orchestrator-role predicate is broader than the actor (a second orchestrator-role
  Session would also be tolerated). This is the same accepted behavior as `reopen` and is
  prevented in practice by the single execution line and generation guard.
- A non-Codex orchestrator Run stays alive after completion; status guards reject new work
  and `archive` still forces a stop. Documented in T3.
- No runtime/tmux code changed, so the tmux suite was not required; the full `go test ./...`
  already includes the non-`WORKSPACE_TMUX_TEST` runtime tests.
