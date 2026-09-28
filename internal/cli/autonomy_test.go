package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"

	"workspace/internal/core"
)

// autonomyProject builds an initialized project whose default orchestrator
// profile uses one client. deliver toggles the deliver_argv that the autonomy
// precondition requires.
func autonomyProject(t *testing.T, deliver bool) string {
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
	client := core.Client{Adapter: "command", LaunchArgv: []string{"wrapper", "{prompt}"}}
	if deliver {
		client.DeliverArgv = []string{"deliver-wrapper", "{message_id}"}
	}
	cfg.Clients = map[string]core.Client{"test": client}
	cfg.Profiles = map[string]core.Profile{"frontier": {Routes: []core.Route{{ID: "frontier", Client: "test", Provider: "provider", Model: "model", MaxConcurrency: 1}}}}
	cfg.Defaults.OrchestratorProfile = "frontier"
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".workspace", "config.yaml"), b, 0600); err != nil {
		t.Fatal(err)
	}
	return project
}

func TestAutonomyCLICreateStatusEnableDisable(t *testing.T) {
	project := autonomyProject(t, true)
	var out, errOut bytes.Buffer

	if code := Execute([]string{"--json", "--project", project, "create", "--no-workflow", "--autonomous", "autonomy cli"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("autonomous create failed: %d %s", code, errOut.String())
	}
	var created struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	ws := created.Data.Workspace.ID
	if !created.OK || created.Data.Workspace.Autonomy == nil || created.Data.Workspace.Autonomy.State != "running" || created.Data.Workspace.Autonomy.Source != "create" {
		t.Fatalf("create response: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Execute([]string{"--json", "--project", project, "status", "--workspace", ws}, nil, &out, &errOut); code != 0 {
		t.Fatalf("status failed: %d %s", code, errOut.String())
	}
	var status struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Data.Workspace.Autonomy == nil || status.Data.Workspace.Autonomy.State != "running" {
		t.Fatalf("status did not expose autonomy: %s", out.String())
	}

	// A second, interactive workspace exercises enable and disable.
	out.Reset()
	errOut.Reset()
	if code := Execute([]string{"--json", "--project", project, "create", "--no-workflow", "interactive"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("interactive create failed: %d %s", code, errOut.String())
	}
	var interactive struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &interactive); err != nil {
		t.Fatal(err)
	}
	interactiveID := interactive.Data.Workspace.ID
	revision := interactive.Data.Workspace.Revision

	out.Reset()
	errOut.Reset()
	if code := Execute([]string{
		"--json", "--project", project, "autonomy", "enable", "--workspace", interactiveID,
		"--reason", "user approved unattended", "--expected-revision", strconv.Itoa(revision), "--operation-key", "enable:cli",
	}, nil, &out, &errOut); code != 0 {
		t.Fatalf("autonomy enable failed: %d %s", code, out.String())
	}
	var enabled struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &enabled); err != nil {
		t.Fatal(err)
	}
	if enabled.Data.Workspace.Autonomy == nil || enabled.Data.Workspace.Autonomy.State != "running" || enabled.Data.Workspace.Autonomy.Source != "enable" {
		t.Fatalf("enable response: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Execute([]string{
		"--json", "--project", project, "autonomy", "disable", "--workspace", interactiveID,
		"--reason", "user took control", "--expected-revision", strconv.Itoa(enabled.Data.Workspace.Revision), "--operation-key", "disable:cli",
	}, nil, &out, &errOut); code != 0 {
		t.Fatalf("autonomy disable failed: %d %s", code, errOut.String())
	}
	var disabled struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &disabled); err != nil {
		t.Fatal(err)
	}
	if disabled.Data.Workspace.Autonomy == nil || disabled.Data.Workspace.Autonomy.State != "disabled" {
		t.Fatalf("disable response: %s", out.String())
	}
}

func TestAutonomyCLIUnsupportedWithoutDelivery(t *testing.T) {
	project := autonomyProject(t, false)
	var out, errOut bytes.Buffer
	code := Execute([]string{"--json", "--project", project, "create", "--no-workflow", "--autonomous", "no delivery"}, nil, &out, &errOut)
	if code != 1 || !bytes.Contains(out.Bytes(), []byte(`"code":"autonomy_unsupported"`)) {
		t.Fatalf("autonomous create without delivery: code=%d output=%s", code, out.String())
	}
}
