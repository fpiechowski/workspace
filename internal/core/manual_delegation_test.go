package core

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func manualFixture(t *testing.T) (*Service, string) {
	t.Helper()
	s, _ := fixture(t)
	status, err := s.Create(context.Background(), CreateOptions{Title: "Manual", Input: "Coordinate manually", NoWorkflow: true})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Workspace.Manual() || status.Workspace.Workflow != nil {
		t.Fatalf("fixture is not a manual workspace: %+v", status.Workspace)
	}
	return s, status.Workspace.ID
}

// An orchestrator can create task/agent/worktree and start a task-bound worker
// in an intentional manual workspace, then submit and accept a handoff without
// ever creating or advancing a workflow.
func TestManualWorkspaceDelegationEndToEnd(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()

	task, err := s.CreateTask(ctx, ws, TaskSpec{Name: "plan", Title: "Plan", Goal: "Diagnose", Role: "planner", AcceptanceCriteria: []string{"Evidence"}}, "task:plan")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "plan", Role: "planner"})
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := s.CreateWorktree(ctx, ws, WorktreeOptions{Name: "plan", Purpose: "planning"})
	if err != nil {
		t.Fatal(err)
	}

	session, err := s.StartSession(ctx, ws, SessionOptions{Agent: agent.ID, Worktree: worktree.ID, Task: task.ID})
	if err != nil {
		t.Fatalf("manual delegation failed: %v", err)
	}
	if session.TaskID != task.ID || session.WorktreeID != worktree.ID {
		t.Fatalf("worker binding lost: task=%q worktree=%q", session.TaskID, session.WorktreeID)
	}
	if session.ParentAgentID == "" {
		t.Fatal("worker was not parented to the orchestrator")
	}

	if err := atomicWrite(filepath.Join(worktree.Path, "work-products", "PLAN.md"), []byte("An actionable plan")); err != nil {
		t.Fatal(err)
	}
	h, err := s.SubmitHandoff(ctx, ws, HandoffOptions{Session: session.ID, Summary: "Plan prepared", Artifacts: []string{"work-products/PLAN.md"}, OperationKey: "result:1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}

	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if status.Workspace.Workflow != nil {
		t.Fatalf("manual delegation created a workflow: %+v", status.Workspace.Workflow)
	}
	if status.Workspace.Tasks[0].State != "accepted" {
		t.Fatalf("handoff was not accepted: %s", status.Workspace.Tasks[0].State)
	}

	if _, err := s.AdvanceWorkflow(ctx, ws, "", "manual-advance"); err == nil {
		t.Fatal("workflow advance must stay unavailable for a manual workspace")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}
	status, err = s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if status.Workspace.Workflow != nil || status.Workspace.Status != "active" {
		t.Fatal("workflow advance mutated a manual workspace")
	}
}

// The same delegation path stays blocked while the creation choice is pending.
func TestNeedsWorkflowBlocksWorkerDelegation(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	pending, err := s.Create(ctx, CreateOptions{Title: "Pending", Input: "Choose later"})
	if err != nil {
		t.Fatal(err)
	}
	ws := pending.Workspace.ID
	agent, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "plan", Role: "planner"})
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := s.CreateWorktree(ctx, ws, WorktreeOptions{Name: "plan", Purpose: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.StartSession(ctx, ws, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	expectCode(t, err, "decision_required")
}

// Manual delegation uses the fixed fallback of three and never becomes
// unbounded.
func TestManualWorkspaceConcurrencyFallback(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	for i := 0; i < defaultMaxParallelTasks; i++ {
		name := fmt.Sprintf("worker-%d", i)
		agent, err := s.CreateAgent(ctx, ws, AgentOptions{Name: name, Role: "planner"})
		if err != nil {
			t.Fatal(err)
		}
		worktree, err := s.CreateWorktree(ctx, ws, WorktreeOptions{Name: name, Purpose: "planning"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.StartSession(ctx, ws, SessionOptions{Agent: agent.ID, Worktree: worktree.ID}); err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	agent, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "overflow", Role: "planner"})
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := s.CreateWorktree(ctx, ws, WorktreeOptions{Name: "overflow", Purpose: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.StartSession(ctx, ws, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	expectCode(t, err, "parallel_limit")
}

// Without a workflow profile mapping, task and agent defaults still select a
// usable profile.
func TestManualProfileFallbacks(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()

	task, err := s.CreateTask(ctx, ws, TaskSpec{Title: "Plan", Goal: "Diagnose", Role: "planner", AcceptanceCriteria: []string{"Evidence"}}, "task")
	if err != nil {
		t.Fatal(err)
	}
	if task.Profile != "frontier" {
		t.Fatalf("task profile fallback: got %q", task.Profile)
	}
	agent, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "impl", Role: "implementer"})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Profile != "implementation" {
		t.Fatalf("agent profile fallback: got %q", agent.Profile)
	}
	custom, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "custom", Role: "implementer", Profile: "live-testing"})
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := s.CreateWorktree(ctx, ws, WorktreeOptions{Name: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.StartSession(ctx, ws, SessionOptions{Agent: custom.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	if session.Profile != "live-testing" {
		t.Fatalf("session agent fallback: got %q", session.Profile)
	}
}
