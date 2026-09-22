package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"workspace/internal/core"
)

func (m *Model) navigationFailure(err error, ref core.EntityRef, afterReconcile bool, modes ...NavigationMode) tea.Cmd {
	m.loadError = sanitizeLine(err.Error())
	mode := NavigationModeJump
	if len(modes) > 0 && modes[0] != "" {
		mode = modes[0]
	}
	var failure *core.Error
	_, canAct := m.backend.(ActionBackend)
	if !afterReconcile && canAct && m.workspaceID != "" && ref.Kind != "" && m.form == nil && !m.actionPending && errors.As(err, &failure) && failure.Code == "pane_missing" {
		m.formAction = ActionCall{Action: "reconcile", WorkspaceID: m.workspaceID, Key: core.ID("tui"), ExpectedRevision: m.snapshot.Status.Workspace.Revision, NavigationRef: &ref, NavigationMode: mode}
		return m.openConfirm("reconcile")
	}
	if afterReconcile {
		m.notice = "Reconcile finished, but this terminal is unavailable. Open Sessions (2) or the Orchestrator (o) and use t to open or resume a session."
	} else {
		m.notice = "Terminal unavailable: " + m.loadError
	}
	m.rebuildViewport()
	return nil
}
