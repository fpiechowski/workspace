# Implementation report — task_01M3MJYRHCY7FP8RBR2G95MCV6 (I3: Safety boundary enforcement, final report, menu)

## Commit

- `48eab2b` — `feat: enforce the autonomy safety boundary and final report`

## Changes

### Code-level safety boundary (`internal/core/autonomy.go`)

- `rejectAutonomousAttestation(d, op)` returns `autonomy_excluded` when
  `Autonomy.State == running` and the actor is an agent, regardless of any
  attestation flag. `requireUserNotAutonomous(d, op)` composes it with
  `requireUser` for the operations that were already terminal-only.
- The check is wired into every excluded operation from PLAN §4:
  - `internal/core/landing.go` — `integration land` (before the
    `--user-confirmed` and capability checks, so a publication/permission flag
    cannot bypass it);
  - `internal/core/completion.go` — `complete` (covers manual and landing);
  - `internal/core/workflow.go` — `decision answer` and `release confirm`;
  - `internal/core/change_request.go` — `change-request publish` (before the
    `forge.publication: allowed` branch, per A1 the run stays local-only) and
    `change-request resolve`;
  - `internal/core/reopen.go` — `reopen` (before the status switch);
  - `internal/core/lifecycle.go` — `archive` and non-dry-run `clean`;
  - `internal/core/deletion.go` — workspace/task/session deletion (the two
    `mutate` authorizers now use `requireUserNotAutonomous`);
  - `internal/core/state.go` — `state edit` (authorizer uses
    `requireUserNotAutonomous`).
- After `delivered`/`disabled` the guard is inert, so the ordinary attestation
  contract applies again.

### Final report (`internal/core/autonomy.go`)

- `AutonomyReportOptions` and `ReportAutonomy` (orchestrator-only, running-only,
  revision-guarded, receipted). It refuses while a result is submitted but
  unreviewed (`handoff_pending`) or a worker Run is active (`session_active`; the
  orchestrator's own Run is allowed), then validates the outcome against state:
  - `ready_to_land` requires an active landing workflow in the integration
    phase, an accepted integrator and `validateIntegration` (so an integration
    changed after acceptance returns `integration_changed`);
  - `ready_to_complete` requires every live task accepted and either a manual
    workspace or a landed/nothing-to-integrate landing workflow;
  - `blocked`/`failed` require a non-empty pending list (`pending_required`);
  - any other value returns `invalid_outcome`.
- The `--summary-file` plus optional `--artifact` sources are read once,
  recorded as `Artifact` entries with digest/size/run provenance, and written
  atomically with the document through `Document.PendingFiles`. The report is
  set on `Autonomy.Report`, `Autonomy.State` becomes `delivered`, an
  `autonomous.final_report` decision is appended (evidence = artifact IDs) and
  the narrative gets an `## Autonomous report` line. Replays idempotently under
  the operation key.

### Menu (`internal/core/workflow.go`)

- New `autonomyReportMenuAction` and `autonomySuggestedOutcome`. While running,
  the integration phase and a finished manual workspace show `report` (command
  `autonomy report --summary-file <path> --outcome …`) instead of `land` /
  `complete`. After delivery the menu shows the report entry plus the existing
  user actions.

### CLI / help

- `workspace autonomy report --outcome --recommendation --summary-file
  --artifact --pending --expected-revision` in `internal/cli/autonomy.go`, with
  `workspace autonomy report` help and flag descriptions.

## Tests

- `internal/core/autonomy_boundary_test.go`: table-driven coverage of every
  excluded operation (agent → `autonomy_excluded`, terminal user not excluded),
  publication `allowed` precedence, workspace deletion, and the boundary being
  released after delivery.
- `internal/core/autonomy_e2e_test.go`: full autonomous plan-first flow
  (create → planner accept → advance → implementer accept → prepare → integrator
  accept → advance → `ready_to_land`), menu while running and after delivery,
  agent land/complete refusal while running, agent land without attestation
  after delivery, then user land + complete; the manual flow ending
  `ready_to_complete`; and report outcome validation
  (`integration_changed`, `handoff_pending`, `pending_required`,
  `invalid_outcome`, blocked delivery).
- `internal/cli/autonomy_test.go`: `autonomy report` delivers and replays.

## Acceptance criteria

- Every excluded operation returns `autonomy_excluded` for an agent while
  running, even with `--user-confirmed` or `publication: allowed`; the terminal
  user is not excluded and the boundary is released after delivery.
- `autonomy report` validates the outcome against state, stores an immutable
  summary artifact, sets `delivered`, appends `autonomous.final_report` and
  replays idempotently.
- `menu --json` shows `report` while running in integration or when manual work
  is done, and shows `land`/`complete` after delivery.
- The end-to-end plan-first and manual tests pass.

## Checks

See `work-products/CHECKS-I3.yaml`. All commands exited 0: gofmt clean,
`go vet ./...`, `go test ./internal/core -run 'Autonomy|Landing|Complete|Reopen'`,
`go test ./internal/cli -run 'Autonom'`, and the full `go test ./... -count=1`.

## Risks and deviations

- Documentation (`README`, `PRODUCT`, `ARCHITECTURE`, `docs/*`) belongs to I4 per
  PLAN §5; this task changes only CLI help.
- `ReportAutonomy` permits the terminal user as well as the orchestrator agent
  (the user always retains authority); it is refused for non-orchestrator agents
  by `requireOrchestrator`.
- The `clean` exclusion applies only to non-dry-run cleanup; the read-only
  `clean --dry-run` inspection stays available.
- Report artifact source paths are read relative to the process working
  directory; the bytes are copied into the workspace and verified by digest, so
  the source path is only used once.
