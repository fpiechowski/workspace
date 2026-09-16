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

func TestWorkViewShowsProgressAndCurrentAssignments(t *testing.T) {
	m := workFixture()
	for _, size := range [][2]int{{60, 24}, {120, 32}, {160, 40}} {
		m.width, m.height = size[0], size[1]
		view := m.View()
		for _, text := range []string{"1/4 accepted", "25%", "2 live agents", "Orchestrator", "Payments worker", "Retry failed payments", "t terminal"} {
			if !strings.Contains(view, text) {
				t.Errorf("%v missing %q:\n%s", size, text, view)
			}
		}
	}
	for _, item := range m.workItems() {
		if item.ID == "sess_old" {
			t.Fatal("historical attempt appeared as current work")
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
	m.navigate(route{Page: "sessions", ParentID: "task_review"})
	if items := m.filteredItems(); len(items) != 1 || items[0].ID != "sess_other" {
		t.Fatalf("unrelated task sessions leaked: %+v", items)
	}
}

func TestFilterEscapeRestoresSelectionAndOwnsRetryKey(t *testing.T) {
	for _, managed := range []bool{false, true} {
		m := workFixture()
		m.managed = managed
		m.navigate(route{Page: "tasks", Query: "payments", SelectedID: "task_work"})
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
		if m.actionPending || m.form != nil || cmd != nil || m.notice != "Action cancelled." {
			t.Fatalf("Enter did not submit Cancel: pending=%t form=%v cmd=%v notice=%q", m.actionPending, m.form, cmd, m.notice)
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

func TestCompletedWorkspaceCanBeArchivedFromTUI(t *testing.T) {
	model := workFixture()
	model.backend = &actionHarness{}
	model.snapshot.Status.Workspace.Status = "completed"
	model.snapshot.Status.Workspace.Revision = 12
	model.navigate(route{Page: "dashboard"})

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
	m.navigate(route{Page: "tasks", SelectedID: "task_work"})
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

func TestDashboardStatusFilterIsVisibleAndEscapeClearsIt(t *testing.T) {
	m := workFixture()
	all := len(m.filteredItems())
	m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.route.StatusFilter == "" || !strings.Contains(m.View(), "state: "+m.route.StatusFilter) || len(m.filteredItems()) == all {
		t.Fatal("active status filter must be visible and restrict work")
	}
	m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.route.StatusFilter != "" || m.route.Page != "dashboard" || len(m.filteredItems()) != all {
		t.Fatal("Esc must clear status before navigating")
	}
}
