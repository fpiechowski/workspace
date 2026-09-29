package core

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func sessionByID(t *testing.T, status Status, id string) Session {
	t.Helper()
	for _, session := range status.Sessions {
		if session.ID == id {
			return session
		}
	}
	t.Fatalf("session %s not found in status", id)
	return Session{}
}

// submitTaskResult preserves the role-required artifact and submits the handoff
// without accepting it.
func submitTaskResult(t *testing.T, s *Service, ws string, p Session, w Worktree, artifact string) Handoff {
	t.Helper()
	ctx := context.Background()
	for _, name := range []string{artifact, "CHECKS.log"} {
		if err := atomicWrite(filepath.Join(w.Path, "work-products", name), []byte("Verified fixture result\n")); err != nil {
			t.Fatal(err)
		}
	}
	h, err := s.SubmitHandoff(ctx, ws, HandoffOptions{Session: p.ID, Summary: "Completed", Artifacts: []string{"work-products/" + artifact, "work-products/CHECKS.log"}, Checks: []Check{{Command: "fixture checks", ExitCode: 0, Evidence: "CHECKS.log"}}})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// submitAndAcceptTask preserves an artifact and accepts the handoff without
// stopping the worker, so the auto-close path is exercised.
func submitAndAcceptTask(t *testing.T, s *Service, ws string, p Session, w Worktree, artifact string) Handoff {
	t.Helper()
	h := submitTaskResult(t, s, ws, p, w, artifact)
	if _, err := s.ReviewHandoff(context.Background(), ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	return h
}

// 1. Accepting a live worker handoff closes the Session, stops its owned Run,
// frees the pane and leaves the accepted task untouched.
func TestAutoCloseAcceptClosesLiveWorker(t *testing.T) {
	s, ws := manualFixture(t)
	task := plannedTask(t, s, ws, "autoclose-close", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitAndAcceptTask(t, s, ws, p, w, "IMPLEMENTATION.md")

	status := workspaceStatus(t, s, ws)
	got := sessionByID(t, status, p.ID)
	if got.LifecycleState != "closed" || got.ClosedAt == nil || got.CloseReason == "" {
		t.Fatalf("session was not closed: %+v", got)
	}
	if got.Active() {
		t.Fatalf("closed session still owns a live run: %+v", got)
	}
	run, err := findRunInStatus(status, h.FromRun)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != "stopped" {
		t.Fatalf("owned run was not stopped: %+v", run)
	}
	if _, ok := s.Runtime.(*fakeRuntime).panes[p.PaneID]; ok {
		t.Fatal("owned pane was not removed")
	}
	if status.Workspace.Tasks[0].State != "accepted" || status.Workspace.Tasks[0].AcceptedHandoff != h.ID {
		t.Fatalf("accepted task changed: %+v", status.Workspace.Tasks[0])
	}
}

// 2. The issue scenario: after the last implementer is accepted, advancing a
// plan-first v1 workflow to completed must not leave a Session that blocks
// archive. No manual session stop/close is performed.
func TestAutoCloseUnblocksArchiveAfterLastWorkerAccepted(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Plan-first v1 auto-close", Input: "autoclose archive", Workflow: "plan-first"})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Workflow.Capabilities = nil
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}

	plan := plannedTask(t, s, ws, "v1-action-plan", "planner", nil)
	pp, pw := startTask(t, s, ws, plan)
	ph := submitPlan(t, s, ws, pp, pw)
	if _, err := s.ReviewHandoff(ctx, ws, ph.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceWorkflow(ctx, ws, "plan_review", "advance:plan-review"); err != nil {
		t.Fatal(err)
	}
	impl := plannedTask(t, s, ws, "v1-implementation", "implementer", []string{plan.ID})
	if _, err := s.AdvanceWorkflow(ctx, ws, "implementing", "advance:implementing"); err != nil {
		t.Fatal(err)
	}
	ip, iw := startTask(t, s, ws, impl)
	submitAndAcceptTask(t, s, ws, ip, iw, "IMPLEMENTATION.md")

	if _, err := s.AdvanceWorkflow(ctx, ws, "completed", "advance:completed"); err != nil {
		t.Fatalf("advance to completed failed: %v", err)
	}
	if _, err := s.Archive(ctx, ws, "archive:autoclose"); err != nil {
		t.Fatalf("archive was blocked by an accepted worker session: %v", err)
	}
}

// 2b. autonomy report no longer fails with session_active after an accept.
func TestAutoCloseUnblocksAutonomyReport(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Autonomous auto-close", Input: "manual autonomy", NoWorkflow: true, Autonomous: true})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	orch, err := s.StartOrchestrator(ctx, ws, "autoclose-autonomy")
	if err != nil {
		t.Fatal(err)
	}
	agent := *s
	agent.Actor = Actor{AgentID: orch.AgentID, SessionID: orch.ID, RunID: orch.CurrentRunID}

	task := plannedTask(t, s, ws, "autoclose-autonomy-task", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitTaskResult(t, s, ws, p, w, "IMPLEMENTATION.md")
	if _, err := agent.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "task result accepted", nil, false); err != nil {
		t.Fatal(err)
	}
	// The orchestrator accepts without an explicit worker stop; the report must
	// not be blocked by the auto-closed worker.
	if _, err := agent.ReportAutonomy(ctx, ws, AutonomyReportOptions{Outcome: autonomyOutcomeComplete, SummaryFile: writeAutonomySummary(t), ExpectedRevision: currentRevision(t, s, ws)}, "autoclose-report"); err != nil {
		t.Fatalf("autonomy report was blocked by the accepted worker: %v", err)
	}
}

// 3. Accepting a worker releases its parallel slot immediately.
func TestAutoCloseReleasesParallelSlot(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	var first Session
	var firstWorktree Worktree
	for i := 0; i < defaultMaxParallelTasks; i++ {
		task := plannedTask(t, s, ws, fmt.Sprintf("autoclose-slot-%d", i), "planner", nil)
		p, w := startTask(t, s, ws, task)
		if i == 0 {
			first, firstWorktree = p, w
		}
	}
	firstPlan := submitPlan(t, s, ws, first, firstWorktree)
	if _, err := s.ReviewHandoff(ctx, ws, firstPlan.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	replacement, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "autoclose-replacement", Role: "planner"})
	if err != nil {
		t.Fatal(err)
	}
	replacementWorktree, err := s.CreateWorktree(ctx, ws, WorktreeOptions{Name: "autoclose-replacement", Purpose: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartSession(ctx, ws, SessionOptions{Agent: replacement.ID, Worktree: replacementWorktree.ID}); err != nil {
		t.Fatalf("parallel slot was not released: %v", err)
	}
}

// 4. A runtime failure never fails acceptance. The intent and error are durable
// and Reconcile converges once the runtime recovers.
func TestAutoCloseRuntimeFailureIsDurableAndConverges(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "autoclose-runtime", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitTaskResult(t, s, ws, p, w, "IMPLEMENTATION.md")

	runtime := s.Runtime.(*fakeRuntime)
	runtime.inspectError = fail("tmux_error", "server unavailable")
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatalf("acceptance failed because of the runtime: %v", err)
	}
	status := workspaceStatus(t, s, ws)
	got := sessionByID(t, status, p.ID)
	if !got.Active() || got.CloseRequestedAt == nil || got.CloseError == "" {
		t.Fatalf("pending close intent was not durable: %+v", got)
	}

	runtime.inspectError = nil
	if _, err := s.Reconcile(ctx, ws); err != nil {
		t.Fatal(err)
	}
	status = workspaceStatus(t, s, ws)
	got = sessionByID(t, status, p.ID)
	if got.Active() || got.ClosedAt == nil || got.CloseError != "" {
		t.Fatalf("pending close did not converge: %+v", got)
	}
}

// 5. A pane that is not verified as owned is never stopped.
func TestAutoCloseNeverKillsUnownedPane(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "autoclose-unowned", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitTaskResult(t, s, ws, p, w, "IMPLEMENTATION.md")

	runtime := s.Runtime.(*fakeRuntime)
	pane := runtime.panes[p.PaneID]
	pane.SessionID, pane.RunID = "sess_other", "run_other"
	runtime.panes[p.PaneID] = pane

	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	status := workspaceStatus(t, s, ws)
	got := sessionByID(t, status, p.ID)
	if got.ClosedAt != nil || got.CloseError == "" {
		t.Fatalf("unowned pane did not leave a pending error: %+v", got)
	}
	if _, ok := runtime.panes[p.PaneID]; !ok {
		t.Fatal("unowned pane was stopped")
	}
}

// 6. Replaying the accept and settling repeatedly closes exactly once and never
// rewrites ClosedAt.
func TestAutoCloseIsIdempotent(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "autoclose-idempotent", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitTaskResult(t, s, ws, p, w, "IMPLEMENTATION.md")

	accepted, err := s.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "", nil, false, "autoclose-key")
	if err != nil {
		t.Fatal(err)
	}
	status := workspaceStatus(t, s, ws)
	first := sessionByID(t, status, p.ID)
	if first.ClosedAt == nil {
		t.Fatal("session did not close")
	}
	closedAt := *first.ClosedAt

	replay, err := s.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "", nil, false, "autoclose-key")
	if err != nil {
		t.Fatalf("accept replay failed: %v", err)
	}
	if replay.ID != accepted.ID || replay.State != "accepted" {
		t.Fatalf("accept replay changed the result: %+v", replay)
	}
	if err := s.settleClosingSessions(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if err := s.settleClosingSessions(ctx, ws); err != nil {
		t.Fatal(err)
	}
	status = workspaceStatus(t, s, ws)
	second := sessionByID(t, status, p.ID)
	if second.ClosedAt == nil || !second.ClosedAt.Equal(closedAt) {
		t.Fatalf("ClosedAt was rewritten: %v -> %v", closedAt, second.ClosedAt)
	}
	if len(status.Runs) != 1 {
		t.Fatalf("repeated settle created runs: %d", len(status.Runs))
	}
}

// 7. A dead pane closes with the Run exited, and the accepted task is not
// blocked.
func TestAutoCloseDeadPaneExitsRunWithoutBlockingTask(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "autoclose-dead", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitTaskResult(t, s, ws, p, w, "IMPLEMENTATION.md")

	runtime := s.Runtime.(*fakeRuntime)
	runtime.panes[p.PaneID] = Pane{ID: p.PaneID, WindowID: "@1", SessionID: p.ID, RunID: p.CurrentRunID, Dead: true}

	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	status := workspaceStatus(t, s, ws)
	got := sessionByID(t, status, p.ID)
	if got.LifecycleState != "closed" || got.ClosedAt == nil {
		t.Fatalf("dead worker was not closed: %+v", got)
	}
	run, err := findRunInStatus(status, h.FromRun)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != "exited" {
		t.Fatalf("dead pane run state = %s, want exited", run.State)
	}
	if status.Workspace.Tasks[0].State != "accepted" {
		t.Fatalf("accepted task was blocked by the dead pane: %+v", status.Workspace.Tasks[0])
	}
}

// 8. Rejecting a handoff does not close the worker.
func TestAutoCloseRejectLeavesSessionOpen(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "autoclose-reject", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitPlan(t, s, ws, p, w)
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, false, "Add the missing edge cases"); err != nil {
		t.Fatal(err)
	}
	status := workspaceStatus(t, s, ws)
	got := sessionByID(t, status, p.ID)
	if got.ClosedAt != nil || got.CloseRequestedAt != nil || !got.Active() {
		t.Fatalf("rejection changed the worker session: %+v", got)
	}
}

// 9. --keep-session accepts leave the worker active with no intent, use a
// distinct idempotency digest, and still support consultation resume.
func TestAutoCloseKeepSessionOptOut(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "autoclose-keep", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitTaskResult(t, s, ws, p, w, "IMPLEMENTATION.md")

	if _, err := s.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "", nil, true, "autoclose-keep-key"); err != nil {
		t.Fatal(err)
	}
	status := workspaceStatus(t, s, ws)
	got := sessionByID(t, status, p.ID)
	if got.ClosedAt != nil || got.CloseRequestedAt != nil || !got.Active() {
		t.Fatalf("--keep-session did not preserve the worker: %+v", got)
	}
	// A plain accept with the same key has a different digest.
	_, err := s.ReviewHandoff(ctx, ws, h.ID, true, "", "autoclose-keep-key")
	expectCode(t, err, "operation_conflict")

	// A kept Session remains resumable for consultation in a completed workspace.
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Status = "completed"
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.StartSession(ctx, ws, SessionOptions{Agent: p.AgentID, Worktree: p.WorktreeID, Parent: p.ParentAgentID, ParentSessionID: p.ParentSessionID, Profile: p.Profile, Task: p.TaskID, ResumeSession: p.ID, ReadOnly: p.ReadOnly})
	if err != nil {
		t.Fatalf("kept session could not resume: %v", err)
	}
	if resumed.ID != p.ID {
		t.Fatalf("kept session started a new logical Session: %s", resumed.ID)
	}
}

// 10. A closing Session refuses an exact resume, and ResumeAgent starts fresh.
func TestAutoCloseClosingSessionCannotResume(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "autoclose-resume", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitTaskResult(t, s, ws, p, w, "IMPLEMENTATION.md")

	runtime := s.Runtime.(*fakeRuntime)
	runtime.inspectError = fail("tmux_error", "server unavailable")
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	runtime.inspectError = nil
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	status := workspaceStatus(t, s, ws)
	if got := sessionByID(t, status, p.ID); got.CloseRequestedAt == nil {
		t.Fatalf("expected a pending close intent: %+v", got)
	}

	_, err := s.StartSession(ctx, ws, SessionOptions{Agent: p.AgentID, Worktree: p.WorktreeID, Parent: p.ParentAgentID, ParentSessionID: p.ParentSessionID, Profile: p.Profile, Task: p.TaskID, ResumeSession: p.ID, ReadOnly: p.ReadOnly})
	expectCode(t, err, "session_closed")
}

// 10b. ResumeAgent ignores a closing latest Session and starts a fresh one.
func TestAutoCloseResumeAgentStartsFreshSession(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	agent, worktree := worker(t, s, ws, "autoclose-resume-fresh")
	first, err := s.StartSession(ctx, ws, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		session, err := findSession(d, first.ID)
		if err != nil {
			return err
		}
		now := nowUTC()
		session.CloseRequestedAt = &now
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}

	fresh, err := s.ResumeAgent(ctx, ws, agent.ID, "autoclose-resume-fresh-run")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID == first.ID {
		t.Fatalf("ResumeAgent reused a closing session: %s", fresh.ID)
	}
}

// 11. The review message to the closed worker stays queued and is never
// rerouted by a supervisor tick.
func TestAutoCloseReviewMessageToClosedWorkerIsNotRerouted(t *testing.T) {
	s, ws := manualFixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "autoclose-message", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	h := submitTaskResult(t, s, ws, p, w, "IMPLEMENTATION.md")
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}

	var review Message
	if err := s.With(ctx, ws, func(d *Document) error {
		for _, m := range d.Registry.Messages {
			if m.HandoffID == h.ID && m.Kind == "review" {
				review = m
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if review.ID == "" || review.ToSession != p.ID || review.AcknowledgedAt != nil {
		t.Fatalf("review message was not preserved for the closed worker: %+v", review)
	}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var after Message
	if err := s.With(ctx, ws, func(d *Document) error {
		for _, m := range d.Registry.Messages {
			if m.ID == review.ID {
				after = m
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if after.ToSession != p.ID || after.AcknowledgedAt != nil {
		t.Fatalf("supervisor rerouted or delivered the closed worker message: %+v", after)
	}
}
