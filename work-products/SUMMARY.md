Implemented T1 — the core Issue-evaluation domain for linked Workspaces at commit
`87bc342` (base `3031ccd`). No launch, CLI or menu changes (T3).

- Split `UpdateIssue` into the public wrapper plus a lock-free `updateIssueLocked`
  helper so an Issue and a Workspace can be written in one project-lock critical
  section; public behavior and existing tests are unchanged.
- Added `Workspace.IssueEvaluation` / `IssueEvaluation` (`issue_evaluation`). Both
  completion paths (`completeManual`, `completeLanding`) record a fresh `pending`
  evaluation with a new `ieval_...` ID in the same revision when `input.issue_id`
  is set, and nothing for an unlinked Workspace.
- Added `RecordIssueEvaluation` (`internal/core/issue_evaluation.go`): every row of
  the outcomes table, the stale-digest `issue_revised` refusal for delivered,
  outcome/reason validation, the not-completed/unlinked/missing/archived/revision
  conflicts, orchestrator-or-user authorization (conversation-only Run included),
  idempotent receipt replay, and a crash-convergent Issue+Workspace write reusing
  `updateIssueLocked` with the stable `issue-evaluation:<ws>:<ieval>` key.
- `ReopenWorkspace` clears the evaluation; the next completion gets a new pending
  ID, and a later not_delivered reopens an Issue this Workspace had closed.

Verification: `gofmt -l ./cmd ./internal` clean, `go vet ./...` and
`go test ./... -count=1 -timeout 570s` pass, plus targeted new core tests. Evidence
files and `CHECKS-T1-EVAL.yaml` are in `work-products/`.
