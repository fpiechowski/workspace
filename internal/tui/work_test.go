package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"workspace/internal/core"
)

func workFixture() *Model {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_demo", NoColor: true})
	m.snapshot = core.WorkspaceSnapshot{ObservedAt: time.Now(), Status: core.Status{
		Workspace: core.Workspace{ID: "ws_demo", Title: "Checkout reliability", Status: "active", OrchestratorAgentID: "agent_orch", Tasks: []core.Task{
			{ID: "task_done", TaskSpec: core.TaskSpec{Title: "Persist payment receipts"}, State: "accepted", Attempt: 1},
			{ID: "task_work", TaskSpec: core.TaskSpec{Title: "Retry failed payments"}, State: "running", Attempt: 2},
			{ID: "task_review", TaskSpec: core.TaskSpec{Title: "Verify duplicate submissions"}, State: "awaiting_review", Attempt: 1},
			{ID: "task_blocked", TaskSpec: core.TaskSpec{Title: "Choose refund policy"}, State: "blocked", Attempt: 1},
		}},
		Sessions: []core.Session{
			{ID: "sess_orch", AgentID: "agent_orch", AgentSnapshot: core.Agent{Name: "Coordinator"}, CurrentRunID: "run_orch", LifecycleState: "active", State: "running"},
			{ID: "sess_worker", TaskID: "task_work", TaskAttempt: 2, AgentSnapshot: core.Agent{Name: "Payments worker"}, CurrentRunID: "run_worker", LifecycleState: "active", State: "running"},
			{ID: "sess_other", TaskID: "task_review", TaskAttempt: 1, AgentSnapshot: core.Agent{Name: "Reviewer"}, LastRunID: "run_review", LifecycleState: "idle"},
			{ID: "sess_old", TaskID: "task_work", TaskAttempt: 1, AgentSnapshot: core.Agent{Name: "Old worker"}, LastRunID: "run_old", LifecycleState: "idle"},
		},
		Runs: []core.Run{
			{ID: "run_orch", SessionID: "sess_orch", State: "running", Route: core.Route{Model: "planner"}},
			{ID: "run_worker", SessionID: "sess_worker", State: "running", Route: core.Route{Model: "coder"}},
			{ID: "run_review", SessionID: "sess_other", State: "exited"},
			{ID: "run_old", SessionID: "sess_old", State: "failed"},
		},
	}}
	m.validateSelection()
	return m
}

func TestTasksAndSessionsCollectionsShowLiveWork(t *testing.T) {
	m := workFixture()
	for _, size := range [][2]int{{100, 24}, {120, 32}, {160, 40}} {
		m.width, m.height = size[0], size[1]
		m.navigate(route{Page: "tasks"})
		view := m.View()
		for _, text := range []string{"Payments worker", "Retry failed payments", "t terminal"} {
			if !strings.Contains(view, text) {
				t.Errorf("%v missing %q:\n%s", size, text, view)
			}
		}
	}
	m.navigate(route{Page: "tasks"})
	if m.filteredItems()[0].ID != "task_work" {
		t.Fatal("running task was not first")
	}
	task, _ := m.task("task_work")
	if subtitle := m.taskItem(task).Subtitle; !strings.Contains(subtitle, "1 live · 1 sessions · 1 runs") {
		t.Fatal(subtitle)
	}
	m.navigate(route{Page: "sessions"})
	if items := m.filteredItems(); len(items) != 4 {
		t.Fatalf("root sessions collection lost rows: %+v", items)
	}
	m.navigate(route{Page: "sessions", ParentID: "task_review"})
	if items := m.filteredItems(); len(items) != 1 || items[0].ID != "sess_other" {
		t.Fatalf("unrelated task sessions leaked: %+v", items)
	}
}

func TestFilterEscapeRestoresSelectionAndOwnsRetryKey(t *testing.T) {
	for _, managed := range []bool{false, true} {
		m := workFixture()
		m.managed = managed
		m.route = route{Page: "tasks", Query: "payments", SelectedID: "task_work"}
		m.actionFailure = true
		m.lastAction = &ActionCall{Action: "pause"}
		m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		_, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		_ = cmd
		if m.actionPending || m.route.Query != "paymentsy" {
			t.Fatal("filter triggered a retry")
		}
		m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
		if m.filtering || m.quit || m.route.Query != "payments" || m.route.SelectedID != "task_work" {
			t.Fatalf("Esc did not restore filter: %+v", m.route)
		}
		m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
		if m.route.Query != "" || m.route.Page != "tasks" {
			t.Fatal("second Esc did not clear applied filter")
		}
	}
}

func TestEscapeCancelsActionSelectAndConfirmation(t *testing.T) {
	m := workFixture()
	m.backend = &actionHarness{}
	m.navigate(route{Page: "orchestrator"})
	m.openActionMenu()
	if m.form == nil {
		t.Fatal("action form did not open")
	}
	m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.form != nil || m.actionPending {
		t.Fatal("Esc did not leave searchable action form")
	}
	m.beginAction("start_orchestrator", "")
	m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.form != nil || m.actionPending {
		t.Fatal("Esc submitted confirmation")
	}
}

func TestEnterSubmitsConfirmationSelection(t *testing.T) {
	t.Run("confirm", func(t *testing.T) {
		m := workFixture()
		backend := &actionHarness{}
		m.backend = backend
		m.beginAction("start_orchestrator", "")

		m.updateKey(tea.KeyMsg{Type: tea.KeyRight})
		_, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Enter did not advance the confirmation field")
		}
		_, cmd = m.Update(cmd())
		if cmd == nil {
			t.Fatal("confirmation field did not advance to form submission")
		}
		_, cmd = m.Update(cmd())
		if !m.actionPending || m.form != nil || cmd == nil {
			t.Fatalf("Enter did not submit Confirm: pending=%t form=%v cmd=%v", m.actionPending, m.form, cmd)
		}
		if len(backend.calls) != 0 {
			t.Fatal("action command ran synchronously while submitting the form")
		}
	})

	t.Run("cancel", func(t *testing.T) {
		m := workFixture()
		backend := &actionHarness{}
		m.backend = backend
		m.beginAction("start_orchestrator", "")

		_, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Enter did not advance the confirmation field")
		}
		_, cmd = m.Update(cmd())
		if cmd == nil {
			t.Fatal("confirmation field did not advance to form submission")
		}
		_, cmd = m.Update(cmd())
		// The returned command may still carry an animation tick while a live
		// run is on screen; cancellation is proven by the absence of a mutation.
		_ = cmd
		if m.actionPending || m.form != nil || m.notice != "Action cancelled." {
			t.Fatalf("Enter did not submit Cancel: pending=%t form=%v notice=%q", m.actionPending, m.form, m.notice)
		}
		if len(backend.calls) != 0 {
			t.Fatal("Cancel invoked an action")
		}
	})
}

func TestProjectAndWorkspaceDeletionActions(t *testing.T) {
	model := New(Config{ProjectFound: true})
	model.backend = &actionHarness{workflows: []string{"plan-first"}}
	model.project = core.ProjectOverview{
		ObservedAt: time.Now(),
		Workspaces: []core.WorkspaceSummary{{ID: "ws_delete", Title: "Disposable", Status: "active", Revision: 9}},
	}
	model.route = route{Page: "project", SelectedID: "ws_delete"}
	model.validateSelection()

	if cmd := model.beginAction("create_workspace", ""); cmd == nil {
		t.Fatal("create workspace did not load workflows")
	} else {
		_, _ = model.Update(cmd())
	}
	if model.form == nil || model.formMode != "create_workspace" {
		t.Fatalf("create workspace form was not opened: mode=%q", model.formMode)
	}
	model.form = nil
	model.formMode = ""

	if cmd := model.beginAction("delete_workspace", "ws_delete"); cmd == nil {
		t.Fatal("delete workspace did not require typed confirmation")
	}
	if model.formMode != "destructive" || model.formAction.ExpectedRevision != 9 || model.formAction.TargetID != "ws_delete" {
		t.Fatalf("workspace deletion guard is incomplete: %+v mode=%q", model.formAction, model.formMode)
	}
	if !strings.Contains(model.formAction.TargetDetails, "uncommitted file") || !strings.Contains(model.formAction.TargetDetails, "cannot be undone") {
		t.Fatalf("workspace deletion warning is incomplete: %q", model.formAction.TargetDetails)
	}
}

func TestCreateWorkspaceOffersDistinctManualChoice(t *testing.T) {
	model := New(Config{ProjectFound: true})
	model.backend = &actionHarness{workflows: []string{"plan-first"}}
	model.project = core.ProjectOverview{ObservedAt: time.Now()}
	model.route = route{Page: "project"}
	model.validateSelection()

	cmd := model.beginAction("create_workspace", "")
	if cmd == nil {
		t.Fatal("create workspace did not load workflows")
	}
	_, _ = model.Update(cmd())
	if model.form == nil || model.formMode != "create_workspace" {
		t.Fatalf("create workspace form was not opened: mode=%q", model.formMode)
	}
	view := model.form.View()
	for _, option := range []string{"Choose later", "No workflow (manual orchestration)"} {
		if !strings.Contains(view, option) {
			t.Fatalf("creation form lacks %q:\n%s", option, view)
		}
	}

	if workflow, manual := createWorkflowChoice(createManualChoice); workflow != "" || !manual {
		t.Fatalf("manual choice decoded as workflow=%q manual=%t", workflow, manual)
	}
	if workflow, manual := createWorkflowChoice("plan-first"); workflow != "plan-first" || manual {
		t.Fatalf("named choice decoded as workflow=%q manual=%t", workflow, manual)
	}
	if workflow, manual := createWorkflowChoice(""); workflow != "" || manual {
		t.Fatalf("choose-later choice decoded as workflow=%q manual=%t", workflow, manual)
	}
	options := createOptions(ActionCall{Action: "create_workspace", TargetName: "Manual", Input: "intent", NoWorkflow: true, Key: "tui:1"})
	if !options.NoWorkflow || options.Workflow != "" || options.Title != "Manual" || options.Input != "intent" || options.OperationKey != "tui:1" {
		t.Fatalf("manual choice did not reach CreateOptions: %+v", options)
	}
}

func TestCompletedWorkspaceCanBeArchivedFromTUI(t *testing.T) {
	model := workFixture()
	model.backend = &actionHarness{}
	model.snapshot.Status.Workspace.Status = "completed"
	model.snapshot.Status.Workspace.Revision = 12
	model.navigate(route{Page: "orchestrator"})

	if cmd := model.beginAction("archive_workspace", ""); cmd == nil {
		t.Fatal("archive did not require confirmation")
	}
	if model.form == nil || model.formMode != "confirm" || model.formAction.ExpectedRevision != 12 {
		t.Fatalf("archive confirmation did not preserve the workspace revision: %+v mode=%q", model.formAction, model.formMode)
	}
	if !strings.Contains(actionCaption(model.formAction), "Archive completed workspace") {
		t.Fatalf("archive confirmation is unclear: %q", actionCaption(model.formAction))
	}
}

func TestWorkspaceDeletionAllowsSlowMountedFilesystemCleanup(t *testing.T) {
	if got := actionTimeout(ActionCall{Action: "delete_workspace"}); got != 10*time.Minute {
		t.Fatalf("workspace deletion timeout = %s", got)
	}
	if got := actionTimeout(ActionCall{Action: "archive_workspace"}); got != 45*time.Second {
		t.Fatalf("ordinary action timeout = %s", got)
	}
}

func TestDeletedTasksAndSessionsDisappearFromCollections(t *testing.T) {
	model := workFixture()
	now := time.Now()
	model.snapshot.Status.Workspace.Tasks = append(model.snapshot.Status.Workspace.Tasks,
		core.Task{ID: "task_deleted", TaskSpec: core.TaskSpec{Title: "Deleted task"}, State: "deleted", DeletedAt: &now})
	model.snapshot.Status.Sessions = append(model.snapshot.Status.Sessions,
		core.Session{ID: "sess_deleted", AgentSnapshot: core.Agent{Name: "Deleted session"}, DeletedAt: &now})

	model.navigate(route{Page: "tasks"})
	for _, item := range model.filteredItems() {
		if item.ID == "task_deleted" {
			t.Fatal("deleted task remained in the task collection")
		}
	}
	model.navigate(route{Page: "sessions"})
	for _, item := range model.filteredItems() {
		if item.ID == "sess_deleted" {
			t.Fatal("deleted session remained in the session collection")
		}
	}
}

func TestActionFailureShowsErrorInsteadOfOperationKey(t *testing.T) {
	model := workFixture()
	model.width, model.height = 160, 30
	call := ActionCall{Action: "delete_task", TargetID: "task_work", Key: "tui_01M2N7NTJQGV6Y84126SVNVD9A"}
	err := &core.Error{Code: "task_referenced", Message: "task is required by task_child"}

	model.finishAction(actionResultMsg{generation: model.generation, call: call, err: err})

	if !strings.Contains(model.notice, err.Error()) {
		t.Fatalf("failure notice does not contain the operation error: %q", model.notice)
	}
	if strings.Contains(model.notice, call.Key) {
		t.Fatalf("failure notice exposes the operation key instead of the error: %q", model.notice)
	}
	if model.lastAction == nil || model.lastAction.Key != call.Key {
		t.Fatal("failed action did not preserve its key for an exact retry")
	}
	if view := model.View(); !strings.Contains(view, err.Error()) {
		t.Fatalf("rendered failure does not contain the operation error:\n%s", view)
	} else if !strings.Contains(view, "action failed") || strings.Contains(view, "refresh failed") {
		t.Fatalf("rendered action failure has a misleading status:\n%s", view)
	}
}

type terminalHarness struct {
	Backend
	refs []core.EntityRef
}

func (h *terminalHarness) ResolveNavigationTarget(_ context.Context, _ string, ref core.EntityRef) (core.NavigationTarget, error) {
	h.refs = append(h.refs, ref)
	return core.NavigationTarget{}, nil
}

type unusedNavigator struct{}

func (unusedNavigator) Select(context.Context, core.NavigationTarget) error { return nil }
func (unusedNavigator) PrepareAttach(context.Context, core.NavigationTarget) (*exec.Cmd, error) {
	return nil, nil
}

func TestTerminalUsesExactCurrentRunAndConfirmsIdleResume(t *testing.T) {
	m := workFixture()
	backend := &terminalHarness{}
	m.backend = backend
	m.navigator = unusedNavigator{}
	m.navigate(route{Page: "sessions"})
	m.route.SelectedID = "sess_worker"
	cmd := m.openTerminal()
	if cmd == nil {
		t.Fatal("terminal command missing")
	}
	cmd()
	if len(backend.refs) != 1 || backend.refs[0].ID != "run_worker" || backend.refs[0].Kind != "run" {
		t.Fatal(backend.refs)
	}
	m.navigationPending = false
	m.route.SelectedID = "sess_other"
	m.openTerminal()
	if m.form == nil || m.formAction.Action != "resume_session" || m.formAction.TargetID != "sess_other" || !m.formAction.OpenTerminal || m.actionPending {
		t.Fatal("idle session must confirm before launch")
	}
}

func TestSortAndAnimationDoNotLoseSelection(t *testing.T) {
	m := workFixture()
	m.route = route{Page: "tasks", SelectedID: "task_work"}
	m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.filteredItems()[0].ID != "task_blocked" || m.route.SelectedID != "task_work" {
		t.Fatal("name sort lost selection or order")
	}
	m.navigate(route{Page: "results"})
	m.navigate(route{Page: "tasks"})
	if m.route.Sort != "name" {
		t.Fatal("sort not remembered")
	}
	before := m.stateLabel("running")
	_, cmd := m.Update(animationMsg{})
	if cmd == nil || m.stateLabel("running") == before || m.route.SelectedID != "task_work" {
		t.Fatal("animation lost state or stopped")
	}
}

func TestWorkspaceEntryLandsOnTasks(t *testing.T) {
	direct := New(Config{ProjectFound: true, WorkspaceID: "ws_entry"})
	if direct.route.Page != "tasks" {
		t.Fatalf("direct workspace entry route = %q, want tasks", direct.route.Page)
	}

	picker := New(Config{ProjectFound: true})
	picker.project = core.ProjectOverview{ObservedAt: time.Now(), Workspaces: []core.WorkspaceSummary{{ID: "ws_entry", Title: "Entry", Status: "active"}}}
	picker.route = route{Page: "project", SelectedID: "ws_entry"}
	picker.validateSelection()
	_, _ = picker.openSelection()
	if picker.route.Page != "tasks" {
		t.Fatalf("project picker selection route = %q, want tasks", picker.route.Page)
	}
}

func TestPrimaryOrderLabelsAtWideAndCompactWidths(t *testing.T) {
	m := workFixture()
	m.navigate(route{Page: "tasks"})
	m.width, m.height = 120, 32
	wide := m.View()
	for _, want := range []string{"1 Tasks", "2 Sessions", "3 Worktrees", "4 Results", "5 More"} {
		if !strings.Contains(wide, want) {
			t.Fatalf("wide shell missing primary label %q:\n%s", want, wide)
		}
	}
	m.width, m.height = 80, 24
	compact := m.View()
	for _, want := range []string{"1 Tasks", "2 Sessions", "3 Worktrees", "4 Results", "5 More"} {
		if !strings.Contains(compact, want) {
			t.Fatalf("compact shell missing primary label %q:\n%s", want, compact)
		}
	}
	m.width, m.height = 40, 12
	narrow := m.View()
	for _, want := range []string{"1 Tasks", "2 Sess", "3 Trees", "4 Out", "5 More"} {
		if !strings.Contains(narrow, want) {
			t.Fatalf("minimum-width shell missing primary label %q:\n%s", want, narrow)
		}
	}
	help := collapseHelp(m.helpView())
	for _, want := range []string{"1 Tasks", "2 Sessions", "3 Worktrees", "4 Results", "5 More"} {
		if !strings.Contains(help, want) {
			t.Fatalf("full help missing primary label %q:\n%s", want, help)
		}
	}
}

func TestTaskDetailRelatedShortcutsStayRelated(t *testing.T) {
	cases := []struct{ key, page, tab string }{
		{"1", "sessions", ""},
		{"2", "worktrees", ""},
		{"3", "results", "handoffs"},
	}
	for _, tc := range cases {
		m := workFixture()
		m.snapshot.Status.Worktrees = []core.Worktree{{ID: "wt_main", Name: "main"}}
		m.navigate(route{Page: "task", EntityID: "task_work"})
		_, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
		if m.route.Page != tc.page || m.route.ParentID != "task_work" {
			t.Fatalf("task detail key %s opened %+v, want related %s", tc.key, m.route, tc.page)
		}
		if tc.tab != "" && m.route.Tab != tc.tab {
			t.Fatalf("task detail key %s tab = %q, want %q", tc.key, m.route.Tab, tc.tab)
		}
	}
}

func TestRuntimeOffersWorkspaceActions(t *testing.T) {
	m := workFixture()
	m.snapshot.Status.Workspace.Status = "active"
	m.navigate(route{Page: "runtime"})
	values := actionValues(t, m)
	for _, action := range []string{"start_orchestrator", "pause", "pause_interrupt"} {
		if !values[action] {
			t.Fatalf("runtime lost workspace action %q: %+v", action, values)
		}
	}
}

func TestSessionsCurrentHistoryFilterIsVisibleAndEscapeClearsIt(t *testing.T) {
	m := workFixture()
	now := time.Now()
	m.snapshot.Status.Sessions = append(m.snapshot.Status.Sessions,
		core.Session{ID: "sess_closed", AgentSnapshot: core.Agent{Name: "Closed worker"}, LifecycleState: "closed", ClosedAt: &now})
	m.navigate(route{Page: "sessions"})
	all := len(m.filteredItems())

	m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.route.StatusFilter != "current" || !strings.Contains(m.View(), "status: current") || len(m.filteredItems()) != all-1 {
		t.Fatalf("current filter must be visible and hide closed sessions: filter=%q view=%q", m.route.StatusFilter, m.View())
	}
	m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	history := m.filteredItems()
	if m.route.StatusFilter != "history" || len(history) != 1 || history[0].ID != "sess_closed" {
		t.Fatalf("history filter must show only closed sessions: filter=%q items=%+v", m.route.StatusFilter, history)
	}
	m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.route.StatusFilter != "" || m.route.Page != "sessions" || len(m.filteredItems()) != all {
		t.Fatalf("Esc must clear status before navigating: %+v", m.route)
	}
}

func TestMoreOmitsSessionsAndKeepsAttentionAndActivity(t *testing.T) {
	m := workFixture()
	m.navigate(route{Page: "more"})
	ids := make(map[string]bool)
	for _, item := range m.filteredItems() {
		ids[item.ID] = true
	}
	if ids["sessions"] {
		t.Fatalf("More still duplicates the primary Sessions route: %+v", ids)
	}
	for _, want := range []string{"attention", "activity"} {
		if !ids[want] {
			t.Fatalf("More lost the %q page: %+v", want, ids)
		}
	}
	m.route.SelectedID = "attention"
	if _, cmd := m.openSelection(); cmd != nil || m.route.Page != "attention" {
		t.Fatalf("More did not open Needs attention: page=%q cmd=%v", m.route.Page, cmd)
	}
}
