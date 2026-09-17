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

func TestReviewSafeResumePreservesPendingHandoffProvenance(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "review-resume", "planner", nil)
	p, w := startTask(t, s, ws, task)
	h := submitPlan(t, s, ws, p, w)
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}

	resumed, err := s.ResumeSession(ctx, ws, p.ID, "resume-review-once")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID != p.ID || resumed.CurrentRunID == h.FromRun {
		t.Fatalf("resume did not reuse the logical Session with a new Run: %+v", resumed)
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Sessions) != 1 || len(status.Runs) != 2 {
		t.Fatalf("unexpected Session/Run history after resume: sessions=%d runs=%d", len(status.Sessions), len(status.Runs))
	}

	var storedTask Task
	var storedHandoff Handoff
	if err := s.With(ctx, ws, func(d *Document) error {
		foundTask, err := findTask(d, task.ID)
		if err != nil {
			return err
		}
		storedTask = *foundTask
		foundHandoff, err := findHandoff(d, h.ID)
		if err != nil {
			return err
		}
		storedHandoff = *foundHandoff
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if storedTask.State != "awaiting_review" || storedTask.RunID != h.FromRun || storedTask.SessionID != p.ID || storedTask.WorktreeID != w.ID || storedTask.AcceptedHandoff != "" {
		t.Fatalf("review-safe resume changed task binding/provenance: %+v", storedTask)
	}
	if storedHandoff.State != "submitted" || storedHandoff.FromRun != h.FromRun || storedHandoff.Stale {
		t.Fatalf("reviewable handoff was changed by resume: %+v", storedHandoff)
	}

	replayed, err := s.ResumeSession(ctx, ws, p.ID, "resume-review-once")
	if err != nil {
		t.Fatal(err)
	}
	status, err = s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.CurrentRunID != resumed.CurrentRunID || len(status.Runs) != 2 {
		t.Fatalf("resume operation replay created another Run: replay=%+v runs=%d", replayed, len(status.Runs))
	}

	accepted, err := s.ReviewHandoff(ctx, ws, h.ID, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.State != "accepted" {
		t.Fatalf("original handoff could not be accepted after resume: %+v", accepted)
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		current, err := findTask(d, task.ID)
		if err != nil {
			return err
		}
		if current.State != "accepted" || current.AcceptedHandoff != h.ID || current.RunID != h.FromRun {
			return fail("test", "accepted task lost original handoff provenance: %+v", *current)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAwaitingReviewRejectsFreshTaskAssignment(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "review-assignment", "planner", nil)
	first, firstWorktree := startTask(t, s, ws, task)
	_ = submitPlan(t, s, ws, first, firstWorktree)
	secondAgent, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "fresh-review-worker", Role: "planner"})
	if err != nil {
		t.Fatal(err)
	}
	secondWorktree, err := s.CreateWorktree(ctx, ws, WorktreeOptions{Name: "fresh-review-worker", Purpose: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.StartSession(ctx, ws, SessionOptions{Agent: secondAgent.ID, Worktree: secondWorktree.ID, Task: task.ID})
	expectCode(t, err, "task_not_ready")
}

func TestReviewResumeRejectsChangedInputLineage(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "review-input-lineage", "planner", nil)
	p, w := startTask(t, s, ws, task)
	_ = submitPlan(t, s, ws, p, w)
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(status.Directory, status.Workspace.Input.Snapshot), []byte("changed input lineage")); err != nil {
		t.Fatal(err)
	}
	_, err = s.ResumeSession(ctx, ws, p.ID, "resume-changed-input")
	expectCode(t, err, "invalid_resume")
}

func TestRejectedReviewSafeResumePromotesActiveSuccessor(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "review-rejection", "planner", nil)
	p, w := startTask(t, s, ws, task)
	original := submitPlan(t, s, ws, p, w)
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.ResumeSession(ctx, ws, p.ID, "resume-before-rejection")
	if err != nil {
		t.Fatal(err)
	}

	rejected, err := s.ReviewHandoff(ctx, ws, original.ID, false, "Add the missing edge cases")
	if err != nil {
		t.Fatal(err)
	}
	if rejected.State != "rejected" {
		t.Fatalf("handoff was not rejected: %+v", rejected)
	}
	var afterRejection Task
	if err := s.With(ctx, ws, func(d *Document) error {
		found, err := findTask(d, task.ID)
		if err != nil {
			return err
		}
		afterRejection = *found
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if afterRejection.State != "needs_changes" || afterRejection.RunID != resumed.CurrentRunID {
		t.Fatalf("rejection did not promote the compatible active Run: %+v", afterRejection)
	}

	if err := atomicWrite(filepath.Join(w.Path, "work-products", "PLAN.md"), []byte("Replacement plan with edge cases")); err != nil {
		t.Fatal(err)
	}
	replacement, err := s.SubmitHandoff(ctx, ws, HandoffOptions{Session: resumed.ID, Summary: "Replacement plan", Artifacts: []string{"work-products/PLAN.md"}, OperationKey: "replacement:" + resumed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Stale || replacement.FromRun != resumed.CurrentRunID {
		t.Fatalf("active successor could not submit a fresh handoff: %+v", replacement)
	}
	if _, err := s.ReviewHandoff(ctx, ws, replacement.ID, true, ""); err != nil {
		t.Fatal(err)
	}
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
	if artifact.SessionID != p.ID || artifact.RunID != p.CurrentRunID || h.FromRun != p.CurrentRunID || artifact.AgentID != p.AgentID || artifact.SourceHandoff != h.ID {
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
	if inbox[0].FromSession != p.ID || inbox[0].FromRun != p.CurrentRunID {
		t.Fatal("message lost session/run provenance")
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
	workerService.Actor = Actor{AgentID: p.AgentID, SessionID: p.ID, RunID: p.CurrentRunID}
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
