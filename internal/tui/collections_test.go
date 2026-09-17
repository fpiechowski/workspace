package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"workspace/internal/core"
)

// TestWorkProgressCountsOnlyAcceptedTasks proves a completed run or process
// state never advances the accepted counter.
func TestWorkProgressCountsOnlyAcceptedTasks(t *testing.T) {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_progress", NoColor: true})
	m.snapshot = core.WorkspaceSnapshot{
		ObservedAt: time.Now(),
		Status: core.Status{Workspace: core.Workspace{
			ID: "ws_progress",
			Tasks: []core.Task{
				{ID: "task_accepted", TaskSpec: core.TaskSpec{Title: "Accepted"}, State: "accepted", Attempt: 1},
				{ID: "task_completed", TaskSpec: core.TaskSpec{Title: "Process finished"}, State: "completed", Attempt: 1},
				{ID: "task_running", TaskSpec: core.TaskSpec{Title: "Running"}, State: "running", Attempt: 1},
				{ID: "task_review", TaskSpec: core.TaskSpec{Title: "Review"}, State: "awaiting_review", Attempt: 1},
				{ID: "task_blocked", TaskSpec: core.TaskSpec{Title: "Blocked"}, State: "blocked", Attempt: 1},
			},
		}},
	}
	metrics := m.workMetrics()
	if metrics.total != 5 || metrics.accepted != 1 || metrics.working != 1 || metrics.review != 1 || metrics.blocked != 1 {
		t.Fatalf("progress counters are wrong: %+v", metrics)
	}
	line := strings.Join(m.progressLines(), "\n")
	if !strings.Contains(line, "1/5 accepted") || !strings.Contains(line, "20%") {
		t.Fatalf("progress line is missing the textual accepted/total fallback: %q", line)
	}
	if strings.Contains(line, "2/5") {
		t.Fatalf("a non-accepted task was counted as complete: %q", line)
	}
}

// TestWorkStatusBarSeparatesMetrics pins the compact live/review/blocked/
// attention summary so it is not a single blended number.
func TestWorkStatusBarSeparatesMetrics(t *testing.T) {
	m := workFixture()
	metrics := m.workMetrics()
	if metrics.live != 2 || metrics.review != 1 || metrics.blocked != 1 || metrics.attention != 2 {
		t.Fatalf("dashboard metrics changed: %+v", metrics)
	}
	bar := m.workStatusBar(metrics)
	for _, want := range []string{"2 live", "1 review", "1 blocked", "2 attention"} {
		if !strings.Contains(bar, want) {
			t.Fatalf("status bar %q is missing %q", bar, want)
		}
	}
	m.width, m.height = 100, 24
	if view := m.View(); !strings.Contains(view, "2 live · 1 review · 1 blocked · 2 attention") {
		t.Fatalf("dashboard view does not render the metric status bar:\n%s", view)
	}
}

// TestCollectionRowsKeepSelectionVisibleInNoColor proves the regularized rows
// keep their focus marker and stay within width without any color.
func TestCollectionRowsKeepSelectionVisibleInNoColor(t *testing.T) {
	m := workFixture()
	m.width, m.height = 80, 24
	m.navigate(route{Page: "tasks", SelectedID: "task_blocked"})
	items := m.filteredItems()
	rows := m.renderItems(items, 40, 12)
	if len(rows) > 12 {
		t.Fatalf("rendered %d rows for a 12-row region", len(rows))
	}
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "› ") {
		t.Fatalf("no-color selection marker disappeared:\n%s", joined)
	}
	if !strings.Contains(joined, "Choose refund policy") {
		t.Fatalf("selected row scrolled out of view:\n%s", joined)
	}
	if !strings.Contains(joined, strings.Repeat("─", 40)) {
		t.Fatalf("lower-emphasis separator missing:\n%s", joined)
	}
	for _, row := range rows {
		if width := ansi.StringWidth(row); width > 40 {
			t.Fatalf("row rendered %d columns: %q", width, row)
		}
	}
}

func TestPreviewActionLineIsSelectionAware(t *testing.T) {
	m := workFixture()
	cases := []struct {
		kind   string
		want   []string
		forbid []string
	}{
		{kind: "session", want: []string{"t terminal", "g jump", "Enter details"}},
		{kind: "task", want: []string{"t terminal", "Enter details"}, forbid: []string{"g jump"}},
		{kind: "artifact", want: []string{"Enter details"}, forbid: []string{"t terminal", "g jump"}},
		{kind: "decision", want: []string{"Enter details"}, forbid: []string{"t terminal", "g jump"}},
	}
	for _, tc := range cases {
		got := m.previewActions(tc.kind)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s preview actions %q missing %q", tc.kind, got, want)
			}
		}
		for _, forbid := range tc.forbid {
			if strings.Contains(got, forbid) {
				t.Errorf("%s preview actions %q should not advertise %q", tc.kind, got, forbid)
			}
		}
	}
}

// TestStableSelectionAcrossSortFilterAndRefresh keeps the ID authoritative.
func TestStableSelectionAcrossSortFilterAndRefresh(t *testing.T) {
	m := workFixture()
	m.width, m.height = 120, 32
	m.navigate(route{Page: "tasks", SelectedID: "task_review"})

	if _, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}); m.route.SelectedID != "task_review" {
		t.Fatalf("sort changed the selected ID: %q", m.route.SelectedID)
	}
	m.route.StatusFilter = "awaiting_review"
	if items := m.filteredItems(); len(items) != 1 || m.route.SelectedID != "task_review" {
		t.Fatalf("filter lost selection: %+v (selected %q)", items, m.route.SelectedID)
	}
	m.route.StatusFilter = ""

	refreshed := workFixture()
	m.snapshot = refreshed.snapshot
	m.validateSelection()
	if m.route.SelectedID != "task_review" {
		t.Fatalf("refresh changed the selected ID: %q", m.route.SelectedID)
	}
}

func TestEmptyStatesExplainAndOfferNextAction(t *testing.T) {
	t.Run("project advertises create", func(t *testing.T) {
		m := New(Config{ProjectFound: true, NoColor: true})
		m.width, m.height = 100, 24
		view := m.View()
		for _, want := range []string{"No workspaces in this project", "Press a to create a workspace."} {
			if !strings.Contains(view, want) {
				t.Fatalf("project empty state missing %q:\n%s", want, view)
			}
		}
		if strings.Contains(view, "never creates") {
			t.Fatalf("project empty state still contradicts the Create workspace action:\n%s", view)
		}
	})

	t.Run("collection titles, explains, and acts", func(t *testing.T) {
		m := New(Config{ProjectFound: true, WorkspaceID: "ws_empty", NoColor: true})
		m.width, m.height = 100, 24
		m.navigate(route{Page: "tasks"})
		view := m.View()
		for _, want := range []string{"No tasks yet", "This workspace has no recorded tasks.", "Press o to start the orchestrator"} {
			if !strings.Contains(view, want) {
				t.Fatalf("tasks empty state missing %q:\n%s", want, view)
			}
		}
	})

	t.Run("filtered empty clears the filter", func(t *testing.T) {
		m := workFixture()
		m.width, m.height = 100, 24
		m.navigate(route{Page: "tasks", Query: "no-such-task"})
		view := m.View()
		for _, want := range []string{"No matches", "Esc clears the filter."} {
			if !strings.Contains(view, want) {
				t.Fatalf("filtered empty state missing %q:\n%s", want, view)
			}
		}
	})
}

func TestWideCollectionsUseNamedPanels(t *testing.T) {
	m := workFixture()
	m.width, m.height = 120, 32
	m.navigate(route{Page: "tasks", SelectedID: "task_work"})
	wide := m.View()
	for _, want := range []string{"Preview", "Tasks", "› "} {
		if !strings.Contains(wide, want) {
			t.Fatalf("wide collection missing %q:\n%s", want, wide)
		}
	}
	m.width, m.height = 80, 24
	compact := m.View()
	if strings.Contains(compact, "Preview") {
		t.Fatalf("compact collection must not render the preview panel:\n%s", compact)
	}
}

// TestCollectionFramesFitAtSupportedSizes keeps the collection and project
// renderers within the accepted frame matrix, including the short-mode row.
func TestCollectionFramesFitAtSupportedSizes(t *testing.T) {
	sizes := [][2]int{{40, 12}, {60, 24}, {80, 18}, {100, 24}, {120, 32}}
	for _, size := range sizes {
		m := workFixture()
		m.width, m.height = size[0], size[1]
		m.navigate(route{Page: "tasks", SelectedID: "task_work"})
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) != size[1] {
			t.Fatalf("tasks %dx%d rendered %d rows", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if width := ansi.StringWidth(line); width > size[0] {
				t.Fatalf("tasks %dx%d line %d rendered %d columns: %q", size[0], size[1], i, width, line)
			}
		}
	}
}

func runtimeFixture() *Model {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_rt", NoColor: true})
	m.snapshot = core.WorkspaceSnapshot{
		ObservedAt: time.Now(),
		Status:     core.Status{Workspace: core.Workspace{ID: "ws_rt", Title: "Runtime demo", Status: "active"}},
	}
	m.runtime = core.RuntimeObservation{
		State: "present", SupervisorState: "running", ObservedAt: time.Now(),
		Topology: core.TmuxTopology{
			SessionName: "workspace-ws_rt", SessionExists: true,
			Windows: []core.TmuxWindow{{ID: "@1", Name: "orchestrator", Kind: "orchestrator", Active: true}},
			Panes:   []core.Pane{{ID: "%1", WindowID: "@1", Kind: "agent", SessionID: "sess_a", RunID: "run_a"}},
		},
	}
	m.route = route{Page: "runtime"}
	m.rebuildViewport()
	return m
}

// TestRuntimeTableAndStackedLayoutsAreEquivalent proves the wide comparison
// table and the compact fallback show the same topology values.
func TestRuntimeTableAndStackedLayoutsAreEquivalent(t *testing.T) {
	m := runtimeFixture()
	values := []string{"@1", "%1", "agent", "sess_a", "run_a", "running"}

	m.width, m.height = 120, 32
	m.rebuildViewport()
	wide := m.detailContent()
	for _, header := range []string{"Window", "Pane", "Kind", "Owner", "Run", "State"} {
		if !strings.Contains(wide, header) {
			t.Fatalf("wide runtime table missing the %q column:\n%s", header, wide)
		}
	}
	for _, value := range values {
		if !strings.Contains(wide, value) {
			t.Fatalf("wide runtime table missing %q:\n%s", value, wide)
		}
	}

	m.width, m.height = 80, 24
	m.rebuildViewport()
	compact := m.detailContent()
	for _, value := range values {
		if !strings.Contains(compact, value) {
			t.Fatalf("compact runtime stack missing %q:\n%s", value, compact)
		}
	}
	if strings.Contains(compact, "Owner") {
		t.Fatalf("compact runtime must not render the table header:\n%s", compact)
	}
	if !strings.Contains(compact, "Window @1 · pane %1 · agent") {
		t.Fatalf("compact runtime stack is not the equivalent representation:\n%s", compact)
	}
}

func TestRuntimeFramesFitAtSupportedSizes(t *testing.T) {
	for _, size := range [][2]int{{100, 24}, {120, 32}, {60, 24}, {40, 12}} {
		m := runtimeFixture()
		m.width, m.height = size[0], size[1]
		m.rebuildViewport()
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d rendered %d rows", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if width := ansi.StringWidth(line); width > size[0] {
				t.Fatalf("%dx%d line %d rendered %d columns: %q", size[0], size[1], i, width, line)
			}
		}
	}
}
