# Integration report — integration round 3 (`task_01M3P2ZDWEZW9727N3NCK3CCH5`)

Merge the two accepted heads for this workspace onto `master` tip `3031ccd`:

| Order | Task | Name | Head |
| --- | --- | --- | --- |
| 1 | `task_01M3MZD82YXFS0G5HKV46G5JNQ` | dispatcher jump (`g`) | `7d4af63dad9a1b93b854cee81c9747fe57d724d4` |
| 2 | `task_01M3P156JTWMYAT9TPBDMZ917B` | orchestrator recovery fix | `3384ce3085d345165ce1f46326e482d6fb8d5299` |

- Integration worktree: `wt_01M3P2W355Z1PHQJ27TA307MZ7`
  (`worktrees/integration-334f877519a2`).
- Base commit: `3031ccd17917f9586d907382a5eb94efc5d2d61d` (`master`).
- Manifest: `integration/manifest.json` — `strategy: merge`, `target: master`.
- Integration branch: `workspace/ws_01M3MKBEJER3T6GPCF4HZC3HX8/integration-334f877519a2`.

The head merge-bases differ: `7d4af63` branched from `aa72616` (pre
single-client auto-jump); `3384ce3` branched from `2a72e3a`. Master had moved to
`3031ccd` with `267725c` (limit-aware resume/retry/supervisor recovery),
`3684988` (native-thread route affinity) and `3031ccd` (test-side recovery
resolution), so both merges are real three-way merges.

## Merge commits

`git merge --no-ff` in the recorded order:

| Merge | Second parent | Parents | Result |
| --- | --- | --- | --- |
| `d3eb5f12b4aa5a8e1a793b28c736c2c6d1154ac3` | `7d4af63` (dispatcher jump) | `3031ccd`, `7d4af63` | 4 textual conflicts |
| `4105c50f22052b34a0d9f0d7eb55aaaf49e8eb3c` | `3384ce3` (recovery fix) | `d3eb5f1`, `3384ce3` | 2 textual conflicts |
| `99d178f0914093ba2505a2e300822a5f2a8aaafb` | — | `4105c50` | integration-resolution commit (test reconciliation) |

Both heads are ancestors of the final integration HEAD, and there are no
unrelated changes: the product diff vs `3031ccd` is exactly the dispatcher-jump
change, the recovery fix, and the recovery-test adaptation (see below).

## Conflict resolutions

### Merge 1 — `7d4af63` (dispatcher jump)

1. `docs/tui.md` — the `g` navigation paragraph.
   - *Conflict*: master (HEAD) documents the single-client auto-jump ("with
     exactly one attached client it jumps immediately with it; with several it
     opens a picker", landed via `367c006`). The incoming head branched before
     that and documented "It always opens a picker, including when there is only
     one client", plus the new dispatcher paragraph.
   - *Resolution*: kept master's single-client auto-jump wording and appended
     the incoming dispatcher-jump sentences (jump-capable while the Dispatcher
     has a live ownership-verified Run; canonical
     `workspace-dispatcher-<project-id>`; hidden and inert when idle;
     `workspace dispatcher attach` unchanged).
   - *Rationale*: `g`'s picker contract is master's newer product behavior and
     must not regress; the dispatcher addition is the incoming intent. Both fit
     in one paragraph.

2. `work-products/CHECKS.yaml` — shared check manifest rewritten by both
   lineages.
   - *Resolution*: both files are top-level YAML sequences; concatenated them
     into one valid list, preserving every entry (validated with a YAML parse:
     14 entries). Artifact-only; no product decision.

3. `work-products/IMPLEMENTATION.md` — shared report path rewritten by both.
   - *Resolution*: preserved both reports verbatim, keeping the existing
     integrated reports first and appending the dispatcher report under an
     "Incoming implementation report" divider. Artifact-only.

4. `work-products/SUMMARY.md` — same whole-file rewrite conflict.
   - *Resolution*: preserved both summaries verbatim under a divider.
     Artifact-only.

### Merge 2 — `3384ce3` (recovery fix)

1. `work-products/IMPLEMENTATION.md` — shared report path rewritten by both.
   - *Resolution*: preserved both reports verbatim (existing content plus the
     recovery-fix report under an "Incoming implementation report" divider).

2. `work-products/SUMMARY.md` — same.
   - *Resolution*: preserved both summaries verbatim under a divider.

`internal/core/session.go` **auto-merged** (no textual conflict). The merged
result keeps both intents:

- the accepted fix's product behavior — on resume the Run keeps the logical
  Session's client definition (`cfg.Clients[prior.Route.Client] =
  prior.ClientSnapshot`, `session.go:313-331`); and
- master's limit-aware resume and native-thread route affinity —
  `chooseRoute` returning the `RoutingDecision`, the single-candidate route
  pinning for a native thread, and `resumeRouteLimitedError`
  (`session.go:333-356`), plus master's test changes.

`ARCHITECTURE.md`, `docs/runtime.md` and `internal/core/reasoning_effort_test.go`
merged without conflicts.

## Semantic conflict resolved after the merges (test adaptation, `99d178f`)

The two merged intents genuinely conflict in `TestTmuxEndToEnd`:

- `3031ccd` made the orchestrator long-running by writing the long-running client
  into the **current project config** before resuming.
- the accepted fix makes a resumed Run keep the **logical Session's client
  definition**, overriding the current-config client for that route.

On the merged tree the fix therefore relaunched the orchestrator with the
session's one-shot helper, so no live orchestrator remained to recover and the
test failed at `tmux_integration_test.go:346` (reproduced: 2/3 runs failed; base
`3031ccd` passed 3/3). Per the task's reconciliation policy the fix's product
behavior wins, so `internal/core/tmux_integration_test.go` was adapted: instead
of writing the client into the project config, the test sets the orchestrator
Session's `ClientSnapshot.LaunchArgv` to the long-running client before resuming
(the mechanism the fix guarantees). The test's intent (whole-window supervisor
recovery) is unchanged; profile/route/limits/reasoning still reload from the
current config, which is covered by `TestResumeKeepsSessionClientSnapshotAndReloadsRoute`
and `TestReasoningEffortReloadsOnResume`.

There were **no other conflicts**. No product/code decision outside this policy
was needed; no blocked question was raised.

## Verification (merged tree)

Commands run from the integration worktree with the harness `WORKSPACE_*` worker
environment cleared; `WORKSPACE_TMUX_TEST=1` re-set for the tmux runs. Real
evidence files.

Declared checks (all exit 0), manifest `work-products/CHECKS-r3.yaml`:

| Command | Exit | Evidence |
| --- | ---: | --- |
| `gofmt -l ./cmd ./internal` | 0 | `evidence-r3-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-r3-vet.txt` |
| `go test ./... -count=1 -timeout 570s` | 0 | `evidence-r3-go-test-all.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./... -count=1 -skip TestNavigatorRealPTYAndClientSelection -timeout 900s` | 0 | `evidence-r3-tmux-all-skip.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -run 'TestTmuxEndToEnd\|TestDispatcherTmuxEndToEnd' -run ... -v` | 0 | `evidence-r3-tmux-e2e.txt` |
| targeted resume + dispatcher navigation tests (core/terminal/tui) | 0 | `evidence-r3-targeted.txt` |

Specifically green:

- `TestTmuxEndToEnd` (whole-window supervisor recovery), `TestDispatcherTmuxEndToEnd`.
- `TestResumeKeepsSessionClientSnapshotAndReloadsRoute`,
  `TestReasoningEffortReloadsOnResume`.
- Dispatcher jump: `TestResolveDispatcherNavigationTarget` (core);
  `TestNavigatorVerifiesProjectScopedDispatcherTarget`,
  `TestNavigatorRejectsNonCanonicalDispatcherSession`,
  `TestNavigatorListsAndJumpsProjectScopedDispatcher`,
  `TestNavigatorReportsMissingDispatcherSessionAsPaneMissing` (terminal);
  `TestDispatcherJumpResolvesVerifiedTargetThroughPicker`,
  `TestDispatcherJumpIsInertWithoutLiveRun`, `TestDispatcherAdvertisesJumpWhenRunning`,
  `TestDispatcherTargetChangeWhilePickerOpenRequiresFreshChoice`,
  `TestDispatcherDetachedClientIsReportedAndNotReplaced` (tui).

Documented evidence, **not** declared checks:

- `TestNavigatorRealPTYAndClientSelection` is a pre-existing environmental
  failure ("pseudo-TTY client did not attach"), reproduced on the base; the tmux
  acceptance run excludes it with `-skip`.
- `work-products/evidence-r3-reconciliation.txt` records the base-vs-merged
  `TestTmuxEndToEnd` runs that justify the test adaptation.

## Result

- Dispatcher jump behavior is intact (core dispatcher navigation target,
  terminal `verifyDispatcher`, TUI gating); the orchestrator jump, existing jump
  kinds and the single-client auto-jump are not regressed.
- The recovery fix's product behavior is intact and master's limit-aware resume,
  native-thread affinity and test changes are kept, except the test setup that
  genuinely conflicts with the fix (adapted above).
- No result was pushed or landed. Landing into the target branch requires the
  user's explicit integration-land confirmation.
