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
| `WORKSPACE_TMUX_TEST=1 env -u … go test -race ./internal/terminal/ ./internal/tui/ -count=1 -timeout 120s` | 1 | `evidence-tmux-term-tui.txt` |
| `python3 work-products/check-doc-links.py` | 0 | `evidence-doc-links.txt` |

The full `go test ./...` passes once the workspace worker environment variables
(`WORKSPACE_AGENT_ID`/`WORKSPACE_SESSION_ID`/`WORKSPACE_RUN_ID`) are unset; with
them set, the pre-existing `TestWorkerProcess` helper re-exec assumes the test
binary argv and fails. The tmux suite's only failure,
`TestNavigatorRealPTYAndClientSelection` (`pseudo-TTY client did not attach`), is
pre-existing and reproduces on base `aa72616` in this environment. See
`CHECKS.yaml` for the exact commands and the pre-existing note.

## Risks and deviations

- The TUI gate uses durable `DispatcherSummary.State == "running"`, not a live tmux
  observation (the plan's accepted trade-off). A pane that dies between refreshes
  may briefly advertise `g`; resolution fails closed with `pane_missing` and the
  standard notice.
- Reconcile is intentionally not offered for the dispatcher failure path because
  the page is project-scoped and `ReconcileWorkspace` is workspace-scoped. This
  matches the plan; no dedicated dispatcher stop notice was added.
- No install/build contract changed, so `scripts/check-install.py` was not run.
