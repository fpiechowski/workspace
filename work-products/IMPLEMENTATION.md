# Implementation: Interactive project initialization wizard

Task: `task_01M3J8B6JEH62S34VM1SRPFT54`

## Changes

- Added a TTY-only wizard for fresh `workspace project init` runs.
- Added cancellable, injectable prompt I/O with validation and Enter defaults.
- Added detected built-in adapter selection, account-specific provider/model prompts,
  optional README role/workflow mappings, optional forge suggestion, summary, and
  final write confirmation.
- Fresh candidates are handed to `core.InitFreshProject`; existing projects and all
  scripted modes retain the legacy initialization path.
- Built-in launch/resume defaults are used without permission-bypass flags.
- Added isolated Git-fixture tests for successful setup and cancellation.

## Files

- `internal/cli/cli.go`
- `internal/cli/project_init.go`
- `internal/cli/project_init_test.go`

## Acceptance

The wizard is gated by both terminal streams, `--json`, `--short`,
`--non-interactive`, and operation-key mode. Prompts/progress use stderr, while the
final configuration remains on stdout. Existing configuration is delegated to the
byte-preserving core path. Cancellation and EOF leave an uninitialized project.

## Evidence

| Command | Exit | Evidence |
| --- | ---: | --- |
| `gofmt -l internal/cli/cli.go internal/cli/project_init.go internal/cli/project_init_test.go` | 0 | `check_01M3J9Z4CFC70MHXACA20HDFAV` |
| `env -u WORKSPACE_SESSION_ID go test ./internal/cli ./internal/core -run 'TestProjectInitWizard|TestValidateConfig|TestInitFreshProject' -count=1` | 0 | `check_01M3J9ZAC995X62NH22JW3K02N` |
| `go vet ./internal/cli` | 0 | `check_01M3J9ZXF3SJKHDG96GPNWAH58` |

## Risks

The full CLI package contains two pre-existing environment-sensitive workspace
tests that fail independently of this change. The focused wizard/core checks pass.

Commit: `pending documentation commit`
