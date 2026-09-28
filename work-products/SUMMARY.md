# Summary — task_01M3KJ6ZS58GEEJX4E4TSS17MR (Documentation, skill, backlog)

Updated the contract documentation, both bundled skill copies, and the backlog for the
plan-first v2 workflow, the removal of `issue-resolution`, and the auto-migration.

- README/PRODUCT/ARCHITECTURE now describe the plan-first v2 phase order
  (`planning → plan_review → implementing → integration → completed`), the user-approved
  local `integration land` gate, conflict handling, `complete` for plan-first, the
  release-gate-only archive rule, the removed workflow, and the on-load migration.
- docs/operations.md documents `integration land` as a receipted mutation with its local
  git fast-forward/CAS effect; docs/revisions.md documents that landed work blocks
  revision/retry; runtime/tui/trackers wording updated.
- Both `internal/core/skill/workspace/SKILL.md` and `.agents/skills/workspace/SKILL.md`
  are now byte-identical and describe the integration/land/complete flow.
- TODO.md records the four deferred follow-ups (stacked strategy, CR-based landing,
  project-level integration branch, optional removal of the extended machine).

Commit `ee255be`. Checks: gofmt clean, `go vet ./...` and `go test ./...` pass, all
documentation links resolve, the skill copies match, and no stale `issue-resolution`
reference remains outside removal/migration notes.
