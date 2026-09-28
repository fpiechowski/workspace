# Implementation report — task_01M3KJ6ZS58GEEJX4E4TSS17MR

## Commit

- `ee255be` — `docs: document plan-first v2 integration, landing and legacy migration`

## Changes

### README.md

- Intro describes `plan-first` as planning → implementation → mandatory integration →
  user-approved local landing.
- `project init` wizard description now lists only the `plan-first` v2 mapping and states
  that the removed `issue-resolution` workflow is never generated.
- Configuration example: `plan-first` gains `integration: worker` and the v2 capabilities
  (`tasks.role.integrator`, `integration`, `landing`); the `issue-resolution` entry is
  removed and replaced by a mandatory-capability / removal note.
- Getting Started: remove `--workflow plan-first` from examples to show the new default;
  `issue dispatch` without `--workflow`; creation modes updated (`needs_workflow` is legacy).
- Tasks and Results: replaces the extended-flow paragraph with the plan-first integration
  prepare/integrate/land/complete contract, conflict handling, and a worked CLI example.
- Archive and TUI wording now use "workflow that declares a release gate" and accept a
  completed plan-first workspace.
- New paragraph documents the automatic `issue-resolution` → `plan-first` migration on load.

### PRODUCT.md

- Creation modes reduced to named workflow (default `plan-first`) and `--no-workflow`;
  legacy `needs_workflow` is a resolvable state, not a new-creation mode.
- Product promise / decision principle now describe the user-approved integration landing
  plus completion instead of release-only closure.
- Scope and workflow-snapshot paragraph describe plan-first v2 and the removed workflow,
  the ignored legacy config entry, custom extended workflows, and the archive rule.

### ARCHITECTURE.md

- Workflow section rewritten to the plan-first v2 phase diagram, landing semantics, and
  the capability snapshot rules (v2 union, v1 frozen contract, `landing` validation).
- Manual Mode creation forms updated; Workspace domain row mentions landing/completion.
- Persisted State/Templates paragraph documents the in-place auto-migration.

### docs/operations.md

- `integration land` added to the receipted-mutation list.
- New "Integration landing" section documents the pending/landed receipt, the
  `merge --ff-only` / CAS `update-ref` effect, `target_moved` / `target_checkout_dirty`
  refusals, idempotent retry, local-only behavior, and the post-landing guards.
- TUI operations paragraph covers plan-first `complete` and the release-gate archive rule.

### docs/revisions.md

- Documents that landed work blocks retry and input/workflow revision until `reopen`.
- Documents the auto-migration and its history manifest for a non-terminal workspace.

### docs/runtime.md

- Completion/archive wording covers landed plan-first workspaces and the release-gate rule.

### docs/tui.md

- Create form, complete actions, and the action list reflect plan-first selection,
  `Complete this workflow workspace`, and the release-gate archive rule.

### docs/trackers.md

- Dispatch example drops the explicit `--workflow plan-first` and notes the default.

### internal/core/skill/workspace/SKILL.md and .agents/skills/workspace/SKILL.md

- Default-workflow guidance, the plan-first integration/land/complete flow, and the
  read-only legacy migration note. The two copies are now byte-identical (the `.agents`
  copy was a stale installed copy).

### TODO.md

- Records the deferred follow-ups: stacked strategy, CR-based landing, project-level
  integration branch, and optional removal of the unused extended machine.

### internal/cli/help.go, internal/cli/workflow.go

- `workflow advance` help names the integration stop and uses a real phase example;
  the `integration` group `Short` mentions landing.

## Acceptance criteria

- `grep -rn issue-resolution` over README/PRODUCT/ARCHITECTURE/docs/skills returns only
  removal/migration notes (negative check exits 0); documentation links resolve
  (29 links checked, 0 broken).
- Help text and docs agree with a freshly built binary's `--help`: every documented flag
  for `integration prepare`, `integration land`, `complete`, and `create` is present.
- The two skill copies are byte-identical.
- TODO.md records the four deferred follow-ups.
- `go vet ./...` and `go test ./...` pass; `gofmt -l ./cmd ./internal` is empty.

## Checks

| Command | Exit | Evidence |
|---|---:|---|
| `gofmt -l ./cmd ./internal` | 0 | `evidence-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-vet.txt` |
| `env -u WORKSPACE_AGENT_ID -u WORKSPACE_SESSION_ID -u WORKSPACE_RUN_ID go test ./... -count=1 -timeout 570s` | 0 | `evidence-go-test-all.txt` |
| `python3 work-products/check-doc-links.py` | 0 | `evidence-doc-links.txt` |
| `diff internal/core/skill/workspace/SKILL.md .agents/skills/workspace/SKILL.md` | 0 | `evidence-skill-identical.txt` |
| `grep -rn issue-resolution ... \| grep -viE 'removed\|migrat\|legacy'` | 0 | `evidence-grep-issue-resolution.txt` |
| `CGO_ENABLED=0 go build ... && ./workspace integration land --help && ./workspace complete --help` | 0 | `evidence-build-help.txt` |

## Risks and deviations

- The documentation describes behavior implemented by T1–T4; it does not change any Go
  behavior beyond two help-string corrections.
- `needs_workflow` is no longer produced by a new `create`, so it is described as a legacy
  state resolved with `workspace workflow select` rather than a creation mode.
- The TUI create form still renders a `Choose later` option that core now resolves to
  `plan-first`; the docs describe the effective mode choice (plan-first preselected or
  manual). Adjusting the form itself is outside this task's scope.
- No tmux race suite is required for a documentation change; the full `go test ./...`
  package set was run instead.
