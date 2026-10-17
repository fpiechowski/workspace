# Sidecar feasibility — round 2

Source: read-only clone of pingdotgg/t3code at HEAD 0e7346c (2026-10-12). All citations are `repo:<path>@0e7346c`. Accessed 2026-10-12.

**Verdict: feasible, with gaps.** A separate local app can drive an unmodified T3 Code through its documented `/mcp` endpoint, acting as an OAuth-authenticated "outside agent". It can create projects, including for non-git directories. It can launch threads with an explicit provider instance, model, first prompt and `existing_worktree` binding, then read, wait on, send follow-ups to, and interrupt them. The desktop app registers a `t3code://` scheme that deep-links to a thread, and `t3 app <path>` opens a project in the running GUI.

**Top 3 gaps**
1. **Auth is interactive and expires.** It uses OAuth authorization-code + PKCE only: no refresh token, no client_credentials, and the session lasts 30 days. Each sign-in needs the user to approve it with a pairing code or an admin browser session. Project creation also needs the **full-access** ceiling.
2. **The surface is very new and changing fast.** The whole thread/project tool set first appeared on 2026-10-02 (`de34391427`, "introduce new orchestrator"). Outside-agent OAuth landed on 2026-10-06. There were 57 commits to `toolkits/` in 60 days. There is no tool-API versioning: the MCP server reports the app's package version. A strictness change landed today: `t3_thread_read` now rejects unknown params.
3. **Workspace model mismatch.** A project is one `workspaceRoot`. `existing_worktree` must be one of *that project's* `git worktree list` entries, so a multi-repo "workspace" means one T3 project per repo, or a non-git parent folder project that can only use `root`. T3 offers no per-project MCP config, so the sidecar's own MCP server reaches agents only through each provider's native config (user/project), which is only partly verified for Claude.

## Per question

### Q1 External control via `/mcp`
- **Documented for outside agents.** "Agents that T3 Code did not start can work with an environment through its MCP server… read projects and threads, start and message threads" (`repo:docs/user/outside-agents.md@0e7346c` L1-6). URL: `<environment-address>/mcp`, with `localhost` allowed on the host (L11-25).
- **Auth.**
  - OAuth authorization-code server, public clients with PKCE, stateless client registration, loopback redirects allowed (`repo:docs/internals/environment-auth.md@0e7346c` L26-37).
  - `grant_types_supported: ["authorization_code"]`; the token endpoint rejects any other grant (`repo:apps/server/src/auth/McpOAuth.ts@0e7346c` L102, L487-488). The token response has no refresh_token (L536-541).
  - Session TTL is `Duration.days(30)` (`repo:apps/server/src/auth/EnvironmentAuth.ts@0e7346c` L86).
  - Approval spends a one-time pairing code, or uses a browser session that holds `access:write` (McpOAuth.ts L432-465). Pairing codes come from Settings → Connections or `t3 auth pairing create` (outside-agents.md L38-40; `repo:apps/server/src/cli/auth.ts@0e7346c` L85-116).
  - The grant is "read-only" or a runtime-mode ceiling. Read-only = `orchestration:read`. Supervised and above add `orchestration:operate`. Full-access also adds settings:write and source-control:write (EnvironmentAuth.ts L88-100; environment-auth.md L39-50).
  - The session subject is `mcp-client` and is accepted only on `/mcp` (environment-auth.md L42-50).
  - `t3 auth session issue` ("scoped bearer access token for headless or remote clients", auth.ts ~L164-173) exists. Whether `/mcp` accepts such a bearer is **unverified**: the `/mcp` middleware accepts provider-session registry tokens or `authenticateMcpClient` tokens (`repo:apps/server/src/mcp/McpHttpServer.ts@0e7346c` L148-175).
- **Outside-client capabilities.** These are `orchestration`, `worktree` and `pull-requests`, and never preview or device. There is no calling thread (`thread: undefined`) (McpOAuth.ts L555-590).
- **Which tools outside clients can use.** Every handler declares an access class (`repo:apps/server/src/mcp/McpToolAccess.ts@0e7346c`).
  - `readsAsCaller` and `actsAsCaller` require a thread caller, so they are **in-session-agent only**: `delegate_task`, `task_status`, `task_cancel`, `create_threads`, `request_secret`, all `preview_*`, `device_*`, `html_*`, `t3_worktree_handoff`, `t3_worktree_status` (`repo:apps/server/src/mcp/toolkits/orchestrator/handlers.ts@0e7346c` L19-98; worktree/handlers.ts L39-46).
  - `writesEnvironment` requires full-access + default interaction mode (`repo:apps/server/src/mcp/threadAccess.ts@0e7346c` L140-147). This covers `t3_project_create`, `t3_project_update`, `t3_project_delete`, `t3_project_clone` and `run_scheduled_task_now`.
  - A read-only client is refused on every write (McpToolAccess.ts `refuseReadOnlyClient`).
- **Tools the sidecar needs, all callable by an outside client with a suitable grant:**
  - `t3_project_create` {title, workspaceRoot?, createWorkspaceRootIfMissing?, defaultModelSelection?, scripts?} (`repo:apps/server/src/mcp/toolkits/project/tools.ts@0e7346c` L63-75; `repo:packages/contracts/src/project.ts@0e7346c` L199-205).
  - `t3_project_list` / `t3_project_read` / `t3_project_update`.
  - `t3_thread_launch` {projectId?, scratch?, title, modelSelection?, runtimeMode?, interactionMode?, workspaceStrategy?, message? (≤120k chars), attachments?} → {threadId, projectId, modelSelection, runId, status} (project/tools.ts L98-160).
    - `modelSelection` = {instanceId, model, options?} (`repo:packages/contracts/src/modelSelection.ts@0e7346c` L16-20).
    - `workspaceStrategy` is one of {type:"root", branch?}, {type:"existing_worktree", worktreePath, branch?}, or {type:"worktree", baseRef, branch?, startFromOrigin?} (`repo:packages/contracts/src/orchestrationV2.ts@0e7346c` L520-536).
    - An outside caller "must pass projectId and gets the project's default model". The handler fails if neither modelSelection nor a project default exists (project/handlers.ts L118-129).
  - `t3_thread_read` {threadId, view?: messages|activity, afterPosition?, limit≤100, …} (`repo:packages/contracts/src/orchestratorMcp.ts@0e7346c` L340-349).
  - `t3_thread_send` {threadId, message, mode?: auto|queue|steer|restart, clientRequestId?} (L426-431).
  - `t3_thread_wait` {threadId, runId?, timeoutMs?} (L445-449).
  - `t3_thread_interrupt` {threadId, runId?, reason?, clientRequestId?} (L460-465).
  - `t3_thread_list`, `t3_thread_search`, `t3_pending_request_list/read/respond` (approvals), `orchestrator_capabilities` (lists provider instances and models), `t3_worktree_list`.
- **Ceiling.** A launched or steered thread may not exceed the client's approved mode ceiling (threadAccess.ts L40-48, L74-84). `t3_thread_launch` has no retry key (orchestrator-mcp-server.md "t3_thread_launch").
- **Practical caveat.** Claude Code aborts HTTP MCP calls after 60 s unless a timeout is set, and `t3_thread_wait` can block up to an hour (`repo:apps/server/src/orchestration-v2/Adapters/ClaudeAdapterV2.ts@0e7346c` L1096-1101). A sidecar client should poll or set a long timeout.

### Q2 Agent-side MCP config
- **Claude.**
  - T3 adds `t3-code` as an HTTP MCP server and registers it after start via `queryRuntime.setMcpServers(...)`, so it stays out of argv (ClaudeAdapterV2.ts L686-735, L1051, L1130-1137).
  - The session query options set **no `settingSources` and no `strictMcpConfig`**. A grep finds `settingSources` only in the capabilities probe (`repo:apps/server/src/provider/ClaudeProvider.ts@0e7346c` L214-235, which uses user/project/local).
  - User reports say user MCP servers do load:
    - Issue #6632 (2026-08): "all user-configured MCP servers from `~/.claude.json` pass through fine".
    - Issue #3089: Claude plugin MCP servers spawn per conversation.
  - A "Launch arguments" provider setting forwards extra CLI flags (#6632 closing comment).
  - Project `.mcp.json` loading inside T3 sessions is **unverified**. The docs do say project `.claude/skills` are read (`repo:docs/user/providers-claude.md@0e7346c` L68-72).
- **OpenCode.**
  - The OpenCode 1.x and OpenCode 2 adapters add T3's server dynamically through the OpenCode API.
    - 1.x registers it as `client.mcp.add({name:"t3-code"…})` (`repo:packages/provider-opencode/src/server/adapter.ts@0e7346c` L975-977).
    - OpenCode 2 registers a per-thread `t3-code-<thread>` remote entry per directory (`repo:packages/provider-opencode/src/server/v2/adapter.ts@0e7346c` L468-482, L3393-3402).
  - OpenCode 2 permission rules deny `t3-code-*` and then allow only the thread's own server, so other T3 thread servers are masked and non-T3 servers are untouched (L486-497).
  - The spawned server keeps a caller- or env-provided `OPENCODE_CONFIG_CONTENT`, else `"{}"`. A code comment says unconditional overriding "previously clobbered the user's opencode config" (`repo:packages/provider-opencode/src/server/OpenCodeRuntime.ts@0e7346c` L44, L55-63, L723-730). So the user's opencode.json MCP servers stay active alongside T3's. Confidence: medium, based on code plus OpenCode merge semantics I did not verify here.
- **Per-project MCP config.** None is managed by T3. No `mcpServers` field exists in the settings/project contracts, and project-settings.md does not mention MCP. Per-project MCP comes only from each provider's native files.

### Q3 Non-git projects and external worktrees
- `t3_project_create` registers any directory and can create it (`createIfMissing`, `repo:apps/server/src/project/ProjectService.ts@0e7346c` L344). I found no git check.
- Checkpointing skips non-git cwds: `isGitCheckpointable` returns before capture (`repo:apps/server/src/orchestration-v2/CheckpointService.ts@0e7346c` L262-272).
- `scratch:true` runs "outside any repository" (project/tools.ts L102).
- Non-git projects are a supported but rough path. Open issues:
  - #17187: rollback is hidden in non-git workspaces.
  - #8665: dot-entries are invisible in the Files panel of a non-git project.
  - #7926: mobile keeps the New worktree default for non-git projects.
  - #18615 (closed as duplicate) and #17309: scheduled tasks saved a worktree strategy and failed in non-git projects.
- **`existing_worktree` outside `worktreesDirectory`: allowed.** The only MCP check is that `worktreePath` (realpath) appears in `git.listWorktreePaths(project.workspaceRoot)` (project/handlers.ts L28-48). ThreadLaunchService then uses the path as given (`repo:apps/server/src/orchestration-v2/ThreadLaunchService.ts@0e7346c` L385-388).
  - A worktree created by the sidecar with `git worktree add <anywhere>` from the project's repo qualifies.
  - A non-git project root yields an empty list, so it cannot use `existing_worktree`.

### Q4 Deep links / GUI handoff
- The desktop app handles `t3code://new?…` and `t3code://threads/<environmentId>/<threadId>?prompt=…` via `open-url`, `second-instance` and startup argv (`repo:apps/desktop/src/app/DesktopClerk.ts@0e7346c` L192-230; `repo:packages/shared/src/promptLinks.ts@0e7346c` L216-249).
- It registers the scheme through `setAsDefaultProtocolClient` (`repo:apps/desktop/src/electron/ElectronApp.ts@0e7346c` L168-173; Linux handler in DesktopLinuxUrlHandler.ts).
- Hosted `https://app.t3.codes/<env>/<thread>?prompt=` links are documented (`repo:docs/user/prompt-links.md@0e7346c`).
- `t3 app [path]`: "Open a project in the running T3 Code desktop app" (`repo:apps/server/src/cli/app.ts@0e7346c` L250-259).
- `t3 browser` only sets up the headless preview browser (`repo:apps/server/src/cli/browser.ts@0e7346c` L102-190). It is not a deep-link tool.
- Agents write `[title](t3-thread://v1/<threadId>)` links inside messages (orchestrator-mcp-server.md).

### Q5 Stability
- **Not versioned.** The MCP server's `version` is the app `packageJson.version` (McpHttpServer.ts L863). I found no stability or deprecation policy in the docs.
- **Commits since 2026-08-12:**

  | Path | Commits |
  |---|---|
  | `apps/server/src/mcp/toolkits` | 57 |
  | `packages/contracts/src/orchestratorMcp.ts` | 9 |
  | `McpHttpServer.ts` | 18 |
  | `McpToolAccess.ts` | 3 |
  | `McpOAuth.ts` | 2 |
  | `docs/user/outside-agents.md` | 2 |

  The repo had 2,946 commits overall in the same window.
- **Tool ages.** `git log -S` shows `t3_thread_launch`, `t3_project_create`, `t3_thread_send`, `t3_thread_read` and `create_threads` all first appear in `de34391427` (2026-10-02).
- **Notable recent changes:**
  - #15219 "T3 MCP tools take explicit thread and project targets" (2026-10-04).
  - #16335 "every T3 MCP tool declares who may call it" (2026-10-06).
  - #16336 outside-agent OAuth (2026-10-06).
  - #17791 launch now refuses unresolvable base refs (2026-10-10).
  - #17823 "Reject unknown params in t3_thread_read" (2026-10-12).
- No tool removals in 60 days except a `preview_status` move during a toolkit split (1f0a14cf7f, 2026-09-08).
- **Filtered subsets:**
  - Read-only Claude sandboxes pre-approve only a fixed read-only tool list (ClaudeAdapterV2.ts L1072-1093, L1103-1127).
  - The provider-session credential omits `preview` when browser access is off (`repo:apps/server/src/mcp/McpSessionRegistry.ts@0e7346c` L19-24, L140-145).
  - Outside clients never get preview or device.
  - `tools/list` does not appear to be filtered per caller: refusals happen at call time, and issue #17202 says agents still see `preview_*` and `device_*` with access off. Delegated children get no special subset beyond mode narrowing (orchestrator-mcp-server.md "Policy And Idempotency").

### Q6 `t3` CLI
- `t3 project add <path> [--title]`, `t3 project remove <id|path>` (with a delete-all-threads flag), and `t3 project rename <id|path> <title>` (`repo:apps/server/src/cli/project.ts@0e7346c` L448-577). `add` dispatches `project.create` and refuses an already-registered root.
- Also: `t3 app [path]`, `t3 auth pairing create|list|revoke`, `t3 auth session issue|list|revoke` (auth.ts L85-248), `t3 connect`, `t3 service`, `t3 theme`, `t3 trace`, `t3 browser setup`.
- I found no CLI command to launch or message threads; that is MCP-only. An external app can shell out to `t3 project add` and `t3 app`. `--base-dir` and location flags select the environment.

## Claims
1. Outside agents can start, message and stop threads via `/mcp` | repo:docs/user/outside-agents.md@0e7346c L1-6, L50-53 | pingdotgg | 2026-10-06 (added 2c8be5893e) | accessed 2026-10-12 | high | documented-feature
2. Outside-client auth is OAuth authorization_code + PKCE only, no refresh token, 30-day session, approval needs a pairing code or admin browser | repo:apps/server/src/auth/McpOAuth.ts@0e7346c L102, L432-541; EnvironmentAuth.ts L86 | pingdotgg | 2026-10-06 | accessed 2026-10-12 | high | code
3. Project create/update/delete/clone require a full-access ceiling client | repo:apps/server/src/mcp/McpToolAccess.ts@0e7346c (writesEnvironment); threadAccess.ts L140-147; project/handlers.ts L209-261 | pingdotgg | 2026-10 | accessed 2026-10-12 | high | code
4. `delegate_task`, `create_threads`, `task_*`, preview, device and html tools are thread-caller-only, so unavailable to outside clients | repo:apps/server/src/mcp/toolkits/orchestrator/handlers.ts@0e7346c L19-98; McpOAuth.ts L555-590 | pingdotgg | 2026-10 | accessed 2026-10-12 | high | code
5. `t3_thread_launch` accepts projectId, modelSelection {instanceId, model}, modes, workspaceStrategy (root/existing_worktree/worktree) and message | repo:apps/server/src/mcp/toolkits/project/tools.ts@0e7346c L98-160; orchestrationV2.ts L520-536 | pingdotgg | 2026-10-02 | accessed 2026-10-12 | high | code
6. `existing_worktree` must be in the project's `git worktree list`; it has no location constraint | repo:apps/server/src/mcp/toolkits/project/handlers.ts@0e7346c L28-48 | pingdotgg | 2026-10 | accessed 2026-10-12 | high | code
7. Non-git project roots are accepted; checkpoint capture is skipped for non-git cwds | repo:apps/server/src/orchestration-v2/CheckpointService.ts@0e7346c L262-272; project/tools.ts L66 | pingdotgg | 2026-10 | accessed 2026-10-12 | high | code
8. Non-git projects have open UX bugs (rollback hidden, scheduled-task failures, mobile defaults) | gh issues #17187, #17309, #18615, #7926, #8665 | GitHub users | 2026-08..10 | accessed 2026-10-12 | medium | issue-tracker
9. The Claude session adds `t3-code` via setMcpServers and sets no settingSources or strictMcpConfig | repo:apps/server/src/orchestration-v2/Adapters/ClaudeAdapterV2.ts@0e7346c L686-735, L1001-1060 | pingdotgg | 2026-10 | accessed 2026-10-12 | high | code
10. User `~/.claude.json` MCP servers are active in T3 Claude sessions | gh issue #6632 | GitHub user | 2026-08-14 | accessed 2026-10-12 | medium | user-report
11. OpenCode gets T3's MCP via dynamic `mcp.add`; the user's OpenCode config is preserved (OPENCODE_CONFIG_CONTENT respected, else "{}") | repo:packages/provider-opencode/src/server/adapter.ts@0e7346c L975-977; v2/adapter.ts L3393-3402; OpenCodeRuntime.ts L55-63, L723-730 | pingdotgg | 2026-10 | accessed 2026-10-12 | medium | code
12. T3 has no per-project MCP server configuration | grep of packages/contracts/src/settings.ts, project.ts, docs/user/project-settings.md @0e7346c | pingdotgg | 2026-10 | accessed 2026-10-12 | medium | absence
13. The desktop app handles `t3code://threads/<env>/<thread>` and `t3code://new?…` deep links | repo:apps/desktop/src/app/DesktopClerk.ts@0e7346c L192-230; packages/shared/src/promptLinks.ts L216-249 | pingdotgg | 2026-10 | accessed 2026-10-12 | high | code
14. `t3 app [path]` opens a project in the running desktop app; `t3 project add|remove|rename` exist | repo:apps/server/src/cli/app.ts@0e7346c L250-259; cli/project.ts L448-577 | pingdotgg | 2026-10 | accessed 2026-10-12 | high | code
15. MCP tool surface is unversioned (server version = app version) | repo:apps/server/src/mcp/McpHttpServer.ts@0e7346c L863 | pingdotgg | 2026-10 | accessed 2026-10-12 | high | code
16. Core thread/project MCP tools first appeared on 2026-10-02; there were 57 toolkit commits in 60 days | git log @0e7346c (de34391427; `--since=2026-08-12`) | pingdotgg | 2026-10-12 | accessed 2026-10-12 | high | repo-history
17. `t3_thread_read` began rejecting unknown params on 2026-10-12 | git log ee2763e9b2 (#17823) | pingdotgg | 2026-10-12 | accessed 2026-10-12 | high | repo-history
18. Claude HTTP MCP calls time out at 60 s by default; T3 wait tools block up to 1 h | repo:apps/server/src/orchestration-v2/Adapters/ClaudeAdapterV2.ts@0e7346c L1096-1101 | pingdotgg | 2026-10 | accessed 2026-10-12 | high | code-comment
19. `tools/list` is not filtered per caller; tools are refused at call time | McpToolAccess.ts design + gh issue #17202 | pingdotgg / GitHub user | 2026-10-08 | accessed 2026-10-12 | medium | inference

## Leads
- Check whether `t3 auth session issue` bearer tokens are accepted on `/mcp`. Read `EnvironmentAuth.authenticateMcpClient`, which probably requires subject `mcp-client`. If they are accepted, it removes the interactive-approval gap.
- Check whether the approval form (`apps/server/src/auth/mcpOAuthHttp.ts` ~L113-150) can be POSTed programmatically with a pairing code. That would allow headless sidecar sign-in.
- Test in a live session whether project `.mcp.json` / settings.json MCP servers load in T3-launched Claude sessions, given the absent `settingSources`. Agent SDK 0.3.276.
- Check whether a WebSocket/RPC API (`orchestration:operate` over WS) would be a richer surface for a sidecar than MCP. Event streams would avoid polling. environment-auth.md says MCP-client sessions are rejected there, but a paired client session may work.
- Read `ThreadManagementService` for how an outside client could subscribe to thread updates instead of polling `t3_thread_read` / `t3_thread_wait`.

## Looked for, not found
- A stability or versioning policy for MCP tools, or a deprecation notice.
- A client_credentials / refresh-token / long-lived API-key path for `/mcp`.
- A CLI command to launch or message threads.
- T3-managed per-project MCP server config.
- `settingSources` or `strictMcpConfig` in the Claude session options.
- A `worktreesDirectory` restriction on `existing_worktree`.
- A git requirement in project creation.
