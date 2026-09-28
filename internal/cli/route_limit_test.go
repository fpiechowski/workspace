package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"workspace/internal/core"
)

func routeLimitProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := core.InitProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	s := &core.Service{Root: project}
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clients = map[string]core.Client{"test": {Adapter: "opencode", LaunchArgv: []string{"wrapper", "{prompt}"}}}
	cfg.Profiles = map[string]core.Profile{"worker": {Routes: []core.Route{{ID: "deepseek", Client: "test", Provider: "deepseek", Model: "deepseek/flash", MaxConcurrency: 1}}}}
	config, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".workspace", "config.yaml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	return project
}

func runJSON(t *testing.T, args ...string) (int, map[string]json.RawMessage) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Execute(append([]string{"--json"}, args...), nil, &out, &errOut)
	var response map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, out.String(), errOut.String())
	}
	return code, response
}

func TestProfileLimitCommands(t *testing.T) {
	for _, name := range []string{"WORKSPACE_ID", "WORKSPACE_AGENT_ID", "WORKSPACE_SESSION_ID", "WORKSPACE_RUN_ID", "WORKSPACE_SCOPE", "WORKSPACE_PROJECT_DIR"} {
		t.Setenv(name, "")
	}
	project := routeLimitProject(t)
	t.Chdir(project)
	if code, response := runJSON(t, "--project", project, "profile", "limit", "set", "worker/deepseek", "--kind", "rate_limited", "--for", "45m", "--message", "manual"); code != 0 {
		t.Fatalf("set failed: %s", response["error"])
	} else {
		var rec core.RouteLimit
		if err := json.Unmarshal(response["data"], &rec); err != nil || rec.Key != "provider:test/deepseek" || rec.Kind != "rate_limited" || rec.Source != "manual" {
			t.Fatalf("set result: %+v %v", rec, err)
		}
	}
	code, response := runJSON(t, "--project", project, "profile", "limit", "list")
	var list core.RouteLimitList
	if err := json.Unmarshal(response["data"], &list); code != 0 || err != nil || len(list.Limits) != 1 {
		t.Fatalf("list: %d %s", code, response["data"])
	}
	code, response = runJSON(t, "--project", project, "profile", "explain", "worker")
	var decision core.RoutingDecision
	if err := json.Unmarshal(response["data"], &decision); code != 0 || err != nil || decision.Candidates[0].LimitedUntil == nil || decision.Selected != "" {
		t.Fatalf("explain did not show the limit: %s", response["data"])
	}
	if code, response = runJSON(t, "--project", project, "profile", "limit", "clear", "worker/deepseek"); code != 0 {
		t.Fatalf("clear failed: %s", response["error"])
	}
	code, response = runJSON(t, "--project", project, "profile", "limit", "list")
	if err := json.Unmarshal(response["data"], &list); code != 0 || err != nil || len(list.Limits) != 0 {
		t.Fatalf("cleared record still active: %s", response["data"])
	}
	code, response = runJSON(t, "--project", project, "profile", "limit", "list", "--all")
	if err := json.Unmarshal(response["data"], &list); code != 0 || err != nil || len(list.Limits) != 1 {
		t.Fatalf("--all omitted the cleared record: %s", response["data"])
	}
	code, response = runJSON(t, "--project", project, "profile", "limit", "report", "--kind", "rate_limited")
	var failure core.Error
	if err := json.Unmarshal(response["error"], &failure); code == 0 || err != nil || failure.Code != "run_required" {
		t.Fatalf("report outside a Run: %d %s", code, response["error"])
	}
	code, response = runJSON(t, "--project", project, "profile", "limit", "set", "worker/deepseek", "--until", "tomorrow")
	if err := json.Unmarshal(response["error"], &failure); code == 0 || err != nil || failure.Code != "invalid_argument" {
		t.Fatalf("invalid --until: %d %s", code, response["error"])
	}
}
