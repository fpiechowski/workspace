package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func plannedTask(t *testing.T, s *Service, ws, name, role string, deps []string) Task {
	t.Helper()
	v, err := s.CreateTask(context.Background(), ws, TaskSpec{Name: name, Title: name, Goal: "Bounded test task", Role: role, DependsOn: deps, AcceptanceCriteria: []string{"Produce an inspected result"}}, "task:"+name)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func startTask(t *testing.T, s *Service, ws string, task Task) (Session, Worktree) {
	t.Helper()
	a, err := s.CreateAgent(context.Background(), ws, AgentOptions{Name: task.Name, Role: task.Role})
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.CreateWorktree(context.Background(), ws, WorktreeOptions{Name: task.Name})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartSession(context.Background(), ws, SessionOptions{Agent: a.ID, Worktree: w.ID, Task: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	return p, w
}
func submitPlan(t *testing.T, s *Service, ws string, p Session, w Worktree) Handoff {
	t.Helper()
	if err := atomicWrite(filepath.Join(w.Path, "work-products", "PLAN.md"), []byte("An actionable plan")); err != nil {
		t.Fatal(err)
	}
	h, err := s.SubmitHandoff(context.Background(), ws, HandoffOptions{Session: p.ID, Summary: "Plan prepared", Artifacts: []string{"work-products/PLAN.md"}, OperationKey: "result:" + p.ID})
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func TestHandoffPreservesArtifactBeforeInboxAndRequiresAcceptance(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "plan", "planner", nil)
	p, w := startTask(t, s, ws, task)
	h := submitPlan(t, s, ws, p, w)
	replay := submitPlan(t, s, ws, p, w)
	if h.ID != replay.ID {
		t.Fatal("duplicated handoff")
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Workspace.Artifacts) != 1 || status.Workspace.Tasks[0].State != "awaiting_review" {
		t.Fatal("submission must not accept task")
	}
	artifact := status.Workspace.Artifacts[0]
	if artifact.SessionID != p.ID || artifact.AgentID != p.AgentID || artifact.SourceHandoff != h.ID {
		t.Fatal("missing provenance")
	}
	if err := os.Remove(filepath.Join(w.Path, "work-products", "PLAN.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(status.Directory, artifact.Path)); err != nil {
		t.Fatal("local deletion lost preserved artifact")
	}
	inbox, err := s.Inbox(ctx, ws, "", false)
	if err != nil || len(inbox) != 1 {
		t.Fatalf("inbox: %v %v", inbox, err)
	}
	if _, err := s.ReadMessage(ctx, ws, inbox[0].ID, true); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status(ctx, ws)
	if status.Workspace.Tasks[0].State == "accepted" {
		t.Fatal("ACK accepted the task")
	}
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status(ctx, ws)
	if status.Workspace.Tasks[0].State != "accepted" {
		t.Fatal("task not accepted")
	}
}
func TestDependenciesRetriesAndStaleResults(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	plan := plannedTask(t, s, ws, "plan", "planner", nil)
	p, w := startTask(t, s, ws, plan)
	impl := plannedTask(t, s, ws, "impl", "implementer", []string{plan.ID})
	a, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "impl", Role: "implementer"})
	if err != nil {
		t.Fatal(err)
	}
	wt, err := s.CreateWorktree(ctx, ws, WorktreeOptions{Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: wt.ID, Task: impl.ID})
	expectCode(t, err, "dependency_pending")
	h := submitPlan(t, s, ws, p, w)
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetryTask(ctx, ws, plan.ID, "changed requirements", "retry:1"); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx, ws)
	if status.Workspace.Tasks[0].Attempt != 2 || status.Workspace.Tasks[1].Attempt != 2 {
		t.Fatal("dependent attempts were not invalidated")
	}
	stale, err := s.SubmitHandoff(ctx, ws, HandoffOptions{Session: p.ID, Summary: "Late response", Artifacts: []string{"work-products/PLAN.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Stale {
		t.Fatal("late result was not marked stale")
	}
	_, err = s.ReviewHandoff(ctx, ws, stale.ID, true, "")
	expectCode(t, err, "stale_handoff")
}
func TestWorkersCannotAcceptResultsOrReadOtherInboxes(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "plan", "planner", nil)
	p, w := startTask(t, s, ws, task)
	h := submitPlan(t, s, ws, p, w)
	workerService := *s
	workerService.Actor = Actor{p.AgentID, p.ID}
	_, err := workerService.ReviewHandoff(ctx, ws, h.ID, true, "")
	expectCode(t, err, "forbidden")
	_, err = workerService.Inbox(ctx, ws, "orchestrator", false)
	expectCode(t, err, "forbidden")
	msg, err := workerService.SendMessage(ctx, ws, MessageOptions{To: "orchestrator", Kind: "question", Body: "Which edge cases?", OperationKey: "question:1"})
	if err != nil {
		t.Fatal(err)
	}
	if msg.FromAgent != p.AgentID || msg.FromSession != p.ID {
		t.Fatal("wrong sender")
	}
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	_, err = workerService.SendMessage(ctx, ws, MessageOptions{To: "orchestrator", Body: "old worker"})
	expectCode(t, err, "stale_actor")
}
func TestArtifactsRejectEscapesAndDetectTampering(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "plan", "planner", nil)
	p, w := startTask(t, s, ws, task)
	_, err := s.SubmitHandoff(ctx, ws, HandoffOptions{Session: p.ID, Summary: "escape", Artifacts: []string{"../outside.txt"}})
	expectCode(t, err, "artifact_invalid")
	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "secret.txt")
		if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(w.Path, "escape.txt")); err != nil {
			t.Fatal(err)
		}
		_, err = s.SubmitHandoff(ctx, ws, HandoffOptions{Session: p.ID, Summary: "escape", Artifacts: []string{"escape.txt"}})
		expectCode(t, err, "artifact_invalid")
	}
	h := submitPlan(t, s, ws, p, w)
	status, _ := s.Status(ctx, ws)
	if err := os.WriteFile(filepath.Join(status.Directory, status.Workspace.Artifacts[0].Path), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = s.ReviewHandoff(ctx, ws, h.ID, true, "")
	expectCode(t, err, "artifact_changed")
}
func TestInboxWaitReleasesProjectLock(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		messages, err := s.WaitInbox(ctx, ws, "", 3*time.Second)
		if err == nil && len(messages) != 1 {
			err = fail("test", "missing message")
		}
		done <- err
	}()
	if _, err := s.SendMessage(ctx, ws, MessageOptions{To: "orchestrator", Body: "Wake up", OperationKey: "wake"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("wait held project lock")
	}
}
