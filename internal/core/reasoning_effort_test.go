package core

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func saveReasoningConfig(t *testing.T, s *Service, update func(*Config)) Config {
	t.Helper()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	update(&cfg)
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestReasoningEffortConfigAndProfileInspection(t *testing.T) {
	s, _ := fixture(t)
	saveReasoningConfig(t, s, func(cfg *Config) {
		profile := cfg.Profiles["frontier"]
		profile.ReasoningEffort = " provider-medium "
		cfg.Profiles["frontier"] = profile
	})

	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Profiles["frontier"].ReasoningEffort; got != " provider-medium " {
		t.Fatalf("configured effort was normalized unexpectedly: %q", got)
	}
	profileJSON, err := json.Marshal(cfg.Profiles)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(profileJSON, []byte(`"reasoning_effort":" provider-medium "`)) {
		t.Fatalf("profile serialization omitted reasoning_effort: %s", profileJSON)
	}
	decision, err := s.ExplainProfile(context.Background(), "frontier")
	if err != nil {
		t.Fatal(err)
	}
	if decision.ReasoningEffort != " provider-medium " {
		t.Fatalf("profile explain omitted the configured effort: %+v", decision)
	}

	saveReasoningConfig(t, s, func(cfg *Config) {
		profile := cfg.Profiles["frontier"]
		profile.ReasoningEffort = " \t\n"
		cfg.Profiles["frontier"] = profile
	})
	_, err = s.Config()
	expectCode(t, err, "invalid_config")
}

func TestReasoningEffortSnapshotsAndResumeLineage(t *testing.T) {
	s, workspace := fixture(t)
	ctx := context.Background()
	saveReasoningConfig(t, s, func(cfg *Config) {
		profile := cfg.Profiles["frontier"]
		profile.ReasoningEffort = "initial-effort"
		cfg.Profiles["frontier"] = profile
	})
	a, w := worker(t, s, workspace, "reasoning-lineage")
	first, err := s.StartSession(ctx, workspace, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if first.ReasoningEffort != "initial-effort" || first.RoutingDecision == nil || first.RoutingDecision.ReasoningEffort != "initial-effort" {
		t.Fatalf("new session did not snapshot the profile effort: %+v", first)
	}
	status, err := s.Status(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	firstRun, err := findRunInStatus(status, first.CurrentRunID)
	if err != nil {
		t.Fatal(err)
	}
	if firstRun.ReasoningEffort != "initial-effort" {
		t.Fatalf("new Run did not snapshot the profile effort: %+v", firstRun)
	}
	if _, err := s.StopSession(ctx, workspace, first.ID); err != nil {
		t.Fatal(err)
	}

	saveReasoningConfig(t, s, func(cfg *Config) {
		profile := cfg.Profiles["frontier"]
		profile.ReasoningEffort = "changed-effort"
		cfg.Profiles["frontier"] = profile
	})
	resumed, err := s.ResumeAgent(ctx, workspace, a.ID, "reasoning-resume")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID != first.ID || resumed.ReasoningEffort != "initial-effort" || resumed.RoutingDecision == nil || resumed.RoutingDecision.ReasoningEffort != "initial-effort" {
		t.Fatalf("resume adopted changed config instead of session provenance: %+v", resumed)
	}
	status, err = s.Status(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	resumedRun, err := findRunInStatus(status, resumed.CurrentRunID)
	if err != nil {
		t.Fatal(err)
	}
	if resumedRun.ReasoningEffort != "initial-effort" {
		t.Fatalf("resumed Run changed reasoning effort: %+v", resumedRun)
	}
	if _, err := s.StopSession(ctx, workspace, resumed.ID); err != nil {
		t.Fatal(err)
	}

	newSession, err := s.StartSession(ctx, workspace, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if newSession.ID == first.ID || newSession.ReasoningEffort != "changed-effort" {
		t.Fatalf("new logical session did not adopt current profile effort: %+v", newSession)
	}
}

func TestReasoningEffortCommandEnvAndOptionalArgv(t *testing.T) {
	s, workspace := fixture(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	saveReasoningConfig(t, s, func(cfg *Config) {
		profile := cfg.Profiles["frontier"]
		profile.ReasoningEffort = "wrapper-effort"
		cfg.Profiles["frontier"] = profile
		client := cfg.Clients["test"]
		client.LaunchArgv = []string{exe, "-test.run=TestWorkerProcess", "--", "--effort={reasoning_effort}", "{prompt_file}"}
		cfg.Clients["test"] = client
	})
	a, w := worker(t, s, workspace, "reasoning-wrapper")
	started, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slicesContain(started.Argv, "--effort=wrapper-effort") || started.ReasoningEffort != "wrapper-effort" {
		t.Fatalf("configured effort did not reach command argv: %+v", started)
	}
	var stdout bytes.Buffer
	if err := s.ExecuteSession(context.Background(), workspace, started.ID, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid command fixture output: %v: %s", err, stdout.String())
	}
	if result["effort"] != "wrapper-effort" {
		t.Fatalf("command wrapper did not receive WORKSPACE_REASONING_EFFORT: %+v", result)
	}

	without := expandClientArgv([]string{"wrapper", "--effort={reasoning_effort}", "--effort", "{reasoning_effort}", "{prompt_file}"}, Route{Model: "model"}, "prompt.md", "prompt", "", "")
	if !reflect.DeepEqual(without, []string{"wrapper", "prompt.md"}) {
		t.Fatalf("empty optional effort arguments were not removed cleanly: %#v", without)
	}
	unchanged := expandClientArgv([]string{"wrapper", "--model", "{model}", "{prompt_file}"}, Route{Model: "model"}, "prompt.md", "prompt", "", "")
	if !reflect.DeepEqual(unchanged, []string{"wrapper", "--model", "model", "prompt.md"}) {
		t.Fatalf("argv without effort changed unexpectedly: %#v", unchanged)
	}
}

func TestClaudeReasoningEffortIsAddedOnceForLaunchAndResume(t *testing.T) {
	for name, argv := range map[string][]string{
		"launch": {"claude", "--model", "{model}", "{prompt}"},
		"resume": {"claude", "--resume", "{thread_id}", "--model", "{model}", "{prompt}"},
	} {
		t.Run(name, func(t *testing.T) {
			got := expandClientArgv(addClaudeReasoningEffort(argv, "high"), Route{Model: "model"}, "prompt.md", "prompt", "thread", "high")
			if countEffortOptions(got) != 1 || !slicesContain(got, "high") {
				t.Fatalf("Claude argv did not contain one configured effort: %#v", got)
			}
		})
	}
	placeholder := expandClientArgv(addClaudeReasoningEffort([]string{"claude", "--effort={reasoning_effort}", "{prompt}"}, "high"), Route{}, "prompt.md", "prompt", "", "high")
	if countEffortOptions(placeholder) != 1 {
		t.Fatalf("Claude placeholder was duplicated: %#v", placeholder)
	}
	explicit := expandClientArgv(addClaudeReasoningEffort([]string{"claude", "--effort", "low", "{prompt}"}, "high"), Route{}, "prompt.md", "prompt", "", "high")
	if countEffortOptions(explicit) != 1 || !slicesContain(explicit, "low") {
		t.Fatalf("explicit Claude effort was duplicated or replaced: %#v", explicit)
	}
}

func TestOpenCodeReasoningEffortMergesConfigContent(t *testing.T) {
	original := []string{
		"OTHER=value",
		`OPENCODE_CONFIG_CONTENT={"theme":"dark","agent":{"build":{"model":"provider/model","permission":{"shell":"ask"}},"plan":{"variant":"low","prompt":"keep"}},"permission":{"edit":"deny"}}`,
	}
	got, err := withOpenCodeReasoningEffortEnv(original, []string{"opencode"}, "high")
	if err != nil {
		t.Fatal(err)
	}
	if effort, ok := lookupEnv(got, "WORKSPACE_REASONING_EFFORT"); !ok || effort != "high" {
		t.Fatalf("OpenCode reasoning environment was not set: %#v", got)
	}
	var root map[string]any
	content, _ := lookupEnv(got, "OPENCODE_CONFIG_CONTENT")
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		t.Fatal(err)
	}
	if root["theme"] != "dark" || root["permission"].(map[string]any)["edit"] != "deny" {
		t.Fatalf("unrelated OpenCode config was lost: %#v", root)
	}
	agents := root["agent"].(map[string]any)
	build := agents["build"].(map[string]any)
	if build["variant"] != "high" || build["model"] != "provider/model" || build["permission"].(map[string]any)["shell"] != "ask" {
		t.Fatalf("default OpenCode agent was not merged: %#v", build)
	}
	if plan := agents["plan"].(map[string]any); plan["variant"] != "low" || plan["prompt"] != "keep" {
		t.Fatalf("sibling OpenCode agent was changed: %#v", plan)
	}

	explicit, err := withOpenCodeReasoningEffortEnv([]string{`OPENCODE_CONFIG_CONTENT={"agent":{"plan":{"model":"x"},"build":{"variant":"old"}}}`}, []string{"opencode", "--agent=plan"}, "medium")
	if err != nil {
		t.Fatal(err)
	}
	var explicitRoot map[string]any
	explicitContent, _ := lookupEnv(explicit, "OPENCODE_CONFIG_CONTENT")
	_ = json.Unmarshal([]byte(explicitContent), &explicitRoot)
	if explicitRoot["agent"].(map[string]any)["plan"].(map[string]any)["variant"] != "medium" {
		t.Fatalf("explicit OpenCode agent was not selected: %#v", explicitRoot)
	}

	if _, err := withOpenCodeReasoningEffortEnv([]string{"OPENCODE_CONFIG_CONTENT=not-json"}, []string{"opencode"}, "high"); err == nil {
		t.Fatal("malformed OpenCode config was silently replaced")
	}
	unchanged := []string{"OPENCODE_CONFIG_CONTENT=not-json"}
	without, err := withOpenCodeReasoningEffortEnv(unchanged, []string{"opencode"}, "")
	if err != nil || !reflect.DeepEqual(without, unchanged) {
		t.Fatalf("empty effort touched OpenCode environment: %#v, %v", without, err)
	}
}

func TestCodexReasoningEffortThreadParams(t *testing.T) {
	session := Session{CWD: "/tmp/project", Route: Route{Model: "provider/model"}, ClientSnapshot: Client{ThreadParams: map[string]any{"effort": "client-default", "approvalPolicy": "never"}}}
	configured := codexThreadParams(session, Run{ReasoningEffort: "profile-effort"})
	if configured["effort"] != "profile-effort" || configured["approvalPolicy"] != "never" {
		t.Fatalf("profile effort did not override client thread params: %#v", configured)
	}
	retained := codexThreadParams(session, Run{})
	if retained["effort"] != "client-default" {
		t.Fatalf("client thread_params effort was not retained when profile omitted it: %#v", retained)
	}
	noSource := codexThreadParams(Session{ClientSnapshot: Client{}}, Run{})
	if _, ok := noSource["effort"]; ok {
		t.Fatalf("Codex effort was injected without a configured source: %#v", noSource)
	}
}

func slicesContain(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func countEffortOptions(argv []string) int {
	count := 0
	for _, arg := range argv {
		if arg == "--effort" || strings.HasPrefix(arg, "--effort=") {
			count++
		}
	}
	return count
}
