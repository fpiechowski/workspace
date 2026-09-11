# Input and workflow revisions

Pause delegation and stop active workers before revising a workspace. The orchestrator
may remain active to conduct the conversation. Use the revision from `workspace status`:

```sh
workspace pause
workspace input update --input-file revised-issue.md \
  --reason 'Acceptance criteria clarified' --expected-revision 42 \
  --operation-key input-revision-2
workspace workflow migrate --reason 'Updated workflow requirements' \
  --expected-revision 43 --operation-key workflow-revision-2
```

Input updates preserve the old input and WORKSPACE.md under `history/revision_ID/`.
Workflow migration compares project templates with workspace snapshots, preserves
changed originals and records before/after hashes in `migration.json`. Previous
Session prompts are never rendered again or overwritten.

These operations reset dependent results for re-evaluation, increment task attempts,
invalidate integration/live-test results and mark existing CRs outdated. They preserve
artifacts and handoffs. The workspace stays paused; resume when the next steps are clear.
Released work cannot be silently reopened. A replayed operation key does not create
another revision. Files and state share a recoverable write-ahead record.

`state update` is for title, narrative and validated phase/status changes. It does not
bypass task acceptance or release gates. `state edit` permits a paused human edit of
the narrative/title while retaining the machine-maintained workflow state.
