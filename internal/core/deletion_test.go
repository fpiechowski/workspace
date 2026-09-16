package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteWorkspaceDiscardsActiveWorkspaceAndReplaysAfterRemoval(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	status, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteWorkspace(ctx, id, "delete-empty", status.Workspace.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Status(ctx, id); err == nil {
		t.Fatal("deleted workspace is still readable")
	}
	if err := s.DeleteWorkspace(ctx, id, "delete-empty", status.Workspace.Revision); err != nil {
		t.Fatalf("project-scoped receipt did not replay after removal: %v", err)
	}

	created, err := s.Create(ctx, CreateOptions{Title: "Has work", Input: "Keep task history", Workflow: "issue-resolution"})
	if err != nil {
		t.Fatal(err)
	}
	agent, worktree := worker(t, s, created.Workspace.ID, "discarded")
	if err := os.WriteFile(filepath.Join(worktree.Path, "uncommitted.txt"), []byte("discard me"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartSession(ctx, created.Workspace.ID, SessionOptions{Agent: agent.ID, Worktree: worktree.ID}); err != nil {
		t.Fatal(err)
	}
	status, err = s.Status(ctx, created.Workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteWorkspace(ctx, created.Workspace.ID, "delete-active", status.Workspace.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worktree.Path); !os.IsNotExist(err) {
		t.Fatalf("discarded worktree still exists: %v", err)
	}
	if exists, err := localBranchExists(ctx, s.Root, worktree.Branch); err != nil || exists {
		t.Fatalf("discarded workspace branch remains: exists=%t err=%v", exists, err)
	}
	if panes := s.Runtime.(*fakeRuntime).panes; len(panes) != 0 {
		t.Fatalf("discarded workspace runtime remains: %+v", panes)
	}
	if err := s.DeleteWorkspace(ctx, created.Workspace.ID, "delete-active", status.Workspace.Revision); err != nil {
		t.Fatalf("active workspace deletion did not replay: %v", err)
	}
}

func TestDeleteSessionAndTaskCreateAuditableTombstones(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	task, err := s.CreateTask(ctx, id, TaskSpec{Title: "Disposable plan", Goal: "Explore an option", Role: "planner", AcceptanceCriteria: []string{"Report findings"}}, "task")
	if err != nil {
		t.Fatal(err)
	}
	agent, worktree := worker(t, s, id, "disposable")
	session, err := s.StartSession(ctx, id, SessionOptions{Agent: agent.ID, Worktree: worktree.ID, Task: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, id, session.ID); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := findSessionInStatus(status, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	deletedSession, err := s.DeleteSession(ctx, id, session.ID, "delete-session", MutationGuard{ExpectedRevision: status.Workspace.Revision, ExpectedRunID: stopped.LastRunID})
	if err != nil {
		t.Fatal(err)
	}
	if deletedSession.DeletedAt == nil || deletedSession.ClosedAt == nil {
		t.Fatalf("session was not tombstoned: %+v", deletedSession)
	}
	status, err = s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	currentTask := status.Workspace.Tasks[0]
	if currentTask.State != "pending" || currentTask.SessionID != "" || currentTask.RunID != "" {
		t.Fatalf("deleting the current session did not reset its task: %+v", currentTask)
	}
	deletedTask, err := s.DeleteTask(ctx, id, task.ID, "delete-task", MutationGuard{ExpectedRevision: status.Workspace.Revision, ExpectedAttempt: task.Attempt})
	if err != nil {
		t.Fatal(err)
	}
	if deletedTask.DeletedAt == nil || deletedTask.State != "deleted" {
		t.Fatalf("task was not tombstoned: %+v", deletedTask)
	}
	if replay, err := s.DeleteTask(ctx, id, task.ID, "delete-task", MutationGuard{ExpectedRevision: status.Workspace.Revision, ExpectedAttempt: task.Attempt}); err != nil || replay.DeletedAt == nil {
		t.Fatalf("task deletion did not replay: %+v %v", replay, err)
	}
}

func TestDeleteTaskRefusesDependentsAndActiveSessions(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	base, err := s.CreateTask(ctx, id, TaskSpec{Title: "Base", Goal: "Provide input", Role: "planner", AcceptanceCriteria: []string{"Input exists"}}, "base")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTask(ctx, id, TaskSpec{Title: "Dependent", Goal: "Use input", Role: "planner", DependsOn: []string{base.ID}, AcceptanceCriteria: []string{"Result exists"}}, "dependent"); err != nil {
		t.Fatal(err)
	}
	_, err = s.DeleteTask(ctx, id, base.ID, "delete-base", MutationGuard{})
	expectCode(t, err, "task_delete_refused")

	standalone, err := s.CreateTask(ctx, id, TaskSpec{Title: "Running", Goal: "Run now", Role: "planner", AcceptanceCriteria: []string{"Done"}}, "running")
	if err != nil {
		t.Fatal(err)
	}
	agent, worktree := worker(t, s, id, "running")
	if _, err := s.StartSession(ctx, id, SessionOptions{Agent: agent.ID, Worktree: worktree.ID, Task: standalone.ID}); err != nil {
		t.Fatal(err)
	}
	_, err = s.DeleteTask(ctx, id, standalone.ID, "delete-running", MutationGuard{})
	expectCode(t, err, "task_delete_refused")
}
