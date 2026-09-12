# Client adapters

The installed clients' `--help` output was checked on 2026-09-11. Launch flags:

| Adapter | Launch | Resume | Automatic mailbox delivery |
|---|---|---|---|
| codex | `codex app-server --stdio` | `thread/resume` | Native app-server bridge between turns |
| claude | `claude --model MODEL PROMPT` | `--resume THREAD` | Only with configured delivery wrapper |
| opencode | `opencode --model PROVIDER/MODEL --prompt PROMPT` | `--session THREAD` | Only with configured delivery wrapper |
| command | Configured argv | Optional resume_argv | Optional deliver_argv |

Codex runs through a terminal bridge that displays assistant output and tool activity.
Type a message followed by Enter. `/interrupt` requests turn interruption; `/quit`
ends the bridge. Approval requests remain user decisions; `/approve ID`, `/decline ID`
and `/respond ID JSON` answer requests displayed by the bridge.
Single native questions also accept `/answer ID your answer`; options and question
text are displayed without requiring a JSON response.
Project-specific native thread parameters can be supplied with `clients.NAME.thread_params`; the bridge always
sets the selected model and workspace cwd. It does not bypass client permissions.

For other clients, use the client's native TUI. `session bind-thread` associates its
conversation with the workspace Session; later `agent resume` can invoke resume_argv.
Without that association, a new conversation receives the persisted bootstrap.

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
