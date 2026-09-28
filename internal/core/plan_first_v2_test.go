package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

func planFirstIntegrationFixture(t *testing.T) (*Service, string, Task) {
	t.Helper()
	s, _ := fixture(t)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Plan-first v2", Input: "Integration stage", Workflow: "plan-first"})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	plan := plannedTask(t, s, ws, "plan", "planner", nil)
	pp, pw := startTask(t, s, ws, plan)
	h := submitPlan(t, s, ws, pp, pw)
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
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
	return s, ws, impl
}

// A new plan-first selection must snapshot the v2 capability set even when the
// project config still lists the v1 set, and implementing may only advance to
// integration after every live implementer is accepted.
func TestPlanFirstV2SnapshotsCapabilitiesAndGates(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Workflows = map[string]WorkflowConfig{"plan-first": {
		Profiles:     map[string]string{"orchestrator": "frontier", "planning": "frontier", "implementation": "implementation"},
		Capabilities: legacySnapshotCapabilities("plan-first"),
	}}
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	created, err := s.Create(ctx, CreateOptions{Title: "Plan-first v2", Input: "Capability union", Workflow: "plan-first"})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	caps := created.Workspace.Workflow.Capabilities
	for _, want := range []string{capIntegrator, capIntegration, capLanding} {
		if !slices.Contains(caps, want) {
			t.Fatalf("plan-first v2 snapshot is missing %s: %v", want, caps)
		}
	}

	plan := plannedTask(t, s, ws, "plan", "planner", nil)
	pp, pw := startTask(t, s, ws, plan)
	h := submitPlan(t, s, ws, pp, pw)
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceWorkflow(ctx, ws, "plan_review", "advance:plan-review"); err != nil {
		t.Fatal(err)
	}
	impl := plannedTask(t, s, ws, "implementation", "implementer", []string{plan.ID})
	if _, err := s.AdvanceWorkflow(ctx, ws, "implementing", "advance:implementing"); err != nil {
		t.Fatal(err)
	}
	_, err = s.AdvanceWorkflow(ctx, ws, "integration", "advance:premature")
	expectCode(t, err, "workflow_gate")

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
	status, err := s.AdvanceWorkflow(ctx, ws, "integration", "advance:integration")
	if err != nil {
		t.Fatal(err)
	}
	if status.Workspace.Workflow.Phase != "integration" {
		t.Fatalf("accepted implementer did not reach integration: %+v", status.Workspace)
	}
	_, err = s.AdvanceWorkflow(ctx, ws, "", "advance:in-integration")
	expectCode(t, err, "user_decision_required")
}

// v1 plan-first snapshots keep the frozen contract and still complete on the
// implementing advance, whether the v1 capabilities are explicit or empty.
func TestV1PlanFirstSnapshotStillCompletesAfterImplementation(t *testing.T) {
	for _, mode := range []string{"explicit", "empty"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := fixture(t)
			ctx := context.Background()
			created, err := s.Create(ctx, CreateOptions{Title: "v1 plan-first", Input: "Frozen contract", Workflow: "plan-first"})
			if err != nil {
				t.Fatal(err)
			}
			ws := created.Workspace.ID
			if err := s.With(ctx, ws, func(d *Document) error {
				if mode == "explicit" {
					d.State.Workflow.Capabilities = legacySnapshotCapabilities("plan-first")
				} else {
					d.State.Workflow.Capabilities = nil
				}
				return saveDocument(d)
			}); err != nil {
				t.Fatal(err)
			}
			plan := plannedTask(t, s, ws, "plan", "planner", nil)
			pp, pw := startTask(t, s, ws, plan)
			h := submitPlan(t, s, ws, pp, pw)
			if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
				t.Fatal(err)
			}
			advancePhase(t, s, ws, "plan_review")
			impl := plannedTask(t, s, ws, "implementation", "implementer", []string{plan.ID})
			advancePhase(t, s, ws, "implementing")
			ip, iw := startTask(t, s, ws, impl)
			if err := os.WriteFile(filepath.Join(iw.Path, "fix.txt"), []byte("v1\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := git(ctx, iw.Path, "add", "fix.txt"); err != nil {
				t.Fatal(err)
			}
			if _, err := git(ctx, iw.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "v1 fix"); err != nil {
				t.Fatal(err)
			}
			finishTask(t, s, ws, ip, iw, "IMPLEMENTATION.md")
			status, err := s.AdvanceWorkflow(ctx, ws, "completed", "advance:completed")
			if err != nil {
				t.Fatal(err)
			}
			if status.Workspace.Status != "completed" || status.Workspace.Workflow.Phase != "completed" {
				t.Fatalf("%s v1 snapshot did not complete: %+v", mode, status.Workspace)
			}
		})
	}
}

// Integration preparation in the landing phase targets the current tip of the
// target branch by default, records Target/Strategy, and rejects invalid
// targets without creating an integration.
func TestIntegrationPrepareTargetsTipForLandingWorkflow(t *testing.T) {
	s, ws, _ := planFirstIntegrationFixture(t)
	ctx := context.Background()
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	baseRef, frozen := status.Workspace.Base.Ref, status.Workspace.Base.Commit
	if _, err := git(ctx, s.Root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "target moved"); err != nil {
		t.Fatal(err)
	}
	tip, err := git(ctx, s.Root, "rev-parse", "--verify", baseRef+"^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	if tip == frozen {
		t.Fatal("fixture target did not move beyond the frozen base")
	}

	_, err = s.PrepareIntegration(ctx, ws, IntegrationOptions{Target: "-bad"})
	expectCode(t, err, "invalid_target")
	if _, err := s.PrepareIntegration(ctx, ws, IntegrationOptions{Target: "bad..name"}); err == nil {
		t.Fatal("invalid target branch was accepted")
	}

	integration, err := s.PrepareIntegration(ctx, ws, IntegrationOptions{OperationKey: "integration:default"})
	if err != nil {
		t.Fatal(err)
	}
	if integration.Target != baseRef || integration.Strategy != "merge" {
		t.Fatalf("landing integration metadata: %+v", integration)
	}
	if integration.BaseCommit != tip {
		t.Fatalf("default base must be the target tip: got %s want %s", integration.BaseCommit, tip)
	}

	if _, err := git(ctx, s.Root, "branch", "release", tip); err != nil {
		t.Fatal(err)
	}
	explicit, err := s.PrepareIntegration(ctx, ws, IntegrationOptions{Target: "release", OperationKey: "integration:release"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Target != "release" || explicit.BaseCommit != tip {
		t.Fatalf("explicit --target not honored: %+v", explicit)
	}
}

// Accepting the integrator handoff records the integrated HEAD on the
// workflow's Integration record and leaves the workspace in the integration
// phase.
func TestPlanFirstIntegratorAcceptanceRecordsHead(t *testing.T) {
	s, ws, impl := planFirstIntegrationFixture(t)
	ctx := context.Background()
	integration, err := s.PrepareIntegration(ctx, ws, IntegrationOptions{OperationKey: "integration:1"})
	if err != nil {
		t.Fatal(err)
	}
	task := plannedTask(t, s, ws, "integrator", "integrator", []string{impl.ID})
	// The dedicated plan-first integration prompt ships with the template
	// refresh; until then the existing implementation prompt starts the session.
	a, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "integrator", Role: "integrator", PromptTemplate: "implementation"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Task: task.ID, Worktree: integration.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx, ws)
	var w Worktree
	for _, wt := range status.Worktrees {
		if wt.ID == integration.WorktreeID {
			w = wt
		}
	}
	if _, err := git(ctx, w.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "merge", "--no-ff", "--no-edit", integration.Heads[0]); err != nil {
		t.Fatal(err)
	}
	finishTask(t, s, ws, p, w, "INTEGRATION.md")
	status, err = s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if status.Workspace.Integration == nil || status.Workspace.Integration.HeadCommit == "" {
		t.Fatalf("integrator acceptance did not record HeadCommit: %+v", status.Workspace.Integration)
	}
	_, err = s.AdvanceWorkflow(ctx, ws, "", "advance:integration")
	expectCode(t, err, "user_decision_required")
}

// Landing configuration combinations are validated, and a removed
// issue-resolution entry loads, is validated and is hidden from the normalized
// configuration.
func TestValidateConfigLandingRulesAndHidesIssueResolution(t *testing.T) {
	valid := freshCandidate()
	valid.ProjectID = "prj_test"
	valid.Workflows["plan-first"] = WorkflowConfig{
		Profiles:     map[string]string{"orchestrator": "orchestrator"},
		Capabilities: []string{capTasks, capPhases, capPlanner, capImplementer, capIntegrator, capIntegration, capLanding},
	}
	if _, err := ValidateConfig(valid); err != nil {
		t.Fatalf("valid landing config rejected: %v", err)
	}
	for name, caps := range map[string][]string{
		"without integration": {capTasks, capPlanner, capImplementer, capIntegrator, capLanding},
		"without integrator":  {capTasks, capPlanner, capImplementer, capIntegration, capLanding},
		"with change_request": {capTasks, capPlanner, capImplementer, capIntegrator, capIntegration, capLanding, capChangeRequest},
		"with live_test":      {capTasks, capPlanner, capImplementer, capIntegrator, capIntegration, capLanding, capLiveTest},
		"with release":        {capTasks, capPlanner, capImplementer, capIntegrator, capIntegration, capLanding, capRelease},
	} {
		cfg := freshCandidate()
		cfg.ProjectID = "prj_test"
		cfg.Workflows["plan-first"] = WorkflowConfig{Profiles: map[string]string{"orchestrator": "orchestrator"}, Capabilities: caps}
		if _, err := ValidateConfig(cfg); err == nil {
			t.Fatalf("landing config %s accepted", name)
		} else {
			expectCode(t, err, "invalid_config")
		}
	}

	cfg := freshCandidate()
	cfg.ProjectID = "prj_test"
	cfg.Workflows["issue-resolution"] = WorkflowConfig{Profiles: map[string]string{"orchestrator": "orchestrator"}}
	got, err := ValidateConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Workflows["issue-resolution"]; ok {
		t.Fatal("issue-resolution entry remained visible in the normalized config")
	}
	bad := freshCandidate()
	bad.ProjectID = "prj_test"
	bad.Workflows["issue-resolution"] = WorkflowConfig{Profiles: map[string]string{"orchestrator": "missing"}}
	if _, err := ValidateConfig(bad); err == nil {
		t.Fatal("invalid issue-resolution profile reference was ignored")
	} else {
		expectCode(t, err, "invalid_config")
	}
}

func TestWorkflowConfigCapabilitiesUnionsPlanFirstV2(t *testing.T) {
	cfg := Config{Workflows: map[string]WorkflowConfig{"plan-first": {Capabilities: legacySnapshotCapabilities("plan-first")}}}
	got := workflowConfigCapabilities("plan-first", cfg)
	for _, want := range []string{capIntegrator, capIntegration, capLanding} {
		if !slices.Contains(got, want) {
			t.Fatalf("plan-first v2 union missing %s: %v", want, got)
		}
	}
	if other := workflowConfigCapabilities("extended", cfg); len(other) != 0 {
		t.Fatalf("custom workflow gained capabilities: %v", other)
	}
	if !knownWorkflowCapability(capLanding) {
		t.Fatal("landing is not a known capability")
	}
}

func TestIntegratorProfileFallsBackToImplementation(t *testing.T) {
	cfg := Config{Workflows: map[string]WorkflowConfig{"plan-first": {Profiles: map[string]string{"implementation": "worker-profile"}}}}
	d := &Document{State: Workspace{Workflow: &Workflow{ID: "plan-first", Phase: "integration"}}}
	if got := workflowProfile(cfg, d, "integrator", "fallback"); got != "worker-profile" {
		t.Fatalf("integrator fallback = %q, want worker-profile", got)
	}
	cfg.Workflows["plan-first"] = WorkflowConfig{Profiles: map[string]string{"implementation": "worker-profile", "integration": "integrator-profile"}}
	if got := workflowProfile(cfg, d, "integrator", "fallback"); got != "integrator-profile" {
		t.Fatalf("explicit integration profile ignored: %q", got)
	}
}
