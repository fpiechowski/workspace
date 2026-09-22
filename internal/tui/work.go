package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"workspace/internal/core"
)

// Only a session's current run represents live work; history never supplies a
// replacement terminal target or contributes to the active count.
func (m *Model) currentRun(session core.Session) (core.Run, bool) {
	run, ok := m.run(session.CurrentRunID)
	return run, ok && run.SessionID == session.ID && run.Active()
}

func (m *Model) taskSessions(task core.Task) []core.Session {
	var sessions []core.Session
	for _, session := range m.snapshot.Status.Sessions {
		if session.DeletedAt != nil {
			continue
		}
		if session.TaskID == task.ID && session.TaskAttempt == task.Attempt {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

func (m *Model) taskItem(task core.Task) collectionItem {
	sessions := m.taskSessions(task)
	active, runs := 0, 0
	var names []string
	var at time.Time
	for _, session := range sessions {
		if _, ok := m.currentRun(session); ok {
			active++
			names = append(names, session.AgentSnapshot.Name)
		}
		for _, run := range m.snapshot.Status.Runs {
			if run.SessionID == session.ID {
				runs++
				if run.CreatedAt.After(at) {
					at = run.CreatedAt
				}
			}
		}
	}
	execution := fmt.Sprintf("%d live · %d sessions · %d runs", active, len(sessions), runs)
	if len(names) > 0 {
		execution += " · " + strings.Join(names, ", ")
	}
	if len(sessions) == 0 {
		execution = "No session yet"
	}
	return collectionItem{ID: task.ID, Kind: "task", Title: firstNonempty(task.Title, task.ID), State: task.State,
		Subtitle: fmt.Sprintf("%s · attempt %d", execution, task.Attempt), At: at}
}

func workPriority(state string) int {
	switch state {
	case "running", "starting", "active":
		return 0
	case "failed", "error", "blocked", "needs_changes", "interrupted":
		return 1
	case "submitted", "awaiting_review", "pending_review":
		return 2
	case "accepted", "completed", "closed", "archived":
		return 4
	default:
		return 3
	}
}

func (m *Model) sortItems(items []collectionItem) {
	if m.route.Sort == "" && m.route.Page != "tasks" {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		switch m.route.Sort {
		case "name":
			return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
		case "recent":
			return items[i].At.After(items[j].At)
		default:
			return workPriority(items[i].State) < workPriority(items[j].State)
		}
	})
}

func (m *Model) stateLabel(state string) string {
	label := statusBadge(state)
	color := m.palette.secondary
	switch state {
	case "running", "starting":
		return m.runningGlyph() + " " + m.palette.style(m.palette.accent, false).Render(state)
	case "accepted", "completed", "ready":
		color = m.palette.success
	case "failed", "error":
		color = m.palette.danger
	case "blocked", "needs_changes", "submitted", "awaiting_review", "pending_review", "interrupted":
		color = m.palette.warning
	}
	return m.palette.style(color, false).Render(label)
}

// t opens live work or presents an explicit confirmation to create a new run.
func (m *Model) openTerminal() tea.Cmd {
	if m.backend == nil || m.snapshot.ObservedAt.IsZero() {
		m.notice = "Terminal actions need a current snapshot. Press r to refresh."
		return nil
	}
	kind, id := m.route.Page, m.route.EntityID
	if m.isCollectionPage() {
		item, _, _ := m.selectedItem()
		kind, id = item.Kind, item.ID
	}
	if kind == "task" {
		task, ok := m.task(id)
		if !ok {
			return nil
		}
		sessions := m.taskSessions(task)
		if len(sessions) == 0 {
			m.notice = "No session for this attempt. Press o, then t to start the orchestrator."
			return nil
		}
		if len(sessions) > 1 {
			m.push(route{Page: "sessions", ParentID: id})
			m.notice = "Choose an agent, then t to open its terminal."
			return nil
		}
		kind, id = "session", sessions[0].ID
	}
	if kind == "agent" {
		m.push(route{Page: "sessions", ParentID: id})
		m.notice = "Choose a session, then t to open or resume its terminal."
		return nil
	}
	if kind == "orchestrator" {
		if m.snapshot.Status.Workspace.Status == "archived" {
			m.notice = "Archived workspaces cannot start or resume the orchestrator."
			return nil
		}
		if session, _ := m.orchestrator(); session != nil {
			kind, id = "session", session.ID
		} else {
			return m.terminalAction("start_orchestrator", "")
		}
	}
	if kind == "session" {
		session, ok := findSession(m.snapshot.Status.Sessions, id)
		if !ok {
			return nil
		}
		if m.snapshot.Status.Workspace.Status == "archived" {
			m.notice = "Archived workspaces cannot resume sessions."
			return nil
		}
		if run, active := m.currentRun(session); active {
			return m.openDedicated(core.EntityRef{Kind: "run", ID: run.ID})
		}
		if session.ClosedAt != nil {
			m.notice = "This session is closed. Open the orchestrator to plan a new assignment."
			return nil
		}
		return m.terminalAction("resume_session", id)
	}
	if kind == "run" || kind == "worktree" || kind == "service" || kind == "workspace" {
		return m.openDedicated(core.EntityRef{Kind: kind, ID: id})
	}
	m.notice = "Select an agent, session, or task to open its terminal."
	return nil
}

func (m *Model) terminalAction(action, id string) tea.Cmd {
	cmd := m.beginAction(action, id)
	m.formAction.OpenTerminal = true
	return cmd
}

func rowText(value string, width int) string {
	return ansi.TruncateWc(sanitizeLine(value), max(1, width), "…")
}
