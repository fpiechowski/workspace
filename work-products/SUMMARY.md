# Integrated implementation summaries (integration worktree, T4 merge)

The shared `work-products/SUMMARY.md` path is rewritten in full by each task
lineage, so every merge of this path conflicts. Per the repository convention,
both lineages are preserved verbatim below.

---

## Lineage A - integration base summary (unchanged)

# Integrated implementation summaries (integration worktree, T3 merge)

The shared `work-products/SUMMARY.md` path is rewritten in full by each task
lineage, so every merge of this path conflicts. Per the repository convention,
both lineages are preserved verbatim below.

---

## Lineage A - integration base summary (unchanged)

# Integrated implementation summaries (integration worktree, T5 merge)

The shared `work-products/SUMMARY.md` path is rewritten in full by each task
lineage, so every merge of this path conflicts. Per the repository convention,
both lineages are preserved verbatim below.

---

## Lineage A - integration base summary (unchanged)

# Integrated implementation summaries (integration worktree, T2 merge)

The shared `work-products/SUMMARY.md` path is rewritten in full by each task
lineage, so every merge of this path conflicts. Per the repository convention,
both lineages are preserved verbatim below.

---

## Lineage A - integration base summary (unchanged)

# Integrated implementation summaries (integration worktree, T1 merge)

The shared `work-products/SUMMARY.md` path is rewritten in full by each task
lineage, so every merge of this path conflicts. Per the repository convention,
both lineages are preserved verbatim below.

---

## Lineage A - integration base summary (unchanged)

# Integration summary — task_01M3P3TAPQBP1PPEKNZVX6JWSF

## What was integrated

The single accepted implementation head
`c8b3bfd4f730f5f52eaeade4ffdec2edb7ef2670` (`ccbd752` product + tests + docs,
`c8b3bfd` evidence-only) for `task_01M3P2JFEFSWE8CD2VHS5PXBQW` — tolerate the
orchestrator's own running session when completing a workspace.

## How

- Integration was re-prepared on the new `master` tip `7cb2b8f` after the first
  attempt could not land (`target_moved`: `master` advanced `3031ccd -> 7cb2b8f`
  while other workspaces landed). Worktree `wt_01M3P76J97AJ71QNSTH7P2JSN4`.
- `git merge --no-ff c8b3bfd4f730f5f52eaeade4ffdec2edb7ef2670` from that
  worktree; because the base moved this is a real three-way merge (merge base
  `3031ccd`).
- Merge HEAD: `aaed27c35ec12c60e890dde1826e0bc4bdbe334d`, parents
  `7cb2b8f` and `c8b3bfd`. Verified `c8b3bfd` (and `ccbd752`) are ancestors of
  the integration HEAD.
- **Two textual conflicts**, both artifact-only shared report paths
  (`work-products/IMPLEMENTATION.md`, `work-products/SUMMARY.md`); both resolved
  by preserving the base and incoming lineages verbatim. `ARCHITECTURE.md` and
  `docs/runtime.md` auto-merged (distinct regions, both intents kept) with no
  semantic conflict. Full detail in `INTEGRATION.md`.

## Checks

Captured as check receipts (`workspace check run`) at the integration reporting
HEAD with a clean tree and the `WORKSPACE_*` actor variables cleared; all exit 0:

- `gofmt -l ./cmd ./internal` — empty.
- `go vet ./...`.
- `go test ./internal/core -run 'Complet|Landing|Land|Reopen|Autonomy|Archive' -count=1` — ok.
- `go test ./... -count=1` — all packages ok.
- `python3 work-products/check-doc-links.py` — `checked=30 broken=0`.

## Result

Merged head is on the `workspace/ws_01M3P1N1MBMTHGFQM8HSN1SMCE/integration-e42444070113`
branch and is ready for the user's integration-land decision. Nothing was landed
into `master`.

---

# Preserved report history

The reports below were preserved from the merge conflict resolution (base branch
lineage first, then the incoming implementation head lineage).

---

# Integration summary — task_01M3P2ZDWEZW9727N3NCK3CCH5 (round 3)

Merged both accepted heads onto `master` tip `3031ccd`, reconciled the
overlapping resume/recovery changes, verified, and committed. Not pushed or
landed.

- **Heads**: dispatcher jump `7d4af63` (task_01M3MZD82YXFS0G5HKV46G5JNQ) then
  recovery fix `3384ce3` (task_01M3P156JTWMYAT9TPBDMZ917B), each with
  `git merge --no-ff`.
- **Merge commits**: `d3eb5f1` (dispatcher jump), `4105c50` (recovery fix), plus
  integration-resolution `99d178f`.
- **Textual conflicts**: only shared artifact files (`docs/tui.md`,
  `work-products/CHECKS.yaml`, `work-products/IMPLEMENTATION.md`,
  `work-products/SUMMARY.md`). `docs/tui.md` kept master's single-client
  auto-jump wording and appended the dispatcher paragraph; the shared
  work-products files preserved both lineages verbatim. `session.go`
  auto-merged, keeping both the fix's Session-owned client definition and
  master's limit-aware resume / native-thread route affinity. Full detail in
  `INTEGRATION.md`.
- **Semantic conflict**: master's `3031ccd` whole-window recovery test configured
  the long-running client in the current project config, which the accepted fix
  overrides on resume. Per the reconciliation policy the fix's product behavior
  wins; the test now sets the Session client snapshot before resuming
  (`internal/core/tmux_integration_test.go`, commit `99d178f`). The test's
  recovery intent is unchanged and routing/reasoning reload remains covered by
  `TestResumeKeepsSessionClientSnapshotAndReloadsRoute` and
  `TestReasoningEffortReloadsOnResume`.

**Checks (all exit 0)**: `gofmt -l` clean; `go vet ./...`; `go test ./...`;
`WORKSPACE_TMUX_TEST=1 go test -race ./... -skip TestNavigatorRealPTYAndClientSelection`;
targeted `TestTmuxEndToEnd`, `TestDispatcherTmuxEndToEnd`, resume tests and the
core/terminal/tui dispatcher-navigation tests. Evidence in
`work-products/evidence-r3-*.txt`; manifest `work-products/CHECKS-r3.yaml`.
`TestNavigatorRealPTYAndClientSelection` is a pre-existing environmental failure,
excluded with `-skip`. Dispatcher jump behavior is intact and no existing jump
kind, orchestrator jump or single-client auto-jump regression was observed.

The reports merged below are preserved verbatim from the two head lineages; the
per-merge working-tree copies also remain in the merge commits.

---

# Integration summary — task_01M3MT65B4K2EF1HHXJS7CQK3P

Integrated the accepted autonomous-mode implementation I1–I5.

- Heads merged in recorded order: I1 `8d19bb7`, I2 `cf333fb`, I3 `58b5862`,
  I4 `5f19643`, I5 `1b8f24b`, on base `aa72616`.
- Strategy: `git merge --no-ff` for each head. I1–I4 are stacked; I5 branched
  from I3, so the final merge is a real three-way merge and the expected
  fast-forward to `1b8f24b` was not possible.
- Integrated merge HEAD: `84a5f5bbfb55b2504d7789310daf17cf7c5c48ed`, followed by
  a work-products evidence commit with no product-code change.
- Conflicts: only two shared work-products evidence files,
  `work-products/IMPLEMENTATION.md` and `work-products/SUMMARY.md`, both
  resolved losslessly by preserving the I4 and I5 reports. No product code
  conflicted and no product decision was needed.
- Checks on the integrated tree: `gofmt` clean, `go vet ./...` clean,
  `go test ./... -count=1 -timeout 570s` green (harness `WORKSPACE_*` cleared).
  Recorded in `work-products/CHECKS-INT.yaml`; the pre-existing known-red
  terminal pseudo-TTY race test was not run as acceptance.
- Not landed into `master`; that needs the user's explicit integration-land
  confirmation.

Full details: `work-products/INTEGRATION.md`.

---

# Integrated summaries (I4 and I5)

Both the I4 and I5 implementation tasks committed a summary to this shared root
path (`work-products/SUMMARY.md`). Merging I5 onto the I4 lineage conflicted on
the whole file; both summaries are preserved verbatim below.

---

# I4 — Orchestrator guidance and documentation

Status: implemented and committed (`c718ed1`).

## What changed

- **Binary-owned autonomous notice.** `autonomyRunNotice` (`internal/core/autonomy.go`)
  is appended to an orchestrator Run prompt while `autonomy.state=running`
  (`internal/core/session.go`). It carries the §3.3 judgment rules and the §4
  user-reserved boundary, so it survives stale or customized project templates.
- **Templates.** `orchestrator.AGENTS.md.tmpl` gains an `{{if .Autonomous}}`
  branch (interactive text becomes the `{{else}}`); the plan-first and manual
  `WORKFLOW.md.tmpl` / `prompts/orchestrator.md.tmpl` gain short autonomy notes.
  `Workspace.Autonomous()` and `promptData.Autonomous` back the switch, and
  `create --autonomous` now sets the record before templates are snapshotted.
- **Stale detection.** The previous stock plan-first workflow and orchestrator
  prompt digests are added to the stale lists in `internal/core/project.go`.
- **Skill.** The bundled `SKILL.md` (and its identical `.agents` copy) document
  `--autonomous`, the `decision_required` autonomous clause, and the
  `workspace autonomy enable|disable|report` commands. `workspace prime` prints
  them.
- **Docs.** `PRODUCT.md` (principle + scope), `README.md` (flag, commands,
  report), `ARCHITECTURE.md` (Domain Model + persisted state), `docs/operations.md`
  (mutations, `autonomy_excluded`, rationale receipts), `docs/runtime.md`
  (delivery precondition, run end).

## Verification

- gofmt clean, `go vet ./...` clean, `go test ./... -count=1` green.
- `go test ./internal/core -run 'Session|Skill|Template|Autonomy' -count=1` green.
- `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core -run
  'Session|Skill|Template|Autonomy' -timeout 120s` green (22.5s, no data race).
- Doc link check: `checked=29 broken=0`.
- New tests: `internal/core/autonomy_prompt_test.go`.

The exact ``WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s`` is red on
this machine for pre-existing reasons (terminal pseudo-TTY test fails at
unmodified HEAD `58b5862`; the core race binary exceeds 90s here). See
`work-products/IMPLEMENTATION.md` and `work-products/CHECKS-I4.yaml`.

---

# Summary — task_01M3MJYS3TAP3E9057R289DR1G (I5: TUI support)

Implemented PLAN.md §5-I5: the TUI surface for autonomous workspaces.

- The workspace header shows an autonomy badge (`● autonomous running`,
  `✓ autonomous delivered`, `○ autonomous disabled`); the state word survives
  `--no-color`.
- A delivered report is a Needs-attention row (and a More entry) that opens a
  report view with the outcome, recommendation, integration head, artifact IDs
  and pending commands.
- Decisions resolved under autonomy are marked `by orchestrator (autonomous)`
  and their detail exposes the subject, evidence and Run provenance.
- The Orchestrator, Runtime and Autonomy pages offer a guarded
  `Disable autonomous run`: a required reason, a confirmation that keeps the
  exact current revision and the `tui_<ULID>` key, dispatched to
  `Service.DisableAutonomy`. Disabled workspaces no longer offer it.
- The TUI does not offer `autonomy enable` on an existing workspace (A4) and
  never auto-accepts handoffs or decisions. The create-form `Autonomous run`
  toggle reaches `CreateOptions.Autonomous` for both the named workflow and the
  manual choice.
- `docs/tui.md` documents all of the above.

Commit `eca7ab5`. Checks (`work-products/CHECKS-I5.yaml`): `go test
./internal/tui -count=1`, gofmt, `go vet ./...` and the full `go test ./...`
all pass (full suite with the harness `WORKSPACE_*` variables cleared, matching
the I3 checks file).

---

# Summary — task_01M3MN4RE4J7NKEJCD45MTSH7S (implement single-client auto-jump)

Implemented the accepted plan (`art_01M3MMYVEGXFT2WCVEFFXECK1X`) on branch
`workspace/ws_01M3MJZRZ42N2F623G70K60M05/impl-single-client-jump`, base `aa72616`.

- **Behavior**: `g` now jumps immediately with the only attached tmux client, through the
  existing verified jump path, without opening the `navigation_client` picker and without
  an extra key press. Zero- and multi-client behavior is unchanged.
- **Code** (`internal/tui/navigation_flow.go`): a `len(clients)==1` branch after the
  zero-client check and the generation/target/ref guard calls `jumpClientCommand(...,
  recheckTarget=true, automatic=true)`; the guard uses single-client notice wording; the
  new `automatic` flag changes only the in-flight and `navigation_target_changed` texts;
  the last-used preference is loaded only when more than one client is attached while the
  successful-jump save is untouched.
- **Tests** (`internal/tui/navigation_flow_test.go`): extracted `discoverNavigationClients`
  and reused it in `openNavigationPicker`; six new single-client tests (no picker,
  target-changed, client-gone, preference save failure, stale discovery, Esc during the
  in-flight jump); existing one-client picker tests moved to two clients.
- **Docs**: README, PRODUCT, DESIGN, ARCHITECTURE, docs/runtime and docs/tui updated so no
  “always / even for one client” wording remains.
- **Checks**: `gofmt -l internal/tui` empty, focused TUI tests, `go vet ./...` and the
  TUI `-race` + `WORKSPACE_TMUX_TEST=1` run pass. `go test ./...` and the full race run
  still fail only on pre-existing, environment-related failures in `internal/cli`,
  `internal/core` and `internal/terminal` (reproduced on the base commit with the change
  stashed). Receipts in `work-products/checks/`; details in `IMPLEMENTATION.md`.
- **Commits**: `f6e666e` (code + tests), `d1bd846` (docs).
- **Deviation**: also updated `ARCHITECTURE.md` and `DESIGN.md`, which the plan omitted but
  which contained the same stale picker wording that the acceptance grep forbids.


---

# Incoming summary — task_01M3MZD82YXFS0G5HKV46G5JNQ (dispatcher jump)

Preserved verbatim from the accepted dispatcher-jump head `7d4af63`.

---

# Summary — task_01M3MZD82YXFS0G5HKV46G5JNQ

Implemented the accepted plan for TUI jump (`g`) support on the project Dispatcher
page. Keyboard-only, matching the orchestrator detail page; `workspace dispatcher
attach` is unchanged.

- **Core** (`internal/core/navigation.go`): `NavigationTarget` gains `ProjectID`;
  `ResolveNavigationTarget` handles `Kind=dispatcher` by resolving the durable
  current Run from `DispatcherStatus` and the verified project topology pane,
  enforcing the `tickDispatcher` ownership gate and returning `pane_missing`
  otherwise (not-started / no live run / unverified / mismatch). Never a scan.
- **Terminal** (`internal/terminal/navigation.go`): `TmuxNavigator.verify` accepts
  a project-scoped dispatcher target (canonical
  `workspace-dispatcher-<project-id>` session plus project/kind/session/run pane
  metadata); `ListClients`/`Select`/`Attach`/`Jump` work through the existing
  `=<session>` paths. Workspace-kind verification is unchanged.
- **TUI** (`keymap.go`, `navigation_flow.go`, `detail.go`): `jumpCapable` includes
  `dispatcher`; `dispatcherJumpReady()` gates `contextFlags().jump`; the footer,
  full help and page hint advertise `g jump` only while the Dispatcher is running,
  and `g` is inert otherwise. `jumpSelected` dispatches `EntityRef{Kind:
  dispatcher}` through the unchanged picker/preference/recheck flow. No
  actions-menu entry.
- **Docs**: `docs/tui.md`, `PRODUCT.md`, `ARCHITECTURE.md` describe the
  jump-capable Dispatcher, the hidden-when-idle rule, and the unchanged CLI attach.
- **Tests**: core resolution cases, terminal accept/reject + ListClients/Jump,
  and TUI success / no-live / changed-target / detached-client flows, plus keymap
  and project-scope coverage.

Commit `bf5a3e92780bd60195261dbcc95c4a19a2a77253`. Checks: `gofmt` clean,
`go vet ./...` passes, `go test ./...` passes (with worker env vars unset),
targeted tmux core navigation/dispatcher suite passes; the only tmux failure
(`TestNavigatorRealPTYAndClientSelection`) is pre-existing and reproduces on the
base commit.


---

# Incoming summary — task_01M3P156JTWMYAT9TPBDMZ917B (recovery fix)

Preserved verbatim from the accepted recovery-fix head `3384ce3`.

---

# Summary — fix supervisor orchestrator recovery regression from 2a72e3a

Task: `task_01M3P156JTWMYAT9TPBDMZ917B` · Base `2a72e3a` · Fix commit `5b75caa`

## Root cause

`2a72e3a` removed the resume block that carried the logical Session's client
definition onto the resumed Run. Recovery is the normal resume path
(`supervisor.go:393-401` → `ResumeAgent` → `StartSession`), so after a whole-window
loss the orchestrator was relaunched with the current config client. In
`TestTmuxEndToEnd` the orchestrator's snapshot was deliberately the long-running
`sh -c sleep 30` (`tmux_integration_test.go:193-203`); the resumed Run used the
configured one-shot helper instead, exited immediately, and was already `exited`
(not `interrupted`) when its window was killed, so the supervisor had nothing to
recover. The test failed at `tmux_integration_test.go:343`.

## Fix

`internal/core/session.go:316-334`: on resume, restore the Session's client
definition into the local routing config before `chooseRoute`, while still
reloading profile, route, limits and reasoning effort from current project
configuration. The `2a72e3a`-removed route pinning and prior-effort override are
not restored.

## Deviation from 2a72e3a

The resumed Run no longer reloads the client *definition* (it keeps the Session
snapshot) but still reloads routing/reasoning. Rationale: recovery reuses resume
and needs a resolvable, stable client. Proven by
`TestReasoningEffortReloadsOnResume` (routing reload), the new
`TestResumeKeepsSessionClientSnapshotAndReloadsRoute` (client snapshot kept), and
`TestTmuxEndToEnd` (recovery works). Documented in `IMPLEMENTATION.md` and in the
updated `ARCHITECTURE.md` / `docs/runtime.md` wording.

## Checks

- `gofmt -l`: clean · `go vet ./...`: clean
- `go test ./...`: pass (harness `WORKSPACE_*` env cleared)
- `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -run TestTmuxEndToEnd`:
  pass (was failing on `2a72e3a`)
- `WORKSPACE_TMUX_TEST=1 go test -race ./... -skip TestNavigatorRealPTYAndClientSelection`:
  pass; that skipped terminal test is a pre-existing environmental failure
  (unchanged package vs base)
- Regression test fails on base (`2a72e3a`) and passes with the fix

Evidence: `work-products/evidence-fix-recovery-*.txt`,
`work-products/CHECKS-fix-recovery.yaml`.

---

# Incoming implementation summary — completion-exemption (`task_01M3P2JFEFSWE8CD2VHS5PXBQW`)

Preserved verbatim from the accepted implementation head `c8b3bfd4f730f5f52eaeade4ffdec2edb7ef2670`; the shared `work-products/SUMMARY.md` path was rewritten by both lineages.

---

# Summary — implement completion tolerance for the orchestrator's own Session

**Task:** `task_01M3P2JFEFSWE8CD2VHS5PXBQW` — implement
`artifacts/art_01M3P2A2Q8TC0RF38KEH0SATFN/PLAN.md`.

**Commit:** `ccbd7525b4256a3977cd3bcfcf358c298af9e533` (`feat: tolerate the orchestrator's own session when completing`) on base `3031ccd`.

**What changed:** `completionRuntimeQuiet` (`internal/core/completion.go`) now skips active
Sessions that belong to the orchestrator (`AgentID == d.State.OrchestratorAgentID ||
AgentSnapshot.Role == "orchestrator"`), matching the existing `reopen` predicate. Worker
Sessions and services still fail with `session_active` / `service_active`; the message now
reads "stop worker session %s before completing". The orchestrator's own Session is
tolerated, not stopped or rewritten, and `archive` still requires every Session to stop.

**Tests:** fixed `TestManualCompletionRequiresUserAttestation` to expect success with the
orchestrator Session still active; added `TestManualCompletionToleratesOrchestratorButNotWorkers`
(worker blocks and is named; archive refuses until the orchestrator stops), 
`TestManualCompletionUserActorToleratesOrchestrator` (empty actor), and
`TestLandCompleteToleratesOrchestratorButNotWorkers` (landed plan-first path). Autonomy tests
were left unchanged.

**Docs:** updated `docs/runtime.md`, `ARCHITECTURE.md` (Manual Mode), `README.md` and
`docs/operations.md` to distinguish `complete` (worker Sessions/services) from `archive`
(all Sessions/services).

**Checks (see `work-products/CHECKS-completion.yaml`):** `gofmt -l ./cmd ./internal` clean,
`go vet ./...` clean, focused `go test ./internal/core -run 'Complet|Landing|Land|Reopen|Autonomy|Archive' -count=1`
green, `go test ./... -count=1` green with the sandbox `WORKSPACE_*` actor variables cleared,
doc-link check `checked=30 broken=0`.

**Caveat:** the sandbox exports orchestrator/worker `WORKSPACE_*` variables that break
`TestWorkerProcess` and four CLI tests on the *unmodified base* as well; the full suite must
be run with them unset. No product-code blocker.


---

# Incoming implementation summary — auto-close accepted worker sessions (`task_01M3P785FNW7WPVAQ0NJKVHCX0`)

Preserved verbatim from the accepted implementation head `415b97cb6919fbb1057c764c323ddfafde57c18f`; the shared `work-products/SUMMARY.md` path was rewritten by both lineages.

---

# Summary: Auto-close completed worker Sessions

Task task_01M3P785FNW7WPVAQ0NJKVHCX0, plan art_01M3P2E8D7AHJE7MHE36Q5CQPK.
Commit 248ae63 `feat: auto-close accepted worker sessions` on base 3031ccd.

**Change.** `handoff accept` now records a durable `close_requested_at` intent on
the submitting worker Session atomically with the acceptance. After the commit a
best-effort settle verifies pane ownership, stops the owned Run, and closes the
Session. Settle failures are stored in `close_error` and retried by `Reconcile`,
the supervisor tick and the next `session start`, so the acceptance never fails on
the runtime and an unowned pane is never stopped. A new `handoff accept
--keep-session` flag preserves consultation resume; rejected handoffs,
orchestrator Sessions and manual closes are unaffected.

**Files.** `internal/core/{model,session,handoff}.go`, `internal/cli/{work,help}.go`,
`internal/core/session_autoclose_test.go`, CLI/autonomy test call-site updates,
and docs (`docs/runtime.md`, `README.md`, `PRODUCT.md`, `ARCHITECTURE.md`). No new
dependencies; stock prompt templates untouched.

**Verification.** `gofmt` clean, `go vet ./...` clean, `go test ./...` passes,
`go test -race ./...` passes. The opt-in tmux race suite
(`WORKSPACE_TMUX_TEST=1`) fails on tmux 3.7c with `invalid tmux pane: "%0_@0"`;
the same failure reproduces on the clean base commit, so it is a pre-existing
environment incompatibility, not caused by this change. Full evidence is in
`work-products/IMPLEMENTATION.md` and the `evidence-autoclose-*.txt` files.


---

## Lineage B - incoming T1 `impl-core` summary (d0bb317)

Implemented T1 — the core Issue-evaluation domain for linked Workspaces at commit
`87bc342` (base `3031ccd`). No launch, CLI or menu changes (T3).

- Split `UpdateIssue` into the public wrapper plus a lock-free `updateIssueLocked`
  helper so an Issue and a Workspace can be written in one project-lock critical
  section; public behavior and existing tests are unchanged.
- Added `Workspace.IssueEvaluation` / `IssueEvaluation` (`issue_evaluation`). Both
  completion paths (`completeManual`, `completeLanding`) record a fresh `pending`
  evaluation with a new `ieval_...` ID in the same revision when `input.issue_id`
  is set, and nothing for an unlinked Workspace.
- Added `RecordIssueEvaluation` (`internal/core/issue_evaluation.go`): every row of
  the outcomes table, the stale-digest `issue_revised` refusal for delivered,
  outcome/reason validation, the not-completed/unlinked/missing/archived/revision
  conflicts, orchestrator-or-user authorization (conversation-only Run included),
  idempotent receipt replay, and a crash-convergent Issue+Workspace write reusing
  `updateIssueLocked` with the stable `issue-evaluation:<ws>:<ieval>` key.
- `ReopenWorkspace` clears the evaluation; the next completion gets a new pending
  ID, and a later not_delivered reopens an Issue this Workspace had closed.

Verification: `gofmt -l ./cmd ./internal` clean, `go vet ./...` and
`go test ./... -count=1 -timeout 570s` pass, plus targeted new core tests. Evidence
files and `CHECKS-T1-EVAL.yaml` are in `work-products/`.


---

## Lineage B - incoming T2 `impl-prompts` summary (c5c78da)

# SUMMARY — T2: Orchestrator evaluation notice in prompts, templates and skill

Task `task_01M3P9E7ZSW0572P8QEP1A7BWH` (plan §4.3) is implemented in worktree
`impl-prompts` at commit `7dbd583`.

## What changed
- `internal/core/session.go`: a conversation-only orchestrator Run for a completed
  Workspace with a `pending` Issue evaluation now gets an **Issue evaluation notice**
  after the existing conversation-only notice. It carries the Issue ID, the frozen
  `inputs/issue.md` path, the evidence sources, the exact
  `workspace issue-evaluation record ... --operation-key issue-evaluation:<ieval id>`
  command and the statement that Issue content is untrusted and does not widen
  authority. No pending evaluation means no added bytes.
- Orchestrator AGENTS template, both WORKFLOW templates and both orchestrator prompt
  templates describe the evaluation step.
- `internal/core/skill/workspace/SKILL.md` and the tracked `.agents/skills/workspace/
  SKILL.md` copy were extended identically (byte-identical `diff`).
- New `internal/core/issue_evaluation_prompt_test.go` covers the notice content and the
  no-pending gating.

## Checks (all exit 0)
- `gofmt -l ./cmd ./internal` — clean
- `go vet ./...`
- targeted core tests (new prompt tests, skill identity, autonomy notices, reopen cycle)
- `go test ./... -count=1 -timeout 570s`

Full command/evidence list: `work-products/CHECKS-T2-PROMPTS.yaml`.

## Notes / risks
- Instruction text only; the `workspace issue-evaluation record` command it documents is
  delivered by T3. No runtime dependency was introduced.
- `.workspace/templates/**` project overrides were left untouched (out of scope: they
  are the project's pinned runtime config; the task scopes `internal/core/templates`).
- The `--expected-revision` placeholder is resolved by the orchestrator from
  `workspace status --json`; the operation remains revision-guarded.


---

## Lineage B - incoming T5 `impl-tui` summary (f737a52)

# SUMMARY — T5: Evaluation state in project overview link and TUI Issue detail

Task `task_01M3P9E8MAVA0KN5WP767JJ5FB` implemented on top of T1 in the `impl-tui`
worktree. Commit `9958a07` (base `d0bb317`).

## What changed
- `IssueWorkspaceLink` and `WorkspaceSummary` now carry `EvaluationState` and
  `EvaluationOutcome`, populated from the Workspace `issue_evaluation`.
- `ProjectOverview` fills the workspace row and the per-Issue linked-workspace entries.
- The TUI Issue detail renders `· evaluation <state>` (and `(<outcome>)` when recorded)
  on each linked Workspace bullet; an un-evaluated Workspace is unchanged.

## Verification
- `gofmt -l ./cmd ./internal`: clean.
- `go vet ./...`: pass.
- Targeted new tests (`TestReadModelsCarryLinkedEvaluationState`,
  `TestIssueDetailShowsLinkedWorkspaceEvaluation`): pass.
- `go test ./... -count=1 -timeout 570s`: pass.

See `IMPLEMENTATION.md` and `CHECKS-T5-EVAL.yaml`; evidence files:
`evidence-t5-eval-gofmt.txt`, `evidence-t5-eval-vet.txt`,
`evidence-t5-eval-targeted.txt`, `evidence-t5-eval-go-test-all.txt`.


---

## Lineage B - incoming T3 `impl-launch` summary (5d7d489)

# SUMMARY — T3: automatic Issue-evaluation launch, CLI command and menu

Task: `task_01M3P9EKSWW97RC87SW2EA875V` · Worktree `impl-launch` · Base `3031ccd`
· Commit `046b33b`

## What shipped
- Completing a linked Workspace now best-effort starts a conversation-only
  orchestrator Run (`issue-evaluation:<ieval id>`) after the commit, without ever
  rolling back the completion. If the orchestrator is already active the launch is
  a no-op, so completion replays create no second Run.
- A failed launch records `issue_evaluation.launch_error` and the completed menu
  offers the evaluation action plus a launch-aware conversation label.
- `CompleteOptions.NoEvaluationLaunch` is exposed as `--no-issue-evaluation-start`.
- New `workspace issue-evaluation record` group/subcommand with `--outcome`,
  `--reason`, `--expected-revision` and the global `--operation-key`.
- `menu` shows `issue_evaluation` for pending evaluations in both completed modes.
- Tests: core launch/no-launch/failure, CLI record-vs-status JSON and replay, TUI
  `complete_workspace` through `tui.Backend`, and an opt-in tmux integration case.

## Acceptance
All six acceptance criteria are met and covered by the new tests. The CLI record
JSON `data` is DeepEqual to `workspace status --json` and replays under the same
key. The TUI path launches through `CoreBackend`.

## Checks
- `gofmt -l ./cmd ./internal` — clean.
- `go vet ./...` — exit 0.
- targeted core+CLI tests and `go test ./... -count=1 -timeout 570s` — exit 0.
- `WORKSPACE_TMUX_TEST=1` isolated new tmux case — exit 0; `-race` core+tui with a
  realistic timeout — exit 0.

## Caveat (environment, not a regression)
`WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s` cannot pass on this WSL
`/mnt/c` sandbox: the `core` package alone needs 153 s at base `c5c78da` and 185 s
with T3 (so the 90 s cap fires), and
`internal/terminal.TestNavigatorRealPTYAndClientSelection` fails at base too. Both
are reproduced on the untouched base commit; the T3 tmux case and the full core
suite pass with an extended timeout. Evidence files record the comparison.

## Risks
- Launch remains best effort: durable `pending` + `launch_error` + `workspace start`
  are the recovery path.
- No T4 documentation or optional T5 TUI Issue-detail change was made.


---

## Lineage B - incoming T4 `impl-docs` summary (c683071)

# SUMMARY — T4: documentation contract for automatic Issue evaluation

Task: `task_01M3P9EWCRRSBEK0XH8F9Y83D9` · Worktree `impl-docs` · Base `5d7d489`
· Commit `e2c28da`

## What shipped
The documentation contract for the automatic linked-Issue evaluation (T1–T3),
with no production-code change:

- `docs/trackers.md` — new "Automatic Issue evaluation on Workspace completion"
  section (trigger, actor, outcomes table, stale-digest guard, no-tracker-write).
- `ARCHITECTURE.md` — narrow `authorizeProjectActor` exception for the completed
  Workspace's own orchestrator and the `issue_evaluation` Workspace state.
- `PRODUCT.md` — scope bullet and Issue-section paragraph.
- `README.md` — `issue-evaluation record` reference and
  `complete --no-issue-evaluation-start`, plus a corrected Issue-authority
  sentence.
- `docs/operations.md` — post-commit launch key and the two-file, idempotent,
  crash-convergent Issue/Workspace write contract.
- `docs/runtime.md` — the conversation-only Run exception for
  `issue-evaluation record`.
- `internal/core/templates/dispatcher.AGENTS.md.tmpl` — the Dispatcher must not
  close an Issue whose linked Workspace has a pending evaluation.
- `TODO.md` — checked item for the shipped feature.

## Acceptance
All four task acceptance criteria are met: every listed document describes the
implemented contract, the docs state that the external tracker is never written
and describe the outcomes behavior and stale-digest guard, the repo doc link
check passes (`checked=34 broken=0`), and `go build ./...` passes with the work
committed in the task worktree.

## Checks
- `python3 work-products/check-doc-links.py` — exit 0, `checked=34 broken=0`.
- `go build ./...` — exit 0.
- `go vet ./...` — exit 0.
- `go test ./internal/core/ -run 'TestSkill|TestPrime|TestDispatcher|TestHelp'`
  — exit 0.

## Risks
- Docs only; behavior is unchanged and owned by T1–T3.
- The `issue_evaluation` field keeps the same strict-YAML downgrade trade-off as
  `autonomy` for a linked Workspace completed by this binary; documented and
  `omitempty` for unlinked Workspaces.
