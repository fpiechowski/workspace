Implemented the plan-first v2 core state machine for T1 (commit `a76b8a6`).

- Added the `landing` capability and split the legacy v1 snapshot fallback from
  the built-in v2 capability set; a plan-first selection unions its config with
  the mandatory v2 set, while v1 snapshots keep completing after implementation.
- Added the third machine: planning -> plan_review -> implementing -> integration
  (only after every live implementer is accepted); `advance` in `integration`
  returns `user_decision_required` naming the next step.
- `integration prepare` is gated on the `integration` phase, defaults its base to
  the target branch tip for landing workflows, accepts/validates `--target`, and
  records `Integration.Target`/`Strategy`; the integrator profile falls back to
  the workflow's implementation profile.
- `ValidateConfig` validates landing combinations and hides a legacy
  `issue-resolution` entry from the normalized config.
- Extended-machine tests are unchanged; new v2 tests cover the snapshot union,
  gating, prepare targeting, integrator acceptance and config validation.

Verification: gofmt clean, `go vet ./...` and `go test -count=1 ./...` pass, and
the required tmux race subset
(`WORKSPACE_TMUX_TEST=1 go test -race ./internal/core -run 'Workflow|Integration' -timeout 90s`)
passes. The unrelated, pre-existing `internal/terminal` pseudo-TTY test still
fails under `WORKSPACE_TMUX_TEST=1` at the frozen base as well.
