# Citation check (final): research-t3-code-fork-vs-build-from-scratch.md

Checked 2026-10-12 against the digests, the clone at 0e7346c, `gh` (read-only) and WebFetch. Report not modified.

## 1. Streszczenie pkt 1
- [3][4] thread = one project, at most 1 worktree: **OK.** orchestrationV2.ts thread has a single `projectId` and a nullable `worktreePath`. RuntimePolicy.ts L103-105: `cwd = input.thread.worktreePath ?? project.workspaceRoot`.
- [8][29] no multi-repo Workspace, task deps, merge order, artifacts or handoffs: **OK.** This is a claim of absence. #7041 is open with no maintainer reply (verify-r2-1 C2). [8] lists MCP tools, and none of them model these things. Weak as positive evidence, but not contradicted.

## 2. Streszczenie pkt 2
- [22][23] ~1,500–1,600 commits/month and +380K/−204K: **OK.** The digests give 1,542 in 30 days and 1,595 in September. PR #2829 was +380K/−204K across 1,907 files (d3d4).
- [26][28] forks sync several times a week with agents and drop features in rewrites: **OK (minor caveat).** travisjneuman ran 48 merges in 30 days, with agent auto-sync every 30 min and a "merge agent". TrogonStack ran 20 sync PRs in 30 days, but only ~2/week in July–September. Its PRs are authored by one human (yordis), and the PR #71 body mentions a "sync skill", so the agent claim is only lightly supported for this fork. Feature drops in #71 are confirmed, though several were cases where upstream superseded the fork's version.
- [1][22] Effect-TS 67–99% and multi-surface norm: **OK (wording overstated).** The digest measures the share of *files importing Effect*: 67% of non-test server .ts and 99% of contracts. That is not "67–99% kodu". AGENTS.md has a "Hit every surface" section and names web, desktop and mobile, so the multi-surface norm is supported.

## 3. Streszczenie pkt 3
- ★[77] PTY input truncation on Windows: **OK.** nimbalyst#987 reports messages "truncated or split into multiple pasted-text placeholders", measured on Windows 11/ConPTY with CLI 2.1.220. The issue is now CLOSED, which the report does not mention.
- ★[80] state-detection issue closed NOT_PLANNED: **OK.** claude-code#38184 has state CLOSED, stateReason NOT_PLANNED, closed 2026-04-23.
- ★[88] OAuth merged 2026-10-06, after stable v0.0.45: **OK.** PR #16336 merged 2026-10-06T21:09Z. The latest stable release is v0.0.45, dated 2026-10-02.
- ★[89] OAuth tool calls failed: **MISMATCH (overgeneralised).** The report says "każde wywołanie narzędzia z klienta OAuth kończyło się błędem 400" (every tool call from an OAuth client returned 400), and the appendix says "wszystkie wywołania". What #16906 actually shows:
  - The 400 hit in-session requests that lacked the `MCP-Protocol-Version` header.
  - The affected client was Grok Bot on Cursor's agent platform.
  - The same request with the header returned 200.
  - The issue author says "The client is wrong here".
  - It was closed COMPLETED on 2026-10-09.
  So it was one non-compliant client, not every OAuth client. The Summary's "już raz się zepsuł" (it has already broken once) is still defensible, because the server was stricter than the spec.
- [90] delete-thread dialog risk: **OK.** useThreadActions.ts L479-520 offers deletion of any unshared thread worktree, with no managed-directory check. It only checks for a scratch project and write scope. One caveat: the claim that automatic cleanup skips foreign worktrees lives in `apps/server/src/storageCleanup.ts`, not in the cited `worktreeCleanup.ts`, and that part was not checked.
- [45] Agent SDK Windows bugs: **OK.** #444 (interrupt segfaults claude.exe on Windows) and #359 (stream closed after first tool call on Windows) are both OPEN.

## 4. Rozdział 5 quotes (checked in the clone)
- [19] README L11: **OK.** The quote is verbatim.
- [1] AGENTS.md L13 "A large number of our users run forks.": **OK.** Verbatim.
- [20] CONTRIBUTING L10 "We are not actively seeking outside contributions and have limited review capacity.": **OK.** Verbatim.
- [21] legacy-orchestration-migration.md L50-52: **OK.** The words are verbatim and the ellipsis correctly stands for the omitted clause "any id at or below a future upstream id masks it forever,".

## 5. Rozdział 8
- [41] third parties may not offer claude.ai login; unmodified CLI allowed: **OK.** The live page says: "Anthropic does not permit third-party developers to offer Claude.ai login into their own applications"; Agent SDK developers "should use API key authentication"; and "Nor does it prevent an end user from signing in to the unmodified Claude Code binary with their own Claude subscription".
- ★[44] SDK billing change paused: **OK.** The live page says "We've paused the previously-announced changes to Claude Agent SDK usage" (update dated June 15, 2026). It also says "Claude Agent SDK, `claude -p`, and third-party app usage still draw from your subscription limits", with the page update dated October 7, 2026.
- [42] Thariq quote: **OK (checked against the digest only).** verify-r2-1 records GIGAZINE quoting Thariq Shihipar: "encourage local development and experimentation with the Agent SDK and claude -p … If you're building a business … use an API key". Not live-fetched.

## 6. Rozdział 6 / 13: forks
- [27] aaditagrawal archived because upstream absorbed its features, 27 stars: **OK.** `gh api` gives archived=true and 27 stars. The README reason ("now ships everything this fork added") is confirmed in verify-r2-1.
- [82] t3kanban built in ~10 days and 1,650 behind: **MISMATCH on "~10 days"; 1,650 OK.**
  - Ahead 17, behind 1,650: confirmed live.
  - The repo was created 2026-08-31T19:52Z. The whole task board arrived on 08-31 as one commit, "feat: add task-first workbench", and v0.1.1 shipped 30 minutes after the repo was created.
  - The 08-31→09-10 window holds an upstream merge plus CI, test and branding fixes.
  - The source does not show how long the board took to build, so "dowiózł tablicę zadań w ok. 10 dni" (delivered the task board in about 10 days) is not supported. It could have been shorter or much longer, privately, before the repo existed. This affects the Summary, §11 and §13.

## 7. Rozdział 4 row 3
- [9] ~80 tools in 11 toolkits; a custom toolkit is ~3 files plus 1 line: **OK.** The clone has 11 toolkit directories and 82 `Tool.make("…")` definitions. The toolkit recipe matches d2-fit R3: tools.ts, handlers.ts, a registration, and one `Layer.mergeAll` line, plus contracts if the result crosses the wire.

## 8. Rozdział 9
- [16] `existing_worktree` accepted from any location if it appears in `git worktree list`: **OK on content; wrong file cited.** The check is in `apps/server/src/mcp/toolkits/project/handlers.ts` L27-48 (`assertProjectWorktree`: realPath must be in `listWorktreePaths(workspaceRoot)`, with no location constraint). The cited McpToolAccess.ts holds only the access classes.
- [15] OAuth with no refresh token, 30 days: **OK; part of it is in another file.** McpOAuth.ts L102 has `grant_types_supported: ["authorization_code"]`, and the token response at L536-541 has no refresh_token. The 30-day TTL is `MCP_CLIENT_SESSION_TTL = Duration.days(30)` in `apps/server/src/auth/EnvironmentAuth.ts` L86, not in McpOAuth.ts.
