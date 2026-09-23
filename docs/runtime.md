# Runtime and recovery

Each workspace owns a tmux session. Worktrees appear as windows; concrete agent Runs
appear as panes. A logical Session can remain idle/disconnected without a pane and can
accumulate many Runs. The orchestrator window runs from the workspace directory.
Auxiliary services use their own records and panes, not agent identities:

```sh
workspace service start app --worktree integration -- npm run dev
workspace service list
workspace service stop service_ID
```

The TUI `t` action may create a runtime-only tmux session-group viewer for this
canonical session. It shares the workspace windows and panes while retaining a separate
client, allowing a dedicated terminal to be reused without switching the TUI client.
Viewer groups are not durable Workspace records and do not represent Agents, Sessions,
Runs, services, or ownership. Closing or detaching the viewer leaves the canonical
workspace processes running. The `g` TUI action and `workspace attach` continue to use
the current client and caller-terminal attach paths respectively.

The orchestrator/user can manage services; workers can manage only services in their
assigned worktree. Active services block archive/cleanup and revision migration.
`pause --interrupt` stops them too. A lost service pane is marked interrupted rather
than restarted silently.
`start`, `session start` and `agent resume` start the project supervisor automatically.
`server status` inspects it; `server stop` stops supervision without killing agents.
`serve` runs supervision in the foreground.

## Project Dispatcher

The project-scoped Dispatcher is separate from every Workspace Agent. Its durable state
lives in `.workspace/dispatcher/state.json`; its stable Agent role is `dispatcher`, its
scope is `project`, and its tmux session is `workspace-dispatcher-<project-id>`. Configure
`defaults.dispatcher_profile` to select its model profile; when omitted, the configured
orchestrator profile is used.

```sh
workspace dispatcher start --operation-key dispatcher-1
workspace dispatcher status
workspace dispatcher stop --operation-key dispatcher-stop-1
workspace dispatcher attach
```

Dispatcher panes carry explicit project scope, project ID, Agent ID, role, Session ID,
and Run ID metadata. Status verifies those fields before exposing a pane as attachable or
eligible for recovery. The supervisor restarts only a verified lost Dispatcher pane;
normal client exit, explicit stop, a dead/unowned pane, or an observation error never
causes a blind duplicate launch. Workspace actors cannot mutate project Issues, and a
Dispatcher cannot use its project identity to perform arbitrary Workspace mutations.

The Dispatcher may intake, inspect, refresh, defer, reopen, or close local Issues, create
a Workspace from an exact Issue revision, and explicitly start that Workspace's
Orchestrator. Issue text is untrusted data and never grants permissions. The Dispatcher
does not implement code, create Tasks or Worktrees, accept results, publish changes, or
mutate the external tracker.

The supervisor reconciles durable state with actual pane ownership. A verified lost
orchestrator pane is marked as an interrupted Run and resumed in the same compatible
logical Session with a new Run. Recovery also applies to an active manually orchestrated
workspace, which is stored as `active` with no workflow, so losing its orchestrator pane
is not treated as a reason to stop. Transient
tmux errors preserve reservations. Explicitly stopped or failed clients are not
automatically restarted. Captured check processes lost with their Run are marked
interrupted. A paused workspace does not delegate or restart work automatically. A
completed workspace is also not restarted automatically: an explicit `workspace start`
or `agent resume orchestrator` creates a conversation-only Run, while an archived
workspace is terminal.

`pause` stops new delegation. `pause --interrupt` also stops active panes, preserving
their worktrees. `resume` permits delegation again; `agent resume NAME` starts the
chosen persona using its prior assignment. Generic clients receive a fresh bootstrap;
adapters with native thread support can reuse the conversation.
An exact resume of the same idle logical Session is also allowed while its Task is
`awaiting_review` when the attempt, input lineage and binding are unchanged. It creates
a new Run without reopening the Task or replacing the Run provenance of the submitted
handoff, so that handoff remains reviewable. If that handoff is rejected while the
successor Run is active, the Task can adopt that compatible Run for a non-stale
replacement; retrying the Task remains a separate operation.

Completed work has a separate continuation contract. `workspace resume` only changes a
paused workspace back to active. `workspace start` and `agent resume orchestrator` reuse
the compatible idle orchestrator Session where possible and mark the successor Run
`conversation_only`; the Codex app-server bridge continues polling so questions and
exact Session-addressed messages can be delivered without reopening task execution.
An existing accepted worker Session can be resumed for consultation only when its task,
attempt, worktree, input digest, and base lineage still match. The prompt explicitly
instructs that Run not to claim work or submit a result. New worker Sessions and all
ordinary resource/task/result mutations require `workspace reopen`.

| Session projection | Meaning |
|---|---|
| `lifecycle_state=active` | `current_run_id` points to the single starting/running Run; the Session owns its runtime and operational `state` follows that Run |
| `lifecycle_state=idle` | no active current Run; the Session can be resumed when lineage is unchanged |
| `lifecycle_state=closed` | explicitly closed logical context; no further Runs are allowed |
| `state=starting` / `state=running` | the current Run is starting/running; this remains the operational state regardless of the client observation |
| `client_state=idle` | the adapter's latest explicit observation; it does not release ownership or make a live Session idle |

`state` is the operational display and decision projection. It normally follows
`run_state`, including for a current starting/running Run whose adapter reports `idle`.
`client_state` remains a separate observation and never changes the concrete operational
Run state. The supervisor never infers idle from tmux focus, output silence, elapsed time,
or an observation failure. `run_state` remains the source of runtime ownership and
recovery decisions, and `Session.Active()` remains true for a current `starting` or
`running` Run.

| Run state | Meaning |
|---|---|
| `starting` | durable launch intent exists; pane identity may still be recovered |
| `running` | the runner claimed ownership |
| `exited` / `failed` | client exited normally / with an error |
| `stopped` | explicitly stopped; supervisor does not restart it |
| `interrupted` | pane was verified lost or superseded; Session stays resumable |

`client_state` records the latest adapter observation. Codex and native OpenCode may
report `busy`, `idle`, or `retry` (Codex may also expose other explicit bridge states such
as `needs_input`); unknown or empty values do not change the concrete operational Run
state. OpenCode observation polls the current Run's loopback `GET /session/status`
endpoint, matches the exact `client_thread_id`, and accepts only the documented `idle`,
`busy`, and `retry` status types. The bounded request runs outside the project lock;
failures preserve the last known value and do not change unrelated Sessions. Thread
discovery and binding are not activity observations.

The `conversation_only` Run marker is persisted independently of the process state. It
does not change accepted task or handoff provenance, and it prevents the Codex bridge
from exiting merely because the workspace status is `completed`; `archived` still stops
the bridge.

Every new runner exports both `WORKSPACE_SESSION_ID` and `WORKSPACE_RUN_ID`, plus
`WORKSPACE_PARENT_SESSION_ID` when the Session was delegated by an active parent. Actor
mutations are accepted only from the Session's current Run. tmux stores both IDs on
the pane; a legacy pane storing only its old concrete `sess_*` remains compatible
because migration preserves that value as the Run ID.

Only one repository writer may own a worktree. An analysis persona can additionally
start with `session start --read-only`; this is a cooperative role contract and does
not sandbox the process. Readers write reports under their own session subdirectory
in work-products. An active writer can change files during analysis, so review of a
stable revision should use another worktree. Resume preserves reader status.

Native Codex delivery is handled through its app-server bridge between turns. Native
OpenCode Runs expose a unique loopback endpoint owned by that Run. The supervisor
checks readiness, reads a bounded recent history window, selects the Run's
`client_thread_id`, appends the marker-bearing prompt through `/tui/append-prompt`,
submits through `/tui/submit-prompt`, and confirms the marker in the same history
window before recording delivery. Every HTTP request has a deadline and the complete
attempt is bounded (currently 750ms per request, 2s readiness, and 5s total). A new
Run gets a new endpoint; the logical Session may retain the same native thread ID.
The durable delivery phases are `checking`, `retry`, `appended`, `submitted`,
`uncertain_append`, `uncertain`, and `confirmed`. A retry always reconciles history
first: it never appends again from `appended`, `submitted`, or an uncertain submit,
and it refuses to repeat an append whose outcome is unknown. A marker already present
converges directly to confirmed/delivered without a TUI mutation.
OpenCode session discovery accepts both endpoint and executable-list metadata shapes.
If an old resumable (`lifecycle_state=idle`) logical Session has empty native-thread fields, resume repairs the
binding from a uniquely correlated historical Run before building the next Run's
`resume_argv`; an ambiguous or unavailable repair stops with the manual
`workspace session bind-thread` recovery path instead of silently starting another
conversation. A successful empty listing still permits a fresh first conversation.
An active Run migrated from the retired bundled OpenCode transport cannot be repaired in
place: it has no server flags or Run-scoped endpoint. The supervisor records a durable
`restart_required` delivery phase, leaves the message unacknowledged and undelivered, and
waits for one explicit stop/resume. That resume creates the native endpoint and keeps the
pending message eligible for delivery on the successor Run.
If native delivery times out or is rejected, the message remains undelivered and the
supervisor emits the existing pending-inbox tmux notification at most once for that
Run. The notification is a transport-failure fallback, not an ACK; handoff acceptance
is independent. A failed OpenCode delivery is recorded and does not prevent later
messages or workspaces from being attempted. A generic client's optional
`deliver_argv` receives `{thread_id}`, `{message_file}` and `{message_id}` and must
return `{"accepted":true}`. It must deduplicate message IDs; transport delivery is at
least once. Without a delivery adapter, the supervisor shows a tmux notification and
the agent reads `inbox` explicitly. No terminal keystroke injection or
`/tui/clear-prompt` is used. An ACK records receipt; accepting a handoff is a separate
decision.

Messages and handoffs are addressed to exact logical Sessions, not native thread IDs or
the most recent Session of an Agent. An idle target remains pending until its own Run is
active; a closed or deleted target is never rerouted. Agent processes are scoped to their
current Session, while users choose `--session` for historical inspection or `--agent` for
an explicit agent-wide view. Replies target the original `FromSession`, and deletion is
guarded by parent, target, provenance and receipt references.

The private registry is currently schema version 5. Migration is staged: legacy
process-shaped sessions become logical Sessions and Runs, then communication targets,
parent Session links, OpenCode snapshot normalization, and deterministic loopback
endpoint repair are applied. Ambiguous historical agent targets remain legacy records
with an empty `ToSession` instead of being assigned to an arbitrary Session.

Restart the project supervisor after installing a newly built binary so the new
delivery code is loaded. An OpenCode Session with a valid existing Run endpoint does
not need to be stopped, recreated, or rebound.

After user-confirmed release, stop remaining sessions and `archive` the workspace. An
intentionally manual workspace has no release: finish it with the explicit, user-confirmed
`workspace complete` operation once no active Sessions or services remain and every
non-deleted task is accepted, then `archive` it without a release reference.
To authorize new work after completion, use `workspace reopen --reason ...
--expected-revision ...`. Reopen preserves tasks, artifacts, handoffs, and the base
commit, records the prior document under `history/reopen_ID/`, invalidates derived
release/integration/live-test state, and marks change requests outdated. It refuses
active worker Sessions and services, is idempotent under its operation key, and does not
apply to archived workspaces.
`clean --dry-run` reports which worktrees can be removed. Uncommitted/unpreserved files
and unpublished commits prevent removal. `clean --backup` can preserve unpublished
commits in a verified Git bundle; source branches remain. Only Git removes worktrees.
Documents, artifacts and decisions remain available after cleanup.

## Managed terminal interface

The optional managed TUI is a separate runtime owner, not a Session, Run, service or
repository writer. Its desired state, generation, pane identity and idempotency
receipts live in `.runtime/ui.json`; this does not change the public workspace status
schema or add UI records to agent lists. A workspace without a UI record starts with
the interface disabled.

The managed TUI is user-operated. `workspace tui show` explicitly records desired state
and performs one reconcile when runtime is available, creating at most one detached
split in the existing orchestrator window without changing focus. `workspace tui hide`
disables the panel and removes only a pane whose UI ownership is verified. `status`
reports desired state, generation, ownership, errors and the last known runtime state.
The supervisor, workspace start, and the general `reconcile` operation do not start or
restore the TUI.

If the managed process exits or its pane is killed, the panel remains absent until the
user runs `workspace tui show` again. Explicit UI operations can verify the exact runner
command and repair metadata without removing adjacent panes. The UI does not count as
an active domain process and does not block `clean`. Before downgrading the binary, run
`workspace tui hide`, since an older launcher cannot identify the new pane type.

Project-picker Delete Workspace is a separate destructive discard, not an archive or
clean shortcut. After typed-ID confirmation it kills the complete workspace tmux session,
force-removes registered worktrees and their local `workspace/<id>/…` branches, then
removes durable workspace state. It intentionally does not preserve dirty or unpublished
local work and therefore does not require release, archive, or clean gates.

The full screen, navigation, action and terminal behavior is documented in
[`tui.md`](tui.md).
