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
