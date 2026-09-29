# Implementation report — task_01M3P156JTWMYAT9TPBDMZ917B

Fix supervisor orchestrator recovery regression from `2a72e3a` (`TestTmuxEndToEnd`).

## Commit

- Base: `2a72e3afcb134e0ab9c82961898aad331ab1c642` (master tip)
- Fix: `5b75caa4382f7359df3af368eb59d04d6b95b5ba`
  - `fix: keep session client snapshot on resume for recovery`
- Files: `internal/core/session.go`, `internal/core/reasoning_effort_test.go`,
  `ARCHITECTURE.md`, `docs/runtime.md`

## Symptom

`WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -run TestTmuxEndToEnd`
fails at `internal/core/tmux_integration_test.go:343`
("supervisor did not recover the orchestrator after its whole tmux window was lost").
Passes on parent `367c006`; fails on unmodified `2a72e3a`.

## Root cause

`TestTmuxEndToEnd` makes the orchestrator long-running by mutating the logical
Session's client snapshot before the explicit resume:

- `tmux_integration_test.go:193-203` sets
  `current.ClientSnapshot.LaunchArgv = ["sh","-c","sleep 30"]`
  (the configured `command` client helper, `TestWorkerProcess`, exits immediately).
- `tmux_integration_test.go:204` resumes the orchestrator.
- `tmux_integration_test.go:300-315` records the orchestrator run and then kills its
  whole window (`kill-window`).
- The supervisor recovers an orchestrator only while it is `interrupted` and had a
  live Run: `internal/core/supervisor.go:393-401`
  (`orch.State == "interrupted"`, then `ResumeAgent`). If the orchestrator already
  `exited`, `Reconcile` never marks it interrupted
  (`internal/core/session.go:1029-1037`) and nothing is recovered.

`2a72e3a` removed the block in `StartSession` that carried the logical Session's
client definition onto the resumed Run (the block deleted between
`logical = prior` and `thread` handling in `internal/core/session.go`). After that,
a resumed Run resolves its client *definition* from the current project config. In
the test the snapshot `["sh","-c","sleep 30"]` was therefore replaced by the
configured one-shot helper:

```
DBG resume role=orchestrator ... priorLaunch=[sh -c sleep 30]
             -> launch=[... core.test -test.run=TestWorkerProcess ...]
```

The resumed orchestrator exited in milliseconds, so by the time its window was
killed it had no live Run and the supervisor had nothing to recover. Recovery is
the normal resume path (`ResumeAgent` → `StartSession`), so a resumed Run must be
able to resolve a stable client definition for the Session.

## Fix

`internal/core/session.go:316-334`: restore only the Session's client definition
into the local routing config before `chooseRoute`, keeping the `2a72e3a` reload of
profile, route, limits and reasoning effort:

```go
if prior.Route.Client != "" && prior.ClientSnapshot.Adapter != "" {
    // A logical Session owns its client definition. Keep the launched
    // client stable across Runs so that a resumed Run — including the
    // supervisor recovering a lost pane — can still resolve the exact
    // client it was running. The profile, route, limits and reasoning
    // effort are still reloaded from the current project configuration
    // below; only the client definition is carried over.
    if cfg.Clients == nil { cfg.Clients = map[string]Client{} }
    priorClient := prior.ClientSnapshot
    if usesNativeOpenCodeDelivery(priorClient) {
        priorClient.NativeDelivery = true
    }
    cfg.Clients[prior.Route.Client] = priorClient
}
```

`route, err := s.chooseRoute(cfg, profile)` and the following
`reasoningEffort` resolution are unchanged, so the resumed Run still uses the
current profile/route/limits/effort. The old `2a72e3a`-removed profile-route
pinning (`profileCfg.Routes = []Route{prior.Route}`) and prior-effort override are
**not** restored.

## Deviation from 2a72e3a (documented)

`2a72e3a` intent: a resumed Run reloads the current project configuration,
including the client. This fix narrows that for the client *definition*: the
resumed Run reloads profile/route/limits/reasoning effort but keeps the client
definition owned by the logical Session. Rationale: recovery reuses the resume
path, and a resolvable, stable client is required for a lost pane to be recovered
(and for a long-running client to remain running across resume). The routing
reload — the actual subject of `2a72e3a` — is preserved.

Tests proving each side:

- `TestReasoningEffortReloadsOnResume` (kept green): route `Model`/`MaxConcurrency`
  and `ReasoningEffort` are reloaded from current config on resume.
- `TestResumeKeepsSessionClientSnapshotAndReloadsRoute` (new): on resume the Run
  argv keeps the Session's client snapshot while `Route.Model` is reloaded.
- `TestTmuxEndToEnd` (kept green): whole-window supervisor recovery works again.

## Check results

All commands below were run with the harness `WORKSPACE_*` environment cleared
(the sandbox exports worker variables that otherwise break the CLI/core fixtures);
`WORKSPACE_TMUX_TEST=1` re-set only for the tmux runs. Real evidence files:

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l ./cmd ./internal` | 0 | `evidence-fix-recovery-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-fix-recovery-vet.txt` |
| `go test ./... -count=1 -timeout 570s` | 0 | `evidence-fix-recovery-go-test-all.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -run TestTmuxEndToEnd -count=1` | 0 | `evidence-fix-recovery-tmux-e2e.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./... -count=1 -skip TestNavigatorRealPTYAndClientSelection` | 0 | `evidence-fix-recovery-tmux-all.txt` |
| `go test ./internal/core/ -run TestResumeKeepsSessionClientSnapshotAndReloadsRoute` | 0 | `evidence-fix-recovery-regression-pass.txt` |
| Base `2a72e3a` + same regression test | 1 | `evidence-fix-recovery-regression-base.txt` |
| Base `2a72e3a` + `TestTmuxEndToEnd` | 1 | `evidence-fix-recovery-tmux-e2e-base.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/terminal/ -run TestNavigatorRealPTYAndClientSelection` | 1 | `evidence-fix-recovery-terminal-known-red.txt` |

`TestNavigatorRealPTYAndClientSelection` is a pre-existing environmental failure
(`pseudo-TTY client did not attach`); it fails identically on the base and the
`internal/terminal` package is byte-for-byte unchanged vs `2a72e3a`, so it is
excluded from the tmux acceptance run with `-skip` and documented here.

## Acceptance criteria

- `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -run TestTmuxEndToEnd`
  passes; the failure at `tmux_integration_test.go:343` is gone. ✅
- Root cause with code evidence documented above (client snapshot dropped by
  `2a72e3a`; recovery needs a resolvable, stable route/client). ✅
- `2a72e3a` intent preserved where compatible: `TestReasoningEffortReloadsOnResume`
  and other routing/reasoning tests pass; the client-snapshot deviation and its
  rationale and proving tests are documented. ✅
- `gofmt -l` clean; `go vet ./...` passes; `go test ./...` passes;
  `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/` passes. ✅
- No unrelated refactors; changes bounded to the resume/recovery path plus one
  regression test and the two docs whose `2a72e3a` wording became wrong
  (`ARCHITECTURE.md`, `docs/runtime.md`). ✅
- Committed in the worktree; `IMPLEMENTATION.md` and `SUMMARY.md` submitted with
  real check evidence. ✅

## Risks

- A route whose `Client` name is no longer present in the current profile selects a
  different client, so the snapshot is not used; this is the intended route reload
  and fails closed only if no eligible route remains.
- If the configured client definition is intentionally changed for the same client
  name, a resumed Session keeps the older argv until a new logical Session is
  started. This is the documented deviation above.
- The fix does not add pane-liveness gating to resume; the supervisor still only
  recovers an `interrupted` Session with a recorded Run, as before.
