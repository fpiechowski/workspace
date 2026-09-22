package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// animationInterval is the shared cadence for the pending spinner and the live
// run glyph. The loop only runs while the spinner is needed.
const animationInterval = 120 * time.Millisecond

// readPending reports non-blocking reads that should animate the spinner: first
// load, background refresh, worktree inspection, preview read, workflow lookup
// and terminal navigation.
func (m *Model) readPending() bool {
	return m.projectPending || m.issuePending || m.supervisorPending || m.snapshotPending || m.runtimePending ||
		m.uiPending || m.previewPending || m.navigationPending || m.worktreePending
}

// mutationActive reports a blocking write that owns the interface until its
// result arrives. Managed hide is a persisted write, so it is blocking too.
func (m *Model) mutationActive() bool {
	return m.actionPending || m.hidePending
}

// pendingWork reports any asynchronous work that keeps the spinner visible.
func (m *Model) pendingWork() bool {
	return m.readPending() || m.mutationActive()
}

// hasLiveRun reports whether a session's current run is visibly active.
func (m *Model) hasLiveRun() bool {
	for _, session := range m.snapshot.Status.Sessions {
		if _, ok := m.currentRun(session); ok {
			return true
		}
	}
	return false
}

// animationNeeded is the single gate for the animation loop: a spinner must be
// visible either because work is pending or because a live run is on screen.
func (m *Model) animationNeeded() bool {
	return m.pendingWork() || m.hasLiveRun()
}

// animationTick schedules one animation frame. It never checks the gate itself;
// callers must use ensureAnimation so at most one loop is active.
func (m *Model) animationTick() tea.Cmd {
	return tea.Tick(animationInterval, func(time.Time) tea.Msg { return animationMsg{} })
}

// ensureAnimation arms the animation loop exactly once when it is needed. It is
// called after every update, so a transition into pending work starts the loop
// and an idle model issues no tick.
func (m *Model) ensureAnimation() tea.Cmd {
	if m.quit || m.closed || m.animating || !m.animationNeeded() {
		return nil
	}
	m.animating = true
	return m.animationTick()
}

// spinnerGlyph is the current frame of the shared bubbles spinner.
func (m *Model) spinnerGlyph() string {
	return m.spinner.View()
}

// runningGlyph animates a running indicator only while the spinner loop is
// active. When idle it degrades to a static marker instead of a frozen frame.
func (m *Model) runningGlyph() string {
	if m.animationNeeded() {
		return m.spinnerGlyph()
	}
	return "●"
}

// actionCompletedNotice is the transient success status shown while the
// post-action refresh is still running. The header indicator communicates the
// refresh independently and this notice stays concise.
const actionCompletedNotice = "Action completed."

// completeAction clears the transient success status after the post-action
// refresh has landed so later refreshes are not mislabelled as success.
func (m *Model) completeAction() {
	if !m.actionCompleted {
		return
	}
	m.actionCompleted = false
	if m.notice == actionCompletedNotice {
		m.notice = ""
	}
}

// statusKind classifies the reserved status/notice row so each state can be
// presented with its own wording and text marker. Read progress is intentionally
// absent: the header owns that compact indicator.
type statusKind int

const (
	statusNone statusKind = iota
	statusNotice
	statusSuccess
	statusMutation
	statusFailure
	statusStale
	statusScroll
)

func (m *Model) statusKind() statusKind {
	switch {
	case m.actionFailure && m.loadError != "":
		return statusFailure
	case m.mutationActive():
		return statusMutation
	case m.actionCompleted && m.notice != "":
		return statusSuccess
	case m.notice != "":
		return statusNotice
	case m.visibleReadError() != "":
		return statusStale
	case m.isDetailPage() && m.scrollPosition() != "":
		return statusScroll
	case m.headerServiceOverflow() != "":
		return statusNotice
	default:
		return statusNone
	}
}

// statusLine returns the status text and its text marker. Markers keep every
// state recognizable without color.
func (m *Model) statusLine() (string, string) {
	health := m.headerServiceOverflow()
	appendHealth := func(text string) string {
		if health == "" {
			return text
		}
		if text == "" {
			return health
		}
		return health + " · " + text
	}
	switch m.statusKind() {
	case statusFailure:
		return appendHealth("Action failed: " + m.loadError + " · y retry · a new action"), "[!] "
	case statusMutation:
		return appendHealth(m.mutationStatus()), ""
	case statusStale:
		return appendHealth(m.staleStatus()), "[!] "
	case statusSuccess:
		return appendHealth(m.notice), "✓ "
	case statusNotice:
		return appendHealth(m.notice), "· "
	case statusScroll:
		return appendHealth(m.scrollPosition()), ""
	default:
		if health != "" {
			return health, "· "
		}
		return "", ""
	}
}

func (m *Model) visibleReadError() string {
	if m.loadError != "" {
		return m.loadError
	}
	return m.supervisorReadError
}

// mutationStatus names the blocking write so it is not confused with a refresh.
func (m *Model) mutationStatus() string {
	if m.hidePending {
		return "Saving hide request… keep this panel open"
	}
	if m.lastAction == nil {
		if m.notice != "" {
			return m.notice
		}
		return "Applying action… keep this panel open"
	}
	return actionVerb(m.lastAction.Action) + "… keep this panel open"
}

// staleStatus keeps failed refreshes explicit while the previous snapshot stays
// on screen.
func (m *Model) staleStatus() string {
	error := m.visibleReadError()
	if !m.lastSuccess.IsZero() {
		return "Stale data from " + timeAgo(m.lastSuccess) + " ago · " + error + " · r retry"
	}
	return "Refresh failed: " + error + " · r retry"
}

// actionVerb is the short present-tense label used by the blocking status row.
func actionVerb(action string) string {
	switch action {
	case "delete_workspace":
		return "Deleting workspace"
	case "delete_task":
		return "Deleting task"
	case "delete_session":
		return "Deleting session"
	case "archive_workspace":
		return "Archiving workspace"
	case "complete_workspace":
		return "Completing workspace"
	case "reopen_workspace":
		return "Reopening workspace"
	case "retry_task":
		return "Retrying task"
	case "close_session":
		return "Closing session"
	case "stop_run":
		return "Stopping run"
	case "stop_service":
		return "Stopping service"
	case "pause":
		return "Pausing workspace"
	case "pause_interrupt":
		return "Pausing and interrupting"
	case "resume_workspace":
		return "Resuming workspace"
	case "resume_session":
		return "Resuming session"
	case "create_workspace":
		return "Creating workspace"
	case "start_orchestrator":
		return "Starting orchestrator"
	case "select_workflow":
		return "Selecting workflow"
	case "reconcile":
		return "Reconciling runtime"
	case "show_managed_tui", "hide_managed_tui":
		return "Updating managed panel"
	default:
		return "Applying action"
	}
}
