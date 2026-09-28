# Implementation report — task_01M3MJYS3TAP3E9057R289DR1G (I5: TUI support)

## Commit

- `eca7ab5` — `feat(tui): surface autonomous runs and guarded disable`

## Changes

### Create form (already in I1; verified here)

- `openCreateWorkspaceForm` offers the `Autonomous run` confirm next to the
  workflow selector, and `createOptions` maps `ActionCall.Autonomous` into
  `CreateOptions.Autonomous` together with either a named workflow or the
  explicit `No workflow (manual orchestration)` choice. No change was needed in
  I5; `TestCreateFormAutonomyCompatibleWithBothWorkflowChoices` now pins both
  combinations.

### Autonomy badge (`internal/tui/autonomy.go`, `internal/tui/view.go`, `internal/tui/status.go`)

- New `autonomyStateBadge` returns `● autonomous running`, `✓ autonomous delivered`
  or `○ autonomous disabled` (plain text, empty for an interactive workspace) and
  `(*Model).autonomyBadge` applies the semantic color (accent / success /
  neutral). Both render the state word in `--no-color` mode.
- `headerIdentityText` appends the badge after the workspace status, so every
  workspace route shows it. `statusBadge` recognizes the `delivered` and
  `disabled` states used by the report and attention rows.

### Report view and Needs attention (`internal/tui/autonomy.go`, `routes.go`, `update.go`, `detail.go`)

- New `autonomyContent` detail page renders the mode, source, enabled revision
  and time, disabled reason, and, once delivered, the final report (outcome,
  recommendation, phase, integration head, artifact IDs, pending commands).
- A delivered report is a Needs-attention entry (`Autonomous run delivered:
  <outcome>`, priority with decisions) whose `Enter` opens the report view. More
  lists `Autonomy` whenever an autonomy record exists, so the view stays
  reachable after the attention row is gone. The `autonomy` route is wired into
  `detailContent`, `detailSections` and `isDetailPage`.

### ResolvedBy marker (`internal/tui/routes.go`, `internal/tui/detail.go`)

- Decisions collection rows append `· by <resolver>` and, when the decision is
  autonomous, `(autonomous)`.
- Decision detail adds `Resolved by`, `Autonomous`, `Subject`, an `Evidence`
  section, and the Run / decider provenance timestamps.

### Guarded disable action (`internal/tui/forms.go`, `backend.go`, `pending.go`)

- `availableActions` offers `Disable autonomous run` from the Orchestrator,
  Runtime and Autonomy pages while an autonomy record exists and is not
  `disabled` (and the workspace is not closed). The action opens the required
  reason form and then a confirmation.
- `beginAction` records the exact current workspace revision
  (`ExpectedRevision`) and the per-action `tui_<ULID>` key, and the backend
  dispatches `Service.DisableAutonomy` with the reason and revision guard. The
  TUI service actor is empty (user), so no `--user-confirmed` attestation is
  needed for the user's own terminal action.
- Added the caption, description, warning severity and `Disabling autonomy`
  status verb for the action.

### Documentation (`docs/tui.md`)

- The create-form paragraph documents the `Autonomous run` toggle.
- A new `## Autonomous runs` section documents the badge and its `--no-color`
  form, the More/report view, the Needs-attention entry, the guarded disable
  action (`tui_<ULID>` key and revision guard), and that the TUI does not offer
  `autonomy enable` (A4) or auto-accept results and decisions.

## Tests

- New `internal/tui/autonomy_test.go`:
  - `TestAutonomyBadgeStatesColorAndNoColor` — every state names itself in color
    and `--no-color`, and no escape is emitted without color; interactive shows
    nothing.
  - `TestAutonomyBadgeRendersInWorkspaceHeader` — the header shows the badge.
  - `TestDeliveredReportAppearsInAttentionAndOpensReportView` — the delivered
    report is a Needs-attention row that opens the report view with its outcome,
    recommendation, artifact and pending command.
  - `TestMoreExposesAutonomyOnlyWhenEnabled` — More lists the view only for an
    autonomous workspace.
  - `TestDisableAutonomyActionUsesRevisionGuardAndTuiKey` — the action is offered
    for running/delivered, carries revision 7 and a `tui_` key, and is absent
    when disabled.
  - `TestDisableAutonomyReasonKeepsGuardedRequest` — the confirmation keeps the
    same key, revision and reason.
  - `TestTUIDoesNotOfferAutonomyEnableOrResultAcceptance` — no enable or
    accept/handoff action on the Orchestrator, Runtime or Autonomy pages.
  - `TestAutonomyDecisionShowsResolvedByAndEvidence` — the collection and detail
    surface the resolver, autonomy marker, subject and evidence.
  - `TestCreateFormAutonomyCompatibleWithBothWorkflowChoices` — autonomy reaches
    `CreateOptions` for a named workflow and for the manual choice.

## Acceptance criteria

- The create form passes `Autonomous` and is compatible with both workflow
  choices.
- The badge renders running/delivered/disabled in color and `--no-color` modes.
- Disable from the TUI uses the current revision guard and a `tui_<ULID>` key.
- No TUI action auto-accepts results or enables autonomy on an existing
  workspace; enable is not offered in v1 (A4).
- `go test ./internal/tui -count=1`, gofmt, `go vet ./...` and `go test ./...`
  pass; `docs/tui.md` documents the new UI.

## Checks

The exact commands, exit codes and evidence files are in
`work-products/CHECKS-I5.yaml`. All exited 0: `go test ./internal/tui -count=1`,
`gofmt -l ./cmd ./internal` (empty), `go vet ./...`, and
`go test ./... -count=1 -timeout 570s`.

## Risks and deviations

- The harness environment exports `WORKSPACE_AGENT_ID`, `WORKSPACE_SESSION_ID`,
  `WORKSPACE_RUN_ID` and `WORKSPACE_ROLE`. These make the pre-existing
  `TestWorkerProcess` fixture and the CLI actor tests behave as an agent and
  fail; the failures reproduce unchanged at the base commit `58b5862` without
  this task's changes. The full-suite evidence therefore clears exactly those
  variables (`env -u …`), matching the I3 checks file and the maintainer
  environment. Targeted `go test ./internal/tui` passes with or without them.
- The TUI never offers `autonomy enable`; enabling stays a terminal-only
  `workspace autonomy enable` decision (A4).
- The autonomy badge in the workspace header is plain text because `header()`
  sanitizes the identity segment; the colored badge is rendered on the Autonomy
  detail page (`autonomyBadge`).
