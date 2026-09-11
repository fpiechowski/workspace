package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Launch struct {
	Service                                                                          bool
	ProjectRoot, WorkspaceID, WorkspaceDir, SessionID, WorktreeName, CWD, Executable string
	Orchestrator                                                                     bool
}
type Pane struct {
	ID, WindowID string
	Dead         bool
	SessionID    string
}
type Runtime interface {
	Launch(context.Context, Launch) (Pane, error)
	Inspect(context.Context, string) (Pane, error)
	Stop(context.Context, string) error
	Attach(context.Context, string, string) error
}
type Tmux struct{ Socket string }

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

// tmux accepts a shell command. Quote every argv element, including embedded quotes.
func shellQuote(arg string) string { return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'" }
func (t Tmux) runnerCommand(l Launch) string {
	verb := "_session-exec"
	if l.Service {
		verb = "_service-exec"
	}
	args := []string{l.Executable, "--project", l.ProjectRoot, "--workspace", l.WorkspaceID, "--tmux-socket", t.Socket, verb, l.SessionID}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	return "exec " + strings.Join(quoted, " ")
}

// Recover closes the gap between creating a pane and recording its ID. The
// exact runner command is unique to the concrete Session, including its ULID.
func (t Tmux) Recover(ctx context.Context, l Launch) (Pane, error) {
	out, err := t.call(ctx, "list-panes", "-a", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_session_id}\t#{pane_start_command}")
	if err != nil {
		return Pane{}, err
	}
	for _, line := range strings.Split(out, "\n") {
		p := strings.SplitN(line, "\t", 5)
		if len(p) != 5 {
			continue
		}
		start := p[4]
		// tmux serializes the single shell-command argument with outer quotes.
		if decoded, err := strconv.Unquote(start); err == nil {
			start = decoded
		}
		if p[3] != l.SessionID && !(p[3] == "" && start == t.runnerCommand(l)) {
			continue
		}
		if _, err := t.call(ctx, "set-option", "-w", "-t", p[1], "remain-on-exit", "on"); err != nil {
			return Pane{}, err
		}
		if _, err := t.call(ctx, "set-option", "-p", "-t", p[0], "@workspace_session_id", l.SessionID); err != nil {
			return Pane{}, err
		}
		return Pane{ID: p[0], WindowID: p[1], Dead: p[2] == "1", SessionID: l.SessionID}, nil
	}
	return Pane{}, fail("pane_missing", "no pane for session %s", l.SessionID)
}
func (t Tmux) Launch(ctx context.Context, l Launch) (Pane, error) {
	name := TmuxName(l.WorkspaceID)
	if _, err := t.call(ctx, "has-session", "-t", "="+name); err != nil {
		if _, err := t.call(ctx, "new-session", "-d", "-s", name, "-n", "orchestrator", "-c", l.WorkspaceDir); err != nil {
			return Pane{}, err
		}
	}
	command := t.runnerCommand(l)
	var output string
	var err error
	format := "#{pane_id}\t#{window_id}"
	if l.Orchestrator {
		output, err = t.call(ctx, "display-message", "-p", "-t", name+":orchestrator", format)
		if err == nil {
			parts := strings.Split(output, "\t")
			if len(parts) != 2 {
				return Pane{}, fmt.Errorf("invalid tmux pane: %q", output)
			}
			_, err = t.call(ctx, "respawn-pane", "-k", "-c", l.CWD, "-t", parts[0], command)
		}
	} else {
		windows, e := t.call(ctx, "list-windows", "-t", name, "-F", "#{window_id}\t#{window_name}")
		if e != nil {
			return Pane{}, e
		}
		target := ""
		for _, line := range strings.Split(windows, "\n") {
			p := strings.SplitN(line, "\t", 2)
			if len(p) == 2 && p[1] == l.WorktreeName {
				target = p[0]
			}
		}
		if target == "" {
			output, err = t.call(ctx, "new-window", "-d", "-P", "-F", format, "-t", name, "-n", l.WorktreeName, "-c", l.CWD, command)
		} else {
			output, err = t.call(ctx, "split-window", "-d", "-P", "-F", format, "-t", target, "-c", l.CWD, command)
		}
	}
	if err != nil {
		return Pane{}, fail("launch_uncertain", "%s", err)
	}
	parts := strings.Split(output, "\t")
	if len(parts) != 2 {
		return Pane{}, fail("launch_uncertain", "invalid tmux pane: %q", output)
	}
	// The runner waits on the project lock, so it cannot exit before these are set.
	if _, err = t.call(ctx, "set-option", "-w", "-t", parts[1], "remain-on-exit", "on"); err != nil {
		return Pane{}, fail("launch_uncertain", "%s", err)
	}
	if _, err = t.call(ctx, "set-option", "-p", "-t", parts[0], "@workspace_session_id", l.SessionID); err != nil {
		return Pane{}, fail("launch_uncertain", "%s", err)
	}
	return Pane{ID: parts[0], WindowID: parts[1], SessionID: l.SessionID}, nil
}
func (t Tmux) Inspect(ctx context.Context, pane string) (Pane, error) {
	out, err := t.call(ctx, "list-panes", "-a", "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_session_id}")
	if err != nil {
		return Pane{}, err
	}
	for _, line := range strings.Split(out, "\n") {
		p := strings.Split(line, "\t")
		if len(p) == 4 && p[0] == pane {
			return Pane{p[0], p[1], p[2] == "1", p[3]}, nil
		}
	}
	return Pane{}, fail("pane_missing", "%s", pane)
}
func (t Tmux) Stop(ctx context.Context, pane string) error {
	_, err := t.call(ctx, "kill-pane", "-t", pane)
	return err
}
func (t Tmux) Attach(ctx context.Context, workspaceID, pane string) error {
	if pane != "" {
		if _, err := t.call(ctx, "select-pane", "-t", pane); err != nil {
			return err
		}
		p, err := t.Inspect(ctx, pane)
		if err != nil {
			return err
		}
		if _, err = t.call(ctx, "select-window", "-t", p.WindowID); err != nil {
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
