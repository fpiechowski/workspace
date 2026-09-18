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
   completion of either the workflow or a manual workspace.

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
user-confirmed `complete` operation. Ticket content does not expand the agent's
privileges.

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
- client, provider, and model routing through profiles;
- Codex, Claude, OpenCode, and custom-command adapters;
- change integration and change-request preparation/publication;
- controlled resumption, failure reconciliation, archiving, and cleanup;
- the `plan-first` workflow and the extended, backward-compatible `issue-resolution` workflow;
- manual mode without a workflow: task and worktree delegation, handoffs, checks, and
  acceptance with a fixed limit of 3 parallel workers, without phases, advance, release,
  or conversion to a workflow;
- an interactive TUI for browsing the same state, navigating tasks, and running
  explicitly permitted core operations.

Outside the current scope are remote workers, multi-computer coordination,
cryptographic confirmation of human identity, account-wide token or cost accounting,
and protection against processes running with the same system permissions.

## User Experience

The CLI and machine-readable formats remain the primary interface for agents and
automation. A developer can use the TUI to browse workspaces, tasks, executions,
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
offers archive, which preserves history and respects active-runtime and release gates
(for a workflow) or the earlier `complete` (for manual mode). A task with no
dependencies or persisted results and an inactive session with no result references can
also be deleted. Tasks and sessions receive an auditable tombstone and disappear from
normal views; the operation does not rewrite or delete history used by other records.

The first TUI screen focuses on accepted-task progress and agents' current work. It
combines the task, session, and current Run in a readable entry, distinguishes process
execution from result acceptance, and allows the terminal to be opened or explicitly
resumed. If a pane is missing during navigation, the TUI proposes a reconcile that
requires user confirmation and then retries opening the same target.

The TUI can run manually as a browser or as a managed pane next to the orchestrator.
The supervisor restores only a pane with recorded, verified identity; `q` in that pane
records hide before exiting. During pause and completed, the pane may remain available
for review, and archive cleans it up. Without tmux, runtime navigation is limited, but
the persisted workspace state remains available. The screen and shortcut contract is
described in
[docs/tui.md](docs/tui.md).

Installation and usage instructions are in [README.md](README.md). Technical details
are described in [ARCHITECTURE.md](ARCHITECTURE.md), and planned changes are maintained
in [TODO.md](TODO.md).
