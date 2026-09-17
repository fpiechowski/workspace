package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// View composes the shell as header, primary navigation, optional breadcrumb,
// content, a status/notice row, and a contextual key legend. The status row and
// legend are always reserved so they stay visible at every supported size.
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
	top := []string{m.header(), m.tabs()}
	if crumb := m.breadcrumb(); crumb != "" {
		top = append(top, crumb)
	}
	contentRows := max(1, m.height-len(top)-2)
	var content []string
	if m.showHelp {
		m.helpViewport.Width = max(1, m.width)
		m.helpViewport.Height = contentRows
		m.helpViewport.SetContent(m.helpView())
		content = strings.Split(m.helpViewport.View(), "\n")
	} else {
		content = m.bodyView(mode)
	}
	content = clipLines(content, contentRows, m.width)
	rows := append(top, content...)
	for len(rows) < m.height-2 {
		rows = append(rows, "")
	}
	rows = append(rows, m.statusRow(), ansi.TruncateWc(m.footer(), m.width, ""))
	if len(rows) > m.height {
		rows = rows[:m.height]
	}
	return fitFrame(strings.Join(rows, "\n"), m.width, m.height)
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
	} else if m.actionFailure && m.loadError != "" {
		parts = append(parts, "·", "[!] action failed")
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
	return m.palette.titleStyle().Render(ansi.TruncateWc(sanitizeLine(line), m.width, "…"))
}

func (m *Model) tabs() string {
	if m.workspaceID == "" || m.route.Page == "project" {
		return m.palette.headingStyle().Render("Workspaces") + "  / filter  f status"
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
			out = append(out, m.palette.headingStyle().Render("["+name+"]"))
		} else {
			out = append(out, name)
		}
	}
	if compact {
		return strings.Join(out, " ")
	}
	return strings.Join(out, "   ")
}

// breadcrumb gives secondary and detail routes an explicit location so the
// primary tabs alone do not have to carry the whole hierarchy.
func (m *Model) breadcrumb() string {
	if m.showHelp || m.workspaceID == "" {
		return ""
	}
	switch m.route.Page {
	case "project", "dashboard", "error":
		return ""
	}
	if m.isDetailPage() {
		line := detailSection(m.route.Page)
		if title := m.routeTitle(); title != "" {
			line += " / " + title
		}
		return m.palette.metaStyle().Render(ansi.TruncateWc(sanitizeLine(line), m.width, "…"))
	}
	if m.route.Page == "results" {
		prefix := "Results"
		if m.route.ParentID != "" {
			if parent := m.parentTitle(m.route.ParentID); parent != "" {
				prefix = parent + " / Results"
			}
		}
		return m.palette.metaStyle().Render(ansi.TruncateWc(sanitizeLine(prefix+" · "+m.resultsTabLine()), m.width, "…"))
	}
	if m.route.ParentID != "" {
		if parent := m.parentTitle(m.route.ParentID); parent != "" {
			return m.palette.metaStyle().Render(ansi.TruncateWc(sanitizeLine(parent+" / "+m.collectionTitle()), m.width, "…"))
		}
	}
	return ""
}

var resultTabs = []struct{ ID, Name string }{
	{"artifacts", "Artifacts"},
	{"handoffs", "Handoffs"},
	{"checks", "Checks"},
}

// resultsTabLine names the active Results type so the secondary navigation is
// explicit instead of only appearing inside the count line.
func (m *Model) resultsTabLine() string {
	active := firstNonempty(m.route.Tab, "artifacts")
	parts := make([]string, 0, len(resultTabs))
	for _, tab := range resultTabs {
		name := tab.Name
		if tab.ID == active {
			name = "[" + name + "]"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, "  ")
}

var detailSections = map[string]string{
	"task": "Tasks", "worktree": "Worktrees", "session": "Sessions", "run": "Runs",
	"agent": "Agents", "service": "Services", "artifact": "Artifacts", "handoff": "Handoffs",
	"check": "Checks", "decision": "Decisions", "change_request": "Change requests",
	"preview": "Preview", "orchestrator": "Orchestrator", "runtime": "Runtime",
}

func detailSection(page string) string {
	if section, ok := detailSections[page]; ok {
		return section
	}
	return "Workspace"
}

func (m *Model) routeTitle() string {
	id := m.route.EntityID
	switch m.route.Page {
	case "task":
		if task, ok := m.task(id); ok {
			return firstNonempty(task.Title, id)
		}
	case "worktree":
		if worktree, ok := findWorktree(m.snapshot.Status.Worktrees, id); ok {
			return firstNonempty(worktree.Name, id)
		}
	case "session":
		if session, ok := findSession(m.snapshot.Status.Sessions, id); ok {
			return firstNonempty(session.AgentSnapshot.Name, id)
		}
	case "agent":
		if agent, ok := findAgent(m.snapshot.Status.Agents, id); ok {
			return firstNonempty(agent.Name, id)
		}
	case "service":
		if service, ok := findService(m.snapshot.Services, id); ok {
			return firstNonempty(service.Name, id)
		}
	case "artifact":
		if artifact, ok := findArtifact(m.snapshot.Status.Workspace.Artifacts, id); ok {
			return firstNonempty(artifact.Name, id)
		}
	case "handoff":
		if handoff, ok := findHandoff(m.snapshot.Handoffs, id); ok {
			return firstNonempty(handoff.Outcome, id)
		}
	case "check":
		if check, ok := findCheck(m.snapshot.Checks, id); ok {
			return firstNonempty(strings.Join(check.Argv, " "), id)
		}
	case "decision":
		if decision, ok := findDecision(m.snapshot.Status.Workspace.Decisions, m.snapshot.Status.Workspace.PendingDecision, id); ok {
			return firstNonempty(decision.Question, id)
		}
	case "change_request":
		if request, ok := findChangeRequest(m.snapshot.Status.Workspace.ChangeRequests, id); ok {
			return firstNonempty(request.Title, id)
		}
	case "preview":
		return firstNonempty(m.preview.Name, id)
	}
	return id
}

func (m *Model) parentTitle(id string) string {
	if task, ok := m.task(id); ok {
		return firstNonempty(task.Title, id)
	}
	if session, ok := findSession(m.snapshot.Status.Sessions, id); ok {
		return firstNonempty(session.AgentSnapshot.Name, id)
	}
	if agent, ok := findAgent(m.snapshot.Status.Agents, id); ok {
		return firstNonempty(agent.Name, id)
	}
	return id
}

// statusRow is the status/notice region. It surfaces an explicit notice or the
// current pending state, and stays reserved even when idle.
func (m *Model) statusRow() string {
	if m.showHelp {
		return m.helpPosition()
	}
	text := sanitizeLine(m.notice)
	switch {
	case text == "" && m.actionPending:
		text = "Working… keep this panel open."
	case text == "" && (m.snapshotPending || m.projectPending):
		text = "Refreshing…"
	case text == "" && m.isDetailPage():
		text = m.scrollPosition()
	}
	if text == "" {
		return ""
	}
	style := m.palette.noticeStyle()
	if !m.palette.noColor {
		style = style.Width(m.width)
	}
	return style.Render(ansi.TruncateWc(text, m.width, "…"))
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
			return appendFilterLine(filterLine, []string{"No matching workspaces", "Nothing matches “" + sanitizeLine(m.route.Query) + "”.", "Esc clears the filter."})
		}
		return appendFilterLine(filterLine, []string{"No workspaces in this project", "No workspace has been created here yet.", "Press a to create a workspace."})
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
	return m.workDashboard(mode)
}

func (m *Model) collectionView(mode layoutMode) []string {
	items := m.filteredItems()
	pending := m.projectPending || m.snapshotPending
	if pending && len(m.allItems()) == 0 {
		return []string{"Loading workspace data…"}
	}
	count := fmt.Sprintf("%s · %d / %d · sort: %s", m.collectionTitle(), len(items), len(m.allItems()), firstNonempty(m.route.Sort, "priority"))
	if pending {
		count += " · refreshing…"
	}
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
			return []string{count, "No matches", "Nothing here matches “" + sanitizeLine(m.route.Query) + "”.", "Esc clears the filter."}
		}
		return append([]string{count}, emptyState(m.route.Page)...)
	}
	available := max(3, m.contentHeight()-1)
	if mode == layoutWide {
		leftWidth, rightWidth := m.panelWidths(2, 34)
		left := m.renderItems(items, max(1, leftWidth-2), available-2)
		right := truncateLines(m.itemSummary(items, max(1, rightWidth-2)), max(1, rightWidth-2))
		return append([]string{count}, m.twoPanels(collectionHeading(m.route.Page), left, "Preview", right, leftWidth, rightWidth, available, true)...)
	}
	return append([]string{count}, m.renderItems(items, m.width-2, available)...)
}

// renderItems keeps the custom two-line domain rows but regularizes the marker,
// badge, title, and subtitle columns. Selection stays keyed by item ID so
// sorting, filtering, and refresh never move it to a different entity.
func (m *Model) renderItems(items []collectionItem, width, height int) []string {
	if width < 1 || height < 1 {
		return nil
	}
	selectedID := m.route.SelectedID
	if selectedID == "" && len(items) > 0 {
		selectedID = items[0].ID
	}
	rowHeight := 3
	if height < 4 {
		rowHeight = 1
	}
	capacity := max(1, (height+1)/rowHeight)
	if rowHeight == 1 {
		capacity = height
	}
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
		rows = append(rows, m.collectionRow(items[i], width, items[i].ID == selectedID, rowHeight)...)
		if rowHeight > 1 && i+1 < end {
			rows = append(rows, m.rowSeparator(width))
		}
	}
	return rows
}

// collectionRow renders one regularized entry: a fixed marker column, a state
// badge, a truncated title, and an optional metadata line. The selected row
// keeps its text marker in no-color mode and adds the selection background when
// colors are available.
func (m *Model) collectionRow(item collectionItem, width int, selected bool, rowHeight int) []string {
	marker := "  "
	if selected {
		marker = "› "
	}
	badge := ""
	badgeWidth := 0
	if item.State != "" {
		badge = m.stateLabel(item.State)
		badgeWidth = ansi.StringWidth(badge)
	}
	title := rowText(item.Title, max(1, width-2-badgeWidth-2))
	head := ansi.TruncateWc(marker+badge+"  "+title, width, "…")
	if selected {
		head = m.palette.selectedStyle(width).Render(head)
	}
	rows := []string{head}
	if rowHeight > 1 {
		subtitle := ansi.TruncateWc("  "+rowText(item.Subtitle, max(1, width-2)), width, "…")
		if selected {
			subtitle = m.palette.selectedStyle(width).Render(subtitle)
		} else {
			subtitle = m.palette.metaStyle().Render(subtitle)
		}
		rows = append(rows, subtitle)
	}
	return rows
}

// rowSeparator is a lower-emphasis divider between entries.
func (m *Model) rowSeparator(width int) string {
	return m.palette.subtleStyle().Render(strings.Repeat("─", max(1, width)))
}

func (m *Model) itemSummary(items []collectionItem, width int) []string {
	selectedID := m.route.SelectedID
	if selectedID == "" && len(items) > 0 {
		selectedID = items[0].ID
	}
	for _, item := range items {
		if item.ID != selectedID {
			continue
		}
		lines := []string{item.Title, statusBadge(item.State), m.previewActions(item.Kind), "", item.Subtitle, "", "ID: " + item.ID}
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
				return truncateLines(lines, max(1, width))
			}
		}
		lines = append(lines, m.quickDetails(item.Kind, item.ID)...)
		return truncateLines(lines, max(1, width))
	}
	return []string{"Select a row to see its details."}
}

// previewActions advertises only the commands the selected kind can honor.
func (m *Model) previewActions(kind string) string {
	actions := make([]string, 0, 3)
	if terminalCapable(kind) {
		actions = append(actions, "t terminal")
	}
	if jumpCapable(kind) {
		actions = append(actions, "g jump")
	}
	actions = append(actions, "Enter details")
	return strings.Join(actions, " · ")
}

// panelWidths splits the terminal width into a list panel and a preview panel
// with a two-column gutter. It keeps both panels usable at the wide breakpoint.
func (m *Model) panelWidths(leftRatio, leftMin int) (int, int) {
	const gap = 2
	left := max(leftMin, m.width*leftRatio/5)
	right := m.width - left - gap
	if right < 20 {
		right = 20
		left = max(10, m.width-gap-right)
	}
	return max(10, left), max(10, right)
}

// twoPanels renders a named list panel and a named preview panel side by side
// with one clear focused edge.
func (m *Model) twoPanels(leftTitle string, leftLines []string, rightTitle string, rightLines []string, leftWidth, rightWidth, height int, leftFocused bool) []string {
	left := m.panelBlock(leftTitle, leftLines, leftWidth, height, leftFocused)
	right := m.panelBlock(rightTitle, rightLines, rightWidth, height, !leftFocused)
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right), "\n")
}

// panelBlock renders a named, bordered panel. Content lines are expected to be
// sanitized already; the title is sanitized here so no external text is styled.
func (m *Model) panelBlock(title string, lines []string, width, height int, focused bool) string {
	if height < 3 {
		height = 3
	}
	if width < 6 {
		width = 6
	}
	inside := max(1, width-2)
	body := make([]string, 0, height-2)
	body = append(body, m.palette.headingStyle().Render(ansi.TruncateWc(sanitizeLine(title), inside, "…")))
	for _, line := range lines {
		if len(body) >= height-2 {
			break
		}
		body = append(body, ansi.TruncateWc(line, inside, "…"))
	}
	return m.palette.panelStyle(focused).Width(inside).Height(max(1, height-2)).Render(strings.Join(body, "\n"))
}

// footer derives the contextual key legend from the enabled bindings. Special
// input modes (forms, filter editing, failures) show their own legends.
func (m *Model) footer() string {
	if m.form != nil {
		return m.legend([]key.Binding{m.keys.FormConfirm, m.keys.Cancel})
	}
	if m.filtering {
		return m.filterInput.View() + "  " + m.legend([]key.Binding{m.keys.Apply, m.keys.Cancel})
	}
	if m.actionFailure && m.lastAction != nil {
		return m.legend([]key.Binding{m.keys.Retry, m.keys.Actions, m.keys.Refresh, m.exitBinding()})
	}
	if m.route.Query != "" || m.route.StatusFilter != "" {
		return m.legend([]key.Binding{m.keys.ClearFilter, m.keys.Up, m.keys.Open, m.keys.Filter})
	}
	return m.legend(m.shortHelp())
}

// helpPosition is the fixed scroll indicator for the viewport-backed full help.
func (m *Model) helpPosition() string {
	total := m.helpViewport.TotalLineCount()
	visible := m.helpViewport.VisibleLineCount()
	top := m.helpViewport.YOffset
	bottom := min(total, top+visible)
	text := fmt.Sprintf("Help · lines %d–%d of %d · ↑/↓ or PgUp/PgDn scroll · ? or Esc to close", top+1, bottom, total)
	style := m.palette.noticeStyle()
	if !m.palette.noColor {
		style = style.Width(m.width)
	}
	return style.Render(ansi.TruncateWc(text, m.width, "…"))
}

// helpView renders the full, categorized help from the same centralized
// bindings as the footer. The viewport in View makes every group reachable at
// the 40x12 minimum.
func (m *Model) helpView() string {
	lines := []string{"Keyboard help"}
	for _, group := range m.keyGroups() {
		keys := make([]key.Binding, 0, len(group.Keys))
		width := 0
		for _, binding := range group.Keys {
			if !binding.Enabled() {
				continue
			}
			keys = append(keys, binding)
			if w := ansi.StringWidth(binding.Help().Key); w > width {
				width = w
			}
		}
		if len(keys) == 0 {
			continue
		}
		lines = append(lines, "", group.Title)
		for _, binding := range keys {
			head := binding.Help()
			pad := strings.Repeat(" ", max(0, width-ansi.StringWidth(head.Key)))
			lines = append(lines, "  "+head.Key+pad+"   "+head.Desc)
		}
	}
	if m.managed {
		lines = append(lines, "", "Press q or Ctrl+C to hide this managed panel; the workspace keeps running.")
	}
	lines = append(lines, "", "Press ? or Esc to return.")
	return strings.Join(lines, "\n")
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

// clipLines splits embedded newlines and limits the content region so the
// reserved status row and key legend always survive the frame.
func clipLines(lines []string, height, width int) []string {
	if height < 1 {
		return nil
	}
	out := make([]string, 0, height)
	for _, line := range lines {
		for _, part := range strings.Split(line, "\n") {
			if len(out) >= height {
				return out
			}
			out = append(out, ansi.TruncateWc(part, width, ""))
		}
	}
	return out
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

// emptyState gives every collection a title, a one-sentence explanation, and
// one valid next action instead of only naming the absence.
func emptyState(page string) []string {
	switch page {
	case "tasks":
		return []string{"No tasks yet", "This workspace has no recorded tasks.", "Press o to start the orchestrator, then t to open its terminal."}
	case "worktrees":
		return []string{"No worktrees yet", "No worktree has been recorded for this workspace.", "Press r to refresh, or 1 to return to Work."}
	case "results":
		return []string{"No results in this view", "No artifacts, handoffs, or checks are recorded here.", "Press Tab to change the results type or r to refresh."}
	case "sessions":
		return []string{"No sessions yet", "This workspace has no logical sessions.", "Press o to open the orchestrator."}
	case "runs":
		return []string{"No runs yet", "This session has no recorded runs.", "Press Esc to return to the session."}
	case "agents":
		return []string{"No agents defined", "No agent definitions are recorded.", "Press r to refresh."}
	case "services":
		return []string{"No services", "No background services are recorded.", "Press r to refresh."}
	case "decisions":
		return []string{"No decisions", "No decisions are recorded.", "Press r to refresh."}
	case "change_requests":
		return []string{"No change requests", "No change requests are recorded.", "Press r to refresh."}
	case "attention":
		return []string{"Nothing needs attention", "No durable issues or failures are recorded.", "Press 1 to return to Work."}
	case "activity":
		return []string{"No recorded activity", "No runs, handoffs, artifacts, or decisions are recorded.", "Press r to refresh."}
	case "documents":
		return []string{"No documents", "No workspace or workflow snapshots are available.", "Press r to refresh."}
	default:
		return []string{"No records", "Nothing is recorded for this view.", "Press r to refresh."}
	}
}

// collectionHeading names a collection panel in wide mode.
func collectionHeading(page string) string {
	switch page {
	case "results":
		return "Results"
	case "change_requests":
		return "Change requests"
	case "more":
		return "More"
	}
	words := strings.Split(strings.ReplaceAll(page, "_", " "), " ")
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}
