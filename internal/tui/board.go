package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// minBoardColumnWidth keeps a named column usable: a status badge plus a short
// title. Below this a multi-column board stops being readable.
const minBoardColumnWidth = 18

// taskBoardStateOrder is the canonical left-to-right column order. It is not
// derived from workPriority: the board must be stable, and review states need
// their own columns instead of collapsing into one priority band.
var taskBoardStateOrder = []string{
	"pending",
	"running",
	"blocked",
	"needs_changes",
	"awaiting_review",
	"accepted",
}

// boardColumn is one status lane. Items are already filtered and sorted by the
// active collection view, so a column's order matches the list view.
type boardColumn struct {
	State string
	Title string
	Items []collectionItem
}

// isBoardPage reports whether the Tasks page is currently presented as a board.
func (m *Model) isBoardPage() bool {
	return m.route.Page == "tasks" && m.route.View == "board"
}

// boardColumns derives the stable board shape from the non-deleted, parent
// scoped Tasks-page set, then places the filtered cards into it. Columns are
// derived before text/status filtering so typing a query or cycling `f` removes
// cards without making the board jumpy. Unknown states get a trailing column in
// first-appearance order and render as neutral raw text.
func (m *Model) boardColumns() []boardColumn {
	present := make([]string, 0)
	seen := make(map[string]bool)
	for _, item := range m.allItems() {
		if item.Kind != "task" || seen[item.State] {
			continue
		}
		seen[item.State] = true
		present = append(present, item.State)
	}
	ordered := make([]string, 0, len(present))
	for _, state := range taskBoardStateOrder {
		if seen[state] {
			ordered = append(ordered, state)
		}
	}
	for _, state := range present {
		if !isKnownBoardState(state) {
			ordered = append(ordered, state)
		}
	}
	byState := make(map[string][]collectionItem)
	for _, item := range m.filteredItems() {
		if item.Kind != "task" {
			continue
		}
		byState[item.State] = append(byState[item.State], item)
	}
	columns := make([]boardColumn, 0, len(ordered))
	for _, state := range ordered {
		columns = append(columns, boardColumn{State: state, Title: statusBadge(state), Items: byState[state]})
	}
	return columns
}

func isKnownBoardState(state string) bool {
	for _, known := range taskBoardStateOrder {
		if known == state {
			return true
		}
	}
	return false
}

// boardHasSelection reports whether id belongs to any rendered column.
func (m *Model) boardHasSelection(columns []boardColumn, id string) bool {
	if id == "" {
		return false
	}
	for _, column := range columns {
		for _, item := range column.Items {
			if item.ID == id {
				return true
			}
		}
	}
	return false
}

// activeBoardColumn resolves the focused lane from route.SelectedID. When the
// selection is unknown or filtered away it falls back to the first non-empty
// column, matching the plan's default.
func (m *Model) activeBoardColumn(columns []boardColumn) int {
	for index, column := range columns {
		for _, item := range column.Items {
			if item.ID == m.route.SelectedID {
				return index
			}
		}
	}
	for index, column := range columns {
		if len(column.Items) > 0 {
			return index
		}
	}
	return 0
}

// boardRowIndex returns the selected card's row within one column, or -1.
func (m *Model) boardRowIndex(column boardColumn) int {
	for index, item := range column.Items {
		if item.ID == m.route.SelectedID {
			return index
		}
	}
	return -1
}

// boardMoveCard moves within the active column and clamps at the ends; it does
// not wrap so the board stays predictable at the first and last card.
func (m *Model) boardMoveCard(delta int) {
	columns := m.boardColumns()
	if len(columns) == 0 {
		return
	}
	current := m.activeBoardColumn(columns)
	items := columns[current].Items
	if len(items) == 0 {
		return
	}
	index := 0
	if row := m.boardRowIndex(columns[current]); row >= 0 {
		index = row
	}
	index = clamp(index+delta, 0, len(items)-1)
	m.route.SelectedID = items[index].ID
}

// boardMoveColumn crosses to the adjacent non-empty column, wrapping around the
// ends and keeping the same row index when the target has it, otherwise the
// last card.
func (m *Model) boardMoveColumn(delta int) {
	columns := m.boardColumns()
	count := len(columns)
	if count == 0 {
		return
	}
	current := m.activeBoardColumn(columns)
	row := m.boardRowIndex(columns[current])
	for step := 1; step <= count; step++ {
		next := ((current+delta*step)%count + count) % count
		items := columns[next].Items
		if len(items) == 0 {
			continue
		}
		if row >= 0 && row < len(items) {
			m.route.SelectedID = items[row].ID
		} else {
			m.route.SelectedID = items[len(items)-1].ID
		}
		return
	}
}

// boardJump selects the first card of the first non-empty column, or the last
// card of the last non-empty column.
func (m *Model) boardJump(first bool) {
	columns := m.boardColumns()
	if first {
		for _, column := range columns {
			if len(column.Items) > 0 {
				m.route.SelectedID = column.Items[0].ID
				return
			}
		}
		return
	}
	for index := len(columns) - 1; index >= 0; index-- {
		if items := columns[index].Items; len(items) > 0 {
			m.route.SelectedID = items[len(items)-1].ID
			return
		}
	}
}

// boardWindowFor returns the contiguous column window that keeps the active
// column visible, its exclusive end, and the panel width that fits the window in
// the available terminal width. It is shared by the renderer and the count line
// so the position indicator never disagrees with what is drawn.
func boardWindowFor(columns, active, width int) (int, int, int) {
	if columns <= 0 {
		return 0, 0, 0
	}
	const gap = 2
	fit := max(1, (width+gap)/(minBoardColumnWidth+gap))
	if fit > columns {
		fit = columns
	}
	start := clamp(active-fit/2, 0, max(0, columns-fit))
	end := start + fit
	columnWidth := (width - gap*(fit-1)) / fit
	if columnWidth < minBoardColumnWidth {
		columnWidth = minBoardColumnWidth
	}
	return start, end, columnWidth
}

// boardView renders the Tasks page as a kanban board. It is selected from
// collectionView, so all selection-driven commands keep working against the
// same route.SelectedID.
func (m *Model) boardView(mode layoutMode) []string {
	items := m.filteredItems()
	pending := m.projectPending || m.snapshotPending
	if pending && len(m.allItems()) == 0 {
		return []string{"Loading workspace data…"}
	}
	columns := m.boardColumns()
	active := m.activeBoardColumn(columns)
	// Keep route.SelectedID authoritative for every command: when it is unset or
	// filtered away, pin it to the active column's first card so selectedItem()
	// and the rendered marker agree.
	if len(items) > 0 && !m.boardHasSelection(columns, m.route.SelectedID) {
		if active < len(columns) && len(columns[active].Items) > 0 {
			m.route.SelectedID = columns[active].Items[0].ID
		}
	}
	count := fmt.Sprintf("%s · %d / %d · sort: %s", m.collectionTitle(), len(items), len(m.allItems()), firstNonempty(m.route.Sort, "priority"))
	if pending {
		count += " · refreshing…"
	}
	if m.route.Query != "" {
		count += " · filter: " + sanitizeLine(m.route.Query)
	}
	if m.route.StatusFilter != "" {
		count += " · status: " + sanitizeLine(m.route.StatusFilter)
	}
	if mode == layoutWide {
		if start, end, _ := boardWindowFor(len(columns), active, m.width); start > 0 || end < len(columns) {
			count += fmt.Sprintf(" · columns %d–%d/%d", start+1, end, len(columns))
		}
	}
	if len(items) == 0 || len(columns) == 0 {
		if m.route.Query != "" {
			return []string{count, "No matches", "Nothing here matches “" + sanitizeLine(m.route.Query) + "”.", "Esc clears the filter."}
		}
		return append([]string{count}, emptyState("tasks")...)
	}
	if mode == layoutWide {
		available := max(3, m.contentHeight()-1)
		return append([]string{count}, m.boardWide(columns, active, m.width, available)...)
	}
	available := max(1, m.contentHeight()-2)
	return append([]string{count, m.boardPager(columns, active)}, m.boardCards(columns[active].Items, max(1, m.width-2), available, m.route.SelectedID)...)
}

// boardWide renders a contiguous window of named column panels side by side. No
// Preview panel is used: the columns need the full content width.
func (m *Model) boardWide(columns []boardColumn, active, width, height int) []string {
	start, end, columnWidth := boardWindowFor(len(columns), active, width)
	parts := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		if index > start {
			parts = append(parts, "  ")
		}
		parts = append(parts, m.boardColumnBlock(columns[index], columnWidth, height, index == active))
	}
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, parts...), "\n")
}

// boardColumnBlock renders one column: a status badge with its visible card
// count as the panel title, then the cards. The active column gets the focus
// border; the badge and count keep the meaning visible without color.
func (m *Model) boardColumnBlock(column boardColumn, width, height int, focused bool) string {
	title := fmt.Sprintf("%s (%d)", column.Title, len(column.Items))
	capacity := max(1, height-3)
	lines := m.boardCards(column.Items, max(1, width-2), capacity, m.route.SelectedID)
	return m.panelBlock(title, lines, width, height, focused)
}

// boardPager is the compact single-column header: the state, its count, and the
// column position.
func (m *Model) boardPager(columns []boardColumn, active int) string {
	if active < 0 || active >= len(columns) {
		return ""
	}
	column := columns[active]
	text := fmt.Sprintf("‹ %s (%d) ›  %d/%d", column.Title, len(column.Items), active+1, len(columns))
	return m.palette.headingStyle().Render(ansi.TruncateWc(text, m.width, "…"))
}

// boardCards renders the cards of one column, scrolled so the selected card
// stays visible. It is the board analogue of renderItems and drops the
// per-card status badge because the column header already names the state.
func (m *Model) boardCards(items []collectionItem, width, height int, selectedID string) []string {
	if width < 1 || height < 1 || len(items) == 0 {
		return nil
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
	for index, item := range items {
		if item.ID == selectedID {
			selected = index
			break
		}
	}
	start := max(0, selected-capacity+1)
	end := min(len(items), start+capacity)
	rows := make([]string, 0, height)
	for index := start; index < end; index++ {
		rows = append(rows, m.boardCard(items[index], width, items[index].ID == selectedID, rowHeight)...)
		if rowHeight > 1 && index+1 < end {
			rows = append(rows, m.rowSeparator(width))
		}
	}
	return rows
}

// boardCard renders one compact two-line card: the selection marker and title,
// then the existing task subtitle. The marker survives no-color mode.
func (m *Model) boardCard(item collectionItem, width int, selected bool, rowHeight int) []string {
	marker := "  "
	if selected {
		marker = "› "
	}
	head := ansi.TruncateWc(marker+rowText(item.Title, max(1, width-2)), width, "…")
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
