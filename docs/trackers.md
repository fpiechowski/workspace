# Issue input

`workspace create --issue URL --workflow issue-resolution` retrieves an issue and
stores its title, body, original URL and retrieval time in `inputs/issue.md`.
GitHub.com and GitLab.com select their installed, authenticated CLI automatically.
For self-hosted services, configure the adapter explicitly:

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

Use `--input-file description.md` to provide an offline description, optionally with
`--issue URL` to retain its source. An inaccessible tracker yields `tracker_unavailable`;
it never creates a workspace with guessed issue contents. Creation with the same
operation key reuses the stored snapshot without fetching a changed issue again.
