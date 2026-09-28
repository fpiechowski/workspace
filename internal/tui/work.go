package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

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

func shortRevision(commit string) string {
	if commit == "" {
		return "unknown"
	}
	if len(commit) > 10 {
		return commit[:10]
	}
	return commit
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

func rowText(value string, width int) string {
	return ansi.TruncateWc(sanitizeLine(value), max(1, width), "…")
}
