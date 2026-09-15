package bootstrap

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"workspace/internal/core"
)

func bootstrapProject(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := core.InitProject(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	s := &core.Service{Root: root, Runtime: core.Tmux{}, Executable: "workspace"}
	created, err := s.Create(context.Background(), core.CreateOptions{Title: "Example", Input: "intent", Workflow: "plan-first"})
	if err != nil {
		t.Fatal(err)
	}
	return root, created.Workspace.ID, created.Directory
}

func TestResolveUsesSharedFlagEnvironmentAndCWDPrecedence(t *testing.T) {
	root, workspaceID, directory := bootstrapProject(t)
	fromWorkspace, err := Resolve(Request{Project: root, CWD: filepath.Join(directory, "inputs"), Env: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !fromWorkspace.ProjectFound || fromWorkspace.WorkspaceID != workspaceID || fromWorkspace.ProjectRoot != root {
		t.Fatalf("workspace cwd was not resolved: %+v", fromWorkspace)
	}
	fromFlags, err := Resolve(Request{Project: root, Workspace: workspaceID, CWD: root, Env: map[string]string{"WORKSPACE_ID": "ws_foreign"}})
	if err != nil {
		t.Fatal(err)
	}
	if fromFlags.WorkspaceID != workspaceID {
		t.Fatalf("explicit workspace did not take precedence: %+v", fromFlags)
	}
	fromEnv, err := Resolve(Request{Project: root, CWD: root, Env: map[string]string{"WORKSPACE_ID": workspaceID, "WORKSPACE_AGENT_ID": "agent_x", "WORKSPACE_SESSION_ID": "sess_x", "WORKSPACE_RUN_ID": "run_x"}})
	if err != nil {
		t.Fatal(err)
	}
	if fromEnv.WorkspaceID != workspaceID || fromEnv.Service.Actor.AgentID != "agent_x" || fromEnv.Service.Actor.RunID != "run_x" {
		t.Fatalf("environment scope/actor was not preserved: %+v", fromEnv)
	}
}

func TestResolveReturnsProjectOnlyAndRejectsForeignWorkspace(t *testing.T) {
	root, workspaceID, _ := bootstrapProject(t)
	projectOnly, err := Resolve(Request{Project: root, CWD: root, Env: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !projectOnly.ProjectFound || projectOnly.WorkspaceID != "" {
		t.Fatalf("project directory should select the picker: %+v", projectOnly)
	}
	if _, err := Resolve(Request{Project: root, Workspace: workspaceID, CWD: root, Env: map[string]string{"WORKSPACE_PROJECT_DIR": "wrong"}}); err != nil {
		t.Fatalf("explicit project should override its environment value: %v", err)
	}
	otherRoot, otherWorkspace, _ := bootstrapProject(t)
	if otherRoot == root {
		t.Fatal("test projects unexpectedly share a root")
	}
	_, err = Resolve(Request{Project: root, Workspace: otherWorkspace, CWD: root, Env: map[string]string{}})
	if err == nil {
		t.Fatal("accepted a workspace ID owned by another project")
	}
}

func TestResolveMakesMissingProjectADisplayableState(t *testing.T) {
	root := t.TempDir()
	scope, err := Resolve(Request{Project: root, CWD: root, Env: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if scope.ProjectFound || scope.ProjectError == nil || scope.Service != nil {
		t.Fatalf("missing project should be a displayable state: %+v", scope)
	}
}
