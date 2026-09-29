# SUMMARY — T2: Named IDs CLI flags, TUI display, documentation

Outcome: success. Implemented PLAN.md task T2 on top of the accepted T1 core work.

- Commit: `7b5963373e3cfe9f652e0014f88c9f515e5e027f`.
- Worktree: `worktrees/impl-cli-docs-rebased`.
- Base: `673bb6514fad11f29c87b880d1307d30e0e5d532`; T1 head: `41fa124`.

## Delivered

- `--id` flags on `workspace create`, `issue dispatch`, `agent create`,
  `task create --spec-file`, and `session start`, wired to the T1 options. Explicit IDs
  surface `invalid_id`/`id_exists` with a non-zero exit.
- `workspace open` exact-ID precedence: an ID match wins over a title match and is never
  reported as ambiguous.
- Selector help text and `help.go` examples now use slug-shaped IDs and mention slug or
  legacy IDs.
- TUI shows full, untruncated slug IDs for workspace, agent, session, and task rows;
  Run IDs keep the compact form.
- Docs updated: `ARCHITECTURE.md` (format, per-storage-root uniqueness, never-released
  reservation ledger, legacy compatibility, persisted-state tree), `README.md` (ID note
  and `--id` examples), `docs/operations.md` (`--id` in the payload, keyed retry, errors),
  `docs/runtime.md` (tmux name `workspace-<workspace-id>`).

## Checks

- `gofmt -l internal/ cmd/` clean; `go vet ./...` clean.
- `go test ./... -count=1 -timeout 600s` passes (exit 0).
- Targeted `-race` CLI/TUI tests pass; `python3 work-products/check-doc-links.py` reports
  `checked=30 broken=0`.
- Evidence files: `evidence-named-ids-cli-gofmt.txt`, `-vet.txt`, `-go-test-all.txt`,
  `-targeted-race.txt`, `-doc-links.txt`, `-help.txt`; manifest
  `CHECKS-named-ids-cli.yaml`.

## Risks / limitations

- A live CLI-level `session start --id` test is omitted because it needs the supervisor and
  a real client; the flag forwarding is trivial and T1 covers `SessionOptions.ID`.
- `IssueDispatchOptions.ID` is a small `omitempty` core addition needed to carry `--id`
  through `issue dispatch`, keeping existing dispatch receipts byte-compatible.
- The tmux suite was not run (no runtime/tmux code touched); it is optional for T2.
