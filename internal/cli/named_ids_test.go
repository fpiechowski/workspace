package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"workspace/internal/core"
)

func namedIDsProject(t *testing.T) string {
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
	return project
}

type cliEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *core.Error     `json:"error"`
}

func runNamedIDCLI(t *testing.T, project string, args ...string) (int, cliEnvelope, string) {
	t.Helper()
	full := append([]string{"--json", "--project", project}, args...)
	var out, errOut bytes.Buffer
	code := Execute(full, nil, &out, &errOut)
	env := cliEnvelope{}
	if out.Len() > 0 {
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatalf("decode %q: %v", out.String(), err)
		}
	}
	return code, env, errOut.String()
}

func decodeData[T any](t *testing.T, env cliEnvelope) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(env.Data, &value); err != nil {
		t.Fatalf("decode data %s: %v", string(env.Data), err)
	}
	return value
}

func TestCLIExplicitWorkspaceAgentAndTaskIDs(t *testing.T) {
	project := namedIDsProject(t)

	code, env, stderr := runNamedIDCLI(t, project, "create", "--no-workflow", "--title", "Named IDs", "issue body")
	if code != 0 || !env.OK {
		t.Fatalf("create: %d %s", code, stderr)
	}
	if got := decodeData[core.Status](t, env).Workspace.ID; got != "ws_named-ids" {
		t.Fatalf("auto workspace id = %q, want ws_named-ids", got)
	}

	code, env, stderr = runNamedIDCLI(t, project, "create", "--no-workflow", "--title", "Custom", "--id", "custom", "issue body")
	if code != 0 || !env.OK {
		t.Fatalf("create --id: %d %s", code, stderr)
	}
	wsID := decodeData[core.Status](t, env).Workspace.ID
	if wsID != "ws_custom" {
		t.Fatalf("explicit workspace id = %q, want ws_custom", wsID)
	}

	code, env, _ = runNamedIDCLI(t, project, "create", "--no-workflow", "--title", "Again", "--id", "custom", "issue body")
	if code == 0 || env.OK || env.Error == nil || env.Error.Code != "id_exists" {
		t.Fatalf("duplicate workspace id: code=%d env=%+v", code, env)
	}

	code, env, _ = runNamedIDCLI(t, project, "create", "--no-workflow", "--title", "Bad", "--id", "Not A Slug", "issue body")
	if code == 0 || env.Error == nil || env.Error.Code != "invalid_id" {
		t.Fatalf("invalid workspace id: code=%d env=%+v", code, env)
	}

	code, env, stderr = runNamedIDCLI(t, project, "--workspace", wsID, "agent", "create", "planner", "--role", "planner", "--id", "agent_planner-one")
	if code != 0 || !env.OK {
		t.Fatalf("agent create: %d %s", code, stderr)
	}
	if got := decodeData[core.Agent](t, env).ID; got != "agent_planner-one" {
		t.Fatalf("agent id = %q, want agent_planner-one", got)
	}

	code, env, _ = runNamedIDCLI(t, project, "--workspace", wsID, "agent", "create", "planner-two", "--role", "planner", "--id", "planner-one")
	if code == 0 || env.Error == nil || env.Error.Code != "id_exists" {
		t.Fatalf("duplicate agent id: code=%d env=%+v", code, env)
	}

	specFile := filepath.Join(t.TempDir(), "task.yaml")
	spec := "title: Bounded task\nrole: implementer\ngoal: Do the bounded thing\nacceptance_criteria:\n  - It works\n"
	if err := os.WriteFile(specFile, []byte(spec), 0600); err != nil {
		t.Fatal(err)
	}
	code, env, stderr = runNamedIDCLI(t, project, "--workspace", wsID, "task", "create", "--spec-file", specFile, "--id", "custom-task")
	if code != 0 || !env.OK {
		t.Fatalf("task create: %d %s", code, stderr)
	}
	var task core.Task
	task = decodeData[core.Task](t, env)
	if task.ID != "task_custom-task" {
		t.Fatalf("task id = %q, want task_custom-task", task.ID)
	}

	code, env, _ = runNamedIDCLI(t, project, "--workspace", wsID, "task", "create", "--spec-file", specFile, "--id", "custom-task")
	if code == 0 || env.Error == nil || env.Error.Code != "id_exists" {
		t.Fatalf("duplicate task id: code=%d env=%+v", code, env)
	}
}

func TestCLIIssueDispatchExplicitID(t *testing.T) {
	project := namedIDsProject(t)

	code, env, stderr := runNamedIDCLI(t, project, "issue", "create", "--title", "Dispatch me", "issue body")
	if code != 0 || !env.OK {
		t.Fatalf("issue create: %d %s", code, stderr)
	}
	issueID := decodeData[core.Issue](t, env).ID

	code, env, stderr = runNamedIDCLI(t, project, "issue", "dispatch", issueID, "--no-workflow", "--id", "dispatched")
	if code != 0 || !env.OK {
		t.Fatalf("issue dispatch: %d %s", code, stderr)
	}
	result := decodeData[core.DispatchResult](t, env)
	if result.Workspace.Workspace.ID != "ws_dispatched" {
		t.Fatalf("dispatched workspace id = %q, want ws_dispatched", result.Workspace.Workspace.ID)
	}

	code, env, _ = runNamedIDCLI(t, project, "issue", "dispatch", issueID, "--no-workflow", "--id", "dispatched")
	if code == 0 || env.Error == nil || env.Error.Code != "id_exists" {
		t.Fatalf("duplicate dispatch id: code=%d env=%+v", code, env)
	}
}

func TestCLISessionStartHasExplicitIDFlag(t *testing.T) {
	root := newRoot(&options{})
	cmd := commandAtPath(root, "workspace session start")
	if cmd == nil || cmd.Flags().Lookup("id") == nil {
		t.Fatal("session start is missing the --id flag")
	}
}

func TestChooseWorkspaceExactIDPrecedence(t *testing.T) {
	statuses := []core.Status{
		{Workspace: core.Workspace{ID: "ws_named-ids", Title: "Named IDs", Status: "active"}},
		{Workspace: core.Workspace{ID: "ws_legacy-abc", Title: "ws_named-ids", Status: "active"}},
		{Workspace: core.Workspace{ID: "ws_01M3Q00000000000000000000", Title: "Legacy ULID", Status: "active"}},
	}
	o := &options{}
	cases := map[string]string{
		"ws_named-ids":                 "ws_named-ids",
		"ws_legacy-abc":                "ws_legacy-abc",
		"ws_01M3Q00000000000000000000": "ws_01M3Q00000000000000000000",
		"Legacy ULID":                  "ws_01M3Q00000000000000000000",
	}
	for selector, want := range cases {
		selected, err := chooseWorkspace(o, statuses, selector)
		if err != nil {
			t.Fatalf("chooseWorkspace(%q): %v", selector, err)
		}
		if selected.Workspace.ID != want {
			t.Errorf("chooseWorkspace(%q) = %q, want %q", selector, selected.Workspace.ID, want)
		}
	}
	if _, err := chooseWorkspace(o, statuses, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing selector error = %v", err)
	}
}
