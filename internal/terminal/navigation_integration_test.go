package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"workspace/internal/core"
)

func TestNavigatorAttachPTYHelper(t *testing.T) {
	if os.Getenv("WORKSPACE_ATTACH_PTY_HELPER") != "1" {
		t.Skip("helper process")
	}
	target := core.NavigationTarget{
		WorkspaceID: os.Getenv("WORKSPACE_ATTACH_WORKSPACE"),
		SessionName: os.Getenv("WORKSPACE_ATTACH_SESSION"),
		Socket:      os.Getenv("WORKSPACE_ATTACH_SOCKET"),
		WindowID:    os.Getenv("WORKSPACE_ATTACH_WINDOW"),
		PaneID:      os.Getenv("WORKSPACE_ATTACH_PANE"),
		Kind:        "session",
		SessionID:   os.Getenv("WORKSPACE_ATTACH_SESSION_ID"),
		RunID:       os.Getenv("WORKSPACE_ATTACH_RUN_ID"),
	}
	if err := NewTmuxNavigator(target.Socket).Attach(context.Background(), target); err != nil {
		t.Fatal(err)
	}
}

func TestNavigatorRealPTYAndClientSelection(t *testing.T) {
	if os.Getenv("WORKSPACE_TMUX_TEST") != "1" {
		t.Skip("set WORKSPACE_TMUX_TEST=1 to run real tmux integration")
	}
	if runtime.GOOS == "windows" {
		t.Skip("tmux integration runs in Linux/WSL or macOS")
	}
	for _, program := range []string{"tmux", "script"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	socket := fmt.Sprintf("workspace-navigation-%d", os.Getpid())
	otherSocket := socket + "-other"
	for _, name := range []string{socket, otherSocket} {
		name := name
		t.Cleanup(func() { _ = exec.Command("tmux", "-L", name, "kill-server").Run() })
	}
	workspaceID := "ws_navigation"
	sessionName := "workspace-" + workspaceID
	sourcePane, sourceWindow := createNavigationSession(t, socket, sessionName)
	targetPane := strings.TrimSpace(tmuxOutput(t, socket, "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", sourceWindow, "sleep 60"))
	setNavigationPaneMetadata(t, socket, sourcePane, workspaceID, "sess_source", "run_source")
	setNavigationPaneMetadata(t, socket, targetPane, workspaceID, "sess_target", "run_target")
	target := core.NavigationTarget{
		WorkspaceID: workspaceID, SessionName: sessionName, Socket: socket,
		WindowID: sourceWindow, PaneID: targetPane, Kind: "session", SessionID: "sess_target", RunID: "run_target",
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	attach := startScriptClient(t, socket, shellQuote(executable)+" -test.run=^TestNavigatorAttachPTYHelper$", append(os.Environ(),
		"WORKSPACE_ATTACH_PTY_HELPER=1",
		"WORKSPACE_ATTACH_WORKSPACE="+workspaceID,
		"WORKSPACE_ATTACH_SESSION="+sessionName,
		"WORKSPACE_ATTACH_SOCKET="+socket,
		"WORKSPACE_ATTACH_WINDOW="+sourceWindow,
		"WORKSPACE_ATTACH_PANE="+targetPane,
		"WORKSPACE_ATTACH_SESSION_ID=sess_target",
		"WORKSPACE_ATTACH_RUN_ID=run_target",
	))
	detachScriptClient(t, socket, attach)
	waitClientCount(t, socket, 0)

	if active := strings.TrimSpace(tmuxOutput(t, socket, "display-message", "-p", "-t", sourceWindow, "#{pane_id}")); active != targetPane {
		t.Fatalf("pseudo-TTY attach did not select the verified target pane: got %s want %s", active, targetPane)
	}
	if _, err := exec.Command("tmux", "-L", socket, "select-pane", "-t", sourcePane).CombinedOutput(); err != nil {
		t.Fatal(err)
	}
	plainAttach := shellQuote("tmux") + " -L " + shellQuote(socket) + " attach-session -t " + shellQuote("="+sessionName)
	clientA := startScriptClient(t, socket, plainAttach, os.Environ())
	clientB := startScriptClient(t, socket, plainAttach, os.Environ())
	for _, client := range []*scriptClient{clientA, clientB} {
		tmuxOutput(t, socket, "switch-client", "-c", client.tty, "-t", sourcePane)
	}

	socketPath := strings.TrimSpace(tmuxOutput(t, socket, "display-message", "-p", "#{socket_path}"))
	env := map[string]string{"TMUX": socketPath + ",1,0", "TMUX_PANE": sourcePane}
	navigator := NewTmuxNavigator(socket)
	navigator.Env = func(key string) string { return env[key] }
	err = navigator.Select(ctx, target)
	expectNavigationCode(t, err, "navigation_ambiguous")

	detachScriptClient(t, socket, clientB)
	waitClientCount(t, socket, 1)
	if err := navigator.Select(ctx, target); err != nil {
		t.Fatal(err)
	}
	clientPane := strings.TrimSpace(tmuxOutput(t, socket, "display-message", "-p", "-c", clientA.tty, "#{pane_id}"))
	if clientPane != targetPane {
		t.Fatalf("navigator did not switch the sole matching client: got %q want %q", clientPane, targetPane)
	}
	detachScriptClient(t, socket, clientA)
	waitClientCount(t, socket, 0)
	expectNavigationCode(t, navigator.Select(ctx, target), "no_attached_client")

	_, _ = createNavigationSession(t, otherSocket, "workspace-other")
	otherSocketPath := strings.TrimSpace(tmuxOutput(t, otherSocket, "display-message", "-p", "#{socket_path}"))
	env["TMUX"] = otherSocketPath + ",1,0"
	expectNavigationCode(t, navigator.Select(ctx, target), "tmux_server_mismatch")
}

type scriptClient struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	tty   string
}

func startScriptClient(t *testing.T, socket, command string, env []string) *scriptClient {
	t.Helper()
	before := tmuxClients(socket)
	cmd := exec.Command("script", "-qfec", command, "/dev/null")
	cmd.Env = env
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	client := &scriptClient{cmd: cmd, stdin: stdin}
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for tty := range tmuxClients(socket) {
			if !before[tty] {
				client.tty = tty
				return client
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("pseudo-TTY client did not attach")
	return client
}

func detachScriptClient(t *testing.T, socket string, client *scriptClient) {
	t.Helper()
	if output, err := exec.Command("tmux", "-L", socket, "detach-client", "-t", client.tty).CombinedOutput(); err != nil {
		t.Fatalf("detach tmux client %s: %v\n%s", client.tty, err, output)
	}
	_ = client.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- client.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pseudo-TTY client did not detach cleanly: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = client.cmd.Process.Kill()
		t.Fatal("pseudo-TTY client did not exit after tmux detach")
	}
}

func tmuxClients(socket string) map[string]bool {
	clients := map[string]bool{}
	output, err := exec.Command("tmux", "-L", socket, "list-clients", "-F", "#{client_tty}").CombinedOutput()
	if err != nil {
		return clients
	}
	for _, tty := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if tty != "" {
			clients[tty] = true
		}
	}
	return clients
}

func waitClientCount(t *testing.T, socket string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cmd := exec.Command("tmux", "-L", socket, "list-clients", "-F", "#{client_tty}")
		output, err := cmd.CombinedOutput()
		count := 0
		if err == nil && strings.TrimSpace(string(output)) != "" {
			count = len(strings.Split(strings.TrimSpace(string(output)), "\n"))
		}
		if count == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("tmux client count did not become %d", want)
}

func createNavigationSession(t *testing.T, socket, session string) (string, string) {
	t.Helper()
	output := tmuxOutput(t, socket, "new-session", "-d", "-P", "-F", "#{pane_id}\t#{window_id}", "-x", "100", "-y", "30", "-s", session, "sleep 60")
	fields := strings.Split(strings.TrimSpace(output), "\t")
	if len(fields) != 2 {
		t.Fatalf("unexpected tmux session output: %q", output)
	}
	return fields[0], fields[1]
}

func setNavigationPaneMetadata(t *testing.T, socket, pane, workspace, session, run string) {
	t.Helper()
	for key, value := range map[string]string{
		"@workspace_id": workspace, "@workspace_kind": "agent", "@workspace_session_id": session, "@workspace_run_id": run,
	} {
		tmuxOutput(t, socket, "set-option", "-p", "-t", pane, key, value)
	}
}

func tmuxOutput(t *testing.T, socket string, args ...string) string {
	t.Helper()
	cmd := exec.Command("tmux", append([]string{"-L", socket}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func expectNavigationCode(t *testing.T, err error, code string) {
	t.Helper()
	var coreError *core.Error
	if !errors.As(err, &coreError) || coreError.Code != code {
		t.Fatalf("wanted %s, got %v", code, err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
