package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"workspace/internal/core"
)

// collapseHelp normalizes the aligned help columns so substring checks are not
// coupled to the key-column padding width.
func collapseHelp(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// TestKeyEnablementPerRepresentativeKind pins the selection-aware capability
// gates that drive both the footer and the full help screen.
func TestKeyEnablementPerRepresentativeKind(t *testing.T) {
	cases := []struct {
		name              string
		setup             func(*Model)
		terminal, jump    bool
		actions           bool
		forbidden, wanted []string
	}{
		{
			name:     "task",
			setup:    func(m *Model) { m.route = route{Page: "task", EntityID: "task_work"} },
			terminal: true, jump: false, actions: true,
			forbidden: []string{"g jump", "5 More"},
			wanted:    []string{"t terminal", "1 Sessions", "3 Results"},
		},
		{
			name:     "session",
			setup:    func(m *Model) { m.route = route{Page: "session", EntityID: "sess_worker"} },
			terminal: true, jump: true, actions: true,
			wanted: []string{"t terminal", "g jump"},
		},
		{
			name:     "artifact",
			setup:    func(m *Model) { m.route = route{Page: "artifact", EntityID: "artifact_x"} },
			terminal: false, jump: false, actions: false,
			forbidden: []string{"t terminal", "g jump", "a actions"},
		},
		{
			name:     "decision",
			setup:    func(m *Model) { m.route = route{Page: "decision", EntityID: "decision_x"} },
			terminal: false, jump: false, actions: false,
			forbidden: []string{"t terminal", "g jump", "a actions"},
		},
		{
			name: "workspace",
			setup: func(m *Model) {
				m.project = core.ProjectOverview{
					ObservedAt: time.Now(),
					Workspaces: []core.WorkspaceSummary{{ID: "ws_x", Title: "X", Status: "active"}},
				}
				m.route = route{Page: "project", SelectedID: "ws_x"}
			},
			terminal: true, jump: true, actions: true,
			wanted: []string{"t terminal", "g jump", "a actions"},
		},
		{
			name:     "runtime",
			setup:    func(m *Model) { m.route = route{Page: "runtime"} },
			terminal: false, jump: false, actions: true,
			forbidden: []string{"t terminal", "g jump"},
			wanted:    []string{"a actions"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := workFixture()
			tc.setup(m)
			flags := m.contextFlags()
			if flags.terminal != tc.terminal {
				t.Errorf("terminal = %t, want %t", flags.terminal, tc.terminal)
			}
			if flags.jump != tc.jump {
				t.Errorf("jump = %t, want %t", flags.jump, tc.jump)
			}
			if flags.actions != tc.actions {
				t.Errorf("actions = %t, want %t", flags.actions, tc.actions)
			}
			help := collapseHelp(m.helpView())
			for _, text := range tc.forbidden {
				if strings.Contains(help, text) {
					t.Errorf("help advertises unavailable %q:\n%s", text, help)
				}
			}
			for _, text := range tc.wanted {
				if !strings.Contains(help, text) {
					t.Errorf("help omits available %q:\n%s", text, help)
				}
			}
		})
	}
}

// TestFullHelpIsScrollableAtMinimumSize proves every help group is reachable at
// 40x12 through the viewport.
func TestFullHelpIsScrollableAtMinimumSize(t *testing.T) {
	m := workFixture()
	m.width, m.height = 40, 12
	m.showHelp = true
	m.helpViewport.Width = m.width
	m.helpViewport.Height = m.contentHeight()
	m.helpViewport.SetContent(m.helpView())
	m.helpViewport.GotoTop()

	content := m.helpView()
	for _, title := range []string{"Navigation", "View", "Runtime", "Actions", "Exit"} {
		if !strings.Contains(content, title) {
			t.Fatalf("full help is missing the %q group:\n%s", title, content)
		}
	}
	if total, height := m.helpViewport.TotalLineCount(), m.helpViewport.Height; total <= height {
		t.Fatalf("help must overflow a %d-line viewport, total=%d", height, total)
	}

	// The scroll keys are owned while help is open and move the viewport.
	before := m.helpViewport.YOffset
	if _, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyDown}); m.helpViewport.YOffset <= before {
		t.Fatalf("Down did not scroll the help viewport: %d -> %d", before, m.helpViewport.YOffset)
	}

	m.helpViewport.GotoBottom()
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 12 {
		t.Fatalf("40x12 help rendered %d rows", len(lines))
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width > 40 {
			t.Fatalf("help line %d rendered %d columns: %q", i, width, line)
		}
	}
	if !strings.Contains(view, "Press ? or Esc to return.") {
		t.Fatalf("bottom of help is unreachable at 40x12:\n%s", view)
	}
	if !strings.Contains(lines[len(lines)-2], "Help · lines") {
		t.Fatalf("help status row must show a scroll position: %q", lines[len(lines)-2])
	}
}

// TestRouteMemoryUnchangedByKeymap ensures selection, filters, sort, and
// viewport offsets still restore after the centralized bindings took over
// matching.
func TestRouteMemoryUnchangedByKeymap(t *testing.T) {
	m := workFixture()
	m.navigate(route{Page: "tasks", Query: "payments", SelectedID: "task_work", StatusFilter: "running", Sort: "name"})
	m.navigate(route{Page: "worktrees"})
	m.navigate(route{Page: "tasks"})
	if m.route.Query != "payments" || m.route.SelectedID != "task_work" || m.route.StatusFilter != "running" || m.route.Sort != "name" {
		t.Fatalf("route memory changed: %+v", m.route)
	}

	m.preview = core.Preview{ResourceID: "document", Name: "WORKSPACE.md", Text: strings.Repeat("line\n", 40)}
	m.navigate(route{Page: "preview", EntityID: "document"})
	m.viewport.SetYOffset(12)
	m.navigate(route{Page: "runtime"})
	m.navigate(route{Page: "preview", EntityID: "document"})
	if m.viewport.YOffset != 12 {
		t.Fatalf("viewport offset not restored: %d", m.viewport.YOffset)
	}
}

// TestSecondaryLabelsAndFocusMarkersAreUnambiguous covers the renamed dashboard
// section, Results type tabs, and the no-color focus marker.
func TestSecondaryLabelsAndFocusMarkersAreUnambiguous(t *testing.T) {
	m := workFixture()
	m.route = route{Page: "results", Tab: "handoffs"}
	crumb := m.breadcrumb()
	if !strings.Contains(crumb, "Artifacts") || !strings.Contains(crumb, "[Handoffs]") || !strings.Contains(crumb, "Checks") {
		t.Fatalf("Results type tabs are not labeled unambiguously: %q", crumb)
	}

	m.route = route{Page: "dashboard"}
	m.focusedPanel = 0
	view := m.View()
	if !strings.Contains(view, "[Agents & runs]") {
		t.Fatalf("no-color focus marker missing for the focused dashboard section:\n%s", view)
	}
	if strings.Contains(view, "Overview") {
		t.Fatalf("stale Overview copy remained:\n%s", view)
	}
}
