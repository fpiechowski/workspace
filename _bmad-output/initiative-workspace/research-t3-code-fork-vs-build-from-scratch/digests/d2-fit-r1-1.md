# D2 requirement fit — round 1

Read-only review of T3 Code at `0e7346c` (2026-10-12) plus GitHub issues, discussions and PRs. T3 Code is an MIT-licensed Electron desktop app with a TypeScript/Effect monorepo. A Node WebSocket server (`apps/server`) wraps provider runtimes and serves web, desktop and mobile clients. State lives in SQLite: an event store, projections and 61+ numbered migrations. Wire types live in `packages/contracts`. The server exposes an app-owned HTTP MCP endpoint (`/mcp`, key `t3-code`) with about 80 tools in 11 toolkits, and it injects that endpoint into every provider session.

For our requirements, the fork paths for R1, R3, R6, R7 and R8 are largely already built. R5 is mostly present. R4 has PR building blocks but no task DAG and no merge-order model, so it needs a new domain model. R2 is the main architectural friction:
- A thread is bound to exactly one `projectId`, one `branch` and one `worktreePath`.
- Non-git cwd works today, both as "scratch" threads and as a non-git project root.
- A thread spanning several repos or worktrees is not supported upstream. It has been requested often (Discussion #7041, 62 upvotes), and an open community PR (#15752, +2259/-115, 54 files, not reviewed) covers repos inside one project folder.

Development pace is very high: 1542 commits in the last 30 days and 5424 in total. That is a fork-maintenance cost (the subject of another dimension), and it matters for every "medium" rating below.

## Per requirement

### R1. Desktop app, including Windows: **easy (already present)**
- Evidence: Electron 44.4.5 (`apps/desktop/package.json`). The release pipeline builds signed Windows x64 and arm64 NSIS installers (Azure Trusted Signing) and Windows CLI zips (`docs/operations/release.md`). Install is `winget install T3Tools.T3Code` (`docs/user/install.md`). WSL backend is supported.
- Observation from this session: this research agent runs inside a T3 Code desktop session on Windows 11, with `C:\Users\fpiec\.t3\userdata\attachments` as an extra working directory.
- Caveat: many Windows bugs are open, including typing lag #12689, DEP0190 spawn shims #12797, slow PowerShell profile discovery at startup #4403, and WSL connectivity #13738.
- Extension point: none needed. Fork build uses `scripts/build-desktop-artifact.ts` (`dist:desktop:win`). Signing is optional and auto-detected from secrets.
- Change scope: none, apart from rebranding and build config.

### R2. Two session scopes, workspace dir (multi-repo) and single worktree: **(b) easy, (a) medium-hard**
- Thread schema: `OrchestrationV2AppThread` has a single `projectId`, `branch: NullOr`, `worktreePath: NullOr` (`packages/contracts/src/orchestrationV2.ts` ~L364-375). One thread therefore has one cwd.
- Non-git cwd is supported in two ways:
  - "No project" threads run in `~/.t3/scratch/<date-slug-id>`. Branch, worktree and diff controls are hidden "because these folders are not Git repositories" (`docs/user/thread-sidebar.md`).
  - A project root can be a non-git directory that contains nested repos. Issue #12902 reproduces exactly this and only reports a diff-path bug.
  - So the "workspace = plain directory" scope works today in a degraded mode: the agent sees everything, but git UX (diffs, branch, worktree, PR detection) does not see the nested repos. Related open bugs: #10553 (file links resolve against the project, not the repo the file is in) and #13390 (PR suggestions only search the project's repo).
- Extra directories: the Claude adapter builds `additionalDirectories` from only `[cwd, attachmentsDir]` (`apps/server/src/orchestration-v2/Adapters/ClaudeAdapterV2.ts` ~L1061-1067). The ACP runtime supports `additionalDirectories` (`packages/provider-acp/src/server/AcpSessionRuntime.ts` L132, L2300). No user-facing multi-root setting exists.
- Maintainer stance: #14267 (multi-directory, Claude `/add-dir` parity) was closed by maintainer juliusmarminge as a duplicate of Discussion #7041. That discussion is open, has 62 upvotes and 29 comments, and remains unresolved. Community PR #15752 "one project can hold several Git repositories" is OPEN with no review decision. It adds per-repo git actions, diffs and rewinds, plus one worktree per repo for "New worktree", "with no schema or event changes", but only for repos inside the project folder. A community fork (LoganRupe, `multi-repo-workspaces`) adds `.code-workspace` folders outside the project.
- Extension points:
  - adapter option builders (ClaudeAdapterV2 `additionalDirectories`, the ACP runtime, OpenCode adapters);
  - the thread/project contract (new workspace-root list);
  - the VCS layer (`apps/server/src/vcs/*`, `GitVcsDriverCore.ts`);
  - checkpointing (`apps/server/src/checkpointing/CheckpointStore.ts`), which is git-based.
- Change scope:
  - (b) needs nothing.
  - (a) "plain dir, agent sees everything" needs nothing. Full git awareness across the repos is about PR #15752 scale (~2.2k LOC, 54 files) or more if repos sit outside the folder. Adopting or rebasing that PR is the cheapest route.

### R3. Agents read and mutate app state via MCP or CLI, visible in the GUI: **easy (strongest fit)**
- MCP server:
  - Built on Effect `McpServer.layerHttp`, path `/mcp`, protocol 2025-06-18.
  - Composed from 11 toolkit layers in `apps/server/src/mcp/McpHttpServer.ts` L813-881, one `toolkits/<name>/{tools.ts,handlers.ts}` pair each. Toolkits: attachment, device, environment, html, orchestrator, preview, previewControls, project, pullRequests, thread, worktree.
  - Tools include `delegate_task`, `create_threads`, `t3_thread_launch` (workspaceStrategy root, worktree or existing_worktree), `t3_thread_read`/`send`/`update`, `link_pull_request`, `watch_pull_request`, `schedule_task`, `t3_project_create`/`clone`, `t3_worktree_*` and `t3_queue_*`.
- Credentials: per provider session, `McpSessionRegistry` mints a scoped bearer token (environment, thread, provider instance, provider session) with capabilities `orchestration` plus optional `preview` (`McpSessionRegistry.ts` L140-144).
- Injection per provider (`docs/orchestration-v2/orchestrator-mcp-server.md`):
  - Codex: `-c mcp_servers.t3-code.url=…`.
  - Claude SDK: `mcpServers["t3-code"]` with type http and `allowedTools: mcp__t3-code__*`.
  - Cursor: SDK `mcpServers`.
  - ACP (Grok and registry agents): `session/new` `mcpServers`.
  - Pi: an extension bridge.
  - OpenCode: registered per directory via `client.mcp.add`, with permission rules limiting the tools to their own thread (`packages/provider-opencode/src/server/v2/adapter.ts` ~L468-517).
- Adding a custom tool: define it with `Tool.make` in a new `toolkits/x/tools.ts`, write handlers, wrap them in `toolkitRegistration(...)`, add the layer to `Layer.mergeAll` in McpHttpServer. Handler construction is centralized through `McpToolAccess` (access checks such as `assertFullAccess` and `assertTargetWithinLimits`). Maintainer guidance says agent-reachable capabilities should be service methods, not handler code (AGENTS.md "Agents" bullet).
- Outside agents: Claude Code, Codex or ChatGPT can connect to the same `/mcp` over OAuth with read-only to full-access scopes (`docs/user/outside-agents.md`).
- CLI (`t3` bin, `apps/server/src/cli/*`): has `project add/remove/rename`, `auth`, `pair`, `connect`, `serve/start`, `service`, `update`, `browser`, `trace`. There are no thread or task CLI commands, so agent-side state access is MCP-first.
- User-configured extra MCP servers: provider-native config appears to be honored. `docs/user/composer.md` says "After you add or change skills, plugins, or MCP servers, use Restart agent session". The Claude capability probe deliberately sets `mcpServers: {}` "isolating the periodic health check from configured MCP servers" (`apps/server/src/provider/ClaudeProvider.ts` L210-239), which implies normal sessions load configured servers. Issue #11398 asks to *surface* OpenCode MCP servers in the UI.
- There is no T3-level per-thread MCP server configuration UI (not found).
- Change scope for a custom toolkit: about 3 new files plus 1 registration line, and contracts if the result type crosses the wire.

### R4. Task DAG and computed cross-repo PR merge order: **medium-hard (new domain model; PR primitives exist)**
- Exists:
  - Multi-PR links per thread, cross-repository on the same host (`pullRequests: Array<ThreadPullRequestLink>` in the thread schema; `docs/user/source-control.md` "Linked pull requests").
  - PR watching with agent wake-ups (`watch_pull_request`), GitHub stack display with "Merge stack" and "Rebase stack", auto-settle merged threads.
  - Delegated tasks (`delegate_task`, `task_status`, parent/child lineage), scheduled tasks (`packages/contracts/src/scheduledTask.ts`, migrations 057-058), agent `todo_list` items in the timeline, and plan interaction mode.
- Not found: any task-dependency or merge-order model. Searched `dependsOn|blockedBy|mergeOrder|merge_order` in `packages/contracts/src`, `apps/server/src` and `apps/web/src`: zero non-test hits. GitHub stacks are single-repo and host-native, not a cross-repo DAG.
- Pattern for a new domain model, from recent commit 1f6df6b (2026-10-11, "guided reviews"):
  - migration `apps/server/src/persistence/Migrations/061_*.ts`;
  - persistence repo `apps/server/src/persistence/PullRequestReviewGuides.ts`;
  - contract `packages/contracts/src/pullRequestReviewGuide.ts`;
  - RPC additions `packages/contracts/src/rpc.ts`, `ws.ts`, `clientRpcPermissions.ts`;
  - client state `packages/client-runtime/src/state/*`;
  - web components and routes `apps/web/src/components/...`, `routes/_chat.*.tsx`;
  - MCP toolkit `apps/server/src/mcp/toolkits/pullRequests`.
  - That commit touched about 50+ files.
- If tasks must be thread-graph events, `apps/server/src/orchestration-v2/*` (Orchestrator, ProjectionStore, WireProjection) and the `Migrations/OrchestrationV2` subfolder are involved.
- Change scope: about 30-60 files across server, contracts, client-runtime and web, plus mobile if parity is wanted. This is a self-contained additive slice, mostly free of upstream conflicts if kept in separate tables and a separate toolkit.

### R5. Several repos and several worktrees of one repo in one workspace: **easy-medium**
- Multiple repos means multiple projects, each with its own `workspaceRoot` (`packages/contracts/src/project.ts`).
- Worktrees:
  - Created via `git worktree add -b <new> <path> <ref>` in `apps/server/src/vcs/GitVcsDriverCore.ts` ~L3460. Orchestrated through `VcsProvisioningService.ts`, `git/GitManager.ts` and `project/WorktreeSetupTracker.ts`.
  - Default location is `~/.t3/worktrees`, configurable per machine.
  - Optional setup script per `t3.json`, plus submodule init.
  - Cleanup policy: inactive days, merged, or no commits. "Active sessions and shared worktrees prevent removal" (`docs/user/project-settings.md`).
- One worktree per thread is not enforced. `t3_thread_launch` supports `worktree` (new), `existing_worktree` (attach) and `root`, and `create_threads` inherits the parent's worktree path (orchestrator MCP doc). So N threads can share a worktree, and one repo can have many worktrees.
- Gap: there is no workspace object grouping projects and worktrees, which ties back to R2. Sidebar grouping by directory (#3722) is closed.
- Change scope: a grouping entity, if needed, is part of the R2/R4 work.

### R6. Claude Code and OpenCode integration maturity: **Claude: mature; OpenCode: works, rougher**
- Claude:
  - `ClaudeAdapterV2.ts` is 9,304 lines and uses the Claude Agent SDK.
  - Features: resume and `resumeSessionAt`, subagents, skills, auto-compact, usage limits, multiple instances via `CLAUDE_CONFIG_DIR` (`docs/user/providers-claude.md`).
  - Replay-fixture recording script exists (`record:claude-replay`).
  - 115 open issues mention "claude" in the title. Claude is a high-use provider, so this count is not normalized.
- OpenCode:
  - Two adapters, `packages/provider-opencode/src/server/adapter.ts` (3,778 lines) and `v2/adapter.ts` (4,496 lines).
  - Requires OpenCode ≥1.14.19; 1.x is labeled "Limited support". OpenCode 2 ≥2.0.18 is auto-detected (`docs/user/providers-opencode.md`).
  - Local or external server, plan mode via the `plan` agent.
  - 41 open and 159 total title-matching issues. Several concern T3 MCP tool injection reliability on OpenCode 2: #16141 (first turn misses MCP tools), #16768 (tools lost after idle eviction), #15736 (intermittently absent), #10485 (`mcp.add` 500). Others: #11458 (Auto mode ignores user permission patterns), #11871 (project-local agents missing).
- Extension point: provider driver plus `orchestrationAdapter` per `docs/internals/adding-a-provider.md` (not read this round).
- Change scope: none for v1 use. Expect to debug the R3 MCP-on-OpenCode bugs.

### R7. State survives restarts and provider switching: **easy (already designed and implemented)**
- Durable state: SQLite `statev2.sqlite` with event store and projections (`apps/server/src/persistence/*`, `OrchestrationEventStore.ts`).
- Provider switching is "a first-class V2 feature". A thread holds runs from multiple providers. On return to a provider, its native thread is resumed with a delta handoff; otherwise a fresh provider thread is started with a budgeted context selection (`docs/orchestration-v2/provider-switching-and-context.md`, `docs/user/portable-handoffs.md`). Implemented in `apps/server/src/orchestration-v2/ContextHandoffService.ts` and `ContextHandoffDelivery.ts`.
- Handoffs do not copy reasoning, tool state or attachments. The agent can fetch omitted history via `t3_thread_read`. Keybinding `mod+shift+up/down` switches providers.
- Restarts: OpenCode reconnects "to the same OpenCode session" on the next prompt. "Restart agent session" continues the conversation. Restarting the background service interrupts running turns (`docs/user/background-service.md`).
- Change scope: none.

### R8. Server mode with phone and browser access: **easy (already present)**
- Surfaces: `apps/mobile` (React Native iOS/Android, in stores), `apps/web` (hosted app.t3.codes or local), `t3 serve` and `t3 service install`.
- Remote options (`docs/user/remote-access.md`): LAN or private-network pairing, Tailscale HTTPS (`packages/tailscale`), desktop-managed SSH (`packages/ssh`), and T3 Connect.
- T3 Connect: tunnel plus a relay in `infra/relay`, a Cloudflare Worker and Durable Objects deployed via alchemy, using Clerk identity (`docs/internals/t3-connect.md`). A fork would have to self-host the relay or skip it and use Tailscale or LAN instead.
- `dev:share` is a dev-runner flag for sharing a dev server (`scripts/dev-runner.ts`).
- Maintainers treat "remote ready" and "multi-surface" as non-negotiable (AGENTS.md). Each new fork feature is expected to support web, desktop and mobile, which raises the cost of R2 and R4 if the fork keeps parity.

## Claims

1. Desktop app is Electron 44.4.5 | repo:apps/desktop/package.json@0e7346c | pingdotgg/T3 Tools | 2026-10 | accessed 2026-10-12 | high | fact
2. Release builds signed Windows x64/arm64 NSIS installers and Windows CLI zips | repo:docs/operations/release.md@0e7346c | T3 Tools | 2026-10-11 | accessed 2026-10-12 | high | fact
3. Windows install via `winget install T3Tools.T3Code` | repo:docs/user/install.md@0e7346c | T3 Tools | 2026-10-10 | accessed 2026-10-12 | high | fact
4. Numerous open Windows-specific bugs (e.g. #12689 typing lag, #12797, #4403, #13738) | gh issue list pingdotgg/t3code "windows in:title" | GitHub | 2026-09/10 | accessed 2026-10-12 | high | fact
5. A thread has a single projectId, branch and worktreePath | repo:packages/contracts/src/orchestrationV2.ts@0e7346c (~L364) | T3 Tools | 2026-10-10 | accessed 2026-10-12 | high | fact
6. Threads without a project run in non-git folders under ~/.t3/scratch, with git controls hidden | repo:docs/user/thread-sidebar.md@0e7346c | T3 Tools | 2026-10-07 | accessed 2026-10-12 | high | fact
7. A project root can be a non-git directory containing nested repos (bug report reproduces it) | gh issue #12902 | GitHub user report | 2026-09-25 | accessed 2026-10-12 | medium | fact (user report)
8. Claude adapter passes additionalDirectories = [cwd, attachmentsDir] only | repo:apps/server/src/orchestration-v2/Adapters/ClaudeAdapterV2.ts@0e7346c L1061-1067 | T3 Tools | 2026-10-11 | accessed 2026-10-12 | high | fact
9. Multi-directory request closed by maintainer as duplicate of Discussion #7041, which is open (62 upvotes) | gh issue #14267; gh graphql discussion 7041 | GitHub / maintainer juliusmarminge | 2026-09-29 / 2026-10-10 | accessed 2026-10-12 | high | fact
10. Community PR #15752 "one project can hold several Git repositories" is open, +2259/-115, 54 files, no review decision, "no schema or event changes", repos inside the project folder only | gh pr view 15752; Discussion #7041 comment by Tr1Fecta-7 | GitHub (community) | 2026-10-04..12 | accessed 2026-10-12 | high (status) / medium (described scope) | fact / author claim
11. MCP endpoint `/mcp` is composed of 11 toolkit layers via Layer.mergeAll | repo:apps/server/src/mcp/McpHttpServer.ts@0e7346c L813-881 | T3 Tools | 2026-10-09 | accessed 2026-10-12 | high | fact
12. Per-provider-session scoped bearer credentials with orchestration/preview capabilities | repo:apps/server/src/mcp/McpSessionRegistry.ts@0e7346c L140-162; docs/orchestration-v2/orchestrator-mcp-server.md | T3 Tools | 2026-10-10 | accessed 2026-10-12 | high | fact
13. T3 MCP is injected into Codex, Claude, Cursor, ACP (Grok/registry), Pi, and OpenCode sessions | repo:docs/orchestration-v2/orchestrator-mcp-server.md@0e7346c; packages/provider-opencode/src/server/v2/adapter.ts | T3 Tools | 2026-10-10 | accessed 2026-10-12 | high | fact
14. Outside agents (Claude Code, Codex, ChatGPT) can use the environment's /mcp via OAuth with read-only to full-access scopes | repo:docs/user/outside-agents.md@0e7346c | T3 Tools | 2026-10-07 | accessed 2026-10-12 | high | fact
15. `t3` CLI has project/auth/pair/connect/serve/service/update commands but no thread or task commands | repo:apps/server/src/cli/*@0e7346c | T3 Tools | 2026-10-11 | accessed 2026-10-12 | high | fact (absence from grep of Command.make)
16. Provider-native MCP servers configured by the user are loaded into sessions (restart agent session after changes) | repo:docs/user/composer.md@0e7346c; apps/server/src/provider/ClaudeProvider.ts L210-239 | T3 Tools | 2026-10-10 | accessed 2026-10-12 | medium | inference (not traced in adapter code)
17. Threads can link several PRs, including cross-repo on the same host; agents can watch PRs; GitHub stacks can be merged and rebased | repo:docs/user/source-control.md@0e7346c | T3 Tools | 2026-10-11 | accessed 2026-10-12 | high | fact
18. No task-dependency or merge-order model exists | grep dependsOn/blockedBy/mergeOrder over contracts, server, web src @0e7346c | T3 Tools | 2026-10-12 | accessed 2026-10-12 | medium-high | absence finding
19. A recent domain-table feature (guided reviews) spanned migration, persistence, contracts, rpc, ws, client-runtime, web UI and MCP toolkit, about 50+ files | git show --stat 1f6df6b @0e7346c | T3 Tools | 2026-10-11 | accessed 2026-10-12 | high | fact
20. Worktrees are created with `git worktree add -b`; threads may launch with new, existing or root workspace strategies; shared worktrees exist | repo:apps/server/src/vcs/GitVcsDriverCore.ts L3460; docs/orchestration-v2/orchestrator-mcp-server.md; docs/user/project-settings.md | T3 Tools | 2026-10-10/11 | accessed 2026-10-12 | high | fact
21. Claude adapter is about 9.3k LOC; OpenCode has v1 (3.8k) and v2 (4.5k) adapters; OpenCode 1.x is labeled "Limited support" | repo:wc -l; docs/user/providers-opencode.md@0e7346c | T3 Tools | 2026-10-02/11 | accessed 2026-10-12 | high | fact
22. Open OpenCode bugs on T3 MCP tool availability (#16141, #16768, #15736, #10485) | gh issue list "opencode in:title" | GitHub | 2026-09/10 | accessed 2026-10-12 | high | fact
23. Provider switching inside one thread is a first-class V2 feature with resume-plus-delta or budgeted-handoff strategies, implemented in ContextHandoffService | repo:docs/orchestration-v2/provider-switching-and-context.md, docs/user/portable-handoffs.md, apps/server/src/orchestration-v2/ContextHandoffService.ts@0e7346c | T3 Tools | 2026-10-02 | accessed 2026-10-12 | high | fact
24. Remote access via T3 Connect (relay plus tunnel, Clerk identity), LAN pairing, Tailscale HTTPS, and desktop SSH; mobile app is React Native | repo:docs/user/remote-access.md, docs/internals/t3-connect.md, AGENTS.md@0e7346c | T3 Tools | 2026-10-09/10 | accessed 2026-10-12 | high | fact
25. 1542 commits in the 30 days before HEAD (5424 total) | git log --since=2026-09-12 @0e7346c | T3 Tools | 2026-10-12 | accessed 2026-10-12 | high | fact
26. License is MIT (Copyright 2026 T3 Tools Inc.) | repo:LICENSE@0e7346c | T3 Tools | — | accessed 2026-10-12 | high | fact

## Leads

- Read `docs/internals/adding-a-provider.md` and `docs/orchestration-v2/core-graph-and-data-model.md` to size the cost of adding a domain model inside the V2 event graph versus side tables.
- Diff PR #15752 and the LoganRupe `multi-repo-workspaces` fork. They are the closest prior art for R2(a)+R5 and show which upstream files a multi-repo fork must keep rebasing (checkpointing, VCS, diff panel).
- Verify claim 16: trace whether ClaudeAdapterV2 passes `settingSources` or relies on the SDK default, and whether the OpenCode server loads the user's `opencode.json` MCP entries alongside T3's registration.
- Observation to check: this subagent's session exposes only the pullRequests, preview and device subset of `t3-code` tools, not orchestrator or thread tools. Find out whether delegated or sub-agent sessions get a filtered toolset (`McpToolAccess.ts`, `threadAccess.ts`).
- Issue #4560 (context leaks across projects in parallel tasks) is relevant to multi-repo orchestration reliability.
- The fork-maintenance burden given about 50 commits/day falls under another dimension (D-maintainability).

## Looked for, not found

- A task-dependency, DAG or merge-order model (grep `dependsOn|blockedBy|mergeOrder|merge_order` over contracts, server and web src): none.
- A thread with multiple workspace roots or a user-configurable `additionalDirectories` in contracts or settings: none. Only the ACP runtime option and the Claude adapter's internal list exist.
- A thread or task CLI subcommand in `apps/server/src/cli`: none.
- A T3-level UI or contract for per-thread custom MCP servers: not found. Provider-native config is the path, and #11398 requests surfacing it.
- A plugin or extension system for T3 Code itself (third-party toolkits without editing source): not found. Toolkits are compiled in via `Layer.mergeAll`.
