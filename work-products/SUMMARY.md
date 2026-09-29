# Summary: Auto-close completed worker Sessions

Task task_01M3P785FNW7WPVAQ0NJKVHCX0, plan art_01M3P2E8D7AHJE7MHE36Q5CQPK.
Commit 248ae63 `feat: auto-close accepted worker sessions` on base 3031ccd.

**Change.** `handoff accept` now records a durable `close_requested_at` intent on
the submitting worker Session atomically with the acceptance. After the commit a
best-effort settle verifies pane ownership, stops the owned Run, and closes the
Session. Settle failures are stored in `close_error` and retried by `Reconcile`,
the supervisor tick and the next `session start`, so the acceptance never fails on
the runtime and an unowned pane is never stopped. A new `handoff accept
--keep-session` flag preserves consultation resume; rejected handoffs,
orchestrator Sessions and manual closes are unaffected.

**Files.** `internal/core/{model,session,handoff}.go`, `internal/cli/{work,help}.go`,
`internal/core/session_autoclose_test.go`, CLI/autonomy test call-site updates,
and docs (`docs/runtime.md`, `README.md`, `PRODUCT.md`, `ARCHITECTURE.md`). No new
dependencies; stock prompt templates untouched.

**Verification.** `gofmt` clean, `go vet ./...` clean, `go test ./...` passes,
`go test -race ./...` passes. The opt-in tmux race suite
(`WORKSPACE_TMUX_TEST=1`) fails on tmux 3.7c with `invalid tmux pane: "%0_@0"`;
the same failure reproduces on the clean base commit, so it is a pre-existing
environment incompatibility, not caused by this change. Full evidence is in
`work-products/IMPLEMENTATION.md` and the `evidence-autoclose-*.txt` files.
