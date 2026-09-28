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
