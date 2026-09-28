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
2. Run `workspace workflow list --json`. `plan-first` is the default workflow; omit
   `--workflow` to select it. Ask the user with the available choices only when a
   different configured workflow clearly matches, or intent is ambiguous. Use
   `--no-workflow` instead when the user asks for manual orchestration: the
   workspace is active with no workflow, so do not select one for it.
3. For a ticket URL, create a durable project Issue first with
   `workspace issue create --issue URL --operation-key <stable-key> --json`.
   Otherwise use `workspace issue create --input-file <file>` (or the equivalent body
   input) when the work should be tracked as an Issue. Preserve the source URL, relevant
   acceptance criteria and retrieval date. If tracker access is unavailable, ask for the
   description instead of inventing issue contents.
4. Inspect `workspace issue show <issue-id> --json`, then create a Workspace from the
   exact frozen revision with `workspace create --from-issue <issue-id>
   --operation-key <stable-key> --json` (omit `--workflow` for the default `plan-first`).
   Pass `--no-workflow` for an explicit manual workspace.
   For free-form work that is not an Issue, pass the description as the positional intent
   or use `--input-file <file>`. Retain the returned workspace ID; repeat the same
   operation key on transport retry.
5. Run `workspace start --workspace <id> --operation-key <start-key> --json`.
   Return its workspace ID and `workspace attach --workspace <id>` to the user.
   Starting creates the orchestrator's tmux session without changing your current view.

For project-level intake, `workspace issue list`, `issue show`, `issue refresh`, and
`issue update` are read/guarded mutation operations. `issue dispatch <issue-id>` creates
a linked Workspace and can start its orchestrator as one idempotent operation. The
project Dispatcher is managed separately with `workspace dispatcher start|status|stop|attach`;
its session and Runs are project-scoped and must not be confused with a Workspace
orchestrator.

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
The `plan-first` workflow continues after implementation: when all live implementation
tasks are accepted, prepare an integration worktree, delegate one `integrator` task to
merge the accepted heads and record every conflict resolution in INTEGRATION.md, then
present `workspace integration land --expected-revision N --user-confirmed` for the
user's approval. Landing fast-forwards the target branch locally and never pushes.
Complete the workspace with `workspace complete --expected-revision N --user-confirmed`
after landing, or with an explicit `--reason` when there is nothing to integrate. A
manual workspace
never selects or advances a workflow; it uses the same task/worktree/handoff flow with
a fixed limit of three parallel workers and is closed only by the explicit `complete`
operation.

A workspace created by an older binary under the removed `issue-resolution` workflow is
migrated to `plan-first` automatically on load, preserving accepted tasks and results. Its
leftover change-request, live-test, and release history is read-only, and those commands
now return `workflow_capability`.

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
