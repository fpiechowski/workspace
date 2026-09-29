# SUMMARY — T3: automatic Issue-evaluation launch, CLI command and menu

Task: `task_01M3P9EKSWW97RC87SW2EA875V` · Worktree `impl-launch` · Base `3031ccd`
· Commit `046b33b`

## What shipped
- Completing a linked Workspace now best-effort starts a conversation-only
  orchestrator Run (`issue-evaluation:<ieval id>`) after the commit, without ever
  rolling back the completion. If the orchestrator is already active the launch is
  a no-op, so completion replays create no second Run.
- A failed launch records `issue_evaluation.launch_error` and the completed menu
  offers the evaluation action plus a launch-aware conversation label.
- `CompleteOptions.NoEvaluationLaunch` is exposed as `--no-issue-evaluation-start`.
- New `workspace issue-evaluation record` group/subcommand with `--outcome`,
  `--reason`, `--expected-revision` and the global `--operation-key`.
- `menu` shows `issue_evaluation` for pending evaluations in both completed modes.
- Tests: core launch/no-launch/failure, CLI record-vs-status JSON and replay, TUI
  `complete_workspace` through `tui.Backend`, and an opt-in tmux integration case.

## Acceptance
All six acceptance criteria are met and covered by the new tests. The CLI record
JSON `data` is DeepEqual to `workspace status --json` and replays under the same
key. The TUI path launches through `CoreBackend`.

## Checks
- `gofmt -l ./cmd ./internal` — clean.
- `go vet ./...` — exit 0.
- targeted core+CLI tests and `go test ./... -count=1 -timeout 570s` — exit 0.
- `WORKSPACE_TMUX_TEST=1` isolated new tmux case — exit 0; `-race` core+tui with a
  realistic timeout — exit 0.

## Caveat (environment, not a regression)
`WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s` cannot pass on this WSL
`/mnt/c` sandbox: the `core` package alone needs 153 s at base `c5c78da` and 185 s
with T3 (so the 90 s cap fires), and
`internal/terminal.TestNavigatorRealPTYAndClientSelection` fails at base too. Both
are reproduced on the untouched base commit; the T3 tmux case and the full core
suite pass with an extended timeout. Evidence files record the comparison.

## Risks
- Launch remains best effort: durable `pending` + `launch_error` + `workspace start`
  are the recovery path.
- No T4 documentation or optional T5 TUI Issue-detail change was made.
