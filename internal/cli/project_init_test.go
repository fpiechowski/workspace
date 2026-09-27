package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"workspace/internal/core"
)

func initGitFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, output)
	}
	return dir
}

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
	dir := initGitFixture(t)
	if _, err := runProjectInitWizard(context.Background(), dir, strings.NewReader("n\n"), &strings.Builder{}); err != errInitCancelled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestProjectInitWizardRetriesInvalidAccountAndConcurrency(t *testing.T) {
	dir := initGitFixture(t)
	answers := strings.NewReader("y\nopencode\n\nYOUR_PROVIDER\nprovider\nYOUR_MODEL\nprovider/model\n\n0\n2\nn\nn\ny\n")
	cfg, err := runProjectInitWizard(context.Background(), dir, answers, &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	route := cfg.Profiles["orchestrator"].Routes[0]
	if route.Provider != "provider" || route.Model != "provider/model" || route.MaxConcurrency != 2 {
		t.Fatalf("invalid values were persisted: %#v", route)
	}
}

func TestProjectInitReinitPreservesBytesAndSkipsPrompts(t *testing.T) {
	dir := initGitFixture(t)
	if _, err := core.InitProject(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".workspace", "config.yaml")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	custom := append([]byte("# preserve this comment\n"), original...)
	if err := os.WriteFile(path, custom, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := Execute([]string{"--project", dir, "project", "init"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("re-init failed: %d %s", code, errOut.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(custom) || strings.Contains(out.String()+errOut.String(), "Start setup") {
		t.Fatalf("re-init changed bytes or prompted: output=%q", out.String()+errOut.String())
	}
}

func TestProjectInitScriptModesDoNotPrompt(t *testing.T) {
	for _, mode := range [][]string{{"--json"}, {"--short"}, {"--non-interactive"}, {"--operation-key", "scripted"}} {
		t.Run(strings.Join(mode, "-"), func(t *testing.T) {
			dir := initGitFixture(t)
			args := append([]string{}, mode...)
			args = append(args, "--project", dir, "project", "init")
			var out, errOut bytes.Buffer
			if code := Execute(args, strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("scripted init failed: %d %s", code, errOut.String())
			}
			if strings.Contains(out.String()+errOut.String(), "Start setup") {
				t.Fatalf("scripted init prompted: %q", out.String()+errOut.String())
			}
			if _, err := os.Stat(filepath.Join(dir, ".workspace", "config.yaml")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
