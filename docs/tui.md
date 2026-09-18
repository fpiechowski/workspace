# Terminal Interface

`workspace tui` is an interactive presentation of local workspace state and uses the
same core queries and mutations as the CLI. Reading or refreshing alone creates nothing
and does not start the orchestrator or supervisor; an explicit action in the picker may
create a workspace.

## Screens and Navigation

The project picker shows the title, status, phase, active Runs, issues, full ID, path,
input source, and creation date. When the list is empty, it shows a title, one-sentence
explanation, and one valid action (`a` creates a workspace), so it no longer contradicts
the Create workspace action. In wide mode, it shows selection details (ID, path, input
source, creation date, and state) next to the list without a panel border. Selecting a
workspace opens Work: a progress bar (`bubbles/progress`) with textual `accepted/total`
and percentage, plus a status bar with separate live, review, blocked, and attention
counters. The summary remains visible while switching among the Agents & runs, Tasks,
Needs attention, and Recent recorded activity sections. Agents & runs shows the
orchestrator, active executions, and open sessions for current attempts of unaccepted
tasks. Each row combines an agent with a task, state, and model. Tasks shows the task
state and the number of active executions, sessions, and runs for its current attempt.
Run completion does not mean task acceptance; the progress percentage counts only
accepted tasks.
`b` switches the Tasks tab between **List** and **Board**. Board shows one column per
task state in the fixed order pending, running, blocked, needs_changes, awaiting_review,
accepted; unknown states are appended at the end and rendered as neutral text. Cards use
the same data as the list (task title and subtitle), and columns are derived from
undeleted tasks on the Tasks page, so filtering and `f` remove cards rather than entire
columns. The selection remains pinned to `route.SelectedID`, so `Enter`, `t`, `a`, `g`,
`s`, `/`, and `f` continue to work unchanged. In wide mode, columns sit side by side;
when they do not fit, the window contains the active column and shows its position in the
count line (for example, `columns 2–5/6`). In compact mode, only the active column is
visible with the `‹ state (n) › k/m` pager header. The default view is List, and the
List/Board choice is remembered only for the TUI session (like sorting and filters) and
is not persisted between launches.

State has a symbol and label; running/starting have an animated indicator, including
without color. `s` changes sorting (work priority, name, last execution); the selection
remains pinned to the ID. Wide view shows named list and `Preview` panels with one clear
focus edge, while the preview action line advertises only commands supported by the
selected kind (`t`, `g`, `Enter`). Narrow view preserves progress and two-line entries.
At small heights, an entry occupies one line. Entries are separated by a lower-emphasis
line, and the selection covers the title and description on a shared background; without
color, the selection marker and separator remain. The shortcut bar always reserves the
last row.

`a` in the project picker provides workspace creation (title, description, and explicit
mode choice: named workflow, `Choose later`, or `No workflow (manual orchestration)`) as
well as permanent deletion of the selected workspace. Deletion is available only after
entering the full ID and does not require release or archive. It is a full discard: it
stops the runtime and removes state, all worktrees including uncommitted files, and
local `workspace/<id>/…` branches. This operation cannot be undone. Workflow selection
is available only for the `needs_workflow` state; a manual workspace does not later offer
selection or advance.

On the Dashboard, `a` provides `Complete this manual workspace` for an active manual
workspace and `Archive completed workspace` when the state is `completed`. Archive
requires confirmation, checks the revision, and preserves all data. A workflow still
requires a confirmed release, while a completed manual workspace requires an earlier
`complete`; in both cases there must be no active Sessions or services.

More contains Sessions, Agents, Services, Decisions, Change requests, Runtime, Needs
attention, Recent recorded activity, and Documents. Runtime detail renders tmux
topology as a table (`bubbles/table`) with window, pane, kind, owner, run, and state
columns in wide mode and an equivalent row layout in compact mode; errors and managed
interface state remain above the topology. A Task leads to current sessions, related
worktrees, and results; `f` switches between the current-attempt and history views. An
artifact whose lineage cannot be determined is marked unknown and shown in history.
Worktree detail shows related tasks/sessions/services, active writers and read-only
readers, and a separate Git observation with a timestamp.

Details for tasks, sessions, runs, worktrees, handoffs, checks, decisions, change
requests, runtime, and the orchestrator, as well as file previews, are documents with a
fixed hierarchy: identity and status first, operational facts next, then narrative and
related-resource sections, and finally IDs, digests, and timestamps. Long goals,
instructions, summaries, reasons, command lines, paths, and change-request content wrap
to the viewport width, and unbreakable tokens are split; explicit newlines in previews
remain preserved. Binary or truncated content has an explicit `[!]` message, while a
missing entity has a recovery message with `Esc` and `r`. Every document scrolls with
arrows or `PgUp`/`PgDn`; the status row shows `line x–y of n` only while scrolling, and
the position returns after coming back from another route.

`Enter` opens the selection, while `Esc` goes back or clears a filter. In collections, `/`
edits a case-insensitive substring filter over the name, ID, and subtitle. `Esc` while
editing restores the previous value and selection. Outside editing, it first clears the
text filter, then the status filter, before returning to the previous page. `f` switches
the active/archived workspace view and the status or history filters for the relevant
collections. On the board, `Up`/`Down` moves within the active column (stopping at the
ends without wrapping), `←`/`→` moves between non-empty columns and wraps at the edges,
`PgUp`/`PgDn` works within a column, and `Home`/`End` jumps to the first or last card of
the outermost non-empty column. On the Dashboard, `Tab`/`Shift+Tab` changes focus among
Agents & runs, Tasks, Needs attention, and Recent recorded activity. On Results, it
changes the result type: artifacts, handoffs, or checks; the active type is named in the
helper line. Detail and dependent-collection routes show a breadcrumb, and on task
details `1`–`3` are shortcuts to related resources (Sessions, Worktrees, Results), not
top-level pages. Use `1`–`5` to go to the main pages.

| Key | Action |
|---|---|
| `1`–`5` | Work, Tasks, Worktrees, Results, More |
| `Up`/`Down`, `j`/`k` | Change selection or scroll details |
| `PgUp`/`PgDn` | Scroll the document or move the selection by one page in a collection |
| `Home`/`End` | Start or end of a document or collection |
| `b` | Switch Tasks between List and Board (`b board` / `b list` in the footer) |
| `←`/`→` | Change the active board column (Board view only) |
| `Enter` | Open the selection; does not start a mutation |
| `Esc` | Cancel a form, exit filter editing, clear a filter, or go back |
| `/` | Edit the collection filter |
| `f` | Switch status or history on supported lists |
| `Tab` / `Shift+Tab` | Change Dashboard panel focus or result type |
| `s` | Sort by priority, name, or last execution |
| `l` | Return to Agents & runs |
| `v` | Open Needs attention from the Dashboard |
| `t` | Open the agent terminal or confirm start/resume |
| `a` | Open available actions for the selection or workspace |
| `g` | Jump to a verified tmux target |
| `w` / `o` | Workspace picker / orchestrator page |
| `r` | Refresh the snapshot and runtime; does not reconcile |
| `?` | Contextual help |
| `q` / `Ctrl+C` | Exit a manual instance; hide a managed pane |

`Esc` and `Ctrl+C` cancel an open form, including its filter and confirmation; the
remaining keys belong to the form. `Ctrl+C` in a managed pane outside a form records
hide. While editing filter text, the filter captures the keys, so `q`, `g`, and digits do
not trigger application shortcuts.

The footer shows only commands available for the current selection and backend
capabilities; unsupported terminal, jump, and action commands are not advertised. In
Tasks, the footer shows `b board` in List view and `b list` in Board view, and
`←/→ column` only in Board view. `?` opens scrollable help grouped into Navigation,
View, Runtime, Actions, and Exit; the status row shows the scroll position, while `?`
or `Esc` returns to the previous view.

`t` on an active session opens its exact current Run after ownership verification. On an
inactive session, it asks for resume confirmation, creates a new Run through core, and
opens its terminal on success. This resumes that specific Session; it is not a Task
retry. When the Session's Task is awaiting review, an unchanged resume still creates a
Run but preserves the `awaiting_review` state and the provenance of the pending handoff.
`o`, followed by `t`, opens the orchestrator or confirms starting it. A task with
multiple sessions opens their list for explicit selection; a task without a session
indicates that delegation through the orchestrator is needed. A historical Run remains
the exact target and is not resumed automatically.

If navigation reports a missing session or tmux pane, the TUI proposes reconcile in a
confirmation form. Esc and Cancel leave the runtime unchanged. After confirmation, it
performs one reconcile operation and retries navigation to the same target. Reconcile
may restore an eligible orchestrator; it does not automatically restart stopped workers.
If the target is still unavailable, the TUI points to Agents & runs and `t` to open or
resume the session, without a confirmation loop. An ambiguous target, mismatched
ownership, or mismatched socket does not trigger a reconcile proposal.

## Actions and Synchronization

Actions are limited to the existing core contract. Available actions include starting or
resuming the orchestrator, pause, guarded pause-and-interrupt, workspace resume,
reconcile, workflow selection (only for `needs_workflow`), `complete` for a manual
workspace, task retry, session resume/stop/close, and service stop. Runtime provides
show and hide for the managed TUI. The Task and Session menus also provide Delete. It
requires entering the full ID and records a `deleted_at` tombstone; the record
disappears from normal collections but remains in history for receipts. Core rejects an
active session, a session with result references, an accepted or dependent task, and a
task with active sessions or persisted handoffs, artifacts, checks, integration, or a
change request. The TUI does not send messages to agents, ACK the inbox, or accept
handoffs or decisions.

Each new, confirmed intent receives a `tui_<ULID>` key. Double confirmation does not
start a second mutation. A retry after an error uses the same key and payload; after
success, the TUI fetches a new snapshot. Core guards check the revision, task attempt,
or exact current Run. Before confirmation, “Pause and interrupt” shows the exact active
Runs and services; a change to either list rejects the operation before processes are
stopped. Task retry shows the attempt, dependent tasks, and reason.

Snapshot, tmux, UI, and Git reads have separate asynchronous states. A late response
from an old workspace cannot overwrite the current selection. A failed read shows an
error and preserves the last good snapshot as stale. An unavailable runtime does not
block browsing persisted state.

## Terminal and Managed Pane

Views adapt to width and height, and a terminal below 40×12 shows a short message
instead of a cramped layout. The available themes are `auto`, `dark`, and `light`, plus
`--no-color`. The TUI does not require Git status for every worktree: Git is inspected
when a specific Worktree detail is opened.

`g` resolves the target from its ID and checks ownership before `switch-client`, window
selection, or attach. A historical Run does not jump to a newer execution. Outside tmux,
the TUI may release the terminal during attach and refresh data after returning. A
runtime error or absence is shown to the user without inventing an alternative target.
Inside tmux, navigation selects the client by its TTY and current pane; no client,
multiple clients viewing the pane, or another socket produces an explicit error instead
of switching a random terminal.

The supervisor maintains at most one managed pane per workspace after orchestrator
history has been recorded. The pane goes into the same orchestrator window with an
inactive split, does not take focus, and is not an Agent, Session, or Run.
`workspace tui show` records desired state and reconciles the pane; `hide` disables it,
and `status` shows generation, ownership, error, and backoff. `q`/`Ctrl+C` records a hide
request and exits the TUI only after restoring the terminal; the supervisor removes the
pane at the next reconcile. An external pane kill leaves desired=true, so the supervisor
can restore it. If the TUI process is still running but its pane metadata disappears or
is corrupted, reconcile confirms the exact runner command and restores the metadata. It
does not remove neighboring, unverified panes. Losing the entire orchestrator window
first causes the orchestrator to be restored, followed by one TUI pane in the new window.

The pane remains available after pause and completed until the user hides it or the
workspace is archived. Archive cleans up the verified pane; the UI does not block `clean`
and is not counted as an active domain process. Before downgrading the binary, run
`workspace tui hide`, because the old launcher does not recognize the new pane.
