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
