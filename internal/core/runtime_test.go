package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDispatcherRunnerCommandExportsLaunchIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tmux runner commands use a POSIX shell")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}

	root := t.TempDir()
	projectDir := filepath.Join(root, "project dir with 'quotes'")
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(root, "runner output with 'quotes'.txt")
	scriptPath := filepath.Join(root, "runner with 'quotes'.sh")
	script := `#!/bin/sh
set -eu
{
    printf 'scope=%s\n' "$WORKSPACE_SCOPE"
    printf 'project_dir=%s\n' "$WORKSPACE_PROJECT_DIR"
    printf 'project_id=%s\n' "$WORKSPACE_PROJECT_ID"
    printf 'agent_id=%s\n' "$WORKSPACE_AGENT_ID"
    printf 'session_id=%s\n' "$WORKSPACE_SESSION_ID"
    printf 'run_id=%s\n' "$WORKSPACE_RUN_ID"
    printf 'role=%s\n' "$WORKSPACE_ROLE"
    printf 'tmux_socket=%s\n' "$WORKSPACE_TMUX_SOCKET"
    for arg do
        printf 'arg=%s\n' "$arg"
    done
} > "$RUNNER_PROBE_OUTPUT"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	launch := Launch{
		ProjectRoot: projectDir,
		ProjectID:   "project id with 'quote'",
		SessionID:   "session id with 'quote'",
		RunID:       "run id with spaces 'quote'",
		Executable:  scriptPath,
		Scope:       "project",
		AgentID:     "agent id with 'quote'",
		Role:        "dispatcher",
	}
	tmux := Tmux{Socket: "socket with 'quote'"}
	command := tmux.runnerCommand(launch)

	env := make([]string, 0, len(os.Environ())+9)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "WORKSPACE_") {
			env = append(env, entry)
		}
	}
	for _, entry := range []string{
		"WORKSPACE_SCOPE=workspace",
		"WORKSPACE_PROJECT_DIR=stale-project",
		"WORKSPACE_PROJECT_ID=stale-project-id",
		"WORKSPACE_AGENT_ID=stale-agent",
		"WORKSPACE_SESSION_ID=stale-session",
		"WORKSPACE_RUN_ID=stale-run",
		"WORKSPACE_ROLE=orchestrator",
		"WORKSPACE_TMUX_SOCKET=stale-socket",
		"RUNNER_PROBE_OUTPUT=" + outputPath,
	} {
		env = append(env, entry)
	}

	cmd := exec.Command(sh, "-c", command)
	cmd.Dir = projectDir
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("runner command failed: %v\n%s\ncommand: %s", err, output, command)
	}

	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	var args []string
	for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("malformed runner probe line %q", line)
		}
		if key == "arg" {
			args = append(args, value)
		} else {
			values[key] = value
		}
	}

	wantValues := map[string]string{
		"scope":       "project",
		"project_dir": projectDir,
		"project_id":  launch.ProjectID,
		"agent_id":    launch.AgentID,
		"session_id":  launch.SessionID,
		"run_id":      launch.RunID,
		"role":        "dispatcher",
		"tmux_socket": tmux.Socket,
	}
	for key, want := range wantValues {
		if values[key] != want {
			t.Errorf("%s=%q, want %q; command: %s", key, values[key], want, command)
		}
	}
	wantArgs := []string{"--project", projectDir, "--tmux-socket", tmux.Socket, "--scope", "project", "_dispatcher-exec", launch.RunID}
	if strings.Join(args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Errorf("runner argv=%q, want %q; command: %s", args, wantArgs, command)
	}
}
