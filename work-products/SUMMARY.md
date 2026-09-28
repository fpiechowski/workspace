# Summary — task_01M3MJYR9QGF20V1CN9WV9NC08 (I2: Decision audit model and rationale-carrying gate mutations)

Implemented PLAN.md §3.4 and §5-I2: the per-decision audit model and rationale
required for orchestrator gates resolved during an autonomous run.

- While `autonomy.state=running`, agent `handoff accept|reject`,
  `workflow advance`, `state update --phase` and `task retry|cancel|supersede`
  without a rationale fail with `rationale_required`. With a rationale, the gate
  and an `autonomous.*` `Decision` (`ResolvedBy: orchestrator`, `Autonomous: true`,
  `Subject`, `Evidence`, `SessionID`, `RunID`, `DecidedAt`) commit in one
  mutation; an injected save failure leaves neither behind.
- `workspace decision record` records `autonomous.assumption` and
  `autonomous.question_answer` for the orchestrator only while running, with
  idempotent receipts.
- Rejection and retry bounds (2/1) are tracked per task per orchestrator Run;
  exceeding them returns `autonomy_bound_exceeded`.
- Historical behavior is preserved: the non-audited entry points and existing
  receipts keep their digests because rationale/evidence join the payload only
  when non-empty. Tests, CLI help and flags were updated.

Commit `4072865`. Checks (`work-products/CHECKS-I2.yaml`): gofmt clean,
`go vet ./...`, the targeted core run, `go test ./internal/cli -count=1`, and the
full `go test ./...` all pass. Docs, report, menu and the safety boundary remain
in the I3/I4 scope.
