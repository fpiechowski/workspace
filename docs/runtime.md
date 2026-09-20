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

The orchestrator/user can manage services; workers can manage only services in their
assigned worktree. Active services block archive/cleanup and revision migration.
`pause --interrupt` stops them too. A lost service pane is marked interrupted rather
than restarted silently.
`start`, `session start` and `agent resume` start the project supervisor automatically.
`server status` inspects it; `server stop` stops supervision without killing agents.
`serve` runs supervision in the foreground.

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

| Logical Session state | Meaning |
|---|---|
| `active` | `current_run_id` points to the single starting/running Run |
| `idle` | no active process; the Session can be resumed when lineage is unchanged |
| `closed` | explicitly closed logical context; no further Runs are allowed |

| Run state | Meaning |
|---|---|
| `starting` | durable launch intent exists; pane identity may still be recovered |
| `running` | the runner claimed ownership |
| `exited` / `failed` | client exited normally / with an error |
| `stopped` | explicitly stopped; supervisor does not restart it |
| `interrupted` | pane was verified lost or superseded; Session stays resumable |

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
If an old idle logical Session has empty native-thread fields, resume repairs the
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
repository writer. Its desired state, generation, pane identity, retry status and
idempotency receipts live in `.runtime/ui.json`; this does not change the public
workspace status schema or add UI records to agent lists.

After orchestrator history exists, the supervisor can start one TUI pane in the same
orchestrator window. The split is detached and does not take focus. A manual
`workspace tui` read or refresh never starts the supervisor. `workspace tui show`
records desired state and reconciles immediately when runtime is available;
`hide` disables the panel and removes only a pane whose UI ownership is verified;
`status` reports desired state, generation, ownership, errors and backoff. The
supervisor reconciles UI independently from agent recovery and message delivery, so a
failure in one does not skip the others.

If the managed process still runs but its pane metadata is missing or damaged, the
reconciler can verify the exact runner command and restore the metadata in place. It
does not remove adjacent panes without a verified UI identity. Losing the entire
orchestrator window recovers the orchestrator first and then creates one managed pane
in the replacement window; unrelated worker runs are not restarted.

In a managed pane, `q` or `Ctrl+C` outside a form records a durable hide request, restores
the terminal, and exits; the next supervisor pass removes the pane. `Ctrl+C` inside a
form cancels that form. An external pane kill leaves desired state enabled and can be
recovered after backoff. A hide request is durable and prevents restart. Paused and
completed workspaces may keep the interface available for inspection. Archive disables
and cleans up the verified pane; UI ownership is not counted as an active domain
process and does not block `clean`. Before downgrading the binary, run `workspace tui
hide`, since an older launcher cannot identify the new pane type.

Project-picker Delete Workspace is a separate destructive discard, not an archive or
clean shortcut. After typed-ID confirmation it kills the complete workspace tmux session,
force-removes registered worktrees and their local `workspace/<id>/…` branches, then
removes durable workspace state. It intentionally does not preserve dirty or unpublished
local work and therefore does not require release, archive, or clean gates.

The full screen, navigation, action and terminal behavior is documented in
[`tui.md`](tui.md).
