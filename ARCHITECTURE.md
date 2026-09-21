# `workspace` Architecture

## System Context

`workspace` is a single Go program run locally in a Git repository. The CLI invokes
domain logic that persists state in files and controls external processes: Git, tmux,
agent clients, and an optional forge or tracker adapter. The project supervisor runs
locally and reconciles persisted state with the runtime; it does not make workflow
decisions.

```text
user / agent
        │
        ▼
   CLI (`cmd/workspace`, `internal/cli`)
        ├── bootstrap (flag/env/CWD → scope)
        ├── TUI (`internal/tui`)
        └── terminal (attach/switch/jump)
                  │
                  ▼
   domain and use cases (`internal/core`)
        │
        ├── state files, locks, and write-ahead records
        ├── Git worktrees and commits
        ├── tmux, client processes, and supervisor
        └── client, tracker, and forge adapters
```

Session runtime requires Linux, macOS, or Linux in WSL because it relies on tmux. The
CLI core also builds and is tested on Windows; process-dependent code has
platform-specific implementations.

Published distribution has a separate boundary from project/workspace state. GitHub
Releases for `fpiechowski/workspace` are canonical and contain versioned archives for
Linux amd64/arm64 and macOS amd64/arm64 plus `checksums.txt`; native Windows runtime
is intentionally outside the release contract and uses the Linux build in WSL.

## Code Organization

- `cmd/workspace/` — binary entry point.
- `internal/cli/` — Cobra command tree, flags, help, and response serialization.
- `internal/bootstrap/` — shared project/workspace, Service, and actor resolution from
  flags, the environment, and the CWD.
- `internal/tui/` — Bubble Tea model, routes, views, Huh forms, and theme; no direct
  writes to domain files.
- `internal/terminal/` — verified tmux client switching or attach with stdio passing;
  it does not accept raw target strings from the interface.
- `internal/core/` — domain model, use cases, persistence, and process adapters.
- `internal/core/templates/` — built-in templates installed by `project init`.
- `internal/core/skill/workspace/SKILL.md` — binary-owned general agent guidance,
  embedded for both skill installation and the project-independent `prime` command.
- `internal/buildinfo/` — link-time version, commit, build-date and runtime metadata.
- `internal/release/` — public repository identity, supported targets and asset naming.
- `internal/upgrade/` — project-independent GitHub Release discovery, checksum/archive
  validation, locking and atomic executable replacement.
- `.workspace/templates/` — project-owned, editable copies of templates and workflow.
- `scripts/` — POSIX release installer, development setup, release build, installation
  checks and fixture tools.
- `docs/` — narrower operational contracts referenced by this document and the README.

The `core` package is intentionally cohesive and file-oriented rather than splitting
each domain into a separate package. Responsibility boundaries are defined by files
(`project`, `workflow`, `session`, `runtime`, `routing`, `mailbox`, `handoff`, `forge`,
and others) and by external adapter interfaces.

### Build, release, and upgrade boundary

Source builds default to `dev`, `unknown` commit and `unknown` build date. Release builds
set those values with `-ldflags -X`, use `CGO_ENABLED=0` and `-trimpath`, and package one
root `workspace` executable per supported target. The tag-triggered workflow validates a
stable tag on `master`, runs the repository checks, builds the archives and checksums,
then publishes a draft GitHub Release only after all five expected assets are attached.

`scripts/setup-dev.sh` is the boundary for a persistent checkout build. It validates a
Linux/WSL or macOS host and Go 1.24+, builds with `CGO_ENABLED=0 go build -trimpath
./cmd/workspace` into a temporary sibling under the ignored checkout `bin/` directory
by default, and atomically renames the result to `bin/workspace`. It then creates or
reuses a protected symlink in the user's development bin directory. The build and
install directories are independently overrideable with `WORKSPACE_DEV_BUILD_DIR` and
`WORKSPACE_DEV_INSTALL_DIR`; an unrelated regular file or symlink at the command path
is an error. This setup is local-only and does not publish metadata or release assets.

`workspace upgrade` runs entirely outside bootstrap and the domain service: it does not
resolve a project, workspace, registry, or tmux session. It queries the latest stable
GitHub Release with bounded HTTP requests, selects the exact runtime archive, verifies
the SHA-256 manifest entry, rejects unsafe tar layouts, stages the executable in its
destination directory, syncs it, and atomically renames it into place. A lock serializes
concurrent upgrades. Any network, validation, permission, or rename failure occurs
before replacement and leaves the current executable untouched. A symlinked launcher is
resolved to its target when possible. The updater never invokes `sudo` or changes
`PATH`; already-running supervisors and tmux processes retain their old in-memory code
until restarted.

Because upgrade resolves symlinked launchers, running it through a development command
can replace the resolved development artifact with a release binary. Development rebuilds
must use `scripts/setup-dev.sh`; release installation remains the supported way to switch
back to a release command.

### Project-independent bundled guidance

`workspace prime` is a read-only root command that also runs outside bootstrap and the
domain service. `internal/core/skill.go` reads the embedded
`internal/core/skill/workspace/SKILL.md` bytes through one shared accessor: skill
installation preserves the complete file and its overwrite protection, while
`PrimeInstructions` validates the leading YAML front-matter boundary and returns only
the Markdown body. Normal `prime` output is that body directly with its final newline;
`prime --json` uses the standard `{ok:true,data:{instructions:"..."}}` envelope. The
command never reads mutable installed skills, project files, prompts, memories, or live
workspace state, and never starts runtime processes or records an operation. Agents use
`workspace status` and `workspace menu` for live state and actions; the embedded guide
is the current binary's general context-recovery input rather than a workspace snapshot.

### TUI health queries

The TUI obtains supervisor health through a project-scoped `SupervisorObservation`
query. Project-picker refresh requests the project overview and this observation as
independent asynchronous reads; workspace refresh composes the supervisor fields from
the existing runtime observation instead of pinging the supervisor a second time.
These reads classify runtime failures for display, do not persist observations, and
have no start, stop, or reconcile side effects.

## Domain Model

| Object | Responsibility |
|---|---|
| Project | Git repository; configuration for clients, profiles, forge, tracker, and workflow. |
| Workspace | Persistent context for one initiative from input to confirmed release (workflow) or confirmed completion (manual mode); completed work can be reopened only through an explicit guarded mutation. |
| Worktree | Isolated checkout and branch for planning, implementation, integration, or testing. |
| Task | Delegated unit of work with an attempt, dependencies, and acceptance criteria. |
| Agent | Stable persona definition: role, instructions, prompt, and profile. |
| Session | Logical conversation context for one agent and task lineage. |
| Run | A specific client and pane execution with routing and process result; a `conversation_only` Run can inspect and discuss completed work without performing task execution. |
| Message | Durable message addressed to an exact logical Session; `ToAgent` is retained as ownership and legacy projection. |
| Handoff | Submission of a task result for separate evaluation by the exact recipient Session. |
| Artifact | Immutable copy of a work product with digest and provenance. |
| Check | Receipt for an actually executed verification command. |
| Change request | Locally prepared or published PR/MR associated with a code revision. |

A Project contains multiple workspaces. A Workspace contains tasks, worktrees, and
agents. An Agent may have multiple logical Sessions, and each Session may have multiple
successive Runs. A worker Session records the exact parent Session that delegated it;
the parent Agent ID remains an ownership and compatibility projection. Results identify
the task, attempt, Session, exact Run, and Git revision. Identifiers are immutable; names
are used only for presentation and resource selection.

Deleting a Task or Session from the TUI is logical: the record receives `deleted_at` and
remains in persisted state as a tombstone for receipts and historical references, but
disappears from normal TUI collections and progress counters. Core rejects tombstoning
an active Session, an accepted or dependent Task, or records with persisted results.
Deleting an entire workspace is a physical project operation with a receipt outside the
directory being deleted. It is an explicit discard independent of archive/release: it
stops the runtime session, forces removal of the workspace's worktrees and local
branches, and then removes the entire state directory. Paths and branch prefixes are
checked before destructive effects.

Key invariants:

- only one Run for a Session may be active;
- the orchestrator has one active execution line protected by a generation;
- at most one writer managed by `workspace` may operate in a worktree;
- a process exit code does not automatically accept a task result;
- message ACK and handoff acceptance are separate operations;
- new input, a task attempt, or incompatible lineage does not resume an old Session;
- live test and release refer to a specific SHA;
- completed workspaces preserve task/result history and allow only conversation, archive,
  or an explicitly authorized reopen; archived workspaces are terminal;
- a completed conversation Run cannot claim work, submit or review results, create
  resources, or advance release state.

## Persisted State

The project stores versionable configuration and templates under `.workspace/`. By
default, local workspaces also live in `.workspace/ws_*`, but `workspaces_dir` may point
to an external directory. Local Git rules ignore runtime data and work products without
hiding configuration or templates.

In each workspace:

```text
ws_ID/
├── WORKSPACE.md          # canonical mode, workflow, and narrative state
├── WORKFLOW.md           # selected workflow snapshot or manual-mode note
├── AGENTS.md             # orchestrator role instructions
├── inputs/               # input snapshot
├── prompts/              # frozen prompts and executor instructions
├── tasks/                # task specifications
├── artifacts/            # immutable copies of submitted products and evidence
├── worktrees/            # Git checkouts when storage is internal
└── .runtime/
    ├── index.json        # operation, Session, and Run registries
    ├── ui.json           # separate desired state, receipts, and TUI generation
    ├── pending.json      # interrupted-write intent
    └── ...               # inbox, prompts, receipts, and supervisor state
```

An explicit reopen writes the pre-reopen `WORKSPACE.md`, authorization reason, and
previous status/revision/base metadata under `history/reopen_ID/` in the same
write-ahead mutation as the new active state. It preserves tasks, artifacts, handoffs,
and the base commit, but invalidates integration, live-test, release, pending-decision,
and change-request state that depended on the completed cycle.

`WORKSPACE.md` has validated frontmatter and descriptive content for the next
orchestrator. It is the source of truth for the mode, workflow phase (when selected),
tasks, decisions, and accepted results.
`.runtime/index.json` is an operational index, not a competing workflow version. Public
`status --json` has its own explicit schema number. The TUI uses a private
`WorkspaceSnapshot`; it does not add entities to `index.json`, change the public Status
schema, or persist a Session/Run for its own process.
The private registry uses staged migrations: when opened, older versions pass through
the required stages in order and are written atomically. Registry schema 5 adds exact
Session targets and parent-Session lineage, then normalizes every OpenCode snapshot
using the adapter-plus-empty-`deliver_argv` predicate. Migration backfills only targets
with strong delivery/provenance evidence and preserves ambiguity as legacy records;
Run provenance remains unchanged. Missing native OpenCode endpoints are derived only
from valid loopback `--hostname`/`--port` flags in immutable Run argv.

Templates are copied into a workspace when it is created. A later change to the project
template does not change work in progress. An explicit migration preserves previous
files, hashes, and history and invalidates dependent results. Details are described in
[docs/revisions.md](docs/revisions.md).

## Mutations, Concurrency, and Recovery

An operation that changes state runs under the project lock: it reads and validates the
document, checks the role and expected revision, records its intent, prepares temporary
files, performs an atomic replacement, and persists a receipt. An interrupted write is
completed or reconciled on the next read.

`--operation-key` identifies a logical mutation. The digest covers its payload, so a
retry returns the preserved result, while using the same key with different content ends
in a conflict. Git, tmux, and external-service effects cannot be covered by a file
transaction; their intent and state are recorded before invocation, and an uncertain
result is checked before retry. The full contract is in
[docs/operations.md](docs/operations.md).

`workspace reopen` is a user- or orchestrator-authorized mutation, not an implicit form
of `workspace resume`. It requires the exact current revision and a non-empty reason;
agent actors must carry an explicit user-confirmation flag. The operation refuses active
worker Sessions and services, preserves immutable result provenance, and is idempotent
under its operation key. `workspace resume` remains limited to paused workspaces.

Destructive TUI actions additionally require retyping the full ID. Delete Task/Session
uses revision and attempt/last-Run guards, while Delete Workspace writes an idempotent
receipt to the project operation registry, stops the runtime, and removes worktrees and
branches before the directory. A retry after physical deletion replays the result from
the project-scoped receipt. Archive from the TUI has a separate revision guard and does
not delete data.

Role control, read-only mode, and worktree leasing are a contract among cooperating
processes running under one system account, not an operating-system security boundary.

## Process Runtime

Each workspace receives a tmux session. Worktrees are windows, specific agent Runs are
panes, and the orchestrator has a separate window started from the workspace directory.
Supporting services are a separate record type and do not inherit agent identity.

The runner receives, among other values, `WORKSPACE_AGENT_ID`, `WORKSPACE_SESSION_ID`,
`WORKSPACE_RUN_ID`, `WORKSPACE_TASK_ID`, `WORKSPACE_ROLE`, and the identifiers of its
parent Agent, exact `WORKSPACE_PARENT_SESSION_ID`, orchestrator, and worktree. The Run
owns the current pane; a recycled tmux identifier does not give an old process the right
to mutate state.

The supervisor compares active reservations with actual panes, delivers messages, and
detects lost processes. It may resume a lost orchestrator in a compatible Session —
including in active manual mode — but it does not blindly restart executors that were
explicitly stopped or ended with an error, and it never auto-restarts an orchestrator
merely because a workspace is completed. An explicit start/resume after completion
creates a `conversation_only` Run in the compatible orchestrator Session; archived
workspaces are not started. Detailed states and procedures are described
[docs/runtime.md](docs/runtime.md).

The managed TUI is a separate runtime ownership type recorded in the workspace
`.runtime/ui.json`. It is user-operated: `workspace tui show` explicitly creates or
restores at most one pane next to the orchestrator without changing focus, while the
supervisor and workspace start do not create or restore it. The launch token and
generation are claimed by the exact pane; explicit UI operations remove only panes
whose identity is confirmed by metadata or a specific runner command. The UI does not
count as a writer, service, or active Run and does not block cleanup. The full contract
is described in [docs/runtime.md](docs/runtime.md).

## Client Adapters and Routing

The client adapter separates five capabilities: launch, resume, deliver, observe, and
interrupt.
Native OpenCode delivery is Run-scoped: its loopback endpoint and delivery phases are
transport state, not an inbox ACK or handoff acceptance. The supervisor uses the
endpoint's active TUI control API (`select-session`, `append-prompt`, and
`submit-prompt`) and confirms the marker in bounded recent-history reads; it does not
use the external `prompt_async` route or a detached client. Append, submitted and
uncertain phases are durable so a retry never blindly appends the same prompt. A
transport failure remains undelivered and produces one Run-scoped tmux inbox
notification as a fallback. Generic `deliver_argv` remains available to command/Claude
adapters.
Process arguments are argv arrays without shell interpolation. Built-in adapters support
Codex, Claude, and OpenCode; the `command` adapter allows custom wrappers. A native
conversation ID is an optional Session binding, never its identity or inbox address.
Details and the wrapper format are described in [docs/clients.md](docs/clients.md).

The profile contains client/provider/model routes, required capabilities, and an optional
reasoning-effort setting applied to every route in that profile. The router rejects
unavailable routes, routes in cooldown, and routes over their limits. It then balances
providers based on local launches and reservations in a 24-hour window and records the
decision plus the selected setting in the Run. These counters approximate local project
load; they do not measure tokens, cost, or limits for the entire account. A resumed
logical Session keeps the original Run setting even if project configuration changes;
a new logical Session resolves the current profile setting.

## Workflow and Delegation

The workflow is a versioned template describing phases, entry conditions, required
results, and recovery paths. The CLI enforces transitions, roles, dependencies, and
acceptance; `WORKFLOW.md` instructs the orchestrator how to compose these operations
idempotently.

The basic `plan-first` workflow leads from planning to implementation. The extended
`issue-resolution` workflow, retained for compatibility and the complete process,
includes:

```text
planning → plan review → implementation → integration → change requests
         → live-testing offer → live test or explicit skip
         → wait for release → confirmed completion
```

`paused`, `blocked`, and `needs_attention` are operational states independent of the
phase. Changing input or retrying increments the attempt and invalidates dependent
results without deleting history. Integration takes place in a dedicated worktree with
an input-SHA manifest. An exact resume of the same idle Session for an unchanged attempt
and input lineage is allowed even during `awaiting_review`: it creates a new Run but does
not reopen the Task or replace the `RunID` that points to the pending handoff. Task retry
remains a separate operation. Only rejecting the handoff may rebind `RunID` to a newer,
compatible active Run of that Session so that Run can submit a replacement result.

### Manual Mode

A workspace does not have to select a workflow. Creation accepts one of three explicit
forms: `--workflow NAME` (active workflow), no flag (the `needs_workflow` state waiting
for a selection), or `--no-workflow` (active manual mode). Combining `--workflow` with
`--no-workflow` is rejected as `invalid_option`. Manual mode stores `status: active`
with an empty `workflow`, which distinguishes it from `needs_workflow`; `WORKFLOW.md`
remains an explicitly marked note about manual orchestration, without a phase or
workflow-template digest.

In manual mode, the orchestrator may create agents, tasks, and worktrees and start
worker sessions with the same task, worktree, actor, and provenance gates as in a
workflow. A fixed limit of 3 parallel workers applies (the shared
`defaultMaxParallelTasks` fallback), so delegation never becomes unbounded. Operations
reserved for workflows — `workflow advance`, `workflow select`, `workflow migrate`,
release confirmation, decisions, integration, change requests, and live testing —
return `operation_not_applicable` in manual mode and do not appear in the menu; there
is also no conversion from a manual workspace to a workflow. Completion is a separate,
idempotent `workspace complete` mutation, available after user confirmation and only
when there are no active sessions, services, or unaccepted tasks; only that operation
allows a manual workspace to be archived without a release. Accepting all tasks, a
process exit, or handoff acceptance alone does not complete a manual workspace.

### Completed conversation and reopen

`completed` is a durable result state, not a paused state. `workspace start` and
`agent resume orchestrator` reuse a compatible idle logical Session when possible and
create a new `Run` marked `conversation_only`; Codex/native delivery and exact
Session-addressed messages remain available. Existing accepted worker Sessions may be
resumed for consultation with the same task attempt, worktree, input digest, and base
lineage, but they cannot submit a new result. New workers, tasks, worktrees, services,
checks, handoffs, integration, change requests, decisions, release, and ordinary state
mutations are rejected with a precise completed/archived error.

`workspace reopen --reason ... --expected-revision ...` is the only normal transition
from completed to active. It is guarded by the current revision, records the user's
authorization, preserves accepted history and base provenance, and starts a fresh
planning/release cycle by invalidating derived completion state. The supervisor does
not infer reopen from process exit or from a conversation message.

## Communication, Artifacts, and Evidence

Messages first go to a durable inbox addressed to one logical Session. An adapter with
`deliver` may wake that Session's current Run; an idle target remains pending and a
closed/deleted target is never rerouted. `--to` and Agent-only legacy records are
accepted only through an unambiguous compatibility path; the resolver never means
"latest". Agent processes read, ACK and review only their exact current Session, while
the user can select a historical Session or explicitly request an agent-wide view.
Delivery is at-least-once and is deduplicated by ID; replies use the original
`FromSession`.

`handoff submit` validates explicitly selected files, copies them to artifacts, computes
hashes, and records provenance before sending the message. It does not automatically
archive the entire checkout. An acceptable implementation identifies a clean commit
and the required evidence. `check run` captures the actual argv, exit code, output
digest, and SHA; success of the CLI command itself means that the receipt was written,
not that the executed test succeeded. The format and limits are described in
[docs/checks.md](docs/checks.md).

## External Integrations

The tracker fetches an issue snapshot through `gh`, `glab`, or a configured command
adapter. It does not comment on or change the ticket state. The input contract is
described in
[docs/trackers.md](docs/trackers.md).

The forge prepares, publishes, finds, and synchronizes change requests for GitHub,
GitLab, or a custom adapter. Without an integration it creates a local CR package.
Publication respects the `ask`/`allowed` policy; an uncertain response is reconciled
through a lookup so that a duplicate is not created.

## Data Safety and Trust Boundaries

- Tracker input and task content are data, not instructions that elevate privileges.
- Artifact paths are restricted to the assigned worktree; traversal and escaping
  symlinks are rejected.
- Cleanup checks activity, local changes, and unpublished commits; an optional backup
  uses a verified Git bundle.
- Local files do not protect against another process run by the same user. The role
  model provides collaboration consistency, not isolation from malicious code.
- User confirmation is an auditable process contract, not cryptographic proof of human
  identity.

## Change Verification

The baseline quality set for Go code is `gofmt`, `go test ./...`, and `go vet ./...`.
Changes to processes, concurrency, or tmux integration require the full race/tmux test
on Linux or WSL. Installation or build changes additionally require building the binary
at a new location and running `scripts/check-install.py` according to
[README.md](README.md#verification).

Release validation additionally runs `scripts/test-install.py`, cross-builds the four
release targets and a non-published Windows portability binary, verifies
`checksums.txt`, and executes the packaged Linux amd64 binary's version/help commands.

Tests use isolated repositories, private tmux servers, and local client/forge fixtures.
Ordinary verification does not publish change requests or invoke paid model runs.

## Operational References

Documents in `docs/` are intentionally narrower than this architecture:

- [clients.md](docs/clients.md) — client adapter and wrapper capabilities;
- [runtime.md](docs/runtime.md) — Session/Run states, supervisor, services, and cleanup;
- [tui.md](docs/tui.md) — terminal interface screens, navigation, and actions;
- [operations.md](docs/operations.md) — idempotency, receipts, and uncertain effects;
- [revisions.md](docs/revisions.md) — input changes and workflow migration;
- [checks.md](docs/checks.md) — test-evidence capture and import;
- [trackers.md](docs/trackers.md) — issue fetching and snapshotting.
