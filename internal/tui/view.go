package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) View() string {
	if m.width < 1 || m.height < 1 {
		return ""
	}
	mode := layoutFor(m.width, m.height)
	if mode == layoutTiny {
		message := "Terminal too small · need 40×12 · " + m.exitHint()
		if m.width < 24 {
			message = "Too small · q"
		}
		return fitFrame(message, m.width, m.height)
	}
	lines := []string{m.header()}
	if m.showHelp {
		lines = append(lines, m.helpView())
	} else {
		lines = append(lines, m.tabs())
		body := m.bodyView(mode)
		lines = append(lines, body...)
	}
	lines = append(lines, m.footer())
	// Reserve the last row for controls even when a form or body overflows.
	body := strings.Join(lines[:len(lines)-1], "\n")
	if m.notice != "" {
		return fitFrame(body, m.width, max(1, m.height-2)) + "\n" + rowText(m.notice, m.width) + "\n" + ansi.TruncateWc(m.footer(), m.width, "")
	}
	return fitFrame(body, m.width, max(1, m.height-1)) + "\n" + ansi.TruncateWc(m.footer(), m.width, "")
}

func (m *Model) header() string {
	project := m.projectRoot
	if project == "" {
		project = "No project found"
	}
	project = shortPath(project, max(12, m.width/3))
	var parts []string
	if m.workspaceID == "" || m.route.Page == "project" {
		parts = append(parts, "Project", project)
	} else {
		name := m.snapshot.Status.Workspace.Title
		if name == "" {
			name = m.workspaceID
		}
		parts = append(parts, name)
		parts = append(parts, statusBadge(m.snapshot.Status.Workspace.Status))
		if phase := m.snapshot.Status.Workspace.Workflow; phase != nil && phase.Phase != "" {
			parts = append(parts, "·", phase.Phase)
		}
	}
	if m.initialError != "" {
		parts = append(parts, "·", "[!] "+m.initialError)
	} else if m.loadError != "" {
		stale := "[!] refresh failed"
		if !m.lastSuccess.IsZero() {
			stale = "[!] Stale · last update " + timeAgo(m.lastSuccess)
		}
		parts = append(parts, "·", stale)
	} else if !m.lastSuccess.IsZero() {
		parts = append(parts, "·", "updated "+timeAgo(m.lastSuccess)+" ago")
	}
	line := strings.Join(parts, " ")
	return m.palette.style(m.palette.text, true).Render(ansi.TruncateWc(sanitizeLine(line), m.width, "…"))
}

func (m *Model) tabs() string {
	if m.workspaceID == "" || m.route.Page == "project" {
		return m.palette.style(m.palette.info, true).Render("Workspaces") + "  / filter  f status"
	}
	labels := []struct{ key, page, name string }{
		{"1", "dashboard", "Work"}, {"2", "tasks", "Tasks"}, {"3", "worktrees", "Worktrees"}, {"4", "results", "Results"}, {"5", "more", "More"},
	}
	var out []string
	compact := m.width < 60
	for i, label := range labels {
		name := label.name
		if compact {
			name = label.key + " " + []string{"Work", "Tasks", "Trees", "Out", "More"}[i]
		} else {
			name = label.key + " " + name
		}
		if m.route.Page == label.page {
			out = append(out, m.palette.style(m.palette.focus, true).Render("["+name+"]"))
		} else {
			out = append(out, name)
		}
	}
	if compact {
		return strings.Join(out, " ")
	}
	return strings.Join(out, "   ")
}

func (m *Model) bodyView(mode layoutMode) []string {
	if m.form != nil {
		return []string{m.form.View()}
	}
	if m.initialError != "" {
		return []string{"Workspace scope could not be resolved:", sanitizeLine(m.initialError), "Use an explicit --workspace ID or repair the workspace document."}
	}
	if !m.projectFound {
		return []string{"No project is configured here.", "Run `workspace project init` in a Git project, then reopen the TUI."}
	}
	if m.route.Page == "project" {
		return m.projectView(mode)
	}
	if m.route.Page == "dashboard" {
		return m.dashboardView(mode)
	}
	if m.isCollectionPage() {
		return m.collectionView(mode)
	}
	if m.route.Page == "actions" {
		return []string{"Actions", "Choose a workspace item, then press a to open its available actions."}
	}
	if m.route.Page == "error" {
		return []string{"Could not load this workspace.", sanitizeLine(m.initialError), "Press w to choose a workspace or " + m.exitHint() + "."}
	}
	return []string{m.viewport.View()}
}

func (m *Model) projectView(mode layoutMode) []string {
	if m.projectPending && len(m.project.Workspaces) == 0 {
		return []string{"Loading workspaces…"}
	}
	if m.loadError != "" && len(m.project.Workspaces) == 0 {
		return []string{"Project overview is unavailable.", sanitizeLine(m.loadError), "Press r to retry."}
	}
	items := m.filteredItems()
	filterLine := ""
	if m.route.StatusFilter != "" {
		filterLine = "Status: " + sanitizeLine(m.route.StatusFilter)
	}
	if len(items) == 0 {
		if m.route.Query != "" {
			return appendFilterLine(filterLine, []string{"No workspaces match “" + sanitizeLine(m.route.Query) + "”.", "Esc clears the filter."})
		}
		return appendFilterLine(filterLine, []string{"No workspaces yet.", "Create one with `workspace create`; the TUI never creates it automatically."})
	}
	if mode == layoutWide {
		leftWidth := max(32, m.width*2/5)
		left := m.renderItems(items, leftWidth, m.height-5)
		right := []string{"Full ID: " + m.route.SelectedID}
		for _, workspace := range m.project.Workspaces {
			if workspace.ID == m.route.SelectedID {
				right = append(right, "Path: "+workspace.Directory, "Input: "+workspace.InputSource, "Created: "+workspace.CreatedAt.Format("2006-01-02 15:04"), "State: "+statusBadge(workspace.Status))
				if workspace.Error != "" {
					right = append(right, "Error: "+workspace.Error)
				}
				break
			}
		}
		return appendFilterLine(filterLine, []string{lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(left, "\n"), "  ", strings.Join(truncateLines(right, m.width-leftWidth-4), "\n"))})
	}
	return appendFilterLine(filterLine, m.renderItems(items, m.width-2, m.height-5))
}

func appendFilterLine(filter string, lines []string) []string {
	if filter == "" {
		return lines
	}
	return append([]string{filter}, lines...)
}

func (m *Model) dashboardView(mode layoutMode) []string {
	return m.workDashboard()
}

func (m *Model) collectionView(mode layoutMode) []string {
	items := m.filteredItems()
	if m.projectPending || m.snapshotPending {
		return append([]string{"Refreshing…"}, m.renderItems(items, m.width-2, m.height-7)...)
	}
	count := fmt.Sprintf("%s · %d / %d · sort: %s", m.collectionTitle(), len(items), len(m.allItems()), firstNonempty(m.route.Sort, "priority"))
	if m.route.Query != "" {
		count += " · filter: " + sanitizeLine(m.route.Query)
	}
	if m.route.Page == "results" {
		count += " · " + firstNonempty(m.route.Tab, "artifacts") + " · Tab changes type"
	}
	if m.isTask(m.route.ParentID) && (m.route.Page == "sessions" || m.route.Page == "results") {
		attemptView := "current attempt"
		if m.route.StatusFilter == "history" {
			attemptView = "history"
		}
		count += " · " + attemptView + " · f toggles"
	} else if m.route.StatusFilter != "" {
		count += " · status: " + sanitizeLine(m.route.StatusFilter)
	}
	if len(items) == 0 {
		if m.route.Query != "" {
			return []string{count, "No results match this filter.", "Esc clears the filter."}
		}
		return []string{count, emptyMessage(m.route.Page)}
	}
	listHeight := max(1, m.height-7)
	if mode == layoutWide {
		leftWidth := max(34, m.width*2/5)
		left := strings.Join(m.renderItems(items, leftWidth, listHeight), "\n")
		detail := m.itemSummary(items)
		right := strings.Join(truncateLines(detail, m.width-leftWidth-4), "\n")
		return []string{count, lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)}
	}
	return append([]string{count}, m.renderItems(items, m.width-2, listHeight)...)
}

func (m *Model) renderItems(items []collectionItem, width, height int) []string {
	if width < 1 || height < 1 {
		return nil
	}
	selectedID := m.route.SelectedID
	if selectedID == "" && len(items) > 0 {
		selectedID = items[0].ID
	}
	rowHeight := 2
	if height < 4 {
		rowHeight = 1
	}
	capacity := max(1, height/rowHeight)
	selected := 0
	for i, item := range items {
		if item.ID == selectedID {
			selected = i
			break
		}
	}
	start := max(0, selected-capacity+1)
	end := min(len(items), start+capacity)
	rows := make([]string, 0, height)
	for i := start; i < end; i++ {
		item := items[i]
		marker := "  "
		if item.ID == selectedID {
			marker = "› "
		}
		badge := m.stateLabel(item.State)
		if item.State == "" {
			badge = ""
		}
		title := rowText(item.Title, width-ansi.StringWidth(badge)-4)
		if item.ID == selectedID {
			title = m.palette.style(m.palette.focus, true).Render(title)
		}
		rows = append(rows, marker+badge+"  "+title)
		if rowHeight == 2 {
			rows = append(rows, m.palette.style(m.palette.muted, false).Render("  "+rowText(item.Subtitle, width-2)))
		}
	}
	return rows
}

func (m *Model) itemSummary(items []collectionItem) []string {
	for _, item := range items {
		if item.ID == m.route.SelectedID {
			lines := []string{item.Title, statusBadge(item.State), "t terminal · Enter details", "", item.Subtitle, "", "ID: " + item.ID}
			if item.Kind == "session" {
				if session, ok := findSession(m.snapshot.Status.Sessions, item.ID); ok {
					if run, live := m.currentRun(session); live {
						lines = append(lines, "Run: "+run.ID, "Model: "+run.Route.Model, "Pane: "+firstNonempty(run.PaneID, "launching"))
					} else {
						lines = append(lines, "No live run · t asks to resume")
					}
					if task, ok := m.task(session.TaskID); ok {
						lines = append(lines, "Task: "+task.Title, statusBadge(task.State))
					}
					return truncateLines(lines, m.width*3/5)
				}
			}
			lines = append(lines, m.quickDetails(item.Kind, item.ID)...)
			return truncateLines(lines, m.width*3/5)
		}
	}
	return []string{"Select a row to see its details."}
}

func (m *Model) panel(title string, lines []string, width, height int, focused bool) string {
	if height < 3 {
		height = 3
	}
	inside := max(1, width-4)
	body := make([]string, 0, height-2)
	body = append(body, m.palette.style(m.palette.info, true).Render(ansi.TruncateWc(sanitizeLine(title), inside, "…")))
	for _, line := range lines {
		if len(body) >= height-2 {
			break
		}
		body = append(body, ansi.TruncateWc(sanitizeLine(line), inside, "…"))
	}
	return m.palette.borderStyle(focused).Width(max(1, width-2)).Height(max(1, height-2)).Render(strings.Join(body, "\n"))
}

func (m *Model) footer() string {
	if m.form != nil {
		return "Huh form   Enter confirm   Esc cancel"
	}
	if m.filtering {
		return m.filterInput.View() + "  Enter apply · Esc cancel"
	}
	if m.actionFailure && m.lastAction != nil {
		return "y retry the same operation key · r refresh · " + m.exitHint()
	}
	if m.route.Query != "" || m.route.StatusFilter != "" {
		return "Esc clear filter · ↑/↓ select · Enter open · / edit"
	}
	if m.route.Page == "dashboard" {
		if m.width < 60 {
			return "t terminal  Tab view  / filter  ? help"
		}
		return "t terminal  Tab view  / filter  s sort  a actions  ? help  " + m.exitHint()
	}
	if m.route.Page == "project" {
		if m.managed {
			return "↑/↓ select   Enter open   / filter   f status   r refresh   ? help   q hide"
		}
		return "↑/↓ select   Enter open   / filter   f status   r refresh   ? help   q quit"
	}
	if m.isCollectionPage() {
		if m.managed {
			return "t terminal  Enter details  / filter  s sort  f state  a actions  q hide"
		}
		return "t terminal  Enter details  / filter  s sort  f state  a actions  Esc back"
	}
	if m.managed {
		return "t terminal  a actions  g jump  Esc back  ? help  q hide"
	}
	return "t terminal  a actions  g jump  Esc back  ? help  q quit"
}

func (m *Model) helpView() string {
	quit := "quit this manual TUI"
	if m.managed {
		quit = "hide this managed panel"
	}
	return strings.Join([]string{
		"Keyboard help",
		"  ↑/↓ or j/k  move through rows or scroll the active detail",
		"  Enter       open selection; it never starts or stops a process",
		"  /           filter by case-insensitive name or ID",
		"  f           cycle status/history filters on supported collections",
		"  Tab / Shift+Tab  cycle focused dashboard panels or result types",
		"  1–5         Work, Tasks, Worktrees, Results, More",
		"  t           open terminal / confirm starting or resuming a run",
		"  l           current work",
		"  s           sort by priority, name, recent execution",
		"  w / o       choose workspace / inspect orchestrator",
		"  v           view all attention items from the dashboard",
		"  g           jump to a verified live tmux target",
		"  r           refresh durable and runtime observations",
		"  Esc         clear filter or return to the previous page",
		"  q           " + quit,
		"Press ? to return.",
	}, "\n")
}

func (m *Model) exitHint() string {
	if m.managed {
		return "q hide"
	}
	return "q quit"
}

func (m *Model) collectionTitle() string {
	return strings.ReplaceAll(m.route.Page, "_", " ")
}

func fitFrame(value string, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.TruncateWc(lines[i], width, "")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func truncateLines(lines []string, width int) []string {
	if width < 1 {
		return nil
	}
	out := make([]string, len(lines))
	for i := range lines {
		out[i] = ansi.TruncateWc(sanitizeLine(lines[i]), width, "…")
	}
	return out
}

func shortPath(path string, width int) string {
	if ansi.StringWidth(path) <= width {
		return path
	}
	if width <= 4 {
		return ansi.TruncateWc(path, width, "")
	}
	return ansi.TruncateLeftWc(path, width, "…")
}

func timeAgo(at time.Time) string {
	if at.IsZero() {
		return "unknown"
	}
	d := time.Since(at)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func firstNonempty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func emptyMessage(page string) string {
	switch page {
	case "tasks":
		return "No tasks are recorded in this workspace."
	case "worktrees":
		return "No worktrees are recorded in this workspace."
	case "results":
		return "No records in this results view."
	case "sessions":
		return "No logical sessions are recorded."
	case "runs":
		return "No runs are recorded for this session."
	case "agents":
		return "No agent definitions are recorded."
	case "services":
		return "No background services are recorded."
	case "decisions":
		return "No decisions are recorded."
	case "change_requests":
		return "No change requests are recorded."
	default:
		return "No records are available."
	}
}
