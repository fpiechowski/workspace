# `workspace` Product Vision

## Goal

`workspace` is a local tool for carrying out complex changes in a Git repository through
one orchestrator and multiple specialized agents. It preserves work context, separates
planning from implementation, isolates changes in Git worktrees, and leaves product
decisions, publication, and completion under human control. Work can use a named
workflow or be conducted manually without a workflow.

The product addresses the problem of agent sessions that are easy to start but difficult
to resume and coordinate safely. A terminal or chat history alone does not say which
result was accepted, which code revision it refers to, or whether an operation can be
safely retried. `workspace` records this information as explicit project state.

## Audience

The primary user is a developer using CLI agents while working on an existing
repository. The second audience is the agent itself: a stable CLI, JSON/YAML, and an
installable skill let it perform operations without guessing the state.

At the beginning of a new agent session, or after context compaction, the agent can run
`workspace prime` to read the current binary's bundled general operational guidance.
This is a project-independent, read-only context-recovery aid; it does not replace
`workspace status` or `workspace menu`, which remain the sources for live workspace
state and available actions.

The tool is especially useful when a task:

- requires diagnosis and a plan first;
- can be split into dependent or parallel parts;
- involves different roles, models, or agent clients;
- must survive interruption of a process, terminal, or computer;
- requires a record of decisions, test results, and artifact provenance.

## Product Promise

The user provides a ticket or problem description and chooses one of three explicit
paths: a named workflow, a deliberate deferral of the choice (`needs_workflow`), or
manual work without a workflow (`--no-workflow`). Work can be observed in tmux. The
orchestrator delegates planning, implementation, integration, and testing in a
workflow; in manual mode it creates explicit tasks and worktrees without phases or
release. Each result identifies a task, execution, commit, and verification evidence.
After an interruption, the system reconstructs state from files instead of relying only
on conversation memory.

Success means that the user can:

1. create a workspace with a durable input snapshot;
2. safely delegate work to isolated worktrees;
3. inspect current state, decisions, artifacts, and execution history;
4. resume interrupted work without duplicating uncertain operations;
5. deliberately approve publication and live testing in a workflow and confirm
   completion of either the workflow or a manual workspace;
6. return to a completed workspace for an explicitly authorized follow-up without
   losing accepted history or confusing conversation with new execution.

## Product Principles

### State Matters More Than Chat History

`WORKSPACE.md`, the workflow snapshot, and runtime registries are the source of
recoverable state. Conversation history can improve continuity, but it does not replace
tasks, decisions, artifacts, or operation identifiers.

### Delegation Is Explicit

A task has a goal, role, acceptance criteria, dependencies, and required products.
Process completion does not mean that the result was accepted, and receiving a message
does not mean that the handoff was accepted. The orchestrator evaluates the result
before accepting a task and, in a workflow, before advancing the phase.

### Humans Retain Decisions with External Effects

An agent may prepare a change request and present a diff, but publication depends on
the policy configured by the user. A workflow ends only after user confirmation of
deployment or release; a manual workspace closes only through the explicit,
user-confirmed `complete` operation. A completed workspace remains conversationally
inspectable, but new execution and durable task/resource mutations require the explicit
`workspace reopen` operation with a reason and exact revision; an agent actor must also
carry the user's confirmation. Ticket content does not expand the agent's privileges.

### Retrying Must Not Duplicate Work

Mutations have operation keys and durable receipts. Repeating the same intent returns
the previous result; a changed intent requires a new key. An uncertain external effect
is reconciled first rather than blindly executed again.

### Local User Work Is Protected

Worktrees isolate writable tasks. Cleanup does not remove active, dirty, or unprotected
changes. The tool does not treat a write lease as an operating-system sandbox and does
not promise protection from arbitrary processes running outside it.

### Client Capabilities Are Explicit

Process launch, native conversation resume, message delivery, observation, and
interruption are separate adapter capabilities. A generic launcher remains useful, but
full autonomous communication requires a client that supports delivery.

## Product Scope

The current scope includes:

- a single local Git project and multiple workspaces;
- three explicit workspace-creation modes: named workflow, deferred choice
  (`needs_workflow`), and manual orchestration without a workflow (`--no-workflow`);
- planning and implementation in separate worktrees;
- tmux as visible process runtime;
- agent personas, logical sessions, and the history of specific executions;
- a durable inbox, handoffs, immutable artifacts, and captured command results;
- durable project-scoped Issues with immutable revision snapshots, source provenance,
  refresh/status history, and guarded Workspace links;
- a project-scoped Dispatcher that can inspect Issues and create linked Workspaces
  through the same durable, idempotent operations as the CLI;
- client, provider, and model routing through profiles;
- Codex, Claude, OpenCode, and custom-command adapters;
- change integration and change-request preparation/publication;
- controlled resumption, failure reconciliation, archiving, and cleanup;
- completed-workspace conversation and an auditable, idempotent reopen path for
  explicitly authorized follow-up work;
- the `plan-first` workflow and the extended, backward-compatible `issue-resolution` workflow;
- manual mode without a workflow: task and worktree delegation, handoffs, checks, and
  acceptance with a fixed limit of 3 parallel workers, without phases, advance, release,
  or conversion to a workflow;
- an interactive TUI for browsing the same state, navigating tasks, and running
  explicitly permitted core operations.
- installation from the published GitHub Release archives and an explicit
  `workspace upgrade` path that verifies and atomically installs a newer stable release.

Outside the current scope are remote workers, multi-computer coordination,
cryptographic confirmation of human identity, account-wide token or cost accounting,
and protection against processes running with the same system permissions.

## User Experience

The CLI and machine-readable formats remain the primary interface for agents and
automation. A developer can use the TUI to browse Issues, workspaces, tasks, executions,
worktrees, results, and runtime, then jump to a verified tmux pane. The TUI uses the
same core queries and operations as the CLI; every mutation has confirmation and a
guard for the current revision, attempt, or RunID. It does not add actions for sending
messages, acknowledging the inbox, or automatically accepting results.
Native OpenCode delivery is visible in the active parent TUI: the Run-scoped loopback
server selects the current session, appends the marker-bearing workspace prompt, and
submits it through the active TUI control API. Delivery is confirmed against the
current Run only after the marker appears in session history; it remains distinct from
inbox ACK and handoff acceptance.

Messages and handoffs use exact logical Session addresses. `ToAgent` remains an audit
and legacy projection, while `--to` is accepted only when it resolves to one eligible
Session; idle Sessions remain pending and closed or deleted Sessions are never silently
rerouted. Agents inspect, acknowledge, and review only their current Session, while the
user selects a Session explicitly or requests an agent-wide historical view explicitly.

In the project picker, the user can create a workspace by choosing a named workflow,
deliberately deferring the choice (`needs_workflow`), or creating a manual workspace
without a workflow. The modes are distinguished by the phase label: `manual` for
manual orchestration and `-` for deferred selection. A manual workspace is completed by
the explicit, user-confirmed `complete` operation, after which archive does not require
a release; a workflow still requires a confirmed release. Alternatively, the user can
deliberately discard the entire workspace without requiring release/archive. Full
deletion requires retyping the ID, stops the runtime, and removes state, worktrees,
uncommitted files, and local workspace branches. Within a completed workspace, the TUI
offers conversation, guarded reopen, and archive. Conversation reuses the compatible
orchestrator Session where possible, marks the new Run as conversation-only, and keeps
exact Session-addressed messaging available. Reopen preserves tasks, artifacts,
handoffs, the base commit, and the prior WORKSPACE.md under history, while invalidating
release/integration/live-test state and marking change requests outdated. It refuses
active worker Sessions and services; archive remains a history-preserving terminal
operation. Ordinary task/resource mutation is not available in completed or archived
state. A task with no dependencies or persisted results and an inactive session with no
result references can also be deleted before completion. Tasks and sessions receive an
auditable tombstone and disappear from normal views; the operation does not rewrite or
delete history used by other records.

The first TUI screen is Tasks: each row combines the task, its state, and the number of
active executions, sessions, and runs of its current attempt. The primary navigation
order is Tasks, Sessions, Worktrees, Results, and More. Sessions is a plain collection
without secondary tabs, with a current/history filter and entry into the session
details and its terminal, and More provides Needs attention and Recent recorded
activity among other pages. Opening a task shows its related sessions, worktrees, and
results, distinguishes process execution from result acceptance, and allows the
terminal to be opened or explicitly resumed. If a pane is missing during navigation,
the TUI proposes a reconcile that requires user confirmation and then retries opening
the same target.

At project scope, the TUI exposes Issues and Dispatcher alongside Workspaces. An Issue
detail shows its source, retrieved revision, status reason, linked Workspaces, and the
guarded action to create a Workspace from that exact snapshot. Dispatcher start, status,
stop, and attach actions are project-scoped; Dispatcher text is treated as untrusted
input and cannot expand project mutation authority.

The TUI can run manually as a browser or, after an explicit user action, as a managed
pane next to the orchestrator. The supervisor does not start, restore, or stop the TUI;
`workspace tui show` and `workspace tui hide` own that lifecycle, while `q` in the pane
records hide before exiting. During pause and completed, the pane may remain available
for review until the user closes it. Without tmux, runtime navigation is limited, but
the persisted workspace state remains available. The screen and shortcut contract is
described in
[docs/tui.md](docs/tui.md).

### Distribution and upgrades

The public GitHub repository and its published Releases are the canonical distribution
source. The supported release targets are Linux amd64/arm64 and macOS amd64/arm64;
Windows users run the Linux build in WSL. `workspace version` is available outside a
project, and `workspace upgrade` explicitly checks the newest stable Release, verifies
its checksum and archive shape, and atomically replaces the installed executable.
Upgrade never changes project/workspace state, performs privilege escalation, or
downgrades a stable build. Supervisors and tmux processes already running keep their
old in-memory code until restarted.

`workspace prime` is the corresponding project-independent guidance command. It reads
the current binary's embedded workspace skill, removes only its leading YAML front
matter, and prints the Markdown body directly. `workspace prime --json` returns the
same text in `data.instructions` within the normal success envelope. Neither form
resolves project state or reports the current workspace; agents use `status` and
`menu` for those live queries.

### TUI health indicators

The TUI shell also shows the current project supervisor health in the project picker
and on every workspace route. A running supervisor is shown as a green dot and
`server running`; stopped, conflicting, or unavailable supervision is shown as a red
dot with the explicit state. Workspace routes add the effective auxiliary-service
health: the latest record for each service name determines the active/failed counts,
so a successful restart supersedes an older failure. These indicators are read-only,
refresh independently, retain the last good observation after a read error, and keep
their marker and state/count text in `--no-color` mode.

Installation and usage instructions are in [README.md](README.md). Technical details
are described in [ARCHITECTURE.md](ARCHITECTURE.md), and planned changes are maintained
in [TODO.md](TODO.md).
