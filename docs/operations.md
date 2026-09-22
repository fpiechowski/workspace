# Operation Retries

Use `--operation-key` as the identifier for one logical mutation. Repeat the same
key when a response is lost. Changed arguments or input-file content with the same
key produce `operation_conflict`. A new decision, corrected payload, or another
attempt at the work requires a new key.

Workspace mutations include creating resources and tasks, starting/resuming/stopping
sessions, messages and ACKs, handoffs and their evaluation, workflows, state updates,
migrations, integration, CRs, decisions, release, `complete` for a manual workspace,
pause/resume, reconcile, `reopen` for a completed workspace, archive, and clean.
`project init`, `skill install`, and `server stop` write project-scoped receipts.
Reads, `clean --dry-run`, interactive attach, and the continuously running `serve`
command are not one-shot mutations that require a receipt.

Project Issue intake, refresh, local status updates, linked Workspace creation, and
Dispatcher start/stop use the same retry contract. Issue receipts live under
`.workspace/issues/.runtime/operations/`; Dispatcher receipts are stored in
`.workspace/dispatcher/state.json`. A linked Workspace records the selected Issue
revision and digest, so replaying or refreshing the Issue cannot alter that input.
Project reads (`issue list/show`, `dispatcher status`, and the project TUI overview) do
not create Issue or Dispatcher stores, and a fresh project does not gain a runtime lock
directory merely from those reads.

A successful retry returns the preserved result. It may describe an older state: retrying
`session start` after the session was stopped returns the earlier response and does not
start it again. Read the current state through `status`, `session list`, or the relevant
`list` command. Resource identifiers and the operation number remain stable.

Mutation JSON with a key contains `operation_id` and, for a workspace, the operation
`revision`. This revision refers to the persisted result. `data.workspace.revision` in
the `status` response is the current revision for the next
`state update --expected-revision`. A project has no shared WORKSPACE.md revision, so its receipts do
not contain this field.

The document change and its result are committed together through a write-ahead record.
A validation error does not persist a partial update. Concurrent calls with the same key
do not perform the change twice. Role authorization is also checked when reading a
persisted result.

External operations record their intent before invoking a process or network and use a
separate operation lock. After a failure, the result remains uncertain until reconciled:
the PR is looked up before another publication attempt; the session keeps its reserved
identity; worktree removal is reconciled with Git. A busy lock does not mean that the
previous execution has finished. A historical receipt does not replace a `reconcile`
command for reading the current runtime state.

Data from an early registry version without a saved response snapshot retains a reference
to the created resource. Replaying it may show the resource's current state.

Message and handoff operation digests include their exact `ToSession` address. The
compatibility `--to` form is resolved to one eligible open Session before the mutation
is recorded; it never selects the latest Session and an idle target remains pending.
Replies preserve the original `FromSession`. A notification receipt is not a delivery
receipt, so `NotifiedSessionID` never makes a message delivered or acknowledged.

The runtime registry is currently schema version 5. Reads apply staged migrations before
returning state: parent Session links and message/handoff targets are backfilled only
from strong evidence, while ambiguous legacy records retain an empty `ToSession`.
OpenCode snapshots are normalized by adapter plus empty custom `deliver_argv`; missing
native Run endpoints are restored only from valid loopback server flags in immutable argv.

## Completed workspaces and reopen

`completed` is intentionally not an alias for `paused`. `workspace resume` cannot
reactivate it, and ordinary task/resource/result operations return a precise
`workspace_completed` error; the corresponding terminal error for an archived workspace
is `workspace_archived`. Explicit `workspace start` or `agent resume orchestrator` is
conversation-only and may reuse the compatible logical Session. An existing accepted
worker Session may also be resumed for consultation, but it cannot claim or submit new
work. Exact Session-addressed messages remain deliverable while the conversation Run is
active. The supervisor does not restart completed Sessions automatically.

`workspace reopen` is a distinct mutation. It requires a non-empty reason and the exact
current `--expected-revision`; an agent actor must also pass `--user-confirmed`. The
operation refuses active worker Sessions and services, records the previous
`WORKSPACE.md`, reason, status, revision, and base commit under
`history/reopen_ID/`, and atomically changes the workspace to active. Tasks, artifacts,
handoffs, and base provenance remain; integration, live-test, release, pending-decision,
and change-request state is invalidated or marked outdated. Repeating the same
operation key and payload replays the original status; changing the reason or revision
with that key returns `operation_conflict`. Archived workspaces cannot be reopened.

## TUI Operations

Mutations started from the TUI use the same core use cases as the CLI. Confirmation
captures the workspace revision, task attempt, or exact current Run as a guard; “Pause
and interrupt” records and compares the exact set of active Runs and services before
stopping any process. A target change after opening the form ends in a conflict instead
of performing the action against the new state. Task retry shows dependent tasks and
requires a reason.

Each confirmed operation receives a `tui_<ULID>` key. A retry after an uncertain response
uses the same key and payload; double confirmation does not perform the mutation again.
`tui show` and `tui hide` write separate receipts to `.runtime/ui.json`, not to the
Session/Run registry. `q` in a managed panel records the same durable hide intent before
restoring the terminal and ending the process. The managed pane is user-owned; the
supervisor does not restore it after an external kill or process exit.

Delete in the TUI requires retyping the full ID. A Task and Session are logically deleted
through an auditable tombstone; the receipt remains in the workspace registry, and the
guard covers the revision and, respectively, the attempt or last Run. Physical Delete
Workspace uses a project-scoped receipt outside the target directory, so the same key
and payload can safely be retried after the directory is removed. It does not require
archive/release: it stops the runtime, forcibly removes worktrees (including dirty ones)
and local `workspace/<id>/…` branches, and then removes all state. A revision or target
change ends in a conflict. Archive from the TUI is non-destructive, has its own revision
guard, and preserves the existing lifecycle gates. `complete` for a manual workspace is
a separate idempotent mutation with a revision guard and operation key; archive respects
the confirmed release for a workflow or an earlier `complete` for manual mode. Reopen
has its own revision/reason guard and never silently reuses release evidence.
