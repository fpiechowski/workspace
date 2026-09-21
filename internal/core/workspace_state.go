package core

// rejectNewWorkspaceWork distinguishes a completed workspace, which still
// permits explicitly started conversations, from an archived workspace, which
// is terminal. Resource creation and ordinary task execution use this guard so
// they do not report a completed workspace as generically closed.
func rejectNewWorkspaceWork(d *Document, operation string) error {
	switch d.State.Status {
	case "completed":
		return fail("workspace_completed", "workspace is completed; reopen it before %s", operation)
	case "archived":
		return fail("workspace_archived", "workspace is archived; %s is not available", operation)
	default:
		return nil
	}
}
