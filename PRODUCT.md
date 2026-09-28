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

The user provides a ticket or problem description and chooses an explicit creation path:
a named workflow (defaulting to `plan-first`) or manual work without a workflow
(`--no-workflow`). Work can be observed in tmux. The orchestrator delegates planning,
implementation, integration, and testing in a workflow; in manual mode it creates explicit
tasks and worktrees without phases or release. Each result identifies a task, execution,
commit, and verification evidence. After an interruption, the system reconstructs state
from files instead of relying only on conversation memory.

Success means that the user can:

1. create a workspace with a durable input snapshot;
2. safely delegate work to isolated worktrees;
3. inspect current state, decisions, artifacts, and execution history;
4. resume interrupted work without duplicating uncertain operations;
5. deliberately approve the integration landing in a workflow and confirm completion of
   either the workflow or a manual workspace;
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
the policy configured by the user. A plan-first workflow ends only after the user
confirms landing the accepted integration into the target branch and then confirms
completion; a custom workflow that declares a release gate ends with release
confirmation instead. A manual workspace closes only through the explicit,
user-confirmed `complete` operation. A completed workspace remains conversationally
inspectable, but new execution and durable task/resource mutations require the explicit
`workspace reopen` operation with a reason and exact revision; an agent actor must also
carry the user's confirmation. Ticket content does not expand the agent's privileges.

### Autonomous Runs Are Auditable and Bounded

A user may create a workspace in autonomous mode, or enable it later from the terminal,
so the orchestrator resolves the gates that are already orchestrator-level in code — plan
acceptance, task and integrator result acceptance, phase advance up to integration,
worker questions, and bounded retry/retire — without waiting for a person. Every such
decision records a rationale, evidence, subject, and Run provenance atomically with the
gate it resolves. The run ends with a durable final report: an immutable summary artifact
and an outcome (`ready_to_land`, `ready_to_complete`, `blocked`, or `failed`) that moves
the workspace to `autonomy.state=delivered`, and nothing in the run waits for an answer.
Operations with external, local-branch, lifecycle-terminal, or authorization effects stay
with the user and are refused in code for agent actors while the run is running:
integration landing, completion, change-request publication/resolution, release
confirmation, live-testing answers, reopen, archive/clean/delete, and state edit.
Autonomy also requires a deliver-capable orchestrator route, and the project Dispatcher
can never create or enable an autonomous workspace.

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

Session status keeps lifecycle, execution, and client activity separate. `lifecycle_state`
describes whether a logical Session is active, resumable, or closed; `run_state` describes
the concrete current Run; and the operational `state` follows that Run. `client_state` is
the latest independent adapter observation. A current starting/running Run therefore
remains operationally starting/running even when the client reports `idle`; the Session
still owns its agent/worktree and continues to receive delivery. The supervisor never
infers idleness from tmux focus, output silence, elapsed time, or a failed observation.
Codex and native OpenCode provide the positive observations, which are shown separately
from the concrete operational state.

## Product Scope

The current scope includes:

- a single local Git project and multiple workspaces;
- two explicit workspace-creation modes: a named workflow (defaulting to `plan-first`)
  and manual orchestration without a workflow (`--no-workflow`); a legacy workspace still
  waiting in `needs_workflow` can be resolved with `workflow select`;
- planning, implementation and integration in separate worktrees;
- tmux as visible process runtime;
- agent personas, logical sessions, and the history of specific executions;
- a durable inbox, handoffs, immutable artifacts, and captured command results;
- durable project-scoped Issues with immutable revision snapshots, source provenance,
  refresh/status history, and guarded Workspace links;
- a project-scoped Dispatcher that can inspect Issues and create linked Workspaces
  through the same durable, idempotent operations as the CLI;
- client, provider, and model routing through profiles;
- Codex, Claude, OpenCode, and custom-command adapters;
- change integration and change-request preparation/publication for workflows that
  declare those gates;
- controlled resumption, failure reconciliation, archiving, and cleanup;
- completed-workspace conversation and an auditable, idempotent reopen path for
  explicitly authorized follow-up work;
- the capability-declared `plan-first` workflow: planning, implementation, a mandatory
  integration stage, and a user-approved local landing gate;
- manual mode without a workflow: task and worktree delegation, handoffs, checks, and
  acceptance with a fixed limit of 3 parallel workers, without phases, advance, release,
  or conversion to a workflow;
- per-workspace autonomous runs, chosen by the user at creation or enabled later from the
  terminal, that resolve the orchestrator-level gates with recorded rationales and end in
  a durable report while external, local-branch, lifecycle, and authorization effects
  stay with the user;
- an interactive TUI for browsing the same state, navigating tasks, and running
  explicitly permitted core operations.
- validated first-run project setup that collects the client adapters, an
  account-specific orchestrator route, and optional role profiles and workflows before
  writing the configuration once, and never enables client permission bypasses.
- installation from the published GitHub Release archives and an explicit
  `workspace upgrade` path that verifies and atomically installs a newer stable release.

Workflow snapshots declare their task roles and resource gates. `plan-first` (version 2)
supports planning, implementation and integration, and ends with a user-approved local
landing followed by `workspace complete`; it does not declare change requests, live
testing, or release. The legacy `issue-resolution` workflow was removed; existing
workspaces that reference it are migrated to `plan-first` on load, and a project
configuration that still lists it keeps loading with the entry ignored. Custom workflows
may still declare the extended integration, change-request, live-test, and release gates.
The CLI rejects a task role or mutation that the selected workflow does not declare.

Task retirement is durable: an orchestrator can mark work `cancelled`,
`abandoned`, or `superseded` with a reason. Retired tasks remain in history and do not
block workflow advancement; retry remains the operation for starting a new attempt.

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

`workspace project init` prepares the project. On a fresh project with a terminal it
runs an interactive setup: it lists the detected client adapters, requires
account-specific orchestrator provider and model identifiers, optionally proposes the
README role profiles, workflow mappings, and a GitHub forge, and writes nothing until
the final confirmation. Non-terminal and scripted runs skip the prompts and write the
minimal generated configuration. Re-initialization never rewrites existing
configuration, and generated launch and resume commands never include approval-bypass
flags; a user who wants auto-approval adds it deliberately.

Native OpenCode delivery is visible in the active parent TUI: the Run-scoped loopback
server selects the current session, appends the marker-bearing workspace prompt, and
submits it through the active TUI control API. Delivery is confirmed against the
current Run only after the marker appears in session history; it remains distinct from
inbox ACK and handoff acceptance.
The TUI and CLI show the concrete live Run state together with the latest client
observation, without treating a client `idle` observation as resumability or releasing
runtime ownership.

Messages and handoffs use exact logical Session addresses. `ToAgent` remains an audit
and legacy projection, while `--to` is accepted only when it resolves to one eligible
Session; resumable idle Sessions remain pending and closed or deleted Sessions are never silently
rerouted. Agents inspect, acknowledge, and review only their current Session, while the
user selects a Session explicitly or requests an agent-wide historical view explicitly.

In the project picker, the user can create a workspace with a named workflow (the form
preselects `plan-first`) or create a manual workspace without a workflow. The modes are
distinguished by the phase label: `manual` for manual orchestration and the workflow
phase otherwise; a legacy workspace still waiting in `needs_workflow` shows `-`. A manual
workspace is completed by the explicit, user-confirmed `complete` operation, after which
archive does not require a release; a plan-first workflow is landed and then completed the
same way, while a custom workflow that declares a release gate still requires a confirmed
release before archive. Alternatively, the user can
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
without secondary tabs, with a current/history filter and entry into session details;
`g` on a jump-capable selection moves an attached tmux client to the verified target
socket: with exactly one attached client it jumps immediately, and with several it opens
a picker. More provides Needs attention and Recent recorded activity among other pages.
The picker identifies each client by TTY and current session. Either path moves only the
confirmed client to the exact verified workspace, window, and pane. The TUI remembers the last successful client in ignored, project-local
`.workspace` UI data scoped by tmux socket, marks and preselects it while it remains
attached, and uses a deterministic live-client default otherwise. Opening a task shows
its related sessions, worktrees, and results and distinguishes process execution from
result acceptance. Starting or resuming remains an explicit action in the `a` menu. If
a pane is missing during navigation, the TUI proposes a confirmed reconcile and retries
the same selected entity and client; a disconnected client must be chosen again.

At project scope, the TUI exposes Issues and Dispatcher alongside Workspaces. An Issue
detail shows its source, retrieved revision, status reason, linked Workspaces, and the
guarded action to create a Workspace from that exact snapshot. Dispatcher start, status,
stop, and attach actions are project-scoped; Dispatcher text is treated as untrusted
input and cannot expand project mutation authority.

Worktrees appear as a revision tree, with source revisions and related task names.
Creation records the parent when the selected local branch identifies a registered
worktree, or an explicit revision matches exactly one worktree tip outside the frozen
workspace base. Later branch changes do not rewrite this relationship. Older records
and ambiguous sources show their base revision without inventing task dependencies;
revision ancestry is separate from a task's declared `depends_on` ordering.

The TUI can run manually as a browser or, after an explicit user action, as a managed
pane next to the orchestrator. The supervisor does not start, restore, or stop the TUI;
`workspace tui show` and `workspace tui hide` own that lifecycle, while `q` in the pane
records hide before exiting. During pause and completed, the pane may remain available
for review until the user closes it. `g` jumps immediately with the only attached client
and otherwise presents the attached-client picker for the selected target's tmux socket,
inside or outside tmux. The TUI rechecks the socket,
the target's ownership, and the selected client's live identity before switching; it
does not attach the caller's terminal or silently choose another client. `Esc` and
`Ctrl+C` cancel without a tmux effect. A different socket from the TUI's attached server,
an empty client list, or a client that detached or restarted is reported clearly. The
CLI's `workspace attach` behavior remains unchanged. Without tmux, runtime navigation
is unavailable, but the persisted workspace state remains available. The screen and
shortcut contract is described in
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

For source development, `scripts/setup-dev.sh` is the separate checkout workflow. It
requires Go 1.24 or newer on Linux/WSL or macOS, atomically refreshes an ignored
`bin/workspace` artifact, and installs a protected symlink from the user's development
bin directory to that artifact. It never silently replaces an unrelated command entry;
the build and command-install directories can be overridden with
`WORKSPACE_DEV_BUILD_DIR` and `WORKSPACE_DEV_INSTALL_DIR`. This workflow does not
change release archive, installer, or upgrade behavior.

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
