# Client adapters

The installed clients' `--help` output was checked on 2026-09-11. Launch flags:

| Adapter | Launch | Resume | Automatic mailbox delivery |
|---|---|---|---|
| codex | `codex app-server --stdio` | `thread/resume` | Native app-server bridge between turns |
| claude | `claude --model MODEL PROMPT` | `--resume THREAD` | Only with configured delivery wrapper |
| opencode | interactive TUI with loopback server | `--session THREAD` with a new Run endpoint | Native delivery through active TUI, confirmed in session history |
| command | Configured argv | Optional resume_argv | Optional deliver_argv |

Codex runs through a terminal bridge that displays assistant output and tool activity.
Type a message followed by Enter. `/interrupt` requests turn interruption; `/quit`
ends the bridge. Approval requests remain user decisions; `/approve ID`, `/decline ID`
and `/respond ID JSON` answer requests displayed by the bridge.
Single native questions also accept `/answer ID your answer`; options and question
text are displayed without requiring a JSON response.
Project-specific native thread parameters can be supplied with `clients.NAME.thread_params`; the bridge always
sets the selected model and workspace cwd. It does not bypass client permissions.

For OpenCode, each Run receives a unique `127.0.0.1` endpoint and the normal TUI is
started with server flags. The native session ID is discovered through that endpoint
and persisted. Resuming a logical Session creates a new Run and endpoint while
preserving the compatible `client_thread_id`. Delivery posts a visible marker-bearing
prompt to that exact session and accepts it only after the marker appears in history;
HTTP success or process startup alone is not delivery. A busy or unready TUI leaves the
message in the inbox for retry. Delivery, inbox ACK and handoff acceptance are separate
operations. OpenCode with no `deliver_argv` is native by default. A genuinely custom,
non-empty `deliver_argv` remains an external transport and is preserved. The exact
historical bundled argv `python3 {project_dir}/scripts/opencode-deliver.py {thread_id}
{message_file} {message_id}` is migrated from persisted Session snapshots to native
delivery; similar or custom wrappers are not changed. Explicit non-native configurations
remain readable and keep their configured generic adapter behavior.

OpenCode discovery normalizes both the run-scoped HTTP payload (`time.created` and
`time.updated`) and the flat executable-list payload. If a Run-scoped endpoint is
unready or expired, discovery falls back to the recorded `opencode session list`
executable and still applies the CWD, baseline and unique-ownership filters. When an
older idle OpenCode Session has historical Runs but no persisted native ID, resume
correlates candidates to Runs by normalized CWD and a bounded post-start window. It
binds the earliest unambiguous historical conversation to both the logical Session
and its matching Run before creating the successor. Ambiguous or unavailable recovery
fails closed with an instruction to use `workspace session bind-thread`; an empty
successful history keeps the fresh-start path, whose new conversation is bound by
normal discovery.

`client_thread_id` is not a workspace identity or mailbox address. The binding is
unique across active logical Sessions for the same adapter. Manual binding, Codex
thread initialization, OpenCode discovery, and resume all enforce that rule. Delivery
and notification receipts are deduplicated per Run so a successor can take over an
undelivered message without erasing the exact execution that received an earlier one.

A generic wrapper configuration:

```yaml
clients:
  custom:
    adapter: command
    launch_argv: [/path/agent-wrapper, --model, "{model}", --prompt-file, "{prompt_file}"]
    resume_argv: [/path/agent-wrapper, --resume, "{thread_id}", --prompt-file, "{prompt_file}"]
    deliver_argv: [/path/deliver-wrapper, "{thread_id}", "{message_file}", "{message_id}"]
```

The launcher receives arguments without shell interpolation. `{prompt}` inserts the
text as one argument; `{prompt_file}` inserts its path. A delivery wrapper reads the
message JSON, delivers it through the client's supported API, deduplicates message ID
and returns `{"accepted":true}` only after acceptance. `{project_dir}` expands to the
repository root, which lets a project keep its delivery wrapper in `scripts/` without
hard-coding a machine-specific path. A busy TUI is not a transport API.
Workspace never injects text into an unknown terminal state.

Profile `required_capabilities` filters clients before launch. Native Codex advertises
launch/resume/deliver/observe/interrupt. Other adapters derive capabilities from their
configured operations. Generic launch remains useful with explicit inbox polling;
automatic wakeup requires a delivery-capable adapter.

On WSL, use a Linux-writable Codex runtime directory. A runtime shared with Windows
can fail SQLite initialization; configure the client environment deliberately rather
than reusing a live Windows database. `scripts/check-codex-handshake.py` verifies the
installed app-server protocol using a temporary isolated runtime, without creating
a conversation or starting a model turn.
