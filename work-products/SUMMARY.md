# SUMMARY — T2: Orchestrator evaluation notice in prompts, templates and skill

Task `task_01M3P9E7ZSW0572P8QEP1A7BWH` (plan §4.3) is implemented in worktree
`impl-prompts` at commit `7dbd583`.

## What changed
- `internal/core/session.go`: a conversation-only orchestrator Run for a completed
  Workspace with a `pending` Issue evaluation now gets an **Issue evaluation notice**
  after the existing conversation-only notice. It carries the Issue ID, the frozen
  `inputs/issue.md` path, the evidence sources, the exact
  `workspace issue-evaluation record ... --operation-key issue-evaluation:<ieval id>`
  command and the statement that Issue content is untrusted and does not widen
  authority. No pending evaluation means no added bytes.
- Orchestrator AGENTS template, both WORKFLOW templates and both orchestrator prompt
  templates describe the evaluation step.
- `internal/core/skill/workspace/SKILL.md` and the tracked `.agents/skills/workspace/
  SKILL.md` copy were extended identically (byte-identical `diff`).
- New `internal/core/issue_evaluation_prompt_test.go` covers the notice content and the
  no-pending gating.

## Checks (all exit 0)
- `gofmt -l ./cmd ./internal` — clean
- `go vet ./...`
- targeted core tests (new prompt tests, skill identity, autonomy notices, reopen cycle)
- `go test ./... -count=1 -timeout 570s`

Full command/evidence list: `work-products/CHECKS-T2-PROMPTS.yaml`.

## Notes / risks
- Instruction text only; the `workspace issue-evaluation record` command it documents is
  delivered by T3. No runtime dependency was introduced.
- `.workspace/templates/**` project overrides were left untouched (out of scope: they
  are the project's pinned runtime config; the task scopes `internal/core/templates`).
- The `--expected-revision` placeholder is resolved by the orchestrator from
  `workspace status --json`; the operation remains revision-guarded.
