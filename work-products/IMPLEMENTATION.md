# Integrated implementation reports (I4 and I5)

The I4 and I5 implementation tasks each committed their report to this shared
root path (`work-products/IMPLEMENTATION.md`). Merging I5 (branched from I3)
onto the I4 lineage produced a whole-file content conflict because each report
rewrote the file. Both reports are preserved verbatim below; the per-task copies
also remain on their branches and in the task artifacts.

---

# Implementation report — task_01M3MJYRXPT60803R31VMFWE6C (I4: Orchestrator guidance and documentation)

## Commit

- `c718ed1` — `feat: add autonomous-mode guidance and documentation`

## Changes

### Binary-owned autonomous notice (`internal/core`)

- `internal/core/autonomy.go` adds the `autonomyRunNotice` constant. It is the
  authoritative autonomous-mode guidance appended to orchestrator Run prompts:
  the §3.3 judgment rules (plan/task/integrator acceptance, phase advance,
  worker questions, the 2-rejection/1-retry bound) and the §4 boundary (the
  operations reserved for the user). It is owned by the binary, so it cannot go
  stale or be removed by a customized project template.
- `internal/core/session.go` appends the notice after the existing
  conversation-only notice when `a.Role == "orchestrator"` and
  `d.State.AutonomyRunning()`. `promptData` gains `Autonomous`, populated from
  the same state, so the stock session prompt templates can switch on it.
- `internal/core/workflow_model.go` adds `Workspace.Autonomous()`, a method that
  mirrors `AutonomyRunning()` and backs the `{{if .Autonomous}}` template switch
  for every render path that passes a `Workspace` value.

### Stock templates

- `internal/core/templates/orchestrator.AGENTS.md.tmpl` replaces the
  "Keep the conversation interactive" paragraph with an `{{if .Autonomous}}`
  branch while the interactive text remains the `{{else}}` branch.
- `internal/core/templates/workflows/plan-first/WORKFLOW.md.tmpl` and
  `internal/core/templates/workflows/plan-first/prompts/orchestrator.md.tmpl`
  add a short autonomy note.
- `internal/core/templates/manual/WORKFLOW.md.tmpl` and
  `internal/core/templates/manual/prompts/orchestrator.md.tmpl` add a one-line
  autonomy note (finish with `autonomy report`, outcome `ready_to_complete`).
- `internal/core/project.go` sets `Workspace.Autonomy` before
  `snapshotTemplates`, so `create --autonomous` renders the autonomous branches
  (not just the Run-time notice).

### Stale-template detection

- `internal/core/project.go` adds the previous stock digests to the
  stale-detection lists: `0f8af6…` to `stockPlanFirstTemplateDigests`
  (`workflows/plan-first/WORKFLOW.md.tmpl`) and `5c789f…` to
  `stockOrchestratorPromptDigests`
  (`workflows/plan-first/prompts/orchestrator.md.tmpl`). Projects on those
  versions are recognized as unmodified stock and refreshed; customized files
  are preserved and listed.

### Agent skill

- `internal/core/skill/workspace/SKILL.md` (and the identical tracked copy at
  `.agents/skills/workspace/SKILL.md`) add the `--autonomous` create flag note,
  the `decision_required` autonomous clause (resolve with
  `--rationale`/`--evidence`, never attest an excluded operation with
  `--user-confirmed` while running), and the `workspace autonomy enable|disable`
  and `workspace autonomy report` commands. `workspace prime` prints the new
  text.

### Documentation

- `PRODUCT.md`: new principle "Autonomous Runs Are Auditable and Bounded" and a
  Product Scope bullet.
- `README.md`: the `--autonomous` create flag, the `workspace autonomy
  enable|disable|report` contract, the report outcome, and the excluded
  operations.
- `ARCHITECTURE.md`: an `Autonomy` Domain Model row and the persisted-state
  contract (autonomy record, `omitempty` audit fields, downgrade behavior,
  delivery precondition, run end).
- `docs/operations.md`: the new receipted mutations, the rationale/evidence
  gate payload committed atomically with an audit `Decision`, and the
  `autonomy_excluded` boundary.
- `docs/runtime.md`: the delivery precondition (`autonomy_unsupported`) and the
  `autonomy report` run end.

### Tests

- New `internal/core/autonomy_prompt_test.go`:
  - an autonomous plan-first orchestrator prompt contains the binary-owned notice
    while an interactive one does not, and `create --autonomous` rendered the
    autonomy branches into `AGENTS.md` and `WORKFLOW.md`;
  - the notice survives customized/older project templates (binary ownership);
  - manual, plan-first and custom (`extended`) workflows all render with
    `missingkey=error` and their autonomy notes.

## Acceptance criteria

- An autonomous orchestrator Run prompt contains the notice; a non-autonomous
  one does not; the notice is present even with customized templates
  (`TestAutonomousOrchestratorPromptCarriesNotice`,
  `TestAutonomyNoticeSurvivesCustomizedTemplates`).
- The changed templates render with `missingkey=error` for manual, plan-first
  and custom workflows, and `workspace prime` includes the new skill text
  (`TestAutonomyTemplateRenderingForEachCreationMode`, `evidence-i4-prime.txt`).
- The previous stock plan-first workflow and orchestrator-prompt digests were
  added to the stale-detection lists.
- The docs describe the current contract; the link check reports
  `checked=29 broken=0`.
- `go test ./internal/core -run 'Session|Skill|Template|Autonomy' -count=1`,
  the tmux race prompt-composition subset, `gofmt` clean, `go vet ./...` and
  `go test ./...` all pass.

## Checks

See `work-products/CHECKS-I4.yaml`. All recorded commands exit 0 except the
full `WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s` suite, which is
red on this machine for pre-existing reasons (see Risks).

## Risks and deviations

- The acceptance names `WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s`.
  On this machine the full race suite is red independently of this change: the
  `internal/terminal` `TestNavigatorRealPTYAndClientSelection` pseudo-TTY test
  fails at the unmodified HEAD (`58b5862`), and the `internal/core` race binary
  exceeds the 90s budget (the non-race core suite alone takes ~65–142s here).
  I ran the change-relevant race subset
  (`WORKSPACE_TMUX_TEST=1 go test -race ./internal/core -run
  'Session|Skill|Template|Autonomy' -timeout 120s`) green in 22.5s; there is no
  data race. Evidence: `evidence-i4-race-all.txt`,
  `evidence-i4-tmux-prompt.txt`.
- `create --autonomous` now assigns `Workspace.Autonomy` before
  `snapshotTemplates`. This only affects the newly created autonomous document;
  non-autonomous creation and its receipt digests are unchanged (covered by the
  existing autonomy serialization test and the full suite).
- The stock `orchestrator.AGENTS.md.tmpl` and `manual/*` templates are not in
  the stale-detection lists (they were not before this change either); only the
  two plan-first stock files are refreshed by digest.

---

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
