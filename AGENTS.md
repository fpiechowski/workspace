# Agent Guidelines

The current user request defines the purpose and scope of changes. This file describes working and verification rules.

## Documentation Map

- [README.md](README.md) — read this when changing installation, configuration, the public CLI, or user instructions.
- [PRODUCT.md](PRODUCT.md) — read this when making decisions about scope, product behavior, workflow, or user experience.
- [ARCHITECTURE.md](ARCHITECTURE.md) — read this before changing the domain model, persistence, concurrency, runtime, adapters, or module boundaries.
- [TODO.md](TODO.md) — read this when planning new work; it contains the backlog but does not expand the scope of the current request.
- [docs/clients.md](docs/clients.md) — read this when changing agent adapters, native resume, or message delivery.
- [docs/runtime.md](docs/runtime.md) — read this when changing Session/Run, tmux, the supervisor, services, recovery, or cleanup.
- [docs/operations.md](docs/operations.md) — read this when changing mutations, idempotency, receipts, locks, or external effects.
- [docs/revisions.md](docs/revisions.md) — read this when changing input, workflow migration, or result invalidation.
- [docs/checks.md](docs/checks.md) — read this when changing test-evidence capture or acceptance.
- [docs/trackers.md](docs/trackers.md) — read this when changing issue fetching or snapshotting.

Documents should describe the current contract, not a one-time implementation state. When behavior changes, update the parent document and only the detailed references whose contract actually changed.

## Planning and Scope

- Before editing, check the repository state and read the relevant files and tests. Preserve existing changes; do not reset or overwrite the user's work.
- [TODO.md](TODO.md) contains the backlog. Read it when planning new work; implement items only when they are covered by the current request. Update the backlog according to agreements with the user.
- Keep changes limited to the requested goal and follow existing conventions. Avoid unrelated refactors and new dependencies without a clear justification.
- Treat untracked and ignored files as potential user data. Do not remove or replace them; store temporary files and build outputs outside the repository when possible.
- Do not publish changes or perform operations on external services as part of ordinary verification. Do so only when explicitly included in the request.
- When a requirement affects scope or compatibility and cannot be resolved from the code and tests, clearly document the assumption or limitation you adopted.

## Tools and Verification

- Go 1.24 or newer is required (see `go.mod`). Format changed Go files with `gofmt`.
- Run the relevant package tests while working, and run `go test ./...` and `go vet ./...` before finishing Go changes.
- The full tmux test suite requires Linux or WSL with tmux installed. Run it when a change affects processes, concurrency, or tmux integration:

  ```sh
  WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s
  ```

- For installation or build changes, run the installation check from `README.md`: build the binary at a new temporary location and pass its path to `python3 scripts/check-install.py <binary-path>`.
- Tests should use existing mocks and fixtures. Do not connect them to real services or accounts or publish changes.
- Match verification to the scope: for documentation changes, check the content and links; for code, report the tests that ran and those that could not be run.

## Git in the Sandbox

If Git reports `dubious ownership`, limit the exception to a single command, for example `git -c safe.directory=<repo-directory> status --short`. Do not change the global Git configuration for this purpose.
