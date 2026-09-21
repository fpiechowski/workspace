# Roadmap / backlog

List of planned improvements. Items are ordered by priority.

## Planned

- [x] **CI build verification** — add a GitHub Actions pipeline that builds the project on pushes to `master` and on pull requests targeting `master`.
- [ ] **`workspace prime` command** — add a CLI command that provides the agent with current operational instructions.
- [ ] **TUI as a tmux sidebar** — consider one TUI instance serving all workspaces in a project; consider nested tmux, with one instance acting as the project selector and the other as the workspace selector.
- [x] **Server status in the TUI** — add an indicator for the supervisor process and related service status in the TUI (green/red dot).
- [ ] **Issues as a first-class entity** — add an Issues view at project level in the TUI; allow creating workspaces for an issue, treating the issue as their input, and show this relationship in the list view.
- [ ] **Concurrency monitoring statistics** — add monitoring for the number of agents and sessions running in parallel.
- [ ] **Refresh indicator in the TUI** — replace the current refresh notification with an icon indicating that a refresh is in progress.
- [ ] **`idle` session status** — extend session statuses with `idle` and detect it when the agent is not doing work.

- [x] **User TUI** — add a terminal interface based on Bubble Tea and Bubbles.
  - **Goal:** make it easier for a person to inspect state and operate the project. The existing CLI exposes YAML/JSON, which works well for agents and scripts but is less convenient for everyday human use.
  - **Initial scope:** a readable overview of workspaces and their tasks/sessions, a detail view for the selected item, and the ability to run the most common existing operations from the keyboard.
  - **Completion criteria:** the TUI supports keyboard navigation and terminal resizing, presents states and errors clearly, and shares operation logic with the existing CLI. Existing commands and YAML/JSON output remain available to agents and automation.

- [x] **Releases, distribution, and upgrades** — simplify installing and upgrading `workspace`.
  - **Goal:** users can install the tool from GitHub Releases and keep it up to date without manually building or replacing the binary.
  - **Installation contract:** the README one-liner installs the exact archive selected from `checksums.txt` into `$HOME/.local/bin` or `WORKSPACE_INSTALL_DIR`; it supports Linux/macOS amd64/arm64 and directs Windows users to WSL.
  - **Upgrade contract:** `workspace upgrade` works outside a project, accepts only stable `vMAJOR.MINOR.PATCH` releases, never downgrades, verifies the checksum and safe single-file archive, and atomically replaces the resolved executable without `sudo`, `PATH` changes, or runtime-session restarts.
  - **Release contract:** a pinned official-actions workflow validates tags on `master`, builds `workspace_VERSION_{linux_amd64,linux_arm64,darwin_amd64,darwin_arm64}.tar.gz` with embedded metadata, writes `checksums.txt`, and publishes only a complete draft-then-published GitHub Release.
  - **Completion criteria:** the public v0.1.0 repository/release exists, all four assets and checksums validate, the documented installer works in a fresh directory, and `workspace upgrade` passes its no-op verification.
