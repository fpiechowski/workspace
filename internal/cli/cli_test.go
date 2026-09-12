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

func TestStructuredErrorsAndWorkflowDiscovery(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Execute([]string{"--json", "workflow", "list"}, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("%s", errOut.String())
	}
	var response struct {
		OK   bool `json:"ok"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || len(response.Data) != 1 || response.Data[0].ID != "plan-first" {
		t.Fatal("invalid workflow list")
	}
	out.Reset()
	errOut.Reset()
	code = Execute([]string{"--json", "--project", t.TempDir(), "status"}, nil, &out, &errOut)
	if code != 1 || !bytes.Contains(out.Bytes(), []byte(`"code":"project_not_found"`)) {
		t.Fatalf("expected structured error: %d %s", code, out.String())
	}
}

func TestCreateAcceptsIntentArgument(t *testing.T) {
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

	const intent = "create workspace improvements"
	var out, errOut bytes.Buffer
	code := Execute([]string{"--json", "--project", project, "create", intent}, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("create failed: %d %s", code, errOut.String())
	}
	var response struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Data.Directory == "" {
		t.Fatalf("invalid create response: %s", out.String())
	}
	input, err := os.ReadFile(filepath.Join(response.Data.Directory, "inputs", "issue.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(input) != intent {
		t.Fatalf("saved intent %q, want %q", input, intent)
	}
}

func TestWorkspaceShortNamesAndSelector(t *testing.T) {
	workspaces := []core.Status{
		{Workspace: core.Workspace{ID: "ws_one", Title: "First", Status: "active"}},
		{Workspace: core.Workspace{ID: "ws_two", Title: "Second", Status: "paused"}},
	}
	short := workspaceNameIDs(workspaces)
	if short["First"] != "ws_one" || short["Second"] != "ws_two" {
		t.Fatalf("unexpected short workspace map: %#v", short)
	}

	o := &options{in: strings.NewReader("2\n"), out: &bytes.Buffer{}}
	selected, err := chooseWorkspace(o, workspaces, "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Workspace.ID != "ws_two" {
		t.Fatalf("selected %s, want ws_two", selected.Workspace.ID)
	}
	selected, err = chooseWorkspace(o, workspaces, "First")
	if err != nil || selected.Workspace.ID != "ws_one" {
		t.Fatalf("title lookup: %v %#v", err, selected)
	}
}
