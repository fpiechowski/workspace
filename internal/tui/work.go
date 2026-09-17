package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
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

func (m *Model) workItems() []collectionItem {
	var items []collectionItem
	orch, _ := m.orchestrator()
	for _, session := range m.snapshot.Status.Sessions {
		isOrch := orch != nil && orch.ID == session.ID
		run, active := m.currentRun(session)
		if !isOrch && !active {
			task, ok := m.task(session.TaskID)
			if !ok || task.Attempt != session.TaskAttempt || task.State == "accepted" || session.ClosedAt != nil {
				continue
			}
		}
		title := firstNonempty(session.AgentSnapshot.Name, session.ID)
		assignment := "No task assigned"
		if task, ok := m.task(session.TaskID); ok {
			assignment = task.Title + " · " + task.State
		}
		if isOrch {
			title = "Orchestrator · " + title
			assignment = "Coordinates workspace"
		}
		state, model, hint := firstNonempty(session.LifecycleState, "idle"), session.Route.Model, "t resume terminal"
		if active {
			state, model, hint = run.State, run.Route.Model, "t open terminal"
		} else if last, ok := m.run(session.LastRunID); ok {
			state = last.State
		}
		if session.ClosedAt != nil {
			hint = "Session closed"
		}
		items = append(items, collectionItem{ID: session.ID, Kind: "session", Title: title, State: state,
			Subtitle: assignment + " · " + firstNonempty(model, "model unspecified") + " · " + hint, At: session.LastActiveAt})
	}
	if orch == nil {
		items = append(items, collectionItem{ID: "orchestrator", Kind: "orchestrator", Title: "Orchestrator", State: "idle", Subtitle: "No session yet · t start terminal"})
	}
	sort.SliceStable(items, func(i, j int) bool {
		pi, pj := workPriority(items[i].State), workPriority(items[j].State)
		if pi != pj {
			return pi < pj
		}
		return items[i].At.After(items[j].At)
	})
	return items
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
	if m.route.Sort == "" && m.route.Page != "tasks" && m.route.Page != "dashboard" && m.route.Page != "work" {
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

// workMetrics is the single source for the dashboard counters. Task completion
// is derived only from the accepted state: a finished process is not an
// accepted result.
type workMetrics struct {
	total     int
	accepted  int
	working   int
	review    int
	blocked   int
	live      int
	attention int
}

func (m *Model) workMetrics() workMetrics {
	metrics := workMetrics{attention: len(m.attentionItems())}
	for _, task := range m.snapshot.Status.Workspace.Tasks {
		if task.DeletedAt != nil {
			continue
		}
		metrics.total++
		switch task.State {
		case "accepted":
			metrics.accepted++
		case "running":
			metrics.working++
		case "submitted", "awaiting_review", "pending_review":
			metrics.review++
		case "blocked", "needs_changes":
			metrics.blocked++
		}
	}
	for _, session := range m.snapshot.Status.Sessions {
		if _, ok := m.currentRun(session); ok {
			metrics.live++
		}
	}
	return metrics
}

// progressBar renders the accepted-task ratio through bubbles/progress with the
// semantic accent token. No-color mode strips the escape sequences so the bar
// stays readable; the textual accepted/total and percent always accompany it.
func (m *Model) progressBar(ratio float64) string {
	bar := progress.New(
		progress.WithWidth(min(24, max(8, m.width/5))),
		progress.WithFillCharacters('━', '─'),
		progress.WithoutPercentage(),
	)
	bar.FullColor = string(m.palette.accent)
	bar.EmptyColor = string(m.palette.subtle)
	out := bar.ViewAs(ratio)
	if m.palette.noColor {
		out = ansi.Strip(out)
	}
	return out
}

func (m *Model) progressLines() []string {
	metrics := m.workMetrics()
	if metrics.total == 0 {
		return []string{"No tasks yet · start the orchestrator with t", "Progress appears when the orchestrator creates tasks."}
	}
	ratio := float64(metrics.accepted) / float64(metrics.total)
	return []string{fmt.Sprintf("%s  %d/%d accepted · %d%%", m.progressBar(ratio), metrics.accepted, metrics.total, 100*metrics.accepted/metrics.total)}
}

// workStatusBar keeps live, review, blocked, and attention counts separate so
// the dashboard summary does not rely on color or a single blended number.
func (m *Model) workStatusBar(metrics workMetrics) string {
	text := fmt.Sprintf("%s %d live · %d review · %d blocked · %d attention",
		m.liveGlyph(), metrics.live, metrics.review, metrics.blocked, metrics.attention)
	return m.palette.metaStyle().Render(text)
}

var workSections = []string{"Agents & runs", "Tasks", "Needs attention", "Recent recorded activity"}

func (m *Model) workSectionName() string {
	index := m.focusedPanel
	if index < 0 || index >= len(workSections) {
		index = 0
	}
	if m.width < 75 {
		return []string{"Agents", "Tasks", "Attention", "Activity"}[index]
	}
	return workSections[index]
}

func (m *Model) workSectionHeading() string {
	sections := make([]string, 0, len(workSections))
	for i, title := range workSections {
		if m.width < 75 {
			title = []string{"Agents", "Tasks", "Attention", "Activity"}[i]
		}
		if m.focusedPanel == i {
			title = "[" + title + "]"
		}
		sections = append(sections, title)
	}
	return m.palette.headingStyle().Render(strings.Join(sections, "  "))
}

func (m *Model) workDashboard(mode layoutMode) []string {
	if m.snapshot.ObservedAt.IsZero() {
		return []string{m.dashboardContent()}
	}
	metrics := m.workMetrics()
	lines := m.progressLines()
	lines = append(lines, m.workStatusBar(metrics))
	lines = append(lines, "", m.workSectionHeading())
	items := m.filteredItems()
	filter := ""
	if m.route.StatusFilter != "" {
		filter = " · state: " + m.route.StatusFilter
	}
	if m.route.Query != "" {
		filter += " · /" + m.route.Query
	}
	lines = append(lines, fmt.Sprintf("%s%s · %d shown · sort: %s", workSections[m.focusedPanel], filter, len(items), firstNonempty(m.route.Sort, "priority")))
	if len(items) == 0 {
		return append(lines, "No records in this view", "Nothing here matches this section and its filters.", "Esc clears the filter · / searches · f filters by state.")
	}
	available := max(3, m.contentHeight()-len(lines))
	if mode == layoutWide {
		leftWidth, rightWidth := m.panelWidths(3, 28)
		left := m.renderItems(items, max(1, leftWidth-2), available-2)
		right := truncateLines(m.itemSummary(items, max(1, rightWidth-2)), max(1, rightWidth-2))
		return append(lines, m.twoPanels(m.workSectionName(), left, "Preview", right, leftWidth, rightWidth, available, true)...)
	}
	return append(lines, m.renderItems(items, m.width, available)...)
}

func (m *Model) liveGlyph() string {
	if m.hasLiveRun() {
		return m.spinnerGlyph()
	}
	return "○"
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
		if run, active := m.currentRun(session); active {
			return m.jump(core.EntityRef{Kind: "run", ID: run.ID})
		}
		if session.ClosedAt != nil {
			m.notice = "This session is closed. Open the orchestrator to plan a new assignment."
			return nil
		}
		return m.terminalAction("resume_session", id)
	}
	if kind == "run" || kind == "worktree" || kind == "service" || kind == "workspace" {
		return m.jump(core.EntityRef{Kind: kind, ID: id})
	}
	m.notice = "Select an agent or task to open its terminal. l opens Agents & runs."
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
