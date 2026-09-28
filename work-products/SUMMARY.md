# I4 — Orchestrator guidance and documentation

Status: implemented and committed (`c718ed1`).

## What changed

- **Binary-owned autonomous notice.** `autonomyRunNotice` (`internal/core/autonomy.go`)
  is appended to an orchestrator Run prompt while `autonomy.state=running`
  (`internal/core/session.go`). It carries the §3.3 judgment rules and the §4
  user-reserved boundary, so it survives stale or customized project templates.
- **Templates.** `orchestrator.AGENTS.md.tmpl` gains an `{{if .Autonomous}}`
  branch (interactive text becomes the `{{else}}`); the plan-first and manual
  `WORKFLOW.md.tmpl` / `prompts/orchestrator.md.tmpl` gain short autonomy notes.
  `Workspace.Autonomous()` and `promptData.Autonomous` back the switch, and
  `create --autonomous` now sets the record before templates are snapshotted.
- **Stale detection.** The previous stock plan-first workflow and orchestrator
  prompt digests are added to the stale lists in `internal/core/project.go`.
- **Skill.** The bundled `SKILL.md` (and its identical `.agents` copy) document
  `--autonomous`, the `decision_required` autonomous clause, and the
  `workspace autonomy enable|disable|report` commands. `workspace prime` prints
  them.
- **Docs.** `PRODUCT.md` (principle + scope), `README.md` (flag, commands,
  report), `ARCHITECTURE.md` (Domain Model + persisted state), `docs/operations.md`
  (mutations, `autonomy_excluded`, rationale receipts), `docs/runtime.md`
  (delivery precondition, run end).

## Verification

- gofmt clean, `go vet ./...` clean, `go test ./... -count=1` green.
- `go test ./internal/core -run 'Session|Skill|Template|Autonomy' -count=1` green.
- `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core -run
  'Session|Skill|Template|Autonomy' -timeout 120s` green (22.5s, no data race).
- Doc link check: `checked=29 broken=0`.
- New tests: `internal/core/autonomy_prompt_test.go`.

The exact ``WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s`` is red on
this machine for pre-existing reasons (terminal pseudo-TTY test fails at
unmodified HEAD `58b5862`; the core race binary exceeds 90s here). See
`work-products/IMPLEMENTATION.md` and `work-products/CHECKS-I4.yaml`.
