package terminal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"workspace/internal/core"
)

// Navigator is the terminal boundary used by the CLI and interactive UI.
type Navigator interface {
	ListClients(context.Context, core.NavigationTarget) ([]Client, error)
	Jump(context.Context, core.NavigationTarget, Client) error
	Select(context.Context, core.NavigationTarget) error
	Attach(context.Context, core.NavigationTarget) error
}

// Client is a snapshot of one attached tmux client. TTY is tmux's explicit
// client target; PID and Created distinguish a reconnect that reuses a TTY.
// Session and the active IDs are captured to detect a client changing while a
// selection dialog is open.
type Client struct {
	TTY       string `json:"tty"`
	Session   string `json:"session"`
	SessionID string `json:"session_id,omitempty"`
	WindowID  string `json:"window_id,omitempty"`
	PaneID    string `json:"pane_id,omitempty"`
	PID       string `json:"pid,omitempty"`
	Name      string `json:"name,omitempty"`
	TermName  string `json:"term_name,omitempty"`
	Created   string `json:"created,omitempty"`
}

// SameClient reports whether two snapshots identify the same tmux client,
// excluding its current session/window/pane so a stored preference can survive
// that client moving elsewhere.
func (c Client) SameClient(other Client) bool {
	if c.TTY == "" || c.TTY != other.TTY {
		return false
	}
	return sameOptionalIdentity(c.PID, other.PID) &&
		sameOptionalIdentity(c.Created, other.Created) &&
		sameOptionalIdentity(c.Name, other.Name)
}

// SameSnapshot also checks the state shown in the picker. Jump uses it after
// the user confirms so a changed client must be selected again.
func (c Client) SameSnapshot(other Client) bool {
	return c.SameClient(other) && c.Session == other.Session &&
		c.SessionID == other.SessionID && c.WindowID == other.WindowID && c.PaneID == other.PaneID
}

func sameOptionalIdentity(a, b string) bool {
	if a == "" && b == "" {
		return true
	}
	return a != "" && a == b
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
	Socket string
	Runner CommandRunner
	Env    func(string) string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func NewTmuxNavigator(socket string) *TmuxNavigator {
	return &TmuxNavigator{
		Socket: socket, Runner: OSCommandRunner{}, Env: os.Getenv,
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
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
	if target.Kind == "dispatcher" {
		return n.verifyDispatcher(ctx, target)
	}
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

// verifyDispatcher validates a project-scoped Dispatcher target. The session
// name must be the canonical project Dispatcher session and any pane must carry
// the durable current Run's project ownership metadata.
func (n *TmuxNavigator) verifyDispatcher(ctx context.Context, target core.NavigationTarget) error {
	if target.ProjectID == "" || target.SessionName == "" {
		return &core.Error{Code: "invalid_navigation_target", Message: "project and tmux session are required"}
	}
	if target.SessionName != core.DispatcherTmuxName(target.ProjectID) {
		return &core.Error{Code: "invalid_navigation_target", Message: "tmux session is not the canonical project Dispatcher session"}
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
			return &core.Error{Code: "pane_mismatch", Message: "window no longer belongs to the selected Dispatcher session"}
		}
	}
	if target.PaneID == "" {
		return nil
	}
	rows, err := n.runner().Output(ctx, socket, "list-panes", "-a", "-t", "="+target.SessionName, "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_scope}\t#{@workspace_project_id}\t#{@workspace_kind}\t#{@workspace_session_id}\t#{@workspace_run_id}")
	if err != nil {
		return err
	}
	for _, row := range strings.Split(rows, "\n") {
		fields := strings.SplitN(row, "\t", 8)
		if len(fields) != 8 || fields[0] != target.PaneID || fields[2] == "1" {
			continue
		}
		if fields[3] != "project" || fields[4] != target.ProjectID || fields[5] != "dispatcher" {
			break
		}
		if fields[6] != target.SessionID || fields[7] != target.RunID {
			break
		}
		if target.WindowID != "" && fields[1] != target.WindowID {
			break
		}
		return nil
	}
	return &core.Error{Code: "pane_mismatch", Message: "pane no longer belongs to the selected Dispatcher"}
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

// ListClients returns every live client on the verified target's tmux server.
// It deliberately works when the caller is outside tmux; the TUI selects the
// target client explicitly instead of attaching its own terminal.
func (n *TmuxNavigator) ListClients(ctx context.Context, target core.NavigationTarget) ([]Client, error) {
	if err := n.verify(ctx, target); err != nil {
		return nil, err
	}
	socket := n.targetSocket(target)
	if err := n.verifyCurrentServer(ctx, socket); err != nil {
		return nil, err
	}
	rows, err := n.runner().Output(ctx, socket, "list-clients", "-F", clientFormat)
	if err != nil {
		if isNoAttachedClients(err) {
			return nil, nil
		}
		return nil, &core.Error{Code: "client_discovery_failed", Message: "could not discover attached tmux clients: " + err.Error()}
	}
	clients, err := parseClients(rows)
	if err != nil {
		return nil, &core.Error{Code: "client_discovery_failed", Message: "could not read attached tmux clients: " + err.Error()}
	}
	return clients, nil
}

const clientFormat = "#{client_tty}\t#{client_session}\t#{session_id}\t#{window_id}\t#{pane_id}\t#{client_pid}\t#{client_name}\t#{client_termname}\t#{client_created}"

func parseClients(output string) ([]Client, error) {
	var clients []Client
	for _, row := range strings.Split(strings.TrimSpace(output), "\n") {
		if row == "" {
			continue
		}
		fields := strings.Split(row, "\t")
		if len(fields) != 9 || strings.TrimSpace(fields[0]) == "" {
			return nil, fmt.Errorf("unexpected list-clients row")
		}
		if _, err := strconv.ParseInt(fields[5], 10, 64); fields[5] != "" && err != nil {
			return nil, fmt.Errorf("invalid client process ID")
		}
		clients = append(clients, Client{
			TTY: fields[0], Session: fields[1], SessionID: fields[2], WindowID: fields[3], PaneID: fields[4],
			PID: fields[5], Name: fields[6], TermName: fields[7], Created: fields[8],
		})
	}
	return clients, nil
}

func isNoAttachedClients(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no clients") || strings.Contains(message, "no client")
}

// Jump switches exactly the chosen live client to the verified target. The
// client is re-read immediately before the first tmux mutation; a detached,
// restarted, or otherwise changed client is never replaced with another one.
func (n *TmuxNavigator) Jump(ctx context.Context, target core.NavigationTarget, chosen Client) error {
	if chosen.TTY == "" {
		return &core.Error{Code: "invalid_client", Message: "an attached tmux client must be selected"}
	}
	socket := n.targetSocket(target)
	if err := n.verifyCurrentServer(ctx, socket); err != nil {
		return err
	}
	if err := n.verify(ctx, target); err != nil {
		return err
	}
	clients, err := n.listClients(ctx, socket)
	if err != nil {
		return err
	}
	current, ok := findClientSnapshot(clients, chosen)
	if !ok {
		return &core.Error{Code: "client_gone", Message: "the selected tmux client detached or restarted; press g and choose a live client"}
	}
	if !chosen.SameSnapshot(current) {
		return &core.Error{Code: "client_changed", Message: "the selected tmux client changed while the picker was open; press g and choose again"}
	}
	// Verify the exact pane and window again after client discovery, just before
	// using the explicit tmux client identity.
	if err := n.verify(ctx, target); err != nil {
		return err
	}
	clients, err = n.listClients(ctx, socket)
	if err != nil {
		return err
	}
	if current, ok = findClientSnapshot(clients, chosen); !ok {
		return &core.Error{Code: "client_gone", Message: "the selected tmux client detached or changed before the jump; press g and choose a live client"}
	}
	if !chosen.SameSnapshot(current) {
		return &core.Error{Code: "client_changed", Message: "the selected tmux client changed before the jump; press g and choose again"}
	}
	if _, err := n.runner().Output(ctx, socket, "switch-client", "-c", chosen.TTY, "-t", "="+target.SessionName); err != nil {
		if live, listErr := n.listClients(ctx, socket); listErr == nil {
			if current, ok := findClientSnapshot(live, chosen); !ok {
				return &core.Error{Code: "client_gone", Message: "the selected tmux client detached or restarted before the jump; press g and choose a live client"}
			} else if !chosen.SameSnapshot(current) {
				return &core.Error{Code: "client_changed", Message: "the selected tmux client changed before the jump; press g and choose again"}
			}
		}
		return fmt.Errorf("switch selected tmux client: %w", err)
	}
	if target.WindowID != "" {
		windowTarget := "=" + target.SessionName + ":" + target.WindowID
		if _, err := n.runner().Output(ctx, socket, "select-window", "-t", windowTarget); err != nil {
			return fmt.Errorf("select verified tmux window: %w", err)
		}
	}
	if target.PaneID != "" {
		if _, err := n.runner().Output(ctx, socket, "select-pane", "-t", target.PaneID); err != nil {
			return fmt.Errorf("select verified tmux pane: %w", err)
		}
	}
	return nil
}

func (n *TmuxNavigator) listClients(ctx context.Context, socket string) ([]Client, error) {
	rows, err := n.runner().Output(ctx, socket, "list-clients", "-F", clientFormat)
	if err != nil {
		if isNoAttachedClients(err) {
			return nil, nil
		}
		return nil, &core.Error{Code: "client_discovery_failed", Message: "could not verify the selected tmux client: " + err.Error()}
	}
	clients, err := parseClients(rows)
	if err != nil {
		return nil, &core.Error{Code: "client_discovery_failed", Message: "could not verify the selected tmux client: " + err.Error()}
	}
	return clients, nil
}

func findClientSnapshot(clients []Client, chosen Client) (Client, bool) {
	for _, client := range clients {
		if chosen.SameClient(client) {
			return client, true
		}
	}
	return Client{}, false
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
		return &core.Error{Code: "tmux_server_mismatch", Message: "the TUI is attached to a different tmux server than this workspace; run it outside that server to choose a workspace client"}
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
