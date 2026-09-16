package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"workspace/internal/core"
)

func (m *Model) navigationFailure(err error, ref core.EntityRef, afterReconcile bool) tea.Cmd {
	m.loadError = sanitizeLine(err.Error())
	var failure *core.Error
	_, canAct := m.backend.(ActionBackend)
	if !afterReconcile && canAct && m.workspaceID != "" && ref.Kind != "" && m.form == nil && !m.actionPending && errors.As(err, &failure) && failure.Code == "pane_missing" {
		m.formAction = ActionCall{Action: "reconcile", WorkspaceID: m.workspaceID, Key: core.ID("tui"), ExpectedRevision: m.snapshot.Status.Workspace.Revision, NavigationRef: &ref}
		return m.openConfirm("reconcile")
	}
	if afterReconcile {
		m.notice = "Reconcile finished, but this terminal is unavailable. Open Agents & runs (l) and use t to open or resume a session."
	} else {
		m.notice = "Terminal unavailable: " + m.loadError
	}
	m.rebuildViewport()
	return nil
}
