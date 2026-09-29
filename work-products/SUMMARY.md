# Summary — implement completion tolerance for the orchestrator's own Session

**Task:** `task_01M3P2JFEFSWE8CD2VHS5PXBQW` — implement
`artifacts/art_01M3P2A2Q8TC0RF38KEH0SATFN/PLAN.md`.

**Commit:** `ccbd7525b4256a3977cd3bcfcf358c298af9e533` (`feat: tolerate the orchestrator's own session when completing`) on base `3031ccd`.

**What changed:** `completionRuntimeQuiet` (`internal/core/completion.go`) now skips active
Sessions that belong to the orchestrator (`AgentID == d.State.OrchestratorAgentID ||
AgentSnapshot.Role == "orchestrator"`), matching the existing `reopen` predicate. Worker
Sessions and services still fail with `session_active` / `service_active`; the message now
reads "stop worker session %s before completing". The orchestrator's own Session is
tolerated, not stopped or rewritten, and `archive` still requires every Session to stop.

**Tests:** fixed `TestManualCompletionRequiresUserAttestation` to expect success with the
orchestrator Session still active; added `TestManualCompletionToleratesOrchestratorButNotWorkers`
(worker blocks and is named; archive refuses until the orchestrator stops), 
`TestManualCompletionUserActorToleratesOrchestrator` (empty actor), and
`TestLandCompleteToleratesOrchestratorButNotWorkers` (landed plan-first path). Autonomy tests
were left unchanged.

**Docs:** updated `docs/runtime.md`, `ARCHITECTURE.md` (Manual Mode), `README.md` and
`docs/operations.md` to distinguish `complete` (worker Sessions/services) from `archive`
(all Sessions/services).

**Checks (see `work-products/CHECKS-completion.yaml`):** `gofmt -l ./cmd ./internal` clean,
`go vet ./...` clean, focused `go test ./internal/core -run 'Complet|Landing|Land|Reopen|Autonomy|Archive' -count=1`
green, `go test ./... -count=1` green with the sandbox `WORKSPACE_*` actor variables cleared,
doc-link check `checked=30 broken=0`.

**Caveat:** the sandbox exports orchestrator/worker `WORKSPACE_*` variables that break
`TestWorkerProcess` and four CLI tests on the *unmodified base* as well; the full suite must
be run with them unset. No product-code blocker.
