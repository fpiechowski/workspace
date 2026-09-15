package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"workspace/internal/core"
)

type managedHideBackend struct {
	Backend
	errors []error
	keys   []string
}

func (b *managedHideBackend) HideManagedUI(_ context.Context, _ string, key string) error {
	b.keys = append(b.keys, key)
	index := len(b.keys) - 1
	if index < len(b.errors) {
		return b.errors[index]
	}
	return nil
}

func TestSanitizeRemovesTerminalControlProtocols(t *testing.T) {
	input := "safe\x1b[31mred\x1b[0m\x1b]52;c;clipboard-secret\a\x1b]8;;https://evil.invalid\x1b\\linked\x1b]8;;\x1b\\\x1bP1;2;secret\x1b\\end"
	got := sanitize(input)
	if strings.Contains(got, "\x1b") || strings.Contains(got, "clipboard-secret") || strings.Contains(got, "https://") || strings.Contains(got, "1;2;secret") {
		t.Fatalf("terminal control payload remained: %q", got)
	}
	if !strings.Contains(got, "safe") || !strings.Contains(got, "red") || !strings.Contains(got, "linked") || !strings.Contains(got, "end") {
		t.Fatalf("ordinary text was removed: %q", got)
	}
}

func TestRenderedFrameFitsSmallAndUnicodeDimensions(t *testing.T) {
	model := New(Config{ProjectFound: true, ProjectRoot: "C:/项目/非常长的工作区路径", WorkspaceID: "ws_unicode", Theme: "dark"})
	model.snapshot = core.WorkspaceSnapshot{
		ObservedAt: time.Now(),
		Status:     core.Status{Workspace: core.Workspace{ID: "ws_unicode", Title: "界面 e\u0301 title with a very long suffix", Status: "active", Workflow: &core.Workflow{Phase: "implementation"}}},
	}
	model.route = route{Page: "dashboard"}
	model.snapshot.Status.Workspace.Tasks = []core.Task{{ID: "task_long", TaskSpec: core.TaskSpec{Title: "漢字 and combining e\u0301 with a long title"}, State: "blocked"}}
	model.snapshot.Metrics = core.WorkspaceMetrics{TaskTotal: 8, TaskAccepted: 3, TaskRunning: 2, TaskBlocked: 1, ReadyWorktrees: 4}
	model.rebuildViewport()
	for _, size := range [][2]int{{160, 48}, {120, 18}, {80, 24}, {60, 40}, {40, 12}, {30, 8}, {1, 1}} {
		model.width, model.height = size[0], size[1]
		model.rebuildViewport()
		view := model.View()
		lines := strings.Split(view, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d rendered %d rows", size[0], size[1], len(lines))
		}
		for lineNo, line := range lines {
			if width := ansi.StringWidth(line); width > size[0] {
				t.Fatalf("%dx%d line %d rendered %d columns: %q", size[0], size[1], lineNo, width, line)
			}
		}
	}
}

func TestGenerationFenceAndFilterOwnTheirKeys(t *testing.T) {
	model := New(Config{ProjectFound: true, WorkspaceID: "ws_b"})
	model.generation = 2
	model.snapshot = core.WorkspaceSnapshot{Status: core.Status{Workspace: core.Workspace{ID: "ws_b", Title: "B"}}}
	model.route = route{Page: "tasks"}
	model.snapshot.Status.Workspace.Tasks = []core.Task{{ID: "task_b", TaskSpec: core.TaskSpec{Title: "build"}, State: "pending"}}
	_, cmd := model.Update(snapshotMsg{generation: 1, value: core.WorkspaceSnapshot{Status: core.Status{Workspace: core.Workspace{ID: "ws_a", Title: "A"}}}})
	if cmd != nil || model.snapshot.Status.Workspace.ID != "ws_b" {
		t.Fatal("a late snapshot from another workspace replaced the current model")
	}
	_, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, key := range []rune{'q', 'g', 'r', '1', ' '} {
		_, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
	}
	if model.quit || !model.filtering || model.route.Query != "qgr1 " {
		t.Fatalf("filter keystrokes escaped into application shortcuts: quit=%t filtering=%t query=%q", model.quit, model.filtering, model.route.Query)
	}
}

func TestMainPageNavigationRestoresCollectionAndPreviewState(t *testing.T) {
	model := New(Config{ProjectFound: true, WorkspaceID: "ws_routes"})
	model.snapshot = core.WorkspaceSnapshot{
		Status: core.Status{
			Workspace: core.Workspace{
				ID: "ws_routes",
				Tasks: []core.Task{
					{ID: "task_compile", TaskSpec: core.TaskSpec{Title: "Compile API"}, State: "pending"},
					{ID: "task_compile_copy", TaskSpec: core.TaskSpec{Title: "Compile API"}, State: "pending"},
					{ID: "task_tests", TaskSpec: core.TaskSpec{Title: "Write tests"}, State: "pending"},
				},
			},
			Worktrees: []core.Worktree{{ID: "wt_main", Name: "main"}},
		},
	}
	model.route = route{Page: "tasks", Query: "compile", SelectedID: "task_compile", StatusFilter: "pending"}
	model.rebuildViewport()

	_, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if model.route.Page != "worktrees" {
		t.Fatalf("key 3 did not open worktrees: %+v", model.route)
	}
	_, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if model.route.Page != "tasks" || model.route.Query != "compile" || model.route.SelectedID != "task_compile" || model.route.StatusFilter != "pending" {
		t.Fatalf("returning to Tasks lost its filter or selection: %+v", model.route)
	}
	model.route.Query = "COMPILE"
	if items := model.filteredItems(); len(items) != 2 {
		t.Fatalf("case-insensitive name search did not retain duplicate titles: %+v", items)
	}
	model.route.Query = "task_tests"
	if items := model.filteredItems(); len(items) != 1 || items[0].ID != "task_tests" {
		t.Fatalf("ID search did not find the matching task: %+v", items)
	}

	model.preview = core.Preview{ResourceID: "document", Name: "WORKSPACE.md", Text: strings.Repeat("preview line\n", 40)}
	model.navigate(route{Page: "preview", EntityID: "document"})
	model.viewport.SetYOffset(20)
	model.navigate(route{Page: "runtime"})
	model.navigate(route{Page: "preview", EntityID: "document"})
	if model.viewport.YOffset != 20 {
		t.Fatalf("returning to a detail route lost its scroll position: got %d, want 20", model.viewport.YOffset)
	}
}

func TestCollectionStateFilterCyclesAvailableValues(t *testing.T) {
	model := New(Config{ProjectFound: true, WorkspaceID: "ws_filters"})
	model.snapshot = core.WorkspaceSnapshot{Status: core.Status{Workspace: core.Workspace{
		ID: "ws_filters",
		Tasks: []core.Task{
			{ID: "task_pending", TaskSpec: core.TaskSpec{Title: "Pending"}, State: "pending"},
			{ID: "task_blocked", TaskSpec: core.TaskSpec{Title: "Blocked"}, State: "blocked"},
			{ID: "task_accepted", TaskSpec: core.TaskSpec{Title: "Accepted"}, State: "accepted"},
		},
	}}}
	model.route = route{Page: "tasks"}
	_, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if model.route.StatusFilter != "accepted" {
		t.Fatalf("first status filter was not selected deterministically: %q", model.route.StatusFilter)
	}
	items := model.filteredItems()
	if len(items) != 1 || items[0].ID != "task_accepted" {
		t.Fatalf("status filter did not restrict the task collection: %+v", items)
	}
	_, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if model.route.StatusFilter != "blocked" {
		t.Fatalf("next status filter did not cycle through available states: %q", model.route.StatusFilter)
	}
}

func TestDashboardAttentionShowsFivePrioritizedRowsAndViewAll(t *testing.T) {
	model := New(Config{ProjectFound: true, WorkspaceID: "ws_attention"})
	model.snapshot = core.WorkspaceSnapshot{Status: core.Status{Workspace: core.Workspace{
		ID:              "ws_attention",
		PendingDecision: &core.Decision{ID: "decision_a", Question: "Choose a direction"},
		Tasks: []core.Task{
			{ID: "task_f", TaskSpec: core.TaskSpec{Title: "task_f"}, State: "blocked"},
			{ID: "task_e", TaskSpec: core.TaskSpec{Title: "task_e"}, State: "needs_changes"},
			{ID: "task_d", TaskSpec: core.TaskSpec{Title: "task_d"}, State: "blocked"},
			{ID: "task_c", TaskSpec: core.TaskSpec{Title: "task_c"}, State: "blocked"},
			{ID: "task_b", TaskSpec: core.TaskSpec{Title: "task_b"}, State: "blocked"},
			{ID: "task_a", TaskSpec: core.TaskSpec{Title: "task_a"}, State: "blocked"},
		},
	}}}
	items := model.attentionItems()
	if len(items) != 7 || items[0].Kind != "decision" || items[1].ID != "task_a" {
		t.Fatalf("attention priority or stable ID ordering changed: %+v", items)
	}
	panel := model.dashboardPanels()[2]
	if len(panel.Lines) != 6 || !strings.Contains(panel.Lines[0], "Choose a direction") || !strings.Contains(panel.Lines[1], "task_a") || !strings.Contains(panel.Lines[4], "task_d") || !strings.Contains(panel.Lines[5], "View all (7)") {
		t.Fatalf("dashboard did not show five attention rows plus View all: %+v", panel)
	}
}

func TestTabCyclesDashboardPanelsAndResultsTypes(t *testing.T) {
	model := New(Config{ProjectFound: true, WorkspaceID: "ws_focus"})
	model.width, model.height = 120, 18
	model.snapshot = core.WorkspaceSnapshot{
		ObservedAt: time.Now(),
		Status:     core.Status{Workspace: core.Workspace{ID: "ws_focus", Title: "Focus test", Status: "active"}},
	}
	model.route = route{Page: "dashboard"}
	model.rebuildViewport()

	for _, want := range []struct {
		key   tea.KeyMsg
		focus int
		text  string
	}{
		{key: tea.KeyMsg{Type: tea.KeyTab}, focus: 1, text: "Orchestrator"},
		{key: tea.KeyMsg{Type: tea.KeyTab}, focus: 2, text: "Needs attention"},
		{key: tea.KeyMsg{Type: tea.KeyTab}, focus: 3, text: "Recent recorded activity"},
	} {
		_, _ = model.updateKey(want.key)
		if model.route.Page != "dashboard" || model.focusedPanel != want.focus || !strings.Contains(model.View(), want.text) {
			t.Fatalf("Tab did not focus dashboard panel %d: page=%q focus=%d view=%q", want.focus, model.route.Page, model.focusedPanel, model.View())
		}
	}
	_, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	if model.focusedPanel != 2 {
		t.Fatalf("Shift+Tab did not move to the previous dashboard panel: %d", model.focusedPanel)
	}

	model.navigate(route{Page: "tasks"})
	model.navigate(route{Page: "dashboard"})
	if model.focusedPanel != 2 {
		t.Fatalf("returning to Dashboard lost panel focus: %d", model.focusedPanel)
	}

	model.navigate(route{Page: "results", Tab: "artifacts", Query: "important", SelectedID: "artifact_a"})
	_, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyTab})
	if model.route.Page != "results" || model.route.Tab != "handoffs" {
		t.Fatalf("Tab did not change the Results type: %+v", model.route)
	}
	model.route.Query, model.route.SelectedID = "review", "handoff_a"
	_, _ = model.updateKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	if model.route.Tab != "artifacts" || model.route.Query != "important" || model.route.SelectedID != "artifact_a" {
		t.Fatalf("returning to the Artifacts type lost its filter or selection: %+v", model.route)
	}
}

func TestManagedQuitSavesHideBeforeExitAndRetriesSameKey(t *testing.T) {
	backend := &managedHideBackend{errors: []error{errors.New("disk unavailable")}}
	model := New(Config{ProjectFound: true, WorkspaceID: "ws_managed", Managed: true, Backend: backend})
	model.width, model.height = 100, 30
	if !strings.Contains(model.footer(), "q hide") || !strings.Contains(model.helpView(), "hide this managed panel") {
		t.Fatal("managed TUI advertised quit instead of hide")
	}
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := model.updateKey(key)
	if cmd == nil {
		t.Fatal("q did not start the hide request")
	}
	_, quitCmd := model.Update(cmd())
	if quitCmd != nil || model.quit || model.hidePending {
		t.Fatalf("failed hide should leave the interface open: quit=%t pending=%t", model.quit, model.hidePending)
	}
	if model.loadError == "" {
		t.Fatal("failed hide did not expose its error")
	}
	_, cmd = model.updateKey(key)
	if cmd == nil {
		t.Fatal("q did not retry the hide request")
	}
	_, quitCmd = model.Update(cmd())
	if quitCmd == nil || !model.quit {
		t.Fatal("successful hide did not quit after persisting intent")
	}
	if len(backend.keys) != 2 || backend.keys[0] == "" || backend.keys[0] != backend.keys[1] {
		t.Fatalf("hide retry did not reuse its stable key: %v", backend.keys)
	}
}

type actionHarness struct {
	Backend
	workflows []string
	calls     []ActionCall
}

func (h *actionHarness) WorkflowNames(context.Context, string) ([]string, error) {
	return h.workflows, nil
}

func (h *actionHarness) PerformAction(_ context.Context, _ string, call ActionCall) error {
	h.calls = append(h.calls, call)
	return nil
}

func TestPauseInterruptConfirmationCapturesExactActiveTargets(t *testing.T) {
	model := New(Config{ProjectFound: true, WorkspaceID: "ws_actions"})
	model.snapshot = core.WorkspaceSnapshot{
		ObservedAt: time.Now(),
		Status: core.Status{
			Workspace: core.Workspace{ID: "ws_actions", Status: "active", Revision: 7},
			Sessions: []core.Session{
				{ID: "sess_active", CurrentRunID: "run_active", State: "running"},
				{ID: "sess_idle", CurrentRunID: "run_idle", State: "idle"},
			},
			Runs: []core.Run{
				{ID: "run_active", SessionID: "sess_active", State: "running"},
				{ID: "run_idle", SessionID: "sess_idle", State: "exited"},
			},
		},
		Services: []core.BackgroundService{
			{ID: "svc_running", State: "running"},
			{ID: "svc_stopped", State: "stopped"},
		},
	}
	if cmd := model.beginAction("pause_interrupt", ""); cmd == nil {
		t.Fatal("pause-interrupt did not open a confirmation")
	}
	if model.formAction.ExpectedRevision != 7 || len(model.formAction.ExpectedRunIDs) != 1 || model.formAction.ExpectedRunIDs[0] != "run_active" || len(model.formAction.ExpectedServiceIDs) != 1 || model.formAction.ExpectedServiceIDs[0] != "svc_running" {
		t.Fatalf("confirmation guard does not match active targets: %+v", model.formAction)
	}
	description := actionDescription("pause_interrupt", model.formAction)
	if !strings.Contains(description, "run_active") || !strings.Contains(description, "svc_running") || strings.Contains(description, "run_idle") || strings.Contains(description, "svc_stopped") {
		t.Fatalf("confirmation showed targets that will not be stopped: %q", description)
	}
}

func TestWorkflowSelectionLoadsNamesThenOpensGuardedConfirmation(t *testing.T) {
	backend := &actionHarness{workflows: []string{"issue-resolution", "plan-first"}}
	model := New(Config{ProjectFound: true, WorkspaceID: "ws_workflow", Backend: backend})
	model.snapshot = core.WorkspaceSnapshot{ObservedAt: time.Now(), Status: core.Status{Workspace: core.Workspace{ID: "ws_workflow", Status: "needs_workflow", Revision: 9}}}
	cmd := model.beginAction("select_workflow", "")
	if cmd == nil || !model.actionPending {
		t.Fatal("workflow list was not loaded asynchronously")
	}
	_, _ = model.Update(cmd())
	if model.actionPending || model.formMode != "workflow" || model.form == nil || model.form.View() == "" {
		t.Fatalf("workflow selector did not open: pending=%t mode=%q form=%v", model.actionPending, model.formMode, model.form)
	}
	if !strings.Contains(model.form.View(), "issue-resolution") || !strings.Contains(model.form.View(), "plan-first") {
		t.Fatalf("workflow selector omitted configured names: %q", model.form.View())
	}
	_ = model.confirmWorkflowSelection("plan-first")
	if model.formMode != "confirm" || model.formAction.TargetID != "plan-first" || model.formAction.ExpectedRevision != 9 {
		t.Fatalf("selected workflow did not reach a guarded confirmation: mode=%q call=%+v", model.formMode, model.formAction)
	}
}
