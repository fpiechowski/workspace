# Issues and tracker input

Project Issues are durable local inputs. `workspace issue create --issue URL` retrieves
the source once, stores the title, body, source URL, digest, timestamps, and revision in
`.workspace/issues/issue_ID/ISSUE.md`, and keeps earlier revisions under `history/`.
`workspace issue list` and `workspace issue show issue_ID` are read-only; they do not
create the Issue store on an otherwise fresh project. Corrupt sibling records are
reported as error rows without hiding healthy Issues.

GitHub.com and GitLab.com select their installed, authenticated CLI automatically. For
self-hosted services, configure the adapter explicitly:

```yaml
tracker:
  adapter: gitlab
```

The adapters use the documented JSON forms of
[gh issue view](https://cli.github.com/manual/gh_issue_view) and
[glab issue view](https://docs.gitlab.com/cli/issue/view/).
They read issues; they do not post comments or change tracker state.

For other trackers, configure a wrapper using an argument array:

```yaml
tracker:
  adapter: command
  command_argv: [/absolute/path/to/tracker-wrapper, fetch, "{url}"]
```

The wrapper receives the source URL as one argument and returns JSON on stdout:
`{"title":"Issue title","body":"Description and acceptance criteria"}`.
Authentication belongs to the wrapper or installed tracker client. The command has
a 30-second timeout. No shell interpolation is performed by workspace.

Use `workspace issue create "description"` or `--input-file description.md` for manual
intake. Manual Issues are distinct records even when their content is identical. A URL
may be combined with a supplied description; that body is an offline snapshot and does
not invoke the tracker. An inaccessible tracker yields `tracker_unavailable`; no guessed
contents are stored.

`workspace issue refresh issue_ID --expected-revision N` reads the configured source and
creates a new revision only when canonical title/body/source content changes. An
unchanged refresh updates the check timestamp without changing the content revision.
The operation never comments, labels, assigns, or closes the external ticket.

`workspace issue update issue_ID --status open|deferred|closed --reason ...
--expected-revision N` changes only the local status and records the reason for deferred
or closed Issues. All mutating Issue commands accept `--operation-key`; a repeated key
replays its stored result and a changed payload returns `operation_conflict`.

Create a frozen linked Workspace with either:

```sh
workspace create --from-issue issue_ID --operation-key workspace-142
workspace issue dispatch issue_ID --start --operation-key dispatch-142
```

The Workspace input contains the exact Issue revision and digest selected at creation;
later refreshes never rewrite it. Omitting `--workflow` selects `plan-first`; `dispatch`
may also use `--no-workflow`, but workflow and manual flags are mutually exclusive. The
project-scoped Dispatcher is allowed to perform these local operations but cannot
implement code, advance Workspace state, or mutate the external tracker.

## Automatic Issue evaluation on Workspace completion

Completing a Workspace whose input carries `issue_id` records a durable Issue evaluation
in the same `WORKSPACE.md` revision: `issue_evaluation.state` starts as `pending`, and its
id is a fresh `ieval_...` for every completion cycle. Completion then tries, best effort
and only after the commit, to start a conversation-only orchestrator Run whose prompt
carries an evaluation notice. A failed launch never rolls back the committed completion;
it stores `issue_evaluation.launch_error`, and `workspace start` or a replayed `complete`
retries while no orchestrator Session is active. `workspace complete
--no-issue-evaluation-start` suppresses the launch and leaves the evaluation `pending`. An
unlinked Workspace never gets an `issue_evaluation`.

The completed Workspace's orchestrator judges whether the Issue's acceptance criteria were
delivered. It reads the frozen `inputs/issue.md` snapshot and compares it with the accepted
handoffs and artifacts, `INTEGRATION.md`, the landing commit, and the completion reason,
then records the outcome with the narrow, Workspace-scoped operation:

```sh
workspace issue-evaluation record --outcome delivered|not_delivered \
  --reason "<per-criterion evidence>" --expected-revision <revision> \
  --operation-key issue-evaluation:<ieval id>
```

Only that Workspace's orchestrator (including a conversation-only Run) or the user
terminal as recovery can record the evaluation; the project Dispatcher and worker actors
are refused, and the operation can change only the Issue named by the Workspace input. The
Dispatcher may still close local Issues, but it must not close an Issue whose linked
Workspace has a pending evaluation: that decision belongs to the Workspace orchestrator.

The outcome changes only the local Issue; its `status_reason` carries the judgment:

| Issue status | `delivered` | `not_delivered` |
|---|---|---|
| `open` | `closed` (`closed`) | stays `open` (`left_open`) |
| `deferred` | `closed` (`closed`) | `open` (`left_open`) |
| `closed` by this Workspace's earlier evaluation | unchanged (`unchanged_closed`) | `open` (`reopened`) |
| `closed` by another actor | unchanged (`unchanged_closed`) | unchanged (`unchanged_closed`) |

The reason is prefixed `<Delivered|Not delivered> by workspace <ws_id> (evaluation
<ieval>)` and bounded to 2000 bytes. `issue show`, `issue list`, and the TUI display the
resulting `status_reason`. A `delivered` judgement is refused with `issue_revised` when the
current Issue content digest no longer matches the snapshot the Workspace froze, so a
Workspace cannot close an Issue whose requirements changed afterwards; the orchestrator
records `not_delivered` with the gap instead. A recorded evaluation cannot be edited:
`workspace reopen` clears it (the Issue is not changed) and the next completion records a
new `pending` evaluation. Workspaces completed by an earlier binary have no
`issue_evaluation` and are not migrated; the existing `workspace issue update` path still
applies to them. No step of this flow writes to the external tracker.
