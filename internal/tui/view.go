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
	return fitFrame(strings.Join(lines, "\n"), m.width, m.height)
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
		parts = append(parts, "Project", project, "›", name, shortID(m.workspaceID))
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
	if m.notice != "" {
		parts = append(parts, "·", m.notice)
	}
	line := strings.Join(parts, " ")
	return m.palette.style(m.palette.text, true).Render(ansi.TruncateWc(sanitizeLine(line), m.width, "…"))
}

func (m *Model) tabs() string {
	if m.workspaceID == "" || m.route.Page == "project" {
		return m.palette.style(m.palette.info, true).Render("Workspaces") + "  / filter  f status"
	}
	labels := []struct{ key, page, name string }{
		{"1", "dashboard", "Overview"}, {"2", "tasks", "Tasks"}, {"3", "worktrees", "Worktrees"}, {"4", "results", "Results"}, {"5", "more", "More"},
	}
	var out []string
	compact := m.width < 80
	for _, label := range labels {
		name := label.name
		if compact {
			name = label.key + " " + name
		} else {
			name = label.key + " " + name
		}
		if m.route.Page == label.page {
			out = append(out, m.palette.style(m.palette.focus, true).Render("["+name+"]"))
		} else {
			out = append(out, name)
		}
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
	if m.snapshotPending && m.snapshot.ObservedAt.IsZero() {
		return []string{"Loading workspace…"}
	}
	if mode == layoutWide {
		panels := m.dashboardPanels()
		colWidth := max(30, (m.width-3)/2)
		rowHeight := max(5, (m.height-7)/2)
		var rendered []string
		for _, index := range [][]int{{0, 1}, {2, 3}} {
			left := m.panel(panels[index[0]].Title, panels[index[0]].Lines, colWidth, rowHeight, index[0] == m.focusedPanel)
			right := m.panel(panels[index[1]].Title, panels[index[1]].Lines, colWidth, rowHeight, index[1] == m.focusedPanel)
			rendered = append(rendered, lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
		}
		return rendered
	}
	if mode == layoutShort {
		panels := m.dashboardPanels()
		if m.focusedPanel >= 2 {
			focused := panels[m.focusedPanel]
			return []string{m.panel(focused.Title, focused.Lines, m.width-2, max(4, m.height-5), true)}
		}
		colWidth := (m.width - 2) / 2
		first := lipgloss.JoinHorizontal(lipgloss.Top, m.panel("Overview", panels[0].Lines, colWidth, 5, m.focusedPanel == 0), " ", m.panel("Orchestrator", panels[1].Lines, colWidth, 5, m.focusedPanel == 1))
		second := m.panel(panels[2].Title, panels[2].Lines, m.width-2, max(4, m.height-12), false)
		return []string{first, second}
	}
	return []string{m.viewport.View()}
}

func (m *Model) collectionView(mode layoutMode) []string {
	items := m.filteredItems()
	if m.projectPending || m.snapshotPending {
		return append([]string{"Refreshing…"}, m.renderItems(items, m.width-2, m.height-7)...)
	}
	count := fmt.Sprintf("%d / %d", len(items), len(m.allItems()))
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
	start := 0
	selected := -1
	for i, item := range items {
		if item.ID == selectedID {
			selected = i
			break
		}
	}
	if selected >= height {
		start = selected - height + 1
	}
	end := min(len(items), start+height)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		item := items[i]
		marker := "  "
		if item.ID == selectedID {
			marker = "> "
		}
		state := statusBadge(item.State)
		line := marker + sanitizeLine(item.Title) + "  " + state + "  " + sanitizeLine(item.Subtitle)
		line = ansi.TruncateWc(line, width, "…")
		if item.ID == m.route.SelectedID {
			line = m.palette.style(m.palette.focus, true).Render(line)
		}
		rows = append(rows, line)
	}
	return rows
}

func (m *Model) itemSummary(items []collectionItem) []string {
	for _, item := range items {
		if item.ID == m.route.SelectedID {
			lines := []string{item.Title, statusBadge(item.State), "ID: " + item.ID, "", item.Subtitle}
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
	if m.actionFailure && m.lastAction != nil {
		return "y retry the same operation key · r refresh · " + m.exitHint()
	}
	if m.filtering {
		return m.filterInput.View() + "  Enter apply · Esc cancel"
	}
	if m.route.Query != "" {
		return "Esc clear filter · ↑/↓ select · Enter open · / edit"
	}
	if m.route.Page == "dashboard" {
		return "Tab/Shift+Tab panel   1–5 pages   a actions   g jump   w workspaces   o orchestrator   ? help   " + m.exitHint()
	}
	if m.route.Page == "project" {
		if m.managed {
			return "↑/↓ select   Enter open   / filter   f status   r refresh   ? help   q hide"
		}
		return "↑/↓ select   Enter open   / filter   f status   r refresh   ? help   q quit"
	}
	if m.isCollectionPage() {
		if m.managed {
			return "↑/↓ select   Enter open   / filter   f state/history   1–5 pages   g jump   r refresh   Esc back   q hide"
		}
		return "↑/↓ select   Enter open   / filter   f state/history   1–5 pages   g jump   r refresh   Esc back"
	}
	if m.managed {
		return "1–5 pages   a actions   g jump   w workspaces   o orchestrator   ? help   q hide"
	}
	return "1–5 pages   a actions   g jump   w workspaces   o orchestrator   ? help   q quit"
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
		"  1–5         Overview, Tasks, Worktrees, Results, More",
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
