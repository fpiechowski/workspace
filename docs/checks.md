# Captured checks

Workers can capture a real command without a shell wrapper:

```sh
workspace check run --operation-key test:attempt-1 -- npm test
workspace check list --json
workspace handoff submit --task "$WORKSPACE_TASK_ID" \
  --summary-file work-products/SUMMARY.md \
  --artifact work-products/IMPLEMENTATION.md --check check_RETURNED_ID
```

Commit code before running a check intended for acceptance. A captured check records
the argv, producing Session and Task attempt, start/end HEAD, exit code and output
digest. Handoff requires the same clean Git revision and Session. CLI command success
means the receipt was recorded: inspect its `exit_code` for the test result. Failed
checks remain visible and cannot be accepted as successful verification.

Output is saved in `work-products/checks/` and independently under the workspace's
runtime directory; handoff copies the preserved bytes into an immutable artifact.
Editing the worker's local output does not alter the captured evidence. The output
limit is 16 MiB. Repeating an operation key returns its receipt and never reruns the
command. An interrupted check needs a new key after inspecting its prior effects.

Existing external test evidence can still be submitted with `--checks-file` and an
explicit evidence artifact. Its YAML schema is:

```yaml
- command: npm test
  exit_code: 0
  evidence: CHECKS.log
```

This is imported evidence, not a CLI-captured execution. The orchestrator must inspect
it before acceptance. Neither form is a security boundary against arbitrary programs
running under the same operating-system user.
