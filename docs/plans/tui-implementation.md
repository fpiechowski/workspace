# User TUI — Implementation Plan

Status: ready for implementation; this document does not mean that the backlog has been completed.
Project date: 2026-09-15. Analysis base: commit `58f4cc29c0d8a62d40187c3587b2aa7824752f01`.
Interface language: English, consistent with the existing CLI. Implementer documentation: English.

## 1. Implementer Instructions

Implement the full scope of this document in the stages from section 13. Before working,
read AGENTS.md and the referenced documents and tests. Preserve existing changes. Do not
implement other TODO.md items. Do not publish changes or invoke real model clients,
trackers, or forges during verification. Do not change the Task/Session/Run model for
interface convenience.

This document defines the product, navigation, architecture, and recovery behavior.
Names of new files and APIs are the target responsibility split; a minor name correction
is allowed if it conflicts with an existing symbol. Do not skip any stage or acceptance
criterion. At the end, report the tests that ran and concrete limitations. Do not mark a
TODO item complete if tmux integration has not been verified.

## 2. Decisions and Scope

1. The new explicit `workspace tui` command starts the interface. Bare `workspace`,
   `open`, `attach`, `menu`, existing commands, and response formats retain their contract.
2. In the project directory, show the workspace picker. In a workspace or its child,
   show that workspace's dashboard. A project with one workspace still shows the picker.
3. The primary navigation axis is **Workspace → Tasks → Task → Sessions / Results**.
   Worktrees are a parallel infrastructure perspective and lead to the same entities.
4. The dashboard shows state, counters, the orchestrator, and at most five issues. It
   does not show every agent, session, task, or worktree.
5. The TUI uses typed core directly. It does not run `workspace ...` commands as
   subprocesses to fetch JSON or perform domain operations.
6. One managed TUI pane lives in the orchestrator window. The supervisor restores a
   lost pane. The TUI is not an Agent, Session, Run, or BackgroundService.
7. The automatic pane remains useful while paused and after processes finish. It does
   not block archive/clean or consume writer leases, model limits, or task limits.
8. Process state, result acceptance, and workflow phase are separate pieces of information.
9. The interface works at 40×12 characters; it supports a wider short panel and a
   narrow tall panel. At smaller sizes it shows a safe minimal screen and an exit option.
10. V1 covers the browsing and operations listed in section 7. It does not implement the
    entire CLI through forms. Task/person/worktree creation, CR publication, live-test
    decisions, release, handoff acceptance, migrations, and data deletion remain in the
    existing CLI. The state of these processes and pending decisions is visible in the TUI.

Out of scope: code editor, terminal within the terminal, agent-client key capture,
cost/token dashboard, remote workspaces, a separate UI daemon, global fuzzy search over
all file contents, drag-and-drop, GUI/web, and automatic workflow decisions.

### 2.1. Stack and Versions

Keep `go 1.24.0`. Use one v1 API family:

| Module | Version | Role |
|---|---|---|
| `github.com/charmbracelet/bubbletea` | `v1.3.10` | Model/Update/View, event loop, terminal |
| `github.com/charmbracelet/bubbles` | `v0.21.0` | list, viewport, textinput, help, key, spinner |
| `github.com/charmbracelet/lipgloss` | `v1.1.0` | layout and styles |
| `github.com/charmbracelet/huh` | `v0.7.0` | embedded forms and confirmations |
| `github.com/charmbracelet/x/ansi` | `v0.10.1` | ANSI-aware width, wrapping, and truncation |
| `github.com/charmbracelet/x/term` | `v0.2.1` | terminal detection |

This is a deliberate compatibility choice for the current Go minimum. The go.mod files
for these releases were verified: Bubble Tea requires Go 1.24; Bubbles and Huh require
Go 1.23. Huh 0.7.0 depends on Bubbles 0.21.0. Huh 0.8.0 introduces a pseudo-version of
Bubbles; do not select it accidentally. Current Bubble Tea/Huh main branches use API v2
and newer Go. Do not copy examples from main into the v1 implementation. Do not use
`@latest`. After `go mod tidy`, check the full module graph with `GOTOOLCHAIN=local`
and Go 1.24; do not accept a silent increase of the minimum.

Glamour: **do not add it in this scope**. Markdown preview is wrapped text in a
viewport, preserving headings and code blocks. This is the complete V1 preview. Markdown
rendering may be a later change; it is not a completion condition for this item.

Version and API sources:

- [Bubble Tea 1.3.10 — go.mod](https://github.com/charmbracelet/bubbletea/blob/v1.3.10/go.mod)
- [Bubbles 0.21.0 — go.mod](https://github.com/charmbracelet/bubbles/blob/v0.21.0/go.mod)
- [Lip Gloss 1.1.0 — go.mod](https://github.com/charmbracelet/lipgloss/blob/v1.1.0/go.mod)
- [Huh 0.7.0 — go.mod](https://github.com/charmbracelet/huh/blob/v0.7.0/go.mod)
- [Bubble Tea — terminal handoff through Exec](https://github.com/charmbracelet/bubbletea/blob/v1.3.10/exec.go)
- [Bubble Tea main — go.mod](https://github.com/charmbracelet/bubbletea/blob/main/go.mod)
- [Huh main — go.mod](https://github.com/charmbracelet/huh/blob/main/go.mod)
- [tmux — reference documentation](https://man.openbsd.org/tmux.1)

## 3. What Exists and What Actually Needs Extraction

Read [PRODUCT.md](../../PRODUCT.md), [ARCHITECTURE.md](../../ARCHITECTURE.md),
[README.md](../../README.md), [runtime](../runtime.md), and [operations](../operations.md).
For historical result details, also read [revisions](../revisions.md),
[checks](../checks.md), and [clients](../clients.md).

| Existing code | Implementation significance |
|---|---|
| `internal/core/model.go` | Workspace, Registry, Status, Session, and Run; `Status()` synchronizes session projections |
| `internal/core/workflow_model.go` | Task identifies the current worktree/session/run; artifacts and handoffs have provenance |
| `internal/core/project.go` | Service, DiscoverProject, InferWorkspace, With, List, Status; project lock |
| `internal/core/files.go` | loadDocument, write recovery, atomic writes, contained; do not bypass it for reads |
| `internal/core/session.go` | start/stop/resume/reconcile, lineage, and runtime ownership |
| `internal/core/runtime.go` | Tmux, Launch, Recover, Inspect, Attach, shellQuote |
| `internal/core/supervisor.go` | EnsureSupervisor, Tick, tickWorkspace, orchestrator recovery |
| `internal/core/actor.go` | empty Actor = user; core enforces roles and stale_actor |
| `internal/core/lifecycle.go` | pause/interrupt, archive, and clean — the TUI must not block these contracts |
| `internal/cli/cli.go` | bootstrap from env/flag/CWD, workspace selection, resume, and attach partially implemented in the adapter |
| `internal/cli/help.go`, `cli_test.go` | help and flag completeness for every visible command |

Core already exists. Do not introduce SQL repositories, an event bus, generic CQRS, or
massive movement of core files into new packages. Extract the specific fragments the TUI
needs from the CLI and add a consistent aggregate read.

### 3.1. Aggregate Relationships

```text
Project
└─ Workspace
    ├─ Task (current Attempt, dependencies, accepted result)
    │  ├─ Sessions by TaskID (current and historical attempts)
    │  │  └─ Runs by SessionID
   │  └─ Results: Handoffs / Artifacts / Checks
    ├─ Worktree (checkout, branch, purpose, lifecycle)
    │  ├─ Tasks: current Task.WorktreeID + history by Session.WorktreeID
   │  ├─ Sessions → Agent persona / Runs
   │  └─ BackgroundServices
    ├─ Orchestrator: Agent → Sessions → Runs; empty WorktreeID
    └─ Workflow / Decisions / Integration / ChangeRequests / LiveTest / Release
```

The `orchestrator` window is not a Git checkout and has no Worktree record. Do not
create a synthetic worktree. In Worktree navigation, add a separate
`Orchestrator · workspace directory` row, clearly separated from checkouts; it opens the
same page as the dashboard tile.

One persona may have multiple historical sessions. Sessions may have no TaskID. Do not
assume one task = one agent = one worktree. Build indexes by ID, not by name. Use
AgentSnapshot for historical session names and show the current persona definition
separately. A historical Run is not the current pane owner.

## 4. Entry, Scope, and CLI Compatibility

### 4.1. Commands

```sh
workspace tui
workspace tui --project /repo
workspace tui --workspace ws_ID
workspace tui --theme dark
workspace tui --theme light
workspace tui --theme auto
workspace tui --no-color
workspace tui show --workspace ws_ID
workspace tui hide --workspace ws_ID
workspace tui status --workspace ws_ID --json
```

`tui show/hide/status` refer to the **managed pane**, not manual instances.
`show` sets desired=true and attempts to restore the pane in an existing tmux session;
it does not start the orchestrator. Without a session, it records the preference and
returns `waiting_for_runtime`. `hide` sets desired=false and removes only the verified
owned pane. `status` shows desired, state, pane/window ID, last_error, and next_retry_at.
These three commands are ordinary CLI commands and support JSON/short/non-interactive.
The show/hide mutations handle operation keys according to section 10.3.

The actual `workspace tui` requires terminal stdin **and** stdout. With a pipe, TERM=dumb,
`--json`, `--short`, or `--non-interactive`, return `interactive_required` without
entering raw mode and without starting tmux/supervisor. Error JSON keeps the standard
CLI shape. Reject `--operation-key` for the TUI loop itself as `invalid_option`: each
action has its own key. Use injected io.Reader/io.Writer rather than secretly opening
`/dev/tty`.

### 4.2. Location Resolution

- Preserve the existing precedence for manual invocation: flags → WORKSPACE_* → CWD.
  The TUI shows the resolved project/workspace name and path in the header.
- Shared bootstrap creates Service and Actor in the same way as the current
  options.service().
- InferWorkspace: `workspace_required` means the project screen; other errors, such as
  broken frontmatter or permission denial, must be shown as errors, not as no workspace.
- In V1, the --workspace flag accepts an ID, consistent with the global CLI flag. Name
  selection happens in the list. Do not add another name resolver with different
  ambiguity rules.
- Verify that the specified/inferred ID belongs to the selected project. Do not switch
  between projects based on an inherited foreign WORKSPACE_ID.
- Normalize the CWD with Abs/EvalSymlinks and use the existing workspaces_dir rules.
- Parent searching works in checkouts inside a workspace. For a registered checkout
  outside this hierarchy, add canonical-CWD matching against that project's Worktree.Path
  only when ordinary InferWorkspace found no workspace. Choose the longest matching path;
  different workspaces with the same match are a conflict. Do not treat the project
  directory as a workspace checkout.
- No project: instruct the user to run `workspace project init`; do not initialize automatically.
- No workspace: show an empty screen with the create command; do not create automatically.

## 5. Navigation and Screen Content

### 5.1. Project Screen

Project header, `/` filter, workspace list: title, status, phase, active-run count,
issue count, short ID. Selection details: full ID, path, input source, creation date.
`Enter` opens the dashboard, and `a` selects an action (including attach). Default
sorting: non-archived workspaces before archived ones, then descending CreatedAt, with
ID as the tie-breaker. Archived workspaces remain available through the status filter.
Do not reorder automatically on every heartbeat; refresh selection data by ID and sort
after filter changes or an explicit `r`.

### 5.2. Workspace Dashboard

Always present: breadcrumb, title/ID, separate status and phase, time of the last
successful read, and a stale-data/error indicator. The content has four panels:

1. **Overview**: tasks accepted/total, running, blocked/needs_changes; active runs;
   ready worktrees; active services. These are record counters, not an entity list.
2. **Orchestrator**: persona, session lifecycle, current/last Run state, model, and a
   `Jump` or `Start/Resume` button in the action menu.
3. **Needs attention**: PendingDecision; blocked/needs_changes tasks; unaccepted
   current handoffs; failed/interrupted current or last executions; supervisor, runtime,
   or UI issues. At most five rows, then `View all (N)`.
4. **Activity**: at most five recent events DERIVED from available dates on Runs,
   Handoffs, Artifacts, and Decision responses. Label: `Recent recorded activity`; this
   is not a full audit log or a new durable event table.

The attention panel deduplicates a task/run issue into one row with links. A stale
handoff has a separate label and does not offer acceptance. Priority: required decision,
read/runtime failure, blocked/needs_changes, current handoff for review, then the rest;
stable by ID within each priority. The full view has search.

Primary navigation: `1 Overview`, `2 Tasks`, `3 Worktrees`, `4 Results`, `5 More`.
More contains Sessions (the entire workspace), Agents, Services, Decisions, Change
requests, and Runtime. These are explicitly selected pages — no expanded lists on the
dashboard. `w` opens the workspace selector in the current project, and `o` opens the
orchestrator page.

### 5.3. Task

List: name/title, Task.State, attempt, Run activity badge, current worktree. Details:
goal, role/profile, acceptance criteria, required artifacts/checks, linked dependencies,
reason, and accepted handoff. Sections opened with Enter:

- Sessions: current attempt by default; the `History` toggle shows previous attempts/input
  lineage. Each session shows the agent, model, lifecycle, and Run state.
- Worktrees: current and historical worktrees linked through sessions, with a history label.
- Results: Handoffs, Artifacts, and Checks for this task only, with attempt/history filters.

Do not assign an artifact to the current attempt using TaskID alone. Link it to the
Run/Session and SourceHandoff, and display old/undetermined lineage as history/unknown.
Task.accepted remains accepted even when an old execution ended with an error.

### 5.4. Worktree and Orchestrator

Worktree: path, branch, purpose, lifecycle, writer/readers/active services, and related
tasks and sessions. Separate `ready` (the checkout exists according to the registry)
from `active` (it has an active Run). Inspect Git dirty/HEAD only on this page, in a
separate query with a timeout; do not run git status for every checkout every two seconds.
The Git result is an observation with its own timestamp, not a Worktree.State change.

Orchestrator page: current session + history, workspace directory, managed TUI pane, a
link to the tmux window, and actions. No fabricated branch/WorktreeID.

### 5.5. Session, Run, Agent, Service

- Session: AgentSnapshot, TaskID/attempt/WorktreeID lineage, lifecycle,
  CurrentRunID/LastRunID, read-only, and native thread if present; Runs in history.
- Run: exact state, model/provider/client, time, exit code and error, and provenance.
  `Jump` only for the current, live, verified pane. An old execution does not silently
  jump to a new Run; a separate `Current session` link is allowed.
- Agent: definition and session list; no invented durable Agent.State.
- Service: state/exit code, worktree, argv as text, jump, and stop in the action menu.

### 5.6. Results and Documents

Results has Artifacts/Handoffs/Checks tabs, a shared name/ID filter, and an optional task
filter. Artifact: provenance metadata, digest, size, commit, and preview. Handoff:
outcome, state, stale, summary, feedback, risks, and links to artifacts/checks. Check:
state, exit_code, SHA, argv, and output preview. A `completed` check with exit != 0 is
not a success; show the exit code regardless of state.

WORKSPACE.md/WORKFLOW.md/input snapshot documents are available through Overview →
Actions → View documents. Preview locally only; do not open URLs from content. Binary
files show metadata. Text preview is limited to 256 KiB with a truncation marker; do not
read the entire large file before truncating. Scrolling and search in the visible list
are separate mechanisms; full-text file search is not required.

### 5.7. Keyboard and Event Routing

| Key | Action |
|---|---|
| Up/Down, j/k | list or scroll the active panel |
| Enter | open the selection; never automatic start/stop |
| Esc | form → cancel; filter editing → go back; active filter → clear; otherwise previous screen |
| Tab / Shift+Tab | next/previous panel; form fields inside a form |
| 1–5 | main pages, only outside text editing |
| / | edit the current collection filter |
| f | status/history filter menu, collection pages only |
| a | action menu for the selection/current page |
| g | Jump: tmux session/window/pane for the selection |
| w / o | workspace picker / orchestrator |
| r | refresh the read; does not reconcile |
| ? | help with shortcuts available in this context |
| q / Ctrl+C | exit a manual instance; hide the managed pane (section 10) |
| PgUp/PgDn, Home/End | list or viewport |

The text field receives q, g, r, digits, and space; they are not application shortcuts
while editing. Ctrl+C in a form cancels the form and exits/hides the pane only outside it.
Do not override global tmux bindings or the user's prefix.

Filter: case-insensitive substring over title/name and ID; additionally by AgentSnapshot
name for sessions, ID/model for Runs, and name/branch for worktrees. Normalize case, do
not use regex, and do not filter hidden collections. Show `N / total` and the filter text.
Search works locally on the snapshot, without I/O per key. An empty result has a clear
message and `Esc clear`. Remember the query, selection ID, and scroll position per route
during one TUI session.

## 6. Layout and Theme

Use Lip Gloss, without emoji and without requiring Nerd Fonts. Icon + text, for example,
`[>] running`, `[x] failed`, `[!] blocked`, `[+] accepted`, `[-] idle`, `[#] stopped`.
Icons are ASCII and have fixed width; color is an additional information channel.
Borders may be Unicode, with an ASCII variant when the locale is unsuitable.

Theme uses semantic tokens: background, surface, text, muted, border, focus, info,
success, warning, danger, selection. Dark: terminal background, surface #1E293B, text
#E2E8F0, muted #94A3B8, border #475569, focus/info #67E8F9, success #86EFAC,
warning #FDE68A, danger #FDA4AF. Light: terminal background, surface #F1F5F9, text
#0F172A, muted #475569, border #94A3B8, focus/info #0E7490, success #166534,
warning #92400E, danger #BE123C. The active panel has a clear border and `>` marker.
Choose the ANSI-16 variant by role; no-color emits no color sequences.

`--theme auto|dark|light`, auto by default; `--no-color` and a non-empty NO_COLOR take
precedence. Use one renderer/palette per instance, without mutating global styles in
tests. Auto uses terminal capabilities; when detection is unavailable, it selects dark
with a transparent background. Huh receives the same palette, not its own high-contrast
theme.

Dimension rules (columns × rows, actual pane dimensions):

- **Wide**: width >= 100 and height >= 24. Dashboard 2×2; collections list 40%,
  details 60% with minimum widths 32/40; main tabs at the top.
- **Short**: width >= 80 and 12 <= height < 24. Header/tabs/footer each take one row;
  Overview uses two short columns, the rest through Tab. Lists use one row per record;
  details open as a separate page with Enter.
- **Narrow**: remaining width >= 40 and height >= 12. One column, one content panel,
  compact tabs `1 Home 2 Tasks …`; dashboard as scrollable sections. Details are always
  a separate page. No persistent left sidebar that consumes width.
- **Tiny**: width < 40 or height < 12. Workspace name is shortened, with
  `Terminal too small`, a 40×12 hint, and q; text is truncated even at 1×1. No negative SetSize.

On resize, recalculate all components and forms while preserving route/query/focus/ID.
Calculations include borders, padding, header, and footer. Do not calculate width with
len(bytes). A long title/path must not push status off-screen; show full text in details.
Forms in narrow/short show one field/group at a time with a scrollable explanation.

Wide example (illustrative content; numbers come from the snapshot):

```text
Project / Checkout fix       [>] active  · implementing      updated 2s ago
1 Overview   2 Tasks   3 Worktrees   4 Results   5 More
┌ Overview ──────────────────┐ ┌ Orchestrator ─────────────────────────┐
│ Accepted 3/8 · Running 2   │ │ [>] running · session active           │
│ Blocked 1 · Worktrees 4    │ │ client / provider / model              │
└───────────────────────────┘ └────────────────────────────────────────┘
┌ Needs attention (2) ───────┐ ┌ Recent recorded activity ──────────────┐
│ [!] parser: blocked       │ │ [+] implementation handoff submitted    │
│ [!] pending user decision │ │ [-] planner process exited              │
└───────────────────────────┘ └────────────────────────────────────────┘
Tab panel   Enter open   / filter   a actions   g jump   ? help   q quit
```

Narrow example: header → compact tabs → Overview with counters → Orchestrator → Needs
attention, all in one scrollable surface. The footer remains visible.

### 6.1. Statuses: Keep Axes Separate

| Entity | Values / presentation |
|---|---|
| Workspace | active, needs_workflow, paused, blocked, needs_attention, completed, archived; phase beside it |
| Task | pending, running, awaiting_review, accepted, needs_changes, blocked; unknown as neutral raw text |
| Worktree | creating, ready, failed, removing, removed; writer/readers/services separately beside it |
| Session | LifecycleState active/idle/closed; current/last Run.State beside it |
| Run | starting, running, exited, failed, stopped, interrupted; exited = process ended, not accepted |
| Service | starting, running, exited, failed, stopped, interrupted |
| Runtime observation | present, missing, unknown/unavailable; not a new Run state in the registry |

Do not assume the table exhausts future states. Test unknown values. The active-run count
is based on `Run.Active()` and Session.CurrentRunID consistency; on a tmux error, mark it
as persisted/unverified state. Do not change a run to interrupted based on a timeout in
the TUI alone. That is the responsibility of core.Reconcile/the supervisor.

## 7. V1 Operations and Their Contracts

Actions display the name, target, and reason for unavailability. Core always validates
authorization and state again. Do not treat the menu as authorization. The mapping from
an action ID to a method is static; do not execute MenuAction.Command strings as shell.

| Screen / action | Core / behavior | Form |
|---|---|---|
| Workspace: Start orchestrator | new StartSupervisedOrchestrator → existing StartOrchestrator | confirm client, workspace, and profile start |
| Workspace: Pause | Pause(..., false, key) | short confirm; active runs remain |
| Workspace: Pause and interrupt | Pause(..., true, key), guarded by the run/service list | confirmation with the exact executions to stop |
| Workspace: Resume workspace | SetPaused(..., false, key) | confirm; does not promise process resumption |
| Workspace: Reconcile | Reconcile(..., key) + reconcile UI after releasing the lock | confirm; explicit operation, not refresh |
| Workspace needs_workflow | SelectWorkflow(..., name, key) | choose from WorkflowNames + confirm |
| Task: Retry | RetryTask(..., reason, key), guarded by attempt/revision | reason + dependencies invalidated by retry |
| Session: Resume | new ResumeSession (exact selected session) with supervision | confirm session/task/attempt/worktree/model |
| Session: Stop current run | new StopRun delegating existing stop logic | confirm with the specific RunID |
| Session: Close | CloseSession(..., reason, key) | idle only, reason + confirm |
| Service: Stop | StopService(..., id, key) | confirm name/worktree and ID |
| Every relevant screen: Jump | ResolveNavigationTarget → terminal adapter | selector when there are multiple targets |
| Runtime: Show/Hide managed TUI | SetUIPaneDesired | information about restoration/hiding |

No automatic inbox ACK, message sending, result acceptance, or decision approval. For
actions outside V1, show the state and the relevant existing CLI command as text. Do not
present a button as functional if it ends in only a placeholder.

### 7.1. Forms, Idempotency, and Concurrency

Huh runs as a nested Update/View model in one Bubble Tea program. Do not start a separate
Form.Run(), NewProgram, or stdin prompt while the TUI is running. Cancelling a form does
not call core. Confirm defaults to Cancel.

When the user opens an action, record the target ID, RunID/attempt, and payload. Show
fresh data immediately before confirm. A change to significant fields requires showing
the form again. A read before the write does not eliminate the race — the guard described
below is checked in core under the same lock as the actual mutation.

- A new confirmed intent receives `tui_<ULID>` (core.ID("tui") may be used).
- Double Enter does not start a second operation; one mutation may be in progress per TUI instance.
- Timeout/launch_uncertain: show the code, key, and `Reconcile / Retry same operation`.
  Do not generate a new key automatically. Retry preserves the identical payload and guard.
- After success, fetch a snapshot; the receipt may represent an earlier state.
- New payload = new deliberate intent and new key.
- Do not repeat a mutation in the tick and do not build a general automatic mutation retry.
- When closing the program during a mutation, cancel the context and wait for command
  cleanup; do not abandon a goroutine holding a lock. For an uncertain result, show the
  key in the final message/log and do not claim that cancellation reverted the effect.

New TUI `MutationGuard`: optional ExpectedRevision, ExpectedRunID, ExpectedAttempt, and
for pause-interrupt the set of active RunID/ServiceID values. Add typed guarded variants
for StopRun, PauseInterrupt, and RetryTask. Shared private helpers perform validation and
the existing operation without calling a public method under an already-held s.With or
nested effect with the same key. Checking an existing receipt takes precedence over
comparing the guard with current state, after actor and payload consistency are verified.
The guard is part of the digest. A failed guard returns `revision_conflict` or
`target_changed` and stops nothing. The CLI without a guard retains existing behavior.
`StopRun` must not stop a new Run of the same Session if the process changed during the form.

For pause-interrupt, do not hold one lock across public StopSession calls. Under the
operation effect lock, check the guard under the project lock, write paused, and persist
the exact RunID/ServiceID list to stop; then release the project lock and stop only those
executions through helpers with their own short locks. Each step has a stable subkey from
the execution ID, and a retry reads the persisted intent list. A process already stopped
is a completed step; a new Run is not added to the list automatically. Preserve the
existing order that stops the invoking actor last. The receipt for the whole operation
is created after the steps; an interrupted operation preserves its intent. Use the
existing effect/intent mechanism; do not create a second transaction system.

## 8. Code Architecture and Shared API

```text
cmd/workspace → internal/cli (Cobra, flags, JSON/YAML, TUI startup)
                         ├→ internal/bootstrap (scope, Service, env)
                         ├→ internal/tui (Bubble Tea, views/forms/theme)
                         └→ internal/terminal (TTY, attach/switch, stdio)
internal/tui ──────────────→ internal/core (typed reads and operations)
internal/terminal ─────────→ internal/core (verified runtime targets)
internal/core ─────────────→ existing files/Git/tmux/clients/supervisor
```

No cli/tui/bootstrap/Charm/Cobra imports in core. Core may continue to contain the
existing tmux adapter according to ARCHITECTURE.md. Do not move all process adapters.
Terminal does not know about screens or forms. The TUI does not parse frontmatter/index.json.

### 8.1. New and Changed Files

| File | Responsibility |
|---|---|
| `internal/bootstrap/context.go` | flag/env/CWD → Service, Actor, selected scope; CLI and TUI |
| `internal/core/query.go` | WorkspaceSnapshot, project summaries, consistent read |
| `internal/core/query_relations.go` | task/worktree/session/run/result indexes, counters, and unstyled attention items |
| `internal/core/preview.go` | safe bounded reads of documents/artifacts/check output |
| `internal/core/session_actions.go` | ResumeSession and supervised entry points extracted from the CLI |
| `internal/core/navigation.go` | entity references, validation, and jump-target resolution |
| `internal/core/ui_runtime.go` | durable pane preference/instance and reconciliation |
| `internal/core/tmux_ui.go` | marking/finding/starting/restoring UI and windows |
| `internal/core/runtime.go` | safe orchestrator-pane selection, metadata, and topology |
| `internal/core/supervisor.go` | independent UI reconciliation in Tick |
| `internal/core/lifecycle.go`, `task.go`, `session.go` | guarded-operation helpers without changing old rules |
| `internal/terminal/navigation.go` | I/O and attach/switch/jump; fake port for tests |
| `internal/tui/app.go`, `model.go`, `update.go` | Run, application state, and event routing |
| `internal/tui/backend.go`, `commands.go` | narrow core interfaces and asynchronous Cmd |
| `internal/tui/routes.go`, `keys.go` | navigation history, focus, shortcuts |
| `internal/tui/layout.go`, `theme.go`, `status.go` | dimensions, palette, badges |
| `internal/tui/project.go`, `dashboard.go`, `collection.go`, `detail.go` | screen presentation |
| `internal/tui/forms.go`, `preview.go` | embedded Huh and viewport |
| `internal/cli/tui.go` | tui/show/hide/status and hidden runner |
| `internal/cli/cli.go`, `help.go` | registration, slimmed resume/attach/bootstrap, complete help |

Do not split every panel into its own package. Add tests next to the files and shared TUI
fixtures in `internal/tui/testdata/`. Do not build a separate TUI binary.

For new runtime capabilities, add named narrow interfaces: topology observation, managed
pane operations, and navigation. Tmux implements them, and UI tests have their own fake
with these capabilities. Existing Runtime Launch/Inspect/Stop/Attach may remain
compatible; an unavailable optional capability means runtime_unsupported, not invoking
real tmux in tests. Do not make core depend on a fake TUI.

### 8.2. Queries and DTOs

Add to core.Service:

- `WorkspaceSnapshot(ctx, selector)` → a new type containing Status, Body, Services,
  Checks, Handoffs, Messages, and ObservedAt. Everything comes from one
  s.With/loadDocument, without nested Status/Menu/Inbox calls under the lock. Do not
  return *Document.
- `ProjectOverview(ctx)` → a list of lightweight summaries and per-workspace errors.
  Do not change the fail-fast contract of the existing List. In the new read, one broken
  workspace remains a row with path/error while the others remain available. Enumerate
  using the same storageRoot configuration; do not call fail-fast workspaceDirs in a loop
  that must handle a broken WORKSPACE.md per item.
- `ObserveWorkspaceRuntime(ctx, workspaceID)` → tmux and supervisor topology read with
  a separate timestamp; failure does not invalidate a valid document snapshot.
- `InspectWorktree(ctx, workspaceID, worktreeID)` → HEAD/dirty/error, on demand only.
- `ReadPreview(ctx, workspaceID, kind, resourceID)` → metadata, text, truncated, binary,
  warning/error; kind is a closed document/artifact/check enum.
- `ResolveNavigationTarget(ctx, workspaceID, EntityRef)` → NavigationTarget.

WorkspaceSnapshot is a private contract shared by interfaces inside the binary. It does
not replace `status --json` or increase its schema_version: 2. Do not add UI records to
public sessions/runs/services lists. The snapshot does not expose Mutations/Operations
maps, the supervisor token, or secret configuration.

Build ID maps once per snapshot. Relation DTOs store IDs and history/current flags, not
pointers to a mutable Document. The TUI may sort, filter, and format, but lineage,
activity, authorization, and runtime-ownership rules belong in core. Reads use
loadDocument and existing recovery/migrations. “Read” does not bypass required completion
of a pending write; it does not, however, reconcile tmux, create new decisions, or start processes.

### 8.3. Extracting Existing Use Cases

- New `ResumeSession(ctx, selector, sessionOrLegacyRunID, key)` contains the existing
  session resume/emitSessionResume logic: resolve alias, preserve read-only, parent/
  profile/task/worktree, and ResumeSession; start the supervisor outside the lock, then
  StartSession. CLI and TUI call the same method.
- Add `StartSupervisedSession`, `StartSupervisedOrchestrator`, and
  `ResumeSupervisedAgent`: EnsureSupervisor, then the existing core method. CLI
  start/session start/agent resume use these entry points. The supervisor continues to
  use StartSession/ResumeAgent primitives and does not start itself.
- After a successful orchestrator start, try ReconcileInterface outside s.With. Record
  companion UI failure as a UI error and leave it for retry; do not turn a successful
  Run into failed or return an error implying that agent start did not happen.
- Orchestrator start through session start and agent/session resume must also enable the
  companion. A hook after the StartSession primitive succeeds may centralize this, but
  only after its lock is released, only when the result concerns the orchestrator, and
  without calling EnsureSupervisor inside the hook.
- Move the existing ownership check from CLI session attach into the core resolver.
  Existing attach/open calls the same terminal adapter as the TUI, preserving their
  selector semantics and error format. Keep compatible legacy Run IDs.

## 9. Event Loop, Reads, and Errors

The TUI Model contains: scope, route stack, selection ID, per-route queries, focus,
snapshot, runtime observation, width/height, palette, form, pending action, last
success/error, request generations, and in-flight flags. It does not hold a Service with
a mutable Actor; the injected backend has a fixed scope/actor per operation.

- Init requests a read. Every I/O is a tea.Cmd with context; Update/View do not read files
  or execute Git/tmux. Cmd returns a message; it does not modify the model in a goroutine.
- One timer after a read completes; refresh every 2 s for a workspace and every 5 s for
  a project. When a read is in progress, do not start another. Read timeout is 3 s (the
  lock honors context).
- Changing the workspace increments generation; late snapshot/runtime/preview from
  another scope is ignored. After a mutation, invalidate the previous read generation and refresh.
- Runtime observation is separate, at most one in progress, with a 2 s timeout. Preview
  timeout is 3 s, and Git detail timeout is 3 s. A slow process ends in an explicit error
  and does not block the UI.
- Do not filter changes only by Workspace.Revision: run/check/UI state may have a
  different write cycle. A new snapshot replaces the entire related-record set.
- Do not reset a form during a heartbeat; after a significant target change, mark a
  conflict and block confirm until the intent is refreshed.
- Keep the last valid snapshot on error. Show `Stale · last update …` after a failed
  refresh; after 6 s without a successful read, block new mutations until refreshed.
- When the selected resource is removed: show information, move to the parent and the
  nearest available selection. When a workspace is removed: go to the project screen without crashing.
- unknown/loading/empty/filtered-empty/error/stale views must be distinct.
- An unavailable runtime does not block browsing data; jump/start are unavailable with
  a reason. On Windows, TUI browsing works while tmux actions return runtime_unsupported
  with instructions to use the Linux binary in WSL.
- Logging must not go to stdout during rendering. Errors have a code and readable
  message, with details in the viewport; do not spam a toast on every heartbeat.

### 9.1. Preview and Terminal Safety

ReadPreview resolves the ID from the registry, never an arbitrary path supplied by the
TUI. For documents, the allowlist is WORKSPACE.md/WORKFLOW.md/Input.Snapshot; for an
artifact, the canonical artifacts directory; for check output, the core-approved path.
Use existing containment checks and inspect symlinks before opening; require a regular
file and bounded read, with no FIFO/devices or paths outside the allowed directory. Mark
an artifact digest error explicitly when verification is performed; do not claim full
digest verification after reading only the first 256 KiB.

Every external text (title, reason, output, argv, path) removes terminal control
sequences CSI/OSC/DCS/APC and disallowed control chars BEFORE styling. Test ansi.Strip
itself for OSC52/OSC8; extend the sanitizer if needed. Multiline text preserves
newline/tab (tabs expanded), while a list replaces newlines with spaces. Do not generate
automatic hyperlinks or commands from artifact text.

## 10. Managed Pane and Tmux Reconciliation

### 10.1. Owner, Data, and Desired State

Add a separate `<workspace>/.runtime/ui.json` file, schema_version 1. Do not change
WORKSPACE.md or Registry/Status schema solely because of the UI. Minimal record:

- workspace_id, project_id;
- desired (bool), ui_id (durable `ui_...`), generation (int);
- launch_token (unique per generation), state;
- pane_id, window_id, anchor_run_id;
- last_error, failures, next_retry_at, started_at, updated_at;
- receipts show/hide: key → digest, result (small, explicit contract for this file).

UI states: disabled, waiting_for_runtime, starting, running, backoff.
desired=false takes precedence over process state. No file means default desired=true,
but the file and pane are created only when the workspace has orchestrator history and
its tmux session exists. TUI/project-list reads alone create neither UI nor tmux.

Core performs all ui.json reads/writes under the existing project lock using
atomicWrite. Do not increment Workspace.Revision for heartbeats, focus, or UI recovery.
Unknown schema_version or corrupt ui.json: error, with no overwrite/new pane.

Metadata tmux:

- session: `@workspace_project_id`, `@workspace_id`;
- window: `@workspace_kind=orchestrator|worktree`, `@workspace_worktree_id` when applicable;
- UI pane: `@workspace_kind=tui`, `@workspace_id`, `@workspace_ui_id`,
  `@workspace_ui_generation`, `@workspace_ui_token`;
- agent pane: `@workspace_kind=agent` + existing session_id/run_id;
- service pane: `@workspace_kind=service` + existing service identity.

The window name is a label; @window_id is the identifier. The workspace tmux session
continues to use the TmuxName(workspaceID) name. New Pane/Launch fields must be added
after checking all constructors, fakeRuntime, and tests. Legacy paneOwns continues to
work for agents; the UI never receives @workspace_session_id or @workspace_run_id.

### 10.2. Critical Orchestrator Launch Fix

The current Launch runs display-message on `name:orchestrator` and then respawn-pane -k
on the returned pane. If the TUI is active, it will kill the TUI. Change this BEFORE
adding the UI.

Agent-pane selection algorithm:

1. Find the dedicated window through metadata or verified orchestrator Runs. For legacy
   state without metadata, use existing pane IDs/runner command and add new metadata
   only after confirming the relationship. The window name alone does not prove ownership.
2. If a matching pane for the new Run already exists (Recover), recover it.
3. If an inactive pane from the previous orchestrator Run exists, respawn only after
   comparing previous session/run ownership and confirming that no active reservation
   remains. Pass explicit ReplacePaneID/ReplaceSessionID/ReplaceRunID from under the
   lock into Launch; do not arbitrarily choose any pane from the agent family.
4. Otherwise create a new pane with a split in this window, or a new window if it is
   missing. When creating a new tmux session, start the correct runner directly as its
   first pane; do not leave an unmarked shell placeholder to kill later.
5. Never respawn a pane with kind=tui/service, a foreign shell, or another active Run.
6. A missing orchestrator window while a worker window is alive must be handled by
   creating the window, not by failing at display-message.

Other worktree-window creation must also mark IDs. With duplicate names, do not choose
the last list-windows row. Resolve legacy state through live panes linked to WorktreeID;
report a conflict when ambiguous. Reconcile and stop always verify ownership again before
using a stored pane_id.

### 10.3. Pane Lifecycle

Hidden runner: `workspace ... _tui-exec <ui-id> <generation> <token>`.
The launch command contains the absolute Executable, --project, --workspace, and
--tmux-socket. Every argument passes through shellQuote, like the agent runner.
The fully unique command enables Recover after a failure between split and set-option/save.

`_tui-exec` checks ui_id/generation/token/desired under the lock and writes running only
after a successful claim. If it finds another generation or desired=false, it exits
without rendering and without writing state for the new generation. Claim is single-use:
a second process with the same token cannot overwrite the live owner. PID alone is not
proof of ownership; check the current TMUX_PANE and metadata/command.

The TUI pane is a user interface. The launcher removes inherited identities
WORKSPACE_AGENT_ID/SESSION_ID/RUN_ID/TASK_ID/ROLE/PARENT_* and ORCHESTRATOR_* from its
environment (it does not remove TERM/TMUX/TMUX_PANE/PATH). After a verified claim, the
handler creates a Service with an empty Actor and explicit project/workspace/socket. Do
not change os.Environ globally in the supervisor process. Manual `workspace tui` keeps
the Actor from bootstrap and shows role limitations; it does not elevate it by clearing env.

`SetUIPaneDesired(ctx, selector, desired, key)`:

1. requireUser, project lock, resolve/loadDocument, and ui.json.
2. Replay the receipt by key/digest before a new change. Different desired with the same key = conflict.
3. Write desired and receipt atomically together in ui.json. Hide is effective even when
   tmux is unavailable; the effect of killing the owned pane is reconciled later.
4. After releasing the lock, run ReconcileInterface; the show/hide result has the UI
   effect state. A replayed receipt does not create a second pane. Keep it separate from
   domain receipts described in operations.md; do not put ui.json in Document.PendingFiles.

The receipt has its own operation_id and no Workspace.Revision. Separate the immutable
desired-setting result from the current pane observation: replay returns the same saved
intent result; `tui status` reads the current state. In show/hide JSON, return operation_id
in the standard envelope, without a fabricated document revision. Add a typed emit path
with the supplied OperationMetadata for these commands; do not look up their keys in
Registry.Operations through the existing options.emit. Other emit commands remain unchanged.

Managed `q`/Ctrl+C: first write desired=false, then leave raw/alt screen and exit the
program. The footer has `q hide`; manual TUI has `q quit`. Do not kill the owned pane
from inside the process before the write completes and the terminal is restored; the
supervisor removes its dead pane after verifying ownership. A failed hide write shows an
error and leaves the program running. `tui show` enables it again. External kill-pane,
SIGKILL, or a TUI crash means loss; desired remains true → recovery.

### 10.4. ReconcileInterface Algorithm

Separate method, called by Tick and explicit reconcile and after orchestrator start. Do
not call s.With under an existing project lock. Use a private helper operating on already
read state. Serialize all UI starts with the project lock; tmux has a short timeout, and
do not hold the lock while the TUI runs or while waiting for a claim.

1. Read workspace/ui.json. For archived, require no managed pane; remove only its pane.
   Keep Ui.json/history. Do this even when tickWorkspace normally returns early for archived.
2. Read the topology of the selected socket. A transient tmux error means preserve IDs
   and reservation and write bounded diagnostics/backoff; do NOT treat it as a missing pane.
3. desired=false → remove only a verified UI pane; do not touch agents or services.
4. No tmux session → waiting_for_runtime. UI itself does not restore the entire session,
   start the orchestrator, or restart an interrupted paused/completed workspace.
5. No orchestrator history → waiting_for_runtime; do not create a window only for UI.
6. Recover a compatible UI pane from metadata or the exact runnerCommand/token. A retry
   after losing the split response must adopt the existing pane.
7. A live valid pane → running. User movement/resize does not cause layout to be imposed
   continuously. Update a changed window ID after verification.
8. Verified dead/missing UI: if there is no uncertain reservation and backoff has elapsed,
   write a new generation/token + starting before split/respawn. An existing dead pane may
   be respawned if it is still owned and is in the target window.
9. Target window: the current orchestrator Run window; if no Run is active, its marked
   historical window. If the window disappeared and no Run can be restored,
   waiting_for_runtime; do not start a shell/orchestrator just for the TUI.
10. The split is detached and does not steal focus. Prefer the right side when available
    at width >= 120: TUI 40%, min. 40 columns; otherwise the bottom when height >= 30:
    TUI 35%, min. 12 rows. If no variant meets the minimum while preserving a usable
    agent pane, use waiting_for_runtime with reason `not enough pane space`. The supervisor
    retries after a size change. Do not shrink the agent pane to one row.
11. After creation, write pane/window IDs and metadata. A failure after split is starting
    with an uncertain effect; the next tick must Recover first. Do not reserve a new
    generation until the old command is confirmed absent.
12. With multiple owned UI panes, adopt the one matching the current reservation; remove
    old generations only after confirming workspace/UI ownership. Leave ambiguous foreign
    panes in place and show a conflict.

Backoff for failures/short crashes: 2, 4, 8, 16, 30, 60 s, then a maximum of 60 s;
reset after 30 s of stable running. `tui show` as a new intent may reset backoff.
Do not write ui.json every tick when nothing changed. A renderer crash must not lead to
hundreds of new panes. This algorithm does not restart workers.

A newly created detached tmux session initially receives 160×48 (`new-session -x/-y`),
so a start from a non-terminal CLI can create both panes immediately. This applies only
to a new session; do not change existing client/window dimensions. After attach, tmux
adjusts the size and the TUI moves to the correct layout, possibly Tiny. Lack of space
for a NEW split does not mean removing an existing pane after resize. A starting pane
does not become running merely because it is alive: it must claim. If it remains
starting for 10 s, show a start error; the next recovery step may stop only the verified
owned runner for that generation and enter backoff.

Tick: reconcile agents according to current rules, perform orchestrator recovery, then
UI. Run message delivery and UI so that one error does not skip the other; collect errors
(errors.Join). The TUI is never the only process responsible for restoring itself. A
stopped supervisor means no automatic recovery; show this state and do not start the
supervisor from a TUI read/refresh alone.

Paused: UI remains/restores with an existing session and window, including after pause
--interrupt. Completed: UI remains until archive or hide and helps review results.
Archived: no managed UI; manual `workspace tui --workspace …` may read the archive.
Clean does not count the UI as an active domain process and does not remove ui.json as a worktree.

### 10.5. Migration and Compatibility

No ui.json in an old workspace requires no Registry migration. Add window/pane metadata
only after proving identity through existing records/runner command. Reconcile is safe
for a mix of old and new panes. Do not change sess_* aliases or the public status schema
number. An older binary does not know the new pane; the README warns that downgrading
with active UI requires `tui hide` before replacing the binary because the old
orchestrator launcher does not distinguish panes.

## 11. Jump to Tmux and Terminal Handoff

`EntityRef` has Kind + ID (workspace/worktree/session/run/service/orchestrator/ui).
`NavigationTarget` has workspaceID, the real tmux session name/ID, windowID, paneID,
expected session/run/service/ui ownership, and server socket. It does not accept an
arbitrary tmux target string from a user form.

Resolver:

- Workspace → existing tmux session, without creation; choose session/orchestrator/UI.
- Worktree → window by @workspace_worktree_id, falling back to verified panes of this
  worktree's sessions/services. A missing window is information, not automatic start.
- Session → only CurrentRunID, Run.Active, and a live pane belonging to that Run.
- Run → only while it remains current for the session; a historical Run has Jump disabled.
- Service/UI → verified own identity and live pane.
- If multiple targets are plausible, use an explicit selector; never choose the “first
  matching model”.

Separate `Select` (non-interactive select-window/select-pane/switch-client) from `Attach`
(a process requiring stdin/stdout). The terminal adapter uses argv and timeouts, not shell
strings. For attach, release the terminal through tea.Exec with a custom ExecCommand or
tea.ExecProcess; return/detach/error restores the terminal and refreshes the view.
Inside tmux, ordinary jump uses tea.Cmd and does not stop the TUI loop.

Distinguish the tmux client and server:

- Outside tmux: attach to an explicit socket/session with passed stdio; the TUI returns after detach.
- In the same server: find the client by TMUX_PANE and list-clients/client_tty, and
  explicitly use `switch-client -c CLIENT`. When multiple clients view the same source
  window/pane and one cannot be identified, show a client selector instead of guessing.
- On another server (for example, --tmux-socket points to an isolated server), return
  readable `tmux_server_mismatch` and instructions to attach from a terminal outside
  the current tmux. V1 does not perform nested attach or switch a random client on
  another server.
- With no client attached to managed UI, jump reports `no attached client`. It does not
  start a new terminal or switch someone else's session.
- Validate pane membership in session/window immediately before select. Files and tmux
  cannot share one transaction; show race errors and refresh, without falling back to an
  unchecked pane. Serialize owned start/stop/focus operations with a short core lock;
  do not hold the lock during interactive attach.

Do not set a global return keybinding. In help, describe standard tmux window/pane
selection and `workspace tui show`. Moving to another window does not change the route of
the TUI pane running in the background. After selecting the pane again, it shows a fresh snapshot.

## 12. Tests and Verification

One screenshot of one screen is insufficient. Test transitions, ownership, and failure
windows. In unit tests use fake backend/runtime/clock and deterministic IDs/dates; no
network clients or dependencies on the developer's terminal. Do not write thousands of
identical goldens; the minimum meaningful set follows.

### 12.1. Core / Bootstrap / Compatibility

- Scope: project root, project subdirectory, workspace, checkout subdirectory, external
  workspaces_dir, symlink, explicit flags, inherited foreign workspace, missing config,
  broken frontmatter, project/workspace conflict.
- Snapshot: multiple sessions for the same persona, historical attempts, session without
  a task, reader+writer, services, results with undetermined lineage; no duplicates.
- ProjectOverview shows a healthy workspace beside a broken one; List keeps its old
  fail-fast contract. Runtime timeout does not corrupt the persisted-state snapshot.
- ResumeSession: read-only, persona snapshot, legacy Run alias, closed/active/invalid
  lineage, operation replay; CLI and TUI produce the same state and errors.
- Guarded stop: Run changes between form and operation → new Run untouched. Retrying a
  changed attempt and pause-interrupt with a changed set → conflict. Replay after success
  with a historical guard still returns a receipt, not a revision conflict.
- ReadPreview: symlink/traversal/FIFO/binary/large file/missing file/ANSI/OSC52;
  the limit applies to reads, not only rendering.
- `status/list/session list/run list/menu --json/--short` receive no UI records or new
  schema version. Preserve current CLI tests.

### 12.2. TUI Model

- Enter/Esc/Tab/1–5 and the task → session → run → back link preserve selection/query.
- Substring search, case differences, duplicate names, IDs, empty results; q/g/digits in
  input do not perform actions; Huh receives only the events intended for it.
- Generation A → workspace B → a late snapshot from A does not replace screen B.
- One operation/refresh in flight; double submit, same-key retry, form cancellation,
  target change, error/stale, and read recovery.
- `exited` vs `accepted`, idle + interrupted, ready + active writer, unknown state,
  failed check with exit code; do not mix statuses.
- Sanitizer protects every rendered field; test long Unicode/CJK/combining chars and
  multi-line names without exceeding width.
- Test rendered width/height (after removing ANSI) at 160×48, 120×18, 80×24, 60×40,
  40×12, 30×8, and 1×1. Golden tests for dashboard/task/form in wide/narrow/short,
  plus the base light/no-color palette. Test resize repeatedly in both directions.
- Fake terminal adapter: jump success/failure, attach returns the terminal, cross-server,
  client selection on ambiguity. The program restores the terminal after quit/cancel/error.

### 12.3. UI Recovery — Fake Runtime

- Default without an orchestrator starts nothing. After start, exactly one UI pane.
- Two concurrent ensure/reconcile calls → one pane and one claim token.
- Failure before split, after split/before metadata, after metadata/before ID write,
  before claim, and after claim. A retry adopts the existing pane, not a duplicate.
- Another pane under the same pane_id, wrong token/generation/workspace → no kill/respawn.
- timeout/uncertain tmux ≠ missing. Backoff limits the crash loop.
- q/hide is durable; an external kill is restored. An old runner does not overwrite a
  new generation. Show/hide receipts and hide-vs-launch concurrency.
- paused/completed follow the table; archived cleans only UI. UI does not count toward
  limits/writer/service_active/session_active/clean.
- UI failure does not block agent recovery/delivery and vice versa; supervisor stop does
  not stop panes. TUI reads do not start the supervisor.

### 12.4. Real Tmux, Linux/WSL

Extend the existing fixture and private socket from tmux_integration_test.go,
supervisor_test.go, and workflow_tmux_test.go. Build the binary in t.TempDir, including
in a path with a space and apostrophe. The default test client is a local fixture process.

1. Orchestrator start creates its pane and the TUI in the same window; focus remains on
   the orchestrator. TUI capture-pane contains title/status/Overview.
2. Make the TUI active, end/lose the orchestrator, and resume it. The TUI pane is not
   respawned as an agent; the new Run has correct IDs, and the orchestrator runs beside it.
3. Kill the UI pane → supervisor restores one; kill the entire window → orchestrator and
   UI recovery when the workspace is active. Do not restart workers.
4. Replace/remove UI metadata in a window with a foreign pane; the foreign shell is not killed.
5. q in TUI → desired=false and no restoration; tui show → one pane again.
6. Pause --interrupt → agent and service stopped, UI running. Archived after existing
   gates are met → UI disappears and does not block archive/clean.
7. Resize the window to wide/short and narrow/tall; capture-pane fits text, preserves the
   page, input, and visible status. Small sizes do not crash.
8. Two tmux clients + another socket: jump selects the correct client/window/pane;
   cross-server does not switch a foreign client. Test attach/detach with a pseudo-TTY.
9. Reconcile does not steal focus or change a manually corrected layout on every tick.
10. Also run the existing Session/Run/replay/readonly/supervisor/workflow tests.

Tests wait for a state with a deadline, not arbitrary long sleeps. On failure, capture-pane
and supervisor logs go into test diagnostics. Clean up the private socket and processes
through t.Cleanup. Using send-keys in tests to control the TUI itself is allowed; the
application does not deliver messages to agents this way.

### 12.5. Required Final Commands

```sh
gofmt -w <changed-Go-files>
go test ./...
go vet ./...
WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s
```

On Linux/WSL, build the binary in a new temporary directory and run:

```sh
go build -o "$TEMP_BUILD_DIR/workspace" ./cmd/workspace
python3 scripts/check-install.py "$TEMP_BUILD_DIR/workspace"
```

TEMP_BUILD_DIR must point to a new directory created for this verification outside the
repository. Also check `GOTOOLCHAIN=local` with Go 1.24 and build/test core and TUI on
Windows. If the environment has no WSL/tmux, run the available tests, record the missing
integration precisely, and do not declare tmux criteria passed.

## 13. Implementation Order

Stages are sequential; each leaves compiling code. Do not create the entire TUI in one
large file. Run tests for changed packages after each stage.

### Stage 0 — Inventory and Baseline

- [ ] git status, read the referenced documents and tests; check differences from the plan baseline.
- [ ] Record the current go test ./... result and Go/WSL/tmux availability.
- [ ] Find all Runtime.Attach/Launch calls, Pane/Launch constructors, session resume,
      EnsureSupervisor, and tests that count panes.

Complete when the current integration points are known and existing work has not been overwritten.

### Stage 1 — Shared Core and Bootstrap

- [ ] query.go/query_relations.go and consistent snapshot, projections/relation tests.
- [ ] Preview and read-limitation tests.
- [ ] Bootstrap with preserved CLI semantics; extract ResumeSession and supervised entry points.
- [ ] Core target resolver and shared guard helpers; replay and concurrency tests.
- [ ] CLI uses extracted use cases; old formats continue to pass tests.

Complete when core without UI imports is sufficient to support the action table and screen reads.

### Stage 2 — Safe Tmux Before the Second Pane

- [ ] Session/window/pane metadata and topology read.
- [ ] Fix orchestrator-pane selection, missing-window creation, and recovery.
- [ ] ID-based resolution/navigation; terminal adapter with client/stdio selection.
- [ ] Test legacy pane and active foreign pane in the orchestrator window.

Complete when start/resume cannot overwrite a pane of another role and existing runtime works.

### Stage 3 — TUI Shell and Layout

- [ ] Pinned dependencies, go mod tidy, Go 1.24 check.
- [ ] `workspace tui`, TTY/flag validation, app/model/update/routes/keys/theme/layout.
- [ ] Project picker + Dashboard on a fake backend and real WorkspaceSnapshot.
- [ ] Async refresh, stale/error/loading, and generation fences; no mutation from the tick.
- [ ] Wide/narrow/short/tiny, input, focus, no-color, and terminal-restore tests.

Complete when manual TUI provides a usable project/workspace dashboard in both orientations.

### Stage 4 — All Views and Transitions

- [ ] Tasks/detail/sessions/runs/results and returns with history/query preservation.
- [ ] Worktrees/orchestrator, services, More/Agents/Decisions/CR/Runtime.
- [ ] Filters/name search and preview; separate states and correct counters.
- [ ] Jump from every required level and handling missing/lost targets.

Complete when no required entity is hidden without a navigation path and the dashboard
is not flooded with full lists. Every page has empty/error cases.

### Stage 5 — Actions and Forms

- [ ] Embedded Huh, static dispatch of the action table from section 7.
- [ ] Confirmations, reason, current-target guards, correct keys, and same-key retry.
- [ ] Error details, double-submit blocking, and refresh after the result.
- [ ] Actor-role check in core and stale_actor/forbidden/read-only tests.

Complete when all V1 actions use the same core as the CLI and are tested on a fake backend
and at least representative real core operations.

### Stage 6 — Managed Pane

- [ ] ui.json, SetUIPaneDesired/receipts, hidden _tui-exec, and generation claim.
- [ ] ReconcileInterface, command/token recovery, backoff, split/layout without focus steal.
- [ ] Hook after orchestrator start and Tick; independent UI/delivery errors.
- [ ] show/hide/status CLI, q hide, paused/completed/archived, and concurrency tests.

Complete when manual and managed TUI can coexist, but the supervisor manages only one pane
per workspace; loss and deliberate hiding have different behavior.

### Stage 7 — Integration and Contract Documentation

- [ ] All tests from section 12, especially real tmux and race.
- [ ] README: entry, scope, shortcuts, theme, dimension requirements, q hide vs quit,
      show/hide/status, Windows/WSL, downgrade, and attach behavior.
- [ ] PRODUCT: TUI as an existing interface and task-first navigation.
- [ ] ARCHITECTURE: two UI adapters, bootstrap/terminal, queries, separate ui.json.
- [ ] docs/runtime.md: pane ownership, supervisor UI recovery, exit, and pause/archive.
- [ ] docs/operations.md: guards, TUI keys/retry, and separate UI receipts.
- [ ] docs/tui.md: concise current screen contract, keymap, and layout matrix; do not
      copy the entire plan as current-implementation documentation.
- [ ] help.go: descriptions/arguments/flags for new visible commands; completeness test.
- [ ] TODO.md: mark the first item only after all criteria; do not change releases.
- [ ] Final report: what works, tests, actual limitations, without claims based only on the plan.

## 14. Definition of Done — User-Requirement Matrix

| Requirement | Completion evidence |
|---|---|
| Project → workspace selection | scope test + working project picker, including 0/1/multiple workspaces |
| Workspace → dashboard | correct snapshot and dashboard, including external storage/CWD |
| Aggregate navigation | task → sessions → runs/results; worktree → tasks/sessions/services; orphan/history available |
| No entity flood | dashboard counters only + max. 5 attention/5 activity; full lists after selection |
| Statuses | distinct Task/Session/Run/Worktree, active vs failed/stopped/interrupted/accepted |
| Name search | all collections, case-insensitive substring, query and selection preservation |
| Polished panels/theme | wide/short/narrow, focus, dark/light/no-color, and textual badges |
| Wide/short and narrow/tall pane | automatic layouts, render width/height tests + real tmux resize |
| Tmux session/window/pane jump | verified targets/client/socket, attach/detach, and race errors |
| Automatic TUI beside orchestrator | real start/resume with active TUI does not destroy its pane |
| Lost UI reconciliation | kill/recover, concurrent ensure, crash windows, backoff, and q hide |
| Shared CLI/TUI core | no workspace shell-out, shared resume/guards/queries/navigation |
| Existing CLI behavior | all current tests, old commands, schema, and formats without regression |

If the code differs from the analysis base, preserve this plan's contract while adapting
integration points to the current code. If runtime verification is unavailable, describe
the unexecuted scenarios precisely. Do not replace them with a claim that it “should work”.
