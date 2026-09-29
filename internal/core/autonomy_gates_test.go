package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func strptr(s string) *string { return &s }

// autonomousOrchestrator creates an autonomous workspace and returns a Service
// whose Actor is the workspace orchestrator's active Run.
func autonomousOrchestrator(t *testing.T, workflow string) (*Service, *Service, string, Session) {
	t.Helper()
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()
	opt := CreateOptions{Title: "Autonomous gates", Input: "unattended gates", Autonomous: true}
	if workflow == "" {
		opt.NoWorkflow = true
	} else {
		opt.Workflow = workflow
	}
	created, err := s.Create(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	orch, err := s.StartOrchestrator(ctx, ws, "autonomy-gates-"+ID("key"))
	if err != nil {
		t.Fatal(err)
	}
	agent := *s
	agent.Actor = Actor{AgentID: orch.AgentID, SessionID: orch.ID, RunID: orch.CurrentRunID}
	return s, &agent, ws, orch
}

func workspaceStatus(t *testing.T, s *Service, ws string) Status {
	t.Helper()
	v, err := s.Status(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func submitPlanForGate(t *testing.T, s *Service, ws string, p Session, w Worktree, key string) Handoff {
	t.Helper()
	if err := atomicWrite(filepath.Join(w.Path, "work-products", "PLAN.md"), []byte("An actionable plan")); err != nil {
		t.Fatal(err)
	}
	h, err := s.SubmitHandoff(context.Background(), ws, HandoffOptions{Session: p.ID, Summary: "Plan prepared", Artifacts: []string{"work-products/PLAN.md"}, OperationKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestAutonomyHandoffRationaleAndPlanDecision(t *testing.T) {
	s, agent, ws, orch := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	plan := plannedTask(t, s, ws, "gate-plan", "planner", nil)
	p, w := startTask(t, s, ws, plan)
	h := submitPlanForGate(t, s, ws, p, w, "gate-plan-result")

	if _, err := agent.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "", nil, false); err == nil {
		t.Fatal("accept without rationale was allowed under autonomy")
	} else {
		expectCode(t, err, "rationale_required")
	}

	accepted, err := agent.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "PLAN maps each criterion to a section", []string{"art_evidence"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.State != "accepted" {
		t.Fatalf("handoff not accepted: %+v", accepted)
	}
	st := workspaceStatus(t, s, ws)
	if st.Workspace.Tasks[0].State != "accepted" {
		t.Fatalf("task not accepted: %+v", st.Workspace.Tasks[0])
	}
	d := lastDecision(t, st, "autonomous.plan_acceptance")
	if !d.Autonomous || d.ResolvedBy != "orchestrator" || d.Subject != "handoff:"+h.ID || d.RunID != orch.CurrentRunID || d.SessionID != orch.ID {
		t.Fatalf("plan acceptance decision provenance: %+v", d)
	}
	if len(d.Evidence) != 1 || d.Evidence[0] != "art_evidence" || d.Reason != "PLAN maps each criterion to a section" {
		t.Fatalf("plan acceptance decision evidence/reason: %+v", d)
	}
}

func TestAutonomyAdvanceRationaleAndDecision(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	plan := plannedTask(t, s, ws, "advance-plan", "planner", nil)
	p, w := startTask(t, s, ws, plan)
	h := submitPlanForGate(t, s, ws, p, w, "advance-plan-result")
	if _, err := agent.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "accepted planner result", nil, false); err != nil {
		t.Fatal(err)
	}

	if _, err := agent.AdvanceWorkflowAudited(ctx, ws, "plan_review", "", nil, ""); err == nil {
		t.Fatal("advance without rationale was allowed under autonomy")
	} else {
		expectCode(t, err, "rationale_required")
	}
	if _, err := agent.AdvanceWorkflowAudited(ctx, ws, "plan_review", "core gates hold", []string{"hs_1"}, ""); err != nil {
		t.Fatal(err)
	}
	st := workspaceStatus(t, s, ws)
	if st.Workspace.Workflow.Phase != "plan_review" {
		t.Fatalf("phase did not advance: %s", st.Workspace.Workflow.Phase)
	}
	d := lastDecision(t, st, "autonomous.phase_advance")
	if !d.Autonomous || d.ResolvedBy != "orchestrator" || d.Subject != "phase:plan_review" {
		t.Fatalf("phase advance decision: %+v", d)
	}
}

func TestAutonomyStatePhaseRationaleAndDecision(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	plan := plannedTask(t, s, ws, "state-plan", "planner", nil)
	p, w := startTask(t, s, ws, plan)
	h := submitPlanForGate(t, s, ws, p, w, "state-plan-result")
	if _, err := agent.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "accepted planner result", nil, false); err != nil {
		t.Fatal(err)
	}
	revision := currentRevision(t, s, ws)
	patch := StatePatch{Phase: strptr("plan_review")}

	if _, err := agent.UpdateStateAudited(ctx, ws, revision, patch, "", nil, ""); err == nil {
		t.Fatal("state phase update without rationale was allowed under autonomy")
	} else {
		expectCode(t, err, "rationale_required")
	}
	if _, err := agent.UpdateStateAudited(ctx, ws, revision, patch, "phase gate satisfied", nil, "state-phase"); err != nil {
		t.Fatal(err)
	}
	st := workspaceStatus(t, s, ws)
	if st.Workspace.Workflow.Phase != "plan_review" {
		t.Fatalf("phase did not advance: %s", st.Workspace.Workflow.Phase)
	}
	d := lastDecision(t, st, "autonomous.phase_advance")
	if !d.Autonomous || d.Subject != "phase:plan_review" {
		t.Fatalf("state phase decision: %+v", d)
	}
}

func TestAutonomyRetryRationaleAndBound(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	task := plannedTask(t, s, ws, "retry-task", "planner", nil)
	p, _ := startTask(t, s, ws, task)
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := agent.RetryTaskAudited(ctx, ws, task.ID, "checks failed", "", nil, "", MutationGuard{}); err == nil {
		t.Fatal("retry without rationale was allowed under autonomy")
	} else {
		expectCode(t, err, "rationale_required")
	}
	if _, err := agent.RetryTaskAudited(ctx, ws, task.ID, "checks failed", "attempt failed the project checks", []string{"ck_1"}, "retry-gate-1", MutationGuard{}); err != nil {
		t.Fatal(err)
	}
	st := workspaceStatus(t, s, ws)
	if st.Workspace.Tasks[0].Attempt != 2 {
		t.Fatalf("retry did not start a new attempt: %+v", st.Workspace.Tasks[0])
	}
	d := lastDecision(t, st, "autonomous.retry")
	if !d.Autonomous || d.ResolvedBy != "orchestrator" || d.Subject != "task:"+task.ID {
		t.Fatalf("retry decision: %+v", d)
	}
	if _, err := agent.RetryTaskAudited(ctx, ws, task.ID, "checks failed again", "second retry", nil, "retry-gate-2", MutationGuard{}); err == nil {
		t.Fatal("second retry was allowed under autonomy")
	} else {
		expectCode(t, err, "autonomy_bound_exceeded")
	}
}

func TestAutonomyRetireRationaleAndDecision(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	task := plannedTask(t, s, ws, "retire-task", "planner", nil)
	p, _ := startTask(t, s, ws, task)
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := agent.RetireTaskAudited(ctx, ws, task.ID, "cancelled", "not needed", "", nil, "", MutationGuard{}); err == nil {
		t.Fatal("retire without rationale was allowed under autonomy")
	} else {
		expectCode(t, err, "rationale_required")
	}
	if _, err := agent.RetireTaskAudited(ctx, ws, task.ID, "cancelled", "not needed", "out of the accepted plan", nil, "retire-gate", MutationGuard{}); err != nil {
		t.Fatal(err)
	}
	st := workspaceStatus(t, s, ws)
	if st.Workspace.Tasks[0].State != "cancelled" {
		t.Fatalf("task not retired: %+v", st.Workspace.Tasks[0])
	}
	d := lastDecision(t, st, "autonomous.retire")
	if !d.Autonomous || d.ResolvedBy != "orchestrator" || d.Subject != "task:"+task.ID {
		t.Fatalf("retire decision: %+v", d)
	}
}

func TestAutonomyRejectionBound(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	plan := plannedTask(t, s, ws, "reject-plan", "planner", nil)
	p, w := startTask(t, s, ws, plan)

	first := submitPlanForGate(t, s, ws, p, w, "reject-result-1")
	if _, err := agent.ReviewHandoffAudited(ctx, ws, first.ID, false, "", "", nil, false); err == nil {
		t.Fatal("reject without rationale was allowed under autonomy")
	} else {
		expectCode(t, err, "rationale_required")
	}
	if _, err := agent.ReviewHandoffAudited(ctx, ws, first.ID, false, "missing edge cases", "first rejection", nil, false); err != nil {
		t.Fatal(err)
	}
	second := submitPlanForGate(t, s, ws, p, w, "reject-result-2")
	if _, err := agent.ReviewHandoffAudited(ctx, ws, second.ID, false, "still missing edge cases", "second rejection", nil, false); err != nil {
		t.Fatal(err)
	}
	third := submitPlanForGate(t, s, ws, p, w, "reject-result-3")
	if _, err := agent.ReviewHandoffAudited(ctx, ws, third.ID, false, "again", "third rejection", nil, false); err == nil {
		t.Fatal("third rejection was allowed under autonomy")
	} else {
		expectCode(t, err, "autonomy_bound_exceeded")
	}
	st := workspaceStatus(t, s, ws)
	for _, d := range st.Workspace.Decisions {
		if d.Kind == "autonomous.handoff_reject" && d.Subject != "task:"+plan.ID {
			t.Fatalf("rejection decision has wrong subject: %+v", d)
		}
	}
}

func TestAutonomyGateSaveFailureIsAtomic(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	plan := plannedTask(t, s, ws, "atomic-plan", "planner", nil)
	p, w := startTask(t, s, ws, plan)
	h := submitPlanForGate(t, s, ws, p, w, "atomic-result")
	before := workspaceStatus(t, s, ws)

	flushDocumentTestHook = func(*Document) error { return errors.New("injected save failure") }
	defer func() { flushDocumentTestHook = nil }()
	_, err := agent.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "would accept", nil, false, "atomic-key")
	flushDocumentTestHook = nil
	if err == nil {
		t.Fatal("injected save failure was not reported")
	}
	if err.Error() != "injected save failure" {
		t.Fatalf("unexpected error: %v", err)
	}

	after := workspaceStatus(t, s, ws)
	if after.Workspace.Revision != before.Workspace.Revision {
		t.Fatalf("revision changed after a failed save: %d -> %d", before.Workspace.Revision, after.Workspace.Revision)
	}
	if len(after.Workspace.Decisions) != len(before.Workspace.Decisions) {
		t.Fatalf("audit decision persisted after a failed save: %d -> %d", len(before.Workspace.Decisions), len(after.Workspace.Decisions))
	}
	if after.Workspace.Tasks[0].State != "awaiting_review" {
		t.Fatalf("gate change persisted after a failed save: %+v", after.Workspace.Tasks[0])
	}
	var stored Handoff
	if err := s.With(ctx, ws, func(d *Document) error {
		found, err := findHandoff(d, h.ID)
		if err != nil {
			return err
		}
		stored = *found
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if stored.State != "submitted" {
		t.Fatalf("handoff state changed after a failed save: %+v", stored)
	}
}

func TestAutonomyDecisionRecord(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	task := plannedTask(t, s, ws, "record-task", "planner", nil)

	if _, err := agent.RecordDecision(ctx, ws, DecisionRecordOptions{Kind: "autonomous.unknown", Subject: "task:" + task.ID, Reason: "r"}, "record-bad"); err == nil {
		t.Fatal("unsupported decision kind was accepted")
	} else {
		expectCode(t, err, "invalid_decision")
	}
	if _, err := s.RecordDecision(ctx, ws, DecisionRecordOptions{Kind: "autonomous.assumption", Subject: "task:" + task.ID, Reason: "user terminal"}, "record-user"); err == nil {
		t.Fatal("user terminal recorded an autonomous decision")
	} else {
		expectCode(t, err, "forbidden")
	}
	if _, err := agent.RecordDecision(ctx, ws, DecisionRecordOptions{Kind: "autonomous.assumption", Subject: "task:" + task.ID, Reason: ""}, "record-empty"); err == nil {
		t.Fatal("empty rationale was accepted")
	} else {
		expectCode(t, err, "rationale_required")
	}

	first, err := agent.RecordDecision(ctx, ws, DecisionRecordOptions{Kind: "autonomous.assumption", Subject: "task:" + task.ID, Reason: "chose the conservative reading", Evidence: []string{"art_1"}}, "record-assumption")
	if err != nil {
		t.Fatal(err)
	}
	d := lastDecision(t, first, "autonomous.assumption")
	if !d.Autonomous || d.ResolvedBy != "orchestrator" || d.Subject != "task:"+task.ID || d.Reason != "chose the conservative reading" {
		t.Fatalf("assumption decision: %+v", d)
	}

	replay, err := agent.RecordDecision(ctx, ws, DecisionRecordOptions{Kind: "autonomous.assumption", Subject: "task:" + task.ID, Reason: "chose the conservative reading", Evidence: []string{"art_1"}}, "record-assumption")
	if err != nil || replay.Workspace.Revision != first.Workspace.Revision {
		t.Fatalf("decision record replay changed the result: %+v %v", replay, err)
	}
	if _, err := agent.RecordDecision(ctx, ws, DecisionRecordOptions{Kind: "autonomous.question_answer", Subject: "worker:sess_1", Reason: "answered from the issue"}, "record-answer"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.DisableAutonomy(ctx, ws, AutonomyDisableOptions{Reason: "hand back to user", ExpectedRevision: currentRevision(t, s, ws)}, "record-disable"); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.RecordDecision(ctx, ws, DecisionRecordOptions{Kind: "autonomous.assumption", Subject: "task:" + task.ID, Reason: "after delivery"}, "record-not-running"); err == nil {
		t.Fatal("decision record worked after autonomy stopped")
	} else {
		expectCode(t, err, "autonomy_not_running")
	}
}

func TestAutonomyGateWithoutAutonomyNeedsNoRationale(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	orch, err := s.StartOrchestrator(ctx, ws, "nonauto-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	agent := *s
	agent.Actor = Actor{AgentID: orch.AgentID, SessionID: orch.ID, RunID: orch.CurrentRunID}
	plan := plannedTask(t, s, ws, "nonauto-plan", "planner", nil)
	p, w := startTask(t, s, ws, plan)
	h := submitPlanForGate(t, s, ws, p, w, "nonauto-result")

	if _, err := agent.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "", nil, false); err != nil {
		t.Fatalf("non-autonomous accept required a rationale: %v", err)
	}
	st := workspaceStatus(t, s, ws)
	for _, d := range st.Workspace.Decisions {
		if d.Autonomous || d.Kind == "autonomous.plan_acceptance" {
			t.Fatalf("autonomous decision recorded on a non-autonomous workspace: %+v", d)
		}
	}
}

// TestAutonomyGateReceiptDigestsUnchanged proves that the audited entry points
// produce the historical payload digest when the rationale and evidence are
// empty: an old receipt replays instead of conflicting.
func TestAutonomyGateReceiptDigestsUnchanged(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	plan := plannedTask(t, s, ws, "digest-plan", "planner", nil)
	p, w := startTask(t, s, ws, plan)
	h := submitPlanForGate(t, s, ws, p, w, "digest-result")

	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, "", "digest-review"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewHandoffAudited(ctx, ws, h.ID, true, "", "", nil, false, "digest-review"); err != nil {
		t.Fatalf("review replay conflicted: %v", err)
	}
	if _, err := s.AdvanceWorkflow(ctx, ws, "plan_review", "digest-advance"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceWorkflowAudited(ctx, ws, "plan_review", "", nil, "digest-advance"); err != nil {
		t.Fatalf("advance replay conflicted: %v", err)
	}
	revision := currentRevision(t, s, ws)
	patch := StatePatch{Title: strptr("Renamed")}
	if _, err := s.UpdateState(ctx, ws, revision, patch, "digest-state"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateStateAudited(ctx, ws, revision, patch, "", nil, "digest-state"); err != nil {
		t.Fatalf("state replay conflicted: %v", err)
	}

	retryTask := plannedTask(t, s, ws, "digest-retry", "planner", nil)
	rp, _ := startTask(t, s, ws, retryTask)
	if _, err := s.StopSession(ctx, ws, rp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetryTask(ctx, ws, retryTask.ID, "reason", "digest-retry-op"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetryTaskAudited(ctx, ws, retryTask.ID, "reason", "", nil, "digest-retry-op", MutationGuard{}); err != nil {
		t.Fatalf("retry replay conflicted: %v", err)
	}

	retireTask := plannedTask(t, s, ws, "digest-retire", "planner", nil)
	ip, _ := startTask(t, s, ws, retireTask)
	if _, err := s.StopSession(ctx, ws, ip.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetireTask(ctx, ws, retireTask.ID, "cancelled", "obsolete", "digest-retire-op", MutationGuard{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetireTaskAudited(ctx, ws, retireTask.ID, "cancelled", "obsolete", "", nil, "digest-retire-op", MutationGuard{}); err != nil {
		t.Fatalf("retire replay conflicted: %v", err)
	}
}
