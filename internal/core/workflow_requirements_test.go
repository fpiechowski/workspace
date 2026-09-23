package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPlanFirstCapabilitiesRejectUndeclaredGatesAndDependenciesAtCreate(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Plan-first", Input: "Capability test", Workflow: "plan-first"})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID

	_, err = s.CreateTask(ctx, ws, TaskSpec{Title: "Live test", Goal: "Run a live test", Role: "tester", AcceptanceCriteria: []string{"report"}}, "task:tester")
	expectCode(t, err, "workflow_capability")
	_, err = s.PrepareIntegration(ctx, ws, IntegrationOptions{})
	expectCode(t, err, "workflow_capability")

	planner := plannedTask(t, s, ws, "plan", "planner", nil)
	_, err = s.CreateTask(ctx, ws, TaskSpec{Title: "Implementation", Goal: "Implement the plan", Role: "implementer", AcceptanceCriteria: []string{"change"}}, "task:implementation")
	expectCode(t, err, "workflow_gate")
	if _, err := s.CreateTask(ctx, ws, TaskSpec{Title: "Implementation", Goal: "Implement the plan", Role: "implementer", DependsOn: []string{planner.ID}, AcceptanceCriteria: []string{"change"}}, "task:implementation-with-plan"); err != nil {
		t.Fatal(err)
	}
}

func TestRetiredImplementerDoesNotBlockPlanFirstCompletion(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Plan-first", Input: "Retirement test", Workflow: "plan-first"})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	planner := plannedTask(t, s, ws, "plan", "planner", nil)
	p, w := startTask(t, s, ws, planner)
	h := submitPlan(t, s, ws, p, w)
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	impl := plannedTask(t, s, ws, "obsolete", "implementer", []string{planner.ID})
	if _, err := s.SupersedeTask(ctx, ws, impl.ID, "replaced by a smaller implementation", "retire:obsolete"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceWorkflow(ctx, ws, "plan_review", "advance:plan-review"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceWorkflow(ctx, ws, "implementing", "advance:implementing"); err != nil {
		t.Fatal(err)
	}
	status, err := s.AdvanceWorkflow(ctx, ws, "completed", "advance:completed")
	if err != nil {
		t.Fatal(err)
	}
	if status.Workspace.Status != "completed" || status.Workspace.Workflow.Phase != "completed" {
		t.Fatalf("retired implementation did not allow completion: %+v", status.Workspace)
	}
}

func TestExpectedExitChecksAndRequireChecksFalseAreDurable(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := TaskSpec{Name: "blocked-evidence", Title: "Blocked evidence", Goal: "Record an intentional block", Role: "implementer", RequireChecks: false, AcceptanceCriteria: []string{"evidence"}}
	created, err := s.CreateTask(ctx, ws, task, "task:blocked-evidence")
	if err != nil {
		t.Fatal(err)
	}
	if created.RequireChecks {
		t.Fatal("require_checks=false was changed during task creation")
	}
	p, w := startTask(t, s, ws, created)
	if err := atomicWrite(filepath.Join(w.Path, "work-products", "IMPLEMENTATION.md"), []byte("implemented")); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(w.Path, "work-products", "CHECKS.log"), []byte("blocked intentionally\n")); err != nil {
		t.Fatal(err)
	}
	expected := 2
	h, err := s.SubmitHandoff(ctx, ws, HandoffOptions{Session: p.ID, Summary: "Blocked result is expected", Artifacts: []string{"work-products/IMPLEMENTATION.md", "work-products/CHECKS.log"}, Checks: []Check{{Command: "validate target", ExitCode: 2, ExpectedExit: &expected, Evidence: "CHECKS.log"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"IMPLEMENTATION.md", "CHECKS.log"} {
		if err := os.Remove(filepath.Join(w.Path, "work-products", name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatalf("expected exit was rejected: %v", err)
	}
}

func TestHandoffAcceptRechecksAWorktreeAfterDirtySubmit(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "implementation", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	artifact := filepath.Join(w.Path, "work-products", "IMPLEMENTATION.md")
	if err := atomicWrite(artifact, []byte("result")); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(w.Path, ".codex", "live-tests", "marker.txt")
	if err := os.MkdirAll(filepath.Dir(marker), 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(marker, []byte("live evidence")); err != nil {
		t.Fatal(err)
	}
	h, err := s.SubmitHandoff(ctx, ws, HandoffOptions{Session: p.ID, Summary: "Submitted while checkout was dirty", Artifacts: []string{"work-products/IMPLEMENTATION.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if !h.Dirty {
		t.Fatal("fixture handoff did not capture a dirty worktree")
	}
	if err := os.Remove(artifact); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Dir(marker)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatalf("acceptance used stale dirty snapshot: %v", err)
	}
}

func TestCompletedWorkerPaneReleasesParallelSlotAndCanClose(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	runtime, ok := s.Runtime.(*fakeRuntime)
	if !ok {
		t.Fatal("fixture runtime is not the fake runtime")
	}
	var sessions []Session
	for i := 0; i < defaultMaxParallelTasks; i++ {
		a, w := worker(t, s, ws, "slot-worker-"+string(rune('a'+i)))
		p, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID})
		if err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, p)
	}
	runtime.panes[sessions[0].PaneID] = Pane{ID: sessions[0].PaneID, WindowID: "@1", SessionID: sessions[0].ID, RunID: sessions[0].CurrentRunID, Dead: true}
	a, w := worker(t, s, ws, "slot-replacement")
	replacement, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatalf("completed worker retained parallel slot: %v", err)
	}
	runtime.panes[replacement.PaneID] = Pane{ID: replacement.PaneID, WindowID: "@1", SessionID: replacement.ID, RunID: replacement.CurrentRunID, Dead: true}
	closed, err := s.CloseSession(ctx, ws, replacement.ID, "completed", "close:replacement")
	if err != nil {
		t.Fatalf("dead worker session could not close: %v", err)
	}
	if closed.Active() || closed.LifecycleState != "closed" || closed.RunState != "exited" {
		t.Fatalf("closed dead worker retained active lifecycle: %+v", closed)
	}
}

func TestWorkflowMigrationKeepsRetiredTasksAndUsesSelectedTemplate(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Plan-first migration", Input: "Migration test", Workflow: "plan-first"})
	if err != nil {
		t.Fatal(err)
	}
	planner, err := s.CreateTask(ctx, created.Workspace.ID, TaskSpec{Title: "Plan", Goal: "Plan the change", Role: "planner", AcceptanceCriteria: []string{"plan"}}, "migration:planner")
	if err != nil {
		t.Fatal(err)
	}
	impl, err := s.CreateTask(ctx, created.Workspace.ID, TaskSpec{Title: "Old implementation", Goal: "Replace later", Role: "implementer", DependsOn: []string{planner.ID}, AcceptanceCriteria: []string{"change"}}, "migration:impl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SupersedeTask(ctx, created.Workspace.ID, impl.ID, "replacement task created", "migration:retire"); err != nil {
		t.Fatal(err)
	}
	paused, err := s.SetPaused(ctx, created.Workspace.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.MigrateWorkflow(ctx, created.Workspace.ID, RevisionOptions{ExpectedRevision: paused.Workspace.Revision, Reason: "Refresh plan-first template", OperationKey: "migration:plan-first"})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range updated.Workspace.Tasks {
		if task.ID == impl.ID && (task.State != "superseded" || task.Reason != "replacement task created") {
			t.Fatalf("migration revived retired task: %+v", task)
		}
	}
}
