package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"workspace/internal/bootstrap"
	"workspace/internal/core"
	"workspace/internal/terminal"
	"workspace/internal/tui"
)

func tuiCommand(o *options) *cobra.Command {
	var theme string
	var noColor bool
	cmd := command("tui", "Browse project workspaces and their recorded workflow", func(c *cobra.Command, _ []string) error {
		if o.key != "" {
			return &core.Error{Code: "invalid_option", Message: "--operation-key is not supported for the TUI; each action creates its own key"}
		}
		if o.json || o.short || o.nonInteractive || !terminalIO(o.in, o.out) || strings.EqualFold(os.Getenv("TERM"), "dumb") {
			return &core.Error{Code: "interactive_required", Message: "workspace tui requires terminal stdin and stdout, TERM other than dumb, and interactive output"}
		}
		if theme != "" && theme != "auto" && theme != "dark" && theme != "light" {
			return &core.Error{Code: "invalid_option", Message: "--theme must be auto, dark, or light"}
		}
		request := bootstrap.Request{Project: o.project, Workspace: o.workspace, Socket: o.socket}
		scope, scopeErr := bootstrap.Resolve(request)
		if scopeErr != nil {
			projectScope, projectErr := bootstrap.Resolve(bootstrap.Request{Project: o.project, Socket: o.socket, ProjectOnly: true})
			if projectErr != nil {
				return projectErr
			}
			scope = projectScope
		}
		config := tui.Config{Theme: theme, NoColor: noColor || os.Getenv("NO_COLOR") != "", InitialError: errorString(scopeErr)}
		if scope != nil {
			config.ProjectRoot, config.CWD, config.ProjectFound = scope.ProjectRoot, scope.CWD, scope.ProjectFound
			if scope.Service != nil {
				config.Backend = tui.CoreBackend{Service: scope.Service}
				if project, err := scope.Service.Config(); err == nil {
					config.ProjectID = project.ProjectID
				}
				config.Navigator = terminal.NewTmuxNavigator(o.socket)
			}
			if scopeErr == nil {
				config.WorkspaceID = scope.WorkspaceID
			}
		}
		model := tui.New(config)
		return runTUI(model, o.in, o.out, o.errOut)
	})
	cmd.Flags().StringVar(&theme, "theme", "auto", "Color theme: auto, dark, or light")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable terminal colors")
	cmd.AddCommand(tuiDesiredCommand(o, true))
	cmd.AddCommand(tuiDesiredCommand(o, false))
	cmd.AddCommand(tuiStatusCommand(o))
	return cmd
}

// runTUI owns the program boundary around interactive external processes.
// Bubble Tea must have completely stopped (and restored the terminal) before
// an attach command starts; each return therefore gets a fresh renderer.
func runTUI(model *tui.Model, input io.Reader, output, errorOutput io.Writer) error {
	for {
		program := tea.NewProgram(model,
			tea.WithInput(input),
			tea.WithOutput(output),
			tea.WithAltScreen(),
		)
		result, err := program.Run()
		if err != nil {
			return err
		}
		if next, ok := result.(*tui.Model); ok {
			model = next
		}
		request := model.TakeExternalProcessRequest()
		if request == nil {
			return nil
		}
		request.Cmd.Stdin = input
		request.Cmd.Stdout = output
		request.Cmd.Stderr = errorOutput
		processErr := request.Cmd.Run()
		model.ResumeExternalProcess(request, processErr)
	}
}

func tuiDesiredCommand(o *options, desired bool) *cobra.Command {
	name, short := "hide", "Hide the managed interface panel"
	if desired {
		name, short = "show", "Show or recover the managed interface panel"
	}
	return command(name, short, func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		status, err := s.SetUIPaneDesired(c.Context(), id, desired, o.key)
		if err != nil {
			return err
		}
		return emitUIStatus(o, status, status.OperationID)
	})
}

func tuiStatusCommand(o *options) *cobra.Command {
	return command("status", "Show the managed interface desired state and runtime", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		status, err := s.UIPaneStatus(c.Context(), id)
		if err != nil {
			return err
		}
		return emitUIStatus(o, status, "")
	})
}

func emitUIStatus(o *options, status core.UIStatus, operationID string) error {
	if o.json {
		response := map[string]any{"ok": true, "data": status}
		if operationID != "" {
			response["operation_id"] = operationID
		}
		return json.NewEncoder(o.out).Encode(response)
	}
	var value any = status
	if o.short {
		value = shortOutput(status)
	}
	b, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	_, err = o.out.Write(b)
	return err
}

func tuiRunnerCommand(o *options) *cobra.Command {
	cmd := command("_tui-exec <ui-id> <generation> <token>", "Internal managed TUI runner", func(c *cobra.Command, args []string) error {
		if !terminalIO(o.in, o.out) || strings.EqualFold(os.Getenv("TERM"), "dumb") {
			return &core.Error{Code: "interactive_required", Message: "managed TUI requires terminal stdin and stdout"}
		}
		generation, err := strconv.Atoi(args[1])
		if err != nil || generation < 1 {
			return &core.Error{Code: "ui_launch_invalid", Message: "managed TUI generation must be a positive integer"}
		}
		paneID := strings.TrimSpace(os.Getenv("TMUX_PANE"))
		if paneID == "" {
			return &core.Error{Code: "ui_claim_invalid", Message: "managed TUI must run in its claimed tmux pane"}
		}
		scope, err := bootstrap.Resolve(bootstrap.Request{Project: o.project, Workspace: o.workspace, Socket: o.socket})
		if err != nil {
			return err
		}
		if !scope.ProjectFound || scope.Service == nil || scope.WorkspaceID == "" {
			return &core.Error{Code: "workspace_required", Message: "managed TUI requires an explicit project and workspace"}
		}
		if err := scope.Service.ClaimUIPane(c.Context(), scope.WorkspaceID, args[0], generation, args[2], paneID); err != nil {
			return err
		}
		scope.Service.Actor = core.Actor{}
		projectID := ""
		if config, err := scope.Service.Config(); err == nil {
			projectID = config.ProjectID
		}
		config := tui.Config{
			ProjectRoot: scope.ProjectRoot, ProjectID: projectID, CWD: scope.CWD,
			WorkspaceID: scope.WorkspaceID, ProjectFound: true, Managed: true,
			Theme: "auto", NoColor: os.Getenv("NO_COLOR") != "",
			Backend:   tui.CoreBackend{Service: scope.Service},
			Navigator: terminal.NewTmuxNavigator(o.socket),
		}
		return runTUI(tui.New(config), o.in, o.out, o.errOut)
	})
	cmd.Args = cobra.ExactArgs(3)
	cmd.Hidden = true
	return cmd
}

func terminalIO(input io.Reader, output io.Writer) bool {
	stdin, okIn := input.(*os.File)
	stdout, okOut := output.(*os.File)
	return okIn && okOut && term.IsTerminal(stdin.Fd()) && term.IsTerminal(stdout.Fd())
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Code + ": " + ce.Message
	}
	return fmt.Sprint(err)
}
