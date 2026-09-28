# Summary — task_01M3MJYS3TAP3E9057R289DR1G (I5: TUI support)

Implemented PLAN.md §5-I5: the TUI surface for autonomous workspaces.

- The workspace header shows an autonomy badge (`● autonomous running`,
  `✓ autonomous delivered`, `○ autonomous disabled`); the state word survives
  `--no-color`.
- A delivered report is a Needs-attention row (and a More entry) that opens a
  report view with the outcome, recommendation, integration head, artifact IDs
  and pending commands.
- Decisions resolved under autonomy are marked `by orchestrator (autonomous)`
  and their detail exposes the subject, evidence and Run provenance.
- The Orchestrator, Runtime and Autonomy pages offer a guarded
  `Disable autonomous run`: a required reason, a confirmation that keeps the
  exact current revision and the `tui_<ULID>` key, dispatched to
  `Service.DisableAutonomy`. Disabled workspaces no longer offer it.
- The TUI does not offer `autonomy enable` on an existing workspace (A4) and
  never auto-accepts handoffs or decisions. The create-form `Autonomous run`
  toggle reaches `CreateOptions.Autonomous` for both the named workflow and the
  manual choice.
- `docs/tui.md` documents all of the above.

Commit `eca7ab5`. Checks (`work-products/CHECKS-I5.yaml`): `go test
./internal/tui -count=1`, gofmt, `go vet ./...` and the full `go test ./...`
all pass (full suite with the harness `WORKSPACE_*` variables cleared, matching
the I3 checks file).
