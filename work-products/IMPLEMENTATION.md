# Implementation: Documentation, help and evidence alignment for the init wizard

Task: `task_01M3J8B6THN6Z4GA8ZE305ZM4J`

## Changes

- `README.md`
  - Added a `### Project initialization` subsection that documents the interactive
    setup wizard: when it activates (fresh project, terminal on both stdin and stdout,
    no `--json`/`--short`/`--non-interactive`/`--operation-key`), what it asks (start
    confirmation, detected client adapters, orchestrator client/provider/model/effort/
    concurrency, optional README role profiles and workflow mappings, optional GitHub
    forge), and how confirmation and cancellation work (`Write configuration? [y/N]`;
    `n`, `q`, `cancel`, EOF, or `Ctrl-C` writes nothing).
  - Documented the non-interactive/script mode selection and that it writes the minimal
    generated configuration without prompting, plus the byte-for-byte preservation of an
    existing valid configuration and the error for an invalid one.
  - Removed `--auto` from the example OpenCode `launch_argv`/`resume_argv` so the example
    matches the safe built-in defaults, and rewrote the note so approval-bypass flags are
    clearly labeled as a deliberate opt-in that the user must add.
- `internal/cli/help.go`
  - Rewrote the `workspace project init` help to describe the interactive wizard, the
    script/non-terminal mode, byte-for-byte preservation, and the absence of
    approval-bypass flags; added a scripted example.
- `PRODUCT.md`
  - Added a scope bullet and a User Experience paragraph for validated first-run project
    setup, account-specific identifiers, script behavior, configuration preservation,
    and the deliberate-opt-in approval model.
- `ARCHITECTURE.md`
  - Documented the fresh-initialization boundary in "Persisted State": candidate
    validation uses the same rules as a persisted config, the candidate is written once
    under the project lock, existing configs are preserved byte-for-byte, keyed replays
    include the finalized candidate, and the CLI wizard only builds the candidate.

## Key decisions

- Left `docs/clients.md` unchanged: the adapter capabilities and native-delivery
  guidance did not change; only the README example that favored `--auto` needed
  alignment.
- Kept the documentation scoped to the init/configuration contract and did not touch
  unrelated behavior.

## Acceptance criteria

- README documents wizard activation, questions, cancellation, confirmation, script
  mode, and an unambiguous approval-safe example.
- `workspace project init` help describes both modes accurately.
- PRODUCT.md and ARCHITECTURE.md are updated only for the changed init/configuration
  contract and remain in English with working links.
- Verification recorded as real check receipts (below).

## Files

- `README.md`
- `internal/cli/help.go`
- `PRODUCT.md`
- `ARCHITECTURE.md`

## Evidence

| Command | Exit | Check receipt | Evidence |
| --- | ---: | --- | --- |
| `gofmt -l internal/cli/help.go` | 0 | `check_01M3JB5SF011BE1APN99XPFZN7` | `work-products/checks/check_01M3JB5SF011BE1APN99XPFZN7.log` |
| `env -u WORKSPACE_* go test ./...` | 0 | `check_01M3JB5ZN3D316BRDTXZG6HQR1` | `work-products/checks/check_01M3JB5ZN3D316BRDTXZG6HQR1.log` |
| `env -u WORKSPACE_* go vet ./...` | 0 | `check_01M3JBA4NVN8F0FBMV2H6HQXH3` | `work-products/checks/check_01M3JBA4NVN8F0FBMV2H6HQXH3.log` |
| `env CGO_ENABLED=0 go build -trimpath -o /tmp/opencode/workspace-impl-docs2-29f65ae ./cmd/workspace` | 0 | `check_01M3JBAER618HM86GAE4E8DNNK` | `work-products/checks/check_01M3JBAER618HM86GAE4E8DNNK.log` |
| `python3 scripts/check-install.py /tmp/opencode/workspace-impl-docs2-29f65ae` | 0 | `check_01M3JBAR2XE8Q8P9KVCZMGG3HN` | `work-products/checks/check_01M3JBAR2XE8Q8P9KVCZMGG3HN.log` |

`go test ./...` passes only when the sandbox-injected `WORKSPACE_*` variables are
removed; with them set, two pre-existing environment-sensitive CLI tests
(`TestCompleteAndArchiveManualWorkspace`, `TestReopenJSONAndHelpContract`) fail
independently of this change.

## Risks

- The full `go test ./...` requires a clean environment; the worktree/session exports
  `WORKSPACE_*`, so the recorded command unsets them explicitly.
- `docs/clients.md` was intentionally not modified; if reviewers expect a note there,
  it would be a separate adapter-guidance change not caused by this task.

Commit: `37c15a2` (`docs: align project init wizard documentation and help`)
