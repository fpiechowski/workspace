package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"workspace/internal/core"
)

// boardFixture has two cards in pending and running so column-local movement and
// row-index retention are observable, plus every canonical state and one
// unknown state.
func boardFixture() *Model {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_board", NoColor: true})
	m.snapshot = core.WorkspaceSnapshot{ObservedAt: time.Now(), Status: core.Status{Workspace: core.Workspace{
		ID: "ws_board", Title: "Board demo", Status: "active",
		Tasks: []core.Task{
			{ID: "task_pending", TaskSpec: core.TaskSpec{Title: "Draft plan"}, State: "pending", Attempt: 1},
			{ID: "task_pending_2", TaskSpec: core.TaskSpec{Title: "Draft plan two"}, State: "pending", Attempt: 1},
			{ID: "task_running", TaskSpec: core.TaskSpec{Title: "Implement board"}, State: "running", Attempt: 2},
			{ID: "task_running_2", TaskSpec: core.TaskSpec{Title: "Implement board two"}, State: "running", Attempt: 2},
			{ID: "task_blocked", TaskSpec: core.TaskSpec{Title: "Blocked card"}, State: "blocked", Attempt: 1},
			{ID: "task_changes", TaskSpec: core.TaskSpec{Title: "Needs changes card"}, State: "needs_changes", Attempt: 1},
			{ID: "task_review", TaskSpec: core.TaskSpec{Title: "Review card"}, State: "awaiting_review", Attempt: 1},
			{ID: "task_accepted", TaskSpec: core.TaskSpec{Title: "Accepted card"}, State: "accepted", Attempt: 1},
			{ID: "task_quarantined", TaskSpec: core.TaskSpec{Title: "Quarantined card"}, State: "quarantined", Attempt: 1},
		},
	}}}
	m.validateSelection()
	return m
}

func boardStates(columns []boardColumn) []string {
	states := make([]string, len(columns))
	for i, column := range columns {
		states[i] = column.State
	}
	return states
}

func boardColumnCounts(columns []boardColumn) map[string]int {
	counts := make(map[string]int)
	for _, column := range columns {
		counts[column.State] = len(column.Items)
	}
	return counts
}

// TestBoardColumnsFollowCanonicalOrderAndShape pins the column order, the
// unknown-state fallback, deleted-task exclusion and the ParentID scope.
func TestBoardColumnsFollowCanonicalOrderAndShape(t *testing.T) {
	m := boardFixture()
	m.navigate(route{Page: "tasks"})

	want := []string{"pending", "running", "blocked", "needs_changes", "awaiting_review", "accepted", "quarantined"}
	if got := boardStates(m.boardColumns()); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("column order = %v, want %v", got, want)
	}
	counts := boardColumnCounts(m.boardColumns())
	if counts["pending"] != 2 || counts["running"] != 2 || counts["accepted"] != 1 {
		t.Fatalf("column counts changed: %+v", counts)
	}
	for _, column := range m.boardColumns() {
		if column.State == "quarantined" && column.Title != statusBadge("quarantined") {
			t.Fatalf("unknown state should render as neutral raw text: %q", column.Title)
		}
	}

	now := time.Now()
	m.snapshot.Status.Workspace.Tasks = append(m.snapshot.Status.Workspace.Tasks,
		core.Task{ID: "task_deleted", TaskSpec: core.TaskSpec{Title: "Deleted"}, State: "pending", DeletedAt: &now})
	if counts := boardColumnCounts(m.boardColumns()); counts["pending"] != 2 {
		t.Fatalf("deleted task created or changed a column: %+v", counts)
	}
	for _, column := range m.boardColumns() {
		for _, item := range column.Items {
			if item.ID == "task_deleted" {
				t.Fatal("deleted task appeared as a card")
			}
		}
	}

	m.route = route{Page: "tasks", ParentID: "task_running"}
	columns := m.boardColumns()
	if got := boardStates(columns); len(got) != 1 || got[0] != "running" {
		t.Fatalf("ParentID scope did not restrict the board: %v", got)
	}
	if len(columns[0].Items) != 1 || columns[0].Items[0].ID != "task_running" {
		t.Fatalf("ParentID scope leaked cards: %+v", columns[0].Items)
	}
}

// TestBoardToggleIsTasksOnlyAndDefaultsToList proves `b` switches List <-> Board
// on Tasks and never on another page.
func TestBoardToggleIsTasksOnlyAndDefaultsToList(t *testing.T) {
	m := boardFixture()
	m.width, m.height = 120, 32
	m.navigate(route{Page: "tasks"})
	if m.route.View != "" {
		t.Fatalf("default Tasks view is not list: %q", m.route.View)
	}
	if _, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}); m.route.View != "board" {
		t.Fatalf("b did not switch Tasks to board: %q", m.route.View)
	}
	if view := m.View(); !strings.Contains(view, "running") || !strings.Contains(view, "b list") {
		t.Fatalf("board view is not rendered or advertised:\n%s", view)
	}
	if _, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}); m.route.View != "list" {
		t.Fatalf("b did not switch back to list: %q", m.route.View)
	}

	m.navigate(route{Page: "worktrees"})
	if _, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}); m.route.View != "" || m.route.Page != "worktrees" {
		t.Fatalf("b must be inert outside Tasks: %+v", m.route)
	}
}

// TestBoardViewIsRememberedForTheSession proves the choice survives navigating
// away and back without any cross-run store.
func TestBoardViewIsRememberedForTheSession(t *testing.T) {
	m := boardFixture()
	m.navigate(route{Page: "tasks"})
	_, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m.navigate(route{Page: "worktrees"})
	m.navigate(route{Page: "tasks"})
	if m.route.View != "board" {
		t.Fatalf("route memory lost the board view: %q", m.route.View)
	}
	if m.routeMemory[routeKey{WorkspaceID: "ws_board", Page: "tasks"}].View != "board" {
		t.Fatal("route memory did not record the board view")
	}
}

// TestBoardFramesFitAtSupportedSizes runs the same accepted matrix as the list
// view.
func TestBoardFramesFitAtSupportedSizes(t *testing.T) {
	sizes := [][2]int{{40, 12}, {60, 24}, {80, 18}, {100, 24}, {120, 32}}
	for _, size := range sizes {
		m := boardFixture()
		m.width, m.height = size[0], size[1]
		m.navigate(route{Page: "tasks", View: "board", SelectedID: "task_running"})
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) != size[1] {
			t.Fatalf("board %dx%d rendered %d rows", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if width := ansi.StringWidth(line); width > size[0] {
				t.Fatalf("board %dx%d line %d rendered %d columns: %q", size[0], size[1], i, width, line)
			}
		}
	}
}

// TestBoardRendersColumnsAndCardsReadableWithoutColor proves headers, titles,
// subtitles and the selection marker survive --no-color.
func TestBoardRendersColumnsAndCardsReadableWithoutColor(t *testing.T) {
	m := boardFixture()
	m.width, m.height = 200, 40
	m.navigate(route{Page: "tasks", View: "board", SelectedID: "task_running"})
	view := m.View()
	for _, want := range []string{
		"○ pending (2)", "● running (2)", "! blocked (1)", "! needs_changes (1)",
		"◈ awaiting_review (1)", "✓ accepted (1)", "· quarantined (1)",
		"Implement board", "No session yet", "› ",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("board view missing %q:\n%s", want, view)
		}
	}

	// When the columns do not all fit, the count line shows the window position.
	m.width, m.height = 120, 32
	m.rebuildViewport()
	if wide := m.View(); !strings.Contains(wide, "columns 1–6/7") {
		t.Fatalf("wide board is missing the window indicator:\n%s", wide)
	}

	// Compact shows only the active column with a pager header.
	m.width, m.height = 80, 24
	m.rebuildViewport()
	compact := m.View()
	for _, want := range []string{"running (2)", "›", "2/7"} {
		if !strings.Contains(compact, want) {
			t.Fatalf("compact board missing %q:\n%s", want, compact)
		}
	}
	if strings.Contains(compact, "pending (2)") {
		t.Fatalf("compact board must show only the active column:\n%s", compact)
	}
}

// TestBoardNavigationWithinAndAcrossColumns keeps route.SelectedID authoritative
// while clamping inside a column and wrapping across columns.
func TestBoardNavigationWithinAndAcrossColumns(t *testing.T) {
	m := boardFixture()
	m.width, m.height = 120, 32
	m.navigate(route{Page: "tasks", View: "board", SelectedID: "task_pending"})

	key := func(k tea.KeyType) {
		_, _ = m.updateKey(tea.KeyMsg{Type: k})
		if item, _, items := m.selectedItem(); len(items) == 0 || item.ID != m.route.SelectedID {
			t.Fatalf("selection drifted from route.SelectedID: route=%q selected=%q", m.route.SelectedID, item.ID)
		}
	}

	key(tea.KeyUp)
	if m.route.SelectedID != "task_pending" {
		t.Fatalf("Up did not clamp at the column start: %q", m.route.SelectedID)
	}
	key(tea.KeyDown)
	if m.route.SelectedID != "task_pending_2" {
		t.Fatalf("Down did not move within the column: %q", m.route.SelectedID)
	}
	key(tea.KeyDown)
	if m.route.SelectedID != "task_pending_2" {
		t.Fatalf("Down did not clamp at the column end: %q", m.route.SelectedID)
	}
	key(tea.KeyUp)

	key(tea.KeyRight)
	if m.route.SelectedID != "task_running" {
		t.Fatalf("Right did not cross columns at the same row: %q", m.route.SelectedID)
	}
	key(tea.KeyDown)
	if m.route.SelectedID != "task_running_2" {
		t.Fatalf("Down did not move inside the new column: %q", m.route.SelectedID)
	}
	key(tea.KeyRight)
	if m.route.SelectedID != "task_blocked" {
		t.Fatalf("Right must fall back to the last card when the row is absent: %q", m.route.SelectedID)
	}
	key(tea.KeyLeft)
	if m.route.SelectedID != "task_running" {
		t.Fatalf("Left did not return to the adjacent column: %q", m.route.SelectedID)
	}
	key(tea.KeyLeft)
	if m.route.SelectedID != "task_pending" {
		t.Fatalf("Left did not reach the first column: %q", m.route.SelectedID)
	}
	key(tea.KeyLeft)
	if m.route.SelectedID != "task_quarantined" {
		t.Fatalf("Left did not wrap to the last non-empty column: %q", m.route.SelectedID)
	}

	key(tea.KeyHome)
	if m.route.SelectedID != "task_pending" {
		t.Fatalf("Home did not jump to the first card: %q", m.route.SelectedID)
	}
	key(tea.KeyEnd)
	if m.route.SelectedID != "task_quarantined" {
		t.Fatalf("End did not jump to the last card: %q", m.route.SelectedID)
	}
	key(tea.KeyPgDown)
	if m.route.SelectedID != "task_quarantined" {
		t.Fatalf("PgDn left the active column: %q", m.route.SelectedID)
	}

	if _, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyEnter}); m.route.Page != "task" || m.route.EntityID != "task_quarantined" || cmd != nil {
		t.Fatalf("Enter did not open the board selection: %+v", m.route)
	}
}

// TestBoardEmptyAndNoMatchStatesReuseExistingCopy proves board mode keeps the
// documented empty and filtered-empty messaging.
func TestBoardEmptyAndNoMatchStatesReuseExistingCopy(t *testing.T) {
	empty := New(Config{ProjectFound: true, WorkspaceID: "ws_empty_board", NoColor: true})
	empty.width, empty.height = 120, 32
	empty.navigate(route{Page: "tasks", View: "board"})
	if view := empty.View(); !strings.Contains(view, "No tasks yet") || !strings.Contains(view, "Press o to start the orchestrator") {
		t.Fatalf("board empty state missing the Tasks copy:\n%s", view)
	}

	m := boardFixture()
	m.width, m.height = 120, 32
	m.navigate(route{Page: "tasks", View: "board", Query: "no-such-card"})
	if view := m.View(); !strings.Contains(view, "No matches") || !strings.Contains(view, "Esc clears the filter.") {
		t.Fatalf("board filtered-empty state missing the no-match copy:\n%s", view)
	}
}

func TestBoardRefreshKeepsCardsWithoutRefreshSuffix(t *testing.T) {
	m := boardFixture()
	m.width, m.height = 80, 24
	m.navigate(route{Page: "tasks", View: "board", SelectedID: "task_running"})
	m.snapshotPending = true

	view := m.View()
	if strings.Contains(view, "refreshing…") {
		t.Fatalf("board count still contains a refresh suffix:\n%s", view)
	}
	for _, want := range []string{"Implement board", "running (2)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("board refresh dropped %q:\n%s", want, view)
		}
	}
}

// TestBoardHelpAndFlagsAreScopedToTasks proves the toggle and column movement
// are advertised only where they are valid.
func TestBoardHelpAndFlagsAreScopedToTasks(t *testing.T) {
	m := boardFixture()
	m.width, m.height = 200, 40
	m.navigate(route{Page: "tasks"})
	flags := m.contextFlags()
	if !flags.board || flags.boardColumns {
		t.Fatalf("list-mode flags = %+v, want board toggle only", flags)
	}
	if help := collapseHelp(m.helpView()); !strings.Contains(help, "b board") {
		t.Fatalf("help does not advertise the board toggle:\n%s", help)
	}
	if footer := m.footer(); !strings.Contains(footer, "b board") {
		t.Fatalf("footer does not advertise the board toggle: %q", footer)
	}

	_, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	flags = m.contextFlags()
	if !flags.board || !flags.boardColumns {
		t.Fatalf("board-mode flags = %+v, want toggle and column movement", flags)
	}
	help := collapseHelp(m.helpView())
	for _, want := range []string{"b list", "←/→ column"} {
		if !strings.Contains(help, want) {
			t.Fatalf("board help missing %q:\n%s", want, help)
		}
	}

	m.navigate(route{Page: "worktrees"})
	if flags := m.contextFlags(); flags.board || flags.boardColumns {
		t.Fatalf("board flags leaked to Worktrees: %+v", flags)
	}
	if help := collapseHelp(m.helpView()); strings.Contains(help, "b board") || strings.Contains(help, "←/→ column") {
		t.Fatalf("Worktrees help advertises board keys:\n%s", help)
	}
}

// TestBoardNavigationLeavesListAndSortIntact proves the board does not change
// list navigation and that sorting keeps the selected ID on both views.
func TestBoardNavigationKeepsSortAndListSelection(t *testing.T) {
	m := boardFixture()
	m.navigate(route{Page: "tasks", SelectedID: "task_running"})
	if _, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}); m.route.Sort != "name" || m.route.SelectedID != "task_running" {
		t.Fatalf("sort changed list selection or order: %+v", m.route)
	}
	if _, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRight}); m.route.SelectedID != "task_running" {
		t.Fatalf("Right arrow changed the list selection: %q", m.route.SelectedID)
	}
}
