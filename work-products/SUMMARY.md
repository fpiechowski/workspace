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
