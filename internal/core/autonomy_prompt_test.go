package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// orchestratorPrompt starts an orchestrator Run and returns the rendered prompt
// so a test can assert the exact guidance the binary delivered.
func orchestratorPrompt(t *testing.T, s *Service, ws, key string) string {
	t.Helper()
	ctx := context.Background()
	orch, err := s.StartOrchestrator(ctx, ws, key)
	if err != nil {
		t.Fatalf("start orchestrator: %v", err)
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	run, err := findRunInStatus(status, orch.CurrentRunID)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := os.ReadFile(run.PromptFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(prompt)
}

func TestAutonomousOrchestratorPromptCarriesNotice(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()

	auto, err := s.Create(ctx, CreateOptions{Title: "Autonomous prompt", Input: "unattended", Workflow: "plan-first", Autonomous: true, OperationKey: "prompt-autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	prompt := orchestratorPrompt(t, s, auto.Workspace.ID, "prompt-autonomous-start")
	for _, want := range []string{"Autonomous mode notice", "workspace autonomy report", "reserved for the user", "2 rejections and 1 retry"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("autonomous prompt missing %q:\n%s", want, prompt)
		}
	}

	agents, err := os.ReadFile(filepath.Join(auto.Directory, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "autonomous run") {
		t.Fatalf("autonomous create did not render the autonomy AGENTS.md branch: %s", agents)
	}
	workflow, err := os.ReadFile(filepath.Join(auto.Directory, "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(workflow), "running autonomously") {
		t.Fatalf("autonomous create did not render the autonomy WORKFLOW.md note: %s", workflow)
	}

	plain, err := s.Create(ctx, CreateOptions{Title: "Interactive prompt", Input: "bring the user", Workflow: "plan-first", OperationKey: "prompt-interactive"})
	if err != nil {
		t.Fatal(err)
	}
	plainPrompt := orchestratorPrompt(t, s, plain.Workspace.ID, "prompt-interactive-start")
	if strings.Contains(plainPrompt, "Autonomous mode notice") {
		t.Fatalf("interactive prompt leaked the autonomy notice:\n%s", plainPrompt)
	}
	plainAgents, err := os.ReadFile(filepath.Join(plain.Directory, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plainAgents), "Keep the conversation interactive") {
		t.Fatalf("interactive AGENTS.md lost the interactive branch: %s", plainAgents)
	}
}

func TestAutonomyNoticeSurvivesCustomizedTemplates(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()

	// A project may keep customized guidance that never mentions autonomy. The
	// notice is owned by the binary and must survive that.
	if err := os.WriteFile(filepath.Join(s.Root, ".workspace", "templates", "orchestrator.AGENTS.md.tmpl"), []byte("Custom orchestrator instructions with no autonomy wording.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, ".workspace", "templates", "workflows", "plan-first", "prompts", "orchestrator.md.tmpl"), []byte("Custom plan-first orchestrator prompt.\n"), 0600); err != nil {
		t.Fatal(err)
	}

	auto, err := s.Create(ctx, CreateOptions{Title: "Stale templates", Input: "unattended", Workflow: "plan-first", Autonomous: true, OperationKey: "prompt-stale"})
	if err != nil {
		t.Fatal(err)
	}
	agents, err := os.ReadFile(filepath.Join(auto.Directory, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "no autonomy wording") {
		t.Fatalf("custom AGENTS.md was not preserved: %s", agents)
	}
	prompt := orchestratorPrompt(t, s, auto.Workspace.ID, "prompt-stale-start")
	if !strings.Contains(prompt, "Autonomous mode notice") {
		t.Fatalf("binary-owned notice missing despite customized templates:\n%s", prompt)
	}
}

func TestAutonomyTemplateRenderingForEachCreationMode(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()

	manual, err := s.Create(ctx, CreateOptions{Title: "Autonomous manual", Input: "manual unattended", NoWorkflow: true, Autonomous: true, OperationKey: "prompt-manual"})
	if err != nil {
		t.Fatalf("manual autonomous create: %v", err)
	}
	manualWorkflow, err := os.ReadFile(filepath.Join(manual.Directory, "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manualWorkflow), "running autonomously") {
		t.Fatalf("manual WORKFLOW.md missing autonomy note: %s", manualWorkflow)
	}
	manualAgents, err := os.ReadFile(filepath.Join(manual.Directory, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manualAgents), "autonomous run") {
		t.Fatalf("manual AGENTS.md missing autonomy note: %s", manualAgents)
	}

	custom, err := s.Create(ctx, CreateOptions{Title: "Autonomous custom", Input: "custom workflow", Workflow: "extended", Autonomous: true, OperationKey: "prompt-custom"})
	if err != nil {
		t.Fatalf("custom autonomous create: %v", err)
	}
	if custom.Workspace.Workflow == nil || custom.Workspace.Workflow.ID != "extended" {
		t.Fatalf("custom workflow not selected: %+v", custom.Workspace.Workflow)
	}
}
