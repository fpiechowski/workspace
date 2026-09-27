package core

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
		if _, err := git(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func freshCandidate() Config {
	return Config{
		SchemaVersion: 1,
		Runtime:       "tmux",
		Clients:       map[string]Client{"codex": {Adapter: "codex"}},
		Profiles: map[string]Profile{
			"orchestrator": {Routes: []Route{{ID: "orchestrator", Client: "codex", Provider: "openai", Model: "gpt-5", MaxConcurrency: 3}}},
		},
		Defaults:  DefaultsConfig{OrchestratorProfile: "orchestrator"},
		Workflows: map[string]WorkflowConfig{"plan-first": {Profiles: map[string]string{"orchestrator": "orchestrator"}}},
	}
}

func claudeCandidate() Config {
	cfg := freshCandidate()
	cfg.Clients = map[string]Client{"claude": {Adapter: "claude"}}
	cfg.Profiles = map[string]Profile{
		"orchestrator": {Routes: []Route{{ID: "orchestrator", Client: "claude", Provider: "anthropic", Model: "claude-sonnet", MaxConcurrency: 2}}},
	}
	return cfg
}

func configPath(root string) string { return filepath.Join(root, ".workspace", "config.yaml") }

func TestValidateConfigAppliesFileRules(t *testing.T) {
	cfg := freshCandidate()
	cfg.ProjectID = "prj_test"
	got, err := ValidateConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if argv := got.Clients["codex"].LaunchArgv; len(argv) == 0 || argv[0] != "codex" {
		t.Fatalf("client normalization not applied: %+v", got.Clients["codex"])
	}

	unknownClient := freshCandidate()
	unknownClient.ProjectID = "prj_test"
	unknownClient.Clients["codex"] = Client{Adapter: "unknown"}
	if _, err := ValidateConfig(unknownClient); err == nil {
		t.Fatal("unknown adapter accepted")
	} else {
		expectCode(t, err, "invalid_config")
	}

	missingRouteClient := freshCandidate()
	missingRouteClient.ProjectID = "prj_test"
	missingRouteClient.Profiles["orchestrator"] = Profile{Routes: []Route{{ID: "orchestrator", Client: "missing", Provider: "p", Model: "m", MaxConcurrency: 1}}}
	if _, err := ValidateConfig(missingRouteClient); err == nil {
		t.Fatal("route reference to unknown client accepted")
	} else {
		expectCode(t, err, "invalid_config")
	}

	missingWorkflowProfile := freshCandidate()
	missingWorkflowProfile.ProjectID = "prj_test"
	missingWorkflowProfile.Workflows["plan-first"] = WorkflowConfig{Profiles: map[string]string{"orchestrator": "missing"}}
	if _, err := ValidateConfig(missingWorkflowProfile); err == nil {
		t.Fatal("workflow reference to unknown profile accepted")
	} else {
		expectCode(t, err, "invalid_config")
	}

	missingDefault := freshCandidate()
	missingDefault.ProjectID = "prj_test"
	missingDefault.Defaults.OrchestratorProfile = "missing"
	if _, err := ValidateConfig(missingDefault); err == nil {
		t.Fatal("default reference to unknown profile accepted")
	} else {
		expectCode(t, err, "invalid_config")
	}

	missingPrompt := freshCandidate()
	missingPrompt.ProjectID = "prj_test"
	missingPrompt.Clients["command"] = Client{Adapter: "command", LaunchArgv: []string{"tool", "no-placeholder"}}
	if _, err := ValidateConfig(missingPrompt); err == nil {
		t.Fatal("client without a prompt placeholder accepted")
	} else {
		expectCode(t, err, "invalid_config")
	}

	missingCore := freshCandidate()
	missingCore.ProjectID = ""
	if _, err := ValidateConfig(missingCore); err == nil {
		t.Fatal("missing project id accepted")
	} else {
		expectCode(t, err, "invalid_config")
	}
}

func TestInitFreshProjectWritesValidatedCandidate(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	cfg, err := InitFreshProject(ctx, dir, freshCandidate())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectID == "" {
		t.Fatal("generated project id missing")
	}
	loaded, err := (&Service{Root: dir}).Config()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProjectID != cfg.ProjectID || loaded.Defaults.OrchestratorProfile != "orchestrator" {
		t.Fatalf("persisted candidate differs: %+v", loaded)
	}
	if _, err := os.Stat(filepath.Join(dir, ".workspace", "templates", "WORKSPACE.md.tmpl")); err != nil {
		t.Fatal("templates not installed", err)
	}
	if _, err := git(ctx, dir, "check-ignore", ".workspace/.runtime/project.lock"); err != nil {
		t.Fatal("exclude rules not installed", err)
	}
}

func TestInitFreshProjectRejectsInvalidCandidateWithoutWrites(t *testing.T) {
	dir := gitRepo(t)
	bad := freshCandidate()
	bad.Profiles["orchestrator"] = Profile{Routes: []Route{{ID: "orchestrator", Client: "missing", Provider: "p", Model: "m", MaxConcurrency: 1}}}
	if _, err := InitFreshProject(context.Background(), dir, bad); err == nil {
		t.Fatal("invalid candidate accepted")
	} else {
		expectCode(t, err, "invalid_config")
	}
	if _, err := os.Stat(configPath(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid candidate wrote config")
	}
	if _, err := os.Stat(filepath.Join(dir, ".workspace", "templates")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid candidate wrote templates")
	}
}

func TestInitFreshProjectPreservesExistingConfig(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	first, err := InitFreshProject(ctx, dir, freshCandidate())
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(configPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	second, err := InitFreshProject(ctx, dir, claudeCandidate())
	if err != nil {
		t.Fatal(err)
	}
	if second.ProjectID != first.ProjectID {
		t.Fatal("fresh init replaced a pre-existing config")
	}
	after, err := os.ReadFile(configPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("existing config bytes changed")
	}
}

func TestInitFreshProjectKeyedReplayAndConflict(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	candidate := freshCandidate()
	first, err := InitFreshProject(ctx, dir, candidate, "setup")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := InitFreshProject(ctx, dir, candidate, "setup")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, replay) {
		t.Fatal("identical keyed init did not replay the original receipt")
	}
	_, err = InitFreshProject(ctx, dir, claudeCandidate(), "setup")
	expectCode(t, err, "operation_conflict")
	loaded, err := (&Service{Root: dir}).Config()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProjectID != first.ProjectID {
		t.Fatal("conflicted retry replaced the original config")
	}
}

func TestInitFreshProjectConcurrentPreservesExisting(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	first, err := InitFreshProject(ctx, dir, freshCandidate())
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(configPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			candidate := freshCandidate()
			if i%2 == 0 {
				candidate = claudeCandidate()
			}
			_, err := InitFreshProject(ctx, dir, candidate)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(configPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("concurrent init replaced the pre-existing config")
	}
	loaded, err := (&Service{Root: dir}).Config()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProjectID != first.ProjectID {
		t.Fatal("concurrent init changed the persisted project id")
	}
}
