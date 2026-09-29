package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// linkedManualWorkspace creates an Issue and an intentionally manual Workspace
// whose input is frozen to that Issue.
func linkedManualWorkspace(t *testing.T, s *Service, source string) (string, Issue) {
	t.Helper()
	ctx := context.Background()
	opt := IssueCreateOptions{Title: "Linked Issue", Body: "The linked description."}
	if source != "" {
		opt.Source = source
	}
	issue, err := s.IntakeIssue(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.CreateFromIssue(ctx, issue.ID, CreateOptions{NoWorkflow: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.Workspace.Input.IssueID != issue.ID {
		t.Fatalf("workspace is not linked: %+v", status.Workspace.Input)
	}
	return status.Workspace.ID, issue
}

func completeLinkedWorkspace(t *testing.T, s *Service, ws, reason string) Status {
	t.Helper()
	// These domain tests own the orchestrator lifecycle explicitly, so they
	// suppress the T3 post-commit evaluation launch.
	completed, err := s.CompleteWorkspace(context.Background(), ws, CompleteOptions{Reason: reason, ExpectedRevision: currentRevision(t, s, ws), NoEvaluationLaunch: true})
	if err != nil {
		t.Fatalf("completion failed: %v", err)
	}
	return completed
}

func startEvaluationOrchestrator(t *testing.T, s *Service, ws string) (*Service, Session) {
	t.Helper()
	p, err := s.StartOrchestrator(context.Background(), ws, "")
	if err != nil {
		t.Fatalf("starting the orchestrator failed: %v", err)
	}
	agentService := *s
	agentService.Actor = Actor{AgentID: p.AgentID, SessionID: p.ID, RunID: p.CurrentRunID}
	return &agentService, p
}

// evaluationFixture is a completed linked manual Workspace with a pending
// evaluation and a live conversation-only orchestrator actor.
func evaluationFixture(t *testing.T) (s *Service, ws string, issue Issue, agent *Service, p Session) {
	t.Helper()
	s, _ = fixture(t)
	ws, issue = linkedManualWorkspace(t, s, "")
	completeLinkedWorkspace(t, s, ws, "")
	agent, p = startEvaluationOrchestrator(t, s, ws)
	return s, ws, issue, agent, p
}

func TestCompletionMarksPendingEvaluation(t *testing.T) {
	ctx := context.Background()

	t.Run("manual", func(t *testing.T) {
		s, _ := fixture(t)
		ws, issue := linkedManualWorkspace(t, s, "")
		completed := completeLinkedWorkspace(t, s, ws, "")
		eval := completed.Workspace.IssueEvaluation
		if eval == nil || eval.State != "pending" || eval.IssueID != issue.ID {
			t.Fatalf("completion evaluation: %+v", eval)
		}
		if !strings.HasPrefix(eval.ID, "ieval_") || eval.RequestedAt.IsZero() {
			t.Fatalf("evaluation identity: %+v", eval)
		}
	})

	t.Run("unlinked", func(t *testing.T) {
		s, ws := manualFixture(t)
		completed := completeLinkedWorkspace(t, s, ws, "")
		if completed.Workspace.IssueEvaluation != nil {
			t.Fatalf("unlinked completion recorded an evaluation: %+v", completed.Workspace.IssueEvaluation)
		}
	})

	t.Run("plan-first nothing to integrate", func(t *testing.T) {
		s, _ := fixture(t)
		issue, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: "Linked plan-first", Body: "Deliver"})
		if err != nil {
			t.Fatal(err)
		}
		created, err := s.CreateFromIssue(ctx, issue.ID, CreateOptions{Workflow: "plan-first"})
		if err != nil {
			t.Fatal(err)
		}
		ws := created.Workspace.ID
		plan := plannedTask(t, s, ws, "plan", "planner", nil)
		pp, pw := startTask(t, s, ws, plan)
		finishTask(t, s, ws, pp, pw, "PLAN.md")
		advancePhase(t, s, ws, "plan_review")
		impl := plannedTask(t, s, ws, "implementation", "implementer", []string{plan.ID})
		if _, err := s.CancelTask(ctx, ws, impl.ID, "not needed", "retire:impl"); err != nil {
			t.Fatal(err)
		}
		advancePhase(t, s, ws, "implementing")
		advancePhase(t, s, ws, "integration")
		completed := completeLinkedWorkspace(t, s, ws, "No implementation was required")
		if completed.Workspace.IssueEvaluation == nil || completed.Workspace.IssueEvaluation.State != "pending" {
			t.Fatalf("plan-first completion evaluation: %+v", completed.Workspace.IssueEvaluation)
		}
	})

	t.Run("plan-first landed", func(t *testing.T) {
		s, _ := fixture(t)
		if _, err := git(ctx, s.Root, "branch", "release"); err != nil {
			t.Fatal(err)
		}
		issue, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: "Linked plan-first", Body: "Deliver"})
		if err != nil {
			t.Fatal(err)
		}
		created, err := s.CreateFromIssue(ctx, issue.ID, CreateOptions{Workflow: "plan-first"})
		if err != nil {
			t.Fatal(err)
		}
		ws := created.Workspace.ID
		integratePlanFirst(t, s, ws, "release")
		if _, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release"}); err != nil {
			t.Fatal(err)
		}
		completed := completeLinkedWorkspace(t, s, ws, "")
		if completed.Workspace.IssueEvaluation == nil || completed.Workspace.IssueEvaluation.State != "pending" {
			t.Fatalf("landed plan-first completion evaluation: %+v", completed.Workspace.IssueEvaluation)
		}
	})
}

func TestUpdateIssueAuthorityUnchangedForOrchestrator(t *testing.T) {
	s, ws, issue, agent, _ := evaluationFixture(t)
	ctx := context.Background()
	current, err := s.ShowIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = agent.UpdateIssue(ctx, issue.ID, IssueUpdateOptions{Status: "closed", Reason: "agent attempt", ExpectedRevision: current.Revision})
	expectCode(t, err, "forbidden")
	if _, err := s.UpdateIssue(ctx, issue.ID, IssueUpdateOptions{Status: "closed", Reason: "user attempt", ExpectedRevision: current.Revision}); err != nil {
		t.Fatalf("user UpdateIssue regressed: %v", err)
	}
	_ = ws
}

func TestRecordIssueEvaluationOutcomesAndAuthorization(t *testing.T) {
	ctx := context.Background()

	t.Run("open delivered closes", func(t *testing.T) {
		s, ws, issue, agent, _ := evaluationFixture(t)
		recorded, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "All criteria met", ExpectedRevision: currentRevision(t, s, ws)}, "eval:open:delivered")
		if err != nil {
			t.Fatal(err)
		}
		if recorded.Workspace.IssueEvaluation.IssueAction != "closed" || recorded.Workspace.IssueEvaluation.Outcome != "delivered" {
			t.Fatalf("workspace evaluation: %+v", recorded.Workspace.IssueEvaluation)
		}
		got, err := s.ShowIssue(ctx, issue.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "closed" || !strings.HasPrefix(got.StatusReason, "Delivered by workspace "+ws) {
			t.Fatalf("Issue state after delivered: %+v", got)
		}
	})

	t.Run("open not delivered stays open", func(t *testing.T) {
		s, ws, issue, agent, _ := evaluationFixture(t)
		recorded, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "not_delivered", Reason: "One criterion failed", ExpectedRevision: currentRevision(t, s, ws)}, "eval:open:not")
		if err != nil {
			t.Fatal(err)
		}
		if recorded.Workspace.IssueEvaluation.IssueAction != "left_open" {
			t.Fatalf("workspace evaluation: %+v", recorded.Workspace.IssueEvaluation)
		}
		got, err := s.ShowIssue(ctx, issue.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "open" || !strings.HasPrefix(got.StatusReason, "Not delivered by workspace "+ws) {
			t.Fatalf("Issue state after not_delivered: %+v", got)
		}
	})

	t.Run("deferred delivered closes", func(t *testing.T) {
		s, ws, issue, agent, _ := evaluationFixture(t)
		if _, err := s.UpdateIssue(ctx, issue.ID, IssueUpdateOptions{Status: "deferred", Reason: "waiting", ExpectedRevision: issue.Revision}); err != nil {
			t.Fatal(err)
		}
		recorded, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "Delivered later", ExpectedRevision: currentRevision(t, s, ws)}, "eval:deferred:delivered")
		if err != nil {
			t.Fatal(err)
		}
		if recorded.Workspace.IssueEvaluation.IssueAction != "closed" {
			t.Fatalf("workspace evaluation: %+v", recorded.Workspace.IssueEvaluation)
		}
		got, _ := s.ShowIssue(ctx, issue.ID)
		if got.Status != "closed" {
			t.Fatalf("deferred Issue not closed: %+v", got)
		}
	})

	t.Run("deferred not delivered opens", func(t *testing.T) {
		s, ws, issue, agent, _ := evaluationFixture(t)
		if _, err := s.UpdateIssue(ctx, issue.ID, IssueUpdateOptions{Status: "deferred", Reason: "waiting", ExpectedRevision: issue.Revision}); err != nil {
			t.Fatal(err)
		}
		recorded, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "not_delivered", Reason: "Still missing", ExpectedRevision: currentRevision(t, s, ws)}, "eval:deferred:not")
		if err != nil {
			t.Fatal(err)
		}
		if recorded.Workspace.IssueEvaluation.IssueAction != "left_open" {
			t.Fatalf("workspace evaluation: %+v", recorded.Workspace.IssueEvaluation)
		}
		got, _ := s.ShowIssue(ctx, issue.ID)
		if got.Status != "open" {
			t.Fatalf("deferred Issue not opened: %+v", got)
		}
	})

	t.Run("closed by other is unchanged", func(t *testing.T) {
		s, ws, issue, agent, _ := evaluationFixture(t)
		closed, err := s.UpdateIssue(ctx, issue.ID, IssueUpdateOptions{Status: "closed", Reason: "Closed elsewhere", ExpectedRevision: issue.Revision})
		if err != nil {
			t.Fatal(err)
		}
		recorded, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "Our work met it", ExpectedRevision: currentRevision(t, s, ws)}, "eval:other:delivered")
		if err != nil {
			t.Fatal(err)
		}
		if recorded.Workspace.IssueEvaluation.IssueAction != "unchanged_closed" {
			t.Fatalf("workspace evaluation: %+v", recorded.Workspace.IssueEvaluation)
		}
		got, _ := s.ShowIssue(ctx, issue.ID)
		if got.Status != "closed" || got.StatusReason != "Closed elsewhere" || got.Revision != closed.Revision {
			t.Fatalf("Issue changed for a closure owned elsewhere: %+v", got)
		}
	})

	t.Run("orchestrator conversation-only run succeeds", func(t *testing.T) {
		s, ws, _, agent, p := evaluationFixture(t)
		status, err := s.Status(ctx, ws)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, run := range status.Runs {
			if run.ID == p.CurrentRunID {
				found = run.ConversationOnly
			}
		}
		if !found {
			t.Fatal("orchestrator evaluation Run was not conversation-only")
		}
		if _, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "Judged", ExpectedRevision: currentRevision(t, s, ws)}, "eval:conv"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("user terminal succeeds", func(t *testing.T) {
		s, ws, _, _, _ := evaluationFixture(t)
		if _, err := s.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "Recorded at the terminal", ExpectedRevision: currentRevision(t, s, ws)}, "eval:user"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("worker forbidden", func(t *testing.T) {
		s, ws, _, _, _ := evaluationFixture(t)
		injectActiveSession(t, s, ws, "agent_eval_worker", "sess_eval_worker", "run_eval_worker", "planner")
		worker := *s
		worker.Actor = Actor{AgentID: "agent_eval_worker", SessionID: "sess_eval_worker", RunID: "run_eval_worker"}
		_, err := worker.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "worker", ExpectedRevision: currentRevision(t, s, ws)}, "eval:worker")
		expectCode(t, err, "forbidden")
	})

	t.Run("project scope forbidden", func(t *testing.T) {
		s, ws, _, _, _ := evaluationFixture(t)
		project := *s
		project.Actor = Actor{AgentID: "agent_dispatcher", SessionID: "sess_dispatcher", RunID: "run_dispatcher", Scope: "project"}
		_, err := project.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "dispatch", ExpectedRevision: currentRevision(t, s, ws)}, "eval:project")
		expectCode(t, err, "forbidden")
	})

	t.Run("stale run is rejected", func(t *testing.T) {
		s, ws, _, _, p := evaluationFixture(t)
		stale := *s
		stale.Actor = Actor{AgentID: p.AgentID, SessionID: p.ID, RunID: "run_not_current"}
		_, err := stale.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "stale", ExpectedRevision: currentRevision(t, s, ws)}, "eval:stale")
		expectCode(t, err, "stale_actor")
	})
}

func injectActiveSession(t *testing.T, s *Service, ws, agentID, sessionID, runID, role string) {
	t.Helper()
	err := s.With(context.Background(), ws, func(d *Document) error {
		d.Registry.Sessions = append(d.Registry.Sessions, Session{
			ID: sessionID, AgentID: agentID,
			AgentSnapshot:  Agent{ID: agentID, Role: role},
			CurrentRunID:   runID,
			RunState:       "running",
			State:          "running",
			LifecycleState: "active",
		})
		d.Registry.Runs = append(d.Registry.Runs, Run{ID: runID, SessionID: sessionID, State: "running"})
		return saveDocument(d)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecordIssueEvaluationRefusals(t *testing.T) {
	ctx := context.Background()

	t.Run("not completed", func(t *testing.T) {
		s, _ := fixture(t)
		ws, _ := linkedManualWorkspace(t, s, "")
		_, err := s.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "x", ExpectedRevision: currentRevision(t, s, ws)}, "eval:not-completed")
		expectCode(t, err, "workspace_not_completed")
	})

	t.Run("unlinked operation not applicable", func(t *testing.T) {
		s, ws := manualFixture(t)
		completeLinkedWorkspace(t, s, ws, "")
		_, err := s.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "x", ExpectedRevision: currentRevision(t, s, ws)}, "eval:unlinked")
		expectCode(t, err, "operation_not_applicable")
	})

	t.Run("missing evaluation state", func(t *testing.T) {
		s, ws, _, _, _ := evaluationFixture(t)
		if err := s.With(ctx, ws, func(d *Document) error {
			d.State.IssueEvaluation = nil
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
		_, err := s.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "x", ExpectedRevision: currentRevision(t, s, ws)}, "eval:missing")
		expectCode(t, err, "operation_not_applicable")
	})

	t.Run("archived", func(t *testing.T) {
		s, _ := fixture(t)
		ws, _ := linkedManualWorkspace(t, s, "")
		completeLinkedWorkspace(t, s, ws, "")
		if _, err := s.Archive(ctx, ws); err != nil {
			t.Fatal(err)
		}
		_, err := s.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "x", ExpectedRevision: currentRevision(t, s, ws)}, "eval:archived")
		expectCode(t, err, "workspace_archived")
	})

	t.Run("invalid outcome", func(t *testing.T) {
		s, ws, _, agent, _ := evaluationFixture(t)
		_, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "maybe", Reason: "x", ExpectedRevision: currentRevision(t, s, ws)}, "eval:bad-outcome")
		expectCode(t, err, "invalid_outcome")
	})

	t.Run("reason required", func(t *testing.T) {
		s, ws, _, agent, _ := evaluationFixture(t)
		_, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "   ", ExpectedRevision: currentRevision(t, s, ws)}, "eval:no-reason")
		expectCode(t, err, "reason_required")
	})

	t.Run("revision conflict", func(t *testing.T) {
		s, ws, _, agent, _ := evaluationFixture(t)
		_, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "x", ExpectedRevision: currentRevision(t, s, ws) + 1}, "eval:revision")
		expectCode(t, err, "revision_conflict")
	})

	t.Run("already recorded with different outcome", func(t *testing.T) {
		s, ws, _, agent, _ := evaluationFixture(t)
		if _, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "done", ExpectedRevision: currentRevision(t, s, ws)}, "eval:recorded:1"); err != nil {
			t.Fatal(err)
		}
		_, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "not_delivered", Reason: "changed", ExpectedRevision: currentRevision(t, s, ws)}, "eval:recorded:2")
		expectCode(t, err, "issue_evaluation_recorded")
	})

	t.Run("already recorded same outcome is idempotent", func(t *testing.T) {
		s, ws, issue, agent, _ := evaluationFixture(t)
		first, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "done", ExpectedRevision: currentRevision(t, s, ws)}, "eval:same:1")
		if err != nil {
			t.Fatal(err)
		}
		afterFirst, _ := s.ShowIssue(ctx, issue.ID)
		again, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "done", ExpectedRevision: first.Workspace.Revision}, "eval:same:2")
		if err != nil {
			t.Fatal(err)
		}
		afterSecond, _ := s.ShowIssue(ctx, issue.ID)
		if afterSecond.Revision != afterFirst.Revision {
			t.Fatalf("idempotent re-record wrote the Issue again: %d -> %d", afterFirst.Revision, afterSecond.Revision)
		}
		if again.Workspace.IssueEvaluation.State != "recorded" {
			t.Fatalf("evaluation state: %+v", again.Workspace.IssueEvaluation)
		}
	})
}

func TestRecordIssueEvaluationStaleDigestGuard(t *testing.T) {
	ctx := context.Background()
	s, _ := fixture(t)
	ws, issue := linkedManualWorkspace(t, s, "https://tracker.example/eval/1")
	completeLinkedWorkspace(t, s, ws, "")
	agent, _ := startEvaluationOrchestrator(t, s, ws)

	revised, err := s.IntakeIssue(ctx, IssueCreateOptions{Source: issue.Source, Title: issue.Title, Body: "Requirements changed after the snapshot."})
	if err != nil {
		t.Fatal(err)
	}
	if revised.Revision != issue.Revision+1 {
		t.Fatalf("expected a content revision, got %d", revised.Revision)
	}

	_, err = agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "looks done", ExpectedRevision: currentRevision(t, s, ws)}, "eval:stale:delivered")
	expectCode(t, err, "issue_revised")

	unchanged, err := s.ShowIssue(ctx, issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Status != "open" {
		t.Fatalf("refused delivered changed the Issue: %+v", unchanged)
	}

	recorded, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "not_delivered", Reason: "requirements moved", ExpectedRevision: currentRevision(t, s, ws)}, "eval:stale:not")
	if err != nil {
		t.Fatalf("not_delivered after a revision was refused: %v", err)
	}
	if recorded.Workspace.IssueEvaluation.IssueAction != "left_open" {
		t.Fatalf("workspace evaluation: %+v", recorded.Workspace.IssueEvaluation)
	}
}

func TestRecordIssueEvaluationIdempotencyAndConvergence(t *testing.T) {
	ctx := context.Background()

	t.Run("replay and conflict", func(t *testing.T) {
		s, ws, issue, agent, _ := evaluationFixture(t)
		opt := IssueEvaluationOptions{Outcome: "delivered", Reason: "Met every criterion", ExpectedRevision: currentRevision(t, s, ws)}
		first, err := agent.RecordIssueEvaluation(ctx, ws, opt, "eval:replay")
		if err != nil {
			t.Fatal(err)
		}
		afterFirst, _ := s.ShowIssue(ctx, issue.ID)
		replay, err := agent.RecordIssueEvaluation(ctx, ws, opt, "eval:replay")
		if err != nil {
			t.Fatal(err)
		}
		afterReplay, _ := s.ShowIssue(ctx, issue.ID)
		if afterReplay.Revision != afterFirst.Revision {
			t.Fatalf("replay wrote a new Issue revision: %d -> %d", afterFirst.Revision, afterReplay.Revision)
		}
		if replay.Workspace.Revision != first.Workspace.Revision {
			t.Fatalf("replay advanced the workspace revision: %d -> %d", first.Workspace.Revision, replay.Workspace.Revision)
		}
		_, err = agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "not_delivered", Reason: "different", ExpectedRevision: opt.ExpectedRevision}, "eval:replay")
		expectCode(t, err, "operation_conflict")
	})

	t.Run("crash after the Issue write converges", func(t *testing.T) {
		s, ws, issue, agent, _ := evaluationFixture(t)
		opt := IssueEvaluationOptions{Outcome: "delivered", Reason: "Met every criterion", ExpectedRevision: currentRevision(t, s, ws)}
		before, _ := s.ShowIssue(ctx, issue.ID)

		flushDocumentTestHook = func(*Document) error { return errors.New("injected save failure") }
		_, err := agent.RecordIssueEvaluation(ctx, ws, opt, "eval:crash")
		flushDocumentTestHook = nil
		if err == nil {
			t.Fatal("injected save failure was not reported")
		}

		mid, _ := s.ShowIssue(ctx, issue.ID)
		if mid.Revision != before.Revision+1 || mid.Status != "closed" {
			t.Fatalf("Issue write did not survive the crash: %+v", mid)
		}
		pending, _ := s.Status(ctx, ws)
		if pending.Workspace.IssueEvaluation.State != "pending" {
			t.Fatalf("workspace recorded the evaluation despite the failed flush: %+v", pending.Workspace.IssueEvaluation)
		}

		retried, err := agent.RecordIssueEvaluation(ctx, ws, opt, "eval:crash")
		if err != nil {
			t.Fatalf("retry after the crash failed: %v", err)
		}
		after, _ := s.ShowIssue(ctx, issue.ID)
		if after.Revision != before.Revision+1 || after.Status != "closed" {
			t.Fatalf("retry wrote a second Issue revision: before=%d after=+%+v", before.Revision, after)
		}
		if retried.Workspace.IssueEvaluation.State != "recorded" || retried.Workspace.IssueEvaluation.Outcome != "delivered" {
			t.Fatalf("retry did not converge: %+v", retried.Workspace.IssueEvaluation)
		}
	})

	t.Run("no deadlock under the lock timeout", func(t *testing.T) {
		s, ws, _, agent, _ := evaluationFixture(t)
		lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		start := time.Now()
		if _, err := agent.RecordIssueEvaluation(lockCtx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "No deadlock", ExpectedRevision: currentRevision(t, s, ws)}, "eval:deadlock"); err != nil {
			t.Fatalf("record under a bounded context failed: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("record exceeded the lock timeout: %s", elapsed)
		}
	})
}

func TestIssueEvaluationReopenCycle(t *testing.T) {
	ctx := context.Background()
	s, _ := fixture(t)
	ws, issue := linkedManualWorkspace(t, s, "")
	completeLinkedWorkspace(t, s, ws, "")
	agent, p := startEvaluationOrchestrator(t, s, ws)

	first, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "delivered", Reason: "Delivered", ExpectedRevision: currentRevision(t, s, ws)}, "eval:cycle:1")
	if err != nil {
		t.Fatal(err)
	}
	closed, _ := s.ShowIssue(ctx, issue.ID)
	if closed.Status != "closed" {
		t.Fatalf("first delivered did not close the Issue: %+v", closed)
	}
	firstID := first.Workspace.IssueEvaluation.ID

	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "More work is needed", ExpectedRevision: currentRevision(t, s, ws)}, "reopen:cycle")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Workspace.IssueEvaluation != nil {
		t.Fatalf("reopen did not clear the evaluation: %+v", reopened.Workspace.IssueEvaluation)
	}

	recompleted := completeLinkedWorkspace(t, s, ws, "More work completed")
	if recompleted.Workspace.IssueEvaluation == nil || recompleted.Workspace.IssueEvaluation.ID == firstID || recompleted.Workspace.IssueEvaluation.State != "pending" {
		t.Fatalf("re-completion did not create a fresh pending evaluation: %+v", recompleted.Workspace.IssueEvaluation)
	}

	agent2, _ := startEvaluationOrchestrator(t, s, ws)
	recorded, err := agent2.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "not_delivered", Reason: "A regression was found", ExpectedRevision: currentRevision(t, s, ws)}, "eval:cycle:2")
	if err != nil {
		t.Fatal(err)
	}
	if recorded.Workspace.IssueEvaluation.IssueAction != "reopened" {
		t.Fatalf("not_delivered after this workspace's delivered did not reopen: %+v", recorded.Workspace.IssueEvaluation)
	}
	again, _ := s.ShowIssue(ctx, issue.ID)
	if again.Status != "open" || !strings.HasPrefix(again.StatusReason, "Not delivered by workspace "+ws) {
		t.Fatalf("Issue state after reopen cycle: %+v", again)
	}
}

func TestIssueEvaluationStaleDigestAllowsNotDelivered(t *testing.T) {
	ctx := context.Background()
	s, _ := fixture(t)
	ws, issue := linkedManualWorkspace(t, s, "https://tracker.example/eval/2")
	completeLinkedWorkspace(t, s, ws, "")
	agent, _ := startEvaluationOrchestrator(t, s, ws)

	if _, err := s.IntakeIssue(ctx, IssueCreateOptions{Source: issue.Source, Title: issue.Title, Body: "New body"}); err != nil {
		t.Fatal(err)
	}
	recorded, err := agent.RecordIssueEvaluation(ctx, ws, IssueEvaluationOptions{Outcome: "not_delivered", Reason: "Snapshot no longer matches", ExpectedRevision: currentRevision(t, s, ws)}, "eval:stale-allow")
	if err != nil {
		t.Fatal(err)
	}
	if recorded.Workspace.IssueEvaluation.IssueAction != "left_open" {
		t.Fatalf("workspace evaluation: %+v", recorded.Workspace.IssueEvaluation)
	}
}
