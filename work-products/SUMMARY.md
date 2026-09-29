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

