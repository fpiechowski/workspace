package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"workspace/internal/core"
)

func (m *Model) openActionMenu() tea.Cmd {
	snapshotCurrent := !m.snapshot.ObservedAt.IsZero()
	if m.route.Page == "project" || m.workspaceID == "" {
		snapshotCurrent = !m.project.ObservedAt.IsZero()
	}
	if m.actionPending || m.backend == nil || !snapshotCurrent {
		m.notice = "Actions need a current workspace snapshot. Press r to refresh."
		return nil
	}
	options, targetID := m.availableActions()
	if len(options) == 0 {
		m.notice = "No actions are available for this selection."
		return nil
	}
	m.formChoice = ""
	m.formTargetID = targetID
	m.formMode = "select"
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Key("action").Title("Choose an action").Options(options...).Value(&m.formChoice),
	)).WithWidth(m.dialogWidth()).WithHeight(m.formHeight(4)).WithTheme(huhTheme(m.palette))
	return m.form.Init()
}

func (m *Model) availableActions() ([]huh.Option[string], string) {
	var actions []huh.Option[string]
	targetID, kind := m.route.EntityID, m.route.Page
	if targetID == "" && m.isCollectionPage() {
		item, _, items := m.selectedItem()
		if len(items) > 0 {
			targetID, kind = item.ID, item.Kind
		}
	}
	if kind == "orchestrator" {
		targetID = ""
	}
	add := func(label, action string) { actions = append(actions, huh.NewOption(sanitizeLine(label), action)) }
	workspaceClosed := m.snapshot.Status.Workspace.Status == "completed" || m.snapshot.Status.Workspace.Status == "archived"
	switch kind {
	case "task":
		if task, ok := m.task(targetID); ok && task.State != "accepted" && !workspaceClosed {
			add("Retry task · increment attempt and invalidate dependent results", "retry_task")
			add("Delete task · only when it has no durable results or dependents", "delete_task")
		}
	case "session":
		if session, ok := findSession(m.snapshot.Status.Sessions, targetID); ok {
			if session.CurrentRunID != "" && session.Active() {
				add("Stop current run · "+shortID(session.CurrentRunID), "stop_run")
			} else if session.ClosedAt == nil && m.snapshot.Status.Workspace.Status != "archived" {
				add("Resume this session", "resume_session")
				add("Close idle session", "close_session")
			}
			if !workspaceClosed && !session.Active() && session.AgentID != m.snapshot.Status.Workspace.OrchestratorAgentID {
				add("Delete session · only when no durable result references it", "delete_session")
			}
		}
	case "run":
		if run, ok := m.run(targetID); ok {
			if session, ok := findSession(m.snapshot.Status.Sessions, run.SessionID); ok && session.CurrentRunID == run.ID && run.Active() {
				targetID = session.ID
				add("Stop current run", "stop_run")
			}
		}
	case "service":
		if service, ok := findService(m.snapshot.Services, targetID); ok && service.Active() {
			add("Stop service · "+service.Name, "stop_service")
		}
	case "workspace":
		add("Create a new workspace", "create_workspace")
		if item, _, items := m.selectedItem(); len(items) > 0 && item.ID == targetID && item.State != "error" {
			add("Jump to the workspace tmux session", "jump")
			add("Permanently delete workspace and all local work", "delete_workspace")
		}
	case "project":
		add("Create a new workspace", "create_workspace")
	case "dashboard", "orchestrator", "runtime":
		workspaceState := m.snapshot.Status.Workspace.Status
		if workspaceState != "archived" {
			add("Start / resume orchestrator", "start_orchestrator")
		}
		if workspaceState == "paused" {
			add("Resume workspace", "resume_workspace")
		} else if workspaceState != "archived" && workspaceState != "completed" {
			add("Pause workspace (leave runs untouched)", "pause")
			add("Pause and interrupt active Runs and services", "pause_interrupt")
		}
		if workspaceState == "needs_workflow" {
			add("Select workflow", "select_workflow")
		}
		if m.snapshot.Status.Workspace.Manual() && workspaceState != "completed" && workspaceState != "archived" {
			add("Complete this manual workspace", "complete_workspace")
		}
		if workspaceState == "completed" {
			add("Reopen completed workspace", "reopen_workspace")
			add("Archive completed workspace", "archive_workspace")
		}
		if workspaceState != "archived" {
			add("Reconcile runtime", "reconcile")
		}
		if kind == "runtime" {
			if m.uiStatus.Desired {
				add("Hide managed interface", "hide_managed_tui")
			} else {
				add("Show managed interface", "show_managed_tui")
			}
		}
	default:
		if targetID == "" && m.workspaceID != "" {
			if m.snapshot.Status.Workspace.Status != "archived" {
				add("Start / resume orchestrator", "start_orchestrator")
			}
			if m.snapshot.Status.Workspace.Status == "paused" {
				add("Resume workspace", "resume_workspace")
			} else if m.snapshot.Status.Workspace.Status != "archived" {
				add("Pause workspace (leave runs untouched)", "pause")
				add("Pause and interrupt active Runs and services", "pause_interrupt")
			}
		}
	}
	return actions, targetID
}

func (m *Model) beginAction(action, targetID string) tea.Cmd {
	if action == "jump" {
		return m.jump(core.EntityRef{Kind: "workspace", ID: targetID})
	}
	if action == "create_workspace" {
		targetID = ""
	}
	call := ActionCall{
		Action: action, TargetID: targetID, WorkspaceID: m.workspaceID, Key: core.ID("tui"),
		ExpectedRevision: m.snapshot.Status.Workspace.Revision,
	}
	if action == "delete_workspace" {
		for _, workspace := range m.project.Workspaces {
			if workspace.ID == targetID {
				call.ExpectedRevision = workspace.Revision
				call.TargetName = firstNonempty(workspace.Title, workspace.ID)
				call.TargetDetails = "This permanently stops its runtime and deletes every task, session, artifact, uncommitted file, worktree and local workspace branch. Release or archive is not required. This cannot be undone."
				break
			}
		}
	}
	if action == "archive_workspace" {
		call.TargetName = firstNonempty(m.snapshot.Status.Workspace.Title, m.workspaceID)
		if m.snapshot.Status.Workspace.Manual() {
			call.TargetDetails = "Archive this completed manual workspace. The core still requires no active Sessions or services; no workflow release reference is needed. Worktrees and history are retained until explicitly cleaned or deleted."
		} else {
			call.TargetDetails = "Archive this completed workspace. The core still requires confirmed release and no active Sessions or services. Worktrees and history are retained until explicitly cleaned or deleted."
		}
	}
	if action == "complete_workspace" {
		call.TargetName = firstNonempty(m.snapshot.Status.Workspace.Title, m.workspaceID)
		call.TargetDetails = "Complete this manual workspace. The core refuses completion while Runs, services or non-accepted tasks remain; archive then no longer requires a workflow release reference."
	}
	if action == "reopen_workspace" {
		call.TargetName = firstNonempty(m.snapshot.Status.Workspace.Title, m.workspaceID)
		call.TargetDetails = "Reopen this completed workspace for explicitly authorized follow-up work. The reason is recorded, the current revision is guarded, accepted tasks and provenance are preserved, and release/integration/testing state must be established again before completion."
	}
	m.formAction = call
	m.formReason = ""
	m.formConfirm = false
	switch action {
	case "retry_task", "delete_task":
		if task, ok := m.task(targetID); ok {
			call.ExpectedAttempt = task.Attempt
			call.TargetName = firstNonempty(task.Title, task.ID)
			if action == "retry_task" {
				call.TargetDetails = m.retryImpact(task)
			} else {
				call.TargetDetails = "The task disappears from normal TUI views. The core refuses deletion when dependencies, active sessions, handoffs, artifacts, checks, integration or change requests still reference it."
			}
		}
	case "stop_run":
		if session, ok := findSession(m.snapshot.Status.Sessions, targetID); ok {
			call.ExpectedRunID = session.CurrentRunID
			call.TargetName = firstNonempty(session.AgentSnapshot.Name, session.ID)
			call.TargetDetails = fmt.Sprintf("Session %s · task %s · attempt %d · worktree %s", session.ID, firstNonempty(session.TaskID, "none"), session.TaskAttempt, firstNonempty(session.WorktreeID, "none"))
		}
	case "resume_session", "close_session", "delete_session":
		if session, ok := findSession(m.snapshot.Status.Sessions, targetID); ok {
			call.TargetName = firstNonempty(session.AgentSnapshot.Name, session.ID)
			call.TargetDetails = fmt.Sprintf("Session %s · task %s · attempt %d · worktree %s · model %s", session.ID, firstNonempty(session.TaskID, "none"), session.TaskAttempt, firstNonempty(session.WorktreeID, "none"), firstNonempty(session.Route.Model, "unspecified"))
			if action == "delete_session" {
				call.ExpectedRunID = session.LastRunID
				call.TargetDetails += "\nThe core refuses deletion while the session is active or referenced by durable results or messages."
			}
		}
	case "stop_service":
		if service, ok := findService(m.snapshot.Services, targetID); ok {
			call.TargetName = firstNonempty(service.Name, service.ID)
			call.TargetDetails = "Worktree " + firstNonempty(service.WorktreeID, "none")
		}
	case "pause_interrupt":
		call.ExpectedRunIDs = make([]string, 0)
		for _, session := range m.snapshot.Status.Sessions {
			if !session.Active() || session.CurrentRunID == "" {
				continue
			}
			if run, ok := m.run(session.CurrentRunID); ok && run.Active() {
				call.ExpectedRunIDs = append(call.ExpectedRunIDs, run.ID)
			}
		}
		sort.Strings(call.ExpectedRunIDs)
		call.ExpectedServiceIDs = make([]string, 0)
		for _, service := range m.snapshot.Services {
			if service.Active() {
				call.ExpectedServiceIDs = append(call.ExpectedServiceIDs, service.ID)
			}
		}
		sort.Strings(call.ExpectedServiceIDs)
	}
	m.formAction = call
	if action == "create_workspace" {
		backend, ok := m.backend.(WorkflowBackend)
		if !ok {
			m.loadError = "this backend does not support workspace creation"
			m.rebuildViewport()
			return nil
		}
		m.actionPending = true
		m.notice = "Loading available workflows…"
		backend, gen := backend, m.generation
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			names, err := backend.WorkflowNames(ctx, "")
			return workflowNamesMsg{generation: gen, action: action, names: names, err: err}
		}
	}
	if action == "select_workflow" {
		backend, ok := m.backend.(WorkflowBackend)
		if !ok {
			m.loadError = "this backend does not support workflow selection"
			m.rebuildViewport()
			return nil
		}
		m.actionPending = true
		m.notice = "Loading available workflows…"
		m.loadError = ""
		backend, workspace, gen := backend, m.workspaceID, m.generation
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			names, err := backend.WorkflowNames(ctx, workspace)
			return workflowNamesMsg{generation: gen, action: action, names: names, err: err}
		}
	}
	if action == "delete_workspace" || action == "delete_task" || action == "delete_session" {
		return m.openDestructiveConfirm(action)
	}
	if action == "retry_task" || action == "close_session" || action == "reopen_workspace" {
		m.formMode = "reason"
		label := "Reason for this action"
		if action == "retry_task" {
			label = "Why should this task be retried?"
		}
		m.form = huh.NewForm(huh.NewGroup(
			huh.NewInput().Key("reason").Title(label).Placeholder("Enter a short reason").Value(&m.formReason).Validate(func(value string) error {
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("a reason is required")
				}
				return nil
			}),
		)).WithWidth(m.dialogWidth()).WithHeight(m.formHeight(4)).WithTheme(huhTheme(m.palette))
		return m.form.Init()
	}
	return m.openConfirm(action)
}

// createManualChoice is the in-form sentinel for the explicit no-workflow
// creation option. It cannot collide with a workflow name because core
// validateName rejects names that do not start with a lowercase letter.
const createManualChoice = "<manual>"

// createWorkflowChoice splits the creation selector value into the workflow
// name and the explicit no-workflow mode passed to core.CreateOptions.
func createWorkflowChoice(choice string) (string, bool) {
	if choice == createManualChoice {
		return "", true
	}
	return choice, false
}

func (m *Model) openCreateWorkspaceForm(names []string) tea.Cmd {
	m.formTitle, m.formInput, m.formWorkflow = "", "", ""
	options := []huh.Option[string]{
		huh.NewOption("Choose later", ""),
		huh.NewOption("No workflow (manual orchestration)", createManualChoice),
	}
	for _, name := range names {
		options = append(options, huh.NewOption(sanitizeLine(name), name))
	}
	m.formMode = "create_workspace"
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().Key("title").Title("Workspace title").Placeholder("Short descriptive title").Value(&m.formTitle).Validate(func(value string) error {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("a title is required")
			}
			return nil
		}),
		huh.NewInput().Key("input").Title("Issue or task description").Placeholder("What should this workspace accomplish?").Value(&m.formInput).Validate(func(value string) error {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("a description is required")
			}
			return nil
		}),
		huh.NewSelect[string]().Key("workflow").Title("Workflow").Options(options...).Value(&m.formWorkflow),
	)).WithWidth(m.dialogWidth()).WithHeight(m.formHeight(7)).WithTheme(huhTheme(m.palette))
	return m.form.Init()
}

func (m *Model) openDestructiveConfirm(action string) tea.Cmd {
	m.formTyped = ""
	target := m.formAction.TargetID
	m.formMode = "destructive"
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().Key("confirm_id").Title(actionCaption(m.formAction)).Description("Type the exact ID to confirm permanent removal:\n" + target).Value(&m.formTyped).Validate(func(value string) error {
			if strings.TrimSpace(value) != target {
				return fmt.Errorf("enter the exact ID %s", target)
			}
			return nil
		}),
	)).WithWidth(m.dialogWidth()).WithHeight(m.formHeight(5)).WithTheme(huhTheme(m.palette))
	return m.form.Init()
}

func (m *Model) openConfirm(action string) tea.Cmd {
	m.formConfirm = false
	caption := sanitizeLine(actionCaption(m.formAction))
	m.formMode = "confirm"
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Key("confirm").Title(caption).Description(actionDescription(action, m.formAction)).Affirmative("Confirm").Negative("Cancel").Value(&m.formConfirm),
	)).WithWidth(m.dialogWidth()).WithHeight(m.formHeight(4)).WithTheme(huhTheme(m.palette))
	return m.form.Init()
}

func (m *Model) updateForm(message tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.form.Update(message)
	form, ok := updated.(*huh.Form)
	if ok {
		m.form = form
	}
	if !ok || m.form.State == huh.StateAborted {
		m.form = nil
		m.formMode = ""
		m.formConfirm = false
		m.rebuildViewport()
		return m, cmd
	}
	if m.form.State != huh.StateCompleted {
		return m, cmd
	}
	mode := m.formMode
	if mode == "select" {
		action := m.form.GetString("action")
		target := m.formTargetID
		m.form = nil
		m.formMode = ""
		return m, tea.Batch(cmd, m.beginAction(action, target))
	}
	if mode == "reason" {
		m.formAction.Reason = strings.TrimSpace(m.form.GetString("reason"))
		if m.formAction.Action == "retry_task" {
			if task, ok := m.task(m.formAction.TargetID); ok {
				m.formAction.TargetDetails = m.retryImpact(task)
			}
		}
		m.form = nil
		m.formMode = ""
		return m, tea.Batch(cmd, m.openConfirm(m.formAction.Action))
	}
	if mode == "workflow" {
		selected := m.form.GetString("workflow")
		m.form = nil
		m.formMode = ""
		return m, tea.Batch(cmd, m.confirmWorkflowSelection(selected))
	}
	if mode == "create_workspace" {
		m.formAction.TargetName = strings.TrimSpace(m.form.GetString("title"))
		m.formAction.Input = strings.TrimSpace(m.form.GetString("input"))
		m.formAction.Workflow, m.formAction.NoWorkflow = createWorkflowChoice(m.form.GetString("workflow"))
		m.form = nil
		m.formMode = ""
		return m, tea.Batch(cmd, m.runAction(m.formAction))
	}
	if mode == "destructive" {
		m.form = nil
		m.formMode = ""
		return m, tea.Batch(cmd, m.runAction(m.formAction))
	}
	confirmed := m.form.GetBool("confirm")
	m.form = nil
	m.formMode = ""
	if !confirmed {
		m.notice = "Action cancelled."
		m.rebuildViewport()
		return m, cmd
	}
	return m, tea.Batch(cmd, m.runAction(m.formAction))
}

func (m *Model) confirmWorkflowSelection(name string) tea.Cmd {
	m.formAction.TargetID = name
	return m.openConfirm(m.formAction.Action)
}

func (m *Model) runAction(call ActionCall) tea.Cmd {
	backend, ok := m.backend.(ActionBackend)
	if !ok {
		m.loadError = "this backend does not support mutations"
		return nil
	}
	m.actionPending, m.mutationPending = true, true
	m.actionFailure = false
	m.actionCompleted = false
	m.lastAction = &call
	m.notice = "Working… " + actionCaption(call)
	backend, workspace, gen := backend, m.workspaceID, m.generation
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout(call))
		defer cancel()
		return actionResultMsg{generation: gen, call: call, err: backend.PerformAction(ctx, workspace, call)}
	}
}

func actionTimeout(call ActionCall) time.Duration {
	if call.Action == "delete_workspace" {
		// Removing large worktrees from a Windows-mounted filesystem can take
		// several minutes. Interrupting Git midway leaves a prunable checkout.
		return 10 * time.Minute
	}
	return 45 * time.Second
}

func (m *Model) finishAction(message actionResultMsg) tea.Cmd {
	if message.generation != m.generation {
		return nil
	}
	m.actionPending, m.mutationPending = false, false
	if message.err != nil {
		m.actionFailure = true
		m.actionCompleted = false
		m.lastAction = &message.call
		m.loadError = sanitizeLine(message.err.Error())
		m.notice = "Operation failed: " + m.loadError + " · y retry · a new action"
		m.rebuildViewport()
		return nil
	}
	m.actionFailure = false
	m.actionCompleted = true
	m.lastAction = nil
	m.loadError = ""
	m.notice = actionCompletedNotice
	m.generation++
	m.projectPending, m.snapshotPending, m.runtimePending, m.uiPending = false, false, false, false
	if message.call.Action == "delete_workspace" {
		m.workspaceID = ""
		m.stack = nil
		m.activateRoute(route{Page: "project"})
	}
	if message.call.Action == "delete_task" && m.route.Page == "task" || message.call.Action == "delete_session" && m.route.Page == "session" {
		m.pop()
	}
	if message.call.NavigationRef != nil {
		return tea.Batch(m.beginRefresh(), m.jumpAttempt(*message.call.NavigationRef, true))
	}
	if message.call.OpenTerminal {
		ref := core.EntityRef{Kind: "session", ID: message.call.TargetID}
		if message.call.Action == "start_orchestrator" {
			ref = core.EntityRef{Kind: "orchestrator"}
		}
		return tea.Batch(m.beginRefresh(), m.jump(ref))
	}
	return m.beginRefresh()
}

func actionCaption(call ActionCall) string {
	targetID := call.TargetID
	targetName := firstNonempty(call.TargetName, targetID)
	switch call.Action {
	case "start_orchestrator":
		return "Start or resume the orchestrator in workspace " + call.WorkspaceID
	case "create_workspace":
		return "Create workspace " + firstNonempty(call.TargetName, "Untitled issue")
	case "delete_workspace":
		return "Permanently delete workspace " + targetName + " · " + targetID
	case "archive_workspace":
		return "Archive completed workspace " + targetName
	case "complete_workspace":
		return "Complete manual workspace " + targetName
	case "pause":
		return "Pause the workspace and leave current Runs untouched"
	case "resume_workspace":
		return "Resume paused workspace; this does not restart stopped Runs"
	case "reopen_workspace":
		return "Reopen completed workspace " + targetName
	case "reconcile":
		if call.NavigationRef != nil {
			return "Terminal unavailable. Reconcile workspace runtime?"
		}
		return "Reconcile recorded sessions against the selected tmux runtime"
	case "pause_interrupt":
		return "Pause workspace and stop only the confirmed active Runs and services"
	case "select_workflow":
		return "Select workflow " + targetID + " for workspace " + call.WorkspaceID
	case "show_managed_tui":
		return "Show or restore the managed TUI panel"
	case "hide_managed_tui":
		return "Hide the managed TUI panel"
	case "resume_session":
		return "Resume session " + targetName + " · " + targetID
	case "stop_run":
		return "Stop only current Run " + call.ExpectedRunID + " from session " + targetID
	case "close_session":
		return "Close idle session " + targetName + " · " + targetID
	case "delete_session":
		return "Delete session " + targetName + " · " + targetID
	case "retry_task":
		return "Retry task " + targetName + " · " + targetID + " and reset affected downstream tasks"
	case "delete_task":
		return "Delete task " + targetName + " · " + targetID
	case "stop_service":
		return "Stop service " + targetName + " · " + targetID
	default:
		return "Apply " + call.Action
	}
}

func actionDescription(action string, call ActionCall) string {
	if call.NavigationRef != nil {
		return "Refresh recorded runtime state and recover eligible processes. This can restart the orchestrator; stopped workers are not automatically restarted. Then retry the selected terminal. A historical run will not redirect to a newer run."
	}
	if action == "pause_interrupt" {
		runs := strings.Join(call.ExpectedRunIDs, ", ")
		services := strings.Join(call.ExpectedServiceIDs, ", ")
		if runs == "" {
			runs = "none"
		}
		if services == "" {
			services = "none"
		}
		return fmt.Sprintf("This pauses the workspace, then stops these exact targets:\nRuns: %s\nServices: %s\nThe core rejects the action if either list changed.", runs, services)
	}
	if action == "select_workflow" {
		return "Select “" + sanitizeLine(call.TargetID) + "” for this workspace. The core checks that the workspace revision is still current."
	}
	if action == "reopen_workspace" {
		return "Record why this completed workspace should receive follow-up work. The core requires the current revision and rejects active worker sessions or services; the prior completed document is preserved in history."
	}
	if action == "show_managed_tui" || action == "hide_managed_tui" {
		return "The workspace supervisor will reconcile the managed TUI panel to this desired state."
	}
	if call.TargetDetails != "" {
		return sanitizeLine(call.TargetDetails)
	}
	if action == "retry_task" {
		return "Reason: " + sanitizeLine(call.Reason)
	}
	return "The core will validate the current workspace and actor before it applies this action."
}

func (m *Model) retryImpact(task core.Task) string {
	affected := map[string]bool{task.ID: true}
	for changed := true; changed; {
		changed = false
		for _, candidate := range m.snapshot.Status.Workspace.Tasks {
			if affected[candidate.ID] {
				continue
			}
			for _, dependency := range candidate.DependsOn {
				if affected[dependency] {
					affected[candidate.ID] = true
					changed = true
					break
				}
			}
		}
	}
	var names []string
	for _, candidate := range m.snapshot.Status.Workspace.Tasks {
		if affected[candidate.ID] {
			names = append(names, firstNonempty(candidate.Title, candidate.ID)+" ("+candidate.ID+")")
		}
	}
	return fmt.Sprintf("Attempt %d → %d. This resets affected tasks and their accepted results: %s. Reason: %s", task.Attempt, task.Attempt+1, strings.Join(names, ", "), sanitizeLine(m.formReason))
}

// dialogSeverity classifies the guarded action shown in a modal form.
type dialogSeverity int

const (
	severityNeutral dialogSeverity = iota
	severityWarning
	severityDanger
)

const (
	// dialogMaxWidth bounds the form block on large terminals.
	dialogMaxWidth = 76
	// dialogMaxHeight keeps the form, dialog header and border inside the
	// content region at the 100x24 wide breakpoint.
	dialogMaxHeight = 14
)

// dialogWidth is the bounded content width shared by every Huh form. It never
// exceeds the terminal and never drops below the minimum usable width.
func (m *Model) dialogWidth() int {
	return clamp(m.width-8, 20, dialogMaxWidth)
}

// formHeight bounds a form to the dialog maximum while preserving the minimum
// height each field group needs.
func (m *Model) formHeight(minHeight int) int {
	return clamp(m.height-8, minHeight, dialogMaxHeight)
}

// dialogSeverityKind derives the severity from the form mode and the guarded
// action. Destructive confirmations always render with danger emphasis, while
// selection and creation forms stay neutral even after a previous action.
func (m *Model) dialogSeverityKind() dialogSeverity {
	switch m.formMode {
	case "destructive":
		return severityDanger
	case "select", "workflow", "create_workspace":
		return severityNeutral
	default:
		return actionSeverity(m.formAction.Action)
	}
}

func actionSeverity(action string) dialogSeverity {
	switch action {
	case "delete_workspace", "delete_task", "delete_session", "pause_interrupt":
		return severityDanger
	case "archive_workspace", "complete_workspace", "reopen_workspace", "stop_run", "stop_service", "retry_task", "pause":
		return severityWarning
	default:
		return severityNeutral
	}
}

// dialogTitle names the form's purpose above the fields.
func (m *Model) dialogTitle() string {
	switch m.formMode {
	case "select":
		return "Choose an action"
	case "reason":
		return "Confirm with a reason"
	case "workflow":
		return "Select workflow"
	case "create_workspace":
		return "Create workspace"
	case "destructive":
		return "Destructive confirmation"
	case "confirm":
		return "Confirm action"
	default:
		return "Action"
	}
}

// dialogTarget summarizes what the form acts on without replacing the exact ID
// shown by destructive validation.
func (m *Model) dialogTarget() string {
	switch m.formMode {
	case "create_workspace":
		return "New workspace"
	case "workflow":
		return m.workspaceID
	case "select":
		return firstNonempty(firstNonempty(m.formTargetID, m.route.SelectedID), m.route.EntityID)
	}
	if name, id := m.formAction.TargetName, m.formAction.TargetID; name != "" && id != "" && name != id {
		return name + " · " + id
	}
	return firstNonempty(firstNonempty(m.formAction.TargetName, m.formAction.TargetID), m.workspaceID)
}

// dialogSeverityText is the text marker for the severity. The bracketed marker
// keeps the meaning readable without color.
func (m *Model) dialogSeverityText() string {
	switch m.dialogSeverityKind() {
	case severityDanger:
		return "[!] Destructive · requires explicit confirmation"
	case severityWarning:
		return "[!] Review the impact before continuing"
	default:
		switch m.formMode {
		case "select":
			return "· Choose an action to continue"
		case "workflow":
			return "· Choose a configured workflow"
		case "create_workspace":
			return "· New workspace"
		default:
			return "· Confirmation required"
		}
	}
}

// dialogHeader describes the form before its fields: title, severity, target and
// the fixed Esc cancellation affordance.
func (m *Model) dialogHeader() []string {
	width := m.dialogWidth()
	truncate := func(value string) string {
		return ansi.TruncateWc(sanitizeLine(value), width, "…")
	}
	lines := []string{m.palette.headingStyle().Render(truncate(m.dialogTitle()))}
	lines = append(lines, m.palette.severityStyle(m.dialogSeverityKind()).Render(truncate(m.dialogSeverityText())))
	if target := sanitizeLine(m.dialogTarget()); target != "" {
		line := m.palette.labelStyle().Render("Target ") + m.palette.valueStyle().Render(target)
		lines = append(lines, ansi.TruncateWc(line, width, "…"))
	}
	lines = append(lines, m.palette.metaStyle().Render(truncate("Esc cancels without applying the action.")))
	return lines
}

// dialogView renders the modal form. Large terminals get a centered, bounded
// dialog with a severity border; compact terminals use the full body so no field
// is hidden behind a boundary.
func (m *Model) dialogView(mode layoutMode) []string {
	content := append(m.dialogHeader(), strings.Split(m.form.View(), "\n")...)
	if mode != layoutWide {
		return content
	}
	box := m.palette.dialogStyle(m.dialogSeverityKind()).Width(m.dialogWidth()).Render(strings.Join(content, "\n"))
	return centerBlock(strings.Split(box, "\n"), m.width, m.contentHeight())
}

func huhTheme(p palette) *huh.Theme {
	theme := huh.ThemeBase()
	if p.noColor {
		// The selected button carries an explicit text marker so the primary
		// choice stays visible without color. Transform keeps the real label.
		selected := lipgloss.NewStyle().Transform(func(label string) string { return "> " + label }).Bold(true)
		unselected := lipgloss.NewStyle()
		theme.Focused.FocusedButton = selected
		theme.Focused.BlurredButton = unselected
		theme.Blurred.FocusedButton = selected
		theme.Blurred.BlurredButton = unselected
		return theme
	}
	theme.Focused.SelectSelector = lipgloss.NewStyle().Foreground(p.accent).Bold(true).SetString("> ")
	theme.Focused.Title = lipgloss.NewStyle().Foreground(p.primary).Bold(true)
	theme.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(p.danger).SetString(" !")
	theme.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(p.danger)
	theme.Focused.FocusedButton = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(p.success).Padding(0, 2)
	theme.Focused.BlurredButton = lipgloss.NewStyle().Foreground(p.primary).Padding(0, 2)
	theme.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(p.accent)
	theme.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(p.secondary)
	theme.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(p.primary)
	return theme
}
