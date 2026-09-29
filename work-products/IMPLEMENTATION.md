# Integrated implementation reports (integration worktree, named-IDs merge)

The shared `work-products/IMPLEMENTATION.md` path is rewritten in full by each
task lineage, so merging this path conflicts. Per the repository convention, the
lineages are preserved verbatim below: the integration base report first, then
the incoming T1 and T2 named-IDs implementation reports. No product code is
affected by this resolution; the final tree has no conflict markers.

---

## Lineage A - integration base report (unchanged)

# Integrated implementation reports (integration worktree, T4 merge)

The shared `work-products/IMPLEMENTATION.md` path is rewritten in full by each
task lineage, so every merge of this path conflicts. Per the repository
convention, both lineages are preserved verbatim below: the integration base
report first, then the incoming `impl-docs` (T4) report. No product code is
affected by this resolution.

---

## Lineage A - integration base report (unchanged)

# Integrated implementation reports (integration worktree, T3 merge)

The shared `work-products/IMPLEMENTATION.md` path is rewritten in full by each
task lineage, so every merge of this path conflicts. Per the repository
convention, both lineages are preserved verbatim below: the integration base
report first, then the incoming `impl-launch` (T3) report. No product code is
affected by this resolution.

---

## Lineage A - integration base report (unchanged)

# Integrated implementation reports (integration worktree, T5 merge)

The shared `work-products/IMPLEMENTATION.md` path is rewritten in full by each
task lineage, so every merge of this path conflicts. Per the repository
convention, both lineages are preserved verbatim below: the integration base
report first, then the incoming `impl-tui` (T5) report. No product code is
affected by this resolution.

---

## Lineage A - integration base report (unchanged)

# Integrated implementation reports (integration worktree, T2 merge)

The shared `work-products/IMPLEMENTATION.md` path is rewritten in full by each
task lineage, so every merge of this path conflicts. Per the repository
convention, both lineages are preserved verbatim below: the integration base
report first, then the incoming `impl-prompts` (T2) report. No product code is
affected by this resolution.

---

## Lineage A - integration base report (unchanged)

# Integrated implementation reports (integration worktree, T1 merge)

The shared `work-products/IMPLEMENTATION.md` path is rewritten in full by each
task lineage, so every merge of this path conflicts. Per the repository
convention, both lineages are preserved verbatim below: the integration base
report first, then the incoming `impl-core` (T1) report. No product code is
affected by this resolution.

---

## Lineage A - integration base report (unchanged)

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

---

# IMPLEMENTATION — single-client auto-jump for the TUI navigation flow

Task: `task_01M3MN4RE4J7NKEJCD45MTSH7S`
Plan: `art_01M3MMYVEGXFT2WCVEFFXECK1X` (task `task_01M3MKVG69T3XM0DPREVQ1BXPP`)
Base: `aa72616494937c5c977187ade2fd342d230fac93`

## Commits

| Commit | Message |
|---|---|
| `f6e666e63bad2dc1d7311f0600a551e37a7dd796` | `feat(tui): jump immediately with the only attached tmux client` |
| `d1bd846e550bf9e437190592206fd7e8319aa66b` | `docs: describe single-client auto-jump navigation` |

Both commits are on the worktree branch
`workspace/ws_01M3MJZRZ42N2F623G70K60M05/impl-single-client-jump`.

## What changed

### `internal/tui/navigation_flow.go`

- **T1**: In `handleNavigationClients`, after the zero-client check (l.126) and the
  generation/target/ref guard (l.133) and before `orderedClients`, a new
  `len(msg.clients) == 1` branch calls
  `jumpClientCommand(msg.target, msg.ref, msg.clients[0], msg.afterReconcile,
  msg.generation, msg.sequence, true, true)`. It does not set `m.form`, `m.formMode`,
  `m.formClient` or `m.navigationClients`, so no picker opens.
- **T1**: Inside the existing staleness guard only, when `len(msg.clients) == 1` the
  notice becomes “The selected workspace target changed before the jump started. Press g
  to try again.” The multi-client wording is unchanged. No second guard was added.
- **T2**: `jumpClientCommand` gained a trailing `automatic bool`. Both existing callers
  (reconcile retry `navigation_flow.go:99`, picker accept `:201`) pass `false`; the new
  single-client branch passes `true`. When automatic, the in-flight notice is
  “Jumping the only attached tmux client to the verified workspace pane…” and the
  `navigation_target_changed` message is “the selected tmux target changed during client
  discovery; press g to try again”; otherwise both texts are unchanged. Recheck, `Jump`,
  `SaveLastUsed` and message shapes are untouched.
- **T3**: `handleNavigationTarget` loads the last-used preference only when
  `len(clients) > 1` (was `> 0`). The `SaveLastUsed` call on a successful jump is
  unchanged, so the single client is still recorded and its save failure is still
  reported by `handleNavigationResult`.

### `internal/tui/navigation_flow_test.go`

- Extracted `discoverNavigationClients` (resolve → `handleNavigationTarget` → discovery
  message) and made `openNavigationPicker` reuse it, still asserting the picker opened.
- Renamed `TestJumpAlwaysShowsClientPickerAndJumpsChosenClient` to
  `TestMultipleClientsShowPickerAndJumpChosenClient`, dropped the one-client subtest, and
  test both rows (index 0 and 1) with `flowClients()`; it asserts `prefs.loads == 1`.
- `TestJumpCancellationAndFailureNeverSavePreference` and
  `TestPreferenceWriteFailureReportsSuccessfulJumpSeparately` now use `flowClients()`.
- Added `TestSingleClientJumpsWithoutPicker`, `TestSingleClientTargetChangedDuringDiscovery`,
  `TestSingleClientGoneOnAutomaticPath`, `TestSingleClientPreferenceSaveFailureReportedSeparately`,
  `TestSingleClientStaleDiscoveryDoesNotJump` (stale sequence/generation/changed target) and
  `TestEscDuringAutomaticJumpDropsResult`.
- Multi-client, zero-client, target-change-while-picker-open, detached-client,
  stale-response and reconcile-retry tests are unchanged and passing.

### Contract docs (T5)

- `README.md` (~446–449 and ~478–482, plus the parallel Sessions line ~466),
  `PRODUCT.md` (~225–229 and ~254), `docs/runtime.md` (~14–17), `docs/tui.md`
  (~65–68, the `g` key table row, the Terminal and Managed Pane paragraph ~254–257 and
  the target-recheck sentence ~269–270).
- No stale “always / even for one client” wording remains. Verified with
  `grep -rn "even for one client\|always opens\|always presents\|including for one client\|even when only one client" --include="*.md" .` → no matches.

## Decisions and deviations from the plan

- **Extended doc scope (deviation)**: the plan’s T5 list did not name `ARCHITECTURE.md`
  or `DESIGN.md`, but both contained the same stale contract
  (`ARCHITECTURE.md:316` “always presents the client picker, including for a single
  client”; `DESIGN.md:165-166` “always opens the attached-client picker … even when only
  one client is attached”). The acceptance criterion requires that no stale
  “always/even for one client” wording remains (grep verified) and `AGENTS.md` requires
  the architecture/design contract to describe current behavior, so both were updated.
- **`docs/tui.md:269-270`**: changed “changed while the picker was open” to “changed
  before the jump started”, because the single-client path has no picker.
- **T3 fallback not used**: the plan offered keeping the load at l.110 as a fallback; the
  chosen behavior is `len(clients) > 1`, pinned by `TestSingleClientJumpsWithoutPicker`
  (`prefs.loads == 0`) and the multi-client test (`prefs.loads == 1`).
- Zero-client notice, multi-client picker behavior (ordering, last-used marker,
  read-error notice, Esc/Ctrl+C cancel), reconcile retry, and auto-select among several
  clients are unchanged/out of scope.

## Acceptance criteria

| Criterion | Status | Evidence |
|---|---|---|
| T1 single-client branch, no form | Met | `navigation_flow.go:143-145`; `TestSingleClientJumpsWithoutPicker` asserts `form==nil`, `formMode==""`, `navigationClients==nil` |
| T1 single-client staleness notice, no second guard | Met | `navigation_flow.go:133-142`; `TestSingleClientStaleDiscoveryDoesNotJump/changed target` |
| T2 `automatic bool`, texts, other callers false | Met | `navigation_flow.go:99,201,204,213-217,226-231`; `TestSingleClientTargetChangedDuringDiscovery` |
| T3 load only when `>1`, save unchanged | Met | `navigation_flow.go:111`; `TestSingleClientJumpsWithoutPicker` (`loads==0`, save on success) and multi-client test (`loads==1`) |
| T4 new tests + helper + renamed/updated existing tests | Met | `navigation_flow_test.go` |
| T5 docs, no stale wording | Met | docs diff; grep clean |
| Out of scope unchanged | Met | existing zero/multi/stale/reconcile/detached tests pass |
| Checks pass and evidence captured | Met (see below) | check receipts |
| Committed with clear messages | Met | `f6e666e`, `d1bd846` |

## Check results

Run via `workspace check run` (receipts under `work-products/checks/`):

| Command | Exit | Receipt |
|---|---|---|
| `gofmt -l internal/tui` (empty) | 0 | `check_01M3MNFZ50HDM3MP552AXF456K` |
| `go test ./internal/tui -run 'Navigation\|Jump\|Client\|Preference\|Reconcile\|Stale' -v` | 0 | `check_01M3MNG1HM1BD0BKWHJHXWM0H8` |
| `go vet ./...` | 0 | `check_01M3MNG4S33XD94MYZCY2NAHKS` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/tui -timeout 90s` | 0 | `check_01M3MNGE76V24JG1KQ1BAW9PQB` |
| `go test ./...` | 1 (pre-existing) | `check_01M3MNGM4N1G072Z0JG568KHES` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s` | 1 (pre-existing) | `check_01M3MNGYY6FAS3APQ3NVD11R1M` |

Raw evidence copies: `work-products/evidence/gofmt.txt`, `tui-focused.txt`,
`go-vet.txt`, `go-test-all.txt`, `tmux-race.txt`.

**Pre-existing failures (not caused by this change).** `go test ./...` and the full race
run fail in `internal/cli`, `internal/core` and `internal/terminal` in exactly the same
way on the base commit `aa72616` with this change stashed:
`TestCompleteAndArchiveManualWorkspace`, `TestReopenJSONAndHelpContract`,
`TestIntegrationLandJSON`, the `internal/core` harness
(`open -test.timeout=...: no such file or directory`), and
`TestNavigatorRealPTYAndClientSelection` (`pseudo-TTY client did not attach`). The
changed package `internal/tui` passes the full suite both normally and under
`-race` with `WORKSPACE_TMUX_TEST=1`.

## Risks

- **Surprise switch**: a user expecting a confirmation step now gets an immediate
  single-client switch. This is the intended issue behavior; it is mitigated by the
  explicit in-flight/success notices and the doc update.
- **TOCTOU**: a second client could attach between `ListClients` and `Jump`; the verified
  jump still moves only the discovered client, the same guarantee the picker gave.
- **Text drift**: tests assert substrings (“only attached”, “press g”, “Jump completed”);
  wording changes must update the tests together.


---

# Incoming implementation report — task_01M3MZD82YXFS0G5HKV46G5JNQ (dispatcher jump)

Preserved verbatim from the accepted dispatcher-jump head `7d4af63`; the shared
`work-products/IMPLEMENTATION.md` path was rewritten by both lineages.

---

# Implementation report — task_01M3MZD82YXFS0G5HKV46G5JNQ

TUI jump (`g`) support for the project Dispatcher page, per
`artifacts/art_01M3MY2RG189S4GCB9VR62D4ST/PLAN.md`.

## Commit

- `bf5a3e92780bd60195261dbcc95c4a19a2a77253` —
  `feat(tui): jump (g) to the project Dispatcher page`
- Base: `aa72616494937c5c977187ade2fd342d230fac93`

## Changes

### Core — `internal/core/navigation.go`

- `NavigationTarget` gains `ProjectID string json:"project_id,omitempty"`.
- `ResolveNavigationTarget` dispatches `Kind=dispatcher` before the workspace
  snapshot (the dispatcher is project-scoped and has no workspace selector).
- New `resolveDispatcherNavigationTarget(ctx)` reads `DispatcherStatus` (durable
  state plus a fresh `ObserveProjectTopology` verification) and returns a target
  only when:
  - the state is initialized (`never_started` → `pane_missing`),
  - an active Session with a non-empty `CurrentRunID` and its Run exist
    (otherwise `pane_missing`),
  - `Runtime.Verified && Runtime.PaneID != ""` (otherwise `pane_missing`),
  - the verified pane ownership metadata matches the durable current Run:
    `Runtime.SessionID == session.ID`, `Runtime.RunID == run.ID`,
    `Runtime.PaneID == run.PaneID` (mirrors `tickDispatcher`; otherwise
    `pane_missing`).
  - The returned target sets `ProjectID`, canonical
    `SessionName == DispatcherTmuxName(projectID)`, durable `SessionID`/`RunID`
    and the verified `PaneID`/`WindowID`; `Socket` is taken from a `Tmux`
    runtime. No heuristic topology scan is used.
- New helper `activeDispatcherNavigationSession(status)` selects the durable
  active Session/Run, matching `status.Agent.ID` when present.

### Terminal — `internal/terminal/navigation.go`

- `TmuxNavigator.verify` branches on `target.Kind == "dispatcher"` to a new
  `verifyDispatcher`. Workspace-kind behavior is unchanged.
- `verifyDispatcher` requires `ProjectID` and a canonical
  `SessionName == core.DispatcherTmuxName(ProjectID)`, checks `has-session`,
  window membership, and (for a non-empty `PaneID`) `list-panes -a` with
  `#{pane_id} #{window_id} #{pane_dead} #{@workspace_scope}
  #{@workspace_project_id} #{@workspace_kind} #{@workspace_session_id}
  #{@workspace_run_id}`, accepting only a live pane with `scope=project`,
  `project_id=ProjectID`, `kind=dispatcher`, and matching session/run.
- `ListClients`, `Select`, `Attach` and `Jump` need no other change: they verify
  and then address `=<SessionName>`. `workspace dispatcher attach` is untouched.

### TUI — `internal/tui/keymap.go`, `navigation_flow.go`, `detail.go`

- `jumpCapable` accepts `dispatcher`.
- New `dispatcherJumpReady()` is `m.project.Dispatcher.State == "running"`
  (`Session.Active` already implies a non-empty `CurrentRunID`).
- `contextFlags().jump` becomes
  `jumpCapable(kind) && (kind != "dispatcher" || m.dispatcherJumpReady())`, so
  the footer (`shortHelp`) and full help (`keyGroups`) hide `g` on an idle
  Dispatcher.
- `jumpSelected` gains a dispatcher branch before the generic branch: it returns
  `nil` when `!dispatcherJumpReady()` and otherwise dispatches
  `EntityRef{Kind: "dispatcher"}` through the unchanged verified-client
  resolution → picker → preference → recheck/`Jump` flow.
- `dispatcherContent` advertises `g jump` in the page hint only when ready.
- No actions-menu entry is added for the Dispatcher.

### Docs — `docs/tui.md`, `PRODUCT.md`, `ARCHITECTURE.md`

- The Dispatcher page is documented as jump-capable while its Run is live, that
  the shortcut is hidden and inert otherwise, and that
  `workspace dispatcher attach` remains unchanged.

## Acceptance criteria

- `core.ResolveNavigationTarget` resolves `Kind=dispatcher` from `DispatcherStatus`
  durable state plus the verified project pane carrying the current Run ownership
  metadata; sets `ProjectID` and the canonical
  `workspace-dispatcher-<project-id>` session name; returns `pane_missing` for
  not-started, no live run, unverified and ownership-mismatch cases. No heuristic
  scan. — `internal/core/dispatcher_navigation_test.go`.
- `core.NavigationTarget` gains `ProjectID`; existing workspace kinds resolve
  unchanged. — `TestWorkspaceRuntimeAndNavigationUseVerifiedOwnership` still passes.
- `terminal.TmuxNavigator.verify` accepts a project-scoped dispatcher target and
  keeps workspace-kind behavior unchanged; `ListClients`/`Jump` work on the
  dispatcher session. — `internal/terminal/dispatcher_navigation_test.go`.
- TUI `jumpCapable` includes dispatcher; `contextFlags().jump` is true only when
  running; footer/full help/page hint advertise `g jump` only then; `g` with no
  live dispatcher does nothing. — `internal/tui/dispatcher_navigation_test.go`,
  `keymap_test.go`, `project_scope_test.go`.
- `jumpSelected` dispatches `EntityRef{Kind: dispatcher}` through the unchanged
  verified-client flow (discovery/choice, preference save on success,
  `navigation_target_changed` and `client_gone` handling); no actions-menu entry.
- Tests added for successful jump, no live dispatcher, changed target, detached
  client (TUI), core resolution (not-started/idle/unverified/mismatch) and
  terminal accept/reject; existing navigation/keymap/project-scope/query/terminal
  tests keep passing.
- No `workspace dispatcher attach` CLI change; gofmt clean; `go vet ./...` passes.
- Committed in the worktree; this report and `SUMMARY.md` written.

## Checks

| Command | Exit | Evidence |
|---|---:|---|
| `gofmt -l ./cmd ./internal` | 0 | `evidence-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-vet.txt` |
| `go test ./internal/tui/ -count=1` | 0 | `evidence-go-test-tui.txt` |
| `go test ./internal/terminal/ -count=1` | 0 | `evidence-go-test-terminal.txt` |
| `go test ./internal/core/ -run '<navigation/dispatcher/supervisor tests>' -count=1` | 0 | `evidence-go-test-core-targeted.txt` |
| `env -u WORKSPACE_AGENT_ID -u WORKSPACE_SESSION_ID -u WORKSPACE_RUN_ID go test ./... -count=1 -timeout 180s` | 0 | `evidence-go-test-all.txt` |
| `WORKSPACE_TMUX_TEST=1 env -u … go test -race ./internal/core/ -run 'Navigator\|Navigation\|Dispatcher\|Supervisor\|Tmux' -count=1 -timeout 300s` | 0 | `evidence-tmux-core.txt` |
| `WORKSPACE_TMUX_TEST=1 env -u … go test -race ./internal/terminal/ ./internal/tui/ -count=1 -timeout 120s -skip 'TestNavigatorRealPTYAndClientSelection'` | 0 | `evidence-tmux-term-tui-skip.txt` |
| `python3 work-products/check-doc-links.py` | 0 | `evidence-doc-links.txt` |

The full `go test ./...` passes once the workspace worker environment variables
(`WORKSPACE_AGENT_ID`/`WORKSPACE_SESSION_ID`/`WORKSPACE_RUN_ID`) are unset; with
them set, the pre-existing `TestWorkerProcess` helper re-exec assumes the test
binary argv and fails. The tmux suite's only failure,
`TestNavigatorRealPTYAndClientSelection` (`pseudo-TTY client did not attach`), is
pre-existing and reproduces on base `aa72616` in this environment, so the declared
tmux check skips it; the unskipped failing output is retained as informational
evidence in `evidence-tmux-term-tui.txt`. See `CHECKS.yaml` for the exact commands
and the pre-existing note.

## Risks and deviations

- The TUI gate uses durable `DispatcherSummary.State == "running"`, not a live tmux
  observation (the plan's accepted trade-off). A pane that dies between refreshes
  may briefly advertise `g`; resolution fails closed with `pane_missing` and the
  standard notice.
- Reconcile is intentionally not offered for the dispatcher failure path because
  the page is project-scoped and `ReconcileWorkspace` is workspace-scoped. This
  matches the plan; no dedicated dispatcher stop notice was added.
- No install/build contract changed, so `scripts/check-install.py` was not run.


---

# Incoming implementation report — task_01M3P156JTWMYAT9TPBDMZ917B (recovery fix)

Preserved verbatim from the accepted recovery-fix head `3384ce3`; the shared
`work-products/IMPLEMENTATION.md` path was rewritten by both lineages.

---

# Implementation report — task_01M3P156JTWMYAT9TPBDMZ917B

Fix supervisor orchestrator recovery regression from `2a72e3a` (`TestTmuxEndToEnd`).

## Commit

- Base: `2a72e3afcb134e0ab9c82961898aad331ab1c642` (master tip)
- Fix: `5b75caa4382f7359df3af368eb59d04d6b95b5ba`
  - `fix: keep session client snapshot on resume for recovery`
- Files: `internal/core/session.go`, `internal/core/reasoning_effort_test.go`,
  `ARCHITECTURE.md`, `docs/runtime.md`

## Symptom

`WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -run TestTmuxEndToEnd`
fails at `internal/core/tmux_integration_test.go:343`
("supervisor did not recover the orchestrator after its whole tmux window was lost").
Passes on parent `367c006`; fails on unmodified `2a72e3a`.

## Root cause

`TestTmuxEndToEnd` makes the orchestrator long-running by mutating the logical
Session's client snapshot before the explicit resume:

- `tmux_integration_test.go:193-203` sets
  `current.ClientSnapshot.LaunchArgv = ["sh","-c","sleep 30"]`
  (the configured `command` client helper, `TestWorkerProcess`, exits immediately).
- `tmux_integration_test.go:204` resumes the orchestrator.
- `tmux_integration_test.go:300-315` records the orchestrator run and then kills its
  whole window (`kill-window`).
- The supervisor recovers an orchestrator only while it is `interrupted` and had a
  live Run: `internal/core/supervisor.go:393-401`
  (`orch.State == "interrupted"`, then `ResumeAgent`). If the orchestrator already
  `exited`, `Reconcile` never marks it interrupted
  (`internal/core/session.go:1029-1037`) and nothing is recovered.

`2a72e3a` removed the block in `StartSession` that carried the logical Session's
client definition onto the resumed Run (the block deleted between
`logical = prior` and `thread` handling in `internal/core/session.go`). After that,
a resumed Run resolves its client *definition* from the current project config. In
the test the snapshot `["sh","-c","sleep 30"]` was therefore replaced by the
configured one-shot helper:

```
DBG resume role=orchestrator ... priorLaunch=[sh -c sleep 30]
             -> launch=[... core.test -test.run=TestWorkerProcess ...]
```

The resumed orchestrator exited in milliseconds, so by the time its window was
killed it had no live Run and the supervisor had nothing to recover. Recovery is
the normal resume path (`ResumeAgent` → `StartSession`), so a resumed Run must be
able to resolve a stable client definition for the Session.

## Fix

`internal/core/session.go:316-334`: restore only the Session's client definition
into the local routing config before `chooseRoute`, keeping the `2a72e3a` reload of
profile, route, limits and reasoning effort:

```go
if prior.Route.Client != "" && prior.ClientSnapshot.Adapter != "" {
    // A logical Session owns its client definition. Keep the launched
    // client stable across Runs so that a resumed Run — including the
    // supervisor recovering a lost pane — can still resolve the exact
    // client it was running. The profile, route, limits and reasoning
    // effort are still reloaded from the current project configuration
    // below; only the client definition is carried over.
    if cfg.Clients == nil { cfg.Clients = map[string]Client{} }
    priorClient := prior.ClientSnapshot
    if usesNativeOpenCodeDelivery(priorClient) {
        priorClient.NativeDelivery = true
    }
    cfg.Clients[prior.Route.Client] = priorClient
}
```

`route, err := s.chooseRoute(cfg, profile)` and the following
`reasoningEffort` resolution are unchanged, so the resumed Run still uses the
current profile/route/limits/effort. The old `2a72e3a`-removed profile-route
pinning (`profileCfg.Routes = []Route{prior.Route}`) and prior-effort override are
**not** restored.

## Deviation from 2a72e3a (documented)

`2a72e3a` intent: a resumed Run reloads the current project configuration,
including the client. This fix narrows that for the client *definition*: the
resumed Run reloads profile/route/limits/reasoning effort but keeps the client
definition owned by the logical Session. Rationale: recovery reuses the resume
path, and a resolvable, stable client is required for a lost pane to be recovered
(and for a long-running client to remain running across resume). The routing
reload — the actual subject of `2a72e3a` — is preserved.

Tests proving each side:

- `TestReasoningEffortReloadsOnResume` (kept green): route `Model`/`MaxConcurrency`
  and `ReasoningEffort` are reloaded from current config on resume.
- `TestResumeKeepsSessionClientSnapshotAndReloadsRoute` (new): on resume the Run
  argv keeps the Session's client snapshot while `Route.Model` is reloaded.
- `TestTmuxEndToEnd` (kept green): whole-window supervisor recovery works again.

## Check results

All commands below were run with the harness `WORKSPACE_*` environment cleared
(the sandbox exports worker variables that otherwise break the CLI/core fixtures);
`WORKSPACE_TMUX_TEST=1` re-set only for the tmux runs. Real evidence files.

Declared checks (all exit 0):

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l ./cmd ./internal` | 0 | `evidence-fix-recovery-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-fix-recovery-vet.txt` |
| `go test ./... -count=1 -timeout 570s` | 0 | `evidence-fix-recovery-go-test-all.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -run TestTmuxEndToEnd -count=1` | 0 | `evidence-fix-recovery-tmux-e2e.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./... -count=1 -skip TestNavigatorRealPTYAndClientSelection` | 0 | `evidence-fix-recovery-tmux-all.txt` |
| `go test ./internal/core/ -run TestResumeKeepsSessionClientSnapshotAndReloadsRoute` | 0 | `evidence-fix-recovery-regression-pass.txt` |

Documented evidence, NOT declared checks (intentional-failure proofs and the known
environmental failure):

| Command | Exit | Kind | Evidence |
|---|---|---|---|
| Base `2a72e3a` + the new regression test | 1 | Intentional-failure proof (catches the regression) | `evidence-fix-recovery-regression-base.txt` |
| Base `2a72e3a` + `TestTmuxEndToEnd` | 1 | Intentional-failure proof (reproduces the report at `tmux_integration_test.go:343`) | `evidence-fix-recovery-tmux-e2e-base.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/terminal/ -run TestNavigatorRealPTYAndClientSelection` | 1 | Known environmental failure, pre-existing | `evidence-fix-recovery-terminal-known-red.txt` |

`TestNavigatorRealPTYAndClientSelection` is a pre-existing environmental failure
(`pseudo-TTY client did not attach`); it fails identically on the base and the
`internal/terminal` package is byte-for-byte unchanged vs `2a72e3a`, so it is
excluded from the tmux acceptance run with `-skip` and documented here.

## Acceptance criteria

- `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -run TestTmuxEndToEnd`
  passes; the failure at `tmux_integration_test.go:343` is gone. ✅
- Root cause with code evidence documented above (client snapshot dropped by
  `2a72e3a`; recovery needs a resolvable, stable route/client). ✅
- `2a72e3a` intent preserved where compatible: `TestReasoningEffortReloadsOnResume`
  and other routing/reasoning tests pass; the client-snapshot deviation and its
  rationale and proving tests are documented. ✅
- `gofmt -l` clean; `go vet ./...` passes; `go test ./...` passes;
  `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/` passes. ✅
- No unrelated refactors; changes bounded to the resume/recovery path plus one
  regression test and the two docs whose `2a72e3a` wording became wrong
  (`ARCHITECTURE.md`, `docs/runtime.md`). ✅
- Committed in the worktree; `IMPLEMENTATION.md` and `SUMMARY.md` submitted with
  real check evidence. ✅

## Risks

- A route whose `Client` name is no longer present in the current profile selects a
  different client, so the snapshot is not used; this is the intended route reload
  and fails closed only if no eligible route remains.
- If the configured client definition is intentionally changed for the same client
  name, a resumed Session keeps the older argv until a new logical Session is
  started. This is the documented deviation above.
- The fix does not add pane-liveness gating to resume; the supervisor still only
  recovers an `interrupted` Session with a recorded Run, as before.

---

# Incoming implementation report — completion-exemption (`task_01M3P2JFEFSWE8CD2VHS5PXBQW`)

Preserved verbatim from the accepted implementation head `c8b3bfd4f730f5f52eaeade4ffdec2edb7ef2670`; the shared `work-products/IMPLEMENTATION.md` path was rewritten by both lineages.

---

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


---

# Incoming implementation report — auto-close accepted worker sessions (`task_01M3P785FNW7WPVAQ0NJKVHCX0`)

Preserved verbatim from the accepted implementation head `415b97cb6919fbb1057c764c323ddfafde57c18f`; the shared `work-products/IMPLEMENTATION.md` path was rewritten by both lineages.

---

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


---

## Lineage B - incoming T1 `impl-core` report (d0bb317)

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


---

## Lineage B - incoming T2 `impl-prompts` report (c5c78da)

# IMPLEMENTATION — T2: Orchestrator evaluation notice in prompts, templates and skill

Task: `task_01M3P9E7ZSW0572P8QEP1A7BWH`
Plan: `art_01M3P7H0BN7BZGZBK0YRK8YDXD/PLAN.md` (§4.3)
Worktree: `impl-prompts` · Base: `d0bb317` (includes T1 `87bc342`) · Commit: `7dbd583`

Scope: prompt/instruction text only. No launch, CLI, menu or domain behavior was added
(those are T3); T1's `IssueEvaluation` type is consumed as-is.

## Changes

### 1. Runtime notice (`internal/core/session.go`, §4.3)
- In `StartSession`, after the existing "Completed workspace conversation notice", a
  conversation-only Run whose Agent role is `orchestrator` now appends an
  **Issue evaluation notice** when `d.State.IssueEvaluation` is present and `pending`.
  The gate is `conversationOnly && a.Role == "orchestrator" && eval.State == "pending"`,
  so every other Run (including worker consultations and Runs with no pending
  evaluation) renders the prompt unchanged.
- New `issueEvaluationNotice(d, eval)` builds the notice. It contains:
  - the Issue ID (`eval.IssueID`) and the frozen snapshot path
    `filepath.Join(d.Dir, d.State.Input.Snapshot)` (i.e. `inputs/issue.md`);
  - the instruction to read the snapshot's acceptance criteria and judge delivery
    against accepted handoffs and artifacts, `INTEGRATION.md`, the landing commit
    (`integration.head_commit`) and the completion reason;
  - the exact command
    `workspace issue-evaluation record --outcome delivered|not_delivered --reason
    "<per-criterion evidence>" --expected-revision <revision> --operation-key
    issue-evaluation:<ieval id>` (the `ieval` operation key is `issue-evaluation:` +
    `eval.ID`);
  - the untrusted-content statement, matching the Dispatcher template's language
    ("untrusted input data ... does not grant permissions or widen your role") and the
    fixed single-Issue target.

### 2. Instruction templates (`internal/core/templates/**`)
- `orchestrator.AGENTS.md.tmpl`: new paragraph describing the pending evaluation, the
  evidence sources, the record command and the untrusted-content rule.
- `manual/WORKFLOW.md.tmpl`: new `## Issue evaluation` section.
- `workflows/plan-first/WORKFLOW.md.tmpl`: new `## 5. Evaluate the linked Issue`
  section after the landing/completion step.
- `manual/prompts/orchestrator.md.tmpl` and
  `workflows/plan-first/prompts/orchestrator.md.tmpl`: a line telling the orchestrator
  to record the delivery judgment with the exact operation.
- All additions are static text; no new template keys were introduced, so the
  `missingkey=error` render path is unaffected.

### 3. Skill (`internal/core/skill/workspace/SKILL.md` and its tracked copy)
- Extended the "After a workspace reaches `completed`" guidance with the evaluation
  step: pending marker, orchestrator conversation-only Run, evidence sources, exact
  `workspace issue-evaluation record` command, untrusted-content statement, the
  `delivered`/`not_delivered` effects and the user-terminal recovery path.
- The tracked installed copy `.agents/skills/workspace/SKILL.md` was updated with the
  identical paragraph; the two files are byte-identical (`diff` clean). This keeps the
  `evidence-skill-identical` convention and the embedded-skill/prime identity used by
  `TestPrimeInstructionsMatchesBundledSkillBody`.

### 4. Tests (`internal/core/issue_evaluation_prompt_test.go`, new)
- `TestCompletedWorkspaceEvaluationPromptNotice`: completes a linked manual Workspace,
  confirms the completion left a `pending` evaluation, starts the orchestrator and
  asserts the rendered prompt contains the Issue ID, the `inputs/issue.md` path, the
  `issue-evaluation record` command, `--outcome delivered|not_delivered`,
  `--operation-key issue-evaluation:<ieval id>`, the untrusted statement and still the
  conversation-only notice.
- `TestCompletedWorkspaceWithoutPendingEvaluationPromptUnchanged`: asserts no notice is
  emitted when the evaluation is `recorded` (linked) or absent (unlinked), while the
  pre-existing conversation-only notice remains. This is the gating that makes the
  no-pending prompt byte-identical to before.

## Acceptance criteria mapping

| Criterion | Evidence |
|---|---|
| Conversation-only orchestrator prompt for a completed Workspace with a pending evaluation contains the Issue ID, the `inputs/issue.md` path and the exact record command with the `ieval` key | `TestCompletedWorkspaceEvaluationPromptNotice`; targeted evidence |
| Without a pending evaluation the prompt is byte-identical to the current one | Notice is gated on `conversationOnly && orchestrator && pending`; `internal/core/session.go:413-421`; `TestCompletedWorkspaceWithoutPendingEvaluationPromptUnchanged` |
| Skill and template text describes the step and stays in sync with the skill-identity test | Template/SKILL edits; `diff` of the two tracked skill copies is empty; `TestPrimeInstructionsMatchesBundledSkillBody` passes |
| `gofmt` on changed files; `go test ./...` and `go vet ./...` pass; committed | checks below; commit `7dbd583` |

## Checks

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l ./cmd ./internal` | 0 (clean) | `evidence-t2-prompts-gofmt.txt` |
| `env -u WORKSPACE_* go vet ./...` | 0 | `evidence-t2-prompts-vet.txt` |
| targeted core tests (new prompt tests + skill/template/autonomy/reopen tests) | 0 | `evidence-t2-prompts-targeted.txt` |
| `env -u WORKSPACE_* go test ./... -count=1 -timeout 570s` | 0 | `evidence-t2-prompts-go-test-all.txt` |

The orchestration exports `WORKSPACE_*`, which the suite's in-process child-process
helper interprets as a nested invocation. The commands therefore unset the
`WORKSPACE_*` variables, as the T1 checks and earlier `work-products/CHECKS-I4.yaml`
do; this is an environment artifact, not a code issue.

## Risks / notes

- The notice is instruction text only. It documents the `workspace issue-evaluation
  record` operation, which is implemented by T3; until T3 lands, the command is not yet
  available in the CLI. There is no runtime dependency in this change.
- The workspace's project-level `.workspace/templates/**` overrides were intentionally
  not changed: they are the project's own pinned runtime configuration and the task
  scope names `internal/core/templates` only.
- The `--expected-revision` value in the notice is the plan's `<revision>` placeholder;
  the orchestrator reads the current revision from `workspace status --json`. The
  operation itself is revision-guarded.
- No `--user-confirmed` is required for the evaluation, matching the accepted plan
  assumptions; this change does not alter authorization.


---

## Lineage B - incoming T5 `impl-tui` report (f737a52)

# IMPLEMENTATION — T5: Evaluation state in project overview link and TUI Issue detail

Task: `task_01M3P9E8MAVA0KN5WP767JJ5FB`
Plan: `art_01M3P7H0BN7BZGZBK0YRK8YDXD/PLAN.md` (§4.6 optional part, T5)
Worktree: `impl-tui` · Base: `d0bb317` · Commit: `9958a07`

Depends on T1 (`87bc342`, core `IssueEvaluation`) which is already in this worktree.

## Changes

### 1. Read models carry the evaluation (`internal/core/issues.go`, `query.go`, `model.go`)
- `IssueWorkspaceLink` gained `EvaluationState` and `EvaluationOutcome`
  (`evaluation_state` / `evaluation_outcome`, `omitempty`). Both are empty when the
  linked Workspace has no `issue_evaluation`.
- `WorkspaceSummary` (the project overview row) gained the same two fields, so the
  overview carries the evaluation without a second Workspace read.
- `ProjectOverview` populates `WorkspaceSummary.EvaluationState/Outcome` from
  `status.Workspace.IssueEvaluation` and copies them into each
  `IssueSummary.LinkedWorkspaces` entry.
- `linkedWorkspaces` (`issues.go`) copies the row fields into `IssueWorkspaceLink`, so
  `issue show` / `IssueDetail` carries them too.
- New unexported helper `linkedIssueEvaluation(*IssueEvaluation) (state, outcome string)`
  in `model.go`: a nil evaluation yields two empty strings, making "no evaluation"
  indistinguishable from Workspaces that predate the field (strict-YAML backward
  compatibility is preserved because the JSON/YAML shape only gains omitempty keys).

### 2. TUI Issue detail rendering (`internal/tui/detail.go`)
- Each "Linked workspaces" bullet now appends `· evaluation <state>` and, when an
  outcome exists, `(<outcome>)`, e.g. `Delivery · completed · revision 2 · evaluation
  pending` or `Shipped · completed · revision 2 · evaluation recorded (delivered)`.
- A linked Workspace with no evaluation renders the exact previous string, so the
  change is invisible for un-evaluated/legacy Workspaces.

## Acceptance criteria mapping

| Criterion | Evidence |
|---|---|
| `IssueWorkspaceLink` and project overview rows carry state and outcome | `TestReadModelsCarryLinkedEvaluationState` asserts pending then recorded/delivered on both `ProjectOverview.Workspaces` and `ShowIssue.LinkedWorkspaces`; empty before completion |
| TUI Issue detail renders the evaluation state for a linked completed Workspace; Workspace without `issue_evaluation` is unchanged | `TestIssueDetailShowsLinkedWorkspaceEvaluation` (pending, recorded+delivered, and an un-evaluated "Legacy" row) |
| Tests follow existing TUI/overview conventions | Core test reuses `fixture`/`linkedManualWorkspace`/`completeLinkedWorkspace`/`startEvaluationOrchestrator`; TUI test follows `project_scope_test.go` |
| gofmt on changed files; `go test ./...` and `go vet ./...` pass; commit in the task worktree | checks below; commit `9958a07` |

## Checks

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l ./cmd ./internal` | 0 (clean) | `evidence-t5-eval-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-t5-eval-vet.txt` |
| targeted `internal/core` + `internal/tui` tests | 0 | `evidence-t5-eval-targeted.txt` |
| `go test ./... -count=1 -timeout 570s` | 0 | `evidence-t5-eval-go-test-all.txt` |

The workspace orchestration exports `WORKSPACE_*` variables, which the suite's
in-process helper (`TestWorkerProcess`) interprets as a child-process invocation. The
commands therefore unset them, as earlier `work-products/CHECKS-*.yaml` do.

## Risks / notes

- The change is read-model only; no new mutation, schema-strictness or authority
  behavior is introduced. `IssueEvaluation` itself was added by T1.
- `evaluation_outcome` is rendered only when a recorded evaluation sets it; a pending
  evaluation shows just the state.
- Scope was held to the required surfaces (project overview row + TUI Issue detail).
  The Issues collection subtitle and the Workspace detail were intentionally left
  unchanged.
- `work-products/IMPLEMENTATION.md` and `SUMMARY.md` are tracked leftovers from the T1
  report; they are overwritten here as this task's reports, consistent with plan
  assumption 6.


---

## Lineage B - incoming T3 `impl-launch` report (5d7d489)

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


---

## Lineage B - incoming T4 `impl-docs` report (c683071)

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

---

# Incoming implementation report - T1: Named IDs core (slugs, reservation ledger, core adoption)

Preserved verbatim from the accepted implementation head `41fa1241480d5b84eb3bcfe6daa440b5fbfc99d1`.

# IMPLEMENTATION — T1: Named IDs core (slugs, reservation ledger, core adoption)

- Task: `task_01M3Q50KD2ASX479XZK15YBHRR` (`core-named-ids`), role implementer.
- Worktree: `worktrees/impl-core` (branch `workspace/ws_01M3PC44DNG1RA29ME3PKPGYSJ/impl-core`).
- Base commit: `673bb6514fad11f29c87b880d1307d30e0e5d532`.
- Implementation commit: `aeee481` (`core: mint slug IDs with a reservation ledger`).
- Accepted plan: `art_01M3Q1HXZ8KMT7QCQJD682H78P/PLAN.md` (sections 2.1–2.6, task T1).

## What changed

New files:

- `internal/core/slug.go` — slug grammar, `slugify`, reserved shapes, `baseSlug`, `parseExplicitID`.
- `internal/core/idreserve.go` — never-released reservation ledger (`<storage>/.runtime/ids/<kind>/<slug>`, `O_EXCL`) and `idAllocator` (`allocate`, `allocateExplicit`).
- `internal/core/slug_test.go` — normalization/validation/ledger/concurrency/adoption/legacy/tmux tests.

Wired adoption (each site now mints `<prefix>_<slug>` instead of `<prefix>_<ULID>`):

- Workspace and orchestrator agent: `internal/core/project.go` (`createWorkspaceLegacy`).
- Worker agents: `internal/core/agent.go` (`CreateAgent`).
- Logical sessions: `internal/core/session.go` (`StartSession`, new-logical branch only).
- Tasks: `internal/core/task.go` (`CreateTaskWithID`; `CreateTask` delegates with an empty explicit ID).
- Dispatcher agent and session: `internal/core/dispatcher_runtime.go` (project storage root, workspace id `-`).

Option surface and resolution:

- `CreateOptions.ID`, `AgentOptions.ID`, `SessionOptions.ID` (all `json:",omitempty"`; `CreateOptions.ID` also `yaml:",omitempty"`).
- `CreateTaskWithID(ctx, selector, spec, explicitID, key)`; the request payload stays `spec` when the ID is empty and becomes `struct{Spec TaskSpec; ID string}` only when set.
- `findAgent` and `findTask` match an exact ID first, then a name (`internal/core/agent.go`, `internal/core/task.go`).
- A workspace `os.Rename` collision is mapped to `id_exists` (`project.go`).
- `go.mod`: `golang.org/x/text` promoted from indirect to direct (already in `go.sum`; no new module).

## Design decisions

- **Separator `_` and slug grammar** exactly as PLAN §2.1: `^[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])?$`, base ≤ 40, total ≤ 48.
- **Reserved shapes** (`^[0-9a-hjkmnp-tv-z]{26}$` lowercase ULID, `^[0-9a-f]{24}$` migrated session hex) are never produced un-suffixed. `slugify` returns the kind's fallback word for an empty/reserved/otherwise invalid result (PLAN §2.2 step 5), and `allocate` additionally skips a reserved candidate defensively. Explicit reserved values are refused with `invalid_id`.
- **Ledger operation identity** is `<project_id>/<workspace_id or "-">/<kind>/<key>`. An unkeyed create has an empty operation and therefore never reuses a leaked reservation; a keyed retry reuses its own reservation even when the process crashed after reserving but before saving.
- **Uniqueness scope** is the storage root (PLAN §2.3 / Q1 default): one project by default, cross-project when `workspaces_dir` is shared. `O_EXCL` makes this atomic across processes and project locks.
- **Task explicit-ID timing**: the idempotency request is captured after `TaskSpec.RequiredArtifacts` defaulting, matching the value the pre-existing `CreateTask` passed to `d.previous`. Capturing it earlier would have changed existing task receipt digests.
- **Dispatcher identity**: the first-time dispatcher agent/session are allocated at the project storage root with workspace id `-`, using the caller's operation key so a keyed restart reuses them.
- **`s.Config()` is now loaded unconditionally** in `CreateAgent`/`CreateTask` because it is needed for the storage root; previously it was loaded only when the profile was empty.
- **Test-only fix outside `internal/core`**: `internal/bootstrap/context_test.go` previously relied on two independent projects producing distinct workspace IDs for the same title. With slug IDs both become `ws_example`, so the "foreign workspace" fixture now uses a different title (`bootstrapProject(t, title)`). This is a test assumption invalidated by the intended behavior change; product code is untouched.

## Acceptance criteria

| # | Criterion | Status | Evidence |
|---|---|---|---|
| 1 | New workspace (incl. orchestrator/dispatcher), agent, logical session and task IDs match `^(ws\|agent\|sess\|task)_[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])?$`; Runs stay `^run_[0-9A-Z]{26}$` | Met | `TestWorkspaceSlugDerivationAndNoReuseAfterDelete`, `TestAgentSessionTaskSlugsAcrossWorkspaces`, `TestExplicitWorkspaceAndAgentIDs`, dispatcher path in `TestDispatcherTmuxEndToEnd` (full suite) |
| 2 | Normalization table incl. Polish diacritics, ß/æ/ø/ł, emoji/empty → fallback, leading digits, hyphen-boundary truncation ≤ 40, reserved shapes never un-suffixed | Met | `TestSlugifyNormalizationTable`, `TestSlugFallbacksAndReservedShapes`, `TestSlugTruncationAtHyphenBoundary`, `TestAllocatorDoesNotReuseReservedShape` |
| 3 | Collisions → base, base-2, base-3 deterministically, across two workspaces in one storage root and against soft-deleted tasks/sessions | Met | `TestAllocatorCollisionsAreDeterministic`, `TestAgentSessionTaskSlugsAcrossWorkspaces`, `TestTaskSlugDoesNotReuseSoftDeletedID`, sessions use the same `d.Registry.Sessions` scan (including deleted) |
| 4 | After `DeleteWorkspace`, same title yields `…-2` (no reuse) | Met | `TestWorkspaceSlugDerivationAndNoReuseAfterDelete` |
| 5 | Concurrent allocation from N goroutines (`-race`) and from two Services sharing an external `workspaces_dir` gives distinct IDs, no errors | Met | `TestAllocatorConcurrentGoroutines`, `TestSharedStorageProjectsGetDistinctSlugs` (both run under `-race`); `TestSharedStorageKeepsProjectsIsolated` still passes |
| 6 | Keyed create that fails after reservation and before save, retried with the same key/payload, returns the same ID; a different key gets the next suffix | Met | `TestKeyedWorkspaceCreateReusesReservationAfterFailure`, `TestAllocatorKeyedReuseAndDifferentKeySuffix` |
| 7 | Explicit ID with/without prefix; invalid shapes → `invalid_id`; taken → `id_exists`; explicit ID in the payload, different explicit ID under the same key → `operation_conflict` | Met | `TestParseExplicitID`, `TestAllocatorExplicitIDExists`, `TestExplicitWorkspaceAndAgentIDs`, `TestExplicitTaskAndSessionIDs` |
| 8 | Legacy ULID workspace/agent/session/task and migrated `sess_<hex>` open, resolve by every old ID (incl. Run-ID alias), accept new slug IDs, leave pre-existing records unchanged | Met | `TestLegacyWorkspaceCompatibility` (before/after `reflect.DeepEqual` on the legacy task; resolves agent/task/session by legacy ID and session by Run-ID alias) |
| 9 | `DeleteWorkspace` basename check, `InferWorkspace`, `workspaceDirs`, `ProjectOverview`, `resolveReadableWorkspace` work for slug workspaces with no prefix-filter change | Met | No prefix filters were changed; slug directories satisfy the existing `ws_` prefix. Covered by the full suite and `TestWorkspaceSlugDerivationAndNoReuseAfterDelete` |
| 10 | `gofmt`, `go vet`, `go test ./...` pass; tmux race suite incl. a slug workspace tmux test (session name, pane recovery, stop isolation) | Partially met (see Limitations) | `TestTmuxSlugWorkspaceIdentity` passes; `go test ./...` and `go vet ./...` pass; full core `-race` + tmux passes with `-timeout 300s`. The exact `-timeout 90s` command exceeds 90 s for `internal/core` in this sandbox at base as well |

Digest compatibility is also asserted directly by `TestNewOptionFieldsDoNotChangeExistingDigests`: the new fields are absent from the JSON payload when empty. Existing receipts therefore replay unchanged, and all pre-existing idempotency tests in the suite pass.

## Checks

Run from the worktree root. Because the agent's shell exports `WORKSPACE_*` variables, the go test commands were run with every `WORKSPACE_*` variable unset (otherwise the child-process helper tests, e.g. `TestWorkerProcess`, trigger inside the main suite). Evidence files are in `work-products/`.

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l internal/` | 0 | `evidence-named-ids-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-named-ids-vet.txt` |
| `go test ./... -timeout 600s -count=1` | 0 | `evidence-named-ids-go-test-all.txt` |
| `go test -race ./internal/core/ -run 'TestSlug\|TestParseExplicitID\|TestAllocator\|TestWorkspaceSlug\|TestAgentSessionTask\|TestTaskSlug\|TestExplicit\|TestKeyed\|TestSharedStorage\|TestNewOption\|TestLegacy' -count=1` | 0 | `evidence-named-ids-targeted-race.txt` |
| `WORKSPACE_TMUX_TEST=1 go test ./internal/core/ -run TestTmuxSlugWorkspaceIdentity -count=1 -v -timeout 180s` | 0 | `evidence-named-ids-tmux-slug-identity.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -timeout 300s -count=1` | 0 | `evidence-named-ids-core-race-tmux.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s -count=1` | 1 | `evidence-named-ids-race-tmux-timeout.txt` |

`TestTmuxSlugWorkspaceIdentity` creates `ws_fix-checkout` and `ws_fix-checkout-2`, asserts both exact tmux session names (`workspace-ws_fix-checkout`, `workspace-ws_fix-checkout-2`), recovers an unrecorded pane inside the first session via `Reconcile`, then stops the first workspace and asserts the live `-2` sibling and its pane are untouched.

## Limitations and risks

- **`-timeout 90s` is not achievable in this sandbox.** `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/` takes ~246 s here; the base suite without this task's ~9 s tmux test would still exceed 90 s under the race detector. The package passes at `-timeout 300s` (exit 0). This is an environment timing limit, not a failure of this change.
- **`TestNavigatorRealPTYAndClientSelection` (`internal/terminal`) fails in this sandbox**: `pseudo-TTY client did not attach`. This test is not touched by this change and relies on `script(1)`/tmux PTY behavior under WSL; it is recorded in `evidence-named-ids-terminal-pty.txt` and in the 90 s run.
- The unique suffix for common names grows over time (`agent_orchestrator-N`, `sess_orchestrator-N`), the accepted PLAN Q3 trade-off.
- Reservations are never released, so an unkeyed crash leaves a gap (`-2` without a visible `-1`). Documented as intended in the plan.
- `O_EXCL` atomicity depends on the filesystem; the plan already accepts the exotic-filesystem risk.
- `internal/bootstrap/context_test.go` was adjusted (test-only) so the foreign-workspace assertion remains meaningful under slug IDs.

## Not in scope (T2)

CLI `--id` flags and `IssueDispatchOptions.ID`, selector/help text, `workspace open` exact-ID precedence, TUI slug rendering, and the documentation updates in PLAN §2.9 remain for `cli-docs-named-ids`.

---

# Incoming implementation report - T2: Named IDs CLI flags, TUI display, documentation

Preserved verbatim from the accepted implementation head `97a819cd00caf14098e05ef8466ba9fa13d51717`.

# IMPLEMENTATION — T2: Named IDs CLI flags, TUI display, documentation

- Task: `task_01M3Q51KDCNHXRPKC0B994DW76` (`cli-docs-named-ids`), role implementer.
- Worktree: `worktrees/impl-cli-docs-rebased` (branch `workspace/ws_01M3PC44DNG1RA29ME3PKPGYSJ/impl-cli-docs-rebased`).
- Base commit: `673bb6514fad11f29c87b880d1307d30e0e5d532`.
- T1 dependency head: `41fa124` (includes `aeee481`, the core slug/reservation implementation).
- Implementation commit: `7b5963373e3cfe9f652e0014f88c9f515e5e027f` (`feat: add named-ID CLI flags, TUI slugs and documentation`).
- Accepted plan: `art_01M3Q1HXZ8KMT7QCQJD682H78P/PLAN.md` (sections 2.6, 2.9, task T2).
- T1 report: `art_01M3Q8A7FAHM3T80A8WQ2VWWPJ/IMPLEMENTATION.md`.

## What changed

### CLI `--id` flags (PLAN §2.6)

| Surface | Change |
|---|---|
| `workspace create` | `--id` bound to `CreateOptions.ID` (`internal/cli/cli.go`). |
| `issue dispatch` | `--id` bound to `IssueDispatchOptions.ID`, passed through to the linked workspace (`internal/cli/issue.go`, `internal/core/issues.go`, `internal/core/issue_workspace.go`). |
| `workspace agent create` | `--id` bound to `AgentOptions.ID` (`internal/cli/cli.go`). |
| `workspace task create --spec-file` | `--id` forwards to `CreateTaskWithID` (`internal/cli/work.go`). |
| `workspace session start` | `--id` bound to `SessionOptions.ID` (`internal/cli/cli.go`). |

`issue dispatch --id` is the one addition outside `internal/cli`/`internal/tui`: `issue
dispatch` builds a fresh `CreateOptions` internally and had no field to carry the value.
`IssueDispatchOptions.ID` is tagged `json:",omitempty"` so an empty value is absent from
the dispatch receipt payload and existing receipt digests do not change.

### Resolution and help

- `chooseWorkspace` now checks an exact ID match first and only then falls back to a
  case-insensitive title match. A selector that is both a workspace ID and another
  workspace's title is never reported as ambiguous (`internal/cli/cli.go`).
- Selector help text now reads "slug or legacy ID" for `--workspace`, `--agent`,
  `--parent`, `--parent-session`, `--task`, `--session`, `--to-session`, and the
  corresponding positional arguments; the new `--id` flags have help entries.
- `internal/cli/help.go` examples use slug-shaped placeholders (`ws_named-ids`,
  `sess_planner`, `sess_orchestrator`, `task_plan-named-ids`, `task_impl-named-ids`).

### TUI display (PLAN task T2, display only)

- New `rowID(kind, id)` helper: IDs of the four first-class kinds (`workspace`, `agent`,
  `session`, `task`) are returned untruncated; Run IDs and other ULID kinds keep the
  compact `shortID` form.
- Workspace, agent, session, and task rows now show their full slug ID; handoff task
  references and the runtime pane owner (a logical Session ID) use `rowID` too.
- Run IDs (`shortID(session.CurrentRunID)`, `shortID(handoff…)`, activity rows, forms)
  keep the compact form.

### Documentation (PLAN §2.9)

- `ARCHITECTURE.md`: the identifier contract now documents the `<prefix>_<slug>` format,
  the 1–48 character slug grammar and reserved shapes, `--id`/`invalid_id`/`id_exists`,
  per-storage-root uniqueness, the never-released `.runtime/ids/<kind>/<slug>` ledger,
  and that legacy IDs are not migrated, rewritten, or aliased. The persisted-state tree
  shows `ws_<slug>` (or legacy `ws_<ULID>`) and the storage-root ledger.
- `README.md`: a new ID-format note after the workspace-creation section, plus `--id`
  examples for `workspace create`, `task create`, `agent create`, and `session start`.
- `docs/operations.md`: `--id` is part of the operation payload, a keyed retry returns
  the same allocated ID, and `invalid_id`/`id_exists` fail with a non-zero exit.
- `docs/runtime.md`: the workspace tmux session name is `workspace-<workspace-id>` for
  both slug and legacy IDs.
- `PRODUCT.md` was left unchanged: its product contract does not list ID shapes.
  `TODO.md` was left unchanged (the plan makes the Q1/Q4 follow-ups optional).

## Tests added

- `internal/cli/named_ids_test.go`:
  - auto slug `ws_named-ids`, explicit `--id custom` → `ws_custom`, duplicate → `id_exists`,
    invalid shape → `invalid_id`;
  - `agent create --id agent_planner-one` and duplicate `id_exists`;
  - `task create --id custom-task` → `task_custom-task` and duplicate `id_exists`;
  - `issue dispatch --id dispatched` → `ws_dispatched` and duplicate `id_exists`;
  - `session start` registers the `--id` flag;
  - `chooseWorkspace` exact-ID precedence over a same-looking title and legacy-ID and
    title resolution.
- `internal/tui/named_ids_test.go`: full slug IDs render untruncated in workspace, agent,
  session, and task rows; `rowID` still compacts Run IDs.

## Acceptance criteria

| # | Criterion | Status | Evidence |
|---|---|---|---|
| 1 | `workspace create --title 'Named IDs'` → `ws_named-ids`; `--id custom` → `ws_custom`; same for agent/task/session/issue dispatch; invalid/taken → `invalid_id`/`id_exists`, non-zero | Met (session flag registration + T1 core test; see Limitations) | `TestCLIExplicitWorkspaceAgentAndTaskIDs`, `TestCLIIssueDispatchExplicitID`, `TestCLISessionStartHasExplicitIDFlag`; T1 `TestExplicitTaskAndSessionIDs` |
| 2 | Selector flags accept slug and legacy IDs; `workspace open ws_x` never ambiguous with a title `ws_x` | Met | `TestChooseWorkspaceExactIDPrecedence`; T1 `TestLegacyWorkspaceCompatibility` |
| 3 | `workspace <cmd> --help` and `help.go` examples use slug-shaped IDs; help tests pass | Met | `TestHelp*`; `evidence-named-ids-cli-help.txt` |
| 4 | TUI renders untruncated slug IDs for the four kinds; existing tests pass or only ULID-truncation assertions change | Met | `TestSlugIDsRenderUntruncatedInRows`, `TestRowIDKeepsRunIDCompact`; full TUI suite |
| 5 | Docs describe format, uniqueness scope, no-reuse reservations, `--id`, legacy compatibility; no migration claim; relative links resolve | Met | `ARCHITECTURE.md`, `README.md`, `docs/operations.md`, `docs/runtime.md`; `evidence-named-ids-cli-doc-links.txt` (`checked=30 broken=0`) |
| 6 | `gofmt`, `go test ./...`, `go vet ./...` pass; tmux suite optional | Met | `evidence-named-ids-cli-gofmt.txt`, `-vet.txt`, `-go-test-all.txt` |

## Checks

Run from the worktree root. The agent shell exports `WORKSPACE_*` variables, so every
`go` command was run after unsetting all `WORKSPACE_*` variables (otherwise child-process
helper tests such as `TestWorkerProcess` trigger inside the main suite).

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l internal/ cmd/` | 0 | `evidence-named-ids-cli-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-named-ids-cli-vet.txt` |
| `go test ./... -count=1 -timeout 600s` | 0 | `evidence-named-ids-cli-go-test-all.txt` |
| `go test -race ./internal/cli ./internal/tui -run 'TestCLIExplicit\|TestCLIIssueDispatchExplicitID\|TestCLISessionStartHasExplicitIDFlag\|TestChooseWorkspaceExactIDPrecedence\|TestSlugIDsRenderUntruncatedInRows\|TestRowIDKeepsRunIDCompact\|TestHelp' -count=1 -v` | 0 | `evidence-named-ids-cli-targeted-race.txt` |
| `python3 work-products/check-doc-links.py` | 0 | `evidence-named-ids-cli-doc-links.txt` |
| `workspace create\|issue dispatch\|agent create\|task create\|session start --help` | 0 | `evidence-named-ids-cli-help.txt` |

The tmux suite was not run for T2: the change touches no runtime or tmux code and only
`internal/cli` and display-only `internal/tui` code. The plan makes it optional for T2.

## Limitations and risks

- **Live `session start --id` is not exercised through `Execute`.** `session start` goes
  through `StartSupervisedSession`, which starts the project supervisor and a real client
  and is therefore outside the unit-test boundary. The flag is registered and forwarded
  verbatim to `SessionOptions.ID`, whose behavior (including refusal when resuming an
  existing logical session) is covered by T1's `TestExplicitTaskAndSessionIDs`. A live
  tmux check remains in the optional suite.
- **`IssueDispatchOptions.ID` is a small core addition** required to carry `--id` through
  `issue dispatch`. It is `omitempty`, so dispatch receipts created before this change
  keep their digests.
- **TUI row subtitles gained the entity ID** for agent, session, and task rows, and the
  workspace and handoff task references now show the full ID. This is display-only; no
  action, selection, or navigation logic changed.
- Reservations are never released, so an unkeyed crashed create leaves a gap (`-2` without
  a visible `-1`). This is the accepted PLAN behavior and is now documented.

## Not in scope

No changes to `ID()`, Run IDs, the reservation ledger internals, schema versions, or tmux
naming. Q1 (machine-wide registry), Q3 (workspace-qualified agent/session bases), and Q4
(legacy slug aliases) remain open plan questions.
