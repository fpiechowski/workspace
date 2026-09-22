package terminal

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"workspace/internal/core"
)

func TestNavigatorRealDedicatedViewerReuse(t *testing.T) {
	if os.Getenv("WORKSPACE_TMUX_TEST") != "1" {
		t.Skip("set WORKSPACE_TMUX_TEST=1 to run real tmux integration")
	}
	if runtime.GOOS == "windows" {
		t.Skip("tmux integration runs in Linux/WSL or macOS")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatal(err)
	}

	socket := "workspace-dedicated-viewer-" + strings.ReplaceAll(t.Name(), "/", "-")
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	workspaceID := "ws_dedicated"
	sessionName := "workspace-" + workspaceID
	sourcePane, sourceWindow := createNavigationSession(t, socket, sessionName)
	targetPane := strings.TrimSpace(tmuxOutput(t, socket, "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", sourceWindow, "sleep 60"))
	setNavigationPaneMetadata(t, socket, sourcePane, workspaceID, "sess_source", "run_source")
	setNavigationPaneMetadata(t, socket, targetPane, workspaceID, "sess_target", "run_target")
	target := core.NavigationTarget{
		WorkspaceID: workspaceID, SessionName: sessionName, Socket: socket,
		WindowID: sourceWindow, PaneID: targetPane, Kind: "session", SessionID: "sess_target", RunID: "run_target",
	}
	launcher := &fakeDedicatedLauncher{}
	navigator := &TmuxNavigator{Socket: socket, Runner: OSCommandRunner{}, Launcher: launcher, Env: func(string) string { return "" }}
	if err := navigator.OpenDedicated(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("first dedicated open launched %d terminals", len(launcher.calls))
	}
	viewer := launcher.calls[0].Session
	client := startScriptClient(t, socket, shellQuote("tmux")+" -L "+shellQuote(socket)+" attach-session -t "+shellQuote("="+viewer), os.Environ())
	defer detachScriptClient(t, socket, client)
	waitClientCount(t, socket, 1)

	if err := navigator.OpenDedicated(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("reusing the viewer launched %d terminals", len(launcher.calls))
	}
	if got := strings.TrimSpace(tmuxOutput(t, socket, "display-message", "-p", "-c", client.tty, "#{pane_id}")); got != targetPane {
		t.Fatalf("viewer client selected pane %q, want %q", got, targetPane)
	}
}
