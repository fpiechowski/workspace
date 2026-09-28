package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func hasMenuAction(menu Menu, id string) bool {
	for _, a := range menu.Actions {
		if a.ID == id {
			return true
		}
	}
	return false
}

func writeAutonomySummary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SUMMARY.md")
	if err := os.WriteFile(path, []byte("# Autonomous summary\n\nNo open questions; assumptions are recorded in the decisions.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTaskCommit(t *testing.T, ctx context.Context, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("implementation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, dir, "add", "fix.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, dir, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fix issue"); err != nil {
		t.Fatal(err)
	}
}

// finishTaskAgent submits a task result and accepts it as the autonomous
// orchestrator with a rationale, then stops the worker so no Run stays active.
func finishTaskAgent(t *testing.T, s, agent *Service, ws string, p Session, w Worktree, artifact, rationale string) Handoff {
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
	if _, err := agent.ReviewHandoffAudited(ctx, ws, h.ID, true, "", rationale, []string{"ck_verified"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	return h
}

// setupAutonomyLanding drives an autonomous plan-first workspace from creation
// to an accepted integrator without a user attestation and returns the pieces a
// report needs.
func setupAutonomyLanding(t *testing.T) (*Service, *Service, string, Session, Worktree, Integration) {
	t.Helper()
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Autonomous landing e2e", Input: "unattended plan-first", Workflow: "plan-first", Autonomous: true})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	if _, err := git(ctx, s.Root, "branch", "release"); err != nil {
		t.Fatal(err)
	}
	orch, err := s.StartOrchestrator(ctx, ws, "autonomy-e2e-"+ID("key"))
	if err != nil {
		t.Fatal(err)
	}
	agent := *s
	agent.Actor = Actor{AgentID: orch.AgentID, SessionID: orch.ID, RunID: orch.CurrentRunID}

	plan := plannedTask(t, s, ws, "e2e-plan", "planner", nil)
	pp, pw := startTask(t, s, ws, plan)
	ph := submitPlanForGate(t, s, ws, pp, pw, "e2e-plan-result")
	if _, err := agent.ReviewHandoffAudited(ctx, ws, ph.ID, true, "", "PLAN maps each criterion", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, pp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.AdvanceWorkflowAudited(ctx, ws, "plan_review", "planner accepted", nil, ""); err != nil {
		t.Fatal(err)
	}
	impl := plannedTask(t, s, ws, "e2e-impl", "implementer", []string{plan.ID})
	if _, err := agent.AdvanceWorkflowAudited(ctx, ws, "implementing", "plan linked", nil, ""); err != nil {
		t.Fatal(err)
	}
	ip, iw := startTask(t, s, ws, impl)
	writeTaskCommit(t, ctx, iw.Path)
	finishTaskAgent(t, s, &agent, ws, ip, iw, "IMPLEMENTATION.md", "evidence covers criteria")
	if _, err := agent.AdvanceWorkflowAudited(ctx, ws, "integration", "implementer accepted", nil, ""); err != nil {
		t.Fatal(err)
	}
	integration, err := agent.PrepareIntegration(ctx, ws, IntegrationOptions{Target: "release", OperationKey: "e2e-integration"})
	if err != nil {
		t.Fatal(err)
	}
	itask := plannedTask(t, s, ws, "e2e-integrator", "integrator", []string{impl.ID})
	ia, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "e2e-integrator", Role: "integrator", PromptTemplate: "implementation"})
	if err != nil {
		t.Fatal(err)
	}
	ip2, err := s.StartSession(ctx, ws, SessionOptions{Agent: ia.ID, Task: itask.ID, Worktree: integration.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	stx, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	var iwork Worktree
	for _, wt := range stx.Worktrees {
		if wt.ID == integration.WorktreeID {
			iwork = wt
		}
	}
	if iwork.ID == "" {
		t.Fatal("integration worktree is missing")
	}
	if _, err := git(ctx, iwork.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "merge", "--no-ff", "--no-edit", integration.Heads[0]); err != nil {
		t.Fatal(err)
	}
	finishTaskAgent(t, s, &agent, ws, ip2, iwork, "INTEGRATION.md", "integrated heads validated")
	return s, &agent, ws, orch, iwork, integration
}

func TestAutonomyEndToEndPlanFirstReportBoundaryAndMenu(t *testing.T) {
	s, agent, ws, orch, _, integration := setupAutonomyLanding(t)
	ctx := context.Background()

	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMenuAction(menu, "report") {
		t.Fatalf("running integration menu lacks the report action: %+v", menu.Actions)
	}
	if hasMenuAction(menu, "land") || hasMenuAction(menu, "complete") {
		t.Fatalf("running integration menu exposed user actions: %+v", menu.Actions)
	}

	// The excluded operations are refused while the run is still running, even
	// with an attestation.
	runningRevision := currentRevision(t, s, ws)
	if _, err := agent.LandIntegration(ctx, ws, LandingOptions{Target: integration.Target, ExpectedRevision: runningRevision, UserConfirmed: true}); err == nil {
		t.Fatal("agent landed while the autonomous run was running")
	} else {
		expectCode(t, err, "autonomy_excluded")
	}
	if _, err := agent.CompleteWorkspace(ctx, ws, CompleteOptions{UserConfirmed: true, ExpectedRevision: runningRevision}); err == nil {
		t.Fatal("agent completed while the autonomous run was running")
	} else {
		expectCode(t, err, "autonomy_excluded")
	}

	summary := writeAutonomySummary(t)
	revision := currentRevision(t, s, ws)
	opt := AutonomyReportOptions{Outcome: autonomyOutcomeLand, SummaryFile: summary, Recommendation: "land into " + integration.Target, ExpectedRevision: revision}
	reported, err := agent.ReportAutonomy(ctx, ws, opt, "e2e-report")
	if err != nil {
		t.Fatal(err)
	}
	if reported.Workspace.Autonomy == nil || reported.Workspace.Autonomy.State != "delivered" {
		t.Fatalf("report did not deliver the run: %+v", reported.Workspace.Autonomy)
	}
	report := reported.Workspace.Autonomy.Report
	if report == nil || report.Outcome != autonomyOutcomeLand || report.IntegrationHead == "" || len(report.ArtifactIDs) == 0 {
		t.Fatalf("report payload incomplete: %+v", report)
	}
	for _, id := range report.ArtifactIDs {
		var artifact *Artifact
		if err := s.With(ctx, ws, func(d *Document) error {
			found, err := findArtifact(d, id)
			if err != nil {
				return err
			}
			artifact = found
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := verifyArtifact(&Document{Dir: reported.Directory}, artifact); err != nil {
			t.Fatalf("stored report artifact %s is not immutable: %v", id, err)
		}
	}
	decision := lastDecision(t, reported, "autonomous.final_report")
	if !decision.Autonomous || decision.ResolvedBy != "orchestrator" || len(decision.Evidence) == 0 {
		t.Fatalf("final report decision: %+v", decision)
	}

	replay, err := agent.ReportAutonomy(ctx, ws, opt, "e2e-report")
	if err != nil || replay.Workspace.Revision != reported.Workspace.Revision {
		t.Fatalf("report replay changed the result: %+v %v", replay.Workspace.Revision, err)
	}

	menu, err = s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMenuAction(menu, "report") || !hasMenuAction(menu, "land") {
		t.Fatalf("delivered menu does not show report plus land: %+v", menu.Actions)
	}

	// After delivery the ordinary attestation contract applies again: an agent
	// without a user confirmation is refused with user_decision_required, not
	// autonomy_excluded.
	revision = currentRevision(t, s, ws)
	if _, err := agent.LandIntegration(ctx, ws, LandingOptions{Target: integration.Target, ExpectedRevision: revision}); err == nil {
		t.Fatal("agent land without an attestation was allowed")
	} else {
		expectCode(t, err, "user_decision_required")
	}

	landed, err := s.LandIntegration(ctx, ws, LandingOptions{Target: integration.Target, ExpectedRevision: revision})
	if err != nil {
		t.Fatalf("user land failed: %v", err)
	}
	if !landingLanded(&Document{State: landed.Workspace}) {
		t.Fatal("user land did not record the landing")
	}
	if _, err := s.StopSession(ctx, ws, orch.ID); err != nil {
		t.Fatal(err)
	}
	completed, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{UserConfirmed: true, ExpectedRevision: currentRevision(t, s, ws), Reason: "user accepted"})
	if err != nil {
		t.Fatalf("user complete failed: %v", err)
	}
	if completed.Workspace.Status != "completed" {
		t.Fatalf("workspace did not complete: %s", completed.Workspace.Status)
	}
}

func TestAutonomyEndToEndManualReport(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Autonomous manual", Input: "unattended manual", NoWorkflow: true, Autonomous: true})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	orch, err := s.StartOrchestrator(ctx, ws, "autonomy-manual-"+ID("key"))
	if err != nil {
		t.Fatal(err)
	}
	agent := *s
	agent.Actor = Actor{AgentID: orch.AgentID, SessionID: orch.ID, RunID: orch.CurrentRunID}

	task := plannedTask(t, s, ws, "manual-task", "implementer", nil)
	p, w := startTask(t, s, ws, task)
	writeTaskCommit(t, ctx, w.Path)
	finishTaskAgent(t, s, &agent, ws, p, w, "IMPLEMENTATION.md", "task result accepted")

	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMenuAction(menu, "report") || hasMenuAction(menu, "complete") {
		t.Fatalf("running manual menu does not show the report: %+v", menu.Actions)
	}

	revision := currentRevision(t, s, ws)
	if _, err := agent.CompleteWorkspace(ctx, ws, CompleteOptions{UserConfirmed: true, ExpectedRevision: revision, Reason: "agent tries"}); err == nil {
		t.Fatal("agent completed the manual workspace while the run was running")
	} else {
		expectCode(t, err, "autonomy_excluded")
	}

	reported, err := agent.ReportAutonomy(ctx, ws, AutonomyReportOptions{Outcome: autonomyOutcomeComplete, SummaryFile: writeAutonomySummary(t), ExpectedRevision: revision}, "manual-report")
	if err != nil {
		t.Fatal(err)
	}
	if reported.Workspace.Autonomy.State != "delivered" || reported.Workspace.Autonomy.Report.Outcome != autonomyOutcomeComplete {
		t.Fatalf("manual report state: %+v", reported.Workspace.Autonomy)
	}

	if _, err := s.StopSession(ctx, ws, orch.ID); err != nil {
		t.Fatal(err)
	}
	completed, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{UserConfirmed: true, ExpectedRevision: currentRevision(t, s, ws), Reason: "user accepted"})
	if err != nil {
		t.Fatalf("user complete failed: %v", err)
	}
	if completed.Workspace.Status != "completed" {
		t.Fatalf("manual workspace did not complete: %s", completed.Workspace.Status)
	}
}

func TestAutonomyReportValidatesOutcomeAgainstState(t *testing.T) {
	t.Run("integration changed after acceptance", func(t *testing.T) {
		s, agent, ws, _, iwork, _ := setupAutonomyLanding(t)
		ctx := context.Background()
		if err := os.WriteFile(filepath.Join(iwork.Path, "after.txt"), []byte("drift\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := git(ctx, iwork.Path, "add", "after.txt"); err != nil {
			t.Fatal(err)
		}
		if _, err := git(ctx, iwork.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "drift after acceptance"); err != nil {
			t.Fatal(err)
		}
		_, err := agent.ReportAutonomy(ctx, ws, AutonomyReportOptions{Outcome: autonomyOutcomeLand, SummaryFile: writeAutonomySummary(t), ExpectedRevision: currentRevision(t, s, ws)}, "drift-report")
		expectCode(t, err, "integration_changed")
	})

	t.Run("unreviewed handoff is refused", func(t *testing.T) {
		s, agent, ws, _ := autonomousOrchestrator(t, "extended")
		ctx := context.Background()
		task := plannedTask(t, s, ws, "pending-task", "planner", nil)
		p, w := startTask(t, s, ws, task)
		submitPlanForGate(t, s, ws, p, w, "pending-result")
		_, err := agent.ReportAutonomy(ctx, ws, AutonomyReportOptions{Outcome: autonomyOutcomeBlocked, SummaryFile: writeAutonomySummary(t), Pending: []string{"review the submitted result"}, ExpectedRevision: currentRevision(t, s, ws)}, "pending-report")
		expectCode(t, err, "handoff_pending")
	})

	t.Run("blocked needs a pending list", func(t *testing.T) {
		s, agent, ws, _ := autonomousOrchestrator(t, "extended")
		ctx := context.Background()
		_, err := agent.ReportAutonomy(ctx, ws, AutonomyReportOptions{Outcome: autonomyOutcomeBlocked, SummaryFile: writeAutonomySummary(t), ExpectedRevision: currentRevision(t, s, ws)}, "blocked-report")
		expectCode(t, err, "pending_required")
	})

	t.Run("invalid outcome", func(t *testing.T) {
		s, agent, ws, _ := autonomousOrchestrator(t, "extended")
		ctx := context.Background()
		_, err := agent.ReportAutonomy(ctx, ws, AutonomyReportOptions{Outcome: "done", SummaryFile: writeAutonomySummary(t), ExpectedRevision: currentRevision(t, s, ws)}, "invalid-report")
		expectCode(t, err, "invalid_outcome")
	})

	t.Run("blocked report delivers", func(t *testing.T) {
		s, agent, ws, _ := autonomousOrchestrator(t, "extended")
		ctx := context.Background()
		st, err := agent.ReportAutonomy(ctx, ws, AutonomyReportOptions{Outcome: autonomyOutcomeBlocked, SummaryFile: writeAutonomySummary(t), Pending: []string{"decision live-testing is pending"}, ExpectedRevision: currentRevision(t, s, ws)}, "blocked-ok")
		if err != nil {
			t.Fatal(err)
		}
		if st.Workspace.Autonomy.State != "delivered" || st.Workspace.Autonomy.Report.Outcome != autonomyOutcomeBlocked {
			t.Fatalf("blocked report state: %+v", st.Workspace.Autonomy)
		}
	})
}

// TestAutonomyBoundaryNotAppliedAfterDelivery shows the exclusions only hold
// while the run is running.
func TestAutonomyBoundaryNotAppliedAfterDelivery(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	if _, err := agent.ReportAutonomy(ctx, ws, AutonomyReportOptions{Outcome: autonomyOutcomeBlocked, SummaryFile: writeAutonomySummary(t), Pending: []string{"pending"}, ExpectedRevision: currentRevision(t, s, ws)}, "after-delivery-report"); err != nil {
		t.Fatal(err)
	}
	_, err := agent.AnswerDecision(ctx, ws, DecisionAnswer{ID: "decision_x", Answer: "run", UserConfirmed: true})
	var e *Error
	if errors.As(err, &e) && e.Code == "autonomy_excluded" {
		t.Fatalf("boundary persisted after delivery: %v", err)
	}
}
