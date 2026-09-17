package core

import (
	"context"
	"testing"
)

func menuHas(menu Menu, id string) bool {
	for _, action := range menu.Actions {
		if action.ID == id {
			return true
		}
	}
	return false
}

// A manual workspace supports the ordinary lifecycle: pause leaves runs
// untouched, pause-and-interrupt stops exactly the confirmed runs, and resume
// makes it active again.
func TestManualWorkspaceLifecycleControls(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	agent, worktree := worker(t, s, ws, "manual-lifecycle")
	session, err := s.StartSession(ctx, ws, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}

	paused, err := s.SetPaused(ctx, ws, true)
	if err != nil {
		t.Fatalf("manual pause failed: %v", err)
	}
	if paused.Workspace.Status != "paused" || !paused.Workspace.Manual() {
		t.Fatalf("manual pause state: %+v", paused.Workspace)
	}
	stillActive := false
	for _, p := range paused.Sessions {
		if p.ID == session.ID && p.Active() {
			stillActive = true
		}
	}
	if !stillActive {
		t.Fatal("pause killed an active manual run")
	}

	resumed, err := s.SetPausedGuarded(ctx, ws, false, "manual-resume", MutationGuard{ExpectedRevision: paused.Workspace.Revision})
	if err != nil {
		t.Fatalf("manual resume failed: %v", err)
	}
	if resumed.Workspace.Status != "active" || !resumed.Workspace.Manual() {
		t.Fatalf("manual resume state: %+v", resumed.Workspace)
	}

	guard := MutationGuard{ExpectedRevision: resumed.Workspace.Revision, ExpectedRunIDs: []string{session.CurrentRunID}}
	interrupted, err := s.PauseInterruptGuarded(ctx, ws, "manual-pause-interrupt", guard)
	if err != nil {
		t.Fatalf("manual pause-interrupt failed: %v", err)
	}
	if interrupted.Workspace.Status != "paused" {
		t.Fatalf("pause-interrupt state: %s", interrupted.Workspace.Status)
	}
	for _, p := range interrupted.Sessions {
		if p.ID == session.ID && p.State != "stopped" {
			t.Fatalf("confirmed manual run was not stopped: %s", p.State)
		}
	}
}

// Pause and pause-interrupt keep their pending-selection prompt while no
// workflow has been chosen.
func TestNeedsWorkflowLifecycleStillBlocked(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	pending, err := s.Create(ctx, CreateOptions{Title: "Pending", Input: "Choose later"})
	if err != nil {
		t.Fatal(err)
	}
	ws := pending.Workspace.ID
	_, err = s.SetPaused(ctx, ws, true)
	expectCode(t, err, "decision_required")
	_, err = s.PauseInterruptGuarded(ctx, ws, "pending-pause", MutationGuard{})
	expectCode(t, err, "decision_required")
}

// The menu distinguishes manual operation from a pending selection and from a
// selected workflow: manual shows pause/resume without select or advance,
// pending still offers selection, and a workflow keeps its advance action.
func TestManualWorkspaceMenuModes(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()

	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if menu.Phase != "manual" {
		t.Fatalf("manual menu phase: %q", menu.Phase)
	}
	for _, id := range []string{"status", "sessions", "artifacts", "inbox", "pause"} {
		if !menuHas(menu, id) {
			t.Fatalf("manual menu is missing %q: %+v", id, menu.Actions)
		}
	}
	for _, id := range []string{"workflow", "advance"} {
		if menuHas(menu, id) {
			t.Fatalf("manual menu offers workflow-only action %q: %+v", id, menu.Actions)
		}
	}

	if _, err := s.SetPaused(ctx, ws, true); err != nil {
		t.Fatal(err)
	}
	pausedMenu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !menuHas(pausedMenu, "resume") || menuHas(pausedMenu, "pause") {
		t.Fatalf("paused manual menu: %+v", pausedMenu.Actions)
	}

	pending, err := s.Create(ctx, CreateOptions{Title: "Pending", Input: "Choose later"})
	if err != nil {
		t.Fatal(err)
	}
	pendingMenu, err := s.Menu(ctx, pending.Workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !menuHas(pendingMenu, "workflow") {
		t.Fatalf("pending menu lost the selection action: %+v", pendingMenu.Actions)
	}

	// The fixture creates an issue-resolution workspace and keeps advancing.
	workflowMenu, err := s.Menu(ctx, fixtureWorkspaceID(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if workflowMenu.Phase == "manual" || !menuHas(workflowMenu, "advance") {
		t.Fatalf("workflow menu changed: phase=%q actions=%+v", workflowMenu.Phase, workflowMenu.Actions)
	}
}

func fixtureWorkspaceID(t *testing.T, s *Service) string {
	t.Helper()
	statuses, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range statuses {
		if status.Workspace.WorkflowSelected() {
			return status.Workspace.ID
		}
	}
	t.Fatal("no workflow workspace found")
	return ""
}

// The shared read model exposes the manual label to the CLI and TUI while a
// pending workspace keeps an empty phase.
func TestManualWorkspacePhaseLabelInOverview(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	pending, err := s.Create(ctx, CreateOptions{Title: "Pending", Input: "Choose later"})
	if err != nil {
		t.Fatal(err)
	}
	overview, err := s.ProjectOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	phases := map[string]string{}
	for _, row := range overview.Workspaces {
		phases[row.ID] = row.Phase
	}
	if phases[ws] != "manual" {
		t.Fatalf("manual overview phase: %q", phases[ws])
	}
	if phases[pending.Workspace.ID] != "" {
		t.Fatalf("pending overview phase: %q", phases[pending.Workspace.ID])
	}
	if phases[fixtureWorkspaceID(t, s)] != "planning" {
		t.Fatalf("workflow overview phase: %q", phases[fixtureWorkspaceID(t, s)])
	}
}

// state update cannot fake a phase transition for a manual workspace and must
// not mutate it.
func TestManualWorkspaceStatePhaseNotApplicable(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	before, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	phase := "implementing"
	_, err = s.UpdateState(ctx, ws, before.Workspace.Revision, StatePatch{Phase: &phase})
	expectCode(t, err, "operation_not_applicable")
	after, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if after.Workspace.Revision != before.Workspace.Revision || after.Workspace.Workflow != nil || after.Workspace.Status != "active" {
		t.Fatalf("manual workspace mutated: %+v", after.Workspace)
	}
}

// Workflow-only operations stay explicitly unavailable in manual mode.
func TestManualWorkspaceWorkflowOnlyOperationsUnavailable(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	before, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	revision := before.Workspace.Revision

	if _, err := s.AdvanceWorkflow(ctx, ws, "", "manual-advance"); err == nil {
		t.Fatal("workflow advance was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	if _, err := s.PrepareIntegration(ctx, ws, IntegrationOptions{}); err == nil {
		t.Fatal("integration was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	if _, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{}); err == nil {
		t.Fatal("change-request preparation was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	if _, err := s.PublishChangeRequest(ctx, ws, "cr_missing", false); err == nil {
		t.Fatal("change-request publication was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	if _, err := s.SyncChangeRequests(ctx, ws); err == nil {
		t.Fatal("change-request sync was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	if _, err := s.ResolveChangeRequest(ctx, ws, "cr_missing", "skip", "", "reason", false); err == nil {
		t.Fatal("change-request resolution was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	if _, err := s.ConfirmRelease(ctx, ws, "release-1", false); err == nil {
		t.Fatal("release confirmation was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	if _, err := s.RefreshDecision(ctx, ws); err == nil {
		t.Fatal("decision refresh was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	if _, err := s.AnswerDecision(ctx, ws, DecisionAnswer{ID: "decision_missing", ExpectedRevision: revision + 1, Answer: "skip", Reason: "reason"}); err == nil {
		t.Fatal("decision answer was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	if _, err := s.SelectWorkflow(ctx, ws, "issue-resolution"); err == nil {
		t.Fatal("workflow selection converted a manual workspace")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}

	// Migration is gated behind a paused workspace, so pause first and confirm
	// the manual guard is still the deciding factor.
	paused, err := s.SetPaused(ctx, ws, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MigrateWorkflow(ctx, ws, RevisionOptions{ExpectedRevision: paused.Workspace.Revision, Reason: "manual migration test"}); err == nil {
		t.Fatal("workflow migration was accepted")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}

	after, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if after.Workspace.Workflow != nil || !after.Workspace.Manual() {
		t.Fatalf("manual workspace gained a workflow: %+v", after.Workspace)
	}
}

// The supervisor recovers an interrupted orchestrator in an active manual
// workspace without inventing a workflow.
func TestSupervisorRecoversInterruptedManualOrchestrator(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	p, err := s.StartOrchestrator(ctx, ws, "manual-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	rt := s.Runtime.(*fakeRuntime)
	delete(rt.panes, p.PaneID)
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Sessions) != 1 || len(v.Runs) != 2 || v.Runs[0].State != "interrupted" || v.Sessions[0].CurrentRunID != v.Runs[1].ID {
		t.Fatalf("manual recovery: %+v", v.Sessions)
	}
	if !v.Workspace.Manual() || v.Workspace.Workflow != nil {
		t.Fatalf("recovery changed the mode: %+v", v.Workspace)
	}
}
