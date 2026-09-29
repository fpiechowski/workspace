# workspace

CLI for managing persistent task context, agents, Git worktrees, and tmux.
The orchestrator implements the `plan-first` workflow: planning in a separate worktree,
delegating implementation to subsequent worktrees, integrating the accepted results in a
dedicated worktree, and — after explicit user approval — landing the integration by
fast-forwarding the target branch. A workspace can also run manually without a workflow,
using explicit tasks, worktrees, handoffs, and checks.

- **Agent**: a persona with a role, instructions, prompt template, and model profile.
- **Session**: persistent logical conversation context for a given agent/task/worktree lineage.
- **Run**: one concrete client and tmux-pane execution; stores the model, argv, prompt,
  process result, and exact operation provenance.
- **Workspace**: WORKSPACE.md, AGENTS.md, WORKFLOW.md, tasks, artifacts, and worktrees.
- **Issue**: a durable project input with revisioned snapshots, local status, source,
  digest, history, and links to Workspaces created from an exact revision.
- **Dispatcher**: a project-scoped Agent that may intake and route Issues, create linked
  Workspaces, and explicitly start their orchestrators; it cannot implement code.

The product vision and boundaries are described in [PRODUCT.md](PRODUCT.md), while the
technical model and reference map are in [ARCHITECTURE.md](ARCHITECTURE.md).

## Installation

Published releases are the canonical installation and upgrade source. The installer
requires only `curl`, `tar`, `awk`, `mktemp`, and either `sha256sum` or `shasum`.
Go is not required for an installed release.

```sh
curl -fsSL https://raw.githubusercontent.com/fpiechowski/workspace/master/scripts/install.sh | sh
```

By default the installer places the executable at `$HOME/.local/bin/workspace` and
prints a `PATH` hint when that directory is not already on `PATH`. Choose another
directory without root access with `WORKSPACE_INSTALL_DIR`:

```sh
curl -fsSL https://raw.githubusercontent.com/fpiechowski/workspace/master/scripts/install.sh \
  | WORKSPACE_INSTALL_DIR="$HOME/bin" sh
```

Release archives are published for `linux/amd64`, `linux/arm64`, `darwin/amd64`, and
`darwin/arm64`. Native Windows binaries are not published: Windows users run the
Linux amd64 or arm64 release inside WSL. Native Windows remains supported for the
portable core's build and tests only; tmux sessions require Linux/macOS or WSL.

The installed binary reports its embedded release metadata without reading project
state:

```sh
workspace version
workspace version --json
```

The binary also carries the current general agent guidance used by the workspace skill.
Refresh it at the start of a new session or after context compaction with:

```sh
workspace prime
workspace prime --json
```

The normal form prints the embedded Markdown directly with a trailing newline. The JSON
form uses the standard `{ok:true,data:{instructions:"..."}}` success envelope and puts
the same Markdown in `data.instructions`. `prime` works from any directory and is
read-only: it does not inspect or create project/workspace state. Use
`workspace status` and `workspace menu` when you need live state or available actions.

Upgrade checks the latest published, non-draft, non-prerelease GitHub Release for the
current target, verifies the exact archive's SHA-256 entry, validates the archive
layout, and atomically replaces the executable in its existing directory:

```sh
workspace upgrade
```

An equal or newer stable version is a successful `updated: false` no-op; a `dev`
source build may upgrade to the latest stable release. The updater never downgrades,
uses `sudo`, changes `PATH`, or mutates a project/workspace. Already-running
supervisors and tmux processes keep their old in-memory code until restarted.
Each release contains one root `workspace` executable per supported target and a
`checksums.txt` SHA-256 manifest.

### Development builds from a checkout

Release installation and local development setup are separate workflows. Use the
published installer and `workspace upgrade` for stable releases; use the platform
setup script when you need the current checkout. Development setup requires Go 1.24
or newer. Native Windows builds the portable CLI as `workspace.exe`; runtime features
that depend on tmux still require WSL or another supported Unix environment.

In native Windows PowerShell, from the checkout:

```powershell
.\scripts\setup-dev.ps1
```

Or invoke it by absolute path from any directory:

```powershell
& 'C:\path\to\workspace\scripts\setup-dev.ps1'
```

The default artifact is `bin/workspace.exe` inside the checkout. The setup creates
`%USERPROFILE%\bin\workspace.cmd`, a command shim that invokes that artifact, and adds
`%USERPROFILE%\bin` to the user PATH if needed. Open a new terminal after the first
setup, then check `Get-Command workspace` and `workspace version`. Rerun the setup
after source changes to replace the binary. It does not change PowerShell startup files.

In WSL, use the Linux build even when the checkout is on a mounted Windows drive:

```sh
/mnt/c/Users/you/Documents/workspace/scripts/setup-dev.sh
```

This builds `bin/workspace` for Linux and links `$HOME/.local/bin/workspace` to it.
Check the selected command with `command -v workspace` and `workspace version`.

From Linux, macOS, or WSL, invoke the shell script through the checkout path; it locates
the repository from its own path:

```sh
/path/to/workspace/scripts/setup-dev.sh
```

From the checkout itself, the equivalent command is `./scripts/setup-dev.sh`. The
default artifact is `bin/workspace` inside that checkout. The command
`$HOME/.local/bin/workspace` is a symlink to it, so rerunning the same command after
source changes atomically refreshes the persistent binary without another shell
configuration change. Verify the selected command with:

```sh
command -v workspace
workspace version
```

If `$HOME/.local/bin` is not on `PATH`, the setup prints the exact `export` hint; apply
it in the current shell or add the directory to your shell startup file yourself. It
does not edit startup files. On Windows, both `WORKSPACE_DEV_BUILD_DIR` and
`WORKSPACE_DEV_INSTALL_DIR` may also be set to absolute or checkout-relative paths; by
default Windows uses `bin` and `%USERPROFILE%\bin`. On Linux/macOS/WSL, set these
variables for isolated tests; both may be absolute or relative to the checkout. These
development-only variables are intentionally distinct from the release installer's
`WORKSPACE_INSTALL_DIR`:

```sh
WORKSPACE_DEV_BUILD_DIR="/tmp/workspace-dev-bin" \
WORKSPACE_DEV_INSTALL_DIR="$HOME/.local/bin" \
/path/to/workspace/scripts/setup-dev.sh
```

PowerShell override example:

```powershell
$env:WORKSPACE_DEV_BUILD_DIR = 'C:\temp\workspace-dev-bin'
$env:WORKSPACE_DEV_INSTALL_DIR = 'C:\Users\you\bin'
& 'C:\path\to\workspace\scripts\setup-dev.ps1'
```

The setup refuses to replace an unrelated existing regular file or symlink named
`workspace`; choose another development install directory instead. It has no force
mode. To return to a release installation, run the release installer again (using
`WORKSPACE_INSTALL_DIR` if the release belongs in another directory). Do not use
`workspace upgrade` as the development rebuild command: upgrade intentionally follows
symlinks and can replace the resolved development artifact with a release binary.

Already-running supervisors and tmux processes retain their old in-memory code. Restart
them after a development rebuild to exercise runtime changes.

Maintainers create a stable release by pushing an annotated `vMAJOR.MINOR.PATCH` tag
whose commit is on `master`. The pinned GitHub Actions workflow tests the source,
builds the four archives with embedded version/commit/date metadata, verifies the
checksums, creates a draft release, uploads all five assets, and publishes it only
after the complete asset set is present.

Help is available at every command level. You can follow the path from the root command
or request help directly after a command:

```sh
workspace help
workspace help session start
workspace session help
workspace session start --help
workspace session start help
```

The final `help` word takes precedence over a regular argument. If an argument literally
has the value `help`, pass it after the `--` separator (for example, `workspace create -- help`);
write a flag value as `--option=help`.

The binary must remain available at this path because tmux starts it later as well.
Do not use `go run` to start sessions.

In a Git project with at least one commit:

```sh
workspace project init
workspace doctor --json
workspace skill install --client codex
```

The skill is installed in `.agents/skills/workspace` for Codex/OpenCode or
`.claude/skills/workspace` for Claude. An existing modified skill is not overwritten.

### Project initialization

`workspace project init` installs editable templates in `.workspace/templates` and
writes `.workspace/config.yaml`. On a fresh project that has no configuration yet and
an interactive terminal on both stdin and stdout, it runs a setup wizard:

- It names the target Git root and asks to start. `n`, `q`, `cancel`, EOF, or `Ctrl-C`
  cancels without writing anything.
- It reports which built-in `codex`, `claude`, and `opencode` executables are detected
  on `PATH`, then asks which client adapters to configure.
- It asks for the default orchestrator client, an account-specific provider and model,
  an optional reasoning effort (suggested `high`; `none` omits it), and a positive
  concurrency (suggested `3`).
- It optionally proposes the README `thinker`, `worker`, and `supervisor` profiles with
  the `plan-first` v2 workflow mapping (`orchestrator`, `planning`, `implementation`,
  `integration`), and an optional GitHub forge. When roles are declined, workflows are
  omitted and must be configured before a workflow that references unmapped roles can
  start. The removed `issue-resolution` workflow is never generated; a configuration that
  still lists it keeps loading, with that entry ignored.
- It prints a summary and asks `Write configuration? [y/N]`; only an explicit `y`
  validates the whole candidate with the same rules as a persisted config and writes it
  once under the project lock.

Provider and model identifiers must be account-specific values; the wizard rejects the
README `YOUR_PROVIDER`/`YOUR_*_MODEL` examples instead of persisting them. Generated
launch and resume argv use the built-in adapter defaults and contain **no**
approval-bypass flags.

Non-interactive and scripted runs never prompt. `--json`, `--short`,
`--non-interactive`, an `--operation-key`, or a redirected/non-terminal stream selects
script mode, which writes the minimal generated configuration and emits the normal
YAML/JSON envelope without blocking. Re-running `init` on a project that already has a
valid `.workspace/config.yaml` skips the wizard and preserves that configuration
byte-for-byte, including comments, while restoring missing templates and Git exclude
rules. An invalid existing configuration remains an error and is never overwritten.
Configuration and templates may be versioned; working data has local Git ignore rules.

## Configuration

Keep the generated `schema_version`, `project_id`, and `runtime`. Add clients and
profiles to `.workspace/config.yaml`. Replace model names with identifiers available in
your own account; profiles contain no built-in assumptions about pricing or subscription.
The example below shows complete client definitions for every supported adapter. Keep the
clients you use and replace the command and wrapper paths with locally installed paths.

```yaml
clients:
  codex:
    adapter: codex
    launch_argv: [codex, app-server, --stdio]
    # Resume is handled natively by the Codex app-server bridge.
  claude:
    adapter: claude
    launch_argv: [claude, --model, "{model}", "{prompt}"]
    resume_argv: [claude, --resume, "{thread_id}", --model, "{model}", "{prompt}"]
    # Optional external mailbox delivery:
    # deliver_argv: [/absolute/path/to/deliver-wrapper, "{thread_id}", "{message_file}", "{message_id}"]
  opencode:
    adapter: opencode
    launch_argv: [opencode, --model, "{model}", --prompt, "{prompt}"]
    resume_argv: [opencode, --session, "{thread_id}", --model, "{model}", --prompt, "{prompt}"]
  command:
    adapter: command
    launch_argv: [/absolute/path/to/agent-wrapper, --model, "{model}", --prompt-file, "{prompt_file}"]
    resume_argv: [/absolute/path/to/agent-wrapper, --resume, "{thread_id}", --model, "{model}", --prompt-file, "{prompt_file}"]
    deliver_argv: [/absolute/path/to/deliver-wrapper, "{thread_id}", "{message_file}", "{message_id}"]
profiles:
  thinker:
    reasoning_effort: medium
    routes:
      - {id: planner, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_THINKER_MODEL, max_concurrency: 1}
  orchestrator:
    reasoning_effort: high
    routes:
      - {id: orchestrator, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_ORCHESTRATOR_MODEL, max_concurrency: 3}
  supervisor:
    reasoning_effort: low
    routes:
      - {id: supervisor, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_SUPERVISOR_MODEL, max_concurrency: 1}
  worker:
    reasoning_effort: medium
    routes:
      - {id: worker, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_WORKER_MODEL, max_concurrency: 3}
defaults:
  orchestrator_profile: orchestrator
  # Optional; falls back to orchestrator_profile when omitted.
  dispatcher_profile: orchestrator
workflows:
  plan-first:
    profiles:
      orchestrator: orchestrator
      planning: thinker
      implementation: worker
      integration: worker
    max_parallel_tasks: 3
    capabilities: [tasks, phases, tasks.role.planner, tasks.role.implementer, tasks.role.integrator, planner_dependency, integration, landing]
forge:
  adapter: github
  remote: origin
  publication: ask
```

The bundled `plan-first` workflow is version 2. Its capability set is mandatory: a
configuration that still lists only the v1 `plan-first` capabilities gains the
integration and landing capabilities, and no configuration can drop them. The legacy
`issue-resolution` workflow was removed; an existing config entry is validated but
ignored, and projects that still reference it should migrate to `plan-first`.

Profiles are named routing policies. Each route is one client/provider/model candidate
with its own concurrency and optional launch budget. The route `id` is a stable name,
unique within its profile, used in routing decisions, `profile explain` output, and the
persisted Run provenance; it is not sent to the model provider. When a profile has
multiple eligible routes, workspace selects one using provider weights, active runs,
recent launches, capability requirements, cooldowns, and route limits. Workflow roles
select profiles through `workflows.<name>.profiles`.

The optional profile-level `reasoning_effort` applies to every route in that profile.
Workspace passes it through the selected client adapter, and the value must be supported
by the selected client and model. Supported spellings are adapter/provider-specific;
leave the field out to keep the client's existing default behavior.

For built-in `codex`, `claude`, and `opencode` clients, `launch_argv` and `resume_argv`
are optional because workspace supplies defaults when they are omitted. The example above
uses those defaults, and neither the generated configuration nor the wizard adds
approval-bypass flags. `--auto` for OpenCode, `--dangerously-skip-permissions`, and
similar options are a deliberate opt-in: add them to `launch_argv` and `resume_argv`
yourself only when you want to auto-approve permissions that are not explicitly denied.

`forge.adapter` can be `github`, `gitlab`, or `command`. Without an adapter, a local CR
package is created; the user can attach a real request or explicitly skip publication.
The `allowed` policy records prior consent to publish. With `ask`, the orchestrator shows
the prepared description and diff, then records the response it receives.
In `per-task` mode, prepare requests in task-dependency order. The `merge_after` field
and CR description identify predecessors; preparing a dependent CR requires their
current requests.

Codex uses app-server and supports wake-up between turns and native resume.
Claude/OpenCode start an interactive client. The default OpenCode client receives a
private loopback endpoint for each Run and delivery through the active TUI; the message
is marked with `message_id` and enters status only after confirmation in session history.
`session bind-thread` remains a fallback tool. [Client adapters and their capabilities](docs/clients.md).
After building a new version, restart the project supervisor to load the new delivery
code; an existing OpenCode session with a valid Run endpoint does not need to be recreated.

Routing counts launches and active reservations in the project over the last 24 hours,
accounting for provider weights, concurrency, capabilities, and cooldown after a start
failure. `profile explain NAME` shows the current route score; score history is stored
in the Session. This approximates load; it is not a counter of tokens, costs, or
account-wide limits. `max_concurrency` is a route limit for the entire project, shared
by all workspaces. Set at least as many slots on the orchestrator route as the number of
workspaces that must be able to keep an orchestrator session active at the same time.
The optional `max_launches_24h` route setting limits local launches of a given
client/provider/model in the project; zero means there is no such limit.

### Route usage limits

Workspace never reads provider HTTP headers; it records the limit signals its clients
expose. Observed limits are advisory, project-scoped, and live in
`.workspace/route-limits.json`. A record matches a whole client account (`client`), a
client and provider (`provider`), or one exact route (`route`); limits reported for
Codex and Claude default to `client`, other adapters to `provider`. A hard record
(`rate_limited` or `quota_exhausted`) makes its routes ineligible until the reset;
`usage_pressure` only ranks a route after routes without pressure. If every route of a
profile is limited, selection fails fast with `route_limited` and `retry_after` set to
the earliest reset.

Set the policy on a profile, or override it per route:

```yaml
profiles:
  worker:
    usage_limits:
      mode: avoid                 # avoid (default) | observe | ignore
      default_backoff_seconds: 900
      max_backoff_seconds: 21600
      soft_limit_percent: 90
    routes:
      - id: deepseek
        client: opencode
        provider: deepseek
        model: deepseek/deepseek-flash
        usage_limits:
          mode: ignore
```

`avoid` excludes limited routes and applies the soft preference, `observe` records and
shows limits without changing selection, and `ignore` ignores records for that route.
Without a reported reset the record lasts `default_backoff_seconds`, doubling on
repeated observations up to `max_backoff_seconds`; a reported reset is capped at seven
days.

Inspect and manage the ledger:

```sh
workspace profile limit list [--all]
workspace profile limit set <profile>/<route-id> --kind quota_exhausted|rate_limited \
  (--until RFC3339 | --for DURATION) [--scope client|provider|route] [--message ...]
workspace profile limit clear <profile>/<route-id> [--scope client|provider|route]
workspace profile limit report --kind ... [--reset-at RFC3339 | --retry-after DURATION] \
  [--used-percent N] [--scope ...] [--message ...]
```

`report` runs only inside a Run and records for that Run's route, so a client hook or
wrapper can call it using the `WORKSPACE_RUN_ID` in its environment. `set` and `clear`
are for the user or a workspace orchestrator and accept `--operation-key`. `workspace
profile explain NAME` shows each route's `limited_until` and `soft_limited` once a
record matches.

The optional `workspaces_dir: /absolute/path/project-workspaces` selects a directory
outside the repository. Configure it before creating workspaces; changing it does not
move existing directories. You can still select the project with `--project`; the CWD
inside an external workspace also contains enough information to discover it.

## Getting Started

Give the agent a ticket or description and use the `workspace` skill, or run:

```sh
workspace create --issue https://github.com/OWNER/REPO/issues/142 \
  --operation-key issue-142
# First-class Issue intake and durable revision history:
workspace issue create --issue https://github.com/OWNER/REPO/issues/142 \
  --operation-key intake-142
workspace issue list
workspace issue show issue_ID
workspace issue dispatch issue_ID --start --operation-key dispatch-142
# Create from an existing local Issue revision; the Workspace input is frozen:
workspace create --from-issue issue_ID --operation-key workspace-142
# You can also provide the description directly as an argument or through --input-file issue.md.
workspace create "Improve workspace creation"
# Choose the ID explicitly; otherwise it is derived from the title as a slug:
workspace create "Improve workspace creation" --id improve-workspace-creation
# --input-file can optionally be combined with --issue URL to preserve the source.
# Manual orchestration without a workflow:
workspace create "Ad-hoc analysis" --no-workflow
# Autonomous run (user-invoked; requires a deliver-capable orchestrator route):
workspace create "Unattended refactor" --autonomous --operation-key auto-1
workspace start --workspace ws_ID_Z_ODPOWIEDZI --operation-key orchestrator-1
workspace attach --workspace ws_ID_Z_ODPOWIEDZI
```

`create` persists the description; `start` launches the supervisor and orchestrator
without changing the view. `attach` switches to an existing tmux client or attaches
from outside. Mode selection is explicit: `--workflow NAME` selects a workflow, omitting
the flag selects the bundled `plan-first`, and `--no-workflow` creates an active manual
workspace — without phases, advance, or release, with task delegation, handoffs, checks,
and a fixed limit of 3 parallel workers. Omit `--workflow` for the common case; a legacy
workspace still waiting in `needs_workflow` can be resolved with `workspace workflow
select plan-first`. `--workflow` and `--no-workflow` cannot be combined, and a manual
workspace cannot later be converted to a workflow. [Trackers and snapshots](docs/trackers.md).

Workspace, Agent, Session, and Task IDs are human-readable slugs derived from the title
or name, such as `ws_named-ids`, `agent_planner`, `sess_planner`, and
`task_plan-named-ids`. They are unique per entity type across the workspace storage root
and are never reused, even after deletion. `workspace create`, `issue dispatch`,
`agent create`, `task create`, and `session start` accept `--id <slug>` to choose the ID
explicitly; the `<kind>_` prefix is optional, an invalid value fails with `invalid_id`, and
an already taken value fails with `id_exists` instead of gaining a suffix. Existing ULID
IDs stay valid and resolvable and are never migrated, so a legacy workspace can contain
new slug-ID entities. Run IDs remain random.

A workspace can run autonomously. `workspace create --autonomous` (or a user-run `issue
dispatch --autonomous`) starts it unattended, and `workspace autonomy enable|disable`
manages an existing active workspace (`enable` is terminal-user only; anyone authorized may
disable; `reopen` clears it). While the run is running, the orchestrator resolves the
orchestrator-level gates (plan and task/integrator acceptance, phase advance up to
integration, worker questions, bounded retry/retire) with recorded `--rationale`/`--evidence`
and finishes with `workspace autonomy report --summary-file <path> --outcome
ready_to_land|ready_to_complete|blocked|failed`, which stores an immutable summary artifact
and sets `autonomy.state=delivered`. Operations with external, local-branch,
lifecycle-terminal, or authorization effects — `integration land`, `complete`, change-request
publish/resolve, `release confirm`, live-testing answers, `reopen`, archive/clean/delete, and
`state edit` — stay with the user and are refused for agent actors while the run is running.
Autonomy requires a deliver-capable orchestrator route, and the project Dispatcher can never
create or enable an autonomous workspace.

Issue operations are project-scoped and local. `workspace issue refresh` reads the
configured tracker but never changes it; `issue update` changes only local status.
`workspace dispatcher start|status|stop|attach` manages the separate project-scoped
Dispatcher runtime. A Dispatcher may route Issues and create linked Workspaces, but
workspace actors cannot mutate project Issues. Every linked Workspace records the
Issue ID, revision, and digest in its durable input.

Returning to an existing workspace does not require remembering its ID:

```sh
workspace list --short
workspace open
# or without the menu:
workspace open "specification"
workspace open ws_ID
```

Global `--short` prints a concise summary for every command. For named resources
(workspaces, agents, tasks, and worktrees), this is a `name: ID` map; records without a
natural name retain their most important identifiers and state. `list --map` remains an
alias for `list --short`. `open` shows an interactive selector with the title, state,
phase, ID, and input source, then attaches to the tmux session.

A tmux session corresponds to a workspace, a window to a worktree, and a pane to a
specific Run. The orchestrator has its own window in the workspace directory.
Detaching the user does not stop processes.

## Terminal User Interface (TUI)

Run `workspace tui` to browse a project's Workspaces, durable Issues, project Dispatcher,
and their tasks, worktrees,
sessions, runs, results, and runtime state. Scope selection works the same as in the
CLI; you can pass `--project` and `--workspace`, and without a workspace the TUI opens
a picker. Reading also works on Windows, but tmux, jump, and a managed panel require a
binary running on Linux/macOS or WSL. The interface expects a terminal on stdin/stdout
and at least 40 columns × 12 rows.

```sh
workspace tui
workspace tui --project ./repo --theme dark
workspace tui --workspace ws_ID --no-color

# Managed pane in an existing orchestrator window:
workspace tui show --workspace ws_ID
workspace tui status --workspace ws_ID
workspace tui hide --workspace ws_ID
```

`g` resolves the selected workspace target and, when exactly one tmux client is attached
on that target's socket, jumps immediately with that client to the verified workspace,
window, and pane. With several clients it opens a picker; each row shows the client TTY
and current session. Confirming a row moves that explicit client to the verified
workspace, window, and pane; `Esc` or `Ctrl+C` cancels without changing tmux.
The TUI remembers the last successfully used client in ignored, project-local
`.workspace` data scoped by socket, marks it `last used`, and preselects it while it
remains attached. Outside tmux, the TUI still selects from the target server's attached
clients and never attaches its own terminal. A missing client, discovery failure,
changed target/client, or socket mismatch is reported without choosing another client.
Use `a` to start or resume an agent, then `g` to choose a client. The CLI command
`workspace attach` keeps its existing behavior.

Without a workspace, the project tabs are `1 Workspaces`, `2 Issues`, and `3 Dispatcher`.
The Issues tab shows durable revisions and linked Workspaces; opening an Issue shows its
frozen description and `a` can create a linked Workspace. The Dispatcher tab shows its
project-scoped state and offers start/stop actions. Opening or selecting a workspace
lands on Tasks. `1`–`5` open Tasks, Sessions,
Worktrees, Results, and More; below 60 columns the tabs shorten to `1 Tasks`,
`2 Sess`, `3 Trees`, `4 Out`, and `5 More`. Sessions is a plain collection without
secondary tabs: `f` switches between the current and history views, `Enter` opens the
session details, and `g` jumps with the only attached client or opens the attached-client
picker for a verified tmux jump.
`Tab`/`Shift+Tab` switches the
result type on Results only (the active type is named in the helper line).
`Up`/`Down` or `j`/`k` changes the selection, `Enter` opens an item, `Esc` goes back,
`/` filters a collection, and `f` switches between the status and history views. `s`
sorts the list. Use `a` for explicit start or resume actions and `g` to choose an attached
tmux client for a verified target. `o` opens the orchestrator page. More groups Agents, Services,
Decisions, Change requests, Runtime, Needs attention, Recent recorded activity, and
Documents, so attention and recorded activity are reachable from `5`. `a` opens only
the operations available for the selection, including creating and fully deleting
workspaces in the project picker, starting completed-workspace conversation, reopening
or archiving a completed workspace, and deleting unrelated tasks and inactive sessions
while ordinary task mutations remain unavailable after completion. `g` moves an attached
tmux client to the verified client/window/pane: it jumps immediately with the only
attached client and otherwise opens the attached-client picker, including when the TUI
itself runs outside tmux. A successful choice is remembered per project and tmux socket.
`r`
refreshes the read without reconcile, and `?` shows scrollable help grouped into
Navigation, View, Runtime, Actions, and Exit (already reachable at 40×12). `--theme`
accepts `auto`, `dark`, or `light`; `--no-color` forces textual badges.

The picker and workspace screens include a read-only supervisor health indicator. On a
workspace screen it also summarizes effective auxiliary-service health; `--no-color`
keeps the dot markers and explicit state/count text.

Worktrees shows a selectable revision tree with parent worktrees, base commits, and
related tasks. To record an explicit parent, create the next worktree with
`workspace worktree create next --base workspace/ws_ID/previous`. Creation also records
a parent for an explicit revision matching one unique worktree tip outside the workspace
base. Shared or unknown revisions remain unlinked. The relationship stays fixed after
branches advance; older records without parent metadata show only their known base.

In manually started TUI, `q` exits the program. In a managed pane, `q`, and also
`Ctrl+C` outside a form, first records a durable hide request, restores the terminal,
and only then exits; if the write fails, the pane remains open and the same operation
can be retried. `Ctrl+C` in a form cancels the form. `tui show` explicitly opens or
restores the pane, `tui hide` disables and cleans up only the verified pane, and
`tui status` shows its generation, ownership, and last known state. The supervisor
does not start or restore the TUI, and workspace start does not open it; show does not
start a new workspace or supervisor by itself.

The TUI uses the same core queries and mutations as the CLI. Confirmations are guarded
by the revision, task attempt, or exact RunID; “Pause and interrupt” shows the Runs and
services that will be stopped. The interface does not automatically accept handoffs or
decisions. Delete operations require entering the full ID. Tasks and sessions are
hidden through an auditable tombstone, and core rejects deletion of active or dependent
data or records with persisted results. Delete Workspace is a separate, irreversible
discard: after the full ID is entered, it stops the runtime and removes state,
worktrees, uncommitted files, and local workspace branches without requiring release or
archive. Archive preserves history; a workflow that declares a release gate requires a
confirmed release, while a completed manual or plan-first workspace requires an earlier
`complete`. `complete` requires no active worker sessions or services (the
orchestrator's own running session is tolerated and not stopped), while `archive`
requires every session and service to be stopped. A completed workspace also offers
conversation and the guarded `Reopen completed workspace` action; reopening records a
reason and revision, preserves
accepted history, and requires derived release/integration evidence to be rebuilt. The
TUI also provides `Complete this manual workspace` and, after landing,
`Complete this workflow workspace` as confirmed core operations.
Before downgrading the binary, hide the managed pane with `workspace tui hide`: the
older launcher does not yet recognize ownership of the new pane.
For screen and shortcut details, see [docs/tui.md](docs/tui.md).

## Tasks and Results

The orchestrator defines tasks with a goal, role, acceptance criteria, and dependencies:

```yaml
name: planning
title: Diagnose issue
goal: Explain the cause and propose an implementation plan.
role: planner
acceptance_criteria:
  - The diagnosis identifies evidence from the code.
  - The plan includes tasks and acceptance tests.
required_artifacts: [PLAN.md]
```

```sh
workspace task create --spec-file planning.yaml --id plan-named-ids --operation-key planning-task
workspace agent create planner --role planner --id agent_planner
workspace worktree create planning --purpose planning
workspace session start --agent planner --id sess_planner --parent-session sess_ORCHESTRATOR --task task_ID --worktree planning \
  --operation-key planning-start-1
workspace status
workspace menu
```

The client receives the agent, logical session, run, task, worktree, parent, and
orchestrator IDs in `WORKSPACE_*` (`WORKSPACE_SESSION_ID` and `WORKSPACE_RUN_ID` are
different). Address messages and handoffs to the exact **Session ID**; `Agent ID`
remains an ownership and legacy-compatibility projection. Executors write local work
products and submit them through `handoff submit`; the CLI copies explicitly selected
files to `artifacts/`. Message ACK and task acceptance are separate decisions.

```sh
workspace message send --to-session sess_PARENT --kind question --body-file question.md
workspace inbox list --session sess_PARENT
workspace inbox read msg_ID --session sess_PARENT
workspace inbox ack msg_ID --session sess_PARENT
workspace inbox list --agent agent_PARENT --all   # explicit agent-wide history
workspace check run --operation-key tests-attempt-1 --expected-exit 0 -- npm test
workspace handoff submit --task task_ID --to-session sess_PARENT \
  --summary-file work-products/SUMMARY.md \
  --artifact work-products/IMPLEMENTATION.md --check check_ID
workspace handoff accept handoff_ID
workspace workflow advance
workspace task supersede task_ID --reason "Replaced by task_ID_v2"
```

`check run` returns a receipt; read its `exit_code` and `expected_exit`. Recording a
receipt does not mean the test succeeded. Acceptance checks workflow criteria, required
artifacts, and declared check outcomes. Use `task retry` for a new attempt of the same
contract; use `task cancel`, `task abandon`, or `task supersede` with a reason to retire
work that will not run. Retired tasks remain in history and do not block phase gates.
`workspace workflow advance` applies only to a workspace with a selected workflow;
manual mode uses the same tasks, handoffs, and checks without advance.
[Test evidence](docs/checks.md).

The `plan-first` workflow continues past implementation. After every live implementation
task is accepted, `workspace workflow advance` enters the `integration` phase. The
orchestrator runs `workspace integration prepare` (the default base is the current tip of
the target branch, `--target` selects another branch) and delegates one `integrator` task
into the prepared worktree. The integrator merges each accepted implementation head with
`git merge --no-ff`, resolves textual conflicts, runs the project checks, and records
every resolution in `INTEGRATION.md`; a conflict that needs a product or implementation
decision becomes a blocked question instead of a guess. Accepting the handoff records the
integrated HEAD.

After that, the user approves the local fast-forward and the orchestrator runs
`workspace integration land --expected-revision N --user-confirmed`. Landing fast-forwards
the target (`git merge --ff-only` in a clean checkout, or a compare-and-swap
`update-ref` otherwise), refuses a dirty, moved, or diverged target, is idempotent under
its operation key, and never pushes. `workspace complete --expected-revision N
--user-confirmed` then closes the workflow; a workspace with no live implementer can
complete with an explicit `--reason` and nothing to integrate.

`change-request prepare/publish/sync`, live testing, and `release confirm` are available
only to custom workflows that declare those capabilities; the removed `issue-resolution`
workflow was the bundled example. Without a release gate, a completed workflow is
archivable without a release reference. `--user-confirmed` in an orchestrator session
means forwarding a response actually received from the user, not the model granting
consent on its own. In manual mode all workflow-only operations return
`operation_not_applicable`, and manual mode ends with an explicit `complete` operation.

Complete a manual workspace, or land and complete a plan-first workflow:

```sh
workspace complete --reason "Analysis delivered" --user-confirmed --operation-key complete-1
workspace archive

workspace integration land --expected-revision 12 --user-confirmed --operation-key land-1
workspace complete --expected-revision 13 --user-confirmed --operation-key complete-2
workspace archive
```

After a workflow or manual workspace is completed, `workspace start` and
`agent resume orchestrator` reopen the existing compatible orchestrator Session for
conversation only. They do not create tasks, worktrees, services, checks, handoffs, or
release state. A worker may resume an existing accepted-task Session for consultation;
new worker execution requires an explicit reopen. Archived workspaces are terminal for
conversation and execution.

## Resumption and State

`agent resume NAME` preserves a compatible logical Session and creates a new Run,
preferring the bound native thread. Changing the agent, task/attempt/input lineage,
worktree, or native thread starts a new Session. `session list` shows one record per
conversation; `session history sess_ID` and `run list` show all executions.
Session output separates `state` (operational projection), `lifecycle_state`,
`run_state`, and `client_state`. A live Codex or native OpenCode Run can therefore show
`state: idle` after a positive client observation while remaining active, owning its
agent/worktree, and eligible for delivery. The compact machine-readable view includes
`client_state`; an observation failure keeps the last known value rather than guessing.
`session close sess_ID --reason ...` closes a resumable idle context and blocks further resume.
`pause` suspends delegation, `pause --interrupt` stops active runs, and `resume` unblocks
the work; `workspace resume` applies only to paused workspaces and never reactivates a
completed workspace. `reconcile` reconciles lost panes and interrupted operations.

Completed workspaces remain inspectable and conversational. A completed Run is marked
`conversation_only`; its Codex/native bridge remains available for questions and exact
Session-addressed messages, but task, handoff, artifact, check, integration, release,
and resource mutations are rejected until the workspace is reopened. To authorize new
work, inspect the current revision and run:

```sh
workspace reopen --reason "User requested follow-up fixes" \
  --expected-revision 42 --operation-key reopen-1
```

Reopen is an explicit, optimistic-concurrency-guarded mutation. It preserves tasks,
artifacts, handoffs, the base commit, and prior WORKSPACE.md under
`history/reopen_ID/`, while invalidating release/integration/live-test state and marking
change requests outdated. An agent actor must also pass `--user-confirmed`, attesting
that the user authorized the follow-up. `archive` and `clean --dry-run` remain separate
from release confirmation; a completed manual or plan-first workspace is archived after an
earlier `complete`, and an archived workspace cannot be reopened.
[Runtime, communication, and cleanup](docs/runtime.md).

A workspace created by an older binary under the removed `issue-resolution` workflow is
migrated to `plan-first` automatically on first load. The migration rewrites the workflow
id, version, capabilities, and phase (every integration-stage phase becomes `integration`),
preserves tasks, handoffs, integration, change-request, live-test, and release history,
supersedes a pending live-testing decision, and — for a non-terminal workspace — stores the
previous snapshots plus a `migration.json` manifest under `history/<revision_id>/`. It does
not invalidate accepted
results and is a no-op on the second load. A v1 `plan-first` snapshot that predates the
integration stage keeps its frozen contract and completes after implementation;
`workspace workflow migrate` opts it into v2 explicitly. Completed legacy workspaces
become archivable without a release.

WORKSPACE.md is the canonical mode and workflow state; in a manual workspace it has an
empty workflow and the `manual` phase label. `.runtime/index.json` contains a private
operational registry at `schema_version: 5`, with separate `sessions` and `runs` arrays.
`status --json` publishes the same explicit versioned contract. An older index is migrated
atomically on first open; historical `sess_*` values remain durable Run aliases, so
checks, handoffs, artifacts, and messages retain provenance. Update the description with
`state update --expected-revision N`; `state edit` is for controlled editing while
paused. Input changes and template migration preserve history and invalidate dependent
results: [revisions](docs/revisions.md).

`--json` returns full `{ok,data}` or `{ok:false,error:{code,message}}` and takes
precedence over `--short`. Keyed mutations also contain `operation_id` and the workspace
revision. `--non-interactive` returns missing decisions for the user to resolve.
`--operation-key` preserves the logical operation result; retrying with a different
payload returns a conflict. [Retry and revision contract](docs/operations.md). Do not
edit registries or WORKSPACE.md outside the CLI during active work.

## Verification

```sh
go test ./...
go vet ./...
WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s
temp_dir=$(mktemp -d)
CGO_ENABLED=0 go build -trimpath -o "$temp_dir/workspace" ./cmd/workspace
python3 scripts/check-install.py "$temp_dir/workspace"
python3 scripts/test-install.py
python3 scripts/test-setup-dev.py
scripts/build-release.sh v0.1.0 "$temp_dir/release"
```

The release build can also be smoke-tested with:

```sh
(cd "$temp_dir/release" && sha256sum -c checksums.txt)
tar -xzf "$temp_dir/release/workspace_0.1.0_linux_amd64.tar.gz" -C "$temp_dir"
"$temp_dir/workspace" --json version
```

On macOS use `shasum -a 256 -c checksums.txt` when `sha256sum` is unavailable.

Tests use isolated repositories and private tmux servers. The full workflow test runs
deterministic agent processes, real Git/CLI/handoff/check flows, and the tester. Forge
and the model are fixtures; tests do not publish external PRs or consume model credits.
The Codex protocol test checks wake-up and native resume.
