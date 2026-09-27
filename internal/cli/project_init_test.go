package cli

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestProjectInitWizardWritesSafeCandidate(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, output)
		}
	}
	answers := strings.NewReader("y\nopencode\n\naccount\naccount/model\n\n\n n\nn\ny\n")
	cfg, err := runProjectInitWizard(context.Background(), dir, answers, &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.OrchestratorProfile != "orchestrator" || cfg.Profiles["orchestrator"].Routes[0].Model != "account/model" {
		t.Fatalf("unexpected wizard config: %#v", cfg)
	}
	for _, arg := range append(cfg.Clients["opencode"].LaunchArgv, cfg.Clients["opencode"].ResumeArgv...) {
		if strings.Contains(arg, "--auto") || strings.Contains(arg, "dangerously-skip-permissions") {
			t.Fatalf("permission bypass in generated argv: %q", arg)
		}
	}
}

func TestProjectInitWizardCancellationDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, output)
	}
	if _, err := runProjectInitWizard(context.Background(), dir, strings.NewReader("n\n"), &strings.Builder{}); err != errInitCancelled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
