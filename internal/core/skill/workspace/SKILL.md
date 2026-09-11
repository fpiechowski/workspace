---
name: workspace
description: Start or resume delegated issue-resolution work in a Git project using the workspace CLI, persistent agent personas, worktrees and tmux sessions.
---

# Workspace

Use the installed `workspace` CLI from the project directory. An Agent is a persona
definition; a Session is one execution. The workspace orchestrator delegates code
changes and keeps durable state in WORKSPACE.md; workers return handoffs and artifacts.

For a new issue or work description:

1. Run `workspace doctor --json`. If the project is uninitialized, use `workspace
   project init`. Session execution requires Linux/macOS or the Linux binary in WSL,
   tmux, and configured model profiles. Inspect `profile list` and `client list`.
   Preserve existing configuration and the user's chosen clients/models.
2. Run `workspace workflow list --json`. Select a clearly matching workflow from
   the user's input; ask with the available choices if intent is ambiguous. A
   workspace without `--workflow` starts in `needs_workflow` for interactive selection.
3. For a ticket URL, `create --issue URL` retrieves supported/configured trackers.
   Otherwise save the supplied description or retrieved ticket text as an input file. Preserve
   the source URL, relevant acceptance criteria and retrieval date. If tracker access
   is unavailable, ask for the description instead of inventing issue contents.
4. Use `workspace create --title <title> --input-file <file> --workflow issue-resolution
   --operation-key <stable-key> --json`; include `--issue <url>` when applicable.
   Retain the returned workspace ID; repeat the same operation key on transport retry.
5. Run `workspace start --workspace <id> --operation-key <start-key> --json`.
   Return its workspace ID and `workspace attach --workspace <id>` to the user.
   Starting creates the orchestrator's tmux session without changing your current view.

For existing work, inspect `workspace status --workspace <id> --json` and
`workspace menu --workspace <id> --json`. Reuse the existing orchestrator Agent;
`agent resume orchestrator` creates a new Session if the previous one has ended.
Do not start a second execution because a busy agent has not answered yet.

Inside an orchestrator Session, read WORKFLOW.md and WORKSPACE.md and use the CLI's
task, worktree, agent, session, inbox and handoff commands. Durable messages address
Agent IDs; native thread IDs and tmux pane IDs are not workspace mailbox addresses.
Use `--json --non-interactive` for machine-readable operations. On `decision_required`,
present the actual choices to the user and record their answer. Preserve existing
authorization; invoking this skill alone does not authorize external publication.

Workers persist outputs in their worktrees and submit explicit files through handoff.
The orchestrator reviews preserved artifacts before acceptance. A merge or a successful
test does not finish issue-resolution: wait for the user's deployment/release confirmation.
