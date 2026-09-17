package core

import (
	"context"
	"testing"
)

// The dedicated complete operation terminates an intentionally manual
// workspace, replays idempotently under its operation key and lets archive
// succeed without a workflow release confirmation.
func TestManualWorkspaceCompletionAndArchive(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	before, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !before.Workspace.Manual() {
		t.Fatalf("fixture is not manual: %+v", before.Workspace)
	}

	// A stale revision guard is rejected before any mutation.
	if _, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: before.Workspace.Revision + 1}); err == nil {
		t.Fatal("stale completion revision was accepted")
	} else {
		expectCode(t, err, "revision_conflict")
	}

	completed, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{Reason: "analysis delivered", ExpectedRevision: before.Workspace.Revision}, "manual-complete")
	if err != nil {
		t.Fatalf("manual completion failed: %v", err)
	}
	if completed.Workspace.Status != "completed" || !completed.Workspace.Manual() || completed.Workspace.Workflow != nil {
		t.Fatalf("completion state: %+v", completed.Workspace)
	}
	if completed.Workspace.Release.UserConfirmed {
		t.Fatal("manual completion recorded a workflow release")
	}
	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !menuHas(menu, "archive") || menuHas(menu, "complete") {
		t.Fatalf("completed manual menu: %+v", menu.Actions)
	}

	// Replaying the same key and payload returns the committed result even
	// though the workspace has advanced past the original revision.
	replay, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{Reason: "analysis delivered", ExpectedRevision: before.Workspace.Revision}, "manual-complete")
	if err != nil {
		t.Fatalf("completion replay failed: %v", err)
	}
	if replay.Workspace.Status != "completed" {
		t.Fatalf("completion replay state: %s", replay.Workspace.Status)
	}

	// Reusing the key with another payload conflicts.
	if _, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{Reason: "another reason", ExpectedRevision: before.Workspace.Revision}, "manual-complete"); err == nil {
		t.Fatal("completion accepted another payload for the same key")
	} else {
		expectCode(t, err, "operation_conflict")
	}

	archived, err := s.Archive(ctx, ws)
	if err != nil {
		t.Fatalf("archiving a completed manual workspace failed: %v", err)
	}
	if archived.Workspace.Status != "archived" {
		t.Fatalf("archive state: %s", archived.Workspace.Status)
	}
}

// A manual workspace cannot be archived before the explicit completion.
func TestManualArchiveRequiresCompletion(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	_, err := s.Archive(ctx, ws)
	expectCode(t, err, "workspace_not_completed")

	if _, err := s.SetPaused(ctx, ws, true); err != nil {
		t.Fatal(err)
	}
	_, err = s.Archive(ctx, ws)
	expectCode(t, err, "workspace_not_completed")
}

// Completion refuses active services, active sessions and non-accepted tasks
// with precise errors, and succeeds once the runtime is quiet.
func TestManualCompletionPreconditions(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()

	if err := s.With(ctx, ws, func(d *Document) error {
		d.Registry.Services = append(d.Registry.Services, BackgroundService{ID: "svc_test", Name: "checkout", CWD: d.Dir, State: "running"})
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	revision := currentRevision(t, s, ws)
	if _, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: revision}); err == nil {
		t.Fatal("completion accepted an active service")
	} else {
		expectCode(t, err, "service_active")
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		d.Registry.Services[0].State = "stopped"
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}

	task, err := s.CreateTask(ctx, ws, TaskSpec{Name: "plan", Title: "Plan", Goal: "Diagnose", Role: "planner", AcceptanceCriteria: []string{"Evidence"}}, "task:plan")
	if err != nil {
		t.Fatal(err)
	}
	agent, worktree := worker(t, s, ws, "precondition")
	session, err := s.StartSession(ctx, ws, SessionOptions{Agent: agent.ID, Worktree: worktree.ID, Task: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	revision = currentRevision(t, s, ws)
	if _, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: revision}); err == nil {
		t.Fatal("completion accepted an active session")
	} else {
		expectCode(t, err, "session_active")
	}

	if _, err := s.StopSession(ctx, ws, session.ID); err != nil {
		t.Fatal(err)
	}
	revision = currentRevision(t, s, ws)
	if _, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: revision}); err == nil {
		t.Fatal("completion accepted a non-accepted task")
	} else {
		expectCode(t, err, "task_incomplete")
	}

	if _, err := s.DeleteTask(ctx, ws, task.ID, "abandon", MutationGuard{ExpectedRevision: revision}); err != nil {
		t.Fatalf("deleting the abandoned task failed: %v", err)
	}
	revision = currentRevision(t, s, ws)
	if _, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: revision}, "manual-complete"); err != nil {
		t.Fatalf("completion after cleanup failed: %v", err)
	}
}

// An agent session must attest that the user explicitly requested completion;
// without the attestation the operation is refused before any precondition.
func TestManualCompletionRequiresUserAttestation(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	p, err := s.StartOrchestrator(ctx, ws, "manual-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	agentService := *s
	agentService.Actor = Actor{AgentID: p.AgentID, SessionID: p.ID, RunID: p.CurrentRunID}
	revision := currentRevision(t, s, ws)

	if _, err := agentService.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: revision}); err == nil {
		t.Fatal("agent completed the workspace without user attestation")
	} else {
		expectCode(t, err, "user_decision_required")
	}
	after, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if after.Workspace.Status != "active" {
		t.Fatalf("refused completion mutated the workspace: %s", after.Workspace.Status)
	}

	// With the attestation the gate is satisfied and the still-active
	// orchestrator session is the next precise refusal.
	if _, err := agentService.CompleteWorkspace(ctx, ws, CompleteOptions{UserConfirmed: true, ExpectedRevision: revision}); err == nil {
		t.Fatal("completion accepted an active session")
	} else {
		expectCode(t, err, "session_active")
	}
}

// Completion is not a workflow operation: selected and pending workflows must
// keep their existing release/selection paths and stay untouched.
func TestCompleteNotApplicableToWorkflowWorkspaces(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	revision := currentRevision(t, s, ws)
	if _, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: revision}); err == nil {
		t.Fatal("workflow workspace was completed manually")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}

	pending, err := s.Create(ctx, CreateOptions{Title: "Pending", Input: "Choose later"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteWorkspace(ctx, pending.Workspace.ID, CompleteOptions{ExpectedRevision: pending.Workspace.Revision}); err == nil {
		t.Fatal("pending workspace was completed manually")
	} else {
		expectCode(t, err, "operation_not_applicable")
	}

	// Workflow archive still requires the confirmed release.
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Status = "completed"
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(ctx, ws); err == nil {
		t.Fatal("workflow archive skipped the release requirement")
	} else {
		expectCode(t, err, "release_required")
	}
}

func currentRevision(t *testing.T, s *Service, ws string) int {
	t.Helper()
	status, err := s.Status(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	return status.Workspace.Revision
}
