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
logical Session with a new Run. Transient
tmux errors preserve reservations. Explicitly stopped or failed clients are not
automatically restarted. Captured check processes lost with their Run are marked
interrupted. A paused workspace does not delegate or restart work automatically.

`pause` stops new delegation. `pause --interrupt` also stops active panes, preserving
their worktrees. `resume` permits delegation again; `agent resume NAME` starts the
chosen persona using its prior assignment. Generic clients receive a fresh bootstrap;
adapters with native thread support can reuse the conversation.

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

Every new runner exports both `WORKSPACE_SESSION_ID` and `WORKSPACE_RUN_ID`. Actor
mutations are accepted only from the Session's current Run. tmux stores both IDs on
the pane; a legacy pane storing only its old concrete `sess_*` remains compatible
because migration preserves that value as the Run ID.

Only one repository writer may own a worktree. An analysis persona can additionally
start with `session start --read-only`; this is a cooperative role contract and does
not sandbox the process. Readers write reports under their own session subdirectory
in work-products. An active writer can change files during analysis, so review of a
stable revision should use another worktree. Resume preserves reader status.

Native Codex delivery is handled through its app-server bridge between turns. OpenCode
sessions are matched automatically after launch using `opencode session list`; the
resulting native ID is persisted as the Session binding and snapshotted on each Run.
A generic client's optional
`deliver_argv` receives `{thread_id}`, `{message_file}` and
`{message_id}` and must return `{"accepted":true}`. It must deduplicate message IDs;
transport delivery is at least once. Without a delivery adapter, the supervisor shows
a tmux notification and the agent reads `inbox` explicitly. No terminal keystroke
injection is used. An ACK records receipt; accepting a handoff is a separate decision.

After user-confirmed release, stop remaining sessions and `archive` the workspace.
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

The full screen, navigation, action and terminal behavior is documented in
[`tui.md`](tui.md).
