package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// planFirstFixture creates a plan-first v2 workspace (the bundled builtin
// capabilities) on top of the shared project fixture and an integration target
// branch that is never checked out.
func planFirstFixture(t *testing.T) (*Service, string) {
	t.Helper()
	s, _ := fixture(t)
	ctx := context.Background()
	w, err := s.Create(ctx, CreateOptions{Title: "Plan-first landing", Input: "Deliver a change", Workflow: "plan-first", OperationKey: "plan-first-landing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, s.Root, "branch", "release"); err != nil {
		t.Fatal(err)
	}
	return s, w.Workspace.ID
}

// commitScaffold commits the project scaffold so a checked-out target branch is
// clean before landing.
func commitScaffold(t *testing.T, s *Service) {
	t.Helper()
	ctx := context.Background()
	if _, err := git(ctx, s.Root, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, s.Root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "project scaffold"); err != nil {
		t.Fatal(err)
	}
}

// drivePlanFirst runs the plan-first v2 machine through implementation and
// advances to the integration phase. It does not prepare or accept an
// integration.
func drivePlanFirst(t *testing.T, s *Service, ws string) (Task, Worktree) {
	t.Helper()
	ctx := context.Background()
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
	advancePhase(t, s, ws, "integration")
	return impl, iw
}

// integratePlanFirst drives a plan-first v2 workspace to an accepted integrator
// using a local integration worktree. No remote is involved.
func integratePlanFirst(t *testing.T, s *Service, ws, target string) Integration {
	t.Helper()
	ctx := context.Background()
	impl, _ := drivePlanFirst(t, s, ws)
	opt := IntegrationOptions{OperationKey: "integration:1"}
	if target != "" {
		opt.Target = target
	}
	i, err := s.PrepareIntegration(ctx, ws, opt)
	if err != nil {
		t.Fatal(err)
	}
	task := plannedTask(t, s, ws, "integrator", "integrator", []string{impl.ID})
	a, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "integrator", Role: "integrator", PromptTemplate: "implementation"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Task: task.ID, Worktree: i.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
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
	status, err = s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	return *status.Workspace.Integration
}

func refCommit(t *testing.T, s *Service, branch string) string {
	t.Helper()
	commit, err := git(context.Background(), s.Root, "rev-parse", "refs/heads/"+branch)
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

func requireRef(t *testing.T, s *Service, branch, want string) {
	t.Helper()
	if got := refCommit(t, s, branch); got != want {
		t.Fatalf("ref %s moved: got %s want %s", branch, got, want)
	}
}

// Land refuses every precondition without touching the target ref.
func TestLandIntegrationRefusals(t *testing.T) {
	ctx := context.Background()
	t.Run("wrong revision", func(t *testing.T) {
		s, ws := planFirstFixture(t)
		integratePlanFirst(t, s, ws, "release")
		before := refCommit(t, s, "release")
		_, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release", ExpectedRevision: currentRevision(t, s, ws) + 1})
		expectCode(t, err, "revision_conflict")
		requireRef(t, s, "release", before)
	})
	t.Run("agent without confirmation", func(t *testing.T) {
		s, ws := planFirstFixture(t)
		integratePlanFirst(t, s, ws, "release")
		p, err := s.StartOrchestrator(ctx, ws, "orch-land")
		if err != nil {
			t.Fatal(err)
		}
		before := refCommit(t, s, "release")
		agentService := *s
		agentService.Actor = Actor{AgentID: p.AgentID, SessionID: p.ID, RunID: p.CurrentRunID}
		_, err = agentService.LandIntegration(ctx, ws, LandingOptions{Target: "release"})
		expectCode(t, err, "user_decision_required")
		requireRef(t, s, "release", before)
	})
	t.Run("no accepted integrator", func(t *testing.T) {
		s, ws := planFirstFixture(t)
		drivePlanFirst(t, s, ws)
		before := refCommit(t, s, "release")
		_, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release"})
		expectCode(t, err, "workflow_gate")
		requireRef(t, s, "release", before)
	})
	t.Run("stale integration", func(t *testing.T) {
		s, ws := planFirstFixture(t)
		i := integratePlanFirst(t, s, ws, "release")
		status, err := s.Status(ctx, ws)
		if err != nil {
			t.Fatal(err)
		}
		var w Worktree
		for _, wt := range status.Worktrees {
			if wt.ID == i.WorktreeID {
				w = wt
			}
		}
		if _, err := git(ctx, w.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "changed after acceptance"); err != nil {
			t.Fatal(err)
		}
		before := refCommit(t, s, "release")
		_, err = s.LandIntegration(ctx, ws, LandingOptions{Target: "release"})
		expectCode(t, err, "integration_changed")
		requireRef(t, s, "release", before)
	})
	t.Run("pending decision", func(t *testing.T) {
		s, ws := planFirstFixture(t)
		integratePlanFirst(t, s, ws, "release")
		if err := s.With(ctx, ws, func(d *Document) error {
			d.State.PendingDecision = &Decision{ID: "decision_land", Kind: "landing", Question: "Approve?", Options: []string{"yes", "no"}, Revision: d.State.Revision + 1}
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
		before := refCommit(t, s, "release")
		_, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release"})
		expectCode(t, err, "decision_pending")
		requireRef(t, s, "release", before)
	})
}

// A checked-out, clean target fast-forwards through merge --ff-only.
func TestLandIntegrationFastForwardsCheckedOutTarget(t *testing.T) {
	ctx := context.Background()
	s, ws := planFirstFixture(t)
	commitScaffold(t, s)
	i := integratePlanFirst(t, s, ws, "")
	if i.Target != "master" {
		t.Fatalf("default target: %s", i.Target)
	}
	v, err := s.LandIntegration(ctx, ws, LandingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !landingLanded(documentFor(t, s, ws)) {
		t.Fatal("landing was not recorded")
	}
	if got := refCommit(t, s, "master"); got != i.HeadCommit {
		t.Fatalf("master did not fast-forward: got %s want %s", got, i.HeadCommit)
	}
	if v.Workspace.Integration.Landing.Target != "master" || v.Workspace.Integration.Landing.Before == v.Workspace.Integration.Landing.After {
		t.Fatalf("landing receipt: %+v", v.Workspace.Integration.Landing)
	}
	body := readBody(t, s, ws)
	if strings.Count(body, "## Integration accepted") != 1 {
		t.Fatalf("integration acceptance note: %s", body)
	}
}

// An unchecked-out target is updated with a compare-and-swap update-ref; the
// checked-out master branch is untouched.
func TestLandIntegrationCASUpdatesUncheckedOutTarget(t *testing.T) {
	ctx := context.Background()
	s, ws := planFirstFixture(t)
	master := refCommit(t, s, "master")
	i := integratePlanFirst(t, s, ws, "release")
	if _, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release"}); err != nil {
		t.Fatal(err)
	}
	if got := refCommit(t, s, "release"); got != i.HeadCommit {
		t.Fatalf("release not updated: got %s want %s", got, i.HeadCommit)
	}
	requireRef(t, s, "master", master)
}

// A dirty checkout of the target refuses the land and leaves the ref unchanged.
func TestLandIntegrationRefusesDirtyCheckout(t *testing.T) {
	ctx := context.Background()
	s, ws := planFirstFixture(t)
	integratePlanFirst(t, s, ws, "")
	before := refCommit(t, s, "master")
	if _, err := s.LandIntegration(ctx, ws, LandingOptions{}); err == nil {
		t.Fatal("land accepted a dirty checkout")
	} else {
		expectCode(t, err, "target_checkout_dirty")
	}
	requireRef(t, s, "master", before)
}

// Retrying an interrupted land (the ref already moved, the receipt still
// pending) completes the record without a second git effect.
func TestLandIntegrationRetryAfterInterruptedLand(t *testing.T) {
	ctx := context.Background()
	s, ws := planFirstFixture(t)
	i := integratePlanFirst(t, s, ws, "release")
	// Simulate the crash window: the ref update happened but the landed state
	// was not persisted. A second update-ref with the old value would fail, so a
	// successful retry proves the git effect was skipped.
	if _, err := git(ctx, s.Root, "update-ref", "refs/heads/release", i.HeadCommit, i.BaseCommit); err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Integration.Landing = &Landing{State: "pending", Target: "release", Before: i.BaseCommit, After: i.HeadCommit}
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release"}, "land-retry"); err != nil {
		t.Fatalf("interrupted land was not recovered: %v", err)
	}
	if !landingLanded(documentFor(t, s, ws)) {
		t.Fatal("retry did not record the landed state")
	}
	if got := refCommit(t, s, "release"); got != i.HeadCommit {
		t.Fatalf("release changed on retry: %s", got)
	}
	if body := readBody(t, s, ws); strings.Count(body, "## Integration accepted") != 1 {
		t.Fatalf("retry duplicated the acceptance note: %s", body)
	}
	// The same operation key replays the committed result.
	if _, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release"}, "land-retry"); err != nil {
		t.Fatalf("land replay failed: %v", err)
	}
}

// The integration-phase menu walks through prepare, land and complete.
func TestLandingMenuSubStates(t *testing.T) {
	ctx := context.Background()
	s, ws := planFirstFixture(t)
	drivePlanFirst(t, s, ws)
	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !menuHas(menu, "prepare") || !menuHas(menu, "advance") || menuHas(menu, "land") {
		t.Fatalf("integration menu before prepare: %+v", menu.Actions)
	}

	s2, ws2 := planFirstFixture(t)
	integratePlanFirst(t, s2, ws2, "release")
	menu2, err := s2.Menu(ctx, ws2)
	if err != nil {
		t.Fatal(err)
	}
	if !menuHas(menu2, "land") || menuHas(menu2, "prepare") || menuHas(menu2, "complete") {
		t.Fatalf("integration menu before land: %+v", menu2.Actions)
	}
	if _, err := s2.LandIntegration(ctx, ws2, LandingOptions{Target: "release"}); err != nil {
		t.Fatal(err)
	}
	menu2, err = s2.Menu(ctx, ws2)
	if err != nil {
		t.Fatal(err)
	}
	if !menuHas(menu2, "complete") || menuHas(menu2, "land") {
		t.Fatalf("integration menu after land: %+v", menu2.Actions)
	}
}

// Completion requires landing (or nothing to integrate) and then archive no
// longer needs a release reference for a plan-first workflow.
func TestLandCompleteAndArchivePlanFirst(t *testing.T) {
	ctx := context.Background()
	s, ws := planFirstFixture(t)
	integratePlanFirst(t, s, ws, "release")

	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !menuHas(menu, "land") {
		t.Fatalf("integration menu lacks land: %+v", menu.Actions)
	}
	if _, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release"}); err != nil {
		t.Fatal(err)
	}
	menu, err = s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !menuHas(menu, "complete") || menuHas(menu, "land") {
		t.Fatalf("landed menu: %+v", menu.Actions)
	}
	completed, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: currentRevision(t, s, ws)})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Workspace.Status != "completed" || completed.Workspace.Workflow.Phase != "completed" {
		t.Fatalf("completion state: %+v", completed.Workspace)
	}
	archived, err := s.Archive(ctx, ws)
	if err != nil {
		t.Fatalf("plan-first archive failed: %v", err)
	}
	if archived.Workspace.Status != "archived" {
		t.Fatalf("archive state: %s", archived.Workspace.Status)
	}
}

// A landed plan-first workspace completes from the orchestrator's own actor
// while its Session runs, but an active worker Session still blocks and is
// named in the error.
func TestLandCompleteToleratesOrchestratorButNotWorkers(t *testing.T) {
	ctx := context.Background()
	s, ws := planFirstFixture(t)
	integratePlanFirst(t, s, ws, "release")
	if _, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release"}); err != nil {
		t.Fatal(err)
	}
	p, err := s.StartOrchestrator(ctx, ws, "orch-complete")
	if err != nil {
		t.Fatal(err)
	}
	agentService := *s
	agentService.Actor = Actor{AgentID: p.AgentID, SessionID: p.ID, RunID: p.CurrentRunID}

	agent, worktree := worker(t, s, ws, "active-worker")
	workerSession, err := s.StartSession(ctx, ws, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	revision := currentRevision(t, s, ws)
	if _, err := agentService.CompleteWorkspace(ctx, ws, CompleteOptions{UserConfirmed: true, ExpectedRevision: revision}); err == nil {
		t.Fatal("landed completion accepted an active worker session")
	} else {
		expectCode(t, err, "session_active")
		if !strings.Contains(err.Error(), workerSession.ID) {
			t.Fatalf("session_active did not name the worker session: %v", err)
		}
	}

	if _, err := s.StopSession(ctx, ws, workerSession.ID); err != nil {
		t.Fatal(err)
	}
	revision = currentRevision(t, s, ws)
	completed, err := agentService.CompleteWorkspace(ctx, ws, CompleteOptions{UserConfirmed: true, ExpectedRevision: revision}, "land-complete-orch")
	if err != nil {
		t.Fatalf("orchestrator completion failed: %v", err)
	}
	if completed.Workspace.Status != "completed" || completed.Workspace.Workflow.Phase != "completed" {
		t.Fatalf("completion state: %+v", completed.Workspace)
	}
}

// A plan-first workspace with no live implementer completes only with an
// explicit reason.
func TestCompletePlanFirstNothingToIntegrate(t *testing.T) {
	ctx := context.Background()
	s, ws := planFirstFixture(t)
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
	revision := currentRevision(t, s, ws)
	if _, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: revision}); err == nil {
		t.Fatal("completion without a reason was accepted")
	} else {
		expectCode(t, err, "reason_required")
	}
	completed, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: revision, Reason: "No implementation was required"})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Workspace.Status != "completed" {
		t.Fatalf("completion state: %s", completed.Workspace.Status)
	}
	if _, err := s.Archive(ctx, ws); err != nil {
		t.Fatalf("archive failed: %v", err)
	}
}

// Landed work refuses retry, input revision and a new integration prepare;
// complete then reopen still works.
func TestLandingGuardsAndReopen(t *testing.T) {
	ctx := context.Background()
	s, ws := planFirstFixture(t)
	integratePlanFirst(t, s, ws, "release")
	if _, err := s.LandIntegration(ctx, ws, LandingOptions{Target: "release"}); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	var implementer Task
	for _, task := range status.Workspace.Tasks {
		if task.Role == "implementer" {
			implementer = task
		}
	}
	if _, err := s.RetryTask(ctx, ws, implementer.ID, "more work", "retry:post-land"); err == nil {
		t.Fatal("retry after landing was accepted")
	} else {
		expectCode(t, err, "workspace_completed")
	}
	revision := currentRevision(t, s, ws)
	if _, err := s.UpdateInput(ctx, ws, RevisionOptions{Input: "new issue text", Reason: "revise", ExpectedRevision: revision}); err == nil {
		t.Fatal("input revision after landing was accepted")
	} else {
		expectCode(t, err, "workspace_completed")
	}
	if _, err := s.PrepareIntegration(ctx, ws, IntegrationOptions{OperationKey: "integration:post-land"}); err == nil {
		t.Fatal("integration prepare after landing was accepted")
	} else {
		expectCode(t, err, "workspace_completed")
	}
	completed, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{ExpectedRevision: currentRevision(t, s, ws)})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "User requested more work", ExpectedRevision: completed.Workspace.Revision}, "reopen:post-land")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Workspace.Status != "active" || reopened.Workspace.Integration != nil || reopened.Workspace.Workflow.Phase != "planning" {
		t.Fatalf("reopen state: %+v", reopened.Workspace)
	}
}

// Completed v1 plan-first workspaces archive without a release; workflows that
// declare a release gate still require it.
func TestArchiveRequiresReleaseOnlyWhenDeclared(t *testing.T) {
	ctx := context.Background()
	t.Run("v1 plan-first", func(t *testing.T) {
		s, ws := planFirstFixture(t)
		if err := s.With(ctx, ws, func(d *Document) error {
			d.State.Status = "completed"
			d.State.Workflow.Phase = "completed"
			d.State.Workflow.Capabilities = []string{capTasks, capPhases, capPlanner, capImplementer, capPlannerDepends}
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
		archived, err := s.Archive(ctx, ws)
		if err != nil {
			t.Fatalf("v1 plan-first archive failed: %v", err)
		}
		if archived.Workspace.Status != "archived" {
			t.Fatalf("archive state: %s", archived.Workspace.Status)
		}
	})
	t.Run("release workflow", func(t *testing.T) {
		s, ws := fixture(t)
		if err := s.With(ctx, ws, func(d *Document) error {
			d.State.Status = "completed"
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Archive(ctx, ws); err == nil {
			t.Fatal("release workflow archived without release")
		} else {
			expectCode(t, err, "release_required")
		}
	})
}

func documentFor(t *testing.T, s *Service, ws string) *Document {
	t.Helper()
	var out *Document
	if err := s.With(context.Background(), ws, func(d *Document) error {
		out = d
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func readBody(t *testing.T, s *Service, ws string) string {
	t.Helper()
	return documentFor(t, s, ws).Body
}
