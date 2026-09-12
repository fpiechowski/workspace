package core

import (
	"context"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWorkflowProfilesAndParallelLimit(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Workflows = map[string]WorkflowConfig{"issue-resolution": {Profiles: map[string]string{"planning": "live-testing", "orchestrator": "implementation"}, MaxParallelTasks: 1}}
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	a, w := worker(t, s, id, "one")
	if a.Profile != "live-testing" {
		t.Fatal("agent ignores workflow profile")
	}
	task, err := s.CreateTask(ctx, id, TaskSpec{Title: "Plan", Goal: "Diagnose", Role: "planner", AcceptanceCriteria: []string{"Evidence"}}, "task")
	if err != nil {
		t.Fatal(err)
	}
	if task.Profile != "live-testing" {
		t.Fatal("task ignores workflow profile")
	}
	if _, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID, Task: task.ID}); err != nil {
		t.Fatal(err)
	}
	a2, w2 := worker(t, s, id, "two")
	_, err = s.StartSession(ctx, id, SessionOptions{Agent: a2.ID, Worktree: w2.ID})
	expectCode(t, err, "parallel_limit")
	orch, err := s.StartOrchestrator(ctx, id, "orch")
	if err != nil {
		t.Fatal(err)
	}
	if orch.Profile != "implementation" {
		t.Fatal("orchestrator ignores profile")
	}
}

func TestOrchestratorUsesGlobalDefaultAndWorkflowOverride(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Defaults.OrchestratorProfile = "live-testing"
	cfg.Workflows = map[string]WorkflowConfig{
		"issue-resolution": {
			Profiles: map[string]string{"orchestrator": "implementation"},
		},
	}
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}

	withoutWorkflow, err := s.Create(ctx, CreateOptions{Title: "No workflow", Input: "Choose later"})
	if err != nil {
		t.Fatal(err)
	}
	if got := withoutWorkflow.Agents[0].Profile; got != "live-testing" {
		t.Fatalf("global orchestrator profile not captured: got %q", got)
	}
	orch, err := s.StartOrchestrator(ctx, withoutWorkflow.Workspace.ID, "default-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	if orch.Profile != "live-testing" {
		t.Fatalf("global orchestrator profile not used: got %q", orch.Profile)
	}

	withWorkflow, err := s.Create(ctx, CreateOptions{Title: "With workflow", Input: "Use override", Workflow: "issue-resolution"})
	if err != nil {
		t.Fatal(err)
	}
	if got := withWorkflow.Agents[0].Profile; got != "implementation" {
		t.Fatalf("workflow orchestrator override not captured: got %q", got)
	}
	orch, err = s.StartOrchestrator(ctx, withWorkflow.Workspace.ID, "workflow-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	if orch.Profile != "implementation" {
		t.Fatalf("workflow orchestrator override not used: got %q", orch.Profile)
	}
}

func TestRoutingCapabilityAndCooldownDiagnostics(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles["frontier"]
	p.RequiredCapabilities = []string{"deliver"}
	cfg.Profiles["frontier"] = p
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	d, err := s.ExplainProfile(ctx, "frontier")
	if err != nil {
		t.Fatal(err)
	}
	if d.Selected != "" || d.Candidates[0].Reason != "missing capability: deliver" {
		t.Fatalf("unsupported delivery route: %+v", d)
	}
	p.RequiredCapabilities = nil
	cfg.Profiles["frontier"] = p
	b, _ = yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, id, func(d *Document) error {
		d.Registry.Sessions = append(d.Registry.Sessions, Session{ID: ID("sess"), State: "failed", Route: p.Routes[0], CreatedAt: nowUTC()})
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	d, err = s.ExplainProfile(ctx, "frontier")
	if err != nil {
		t.Fatal(err)
	}
	if d.Selected != "" || d.Candidates[0].CooldownUntil == nil {
		t.Fatalf("failed launch not cooled down: %+v", d)
	}
}

func TestRoutingLaunchBudget(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles["frontier"]
	p.Routes[0].MaxLaunches24h = 1
	cfg.Profiles["frontier"] = p
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartOrchestrator(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	d, err := s.ExplainProfile(ctx, "frontier")
	if err != nil {
		t.Fatal(err)
	}
	if d.Selected != "" || d.Candidates[0].Reason != "24h launch budget exhausted" {
		t.Fatal("launch reservation bypassed budget")
	}
}
