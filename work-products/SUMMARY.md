# Summary — T1: Named IDs core (slugs, reservation ledger, core adoption)

Outcome: implemented and committed. Commit `aeee481` on
`workspace/ws_01M3PC44DNG1RA29ME3PKPGYSJ/impl-core` (base `673bb65`).

## What was done

- Added `internal/core/slug.go`: slug grammar, NFKD + ASCII folding (Polish
  diacritics, ß/æ/ø/ł/œ/þ/đ…), hyphenation, 40-char base cap, hyphen-boundary
  truncation, reserved-shape detection and `parseExplicitID`.
- Added `internal/core/idreserve.go`: a never-released, `O_EXCL` reservation
  ledger at `<storage>/.runtime/ids/<kind>/<slug>` plus an allocator that mints
  `base`, `base-2`, … deterministically and reuses a keyed operation's own
  reservation after a crash.
- Wired slug IDs into workspace creation (plus orchestrator), `CreateAgent`,
  `StartSession` new-logical sessions, `CreateTaskWithID`, and the project
  Dispatcher agent/session.
- Added `CreateOptions.ID`, `AgentOptions.ID`, `SessionOptions.ID` and
  `CreateTaskWithID`; explicit IDs accept an optional prefix, return
  `invalid_id`/`id_exists`, and join the idempotency payload only when set.
- `findAgent`/`findTask` now resolve an exact ID before a name; a workspace
  rename collision maps to `id_exists`.
- Legacy ULID IDs and the migrated `sess_<hex>` alias keep working unchanged;
  no persisted reference is rewritten.
- Added `internal/core/slug_test.go` with normalization, reserved-shape,
  collision, concurrency, explicit-ID, keyed-retry, legacy-compatibility and
  tmux slug-workspace identity/recovery/stop-isolation tests.
- Adjusted one bootstrap test fixture (`internal/bootstrap/context_test.go`) so
  its "foreign workspace" case stays meaningful now that the same title in two
  projects yields the same slug.

## Checks

- `gofmt -l internal/`: clean.
- `go vet ./...`: pass.
- `go test ./... -timeout 600s`: pass.
- Targeted `-race` new core tests: pass.
- `WORKSPACE_TMUX_TEST=1 go test ./internal/core/ -run TestTmuxSlugWorkspaceIdentity`: pass.
- `WORKSPACE_TMUX_TEST=1 go test -race ./internal/core/ -timeout 300s`: pass (246 s).

Known environment limitation: the exact `WORKSPACE_TMUX_TEST=1 go test -race
./... -timeout 90s` command times out on `internal/core` in this sandbox, and
`TestNavigatorRealPTYAndClientSelection` (`internal/terminal`) cannot attach a
pseudo-TTY under WSL. Neither is caused by this change; details and evidence are
in `IMPLEMENTATION.md` and `evidence-named-ids-race-tmux-timeout.txt`.

## Handoff

All products are in `work-products/`: `IMPLEMENTATION.md`,
`CHECKS-named-ids.yaml` and `evidence-named-ids-*.txt`. T2 (`cli-docs-named-ids`)
still owns the CLI flags, TUI display and documentation.
