# Roadmap / backlog

List of planned improvements. Items are ordered by priority.

## Planned

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

- [ ] **Releases, distribution, and upgrades** — simplify installing and upgrading `workspace`.
  - **Goal:** users can quickly install the tool and keep it up to date without manually building the binary.
  - **Installation:** prepare a one-liner in the README that downloads the appropriate release and installs it on the user's machine.
  - **Upgrade:** add a `workspace upgrade` command that downloads and installs a newer release.
  - **Release process:** automatically build and publish versioned packages/binaries for supported platforms; document supported systems and architectures.
  - **Completion criteria:** a new user can install `workspace` with the command from the README, and an existing user can upgrade through `workspace upgrade`, without manually replacing the binary.
