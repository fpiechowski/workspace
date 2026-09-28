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
