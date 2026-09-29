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
