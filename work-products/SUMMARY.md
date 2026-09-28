# Summary — task_01M3MJYRHCY7FP8RBR2G95MCV6 (I3: Safety boundary enforcement, final report, menu)

Implemented PLAN.md §3.5, §4 and §5-I3: the code-level safety boundary, the
receipted final autonomy report and the menu switch.

- `rejectAutonomousAttestation` makes `integration land`, `complete`,
  `decision answer`, `release confirm`, `change-request publish/resolve`,
  `reopen`, `archive`, non-dry-run `clean`, workspace/task/session `delete` and
  `state edit` return `autonomy_excluded` for an agent actor while the run is
  running, even with `--user-confirmed` or `forge.publication: allowed`. The
  terminal user is never excluded and the ordinary contract returns after
  `delivered`/`disabled`.
- `workspace autonomy report --outcome --summary-file [--artifact] [--pending]`
  validates the outcome against state (`integration_changed`, `handoff_pending`,
  `session_active`, `pending_required`, `invalid_outcome`), stores an immutable
  summary artifact atomically, sets `delivered`, appends an
  `autonomous.final_report` decision and replays idempotently.
- `menu --json` shows `autonomy report` while running in integration or when a
  manual workspace's work is accepted, and the report plus `land`/`complete`
  after delivery.
- End-to-end tests cover the autonomous plan-first flow to `ready_to_land`
  (agent land/complete refused, user land + complete succeeding), the manual flow
  to `ready_to_complete`, the boundary table and the menu.

Commit `48eab2b`. Checks (`work-products/CHECKS-I3.yaml`): gofmt clean,
`go vet ./...`, the targeted core run, the CLI autonomy run and the full
`go test ./...` all pass. Docs and the orchestrator guidance notice stay in I4.
