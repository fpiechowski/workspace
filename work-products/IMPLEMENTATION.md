# IMPLEMENTATION — single-client auto-jump for the TUI navigation flow

Task: `task_01M3MN4RE4J7NKEJCD45MTSH7S`
Plan: `art_01M3MMYVEGXFT2WCVEFFXECK1X` (task `task_01M3MKVG69T3XM0DPREVQ1BXPP`)
Base: `aa72616494937c5c977187ade2fd342d230fac93`

## Commits

| Commit | Message |
|---|---|
| `f6e666e63bad2dc1d7311f0600a551e37a7dd796` | `feat(tui): jump immediately with the only attached tmux client` |
| `d1bd846e550bf9e437190592206fd7e8319aa66b` | `docs: describe single-client auto-jump navigation` |

Both commits are on the worktree branch
`workspace/ws_01M3MJZRZ42N2F623G70K60M05/impl-single-client-jump`.

## What changed

### `internal/tui/navigation_flow.go`

- **T1**: In `handleNavigationClients`, after the zero-client check (l.126) and the
  generation/target/ref guard (l.133) and before `orderedClients`, a new
  `len(msg.clients) == 1` branch calls
  `jumpClientCommand(msg.target, msg.ref, msg.clients[0], msg.afterReconcile,
  msg.generation, msg.sequence, true, true)`. It does not set `m.form`, `m.formMode`,
  `m.formClient` or `m.navigationClients`, so no picker opens.
- **T1**: Inside the existing staleness guard only, when `len(msg.clients) == 1` the
  notice becomes “The selected workspace target changed before the jump started. Press g
  to try again.” The multi-client wording is unchanged. No second guard was added.
- **T2**: `jumpClientCommand` gained a trailing `automatic bool`. Both existing callers
  (reconcile retry `navigation_flow.go:99`, picker accept `:201`) pass `false`; the new
  single-client branch passes `true`. When automatic, the in-flight notice is
  “Jumping the only attached tmux client to the verified workspace pane…” and the
  `navigation_target_changed` message is “the selected tmux target changed during client
  discovery; press g to try again”; otherwise both texts are unchanged. Recheck, `Jump`,
  `SaveLastUsed` and message shapes are untouched.
- **T3**: `handleNavigationTarget` loads the last-used preference only when
  `len(clients) > 1` (was `> 0`). The `SaveLastUsed` call on a successful jump is
  unchanged, so the single client is still recorded and its save failure is still
  reported by `handleNavigationResult`.

### `internal/tui/navigation_flow_test.go`

- Extracted `discoverNavigationClients` (resolve → `handleNavigationTarget` → discovery
  message) and made `openNavigationPicker` reuse it, still asserting the picker opened.
- Renamed `TestJumpAlwaysShowsClientPickerAndJumpsChosenClient` to
  `TestMultipleClientsShowPickerAndJumpChosenClient`, dropped the one-client subtest, and
  test both rows (index 0 and 1) with `flowClients()`; it asserts `prefs.loads == 1`.
- `TestJumpCancellationAndFailureNeverSavePreference` and
  `TestPreferenceWriteFailureReportsSuccessfulJumpSeparately` now use `flowClients()`.
- Added `TestSingleClientJumpsWithoutPicker`, `TestSingleClientTargetChangedDuringDiscovery`,
  `TestSingleClientGoneOnAutomaticPath`, `TestSingleClientPreferenceSaveFailureReportedSeparately`,
  `TestSingleClientStaleDiscoveryDoesNotJump` (stale sequence/generation/changed target) and
  `TestEscDuringAutomaticJumpDropsResult`.
- Multi-client, zero-client, target-change-while-picker-open, detached-client,
  stale-response and reconcile-retry tests are unchanged and passing.

### Contract docs (T5)

- `README.md` (~446–449 and ~478–482, plus the parallel Sessions line ~466),
  `PRODUCT.md` (~225–229 and ~254), `docs/runtime.md` (~14–17), `docs/tui.md`
  (~65–68, the `g` key table row, the Terminal and Managed Pane paragraph ~254–257 and
  the target-recheck sentence ~269–270).
- No stale “always / even for one client” wording remains. Verified with
  `grep -rn "even for one client\|always opens\|always presents\|including for one client\|even when only one client" --include="*.md" .` → no matches.

## Decisions and deviations from the plan

- **Extended doc scope (deviation)**: the plan’s T5 list did not name `ARCHITECTURE.md`
  or `DESIGN.md`, but both contained the same stale contract
  (`ARCHITECTURE.md:316` “always presents the client picker, including for a single
  client”; `DESIGN.md:165-166` “always opens the attached-client picker … even when only
  one client is attached”). The acceptance criterion requires that no stale
  “always/even for one client” wording remains (grep verified) and `AGENTS.md` requires
  the architecture/design contract to describe current behavior, so both were updated.
- **`docs/tui.md:269-270`**: changed “changed while the picker was open” to “changed
  before the jump started”, because the single-client path has no picker.
- **T3 fallback not used**: the plan offered keeping the load at l.110 as a fallback; the
  chosen behavior is `len(clients) > 1`, pinned by `TestSingleClientJumpsWithoutPicker`
  (`prefs.loads == 0`) and the multi-client test (`prefs.loads == 1`).
- Zero-client notice, multi-client picker behavior (ordering, last-used marker,
  read-error notice, Esc/Ctrl+C cancel), reconcile retry, and auto-select among several
  clients are unchanged/out of scope.

## Acceptance criteria

| Criterion | Status | Evidence |
|---|---|---|
| T1 single-client branch, no form | Met | `navigation_flow.go:143-145`; `TestSingleClientJumpsWithoutPicker` asserts `form==nil`, `formMode==""`, `navigationClients==nil` |
| T1 single-client staleness notice, no second guard | Met | `navigation_flow.go:133-142`; `TestSingleClientStaleDiscoveryDoesNotJump/changed target` |
| T2 `automatic bool`, texts, other callers false | Met | `navigation_flow.go:99,201,204,213-217,226-231`; `TestSingleClientTargetChangedDuringDiscovery` |
| T3 load only when `>1`, save unchanged | Met | `navigation_flow.go:111`; `TestSingleClientJumpsWithoutPicker` (`loads==0`, save on success) and multi-client test (`loads==1`) |
| T4 new tests + helper + renamed/updated existing tests | Met | `navigation_flow_test.go` |
| T5 docs, no stale wording | Met | docs diff; grep clean |
| Out of scope unchanged | Met | existing zero/multi/stale/reconcile/detached tests pass |
| Checks pass and evidence captured | Met (see below) | check receipts |
| Committed with clear messages | Met | `f6e666e`, `d1bd846` |

## Check results

Run via `workspace check run` (receipts under `work-products/checks/`):

| Command | Exit | Receipt |
|---|---|---|
| `gofmt -l internal/tui` (empty) | 0 | `check_01M3MNFZ50HDM3MP552AXF456K` |
| `go test ./internal/tui -run 'Navigation\|Jump\|Client\|Preference\|Reconcile\|Stale' -v` | 0 | `check_01M3MNG1HM1BD0BKWHJHXWM0H8` |
| `go vet ./...` | 0 | `check_01M3MNG4S33XD94MYZCY2NAHKS` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/tui -timeout 90s` | 0 | `check_01M3MNGE76V24JG1KQ1BAW9PQB` |
| `go test ./...` | 1 (pre-existing) | `check_01M3MNGM4N1G072Z0JG568KHES` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s` | 1 (pre-existing) | `check_01M3MNGYY6FAS3APQ3NVD11R1M` |

Raw evidence copies: `work-products/evidence/gofmt.txt`, `tui-focused.txt`,
`go-vet.txt`, `go-test-all.txt`, `tmux-race.txt`.

**Pre-existing failures (not caused by this change).** `go test ./...` and the full race
run fail in `internal/cli`, `internal/core` and `internal/terminal` in exactly the same
way on the base commit `aa72616` with this change stashed:
`TestCompleteAndArchiveManualWorkspace`, `TestReopenJSONAndHelpContract`,
`TestIntegrationLandJSON`, the `internal/core` harness
(`open -test.timeout=...: no such file or directory`), and
`TestNavigatorRealPTYAndClientSelection` (`pseudo-TTY client did not attach`). The
changed package `internal/tui` passes the full suite both normally and under
`-race` with `WORKSPACE_TMUX_TEST=1`.

## Risks

- **Surprise switch**: a user expecting a confirmation step now gets an immediate
  single-client switch. This is the intended issue behavior; it is mitigated by the
  explicit in-flight/success notices and the doc update.
- **TOCTOU**: a second client could attach between `ListClients` and `Jump`; the verified
  jump still moves only the discovered client, the same guarantee the picker gave.
- **Text drift**: tests assert substrings (“only attached”, “press g”, “Jump completed”);
  wording changes must update the tests together.
