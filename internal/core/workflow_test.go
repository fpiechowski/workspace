package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fakeForge struct {
	result        *ForgeResult
	creates       int
	uncertain     bool
	duringPublish func()
	duringLookup  func()
}

func (f *fakeForge) Lookup(context.Context, string, ChangeRequest) (*ForgeResult, error) {
	if f.duringLookup != nil {
		f.duringLookup()
	}
	return f.result, nil
}

func TestSyncChangeRequestsFencesActors(t *testing.T) {
	s, ws, _ := integratedWorkflow(t)
	ctx := context.Background()
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeForge{}
	s.Forge = f
	if _, err := s.PublishChangeRequest(ctx, ws, cr.ID, false); err != nil {
		t.Fatal(err)
	}
	v, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		d.Registry.Sessions = append(d.Registry.Sessions,
			Session{ID: "worker-sync", AgentID: "worker", State: "running"},
			Session{ID: "orch-sync", AgentID: d.State.OrchestratorAgentID, State: "running"})
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	lookups := 0
	f.duringLookup = func() { lookups++ }
	s.Actor = Actor{AgentID: "worker", SessionID: "worker-sync"}
	_, err = s.SyncChangeRequests(ctx, ws)
	expectCode(t, err, "forbidden")
	_, err = s.Reconcile(ctx, ws)
	expectCode(t, err, "forbidden")
	if lookups != 0 {
		t.Fatal("unauthorized sync reached forge")
	}
	s.Actor = Actor{AgentID: v.Workspace.OrchestratorAgentID, SessionID: "orch-sync"}
	f.result.State = "merged"
	f.duringLookup = func() {
		if err := s.With(ctx, ws, func(d *Document) error {
			p, err := findSession(d, "orch-sync")
			if err != nil {
				return err
			}
			p.State = "exited"
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.SyncChangeRequests(ctx, ws)
	expectCode(t, err, "stale_actor")
	v, err = s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if v.Workspace.ChangeRequests[0].State != "open" {
		t.Fatal("stale actor wrote remote state")
	}
	s.Actor = Actor{}
	f.duringLookup = nil
	requests, err := s.SyncChangeRequests(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if requests[0].State != "merged" {
		t.Fatal("user sync failed")
	}
}
func (f *fakeForge) Publish(_ context.Context, _ string, cr ChangeRequest, _ string) (ForgeResult, error) {
	f.creates++
	if f.duringPublish != nil {
		f.duringPublish()
	}
	f.result = &ForgeResult{ID: "42", URL: "https://forge.example/project/pull/42", State: "open", HeadCommit: cr.HeadCommit}
	if f.uncertain {
		return ForgeResult{}, fail("forge_error", "connection lost after publication")
	}
	return *f.result, nil
}

func TestPublicationDoesNotReviveInvalidatedRequest(t *testing.T) {
	s, ws, _ := integratedWorkflow(t)
	ctx := context.Background()
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s.Forge = &fakeForge{duringPublish: func() {
		v, err := s.Status(ctx, ws)
		if err != nil {
			t.Fatal(err)
		}
		for _, task := range v.Workspace.Tasks {
			if task.Role == "implementer" {
				if _, err := s.RetryTask(ctx, ws, task.ID, "new feedback", ""); err != nil {
					t.Fatal(err)
				}
			}
		}
	}}
	published, err := s.PublishChangeRequest(ctx, ws, cr.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if published.State != "outdated" || published.ExternalID != "42" {
		t.Fatalf("publication revived invalidated results: %+v", published)
	}
}

func TestPublicationFromExpiredSessionCanBeRecoveredWithoutDuplicate(t *testing.T) {
	s, ws, _ := integratedWorkflow(t)
	ctx := context.Background()
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var agent string
	if err := s.With(ctx, ws, func(d *Document) error {
		agent = d.State.OrchestratorAgentID
		d.Registry.Sessions = append(d.Registry.Sessions, Session{ID: "publisher", AgentID: agent, State: "running"})
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	s.Actor = Actor{AgentID: agent, SessionID: "publisher"}
	f := &fakeForge{duringPublish: func() {
		if err := s.With(ctx, ws, func(d *Document) error {
			p, err := findSession(d, "publisher")
			if err != nil {
				return err
			}
			p.State = "interrupted"
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
	}}
	s.Forge = f
	_, err = s.PublishChangeRequest(ctx, ws, cr.ID, true)
	expectCode(t, err, "stale_actor")
	v, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if v.Workspace.ChangeRequests[0].State != "publishing" {
		t.Fatal("lost publication intent")
	}
	s.Actor = Actor{}
	recovered, err := s.PublishChangeRequest(ctx, ws, cr.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 || recovered.ExternalID != "42" || recovered.State != "open" {
		t.Fatalf("publication recovery: creates=%d request=%+v", f.creates, recovered)
	}
}

func TestChangeRequestPolicyRequiresCorrectScope(t *testing.T) {
	s, ws, i := integratedWorkflow(t)
	ctx := context.Background()
	v, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	var implementation Worktree
	for _, task := range v.Workspace.Tasks {
		if task.Role == "implementer" {
			for _, w := range v.Worktrees {
				if w.ID == task.WorktreeID {
					implementation = w
				}
			}
		}
	}
	_, err = s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{Worktree: implementation.ID})
	expectCode(t, err, "revision_mismatch")
	if err := s.With(ctx, ws, func(d *Document) error { d.State.ChangeRequestMode = "per-task"; return saveDocument(d) }); err != nil {
		t.Fatal(err)
	}
	_, err = s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{})
	expectCode(t, err, "worktree_required")
	_, err = s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{Worktree: i.WorktreeID})
	expectCode(t, err, "revision_mismatch")
	_, err = s.AdvanceWorkflow(ctx, ws, "", "")
	expectCode(t, err, "workflow_gate")
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{Worktree: implementation.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveChangeRequest(ctx, ws, cr.ID, "skip", "", "Local fixture release", false); err != nil {
		t.Fatal(err)
	}
	advancePhase(t, s, ws, "live_test_offer")
}
func advancePhase(t *testing.T, s *Service, ws, expected string) {
	t.Helper()
	v, err := s.AdvanceWorkflow(context.Background(), ws, expected, "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Workspace.Workflow.Phase != expected {
		t.Fatal("wrong phase")
	}
}
func finishTask(t *testing.T, s *Service, ws string, p Session, w Worktree, artifact string) Handoff {
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
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	return h
}
func integratedWorkflow(t *testing.T) (*Service, string, Integration) {
	t.Helper()
	s, ws := fixture(t)
	ctx := context.Background()
	_, err := s.AdvanceWorkflow(ctx, ws, "", "")
	expectCode(t, err, "workflow_gate")
	plan := plannedTask(t, s, ws, "plan", "planner", nil)
	pp, pw := startTask(t, s, ws, plan)
	finishTask(t, s, ws, pp, pw, "PLAN.md")
	advancePhase(t, s, ws, "plan_review")
	impl := plannedTask(t, s, ws, "implementation", "implementer", []string{plan.ID})
	advancePhase(t, s, ws, "implementing")
	ip, iw := startTask(t, s, ws, impl)
	if err := os.WriteFile(filepath.Join(iw.Path, "fix.txt"), []byte("implementation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, iw.Path, "add", "fix.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, iw.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fix issue"); err != nil {
		t.Fatal(err)
	}
	finishTask(t, s, ws, ip, iw, "IMPLEMENTATION.md")
	advancePhase(t, s, ws, "integrating")
	i, err := s.PrepareIntegration(ctx, ws, IntegrationOptions{OperationKey: "integration:1"})
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := s.PrepareIntegration(ctx, ws, IntegrationOptions{OperationKey: "integration:1"})
	if err != nil || repeat.WorktreeID != i.WorktreeID {
		t.Fatal("integration duplicated")
	}
	task := plannedTask(t, s, ws, "integrator", "integrator", []string{impl.ID})
	a, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "integrator", Role: "integrator"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Task: task.ID, Worktree: i.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx, ws)
	var w Worktree
	for _, wt := range status.Worktrees {
		if wt.ID == i.WorktreeID {
			w = wt
		}
	}
	if _, err := git(ctx, w.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "merge", "--no-ff", "--no-edit", i.Heads[0]); err != nil {
		t.Fatal(err)
	}
	finishTask(t, s, ws, p, w, "INTEGRATION.md")
	advancePhase(t, s, ws, "change_requests")
	status, _ = s.Status(ctx, ws)
	return s, ws, *status.Workspace.Integration
}
func TestIssueResolutionThroughExplicitRelease(t *testing.T) {
	s, ws, i := integratedWorkflow(t)
	ctx := context.Background()
	s.Forge = &fakeForge{}
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{OperationKey: "cr:1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfirmRelease(ctx, ws, "premature", false)
	expectCode(t, err, "workflow_gate")
	if _, err := s.PublishChangeRequest(ctx, ws, cr.ID, false); err != nil {
		t.Fatal(err)
	}
	advancePhase(t, s, ws, "live_test_offer")
	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	decision := menu.PendingDecision
	if decision == nil {
		t.Fatal("missing interactive test offer")
	}
	_, err = s.AnswerDecision(ctx, ws, DecisionAnswer{ID: decision.ID, Answer: "run", ExpectedRevision: decision.Revision - 1, Environment: "http://localhost:3000"})
	expectCode(t, err, "revision_conflict")
	if _, err := s.AnswerDecision(ctx, ws, DecisionAnswer{ID: decision.ID, Answer: "run", ExpectedRevision: decision.Revision, Environment: "http://localhost:3000"}); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx, ws)
	var integrator Task
	var w Worktree
	for _, t := range status.Workspace.Tasks {
		if t.Role == "integrator" {
			integrator = t
		}
	}
	for _, wt := range status.Worktrees {
		if wt.ID == i.WorktreeID {
			w = wt
		}
	}
	task := plannedTask(t, s, ws, "tester", "tester", []string{integrator.ID})
	a, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "tester", Role: "tester"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID, Task: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	finishTask(t, s, ws, p, w, "LIVE_TEST.md")
	advancePhase(t, s, ws, "awaiting_release")
	status, _ = s.Status(ctx, ws)
	if status.Workspace.Release.UserConfirmed {
		t.Fatal("release inferred from test")
	}
	final, err := s.ConfirmRelease(ctx, ws, "release-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if final.Workspace.Status != "completed" || !final.Workspace.Release.UserConfirmed || final.Workspace.Release.HeadCommit != i.HeadCommit {
		t.Fatal("release not bound to tested integration")
	}
}
func TestPublicationUncertaintyDoesNotDuplicateRequest(t *testing.T) {
	s, ws, _ := integratedWorkflow(t)
	ctx := context.Background()
	forge := &fakeForge{uncertain: true}
	s.Forge = forge
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.PublishChangeRequest(ctx, ws, cr.ID, false)
	expectCode(t, err, "forge_error")
	published, err := s.PublishChangeRequest(ctx, ws, cr.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if forge.creates != 1 || published.ExternalID != "42" {
		t.Fatal("uncertain publication was duplicated")
	}
}
func TestLiveTestingSkipAndChangedHeadGate(t *testing.T) {
	s, ws, i := integratedWorkflow(t)
	ctx := context.Background()
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveChangeRequest(ctx, ws, cr.ID, "skip", "", "Local release approved", false); err != nil {
		t.Fatal(err)
	}
	advancePhase(t, s, ws, "live_test_offer")
	menu, _ := s.Menu(ctx, ws)
	d := menu.PendingDecision
	_, err = s.AnswerDecision(ctx, ws, DecisionAnswer{ID: d.ID, Answer: "skip", ExpectedRevision: d.Revision})
	expectCode(t, err, "reason_required")
	if _, err := s.AnswerDecision(ctx, ws, DecisionAnswer{ID: d.ID, Answer: "skip", ExpectedRevision: d.Revision, Reason: "Covered by existing release verification"}); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx, ws)
	for _, w := range status.Worktrees {
		if w.ID == i.WorktreeID {
			if _, err := git(ctx, w.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "new revision"); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, err = s.ConfirmRelease(ctx, ws, "release-stale", false)
	expectCode(t, err, "integration_changed")
}
