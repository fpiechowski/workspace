---
title: "Product Brief Addendum: Workspace"
created: 2026-10-08
updated: 2026-10-08
---

# Addendum: Workspace

Depth that does not fit the brief but should carry into the PRD and architecture.

## Reference scenario: one Jira ticket, three repos, seven PRs

The user described this journey. It is **one example workflow** (plan, then implement), not the core of the product: Workspace does not impose a workflow. "Orchestrator", "planner" and "worker" are the user's own role labels, not domain concepts. Since this was written, `WORKSPACE.md` has become the `intent` field of the Workspace object, `PLAN.md` has become an Artifact, and Project-scope sessions have moved out of v1.

A worker's handoff contains: a summary of changes, doubts, open questions, decisions needed, and references to the artifacts the worker produced.

1. **The ticket.** An 8 SP Jira issue. Done by hand, it became a stack of 5 PRs in the service repo so reviewers get small pieces. The same ticket also needs changes in the infrastructure repo (GitOps) and small changes in the service's BFF repo.
2. **Create the Workspace.** A Project (the set of repos) already exists in the app. The user creates a Workspace named after the Jira issue key and title. The app creates a plain directory, not a git repo, and seeds it with:
   - `AGENTS.md`: the orchestrator's instructions for this Workspace, from a template;
   - `WORKSPACE.md`: a description of what this Workspace is working on.
3. **Planning.** The user starts an orchestrator session and asks for an implementation plan. The orchestrator reads the issue and hands planning to a separate agent at Workspace scope. That agent's profile, "planner", sets its provider and model. The planner analyses the codebase and the issue and writes `PLAN.md` as a Workspace artifact that every agent in the Workspace can read.
4. **Approval.** The user reviews and approves the plan, which is broken into Tasks.
5. **Implementation.** The user tells the orchestrator to implement. The orchestrator creates worktrees in the right repos, for example 5 in the service repo (one per PR), 1 in the infra repo and 1 in the BFF repo. It hands Tasks from the plan to worker agents, one per worktree.
6. **Progress and control.** Workers report results to the orchestrator, and the user follows progress in the GUI. Because each Task runs in its own worktree, the user can go back to any Task to ask questions, request changes or change the approach.
7. **Outcome.** Everything needed to resolve the issue lives in one organised place.

### Session scopes (from `model.puml` and the user's explanation)

- **Project scope:** creating new worktrees and checking the state of existing ones.
- **Workspace scope:** the orchestrator and the planner, which hand out work within one Workspace.
- **Worktree scope:** implementers working in a single repo.

## Competitive landscape (web research, 2026-10-08)

Capability key: (a) many repos per task, (b) a git worktree per task, (c) agents from several providers, (d) an orchestrator that hands subtasks to sub-agents, (e) stacked PRs across repos, (f) context shared and kept across agents.

Only Cursor, Warp, Conductor, Vibe Kanban and Superset were checked against their own docs. The other entries come from third-party write-ups and may be inexact.

| Tool | Form | a | b | c | d | e | f | Note |
|---|---|---|---|---|---|---|---|---|
| Conductor | macOS app | workaround | yes | Claude/Codex | no | no | no | Docs say multi-repo "isn't perfect" |
| Vibe Kanban | local app, OSS | yes | yes | yes (10+ CLIs) | no | no | no | No link between the repos. Company wound down in April 2026, now community-maintained |
| Superset | local app | no | yes | yes | no | no | no | CLI, SDK, MCP |
| Claude Squad / Emdash / Parallel Code / Nimbalyst | TUI/desktop | no | yes | yes | no | no | no | Crystal was deprecated in Feb 2026 |
| ivar | OSS (Aug 2026) | yes | yes | ? | ? | one PR per repo | shared HALL.md | Closest to Workspace's model: one view directory per task, read-only reference repos |
| Cursor | IDE | yes (multi-root) | yes | model picker only | yes (/multitask) | no | no | |
| OpenAI Codex | app/CLI | no | yes | OpenAI only | yes | no | no | "Symphony" uses the issue tracker as its control plane |
| Claude Code | CLI | no | yes | Anthropic only | yes (agent teams) | no | partial (task list) | |
| Warp Oz | cloud/enterprise | yes | not mentioned | Claude Code + Codex | yes | no | claims memory across harnesses | Biggest overlap, but in the cloud |
| GitHub Agent HQ | cloud | no (roadmap) | n/a | yes | no | stacked PRs in one repo only (preview since Jul 2026) | no | |
| Devin / Factory | cloud | unclear | n/a | no | yes | no | ? | |
| OpenCode | CLI/desktop | no | no | yes (75+ providers) | partial | no | no | |

**Commodity:** a worktree per task, several agent CLIs side by side, subagents inside one vendor's agent, stacked PRs in a single repo.

**No match found (moderate confidence):**
- PR stacks across repos, ordered by dependency and tracked as one unit.
- A local tool that has (a)+(b)+(c)+(d) together.
- Context and progress kept per ticket that survive changes of provider and session.

Worth watching: Warp Oz and ivar.
