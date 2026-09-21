# workspace

CLI for managing persistent task context, agents, Git worktrees, and tmux.
The orchestrator implements the simple `plan-first` workflow: planning in a separate
worktree and delegating implementation to subsequent worktrees. A workspace can also
run manually without a workflow, using explicit tasks, worktrees, handoffs, and checks.

- **Agent**: a persona with a role, instructions, prompt template, and model profile.
- **Session**: persistent logical conversation context for a given agent/task/worktree lineage.
- **Run**: one concrete client and tmux-pane execution; stores the model, argv, prompt,
  process result, and exact operation provenance.
- **Workspace**: WORKSPACE.md, AGENTS.md, WORKFLOW.md, tasks, artifacts, and worktrees.

The product vision and boundaries are described in [PRODUCT.md](PRODUCT.md), while the
technical model and reference map are in [ARCHITECTURE.md](ARCHITECTURE.md).

## Installation

Requirements: Go 1.24+, Git, and tmux. Runtime runs on Linux/macOS or Linux in WSL.
Windows supports building and core tests; run sessions with a Linux binary in WSL.

```sh
go build -o bin/workspace ./cmd/workspace
export PATH="$PWD/bin:$PATH"
workspace --help
```

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
`init` preserves configuration and installs editable templates in `.workspace/templates`.
Configuration and templates may be versioned; working data has local Git ignore rules.

## Configuration

Keep the generated `schema_version`, `project_id`, and `runtime`. Add clients and
profiles to `.workspace/config.yaml`. Replace model names with identifiers available in
your own account; profiles contain no built-in assumptions about pricing or subscription.

```yaml
clients:
  codex:
    adapter: codex
  claude:
    adapter: claude
  opencode:
    adapter: opencode
profiles:
  thinker:
    routes:
      - {id: planner, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_THINKER_MODEL, max_concurrency: 1}
  orchestrator:
    routes:
      - {id: orchestrator, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_ORCHESTRATOR_MODEL, max_concurrency: 3}
  supervisor:
    routes:
      - {id: supervisor, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_SUPERVISOR_MODEL, max_concurrency: 1}
  worker:
    routes:
      - {id: worker, client: opencode, provider: YOUR_PROVIDER, model: YOUR_PROVIDER/YOUR_WORKER_MODEL, max_concurrency: 3}
defaults:
  orchestrator_profile: orchestrator
workflows:
  plan-first:
    profiles:
      orchestrator: orchestrator
      planning: thinker
      implementation: worker
    max_parallel_tasks: 3
forge:
  adapter: github
  remote: origin
  publication: ask
```

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

The optional `workspaces_dir: /absolute/path/project-workspaces` selects a directory
outside the repository. Configure it before creating workspaces; changing it does not
move existing directories. You can still select the project with `--project`; the CWD
inside an external workspace also contains enough information to discover it.

## Getting Started

Give the agent a ticket or description and use the `workspace` skill, or run:

```sh
workspace create --issue https://github.com/OWNER/REPO/issues/142 \
  --workflow plan-first --operation-key issue-142
# You can also provide the description directly as an argument or through --input-file issue.md.
workspace create "Improve workspace creation" --workflow plan-first
# --input-file can optionally be combined with --issue URL to preserve the source.
# Manual orchestration without a workflow:
workspace create "Ad-hoc analysis" --no-workflow
workspace start --workspace ws_ID_Z_ODPOWIEDZI --operation-key orchestrator-1
workspace attach --workspace ws_ID_Z_ODPOWIEDZI
```

`create` persists the description; `start` launches the supervisor and orchestrator
without changing the view. `attach` switches to an existing tmux client or attaches
from outside. Mode selection is explicit: `--workflow NAME` selects a workflow, no flag
creates the `needs_workflow` state (the orchestrator asks for a choice before
delegating), and `--no-workflow` creates an active manual workspace — without phases,
advance, or release, with task delegation, handoffs, checks, and a fixed limit of 3
parallel workers. `--workflow` and `--no-workflow` cannot be combined, and a manual
workspace cannot later be converted to a workflow. [Trackers and snapshots](docs/trackers.md).

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

Run `workspace tui` to browse a project's workspaces and their tasks, worktrees,
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

Opening or selecting a workspace lands on Tasks. `1`–`5` open Tasks, Sessions,
Worktrees, Results, and More; below 60 columns the tabs shorten to `1 Tasks`,
`2 Sess`, `3 Trees`, `4 Out`, and `5 More`. Sessions is a plain collection without
secondary tabs: `f` switches between the current and history views, `Enter` opens the
session details, and `t` opens or resumes its terminal. `Tab`/`Shift+Tab` switches the
result type on Results only (the active type is named in the helper line).
`Up`/`Down` or `j`/`k` changes the selection, `Enter` opens an item, `Esc` goes back,
`/` filters a collection, and `f` switches between the status and history views. `s`
sorts the list. `t` opens the selected agent's terminal or a resume confirmation;
`o`, then `t`, quickly opens or starts the orchestrator. More groups Agents, Services,
Decisions, Change requests, Runtime, Needs attention, Recent recorded activity, and
Documents, so attention and recorded activity are reachable from `5`. `a` opens only
the operations available for the selection, including creating and fully deleting
workspaces in the project picker, starting completed-workspace conversation, reopening
or archiving a completed workspace, and deleting unrelated tasks and inactive sessions
while ordinary task mutations remain unavailable after completion. `g` jumps to a
verified tmux window/pane. `r`
refreshes the read without reconcile, and `?` shows scrollable help grouped into
Navigation, View, Runtime, Actions, and Exit (already reachable at 40×12). `--theme`
accepts `auto`, `dark`, or `light`; `--no-color` forces textual badges.

In manually started TUI, `q` exits the program. In a managed pane, `q`, and also
`Ctrl+C` outside a form, first records a durable hide request, restores the terminal,
and only then exits; if the write fails, the pane remains open and the same operation
can be retried. `Ctrl+C` in a form cancels the form. `tui show` restores the pane,
`tui hide` disables and cleans up only the verified pane, and `tui status` shows its
generation, ownership, and backoff. The supervisor reconciles the pane together with
the orchestrator; show does not start a new workspace or supervisor by itself.

The TUI uses the same core queries and mutations as the CLI. Confirmations are guarded
by the revision, task attempt, or exact RunID; “Pause and interrupt” shows the Runs and
services that will be stopped. The interface does not automatically accept handoffs or
decisions. Delete operations require entering the full ID. Tasks and sessions are
hidden through an auditable tombstone, and core rejects deletion of active or dependent
data or records with persisted results. Delete Workspace is a separate, irreversible
discard: after the full ID is entered, it stops the runtime and removes state,
worktrees, uncommitted files, and local workspace branches without requiring release or
archive. Archive preserves history; a workflow still requires a confirmed release,
while a manual workspace requires an earlier `complete`, and both require no active
sessions/services. A completed workspace also offers conversation and the guarded
`Reopen completed workspace` action; reopening records a reason and revision, preserves
accepted history, and requires derived release/integration evidence to be rebuilt. The
TUI also provides `Complete this manual workspace` as a confirmed core operation.
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
workspace task create --spec-file planning.yaml --operation-key planning-task
workspace agent create planner --role planner
workspace worktree create planning --purpose planning
workspace session start --agent planner --parent-session sess_ORCHESTRATOR --task task_ID --worktree planning \
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
workspace check run --operation-key tests-attempt-1 -- npm test
workspace handoff submit --task task_ID --to-session sess_PARENT \
  --summary-file work-products/SUMMARY.md \
  --artifact work-products/IMPLEMENTATION.md --check check_ID
workspace handoff accept handoff_ID
workspace workflow advance
```

`check run` returns a receipt; read its `exit_code`. Recording a receipt does not mean
the test succeeded. Acceptance checks workflow criteria, required artifacts, and results.
`workspace workflow advance` applies only to a workspace with a selected workflow;
manual mode uses the same tasks, handoffs, and checks without advance.
[Test evidence](docs/checks.md).

The orchestrator delegates integration to a separate executor after `integration prepare`.
`change-request prepare/publish/sync` preserve the revision and CR identifier. After
publication, the menu offers live testing with a separate profile and session in the
integrated worktree. A test or explicit skip leads to waiting for release. A merge alone
does not complete the workflow; `release confirm --reference REF` records user
confirmation. `--user-confirmed` in an orchestrator session means forwarding a response
actually received from the user, not the model granting consent on its own. Integration,
change requests, live testing, and `release confirm` are workflow-only operations; in
manual mode they return `operation_not_applicable`, and manual mode ends with an
explicit `complete` operation.

Complete a manual workspace with a confirmed operation that does not require a release:

```sh
workspace complete --reason "Analysis delivered" --user-confirmed --operation-key complete-1
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
`session close sess_ID --reason ...` closes the idle context and blocks further resume.
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
from release confirmation; a manual workspace is archived after an earlier `complete`,
and an archived workspace cannot be reopened.
[Runtime, communication, and cleanup](docs/runtime.md).

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
go build -o bin/workspace ./cmd/workspace
python3 scripts/check-install.py bin/workspace
```

Tests use isolated repositories and private tmux servers. The full workflow test runs
deterministic agent processes, real Git/CLI/handoff/check flows, and the tester. Forge
and the model are fixtures; tests do not publish external PRs or consume model credits.
The Codex protocol test checks wake-up and native resume.
