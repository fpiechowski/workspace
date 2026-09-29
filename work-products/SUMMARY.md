# Summary — fix supervisor orchestrator recovery regression from 2a72e3a

Task: `task_01M3P156JTWMYAT9TPBDMZ917B` · Base `2a72e3a` · Fix commit `5b75caa`

## Root cause

`2a72e3a` removed the resume block that carried the logical Session's client
definition onto the resumed Run. Recovery is the normal resume path
(`supervisor.go:393-401` → `ResumeAgent` → `StartSession`), so after a whole-window
loss the orchestrator was relaunched with the current config client. In
`TestTmuxEndToEnd` the orchestrator's snapshot was deliberately the long-running
`sh -c sleep 30` (`tmux_integration_test.go:193-203`); the resumed Run used the
configured one-shot helper instead, exited immediately, and was already `exited`
(not `interrupted`) when its window was killed, so the supervisor had nothing to
recover. The test failed at `tmux_integration_test.go:343`.

## Fix

`internal/core/session.go:316-334`: on resume, restore the Session's client
definition into the local routing config before `chooseRoute`, while still
reloading profile, route, limits and reasoning effort from current project
configuration. The `2a72e3a`-removed route pinning and prior-effort override are
not restored.

## Deviation from 2a72e3a

The resumed Run no longer reloads the client *definition* (it keeps the Session
snapshot) but still reloads routing/reasoning. Rationale: recovery reuses resume
and needs a resolvable, stable client. Proven by
`TestReasoningEffortReloadsOnResume` (routing reload), the new
`TestResumeKeepsSessionClientSnapshotAndReloadsRoute` (client snapshot kept), and
`TestTmuxEndToEnd` (recovery works). Documented in `IMPLEMENTATION.md` and in the
updated `ARCHITECTURE.md` / `docs/runtime.md` wording.

## Checks

- `gofmt -l`: clean · `go vet ./...`: clean
- `go test ./...`: pass (harness `WORKSPACE_*` env cleared)
- `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -run TestTmuxEndToEnd`:
  pass (was failing on `2a72e3a`)
- `WORKSPACE_TMUX_TEST=1 go test -race ./... -skip TestNavigatorRealPTYAndClientSelection`:
  pass; that skipped terminal test is a pre-existing environmental failure
  (unchanged package vs base)
- Regression test fails on base (`2a72e3a`) and passes with the fix

Evidence: `work-products/evidence-fix-recovery-*.txt`,
`work-products/CHECKS-fix-recovery.yaml`.
