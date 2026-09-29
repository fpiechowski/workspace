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
