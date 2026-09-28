# Implementation report

## Commit

`8287d04` — `test: decouple workflow tests from legacy name`

## Changes

- Added `internal/core/testdata/extended/` as a test-only custom workflow fixture containing the extended workflow template and role prompts.
- Updated the shared core fixture to install and configure the custom `extended` workflow with the full integration, live-testing, change-request, and release capability set.
- Replaced test setup and assertions that selected the built-in legacy workflow with `extended`, including core, CLI, TUI, routing, storage, issue, deletion, migration, and lifecycle tests.
- Preserved workflow discovery assertions without coupling tests to the legacy built-in workflow name.

No production (non-test) files were modified.

## Acceptance criteria

- No internal Go test file references the legacy built-in workflow name.
- Extended-machine tests continue to exercise change requests, live testing, and release through the custom `extended` workflow.
- Test-only fixture and configuration changes leave production behavior untouched.

## Checks

- `gofmt -l ./cmd ./internal` — exit 0, empty output.
- `go vet ./...` — exit 0.
- `go test ./internal/core -count=1 -timeout 90s` — exit 0.
- `go test ./internal/bootstrap ./internal/release ./internal/terminal ./internal/tui ./internal/upgrade -count=1 -timeout 90s` — exit 0.
- `go test ./...` — did not complete within 180 seconds in this environment; the core and remaining package suites pass independently. The CLI suite also passes independently with the worker actor environment unset.
- `grep -rn issue-resolution internal --include='*_test.go'` — exit 1 (no matches).

The worker environment exports an actor/session identity, which causes CLI manual-completion tests to reject the synthetic actor as `stale_actor`; those tests pass with `WORKSPACE_AGENT_ID`, `WORKSPACE_SESSION_ID`, and `WORKSPACE_RUN_ID` unset.

## Risks and deviations

The full repository test command timed out despite the relevant package suites passing independently. No implementation deviation was made.
