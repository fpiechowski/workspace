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
preserving the compatible `client_thread_id`. Delivery to the active parent TUI uses
this bounded route sequence on the current Run's endpoint:

1. check `/global/health`;
2. read `/session/{client_thread_id}/message?limit=100` and stop if the marker is
   already present;
3. `POST /tui/select-session` with `{"sessionID":"..."}`;
4. `POST /tui/append-prompt` with `{"text":"..."}` containing the durable message
   marker and inbox instruction, then persist the `appended` phase;
5. `POST /tui/submit-prompt` with `{}`, then persist the `submitted` phase;
6. poll the same bounded history window and mark the message delivered only after the
   marker is observed.

Each TUI mutation must return a 2xx response containing the JSON boolean `true`.
The complete attempt and every request have deadlines (currently 5 seconds per
attempt, 750ms per request, and 2 seconds for readiness). A rejected operation is
retryable; an uncertain append is recorded as `uncertain_append` and is not repeated,
while `appended`, `submitted`, and the legacy-compatible `uncertain` submit phases
retry from submit without appending again. A timeout or other failure leaves the
message undelivered and shows the Run-scoped tmux pending-inbox notification once.
The notification is only a fallback; it is not transport delivery or an ACK. This
active-TUI sequence is deliberately distinct from OpenCode's external
`/session/{id}/prompt_async` request, which can be accepted without appearing in the
visible TUI. No terminal keystrokes or `/tui/clear-prompt` are used.

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

After rebuilding `workspace`, restart the project supervisor so it loads the new
delivery implementation. An already-running OpenCode Session with a valid Run-scoped
endpoint does not need to be recreated.

`client_thread_id` is not a workspace identity or mailbox address. The binding is
unique across active logical Sessions for the same adapter. Manual binding, Codex
thread initialization, OpenCode discovery, and resume all enforce that rule. Delivery
and notification receipts are deduplicated per Run so a successor can take over an
undelivered message without erasing the exact execution that received an earlier one.

Mailbox addressing is separate from native conversation binding. New messages and
handoffs persist `ToSession`; `ToAgent` remains an audit and legacy projection. An
agent's current Run may read, acknowledge, review and receive delivery only for its
exact logical Session. User commands can name a historical Session explicitly, or opt
into an agent-wide historical view with `--agent`. The compatibility `--to` selector
is accepted only when exactly one eligible open Session exists; it never means latest.

When delivery starts, the supervisor reloads the exact target Session and current Run.
For native OpenCode it may restore a missing Run endpoint only from valid loopback
`--hostname` and `--port` flags recorded in that Run's immutable argv. Missing,
malformed or non-loopback flags lead to `restart_required`; the supervisor never
guesses a host or port. A `NotifiedSessionID` is only a fallback notification receipt
and never marks the message delivered.

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
