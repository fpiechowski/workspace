package terminal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"workspace/internal/core"
)

// Navigator is the terminal boundary used by the CLI and interactive UI.
type Navigator interface {
	Select(context.Context, core.NavigationTarget) error
	Attach(context.Context, core.NavigationTarget) error
	OpenDedicated(context.Context, core.NavigationTarget) error
}

type CommandRunner interface {
	Output(context.Context, string, ...string) (string, error)
	Run(context.Context, io.Reader, io.Writer, io.Writer, string, ...string) error
}

type OSCommandRunner struct{}

func (OSCommandRunner) Output(ctx context.Context, socket string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", tmuxArgs(socket, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (OSCommandRunner) Run(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, socket string, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", tmuxArgs(socket, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return cmd.Run()
}

type TmuxNavigator struct {
	Socket            string
	Runner            CommandRunner
	Launcher          Launcher
	Env               func(string) string
	Stdin             io.Reader
	Stdout            io.Writer
	Stderr            io.Writer
	dedicatedMu       sync.Mutex
	dedicatedLaunches map[string]time.Time
}

func NewTmuxNavigator(socket string) *TmuxNavigator {
	return &TmuxNavigator{
		Socket: socket, Runner: OSCommandRunner{}, Launcher: NewDefaultLauncher(), Env: os.Getenv,
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		dedicatedLaunches: make(map[string]time.Time),
	}
}

func tmuxArgs(socket string, args ...string) []string {
	if socket != "" {
		return append([]string{"-L", socket}, args...)
	}
	return args
}

func (n *TmuxNavigator) runner() CommandRunner {
	if n.Runner != nil {
		return n.Runner
	}
	return OSCommandRunner{}
}

func (n *TmuxNavigator) env(key string) string {
	if n.Env != nil {
		return n.Env(key)
	}
	return os.Getenv(key)
}

func (n *TmuxNavigator) targetSocket(target core.NavigationTarget) string {
	if target.Socket != "" {
		return target.Socket
	}
	return n.Socket
}

func (n *TmuxNavigator) verify(ctx context.Context, target core.NavigationTarget) error {
	if target.WorkspaceID == "" || target.SessionName == "" {
		return &core.Error{Code: "invalid_navigation_target", Message: "workspace and tmux session are required"}
	}
	if target.SessionName != core.TmuxName(target.WorkspaceID) {
		return &core.Error{Code: "invalid_navigation_target", Message: "tmux session is not the canonical workspace session"}
	}
	socket := n.targetSocket(target)
	if _, err := n.runner().Output(ctx, socket, "has-session", "-t", "="+target.SessionName); err != nil {
		return &core.Error{Code: "pane_missing", Message: err.Error()}
	}
	if target.WindowID != "" {
		windows, err := n.runner().Output(ctx, socket, "list-windows", "-t", "="+target.SessionName, "-F", "#{window_id}")
		if err != nil {
			return err
		}
		if !containsLine(windows, target.WindowID) {
			return &core.Error{Code: "pane_mismatch", Message: "window no longer belongs to the selected workspace session"}
		}
	}
	if target.PaneID == "" {
		return nil
	}
	rows, err := n.runner().Output(ctx, socket, "list-panes", "-a", "-t", "="+target.SessionName, "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_id}\t#{@workspace_kind}\t#{@workspace_session_id}\t#{@workspace_run_id}")
	if err != nil {
		return err
	}
	for _, row := range strings.Split(rows, "\n") {
		fields := strings.SplitN(row, "\t", 7)
		if len(fields) != 7 || fields[0] != target.PaneID || fields[2] == "1" {
			continue
		}
		if target.WindowID != "" && fields[1] != target.WindowID {
			break
		}
		if fields[3] != "" && fields[3] != target.WorkspaceID {
			break
		}
		if target.Kind == "service" && (fields[4] != "service" || fields[5] != target.ServiceID) {
			break
		}
		if target.Kind != "service" && target.SessionID != "" && fields[4] == "agent" &&
			!(fields[5] == target.SessionID && fields[6] == target.RunID) &&
			!(fields[5] == "" && fields[6] == target.RunID) {
			break
		}
		return nil
	}
	return &core.Error{Code: "pane_mismatch", Message: "pane no longer belongs to the selected workspace entity"}
}

// Select moves the attached client to a verified target without starting an
// interactive tmux process or accepting a user-controlled target string.
func (n *TmuxNavigator) Select(ctx context.Context, target core.NavigationTarget) error {
	if err := n.verify(ctx, target); err != nil {
		return err
	}
	socket := n.targetSocket(target)
	if n.env("TMUX") == "" {
		return &core.Error{Code: "not_attached", Message: "select requires a terminal attached to tmux; use attach outside tmux"}
	}
	if err := n.verifyCurrentServer(ctx, socket); err != nil {
		return err
	}
	client, err := n.attachedClient(ctx, socket)
	if err != nil {
		return err
	}
	if _, err := n.runner().Output(ctx, socket, "switch-client", "-c", client, "-t", "="+target.SessionName); err != nil {
		return err
	}
	if target.WindowID != "" {
		if _, err := n.runner().Output(ctx, socket, "select-window", "-t", target.WindowID); err != nil {
			return err
		}
	}
	if target.PaneID != "" {
		if _, err := n.runner().Output(ctx, socket, "select-pane", "-t", target.PaneID); err != nil {
			return err
		}
	}
	return nil
}

// Attach uses tmux's attached client when one is available; otherwise it
// starts the workspace session with the caller's terminal streams.
func (n *TmuxNavigator) Attach(ctx context.Context, target core.NavigationTarget) error {
	if n.env("TMUX") != "" {
		return n.Select(ctx, target)
	}
	if err := n.verify(ctx, target); err != nil {
		return err
	}
	socket := n.targetSocket(target)
	if target.WindowID != "" {
		if _, err := n.runner().Output(ctx, socket, "select-window", "-t", target.WindowID); err != nil {
			return err
		}
	}
	if target.PaneID != "" {
		if _, err := n.runner().Output(ctx, socket, "select-pane", "-t", target.PaneID); err != nil {
			return err
		}
	}
	return n.runner().Run(ctx, n.Stdin, n.Stdout, n.Stderr, socket, "attach-session", "-t", "="+target.SessionName)
}

// PrepareAttach verifies the target and prepares an interactive command for
// the CLI's terminal-restoring handoff loop.
func (n *TmuxNavigator) PrepareAttach(ctx context.Context, target core.NavigationTarget) (*exec.Cmd, error) {
	if n.env("TMUX") != "" {
		return nil, &core.Error{Code: "tmux_server_mismatch", Message: "interactive attach must start outside tmux; use jump from an attached TUI"}
	}
	if err := n.verify(ctx, target); err != nil {
		return nil, err
	}
	socket := n.targetSocket(target)
	if target.WindowID != "" {
		if _, err := n.runner().Output(ctx, socket, "select-window", "-t", target.WindowID); err != nil {
			return nil, err
		}
	}
	if target.PaneID != "" {
		if _, err := n.runner().Output(ctx, socket, "select-pane", "-t", target.PaneID); err != nil {
			return nil, err
		}
	}
	return exec.Command("tmux", tmuxArgs(socket, "attach-session", "-t", "="+target.SessionName)...), nil
}

func (n *TmuxNavigator) verifyCurrentServer(ctx context.Context, socket string) error {
	tmuxEnv := n.env("TMUX")
	if tmuxEnv == "" {
		return nil
	}
	currentSocket := strings.SplitN(tmuxEnv, ",", 2)[0]
	expected := socket
	if expected == "" {
		expected = "default"
	}
	if filepath.Base(currentSocket) != expected {
		return &core.Error{Code: "tmux_server_mismatch", Message: "attach from outside the current tmux server to use this workspace socket"}
	}
	return nil
}

func (n *TmuxNavigator) attachedClient(ctx context.Context, socket string) (string, error) {
	pane := n.env("TMUX_PANE")
	if pane == "" {
		return "", &core.Error{Code: "not_attached", Message: "the current terminal is not attached to a tmux pane"}
	}
	rows, err := n.runner().Output(ctx, socket, "list-clients", "-F", "#{client_tty}")
	if err != nil {
		return "", err
	}
	var matches []string
	for _, row := range strings.Split(rows, "\n") {
		client := strings.TrimSpace(row)
		if client == "" {
			continue
		}
		clientPane, paneErr := n.runner().Output(ctx, socket, "display-message", "-p", "-c", client, "#{pane_id}")
		if paneErr != nil {
			return "", paneErr
		}
		if strings.TrimSpace(clientPane) == pane {
			matches = append(matches, client)
		}
	}
	if len(matches) == 0 {
		return "", &core.Error{Code: "no_attached_client", Message: "no attached tmux client is viewing this pane"}
	}
	if len(matches) > 1 {
		return "", &core.Error{Code: "navigation_ambiguous", Message: "multiple attached clients view this pane"}
	}
	return matches[0], nil
}

func containsLine(lines, value string) bool {
	for _, line := range strings.Split(lines, "\n") {
		if line == value {
			return true
		}
	}
	return false
}
