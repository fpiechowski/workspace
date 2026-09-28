# Summary — task_01M3MJYR3CFG2SYPQ3Q4A39YS1 (I1: Autonomy state, creation, enable/disable, status)

Implemented the per-workspace autonomy state and its lifecycle entry points per
PLAN.md §3.1-3.2 and §5-I1.

- `Workspace.Autonomy` plus `Autonomy`/`AutonomyReport` types (all omitempty) and
  the audit `Decision` fields; non-autonomous documents and receipts serialize
  byte-compatibly.
- `workspace create --autonomous` (plan-first and `--no-workflow`) and
  `workspace issue dispatch --autonomous`, with a `deliver`-capability
  precondition and a Dispatcher `forbidden` refusal before the actor clear.
- `workspace autonomy enable` (terminal user only) and `autonomy disable`
  (user or attesting orchestrator), both revision-guarded, receipted and
  recorded as `autonomy.enabled`/`autonomy.disabled` decisions.
- `reopen` clears autonomy while the history copy retains it; status/JSON and the
  workspace document expose autonomy. TUI create form passes `Autonomous`.

Commit `3af359c`. Checks (`work-products/CHECKS.yaml`): gofmt clean,
`go vet ./...`, targeted `go test ./internal/core -run 'Autonomy'` and
`go test ./internal/cli -run 'Autonom'`, and the full `go test ./...` all pass.
Docs and report/menu/boundary stay in the I2-I4 scope.
