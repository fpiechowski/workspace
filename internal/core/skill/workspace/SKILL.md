---
name: workspace
description: Start or resume delegated plan-first or explicit manual workspace work in a Git project using the workspace CLI, persistent agent personas, worktrees and tmux sessions.
---

# Workspace

Use the installed `workspace` CLI from the project directory. An Agent is a persona
definition; a Session is a durable logical conversation and a Run is one concrete
client/tmux execution. The workspace orchestrator delegates code
changes and keeps durable state in WORKSPACE.md; workers return handoffs and artifacts.

At the start of a new session, or after context compaction, run `workspace prime` once
to refresh this general guidance from the current binary. `prime` does not report live
workspace state; use `workspace status --workspace <id> --json` and
`workspace menu --workspace <id> --json` for that.

For a new issue or work description:

1. Run `workspace doctor --json`. If the project is uninitialized, use `workspace
   project init`. Session execution requires Linux/macOS or the Linux binary in WSL,
   tmux, and configured model profiles. Inspect `profile list` and `client list`.
   Preserve existing configuration and the user's chosen clients/models.
2. Run `workspace workflow list --json`. Select a clearly matching workflow from
   the user's input; ask with the available choices if intent is ambiguous. A
   workspace without `--workflow` starts in `needs_workflow` for interactive selection.
   Use `--no-workflow` instead when the user asks for manual orchestration: the
   workspace is active with no workflow, so do not select one for it.
3. For a ticket URL, `create --issue URL` retrieves supported/configured trackers.
   Otherwise pass the supplied description as the positional intent or save it as an input
   file. Preserve the source URL, relevant acceptance criteria and retrieval date. If tracker
   access is unavailable, ask for the description instead of inventing issue contents.
4. Use `workspace create "<intent>" --title <title> --workflow plan-first
   --operation-key <stable-key> --json`; include `--issue <url>` when applicable.
   For longer or file-based descriptions, use `--input-file <file>` instead of the
   positional intent.
   Replace `--workflow plan-first` with `--no-workflow` for an explicit manual workspace.
   Retain the returned workspace ID; repeat the same operation key on transport retry.
5. Run `workspace start --workspace <id> --operation-key <start-key> --json`.
   Return its workspace ID and `workspace attach --workspace <id>` to the user.
   Starting creates the orchestrator's tmux session without changing your current view.

For existing work, inspect `workspace status --workspace <id> --json` and
`workspace menu --workspace <id> --json`. Reuse the existing orchestrator Agent;
`agent resume orchestrator` creates a new Run in the same compatible Session; a
changed task attempt, worktree, agent, input lineage, or native thread starts a new Session.
Do not start a second execution because a busy agent has not answered yet.

Inside an orchestrator Session, read WORKFLOW.md and WORKSPACE.md and use the CLI's
task, worktree, agent, session, run, inbox and handoff commands. In a manual workspace,
WORKFLOW.md is a labeled manual-orchestration note: create explicit tasks and worktrees
and finish only with the user-confirmed `workspace complete` command instead of a
release. `session list` is
logical and compact; use `session history <session>` or `run list` for execution history.
Durable messages address
Agent IDs; native thread IDs and tmux pane IDs are not workspace mailbox addresses.
Use `--json --non-interactive` for machine-readable operations. On `decision_required`,
present the actual choices to the user and record their answer. Preserve existing
authorization; invoking this skill alone does not authorize external publication.

Workers persist outputs in their worktrees and submit explicit files through handoff.
The orchestrator reviews the plan and implementation artifacts before acceptance.
The example `plan-first` workflow completes after all implementation tasks are accepted;
it has no integration, publication, live-testing or release gate. A manual workspace
never selects or advances a workflow; it uses the same task/worktree/handoff flow with
a fixed limit of three parallel workers and is closed only by the explicit `complete`
operation.

After a workspace reaches `completed`, inspect `workspace status` and `workspace menu`
before acting. `workspace start` or `agent resume orchestrator` is a conversation-only
continuation and may reuse the compatible orchestrator Session; it does not create
tasks, worktrees, services, checks, handoffs, or release state. An accepted worker
Session may be resumed for consultation only, with its task/attempt/worktree/input/base
lineage unchanged. Address follow-up questions with the exact Session ID when needed.

Do not use `workspace resume` to reactivate completed work, and do not create new
resources or submit task results from a conversation-only Run. If the user explicitly
authorizes new work, inspect the current revision and run
`workspace reopen --reason "..." --expected-revision <revision> --operation-key <key>`.
The operation preserves tasks, artifacts, handoffs, and the base commit, records the
prior document under `history/reopen_ID/`, invalidates derived release/integration/test
state, and requires `--user-confirmed` when invoked by an agent. Archived workspaces
cannot be reopened or resumed.
