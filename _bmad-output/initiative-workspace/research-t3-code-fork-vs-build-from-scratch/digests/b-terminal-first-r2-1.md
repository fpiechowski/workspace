# Terminal-first build path — round 2

**Verdict: viable and cheap, with clear limits.** Both CLIs offer documented, per-session injection points, so the app never has to edit user config. Claude Code has `--mcp-config`, `--add-dir`, `--settings`, `--plugin-dir`, `--session-id` and `--append-system-prompt-file`. OpenCode has the `OPENCODE_CONFIG_CONTENT` / `OPENCODE_CONFIG_DIR` env vars. Both expose enough lifecycle signals to drive a status dot (working / needs permission / done):

- **Claude Code:** 33 hook events, command or HTTP hooks, and a `Notification` hook with a `notification_type` such as `permission_prompt`. A new OSC 7501 terminal status protocol also exists.
- **OpenCode:** plugin `event` hooks (`session.status`, `session.idle`, `permission.asked`). The TUI also runs an HTTP server with `/event` SSE.

The OpenCode side is close to structured: the TUI's own server can be pinned with `--port`, and permissions can be answered over HTTP. The Claude side stays limited to signals. You get no structured diffs, no in-app approval UI and no message model unless you also parse the transcript JSONL (`transcript_path` is in every hook payload). Two maintained products (Superset at about 15k stars, Emdash at about 6k) already ship this pattern on node-pty plus hooks. Claude Squad still screen-scrapes.

Main costs:
- Windows ConPTY quirks: swallowed or out-of-order escape sequences, resize problems, and mouse and modifier input loss on Windows 10.
- Hook semantics drift between CLI versions.
- No rich UI unless you add transcript or SSE parsing.

Remote access over WebSocket is easy for the transport. The terminal-fidelity issues are the harder part.

## Per question

### 1. Claude Code flags (cli-reference, accessed 2026-10-12; CLI at v2.1.296)

All of these are documented flags:
- `--mcp-config` (JSON files or strings) and `--strict-mcp-config` ("Only use MCP servers from `--mcp-config`, ignoring all other MCP configurations").
- `--add-dir`. Caveat: it grants file access, but Claude Code "doesn't discover most `.claude/` configuration from these directories".
- Session flags: `--session-id <uuid>`, `--resume` (ID, name or `.jsonl` path), `--continue`, `--fork-session` and `--name`.
- System prompt flags: `--append-system-prompt[-file]` and `--system-prompt[-file]`. Caveat: by default the prompt is recorded on the first request and reused on resume until compaction. `--system-prompt-snapshot off` changes that.
- `--model`.
- `--permission-mode` (default / acceptEdits / plan / auto / dontAsk / bypassPermissions).
- `--settings` (file or inline JSON, session-only, sits above the user, project and local files).
- `--plugin-dir` / `--plugin-url` (session-only plugin).
- Headless mode: `-p --output-format stream-json`, plus `--input-format stream-json`, `--include-partial-messages`, `--include-hook-events` and `--permission-prompt-tool`.

**Official per-session injection without touching user config: yes.** Use `--mcp-config` plus `--add-dir`. Hooks can come from `--settings` or `--plugin-dir`. The settings docs say `--settings` "can set any key your user settings file can set" and that list keys merge across scopes. The docs say hooks can live in plugins (`hooks/hooks.json`). So a bundled app plugin passed with `--plugin-dir` could carry both hooks and MCP. That last point is inferred and not tested.

`--session-id` lets the app pre-assign the UUID. That UUID then appears as `session_id` in every hook payload, which makes correlation easy.

### 2. Claude Code hooks

There are 33 events. Status-relevant ones:
- SessionStart, UserPromptSubmit
- PreToolUse, PermissionRequest, PermissionDenied, PostToolUse, PostToolUseFailure
- Notification
- SubagentStart, SubagentStop
- Stop, StopFailure
- PreCompact, PostCompact
- Elicitation
- SessionEnd

There are five hook types: `command`, `http` (payload sent as the POST body), `mcp_tool`, `prompt` and `agent`. Hooks can also run async.

`Notification` carries `notification_type`. Values include `permission_prompt`, `idle_prompt`, `elicitation_dialog`, `agent_needs_input` and `agent_completed`. Caveat: `idle_prompt` fires only about 60 s after Claude finishes, so use `Stop` for "done".

Status mapping:

| State | Signal |
|---|---|
| Working | `UserPromptSubmit` / `PreToolUse` |
| Done | `Stop` |
| API error | `StopFailure` |
| Needs permission | `PermissionRequest` or Notification `permission_prompt` |
| Ended | `SessionEnd` |

A `PermissionRequest` hook can also return allow or deny. That gives a path to app-side approvals even in TUI mode, though it is not tested.

Changelog 2.1.295 adds the OSC 7501 "Program Status Protocol", which lets a terminal show whether Claude is "working, waiting on you, or done". It is a PTY-native status channel. It is very new, and an open issue reports that the reports are not being sent in 2.1.296.

The changelog also shows hooks keep changing. Examples: an `idle_prompt` misfire fix, and a fix for Notification hooks not firing for permission prompts under Desktop/VS Code. That is a maintenance risk.

### 3. OpenCode

**MCP.** There is no `--mcp-config` flag. Use the `mcp` key in config. To inject per session, set the `OPENCODE_CONFIG_CONTENT` env var ("Inline json config content"; listed as "runtime overrides"). Configs "are merged together, not replaced", so this does not clobber user config. `OPENCODE_CONFIG` (a file path) and `OPENCODE_CONFIG_DIR` (searched for agents, commands, modes and plugins) also exist.

**Extra directories.** There is no add-dir flag. Use `permission.external_directory` glob rules, which can be passed inline. Allowed paths are not part of the workspace.

**Resume.** `--continue`, `--session <id>` and `--fork`, plus `--model`, `--agent` and `--prompt`.

**Status.** Plugins receive `session.status`, `session.idle`, `session.error`, `permission.asked` / `permission.replied`, `message.*`, `session.diff` and `tool.execute.*`. Plugins load from plugin dirs or from npm via the `plugin` key. Superset's shipped plugin uses `session.status` busy/idle, `session.idle`, `session.error` and permission events, and it filters child (subagent) sessions by `parentID`.

**Hybrid option.** "When you run `opencode` it starts a TUI and a server". `--port` and `--hostname` pin it. The app can then subscribe to `/event` SSE, read `GET /session/status`, and answer `POST /session/:id/permissions/:permissionID` while the user sees the real TUI in the PTY. `opencode attach <url>` and `opencode serve` (with `OPENCODE_SERVER_PASSWORD` basic auth) are also available.

### 4. Prior art

| Tool | Approach | Status detection |
|---|---|---|
| Superset (15.1k stars, pushed 2026-10-12) | Terminal-first | Installs lifecycle hooks and command wrappers into each agent's config, guarded by env vars so they do nothing outside Superset terminals. Ships an OpenCode plugin template. Docs: status works only for agents launched via Superset; "some agents lack a waiting-for-input signal"; a "Clear Status" escape hatch handles stuck "running". |
| Emdash (6.0k stars, pushed 2026-10-10) | node-pty "tui-agents" runtime | Runtime-owned hook server. Installs Claude hooks (SessionStart, UserPromptSubmit, Notification, Stop) into the Claude config root `settings.json`, so it touches user config. Has Codex hooks and OpenCode-family plugin files. Its code says Claude Notification events "carry no `notification_type`" and classifies by regex on the message text. That conflicts with current docs, so it is likely stale or defensive. Its risk notes list PTY cleanup, resize, Windows command wrapping, xterm DA-query handling and provider-specific resume. |
| Claude Squad (8.6k stars, pushed 2026-08-20) | tmux | Screen-scrapes the pane for the string "No, and tell Claude what to do differently". |
| Crystal → Nimbalyst | Structured | Repo references `claude-agent-sdk`, plus an embedded ghostty terminal. Supports Claude Code, Codex and OpenCode. |
| Conductor | Not verified | Mechanism (SDK vs CLI) not verified. |
| Pane | Not found | — |

No dedicated trade-off write-up was found. The trade-off is evidenced only implicitly: Superset's "Clear Status" escape hatch and missing-signal caveats, and Emdash's risk register.

### 5. Remote/server mode with PTY

**Transport is trivial.** `@xterm/addon-attach` attaches a terminal to a WebSocket. node-pty supports flow control (`handleFlowControl`), and its README warns that processes run at the parent's permission level and recommends a container when the PTY is internet-exposed. node-pty requires Windows 1809+ with ConPTY, and the latest releases are still `1.2.0-beta.x` (beta.15, 2026-08-03).

**ConPTY pitfalls:**
- Warp (Jan 2025, history) found ConPTY "swallowing the DCS messages", sending OSCs "out of order", and keeping its own grid. Warp forked and bundled ConPTY.
- Open Claude Code issues:
  - #80123: stale width after resize on ConPTY.
  - #99659: Windows 10 inbox ConPTY drops mouse input; Esc/Ctrl+C ignored in attached sessions.
  - #92771: Shift+Enter is indistinguishable from Enter on Windows.
- OSC-based status (7501) could be affected by ConPTY passthrough. That is unverified.

**Alternatives to PTY-over-WebSocket.** Both vendors ship their own remote paths. Claude Code has `claude remote-control` / `--remote-control` (claude.ai or the Claude app). OpenCode has `opencode serve` + `attach`, with mDNS and CORS flags. Those reduce the need to stream raw PTYs to a phone.

## Claims

1. `--mcp-config` loads MCP servers from JSON files/strings; `--strict-mcp-config` ignores all other MCP configs | https://code.claude.com/docs/en/cli-reference | Anthropic | n/a (live doc) | accessed 2026-10-12 | high | fact
2. `--add-dir` grants file access but doesn't discover most `.claude/` config from those dirs | same | Anthropic | n/a | accessed 2026-10-12 | high | fact
3. `--session-id` takes a UUID; `--resume` accepts ID/name/.jsonl path; `--fork-session` exists | same | Anthropic | n/a | accessed 2026-10-12 | high | fact
4. System prompt is recorded on first request and reused on resume until compaction unless `--system-prompt-snapshot off` | same | Anthropic | n/a | accessed 2026-10-12 | high | fact
5. `--plugin-dir` loads a plugin for this session only | same | Anthropic | n/a | accessed 2026-10-12 | high | fact
6. `--settings` sits above user/project/local and below managed; can set any key user settings can set; session-only, no file write; list keys merge across scopes | https://code.claude.com/docs/en/settings | Anthropic | n/a | accessed 2026-10-12 | high | fact
7. Hooks can therefore be injected per session via `--settings` or a plugin's `hooks/hooks.json` without editing user config | derived from claims 5, 6, 9 | — | — | accessed 2026-10-12 | medium | inference (untested)
8. 33 hook events exist, including PermissionRequest, Stop, StopFailure, Notification, SessionEnd, SubagentStart/Stop | https://code.claude.com/docs/en/hooks | Anthropic | n/a | accessed 2026-10-12 | high | fact
9. Hook types: command, http (payload as POST body), mcp_tool, prompt, agent; async supported; plugins can carry hooks | same | Anthropic | n/a | accessed 2026-10-12 | high | fact
10. Notification hook gets `notification_type` (`permission_prompt`, `idle_prompt`, `agent_needs_input`, `agent_completed`, …); `idle_prompt` fires ~60 s after Claude finishes | same | Anthropic | n/a | accessed 2026-10-12 | high | fact
11. PermissionRequest hook can return allow/deny with updatedInput | same | Anthropic | n/a | accessed 2026-10-12 | high | fact
12. Claude Code 2.1.295 added OSC 7501 Program Status Protocol (working / waiting / done) | https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md | Anthropic | 2026-10 (2.1.295) | accessed 2026-10-12 | high | fact
13. OSC 7501 reports reportedly not sent in 2.1.296 even after terminal answers probe | https://github.com/anthropics/claude-code/issues/101114 | GitHub user report | 2026-10-10 | accessed 2026-10-12 | low | anecdote
14. Hook behaviour keeps changing (e.g. fixed idle_prompt firing during background agents; Notification not firing for permission prompts under Desktop/VS Code) | https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md | Anthropic | 2026 | accessed 2026-10-12 | high | fact
15. OpenCode config sources merge; precedence includes OPENCODE_CONFIG (custom), OPENCODE_CONFIG_DIR, OPENCODE_CONFIG_CONTENT (inline runtime overrides); MCP via `mcp` key | https://opencode.ai/docs/config/ | OpenCode (Anomaly/SST) | updated 2026-10-12 | accessed 2026-10-12 | high | fact
16. OpenCode TUI flags: --continue, --session, --fork, --model, --agent, --prompt, --port, --hostname; `attach`, `serve`, `run --format json`, `acp` exist; OPENCODE_SERVER_PASSWORD enables basic auth | https://opencode.ai/docs/cli/ | OpenCode | updated 2026-10-12 | accessed 2026-10-12 | high | fact
17. Extra dirs in OpenCode via `permission.external_directory` globs; allowed paths are not part of the workspace | https://opencode.ai/docs/permissions/ | OpenCode | n/a | accessed 2026-10-12 | high | fact
18. OpenCode plugin events include session.status, session.idle, session.error, permission.asked/replied, message.*, session.diff, tool.execute.* | https://opencode.ai/docs/plugins/ | OpenCode | updated 2026-10-12 | accessed 2026-10-12 | high | fact
19. Running `opencode` starts a TUI and a server; --port pins it; `/event` SSE, `GET /session/status`, `POST /session/:id/permissions/:permissionID` | https://opencode.ai/docs/server/ | OpenCode | updated 2026-10-12 | accessed 2026-10-12 | high | fact
20. Superset detects status via hooks/wrappers installed in each agent's config, env-guarded; status works only for agents launched in Superset; some agents lack a waiting signal; "Clear Status" for stuck state | https://docs.superset.sh/agent-status | Superset | n/a | accessed 2026-10-12 | high | fact
21. Superset's OpenCode plugin uses session.status/idle/error and permission events, filtering child sessions by parentID | https://github.com/superset-sh/superset/blob/main/packages/agent-setup/templates/opencode-plugin.template.js | Superset | repo pushed 2026-10-12 | accessed 2026-10-12 | high | fact
22. Emdash runs TUI agents on PTYs with a runtime hook server; installs Claude hooks (SessionStart, UserPromptSubmit, Notification, Stop) into the Claude config-root settings.json; classifies Notification by regex on message text | https://github.com/generalaction/emdash (packages/plugins/src/agents/impl/claude/hooks.ts; agents/risky-areas/pty.md) | Emdash / General Action | pushed 2026-10-10 | accessed 2026-10-12 | high | fact
23. Claude Squad detects Claude's permission prompt by screen-scraping the tmux pane for a literal string | https://github.com/smtg-ai/claude-squad (session/tmux/tmux.go) | smtg-ai | pushed 2026-08-20 | accessed 2026-10-12 | high | fact
24. Crystal is now Nimbalyst; Nimbalyst references claude-agent-sdk (structured) and embeds a ghostty terminal; supports Claude Code, Codex, OpenCode | https://github.com/nimbalyst/nimbalyst | Nimbalyst | pushed 2026-10-11 | accessed 2026-10-12 | medium | fact
25. `@xterm/addon-attach` attaches xterm.js to a WebSocket | https://github.com/xtermjs/xterm.js/tree/master/addons/addon-attach | xterm.js | xterm 6.0.0 2025-12-22 | accessed 2026-10-12 | high | fact
26. node-pty: ConPTY on Windows 1809+ (winpty removed); flow control option; security warning recommending containers for internet-exposed PTYs; latest 1.2.0-beta.15 (2026-08-03) | https://github.com/microsoft/node-pty | Microsoft | 2026-08-03 | accessed 2026-10-12 | high | fact
27. ConPTY swallows DCS, sends OSCs out of order, maintains its own grid; Warp forked and bundled ConPTY | https://www.warp.dev/blog/building-warp-on-windows | Warp | 2025-01-22 | accessed 2026-10-12 | medium | history (>2y window ok; pattern)
28. Open Claude Code Windows/ConPTY bugs: stale width after resize (#80123), Win10 inbox ConPTY drops mouse / keys in attached sessions (#99659), Shift+Enter == Enter (#92771) | https://github.com/anthropics/claude-code/issues | GitHub reports | 2026-07 to 2026-10 | accessed 2026-10-12 | medium | anecdote
29. Claude Code ships its own Remote Control server mode (`claude remote-control`, `--remote-control`) | https://code.claude.com/docs/en/cli-reference | Anthropic | n/a | accessed 2026-10-12 | high | fact

## Looked for, not found

- An explicit doc sentence that hook definitions supplied via `--settings` merge with user-file hooks. The docs only say list keys merge generally and `--settings` can set any user key, so claim 7 is an inference.
- Format and reliability of `OPENCODE_CONFIG_CONTENT` beyond the one-line description; whether the `plugin` key accepts local file paths.
- Conductor's integration mechanism (SDK vs CLI) and a product named "Pane".
- A dedicated write-up comparing terminal-first vs SDK/ACP GUIs (lost diffs/approvals vs robustness).
- Evidence on multi-client PTY sharing over WebSocket (resize arbitration, backpressure) on Windows specifically.
- Whether OSC 7501 sequences survive ConPTY passthrough.
