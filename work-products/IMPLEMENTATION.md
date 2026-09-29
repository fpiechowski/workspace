# IMPLEMENTATION — T1: Named IDs core (slugs, reservation ledger, core adoption)

- Task: `task_01M3Q50KD2ASX479XZK15YBHRR` (`core-named-ids`), role implementer.
- Worktree: `worktrees/impl-core` (branch `workspace/ws_01M3PC44DNG1RA29ME3PKPGYSJ/impl-core`).
- Base commit: `673bb6514fad11f29c87b880d1307d30e0e5d532`.
- Implementation commit: `aeee481` (`core: mint slug IDs with a reservation ledger`).
- Accepted plan: `art_01M3Q1HXZ8KMT7QCQJD682H78P/PLAN.md` (sections 2.1–2.6, task T1).

## What changed

New files:

- `internal/core/slug.go` — slug grammar, `slugify`, reserved shapes, `baseSlug`, `parseExplicitID`.
- `internal/core/idreserve.go` — never-released reservation ledger (`<storage>/.runtime/ids/<kind>/<slug>`, `O_EXCL`) and `idAllocator` (`allocate`, `allocateExplicit`).
- `internal/core/slug_test.go` — normalization/validation/ledger/concurrency/adoption/legacy/tmux tests.

Wired adoption (each site now mints `<prefix>_<slug>` instead of `<prefix>_<ULID>`):

- Workspace and orchestrator agent: `internal/core/project.go` (`createWorkspaceLegacy`).
- Worker agents: `internal/core/agent.go` (`CreateAgent`).
- Logical sessions: `internal/core/session.go` (`StartSession`, new-logical branch only).
- Tasks: `internal/core/task.go` (`CreateTaskWithID`; `CreateTask` delegates with an empty explicit ID).
- Dispatcher agent and session: `internal/core/dispatcher_runtime.go` (project storage root, workspace id `-`).

Option surface and resolution:

- `CreateOptions.ID`, `AgentOptions.ID`, `SessionOptions.ID` (all `json:",omitempty"`; `CreateOptions.ID` also `yaml:",omitempty"`).
- `CreateTaskWithID(ctx, selector, spec, explicitID, key)`; the request payload stays `spec` when the ID is empty and becomes `struct{Spec TaskSpec; ID string}` only when set.
- `findAgent` and `findTask` match an exact ID first, then a name (`internal/core/agent.go`, `internal/core/task.go`).
- A workspace `os.Rename` collision is mapped to `id_exists` (`project.go`).
- `go.mod`: `golang.org/x/text` promoted from indirect to direct (already in `go.sum`; no new module).

## Design decisions

- **Separator `_` and slug grammar** exactly as PLAN §2.1: `^[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])?$`, base ≤ 40, total ≤ 48.
- **Reserved shapes** (`^[0-9a-hjkmnp-tv-z]{26}$` lowercase ULID, `^[0-9a-f]{24}$` migrated session hex) are never produced un-suffixed. `slugify` returns the kind's fallback word for an empty/reserved/otherwise invalid result (PLAN §2.2 step 5), and `allocate` additionally skips a reserved candidate defensively. Explicit reserved values are refused with `invalid_id`.
- **Ledger operation identity** is `<project_id>/<workspace_id or "-">/<kind>/<key>`. An unkeyed create has an empty operation and therefore never reuses a leaked reservation; a keyed retry reuses its own reservation even when the process crashed after reserving but before saving.
- **Uniqueness scope** is the storage root (PLAN §2.3 / Q1 default): one project by default, cross-project when `workspaces_dir` is shared. `O_EXCL` makes this atomic across processes and project locks.
- **Task explicit-ID timing**: the idempotency request is captured after `TaskSpec.RequiredArtifacts` defaulting, matching the value the pre-existing `CreateTask` passed to `d.previous`. Capturing it earlier would have changed existing task receipt digests.
- **Dispatcher identity**: the first-time dispatcher agent/session are allocated at the project storage root with workspace id `-`, using the caller's operation key so a keyed restart reuses them.
- **`s.Config()` is now loaded unconditionally** in `CreateAgent`/`CreateTask` because it is needed for the storage root; previously it was loaded only when the profile was empty.
- **Test-only fix outside `internal/core`**: `internal/bootstrap/context_test.go` previously relied on two independent projects producing distinct workspace IDs for the same title. With slug IDs both become `ws_example`, so the "foreign workspace" fixture now uses a different title (`bootstrapProject(t, title)`). This is a test assumption invalidated by the intended behavior change; product code is untouched.

## Acceptance criteria

| # | Criterion | Status | Evidence |
|---|---|---|---|
| 1 | New workspace (incl. orchestrator/dispatcher), agent, logical session and task IDs match `^(ws\|agent\|sess\|task)_[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])?$`; Runs stay `^run_[0-9A-Z]{26}$` | Met | `TestWorkspaceSlugDerivationAndNoReuseAfterDelete`, `TestAgentSessionTaskSlugsAcrossWorkspaces`, `TestExplicitWorkspaceAndAgentIDs`, dispatcher path in `TestDispatcherTmuxEndToEnd` (full suite) |
| 2 | Normalization table incl. Polish diacritics, ß/æ/ø/ł, emoji/empty → fallback, leading digits, hyphen-boundary truncation ≤ 40, reserved shapes never un-suffixed | Met | `TestSlugifyNormalizationTable`, `TestSlugFallbacksAndReservedShapes`, `TestSlugTruncationAtHyphenBoundary`, `TestAllocatorDoesNotReuseReservedShape` |
| 3 | Collisions → base, base-2, base-3 deterministically, across two workspaces in one storage root and against soft-deleted tasks/sessions | Met | `TestAllocatorCollisionsAreDeterministic`, `TestAgentSessionTaskSlugsAcrossWorkspaces`, `TestTaskSlugDoesNotReuseSoftDeletedID`, sessions use the same `d.Registry.Sessions` scan (including deleted) |
| 4 | After `DeleteWorkspace`, same title yields `…-2` (no reuse) | Met | `TestWorkspaceSlugDerivationAndNoReuseAfterDelete` |
| 5 | Concurrent allocation from N goroutines (`-race`) and from two Services sharing an external `workspaces_dir` gives distinct IDs, no errors | Met | `TestAllocatorConcurrentGoroutines`, `TestSharedStorageProjectsGetDistinctSlugs` (both run under `-race`); `TestSharedStorageKeepsProjectsIsolated` still passes |
| 6 | Keyed create that fails after reservation and before save, retried with the same key/payload, returns the same ID; a different key gets the next suffix | Met | `TestKeyedWorkspaceCreateReusesReservationAfterFailure`, `TestAllocatorKeyedReuseAndDifferentKeySuffix` |
| 7 | Explicit ID with/without prefix; invalid shapes → `invalid_id`; taken → `id_exists`; explicit ID in the payload, different explicit ID under the same key → `operation_conflict` | Met | `TestParseExplicitID`, `TestAllocatorExplicitIDExists`, `TestExplicitWorkspaceAndAgentIDs`, `TestExplicitTaskAndSessionIDs` |
| 8 | Legacy ULID workspace/agent/session/task and migrated `sess_<hex>` open, resolve by every old ID (incl. Run-ID alias), accept new slug IDs, leave pre-existing records unchanged | Met | `TestLegacyWorkspaceCompatibility` (before/after `reflect.DeepEqual` on the legacy task; resolves agent/task/session by legacy ID and session by Run-ID alias) |
| 9 | `DeleteWorkspace` basename check, `InferWorkspace`, `workspaceDirs`, `ProjectOverview`, `resolveReadableWorkspace` work for slug workspaces with no prefix-filter change | Met | No prefix filters were changed; slug directories satisfy the existing `ws_` prefix. Covered by the full suite and `TestWorkspaceSlugDerivationAndNoReuseAfterDelete` |
| 10 | `gofmt`, `go vet`, `go test ./...` pass; tmux race suite incl. a slug workspace tmux test (session name, pane recovery, stop isolation) | Partially met (see Limitations) | `TestTmuxSlugWorkspaceIdentity` passes; `go test ./...` and `go vet ./...` pass; full core `-race` + tmux passes with `-timeout 300s`. The exact `-timeout 90s` command exceeds 90 s for `internal/core` in this sandbox at base as well |

Digest compatibility is also asserted directly by `TestNewOptionFieldsDoNotChangeExistingDigests`: the new fields are absent from the JSON payload when empty. Existing receipts therefore replay unchanged, and all pre-existing idempotency tests in the suite pass.

## Checks

Run from the worktree root. Because the agent's shell exports `WORKSPACE_*` variables, the go test commands were run with every `WORKSPACE_*` variable unset (otherwise the child-process helper tests, e.g. `TestWorkerProcess`, trigger inside the main suite). Evidence files are in `work-products/`.

| Command | Exit | Evidence |
|---|---|---|
| `gofmt -l internal/` | 0 | `evidence-named-ids-gofmt.txt` |
| `go vet ./...` | 0 | `evidence-named-ids-vet.txt` |
| `go test ./... -timeout 600s -count=1` | 0 | `evidence-named-ids-go-test-all.txt` |
| `go test -race ./internal/core/ -run 'TestSlug\|TestParseExplicitID\|TestAllocator\|TestWorkspaceSlug\|TestAgentSessionTask\|TestTaskSlug\|TestExplicit\|TestKeyed\|TestSharedStorage\|TestNewOption\|TestLegacy' -count=1` | 0 | `evidence-named-ids-targeted-race.txt` |
| `WORKSPACE_TMUX_TEST=1 go test ./internal/core/ -run TestTmuxSlugWorkspaceIdentity -count=1 -v -timeout 180s` | 0 | `evidence-named-ids-tmux-slug-identity.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -timeout 300s -count=1` | 0 | `evidence-named-ids-core-race-tmux.txt` |
| `WORKSPACE_TMUX_TEST=1 go test -race ./... -timeout 90s -count=1` | 1 | `evidence-named-ids-race-tmux-timeout.txt` |

`TestTmuxSlugWorkspaceIdentity` creates `ws_fix-checkout` and `ws_fix-checkout-2`, asserts both exact tmux session names (`workspace-ws_fix-checkout`, `workspace-ws_fix-checkout-2`), recovers an unrecorded pane inside the first session via `Reconcile`, then stops the first workspace and asserts the live `-2` sibling and its pane are untouched.

## Limitations and risks

- **`-timeout 90s` is not achievable in this sandbox.** `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/` takes ~246 s here; the base suite without this task's ~9 s tmux test would still exceed 90 s under the race detector. The package passes at `-timeout 300s` (exit 0). This is an environment timing limit, not a failure of this change.
- **`TestNavigatorRealPTYAndClientSelection` (`internal/terminal`) fails in this sandbox**: `pseudo-TTY client did not attach`. This test is not touched by this change and relies on `script(1)`/tmux PTY behavior under WSL; it is recorded in `evidence-named-ids-terminal-pty.txt` and in the 90 s run.
- The unique suffix for common names grows over time (`agent_orchestrator-N`, `sess_orchestrator-N`), the accepted PLAN Q3 trade-off.
- Reservations are never released, so an unkeyed crash leaves a gap (`-2` without a visible `-1`). Documented as intended in the plan.
- `O_EXCL` atomicity depends on the filesystem; the plan already accepts the exotic-filesystem risk.
- `internal/bootstrap/context_test.go` was adjusted (test-only) so the foreign-workspace assertion remains meaningful under slug IDs.

## Not in scope (T2)

CLI `--id` flags and `IssueDispatchOptions.ID`, selector/help text, `workspace open` exact-ID precedence, TUI slug rendering, and the documentation updates in PLAN §2.9 remain for `cli-docs-named-ids`.
