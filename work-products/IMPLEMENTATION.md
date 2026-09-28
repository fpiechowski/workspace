# T3 implementation report

## Commit

The implementation commit is recorded below after verification.

## Changes

- Removed the embedded `issue-resolution` workflow templates and removed its listing/selection special cases.
- Made `plan-first` the effective default for every create path unless `--no-workflow` is explicit. The effective default is applied after replay lookup, so older operation payloads remain replayable.
- Updated explicit legacy workflow errors with the removal hint and updated project-init workflow configuration to plan-first v2, including the integration profile and landing capabilities.
- Added stock-template refresh for known plan-first template digests, `stale_templates` reporting for customized files, embedded prompt fallback, and the plan-first v2 integration/orchestrator templates.
- Updated CLI flag/help text and preselected plan-first in the TUI create form.
- Updated tests whose prior contract expected omitted workflows to remain pending; explicit manual tests now use `--no-workflow`.

## Acceptance criteria

- Workflow listing no longer exposes `issue-resolution`, including when old on-disk templates or config entries exist; explicit use returns `unknown_workflow` with the removal hint.
- Omitted workflow creates produce active plan-first workspaces, while `--no-workflow` produces manual workspaces; replay comparison uses the pre-upgrade request payload.
- Project initialization refreshes known stock templates, preserves customized templates while reporting them, and snapshots missing plan-first prompts from the embedded filesystem.
- Embedded legacy templates are deleted and plan-first v2 documentation includes integration preparation, conflict handling, user-approved landing, and completion.

## Checks

- `gofmt -l ./cmd ./internal` — exit 0, empty output.
- `go vet ./...` — exit 0.
- `env -u WORKSPACE_AGENT_ID -u WORKSPACE_SESSION_ID -u WORKSPACE_RUN_ID go test ./... -count=1 -timeout 180s` — exit 0.
- `go build -o /tmp/workspace-t3-removal ./cmd/workspace && python3 scripts/check-install.py /tmp/workspace-t3-removal` — exit 0.

The worker identity variables were unset for the full Go test command because existing CLI tests intentionally reject inherited worker identity in synthetic actor scenarios.

## Risks and deviations

- `stale_templates` is a result-only field (`yaml:"-"`) and is not written into the persisted project configuration.
- The extended workflow capability fallback remains in capability snapshots for compatibility with legacy workspace migration; it is no longer bundled or selectable.
