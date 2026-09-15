package core

import (
	"context"
	"strconv"
	"strings"
)

func (t Tmux) uiRunnerCommand(l UILaunch) string {
	args := []string{l.Executable, "--project", l.ProjectRoot, "--workspace", l.WorkspaceID, "--tmux-socket", l.Socket}
	args = append(args, "_tui-exec", l.UIID, intString(l.Generation), l.Token)
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	clean := []string{"WORKSPACE_AGENT_ID", "WORKSPACE_SESSION_ID", "WORKSPACE_RUN_ID", "WORKSPACE_TASK_ID", "WORKSPACE_ROLE", "WORKSPACE_PARENT_AGENT_ID", "WORKSPACE_PARENT_SESSION_ID", "WORKSPACE_PARENT_RUN_ID", "WORKSPACE_ORCHESTRATOR_ID", "WORKSPACE_ORCHESTRATOR_AGENT_ID"}
	var command strings.Builder
	command.WriteString("exec env")
	for _, name := range clean {
		command.WriteString(" -u ")
		command.WriteString(shellQuote(name))
	}
	command.WriteByte(' ')
	command.WriteString(strings.Join(quoted, " "))
	return command.String()
}

func intString(value int) string {
	return strconv.Itoa(value)
}

func (t Tmux) setUIPaneMetadata(ctx context.Context, launch UILaunch, paneID string) error {
	if _, err := t.call(ctx, "set-option", "-p", "-t", paneID, "remain-on-exit", "on"); err != nil {
		return err
	}
	values := [][2]string{
		{"@workspace_kind", "tui"},
		{"@workspace_id", launch.WorkspaceID},
		{"@workspace_project_id", launch.ProjectID},
		{"@workspace_ui_id", launch.UIID},
		{"@workspace_ui_generation", intString(launch.Generation)},
		{"@workspace_ui_token", launch.Token},
	}
	for _, value := range values {
		if value[1] == "" {
			continue
		}
		if _, err := t.call(ctx, "set-option", "-p", "-t", paneID, value[0], value[1]); err != nil {
			return err
		}
	}
	return nil
}

func (t Tmux) LaunchUI(ctx context.Context, launch UILaunch) (Pane, error) {
	if launch.UIID == "" || launch.Token == "" || launch.Generation < 1 {
		return Pane{}, fail("ui_launch_invalid", "UI identity, generation and launch token are required")
	}
	topology, err := t.ObserveTopology(ctx, launch.WorkspaceID)
	if err != nil {
		return Pane{}, err
	}
	if !topology.SessionExists {
		return Pane{}, fail("pane_missing", "workspace tmux session does not exist")
	}
	command := t.uiRunnerCommand(launch)
	var adopted *Pane
	for i := range topology.Panes {
		pane := topology.Panes[i]
		uiID, generation, token, verified := uiPaneIdentity(pane, launch.WorkspaceID)
		if !verified || uiID != launch.UIID || generation != launch.Generation || token != launch.Token {
			continue
		}
		if adopted != nil {
			return Pane{}, fail("ui_pane_conflict", "multiple panes claim the same UI generation")
		}
		adopted = &pane
	}
	if adopted != nil {
		if err := t.setUIPaneMetadata(ctx, launch, adopted.ID); err != nil {
			return Pane{}, err
		}
		adopted.Kind, adopted.WorkspaceID, adopted.UIID = "tui", launch.WorkspaceID, launch.UIID
		adopted.UIGeneration, adopted.UIToken = launch.Generation, launch.Token
		return *adopted, nil
	}
	if launch.WindowID == "" {
		return Pane{}, fail("ui_waiting_for_runtime", "no verified orchestrator window is available")
	}
	var anchor *TmuxWindow
	for i := range topology.Windows {
		if topology.Windows[i].ID == launch.WindowID && topology.Windows[i].Kind == "orchestrator" {
			anchor = &topology.Windows[i]
			break
		}
	}
	if anchor == nil {
		return Pane{}, fail("ui_waiting_for_runtime", "orchestrator window is not available")
	}
	sizeFlag, size := "", ""
	if anchor.Width >= 120 {
		sizeFlag, size = "-h", "40%"
	} else if anchor.Height >= 36 {
		sizeFlag, size = "-v", "35%"
	} else if anchor.Height >= 30 {
		sizeFlag, size = "-v", "12"
	} else {
		return Pane{}, fail("ui_space", "not enough pane space for the managed interface")
	}
	output, err := t.call(ctx, "split-window", "-d", sizeFlag, "-l", size, "-P", "-F", "#{pane_id}\t#{window_id}", "-t", launch.WindowID, "-c", launch.ProjectRoot, command)
	if err != nil {
		return Pane{}, fail("launch_uncertain", "%s", err)
	}
	parts := strings.Split(output, "\t")
	if len(parts) != 2 {
		return Pane{}, fail("launch_uncertain", "invalid tmux pane: %q", output)
	}
	if err := t.setUIPaneMetadata(ctx, launch, parts[0]); err != nil {
		return Pane{}, fail("launch_uncertain", "%s", err)
	}
	return Pane{ID: parts[0], WindowID: parts[1], Kind: "tui", WorkspaceID: launch.WorkspaceID, UIID: launch.UIID, UIGeneration: launch.Generation, UIToken: launch.Token}, nil
}

func (t Tmux) StopUI(ctx context.Context, launch UILaunch, paneID string) error {
	pane, err := t.Inspect(ctx, paneID)
	if err != nil {
		return err
	}
	uiID, generation, token, verified := uiPaneIdentity(pane, launch.WorkspaceID)
	if !verified || uiID != launch.UIID || generation != launch.Generation || token != launch.Token {
		return fail("pane_mismatch", "pane is not owned by this managed UI generation")
	}
	_, err = t.call(ctx, "kill-pane", "-t", paneID)
	return err
}
