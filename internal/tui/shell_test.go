package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"workspace/internal/core"
)

func TestLayoutPolicyIsNormalized(t *testing.T) {
	cases := []struct {
		width, height int
		want          layoutMode
	}{
		{width: 39, height: 40, want: layoutTiny},
		{width: 120, height: 11, want: layoutTiny},
		{width: 40, height: 12, want: layoutCompact},
		{width: 60, height: 24, want: layoutCompact},
		{width: 80, height: 18, want: layoutCompact},
		{width: 99, height: 24, want: layoutCompact},
		{width: 100, height: 23, want: layoutCompact},
		{width: 100, height: 24, want: layoutWide},
		{width: 120, height: 32, want: layoutWide},
	}
	for _, tc := range cases {
		if got := layoutFor(tc.width, tc.height); got != tc.want {
			t.Fatalf("layoutFor(%d, %d) = %d, want %d", tc.width, tc.height, got, tc.want)
		}
	}
}

func shellFixture(theme string, noColor bool) *Model {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_frame", Theme: theme, NoColor: noColor})
	m.snapshot = core.WorkspaceSnapshot{
		ObservedAt: time.Now(),
		Status: core.Status{Workspace: core.Workspace{
			ID: "ws_frame", Title: "Checkout reliability", Status: "active",
			Workflow: &core.Workflow{Phase: "implementation"},
			Tasks: []core.Task{
				{ID: "task_done", TaskSpec: core.TaskSpec{Title: "Persist payment receipts"}, State: "accepted", Attempt: 1},
				{ID: "task_work", TaskSpec: core.TaskSpec{Title: "Retry failed payments"}, State: "running", Attempt: 2},
			},
		}},
	}
	m.route = route{Page: "tasks"}
	m.validateSelection()
	return m
}

// TestShellRegionsAtSupportedSizes covers every palette mode across the
// accepted frame sizes: each rendered line fits and the header, primary
// navigation, status row, and contextual key legend stay visible.
func TestShellRegionsAtSupportedSizes(t *testing.T) {
	modes := []struct {
		name    string
		theme   string
		noColor bool
	}{
		{name: "dark", theme: "dark"},
		{name: "light", theme: "light"},
		{name: "no-color", theme: "dark", noColor: true},
		{name: "auto-no-color", theme: "auto", noColor: true},
	}
	sizes := [][2]int{{40, 12}, {60, 24}, {80, 18}, {100, 24}, {120, 32}}
	for _, mode := range modes {
		for _, size := range sizes {
			name := fmt.Sprintf("%s-%dx%d", mode.name, size[0], size[1])
			t.Run(name, func(t *testing.T) {
				m := shellFixture(mode.theme, mode.noColor)
				m.width, m.height = size[0], size[1]
				m.notice = "Saved."
				m.rebuildViewport()
				view := m.View()
				lines := strings.Split(view, "\n")
				if len(lines) != size[1] {
					t.Fatalf("%dx%d rendered %d rows, want %d", size[0], size[1], len(lines), size[1])
				}
				for i, line := range lines {
					if width := ansi.StringWidth(line); width > size[0] {
						t.Fatalf("%dx%d line %d rendered %d columns: %q", size[0], size[1], i, width, line)
					}
				}
				if !strings.Contains(lines[0], "Checkout reliability") {
					t.Fatalf("%dx%d header missing the workspace identity: %q", size[0], size[1], lines[0])
				}
				if !strings.Contains(view, "1 Tasks") {
					t.Fatalf("%dx%d primary navigation missing: %q", size[0], size[1], view)
				}
				if !strings.Contains(lines[len(lines)-2], "Saved.") {
					t.Fatalf("%dx%d status row missing the notice: %q", size[0], size[1], lines[len(lines)-2])
				}
				if !strings.Contains(lines[len(lines)-1], "t terminal") {
					t.Fatalf("%dx%d key legend missing: %q", size[0], size[1], lines[len(lines)-1])
				}
			})
		}
	}
}

// TestCollectionHonorsSharedLayoutMode proves the Tasks collection no longer
// makes an independent width decision: the shared wide mode adds the preview
// column at 100x24, while 99x24 stays single-column.
func TestCollectionHonorsSharedLayoutMode(t *testing.T) {
	m := workFixture()
	if m.route.Page != "tasks" {
		t.Fatalf("fixture route = %q, want tasks", m.route.Page)
	}
	m.width, m.height = 100, 24
	m.rebuildViewport()
	wide := m.View()
	m.width, m.height = 99, 24
	m.rebuildViewport()
	compact := m.View()
	if !strings.Contains(wide, "Enter details") {
		t.Fatalf("tasks did not render the preview column at the shared wide breakpoint:\n%s", wide)
	}
	if strings.Contains(compact, "Enter details") {
		t.Fatalf("tasks rendered the preview column below the shared wide breakpoint:\n%s", compact)
	}
}

func TestBreadcrumbIdentifiesDetailAndParentedRoutes(t *testing.T) {
	m := workFixture()
	m.route = route{Page: "sessions", ParentID: "task_work"}
	if crumb := m.breadcrumb(); !strings.Contains(crumb, "Retry failed payments") || !strings.Contains(crumb, "sessions") {
		t.Fatalf("parented collection breadcrumb is incomplete: %q", crumb)
	}
	m.route = route{Page: "task", EntityID: "task_work"}
	if crumb := m.breadcrumb(); !strings.Contains(crumb, "Tasks") || !strings.Contains(crumb, "Retry failed payments") {
		t.Fatalf("detail breadcrumb is incomplete: %q", crumb)
	}
	m.route = route{Page: "tasks"}
	if crumb := m.breadcrumb(); crumb != "" {
		t.Fatalf("primary route should not render a breadcrumb: %q", crumb)
	}
}
