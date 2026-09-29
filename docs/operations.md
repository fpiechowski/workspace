# Operation Retries

Use `--operation-key` as the identifier for one logical mutation. Repeat the same
key when a response is lost. Changed arguments or input-file content with the same
key produce `operation_conflict`. A new decision, corrected payload, or another
attempt at the work requires a new key.

Workspace mutations include creating resources and tasks, starting/resuming/stopping
sessions, messages and ACKs, handoffs and their evaluation, workflows, state updates,
migrations, integration prepare and `integration land`, CRs, decisions, release,
`complete` for a manual workspace or a landed plan-first workflow, `issue-evaluation
record` for a completed linked workspace, pause/resume, reconcile, `reopen` for a
completed workspace, archive, clean, autonomy enable/disable/report, and the optional
rationale/evidence payload that records an autonomous gate decision.
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

`workspace create`, `issue dispatch`, `agent create`, `task create`, and `session start`
accept `--id` to choose the new resource's ID. The explicit ID is part of the operation
payload, so replaying the key with a different `--id` returns `operation_conflict`. A
keyed retry after an uncertain create returns the same allocated ID: an auto-generated
slug is reserved before the entity is written, and the retry reuses its own reservation.
An invalid ID shape returns `invalid_id`; an ID that is already taken returns `id_exists`
instead of gaining a suffix. Both fail with a non-zero exit.

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

## Integration landing

`workspace integration land` is a receipted mutation with a local Git effect, analogous to
change-request publication. The `--operation-key` digest covers the target and the
expected revision. The operation records a `pending` landing intent under the key before it
changes any ref, then commits the `landed` state with the target, the before/after
commits, and the user-confirmation attestation. A checked-out clean target is fast-forwarded
with `git merge --ff-only`; an unchecked-out target is compare-and-swap updated with
`git update-ref refs/heads/<target> <after> <before>`. A retry with the same key, or a new
key against the same state, finds the target already at the accepted commit and completes
the record without a second Git effect. A target that moved to anything other than the
recorded before or after commit is refused with `target_moved`, and a dirty checked-out
target with `target_checkout_dirty`; no ref changes in either case. Landing never pushes.
After landing, task retry and input/workflow revision are refused until `workspace reopen`.

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

## Autonomous runs

`workspace autonomy enable|disable` and `workspace autonomy report` are receipted
mutations, as is `workspace decision record` for an assumption or a worker-question answer.
`enable` is terminal-user only (an agent is refused because enabling hands over the user's
gates); `disable` accepts the user or an orchestrator with `--user-confirmed`; `report` and
`decision record` are orchestrator-only while the run is running. Each uses its
`--operation-key` and replays idempotently, and the revision-guarded ones require the exact
`--expected-revision`.

While a run is running, the orchestrator resolves the orchestrator-level gates (plan
acceptance, task and integrator result acceptance, phase advance, retry/retire) with an
optional `--rationale` and `--evidence`. The rationale is mandatory for an agent actor while
the run is running (otherwise `rationale_required`); the gate and an audit `Decision`
(`resolved_by=orchestrator`, `autonomous=true`, `subject`, `evidence`, Run provenance)
commit in the same write-ahead mutation as the change, so a validation error persists
neither. The rationale is added to the receipt payload only when non-empty, so an interactive
operation's existing digest and replay are unchanged.

Operations with external, local-branch, lifecycle-terminal, or authorization effects are
excluded from autonomous resolution. While the run is running, an agent actor receives
`autonomy_excluded` for them even with `--user-confirmed` (or `publication: allowed`):
`integration land`, `complete`, `change-request publish`/`resolve`, `release confirm`,
`decision answer` for live testing, `reopen`, `archive`/`clean`/delete, and `state edit`. The
terminal user (empty actor) can always perform them, and after `delivered` or `disabled` the
ordinary attestation contract applies again. The run itself ends with `autonomy report`, which
validates the outcome against state, stores an immutable summary artifact, appends an
`autonomous.final_report` decision, and sets `autonomy.state=delivered`.

## Completed workspaces and reopen

`completed` is intentionally not an alias for `paused`. `workspace resume` cannot
reactivate it, and ordinary task/resource/result operations return a precise
`workspace_completed` error; the corresponding terminal error for an archived workspace
is `workspace_archived`. Explicit `workspace start` or `agent resume orchestrator` is
conversation-only and may reuse the compatible logical Session. An existing accepted
worker Session may also be resumed for consultation, but it cannot claim or submit new
work. Exact Session-addressed messages remain deliverable while the conversation Run is
active. The supervisor does not restart completed Sessions automatically.
`workspace complete` tolerates the orchestrator's own running Session and does not stop
it; that Run does not become `conversation_only`, cannot create work (status guards), and
must be stopped before `archive`.

`workspace reopen` is a distinct mutation. It requires a non-empty reason and the exact
current `--expected-revision`; an agent actor must also pass `--user-confirmed`. The
operation refuses active worker Sessions and services, records the previous
`WORKSPACE.md`, reason, status, revision, and base commit under
`history/reopen_ID/`, and atomically changes the workspace to active. Tasks, artifacts,
handoffs, and base provenance remain; integration, live-test, release, pending-decision,
and change-request state is invalidated or marked outdated. Repeating the same
operation key and payload replays the original status; changing the reason or revision
with that key returns `operation_conflict`. Archived workspaces cannot be reopened.

## Automatic Issue evaluation

Completing a linked Workspace records a `pending` `issue_evaluation` in the same
`WORKSPACE.md` revision and then tries to start a conversation-only orchestrator Run
under the stable key `issue-evaluation:<ieval id>`. The launch is a post-commit side
effect, not part of the completion receipt: a failure never rolls back the committed
completion and is stored as `issue_evaluation.launch_error`, which `status` and `menu`
surface. `workspace start`, or a replay of `complete`, retries it only while no
orchestrator Session is active, so an active or replayed evaluation never creates a
second Run.

`workspace issue-evaluation record --outcome delivered|not_delivered --reason ...
--expected-revision N` runs under the project lock and writes two files in one critical
section: the linked `ISSUE.md`, through the lock-free `updateIssueLocked` helper (so the
non-reentrant project lock is not taken twice), and the Workspace with the `recorded`
evaluation. The Issue write carries its own receipt key
`issue-evaluation:<ws_id>:<ieval_id>`, independent of the caller's `--operation-key`.
The same key and payload returns the preserved result without advancing the Issue
revision; the same key with a different payload returns `operation_conflict`. A crash
after the Issue write but before the Workspace save converges when the same key is
retried: the Issue receipt replays, then the evaluation is recorded. A different
evaluation is refused with `issue_evaluation_recorded`; `workspace reopen` clears it and
the next completion creates a new `ieval` id. A `delivered` judgement whose Issue digest
no longer matches the frozen snapshot is refused with `issue_revised`. None of these
writes leave the local project or touch the external tracker.

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
guard, and preserves the existing lifecycle gates. `complete` for a manual workspace or a
landed plan-first workflow is a separate idempotent mutation with a revision guard and
operation key; `integration land` is a plain confirmed action in the integration phase;
archive respects a confirmed release for a workflow that declares a release gate, or an
earlier `complete` for a manual or plan-first workspace. Reopen
has its own revision/reason guard and never silently reuses release evidence.
