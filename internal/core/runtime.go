package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Launch struct {
	Service                                                                                                        bool
	ProjectRoot, ProjectID, WorkspaceID, WorkspaceDir, SessionID, RunID, WorktreeID, WorktreeName, CWD, Executable string
	ReplacePaneID, ReplaceSessionID, ReplaceRunID, PreferredOrchestratorWindowID                                   string
	Orchestrator                                                                                                   bool
	Scope, AgentID, Role                                                                                           string
}
type Pane struct {
	ID, WindowID, SessionID, RunID                                    string
	Kind, WorkspaceID, ProjectID, AgentID, Role, Scope, UIID, UIToken string
	StartCommand, SessionName, WindowName                             string
	UIGeneration                                                      int
	Dead, Active                                                      bool
}
type TmuxWindow struct {
	ID, Name, Kind, WorktreeID string
	Width, Height              int
	Active                     bool
}
type TmuxTopology struct {
	WorkspaceID   string
	ProjectID     string
	Scope         string
	SessionName   string
	SessionExists bool
	Windows       []TmuxWindow
	Panes         []Pane
	ObservedAt    time.Time
}

type RuntimeTopologyReader interface {
	ObserveTopology(context.Context, string) (TmuxTopology, error)
}
type Runtime interface {
	Launch(context.Context, Launch) (Pane, error)
	Inspect(context.Context, string) (Pane, error)
	Stop(context.Context, string) error
	StopWorkspace(context.Context, string) error
	Attach(context.Context, string, string) error
}
type Tmux struct{ Socket string }

// DisplayMessage shows a short pending-inbox notice without changing pane
// input or selecting another tmux window.
func (t Tmux) DisplayMessage(ctx context.Context, paneID, message string) error {
	_, err := t.call(ctx, "display-message", "-t", paneID, message)
	return err
}

func (t Tmux) args(args ...string) []string {
	if t.Socket != "" {
		return append([]string{"-L", t.Socket}, args...)
	}
	return args
}
func (t Tmux) call(ctx context.Context, args ...string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", fail("runtime_unsupported", "run tmux sessions with the Linux build inside WSL")
	}
	b, err := exec.CommandContext(ctx, "tmux", t.args(args...)...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(b))
		if args[0] == "list-panes" && (strings.HasPrefix(message, "no server running on ") || (strings.HasPrefix(message, "error connecting to ") && strings.HasSuffix(message, "(No such file or directory)"))) {
			return "", fail("pane_missing", "tmux server is absent: %s", message)
		}
		return "", fail("tmux_error", "%s: %s", args[0], strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func TmuxName(id string) string { return "workspace-" + id }

func DispatcherTmuxName(projectID string) string { return "workspace-dispatcher-" + projectID }

func tmuxMissingSession(err error) bool {
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != "tmux_error" {
		return false
	}
	message := strings.ToLower(ce.Message)
	return strings.Contains(message, "no server running on ") || strings.Contains(message, "can't find session") || strings.Contains(message, "session not found")
}

// tmux accepts a shell command. Quote every argv element, including embedded quotes.
func shellQuote(arg string) string              { return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'" }
func shellAssignment(name, value string) string { return name + "=" + shellQuote(value) }
func (t Tmux) runnerCommand(l Launch) string {
	verb := "_session-exec"
	if l.Service {
		verb = "_service-exec"
	}
	executionID := l.RunID
	if executionID == "" {
		executionID = l.SessionID
	}
	args := []string{l.Executable, "--project", l.ProjectRoot, "--tmux-socket", t.Socket}
	if l.Scope == "project" {
		args = append(args, "--scope", "project", "_dispatcher-exec", executionID)
	} else {
		args = append(args, "--workspace", l.WorkspaceID, verb, executionID)
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	command := make([]string, 0, len(quoted)+9)
	if l.Scope == "project" {
		command = append(command,
			shellAssignment("WORKSPACE_SCOPE", "project"),
			shellAssignment("WORKSPACE_PROJECT_DIR", l.ProjectRoot),
			shellAssignment("WORKSPACE_PROJECT_ID", l.ProjectID),
			shellAssignment("WORKSPACE_AGENT_ID", l.AgentID),
			shellAssignment("WORKSPACE_SESSION_ID", l.SessionID),
			shellAssignment("WORKSPACE_RUN_ID", l.RunID),
			shellAssignment("WORKSPACE_ROLE", l.Role),
			shellAssignment("WORKSPACE_TMUX_SOCKET", t.Socket),
		)
	}
	command = append(command, "exec")
	command = append(command, quoted...)
	return strings.Join(command, " ")
}

func paneKind(l Launch) string {
	if l.Service {
		return "service"
	}
	return "agent"
}

func (t Tmux) setWindowMetadata(ctx context.Context, window string, l Launch, kind string) error {
	options := [][2]string{{"@workspace_window_kind", kind}, {"@workspace_id", l.WorkspaceID}, {"@workspace_project_id", l.ProjectID}}
	if l.Scope == "project" {
		options = [][2]string{{"@workspace_window_kind", "dispatcher"}, {"@workspace_scope", "project"}, {"@workspace_project_id", l.ProjectID}, {"@workspace_agent_id", l.AgentID}, {"@workspace_role", l.Role}}
	}
	if kind == "worktree" {
		options = append(options, [2]string{"@workspace_worktree_id", l.WorktreeID})
	}
	for _, option := range options {
		if option[1] == "" {
			continue
		}
		if _, err := t.call(ctx, "set-option", "-w", "-t", window, option[0], option[1]); err != nil {
			return err
		}
	}
	return nil
}

func (t Tmux) setRuntimeMetadata(ctx context.Context, l Launch, pane, window string) error {
	if _, err := t.call(ctx, "set-option", "-w", "-t", window, "remain-on-exit", "on"); err != nil {
		return err
	}
	if l.Scope == "project" {
		for _, option := range [][2]string{{"@workspace_scope", "project"}, {"@workspace_project_id", l.ProjectID}, {"@workspace_agent_id", l.AgentID}, {"@workspace_role", l.Role}, {"@workspace_session_id", l.SessionID}, {"@workspace_run_id", l.RunID}} {
			if option[1] == "" {
				continue
			}
			if _, err := t.call(ctx, "set-option", "-p", "-t", pane, option[0], option[1]); err != nil {
				return err
			}
		}
		if err := t.setWindowMetadata(ctx, window, l, "dispatcher"); err != nil {
			return err
		}
		if _, err := t.call(ctx, "set-option", "-p", "-t", pane, "@workspace_kind", "dispatcher"); err != nil {
			return err
		}
		return nil
	}
	if _, err := t.call(ctx, "set-option", "-t", pane, "@workspace_id", l.WorkspaceID); err != nil {
		return err
	}
	if l.ProjectID != "" {
		if _, err := t.call(ctx, "set-option", "-t", pane, "@workspace_project_id", l.ProjectID); err != nil {
			return err
		}
	}
	kind := "worktree"
	if l.Orchestrator {
		kind = "orchestrator"
	}
	if err := t.setWindowMetadata(ctx, window, l, kind); err != nil {
		return err
	}
	if _, err := t.call(ctx, "set-option", "-p", "-t", pane, "@workspace_kind", paneKind(l)); err != nil {
		return err
	}
	if _, err := t.call(ctx, "set-option", "-p", "-t", pane, "@workspace_id", l.WorkspaceID); err != nil {
		return err
	}
	if l.ProjectID != "" {
		if _, err := t.call(ctx, "set-option", "-p", "-t", pane, "@workspace_project_id", l.ProjectID); err != nil {
			return err
		}
	}
	if l.WorktreeID != "" {
		if _, err := t.call(ctx, "set-option", "-p", "-t", pane, "@workspace_worktree_id", l.WorktreeID); err != nil {
			return err
		}
	}
	if _, err := t.call(ctx, "set-option", "-p", "-t", pane, "@workspace_session_id", l.SessionID); err != nil {
		return err
	}
	if l.RunID != "" {
		if _, err := t.call(ctx, "set-option", "-p", "-t", pane, "@workspace_run_id", l.RunID); err != nil {
			return err
		}
	}
	return nil
}

func (t Tmux) legacyOrchestratorWindow(ctx context.Context, l Launch) bool {
	if l.PreferredOrchestratorWindowID == "" || l.ReplacePaneID == "" || l.ReplaceSessionID == "" || l.ReplaceRunID == "" {
		return false
	}
	panes, err := t.call(ctx, "list-panes", "-t", l.PreferredOrchestratorWindowID, "-F", "#{pane_id}")
	if err != nil {
		return false
	}
	for _, id := range strings.Split(panes, "\n") {
		if id != l.ReplacePaneID {
			continue
		}
		pane, err := t.Inspect(ctx, id)
		if err != nil || !paneOwns(pane, l.ReplaceSessionID, l.ReplaceRunID) || pane.Kind == "tui" || pane.Kind == "service" {
			return false
		}
		return pane.WindowID == l.PreferredOrchestratorWindowID
	}
	return false
}

// ObserveTopology reads identity metadata and dimensions without changing tmux
// state or selecting a window. An absent server/session is a normal observation.
func (t Tmux) ObserveTopology(ctx context.Context, workspaceID string) (TmuxTopology, error) {
	out := TmuxTopology{WorkspaceID: workspaceID, SessionName: TmuxName(workspaceID), ObservedAt: nowUTC()}
	if runtime.GOOS == "windows" {
		return out, fail("runtime_unsupported", "run tmux sessions with the Linux build inside WSL")
	}
	if _, err := t.call(ctx, "has-session", "-t", "="+out.SessionName); err != nil {
		if tmuxMissingSession(err) {
			return out, nil
		}
		return out, err
	}
	out.SessionExists = true
	windows, err := t.call(ctx, "list-windows", "-t", "="+out.SessionName, "-F", "#{window_id}\t#{window_name}\t#{@workspace_window_kind}\t#{@workspace_id}\t#{@workspace_worktree_id}\t#{window_width}\t#{window_height}\t#{window_active}")
	if err != nil {
		return out, err
	}
	for _, line := range strings.Split(windows, "\n") {
		fields := strings.SplitN(line, "\t", 8)
		if len(fields) != 8 {
			continue
		}
		kind := fields[2]
		if kind == "" && fields[4] != "" {
			// Older worktree windows have @workspace_worktree_id but their
			// @workspace_kind was shadowed by the active pane's role.
			kind = "worktree"
		}
		width, _ := strconv.Atoi(fields[5])
		height, _ := strconv.Atoi(fields[6])
		out.Windows = append(out.Windows, TmuxWindow{ID: fields[0], Name: fields[1], Kind: kind, WorktreeID: fields[4], Width: width, Height: height, Active: fields[7] == "1"})
	}
	panes, err := t.call(ctx, "list-panes", "-a", "-t", "="+out.SessionName, "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{pane_active}\t#{@workspace_session_id}\t#{@workspace_run_id}\t#{@workspace_kind}\t#{@workspace_id}\t#{@workspace_ui_id}\t#{@workspace_ui_generation}\t#{@workspace_ui_token}\t#{pane_start_command}\t#{session_name}\t#{window_name}")
	if err != nil {
		return out, err
	}
	for _, line := range strings.Split(panes, "\n") {
		fields := strings.SplitN(line, "\t", 14)
		if len(fields) != 14 {
			continue
		}
		generation, _ := strconv.Atoi(fields[9])
		out.Panes = append(out.Panes, Pane{
			ID: fields[0], WindowID: fields[1], Dead: fields[2] == "1", Active: fields[3] == "1",
			SessionID: fields[4], RunID: fields[5], Kind: fields[6], WorkspaceID: fields[7], UIID: fields[8],
			UIGeneration: generation, UIToken: fields[10], StartCommand: fields[11], SessionName: fields[12], WindowName: fields[13],
		})
	}
	return out, nil
}

// ObserveProjectTopology is the project-scope counterpart to ObserveTopology.
// It deliberately reads @workspace_scope/project and never treats a pane with
// only a project ID as a Workspace pane.
func (t Tmux) ObserveProjectTopology(ctx context.Context, projectID string) (TmuxTopology, error) {
	out := TmuxTopology{ProjectID: projectID, Scope: "project", SessionName: DispatcherTmuxName(projectID), ObservedAt: nowUTC()}
	if runtime.GOOS == "windows" {
		return out, fail("runtime_unsupported", "run tmux sessions with the Linux build inside WSL")
	}
	if _, err := t.call(ctx, "has-session", "-t", "="+out.SessionName); err != nil {
		if tmuxMissingSession(err) {
			return out, nil
		}
		return out, err
	}
	out.SessionExists = true
	windows, err := t.call(ctx, "list-windows", "-t", "="+out.SessionName, "-F", "#{window_id}\t#{window_name}\t#{@workspace_window_kind}\t#{@workspace_scope}\t#{@workspace_project_id}\t#{window_width}\t#{window_height}\t#{window_active}")
	if err != nil {
		return out, err
	}
	for _, line := range strings.Split(windows, "\n") {
		fields := strings.SplitN(line, "\t", 8)
		if len(fields) != 8 || fields[3] != "project" || fields[4] != projectID {
			continue
		}
		width, _ := strconv.Atoi(fields[5])
		height, _ := strconv.Atoi(fields[6])
		out.Windows = append(out.Windows, TmuxWindow{ID: fields[0], Name: fields[1], Kind: fields[2], Width: width, Height: height, Active: fields[7] == "1"})
	}
	panes, err := t.call(ctx, "list-panes", "-a", "-t", "="+out.SessionName, "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{pane_active}\t#{@workspace_scope}\t#{@workspace_project_id}\t#{@workspace_agent_id}\t#{@workspace_role}\t#{@workspace_session_id}\t#{@workspace_run_id}\t#{@workspace_kind}\t#{pane_start_command}\t#{session_name}\t#{window_name}")
	if err != nil {
		return out, err
	}
	for _, line := range strings.Split(panes, "\n") {
		fields := strings.SplitN(line, "\t", 14)
		if len(fields) != 14 || fields[4] != "project" || fields[5] != projectID {
			continue
		}
		out.Panes = append(out.Panes, Pane{ID: fields[0], WindowID: fields[1], Dead: fields[2] == "1", Active: fields[3] == "1", Scope: fields[4], ProjectID: fields[5], AgentID: fields[6], Role: fields[7], SessionID: fields[8], RunID: fields[9], Kind: fields[10], StartCommand: fields[11], SessionName: fields[12], WindowName: fields[13]})
	}
	return out, nil
}

// Recover closes the gap between creating a pane and recording its ID. The
// exact runner command is unique to the concrete Run, including its ULID.
func (t Tmux) Recover(ctx context.Context, l Launch) (Pane, error) {
	if l.Scope == "project" {
		return t.recoverDispatcher(ctx, l)
	}
	out, err := t.call(ctx, "list-panes", "-a", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_session_id}\t#{@workspace_run_id}\t#{@workspace_kind}\t#{@workspace_id}\t#{@workspace_ui_id}\t#{@workspace_ui_generation}\t#{@workspace_ui_token}\t#{pane_start_command}")
	if err != nil {
		return Pane{}, err
	}
	for _, line := range strings.Split(out, "\n") {
		p := strings.SplitN(line, "\t", 11)
		if len(p) != 11 {
			continue
		}
		start := p[10]
		// tmux serializes the single shell-command argument with outer quotes.
		if decoded, err := strconv.Unquote(start); err == nil {
			start = decoded
		}
		legacyMetadata := l.RunID != "" && p[3] == l.RunID && p[4] == ""
		if p[5] == "tui" || (p[3] != l.SessionID || p[4] != l.RunID) && !legacyMetadata && start != t.runnerCommand(l) {
			continue
		}
		if err := t.setRuntimeMetadata(ctx, l, p[0], p[1]); err != nil {
			return Pane{}, err
		}
		return Pane{ID: p[0], WindowID: p[1], Dead: p[2] == "1", SessionID: l.SessionID, RunID: l.RunID, Kind: paneKind(l), WorkspaceID: l.WorkspaceID, StartCommand: start}, nil
	}
	return Pane{}, fail("pane_missing", "no pane for session %s", l.SessionID)
}
func (t Tmux) Launch(ctx context.Context, l Launch) (Pane, error) {
	if l.Scope == "project" {
		return t.launchDispatcher(ctx, l)
	}
	name := TmuxName(l.WorkspaceID)
	command := t.runnerCommand(l)
	format := "#{pane_id}\t#{window_id}"
	var output string
	var err error
	if _, err = t.call(ctx, "has-session", "-t", "="+name); err != nil {
		// Start the actual runner as the first pane. Do not leave an unowned shell
		// placeholder that a later recovery might mistake for an owned process.
		window := "orchestrator"
		if !l.Orchestrator {
			window = l.WorktreeName
		}
		output, err = t.call(ctx, "new-session", "-d", "-P", "-F", format, "-x", "160", "-y", "48", "-s", name, "-n", window, "-c", l.CWD, command)
		if err != nil {
			return Pane{}, err
		}
		parts := strings.Split(output, "\t")
		if len(parts) != 2 {
			return Pane{}, fail("launch_uncertain", "invalid tmux pane: %q", output)
		}
		if err := t.setRuntimeMetadata(ctx, l, parts[0], parts[1]); err != nil {
			return Pane{}, fail("launch_uncertain", "%s", err)
		}
		return Pane{ID: parts[0], WindowID: parts[1], SessionID: l.SessionID, RunID: l.RunID, Kind: paneKind(l), WorkspaceID: l.WorkspaceID}, nil
	}

	windows, err := t.call(ctx, "list-windows", "-t", "="+name, "-F", "#{window_id}\t#{window_name}\t#{@workspace_window_kind}\t#{@workspace_id}\t#{@workspace_worktree_id}")
	if err != nil {
		return Pane{}, err
	}
	targetWindow := ""
	for _, line := range strings.Split(windows, "\n") {
		p := strings.SplitN(line, "\t", 5)
		if len(p) != 5 {
			continue
		}
		if l.Orchestrator && p[2] == "orchestrator" && p[3] == l.WorkspaceID {
			targetWindow = p[0]
			break
		}
		if !l.Orchestrator && p[3] == l.WorkspaceID && p[4] == l.WorktreeID && l.WorktreeID != "" {
			targetWindow = p[0]
			break
		}
	}
	if l.Orchestrator && targetWindow == "" && l.PreferredOrchestratorWindowID != "" {
		if t.legacyOrchestratorWindow(ctx, l) {
			targetWindow = l.PreferredOrchestratorWindowID
			_ = t.setWindowMetadata(ctx, targetWindow, l, "orchestrator")
		}
	}
	if targetWindow == "" && l.Orchestrator {
		// Do not infer ownership from the user-visible window name.
		for _, line := range strings.Split(windows, "\n") {
			p := strings.SplitN(line, "\t", 5)
			if len(p) == 5 && p[2] == "orchestrator" && p[3] == l.WorkspaceID {
				targetWindow = p[0]
				break
			}
		}
	}
	if targetWindow == "" {
		windowName := l.WorktreeName
		if l.Orchestrator {
			windowName = "orchestrator"
		}
		output, err = t.call(ctx, "new-window", "-d", "-P", "-F", format, "-t", "="+name, "-n", windowName, "-c", l.CWD, command)
	} else if l.Orchestrator && l.ReplacePaneID != "" && l.ReplaceSessionID != "" && l.ReplaceRunID != "" {
		pane, inspectErr := t.Inspect(ctx, l.ReplacePaneID)
		if inspectErr == nil && pane.WindowID == targetWindow && pane.Dead && paneOwns(pane, l.ReplaceSessionID, l.ReplaceRunID) && pane.Kind != "tui" && pane.Kind != "service" {
			output = l.ReplacePaneID + "\t" + targetWindow
			_, err = t.call(ctx, "respawn-pane", "-k", "-c", l.CWD, "-t", l.ReplacePaneID, command)
		} else {
			output, err = t.call(ctx, "split-window", "-d", "-P", "-F", format, "-t", targetWindow, "-c", l.CWD, command)
		}
	} else {
		output, err = t.call(ctx, "split-window", "-d", "-P", "-F", format, "-t", targetWindow, "-c", l.CWD, command)
	}
	if err != nil {
		return Pane{}, fail("launch_uncertain", "%s", err)
	}
	parts := strings.Split(output, "\t")
	if len(parts) != 2 {
		return Pane{}, fail("launch_uncertain", "invalid tmux pane: %q", output)
	}
	if err := t.setRuntimeMetadata(ctx, l, parts[0], parts[1]); err != nil {
		return Pane{}, fail("launch_uncertain", "%s", err)
	}
	return Pane{ID: parts[0], WindowID: parts[1], SessionID: l.SessionID, RunID: l.RunID, Kind: paneKind(l), WorkspaceID: l.WorkspaceID}, nil
}

func (t Tmux) launchDispatcher(ctx context.Context, l Launch) (Pane, error) {
	name := DispatcherTmuxName(l.ProjectID)
	command := t.runnerCommand(l)
	format := "#{pane_id}\t#{window_id}"
	if _, err := t.call(ctx, "has-session", "-t", "="+name); err != nil {
		output, err := t.call(ctx, "new-session", "-d", "-P", "-F", format, "-x", "160", "-y", "48", "-s", name, "-n", "dispatcher", "-c", l.CWD, command)
		if err != nil {
			return Pane{}, fail("launch_uncertain", "%s", err)
		}
		parts := strings.Split(output, "\t")
		if len(parts) != 2 {
			return Pane{}, fail("launch_uncertain", "invalid tmux pane: %q", output)
		}
		if err := t.setRuntimeMetadata(ctx, l, parts[0], parts[1]); err != nil {
			return Pane{}, fail("launch_uncertain", "%s", err)
		}
		return Pane{ID: parts[0], WindowID: parts[1], SessionID: l.SessionID, RunID: l.RunID, Kind: "dispatcher", ProjectID: l.ProjectID, Scope: "project", AgentID: l.AgentID, Role: l.Role, SessionName: name}, nil
	}
	windows, err := t.call(ctx, "list-windows", "-t", "="+name, "-F", "#{window_id}\t#{@workspace_window_kind}\t#{@workspace_scope}\t#{@workspace_project_id}")
	if err != nil {
		return Pane{}, err
	}
	window := ""
	for _, line := range strings.Split(windows, "\n") {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) == 4 && parts[1] == "dispatcher" && parts[2] == "project" && parts[3] == l.ProjectID {
			window = parts[0]
			break
		}
	}
	var output string
	if window == "" {
		output, err = t.call(ctx, "new-window", "-d", "-P", "-F", format, "-t", "="+name, "-n", "dispatcher", "-c", l.CWD, command)
	} else {
		output, err = t.call(ctx, "split-window", "-d", "-P", "-F", format, "-t", window, "-c", l.CWD, command)
	}
	if err != nil {
		return Pane{}, fail("launch_uncertain", "%s", err)
	}
	parts := strings.Split(output, "\t")
	if len(parts) != 2 {
		return Pane{}, fail("launch_uncertain", "invalid tmux pane: %q", output)
	}
	if err := t.setRuntimeMetadata(ctx, l, parts[0], parts[1]); err != nil {
		return Pane{}, fail("launch_uncertain", "%s", err)
	}
	return Pane{ID: parts[0], WindowID: parts[1], SessionID: l.SessionID, RunID: l.RunID, Kind: "dispatcher", ProjectID: l.ProjectID, Scope: "project", AgentID: l.AgentID, Role: l.Role, SessionName: name}, nil
}

func (t Tmux) recoverDispatcher(ctx context.Context, l Launch) (Pane, error) {
	rows, err := t.call(ctx, "list-panes", "-a", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_scope}\t#{@workspace_project_id}\t#{@workspace_agent_id}\t#{@workspace_session_id}\t#{@workspace_run_id}\t#{@workspace_kind}\t#{pane_start_command}")
	if err != nil {
		return Pane{}, err
	}
	for _, line := range strings.Split(rows, "\n") {
		parts := strings.SplitN(line, "\t", 10)
		if len(parts) != 10 || parts[2] == "1" || parts[3] != "project" || parts[4] != l.ProjectID || parts[6] != l.SessionID || parts[7] != l.RunID || parts[8] != "dispatcher" {
			continue
		}
		if err := t.setRuntimeMetadata(ctx, l, parts[0], parts[1]); err != nil {
			return Pane{}, err
		}
		return Pane{ID: parts[0], WindowID: parts[1], SessionID: l.SessionID, RunID: l.RunID, Kind: "dispatcher", ProjectID: l.ProjectID, Scope: "project", AgentID: l.AgentID, Role: l.Role, StartCommand: parts[9], SessionName: DispatcherTmuxName(l.ProjectID)}, nil
	}
	return Pane{}, fail("pane_missing", "no Dispatcher pane for session %s", l.SessionID)
}
func (t Tmux) Inspect(ctx context.Context, pane string) (Pane, error) {
	out, err := t.call(ctx, "list-panes", "-a", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{pane_active}\t#{@workspace_session_id}\t#{@workspace_run_id}\t#{@workspace_kind}\t#{@workspace_id}\t#{@workspace_ui_id}\t#{@workspace_ui_generation}\t#{@workspace_ui_token}\t#{pane_start_command}\t#{session_name}\t#{window_name}")
	if err != nil {
		return Pane{}, err
	}
	for _, line := range strings.Split(out, "\n") {
		p := strings.SplitN(line, "\t", 14)
		if len(p) == 14 && p[0] == pane {
			generation, _ := strconv.Atoi(p[9])
			return Pane{ID: p[0], WindowID: p[1], Dead: p[2] == "1", Active: p[3] == "1", SessionID: p[4], RunID: p[5], Kind: p[6], WorkspaceID: p[7], UIID: p[8], UIGeneration: generation, UIToken: p[10], StartCommand: p[11], SessionName: p[12], WindowName: p[13]}, nil
		}
	}
	return Pane{}, fail("pane_missing", "%s", pane)
}

func (t Tmux) InspectProject(ctx context.Context, pane, projectID string) (Pane, error) {
	out, err := t.call(ctx, "list-panes", "-a", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_scope}\t#{@workspace_project_id}\t#{@workspace_agent_id}\t#{@workspace_role}\t#{@workspace_session_id}\t#{@workspace_run_id}\t#{@workspace_kind}\t#{pane_start_command}\t#{session_name}\t#{window_name}")
	if err != nil {
		return Pane{}, err
	}
	for _, line := range strings.Split(out, "\n") {
		p := strings.SplitN(line, "\t", 13)
		if len(p) == 13 && p[0] == pane && p[3] == "project" && p[4] == projectID {
			return Pane{ID: p[0], WindowID: p[1], Dead: p[2] == "1", Scope: p[3], ProjectID: p[4], AgentID: p[5], Role: p[6], SessionID: p[7], RunID: p[8], Kind: p[9], StartCommand: p[10], SessionName: p[11], WindowName: p[12]}, nil
		}
	}
	return Pane{}, fail("pane_missing", "%s", pane)
}

func paneOwns(p Pane, sessionID, runID string) bool {
	return (p.SessionID == sessionID && p.RunID == runID) || (p.RunID == "" && p.SessionID == runID)
}
func (t Tmux) Stop(ctx context.Context, pane string) error {
	_, err := t.call(ctx, "kill-pane", "-t", pane)
	return err
}
func (t Tmux) StopWorkspace(ctx context.Context, workspaceID string) error {
	_, err := t.call(ctx, "kill-session", "-t", "="+TmuxName(workspaceID))
	if tmuxMissingSession(err) {
		return nil
	}
	return err
}

func (t Tmux) StopDispatcher(ctx context.Context, projectID string) error {
	_, err := t.call(ctx, "kill-session", "-t", "="+DispatcherTmuxName(projectID))
	if tmuxMissingSession(err) {
		return nil
	}
	return err
}

func (t Tmux) AttachProject(ctx context.Context, projectID, pane string) error {
	name := DispatcherTmuxName(projectID)
	if os.Getenv("TMUX") != "" {
		currentSocket := strings.Split(os.Getenv("TMUX"), ",")[0]
		currentName := filepath.Base(currentSocket)
		expectedName := t.Socket
		if expectedName == "" {
			expectedName = "default"
		}
		if currentName != expectedName {
			return fail("tmux_server_mismatch", "attach from outside the current tmux server to use this project socket")
		}
	}
	if _, err := t.call(ctx, "has-session", "-t", "="+name); err != nil {
		return fail("pane_missing", "tmux Dispatcher session %s does not exist", name)
	}
	if pane != "" {
		p, err := t.InspectProject(ctx, pane, projectID)
		if err != nil {
			return err
		}
		if _, err := t.call(ctx, "select-window", "-t", p.WindowID); err != nil {
			return err
		}
		if _, err := t.call(ctx, "select-pane", "-t", pane); err != nil {
			return err
		}
	}
	verb := "attach-session"
	if os.Getenv("TMUX") != "" {
		verb = "switch-client"
	}
	cmd := exec.CommandContext(ctx, "tmux", t.args(verb, "-t", "="+name)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
func (t Tmux) Attach(ctx context.Context, workspaceID, pane string) error {
	topology, err := t.ObserveTopology(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !topology.SessionExists {
		return fail("pane_missing", "tmux session %s does not exist", TmuxName(workspaceID))
	}
	if os.Getenv("TMUX") != "" {
		currentSocket := strings.Split(os.Getenv("TMUX"), ",")[0]
		currentName := filepath.Base(currentSocket)
		expectedName := t.Socket
		if expectedName == "" {
			expectedName = "default"
		}
		if currentName != expectedName {
			return fail("tmux_server_mismatch", "attach from outside the current tmux server to use this workspace socket")
		}
	}
	if pane != "" {
		var p Pane
		found := false
		for _, candidate := range topology.Panes {
			if candidate.ID == pane {
				p, found = candidate, true
				break
			}
		}
		if !found {
			return fail("pane_mismatch", "pane does not belong to workspace session")
		}
		if _, err := t.call(ctx, "select-window", "-t", p.WindowID); err != nil {
			return err
		}
		if _, err := t.call(ctx, "select-pane", "-t", pane); err != nil {
			return err
		}
	}
	verb := "attach-session"
	if os.Getenv("TMUX") != "" {
		verb = "switch-client"
	}
	cmd := exec.CommandContext(ctx, "tmux", t.args(verb, "-t", "="+TmuxName(workspaceID))...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
