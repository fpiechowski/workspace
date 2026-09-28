# Implementation report — task_01M3MJYR9QGF20V1CN9WV9NC08 (I2: Decision audit model and rationale-carrying gate mutations)

## Commit

- `4072865` — `feat: add autonomy decision audit to gate mutations`

## Changes

### Core audit helpers (`internal/core/autonomy.go`)

- `isAgentActor`, `requireGateRationale`, `appendGateDecision`,
  `autonomyDecisionCount`, `requireAutonomyBound` implement the PLAN §3.4
  contract: while `Autonomy.State == running` and the actor is an agent, an
  orchestrator-level gate needs a non-empty rationale, and the gate and its
  audit `Decision` are written in the same mutation.
- `DecisionRecordOptions` and `RecordDecision` add
  `workspace decision record --kind autonomous.assumption|autonomous.question_answer
  --subject … --rationale … --evidence …`. It is orchestrator-agent only and
  only while the run is running, with a receipted, replayable payload.
- Decisions carry `ResolvedBy: orchestrator`, `Autonomous: true`, a `Subject`
  (`handoff:…`, `task:…`, `phase:…`), `Evidence`, `SessionID`, `RunID`,
  `DecidedAt` and a per-kind question; the narrative gets one
  `## Autonomous decisions` line. `autonomy.enabled`/`autonomy.disabled`
  continue to use the I1 helper.

### Gate mutations

- `ReviewHandoffAudited` (handoff accept/reject): rationale is required while a
  run is running; accept records `autonomous.plan_acceptance` for planners and
  `autonomous.handoff_accept` otherwise; reject records
  `autonomous.handoff_reject` and enforces the 2-rejection bound.
- `AdvanceWorkflowAudited`: requires rationale, records
  `autonomous.phase_advance` with `phase:<new>`.
- `UpdateStateAudited`: requires rationale only for a `phase` patch and records
  `autonomous.phase_advance`.
- `RetireTaskAudited` (cancel/abandon/supersede) records `autonomous.retire`.
- `RetryTaskAudited`: requires rationale, enforces the 1-retry bound and records
  `autonomous.retry`.

The existing entry points (`ReviewHandoff`, `AdvanceWorkflow`, `UpdateState`,
`RetireTask`, `RetryTask`, `RetryTaskGuarded`) delegate with an empty rationale,
so their callers and tests are unchanged. Rationale and evidence join the
idempotency payload only when non-empty (`omitempty` struct fields, or a
conditionally extended array), preserving historical receipt digests.

### Bounds and failure injection

- Rejection and retry counts are derived per task and per orchestrator Run from
  the appended decisions (`autonomyDecisionCount`); exceeding a bound returns
  `autonomy_bound_exceeded`.
- `flushDocumentTestHook` (nil in production) lets a test inject a persistence
  failure and prove the gate change and its decision are lost together.

### Interfaces

- CLI: `--rationale`/`--evidence` on `handoff accept|reject`,
  `workflow advance`, `state update`, `task retry|cancel|abandon|supersede`, plus
  the new `decision record` command and updated help/flag documentation. The TUI
  and other callers keep using the unchanged entry points.

## Acceptance criteria

- Autonomy-running agent gates without `--rationale` return `rationale_required`;
  with a rationale, the gate and a
  `Decision{ResolvedBy: orchestrator, Autonomous: true, Subject, Evidence, RunID}`
  commit together. `TestAutonomyGateSaveFailureIsAtomic` injects a save failure
  and shows the handoff, task state, decisions and revision are unchanged.
- Without autonomy, agents need no rationale and no audit decision is written
  (`TestAutonomyGateWithoutAutonomyNeedsNoRationale`); old receipts replay
  through the audited entry points with an empty rationale
  (`TestAutonomyGateReceiptDigestsUnchanged`).
- `decision record` works only for the orchestrator agent while running, with
  idempotent receipts and the documented kinds
  (`TestAutonomyDecisionRecord`, `TestAutonomyCLIDecisionRecordIsOrchestratorOnly`).
- Rejection (2) and retry (1) bounds return `autonomy_bound_exceeded`
  (`TestAutonomyRejectionBound`, `TestAutonomyRetryRationaleAndBound`).

## Checks

See `work-products/CHECKS-I2.yaml`. All commands exited 0: gofmt clean,
`go vet ./...`, the targeted `Decision|Handoff|Autonomy` core run,
`go test ./internal/cli -count=1`, and the full `go test ./... -count=1`.

## Risks and deviations

- The rejection bound counts rejections made by the current orchestrator Run, so
  a resumed orchestrator Run starts a fresh count. The PLAN says "per task
  during a run" and the Decision provenance is the concrete Run, so this is the
  literal reading; a stricter cross-Run counter would need an extra durable
  field.
- Evidence is taken from `--evidence` verbatim; no artifact IDs are inferred.
- Documentation (`README`, `PRODUCT`, `ARCHITECTURE`, `docs/*`) stays in the I4
  scope per PLAN §5, so this task changes no document.
- The safety boundary (`autonomy_excluded`), `autonomy report` and the menu are
  I3 scope.
