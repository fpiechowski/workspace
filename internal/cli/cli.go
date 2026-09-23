package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"workspace/internal/bootstrap"
	"workspace/internal/buildinfo"
	"workspace/internal/core"
	"workspace/internal/terminal"
	"workspace/internal/upgrade"
)

type options struct {
	commandPath                     string
	project, workspace, socket, key string
	scopeName                       string
	json, short, nonInteractive     bool
	in                              io.Reader
	out, errOut                     io.Writer
}

var errWorkspaceSelectionCancelled = errors.New("workspace selection cancelled")

func Execute(args []string, in io.Reader, out, errOut io.Writer) int {
	o := &options{in: in, out: out, errOut: errOut}
	root := newRoot(o)
	root.SetArgs(normalizeHelpArgs(args))
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	if err := root.Execute(); err != nil {
		var ce *core.Error
		if !errors.As(err, &ce) {
			ce = &core.Error{Code: "command_failed", Message: err.Error()}
		}
		if o.json {
			_ = json.NewEncoder(out).Encode(map[string]any{"ok": false, "error": ce})
		} else {
			fmt.Fprintln(errOut, ce.Error())
		}
		return 1
	}
	return 0
}
func (o *options) emit(v any) error {
	if o.json {
		response := map[string]any{"ok": true, "data": v}
		if o.key != "" {
			if s, err := o.service(); err == nil {
				selector, _ := o.selector()
				if status, ok := v.(core.Status); ok {
					selector = status.Workspace.ID
				}
				key := o.key
				switch o.commandPath {
				case "workspace create":
					key = "create:" + key
				case "workspace issue dispatch":
					key = "issue.dispatch:" + key
				case "workspace project init", "workspace skill install", "workspace server stop":
					selector = ""
				}
				metadata, err := s.OperationMetadata(context.Background(), selector, key)
				if err != nil {
					return err
				}
				if metadata.ID != "" {
					response["operation_id"] = metadata.ID
					if metadata.Revision != 0 {
						response["revision"] = metadata.Revision
					}
				}
			}
		}
		return json.NewEncoder(o.out).Encode(response)
	}
	if o.short {
		v = shortOutput(v)
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = o.out.Write(b)
	return err
}
func (o *options) service() (*core.Service, error) {
	scope, err := o.bootstrapScope(true)
	if err != nil {
		return nil, err
	}
	if !scope.ProjectFound {
		return nil, scope.ProjectError
	}
	return scope.Service, nil
}
func (o *options) selector() (string, error) {
	scope, err := o.bootstrapScope(false)
	if err != nil {
		return "", err
	}
	if !scope.ProjectFound {
		return "", scope.ProjectError
	}
	if scope.WorkspaceID != "" {
		return scope.WorkspaceID, nil
	}
	return "", &core.Error{Code: "workspace_required", Message: "use --workspace <id> or run from a workspace directory"}
}
func (o *options) scope() (*core.Service, string, error) {
	scope, err := o.bootstrapScope(false)
	if err != nil {
		return nil, "", err
	}
	if !scope.ProjectFound {
		return nil, "", scope.ProjectError
	}
	if scope.WorkspaceID == "" {
		return nil, "", &core.Error{Code: "workspace_required", Message: "use --workspace <id> or run from a workspace directory"}
	}
	return scope.Service, scope.WorkspaceID, nil
}

func (o *options) bootstrapScope(projectOnly bool) (*bootstrap.Scope, error) {
	return bootstrap.Resolve(bootstrap.Request{Project: o.project, Workspace: o.workspace, Socket: o.socket, Scope: o.scopeName, ProjectOnly: projectOnly})
}
func command(use, short string, fn func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: fn}
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.TrimSpace(args[0])
}

func workspaceTitle(status core.Status) string {
	if title := strings.TrimSpace(status.Workspace.Title); title != "" {
		return title
	}
	return status.Workspace.ID
}

func workspaceNameIDs(statuses []core.Status) map[string]string {
	out := make(map[string]string, len(statuses))
	for _, status := range statuses {
		name := workspaceTitle(status)
		if _, exists := out[name]; exists {
			name = fmt.Sprintf("%s (%s)", name, status.Workspace.ID)
		}
		out[name] = status.Workspace.ID
	}
	return out
}

func chooseWorkspace(o *options, statuses []core.Status, selector string) (core.Status, error) {
	if selector != "" {
		matches := make([]core.Status, 0, 1)
		for _, status := range statuses {
			if status.Workspace.ID == selector || strings.EqualFold(workspaceTitle(status), selector) {
				matches = append(matches, status)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			ids := make([]string, 0, len(matches))
			for _, match := range matches {
				ids = append(ids, match.Workspace.ID)
			}
			return core.Status{}, fmt.Errorf("workspace title %q is ambiguous; use one of: %s", selector, strings.Join(ids, ", "))
		}
		return core.Status{}, fmt.Errorf("workspace %q not found", selector)
	}
	if o.json || o.nonInteractive {
		return core.Status{}, fmt.Errorf("workspace open without a selector requires an interactive terminal")
	}

	ordered := append([]core.Status(nil), statuses...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Workspace.CreatedAt.After(ordered[j].Workspace.CreatedAt)
	})
	fmt.Fprintln(o.out, "Available workspaces:")
	for i, status := range ordered {
		phase := status.Workspace.PhaseLabel()
		if phase == "" {
			phase = "-"
		}
		detail := status.Workspace.Input.Source
		if detail == "" {
			detail = status.Workspace.Input.Snapshot
		}
		fmt.Fprintf(o.out, "  %d) %s [%s / %s] %s\n", i+1, workspaceTitle(status), status.Workspace.Status, phase, status.Workspace.ID)
		if detail != "" {
			fmt.Fprintf(o.out, "     %s\n", detail)
		}
	}
	fmt.Fprint(o.out, "Select workspace number (Enter = 1, q = cancel): ")
	input := o.in
	if input == nil {
		input = os.Stdin
	}
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return core.Status{}, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return ordered[0], nil
	}
	if strings.EqualFold(line, "q") || strings.EqualFold(line, "quit") || strings.EqualFold(line, "cancel") {
		return core.Status{}, errWorkspaceSelectionCancelled
	}
	var choice int
	if _, err := fmt.Sscanf(line, "%d", &choice); err != nil || choice < 1 || choice > len(ordered) {
		return core.Status{}, fmt.Errorf("invalid workspace selection %q", line)
	}
	return ordered[choice-1], nil
}

func newRoot(o *options) *cobra.Command {
	root := &cobra.Command{Use: "workspace", Short: "Manage agent personas and tmux sessions in Git workspaces", SilenceUsage: true, SilenceErrors: true}
	root.PersistentPreRun = func(c *cobra.Command, _ []string) { o.commandPath = c.CommandPath() }
	root.CompletionOptions.DisableDefaultCmd = true
	f := root.PersistentFlags()
	f.StringVar(&o.project, "project", "", "Project path (also WORKSPACE_PROJECT_DIR)")
	f.StringVar(&o.workspace, "workspace", "", "Workspace ID (also WORKSPACE_ID)")
	f.StringVar(&o.socket, "tmux-socket", os.Getenv("WORKSPACE_TMUX_SOCKET"), "Optional isolated tmux server name")
	f.StringVar(&o.scopeName, "scope", "", "Internal runtime scope (project or workspace)")
	f.StringVar(&o.key, "operation-key", "", "Idempotency key for a mutation; changed payload with the same key is rejected")
	f.BoolVar(&o.json, "json", false, "Print structured JSON")
	f.BoolVar(&o.short, "short", false, "Print a compact human-readable summary")
	f.BoolVar(&o.nonInteractive, "non-interactive", false, "Never ask terminal questions")
	_ = f.MarkHidden("scope")
	project := &cobra.Command{Use: "project", Short: "Configure a Git project"}
	project.AddCommand(command("init", "Install project config and workflow templates", func(c *cobra.Command, _ []string) error {
		dir := o.project
		if dir == "" {
			var err error
			dir, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		cfg, err := core.InitProject(c.Context(), dir, o.key)
		if err != nil {
			return err
		}
		return o.emit(cfg)
	}))
	root.AddCommand(project)
	skill := &cobra.Command{Use: "skill", Short: "Install the bundled workspace skill in this project"}
	var skillClient string
	installSkill := command("install", "Install a discoverable project skill", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		path, err := s.InstallSkill(c.Context(), skillClient, o.key)
		if err != nil {
			return err
		}
		return o.emit(map[string]string{"path": path})
	})
	installSkill.Flags().StringVar(&skillClient, "client", "codex", "codex, claude or opencode")
	skill.AddCommand(installSkill)
	root.AddCommand(skill)
	root.AddCommand(command("version", "Show workspace version and build metadata", func(c *cobra.Command, _ []string) error {
		return o.emit(buildinfo.Current())
	}))
	root.AddCommand(command("prime", "Print the current binary's bundled agent guidance", func(c *cobra.Command, _ []string) error {
		instructions, err := core.PrimeInstructions()
		if err != nil {
			return err
		}
		if o.json {
			key := o.key
			o.key = ""
			err := o.emit(map[string]string{"instructions": instructions})
			o.key = key
			return err
		}
		_, err = io.WriteString(o.out, instructions)
		return err
	}))
	root.AddCommand(command("upgrade", "Upgrade the installed workspace executable from the latest stable GitHub Release", func(c *cobra.Command, _ []string) error {
		result, err := upgrade.Upgrade(c.Context(), upgrade.Options{})
		if err != nil {
			return &core.Error{Code: "upgrade_failed", Message: err.Error()}
		}
		if result.Updated {
			fmt.Fprintln(o.errOut, "workspace was upgraded; restart already-running supervisors and tmux processes to load the new binary")
		}
		return o.emit(result)
	}))
	root.AddCommand(command("doctor", "Inspect local runtime prerequisites", func(c *cobra.Command, _ []string) error {
		checks := map[string]any{"os": runtime.GOOS, "session_runtime_supported": runtime.GOOS != "windows"}
		for _, name := range []string{"git", "tmux"} {
			path, err := exec.LookPath(name)
			checks[name] = map[string]any{"available": err == nil, "path": path}
		}
		s, err := o.service()
		if err != nil {
			checks["project"] = err.Error()
		} else {
			cfg, err := s.Config()
			if err != nil {
				checks["config"] = err.Error()
			} else {
				checks["project"] = s.Root
				checks["configured_profiles"] = len(cfg.Profiles)
				checks["configured_clients"] = len(cfg.Clients)
			}
		}
		return o.emit(checks)
	}))
	var create core.CreateOptions
	var inputFile string
	createCmd := command("create [intent]", "Create a workspace from an issue description", func(c *cobra.Command, args []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		if create.FromIssue != "" && (len(args) > 0 || inputFile != "" || create.Source != "") {
			return &core.Error{Code: "input_conflict", Message: "--from-issue is mutually exclusive with positional intent, --input-file, and --issue"}
		}
		if len(args) == 1 {
			if inputFile != "" {
				return &core.Error{Code: "input_conflict", Message: "provide the intent either as an argument or with --input-file, not both"}
			}
			create.Input = args[0]
		}
		if inputFile != "" {
			b, err := os.ReadFile(inputFile)
			if err != nil {
				return err
			}
			create.Input = string(b)
		}
		create.OperationKey = o.key
		v, err := s.Create(c.Context(), create)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	createCmd.Args = cobra.MaximumNArgs(1)
	createCmd.Flags().StringVar(&create.Title, "title", "", "Workspace title")
	createCmd.Flags().StringVar(&inputFile, "input-file", "", "Saved issue/description file")
	createCmd.Flags().StringVar(&create.Source, "issue", "", "Issue URL; fetch from configured tracker unless an intent or --input-file is supplied")
	createCmd.Flags().StringVar(&create.FromIssue, "from-issue", "", "Create from an existing durable Issue ID; mutually exclusive with intent, --input-file and --issue")
	createCmd.Flags().StringVar(&create.Workflow, "workflow", "", "Workflow name; omit to choose a workflow later")
	createCmd.Flags().BoolVar(&create.NoWorkflow, "no-workflow", false, "Create an active workspace with no workflow for manual orchestration")
	createCmd.Flags().StringVar(&create.Base, "base", "HEAD", "Base Git revision")
	root.AddCommand(createCmd)
	listCmd := command("list", "List workspaces", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		v, err := s.List(c.Context())
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	listCmd.Flags().BoolVar(&o.short, "map", false, "Alias for --short")
	root.AddCommand(listCmd)
	openCmd := command("open [workspace]", "Select a workspace and attach to its tmux session", func(c *cobra.Command, args []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		workspaces, err := s.List(c.Context())
		if err != nil {
			return err
		}
		if len(workspaces) == 0 {
			return fmt.Errorf("no workspaces found")
		}
		selected, err := chooseWorkspace(o, workspaces, firstArg(args))
		if err != nil {
			if errors.Is(err, errWorkspaceSelectionCancelled) {
				return nil
			}
			return err
		}
		if !o.json {
			fmt.Fprintf(o.out, "Opening %s (%s)\n", workspaceTitle(selected), selected.Workspace.ID)
		}
		return attachNavigation(c.Context(), s, selected.Workspace.ID, core.EntityRef{Kind: "workspace", ID: selected.Workspace.ID})
	})
	openCmd.Args = cobra.MaximumNArgs(1)
	root.AddCommand(openCmd)
	root.AddCommand(command("status", "Show durable state with logical sessions and run history", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	workflow := &cobra.Command{Use: "workflow", Short: "Select a workflow"}
	workflow.AddCommand(command("list", "Show available workflows", func(c *cobra.Command, _ []string) error {
		s, err := o.service()
		if err != nil {
			return err
		}
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		workflows := make([]map[string]any, 0)
		for _, name := range core.WorkflowNames(s.Root, cfg) {
			workflows = append(workflows, map[string]any{"id": name, "version": 1, "input": "issue URL with snapshot, or issue description", "capabilities": core.WorkflowCapabilitiesForList(name, cfg)})
		}
		return o.emit(workflows)
	}))
	selectCmd := command("select <name>", "Select the workflow for an undecided workspace", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.SelectWorkflow(c.Context(), id, args[0], o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	selectCmd.Args = cobra.ExactArgs(1)
	workflow.AddCommand(selectCmd)
	var phase string
	advance := command("advance", "Validate current results and advance one workflow phase", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.AdvanceWorkflow(c.Context(), id, phase, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	advance.Flags().StringVar(&phase, "to", "", "Expected next phase")
	workflow.AddCommand(advance)
	workflow.AddCommand(revisionCommand(o, true))
	root.AddCommand(workflow)
	input := &cobra.Command{Use: "input", Short: "Revise issue input with preserved history"}
	input.AddCommand(revisionCommand(o, false))
	root.AddCommand(input)
	for _, kind := range []string{"profile", "client"} {
		kind := kind
		group := &cobra.Command{Use: kind, Short: "Inspect configured " + kind + " definitions"}
		group.AddCommand(command("list", "List configured definitions", func(c *cobra.Command, _ []string) error {
			s, err := o.service()
			if err != nil {
				return err
			}
			cfg, err := s.Config()
			if err != nil {
				return err
			}
			if kind == "profile" {
				return o.emit(cfg.Profiles)
			}
			return o.emit(cfg.Clients)
		}))
		if kind == "profile" {
			explain := command("explain <name>", "Explain route eligibility and provider balancing", func(c *cobra.Command, args []string) error {
				s, err := o.service()
				if err != nil {
					return err
				}
				v, err := s.ExplainProfile(c.Context(), args[0])
				if err != nil {
					return err
				}
				return o.emit(v)
			})
			explain.Args = cobra.ExactArgs(1)
			group.AddCommand(explain)
		}
		root.AddCommand(group)
	}
	root.AddCommand(issueCommands(o))
	dispatcher, dispatcherRunner := dispatcherCommands(o)
	root.AddCommand(dispatcher, dispatcherRunner)
	root.AddCommand(agentCommands(o), worktreeCommands(o), sessionCommands(o), runCommands(o))
	root.AddCommand(taskCommands(o), messageCommands(o), inboxCommands(o), handoffCommands(o), artifactCommands(o))
	root.AddCommand(integrationCommands(o), decisionCommands(o), stateCommands(o), releaseCommands(o))
	root.AddCommand(changeRequestCommands(o))
	root.AddCommand(checkCommands(o))
	services, serviceRunner := serviceCommands(o)
	root.AddCommand(services, serviceRunner)
	serve, server := supervisorCommands(o)
	root.AddCommand(serve, server)
	root.AddCommand(command("start", "Launch or converse with the orchestrator in tmux", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.StartSupervisedOrchestrator(c.Context(), id, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	root.AddCommand(command("attach", "Attach to the workspace tmux session", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		return attachNavigation(c.Context(), s, id, core.EntityRef{Kind: "workspace", ID: id})
	}))
	for _, verb := range []string{"pause", "resume"} {
		verb := verb
		var interrupt bool
		lifecycle := command(verb, verb+" delegation", func(c *cobra.Command, _ []string) error {
			s, id, err := o.scope()
			if err != nil {
				return err
			}
			var v core.Status
			if verb == "pause" {
				v, err = s.Pause(c.Context(), id, interrupt, o.key)
			} else {
				v, err = s.SetPaused(c.Context(), id, false, o.key)
			}
			if err != nil {
				return err
			}
			return o.emit(v)
		})
		if verb == "pause" {
			lifecycle.Flags().BoolVar(&interrupt, "interrupt", false, "Stop active runs while preserving logical sessions and local work")
		}
		root.AddCommand(lifecycle)
	}
	var completeReason string
	var completeConfirmed bool
	var completeExpected int
	complete := command("complete", "Complete an intentionally manual workspace after explicit user confirmation", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.CompleteWorkspace(c.Context(), id, core.CompleteOptions{Reason: completeReason, UserConfirmed: completeConfirmed, ExpectedRevision: completeExpected}, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	complete.Flags().StringVar(&completeReason, "reason", "", "Reason recorded for the completion")
	complete.Flags().BoolVar(&completeConfirmed, "user-confirmed", false, "Attest that the user explicitly requested completion when running from an agent session")
	complete.Flags().IntVar(&completeExpected, "expected-revision", 0, "Required current workspace revision")
	root.AddCommand(complete)
	var reopenReason string
	var reopenConfirmed bool
	var reopenExpected int
	reopen := command("reopen", "Reopen a completed workspace for explicitly authorized follow-up work", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.ReopenWorkspace(c.Context(), id, core.ReopenOptions{Reason: reopenReason, ExpectedRevision: reopenExpected, UserConfirmed: reopenConfirmed}, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	reopen.Flags().StringVar(&reopenReason, "reason", "", "Required reason for reopening the completed workspace")
	reopen.Flags().IntVar(&reopenExpected, "expected-revision", 0, "Required exact current workspace revision")
	reopen.Flags().BoolVar(&reopenConfirmed, "user-confirmed", false, "Attest that the user explicitly requested follow-up work when running from an agent session")
	root.AddCommand(reopen)
	root.AddCommand(command("archive", "Archive a completed workspace", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Archive(c.Context(), id, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	var dryRun, backup bool
	clean := command("clean", "Remove archived worktrees after checking preservation", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Clean(c.Context(), id, dryRun, backup, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	clean.Flags().BoolVar(&dryRun, "dry-run", false, "Show the removal plan without changes")
	clean.Flags().BoolVar(&backup, "backup", false, "Preserve unpublished commits in verified Git bundles before removal")
	root.AddCommand(clean)
	root.AddCommand(command("reconcile", "Reconcile recorded sessions against tmux panes", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.ReconcileWorkspace(c.Context(), id, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	root.AddCommand(command("menu", "Show workflow actions and pending user decisions", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Menu(c.Context(), id)
		if err != nil {
			return err
		}
		return o.emit(v)
	}))
	execCmd := command("_session-exec <id>", "Internal tmux runner", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		s.Actor = core.Actor{}
		return s.ExecuteSession(context.Background(), id, args[0], o.in, o.out, o.errOut)
	})
	execCmd.Args = cobra.ExactArgs(1)
	execCmd.Hidden = true
	root.AddCommand(execCmd)
	root.AddCommand(tuiCommand(o))
	root.AddCommand(tuiRunnerCommand(o))
	root.InitDefaultHelpCmd()
	configureHelp(root)
	return root
}

func agentCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "agent", Short: "Define personas; each can have multiple logical sessions and runs"}
	var opt core.AgentOptions
	var instructionsFile string
	create := command("create <name>", "Define a worker persona", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		opt.Name = args[0]
		opt.OperationKey = o.key
		if instructionsFile != "" {
			b, err := os.ReadFile(instructionsFile)
			if err != nil {
				return err
			}
			opt.Instructions = string(b)
		}
		v, err := s.CreateAgent(c.Context(), id, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	create.Args = cobra.ExactArgs(1)
	f := create.Flags()
	f.StringVar(&opt.Role, "role", "", "planner, implementer, integrator or tester")
	f.StringVar(&opt.Profile, "profile", "", "Default model profile")
	f.StringVar(&opt.PromptTemplate, "prompt-template", "", "Prompt template name")
	f.StringVar(&instructionsFile, "instructions-file", "", "Persona/delegated scope instructions")
	group.AddCommand(create)
	group.AddCommand(command("list", "List agent definitions", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		return o.emit(v.Agents)
	}))
	resume := command("resume <agent>", "Start a new run, reusing a compatible logical session", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.ResumeSupervisedAgent(c.Context(), id, args[0], o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	resume.Args = cobra.ExactArgs(1)
	group.AddCommand(resume)
	return group
}
func worktreeCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "worktree", Short: "Manage task checkouts"}
	var opt core.WorktreeOptions
	create := command("create <name>", "Create a dedicated Git branch and worktree", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		opt.Name = args[0]
		opt.OperationKey = o.key
		v, err := s.CreateWorktree(c.Context(), id, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	create.Args = cobra.ExactArgs(1)
	create.Flags().StringVar(&opt.Base, "base", "", "Base revision (defaults to frozen workspace commit)")
	create.Flags().StringVar(&opt.Purpose, "purpose", "implementation", "Purpose of the checkout")
	group.AddCommand(create)
	group.AddCommand(command("list", "List worktrees", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		return o.emit(v.Worktrees)
	}))
	return group
}
func sessionCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "session", Short: "Manage durable logical agent conversations"}
	var opt core.SessionOptions
	start := command("start", "Launch a persona in a tmux pane", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		opt.OperationKey = o.key
		v, err := s.StartSupervisedSession(c.Context(), id, opt)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	f := start.Flags()
	f.StringVar(&opt.Agent, "agent", "", "Agent ID or name")
	f.StringVar(&opt.Worktree, "worktree", "", "Worktree ID or name")
	f.StringVar(&opt.Parent, "parent", "", "Parent agent ID (defaults to orchestrator)")
	f.StringVar(&opt.ParentSessionID, "parent-session", "", "Exact parent logical Session ID for a worker session")
	f.StringVar(&opt.Profile, "profile", "", "Override model profile")
	f.StringVar(&opt.Task, "task", "", "Task ID or name")
	f.StringVar(&opt.PromptTemplate, "prompt-template", "", "Override snapshotted prompt template")
	f.BoolVar(&opt.ReadOnly, "read-only", false, "Analysis persona shares the checkout without repository write rights (cooperative)")
	group.AddCommand(start)
	var thread string
	bind := command("bind-thread <session>", "Record the native client conversation ID", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.BindThread(c.Context(), id, args[0], thread, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	bind.Args = cobra.ExactArgs(1)
	bind.Flags().StringVar(&thread, "thread-id", "", "Native client thread/session ID")
	group.AddCommand(bind)
	group.AddCommand(command("list", "List logical sessions", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		return o.emit(v.Sessions)
	}))
	history := command("history <session>", "List every concrete run in a logical session", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		sessionID := args[0]
		for _, run := range v.Runs {
			if run.ID == args[0] {
				sessionID = run.SessionID
				break
			}
		}
		runs := make([]core.Run, 0)
		for _, run := range v.Runs {
			if run.SessionID == sessionID {
				runs = append(runs, run)
			}
		}
		if len(runs) == 0 {
			return fmt.Errorf("session not found or has no runs")
		}
		return o.emit(runs)
	})
	history.Args = cobra.ExactArgs(1)
	group.AddCommand(history)
	resume := command("resume <session>", "Create a new run in a resumable logical session", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.ResumeSession(c.Context(), id, args[0], o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	resume.Args = cobra.ExactArgs(1)
	group.AddCommand(resume)
	stop := command("stop <session>", "Stop an owned tmux pane", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.StopSession(c.Context(), id, args[0], o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	stop.Args = cobra.ExactArgs(1)
	group.AddCommand(stop)
	var closeReason string
	closeCmd := command("close <session>", "Close an idle logical session", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.CloseSession(c.Context(), id, args[0], closeReason, o.key)
		if err != nil {
			return err
		}
		return o.emit(v)
	})
	closeCmd.Args = cobra.ExactArgs(1)
	closeCmd.Flags().StringVar(&closeReason, "reason", "", "Optional reason for closing the logical context")
	group.AddCommand(closeCmd)
	attach := command("attach <session>", "Focus the session's pane", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		sessionID := args[0]
		for _, run := range v.Runs {
			if run.ID == args[0] {
				sessionID = run.SessionID
				break
			}
		}
		for _, session := range v.Sessions {
			if session.ID == sessionID {
				return attachNavigation(c.Context(), s, v.Workspace.ID, core.EntityRef{Kind: "session", ID: session.ID})
			}
		}
		return fmt.Errorf("session not found")
	})
	attach.Args = cobra.ExactArgs(1)
	group.AddCommand(attach)
	return group
}

func attachNavigation(ctx context.Context, s *core.Service, selector string, ref core.EntityRef) error {
	target, err := s.ResolveNavigationTarget(ctx, selector, ref)
	if err != nil {
		return err
	}
	return terminal.NewTmuxNavigator(target.Socket).Attach(ctx, target)
}

func runCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "run", Short: "Inspect concrete client and tmux executions"}
	group.AddCommand(command("list", "List all runs", func(c *cobra.Command, _ []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		return o.emit(v.Runs)
	}))
	inspect := command("inspect <run>", "Inspect one concrete run", func(c *cobra.Command, args []string) error {
		s, id, err := o.scope()
		if err != nil {
			return err
		}
		v, err := s.Status(c.Context(), id)
		if err != nil {
			return err
		}
		for _, run := range v.Runs {
			if run.ID == args[0] {
				return o.emit(run)
			}
		}
		return fmt.Errorf("run not found")
	})
	inspect.Args = cobra.ExactArgs(1)
	group.AddCommand(inspect)
	return group
}
