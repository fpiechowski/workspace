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
workspace issue dispatch issue_ID --workflow plan-first --start --operation-key dispatch-142
```

The Workspace input contains the exact Issue revision and digest selected at creation;
later refreshes never rewrite it. `dispatch` may also use `--no-workflow`, but workflow
and manual flags are mutually exclusive. The project-scoped Dispatcher is allowed to
perform these local operations but cannot implement code, advance Workspace state, or
mutate the external tracker.
