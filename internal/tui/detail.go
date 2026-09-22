package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"workspace/internal/core"
)

func (m *Model) detailContent() string {
	if m.initialError != "" || m.route.Page == "error" {
		doc := newDetailDoc(m.palette, m.detailWidth())
		doc.title("Workspace", "", "")
		doc.warning("Could not load this workspace")
		doc.body(m.initialError)
		doc.action("Press w to choose a workspace or q to quit.")
		return doc.render()
	}
	if m.route.Page == "preview" {
		return m.previewContent()
	}
	if m.route.Page == "runtime" {
		return m.runtimeContent()
	}
	if m.route.Page == "orchestrator" {
		return m.orchestratorContent()
	}
	if m.route.Page == "issue" {
		return m.issueContent()
	}
	if m.route.Page == "dispatcher" {
		return m.dispatcherContent()
	}
	id := m.route.EntityID
	doc := newDetailDoc(m.palette, m.detailWidth())
	switch m.route.Page {
	case "task":
		task, ok := m.task(id)
		if !ok {
			return m.missingEntity("task", id)
		}
		doc.title("Task", task.Title, task.State)
		doc.action("t open / resume terminal")
		doc.section("Facts")
		doc.field("Attempt", fmt.Sprint(task.Attempt))
		doc.field("Profile", task.Profile)
		doc.field("Role", task.Role)
		doc.field("Activity", m.taskItem(task).Subtitle)
		doc.section("Goal")
		doc.body(task.Goal)
		if task.Reason != "" {
			doc.section("Reason")
			doc.body(task.Reason)
		}
		doc.section("Acceptance criteria")
		doc.bullets(task.AcceptanceCriteria)
		doc.section("Depends on")
		doc.bullets(task.DependsOn)
		doc.section("Required artifacts")
		doc.bullets(task.RequiredArtifacts)
		doc.section("Related resources")
		doc.link("Shortcuts, not primary navigation: 1 Sessions · 2 Worktrees · 3 Results · f toggles attempt history")
		doc.section("Provenance")
		doc.provenance("ID", task.ID)
		if task.AcceptedHandoff != "" {
			doc.provenance("Accepted handoff", task.AcceptedHandoff)
		}
	case "worktree":
		worktree, ok := findWorktree(m.snapshot.Status.Worktrees, id)
		if !ok {
			return m.missingEntity("worktree", id)
		}
		doc.title("Worktree", worktree.Name, worktree.State)
		doc.section("Facts")
		doc.field("Path", worktree.Path)
		doc.field("Branch", worktree.Branch)
		doc.field("Purpose", worktree.Purpose)
		if observation, exists := m.worktreeInspection[id]; exists {
			doc.section("Git observation")
			doc.field("Checked", formatTime(observation.CheckedAt))
			if observation.Error != "" {
				doc.warning(observation.Error)
			} else {
				dirty := "clean"
				if observation.Dirty {
					dirty = "dirty"
				}
				doc.field("HEAD", observation.Head)
				doc.field("Working tree", dirty)
			}
		} else if m.worktreePending {
			doc.section("Git observation")
			doc.body("Inspecting HEAD and working tree…")
		}
		if relation, ok := worktreeRelation(m.snapshot.Relations.Worktrees, id); ok {
			doc.section("Relations")
			doc.field("Task IDs", strings.Join(relation.TaskIDs, ", "))
			doc.field("Session IDs", strings.Join(relation.SessionIDs, ", "))
			doc.field("Service IDs", strings.Join(relation.ServiceIDs, ", "))
		}
		var writers, readers, services []string
		for _, session := range m.snapshot.Status.Sessions {
			if session.WorktreeID != id || !session.Active() {
				continue
			}
			run, ok := m.run(session.CurrentRunID)
			if !ok || !run.Active() {
				continue
			}
			label := firstNonempty(session.AgentSnapshot.Name, session.ID) + " (" + session.ID + ")"
			if session.ReadOnly {
				readers = append(readers, label)
			} else {
				writers = append(writers, label)
			}
		}
		for _, service := range m.snapshot.Services {
			if service.WorktreeID == id && service.Active() {
				services = append(services, firstNonempty(service.Name, service.ID)+" ("+service.ID+")")
			}
		}
		doc.section("Active processes")
		doc.field("Writers", firstNonempty(strings.Join(writers, ", "), "none"))
		doc.field("Read-only", firstNonempty(strings.Join(readers, ", "), "none"))
		doc.field("Services", firstNonempty(strings.Join(services, ", "), "none"))
		doc.section("Provenance")
		doc.provenance("ID", worktree.ID)
	case "session":
		session, ok := findSession(m.snapshot.Status.Sessions, id)
		if !ok {
			return m.missingEntity("session", id)
		}
		doc.title("Session", session.AgentSnapshot.Name, session.LifecycleState)
		doc.action("Enter opens Run history · g jumps to the current Run")
		doc.section("Facts")
		doc.field("Role", session.AgentSnapshot.Role)
		doc.field("Profile", session.Profile)
		doc.field("Client", session.ClientSnapshot.Adapter+" · "+session.Route.Model)
		doc.field("Task", session.TaskID+" · attempt "+fmt.Sprint(session.TaskAttempt))
		doc.field("Parent Agent", session.ParentAgentID)
		doc.field("Parent Session", session.ParentSessionID)
		doc.field("Worktree", session.WorktreeID)
		doc.field("Read only", fmt.Sprintf("%t", session.ReadOnly))
		doc.field("Current run", session.CurrentRunID)
		doc.field("Last run", session.LastRunID)
		doc.field("Native thread", session.ClientThreadID)
		if relation, ok := sessionRelation(m.snapshot.Relations.Sessions, session.ID); ok {
			doc.section("Relations")
			doc.field("Run IDs", strings.Join(relation.RunIDs, ", "))
			doc.field("Message IDs", strings.Join(relation.MessageIDs, ", "))
			doc.field("Handoff IDs", strings.Join(relation.HandoffIDs, ", "))
		}
		doc.section("Provenance")
		doc.provenance("ID", session.ID)
	case "run":
		run, ok := m.run(id)
		if !ok {
			return m.missingEntity("run", id)
		}
		current := false
		for _, session := range m.snapshot.Status.Sessions {
			if session.ID == run.SessionID {
				current = session.CurrentRunID == run.ID
				break
			}
		}
		doc.title("Run", run.ID, run.State)
		doc.section("Facts")
		doc.field("Session", run.SessionID)
		doc.field("Generation", fmt.Sprint(run.Generation))
		doc.field("Current", fmt.Sprint(current))
		doc.field("Provider / model", run.Route.Provider+" / "+run.Route.Model)
		doc.field("Client", run.Route.Client)
		doc.field("Pane / window", run.PaneID+" / "+run.WindowID)
		if run.Error != "" {
			doc.section("Error")
			doc.warning(run.Error)
		}
		if !current {
			doc.warning("This run is historical. Jump is unavailable; open its current Session instead.")
		}
		doc.section("Provenance")
		doc.provenance("Created", formatTime(run.CreatedAt))
		doc.provenance("Finished", formatTimePtr(run.FinishedAt))
		doc.provenance("Exit code", formatIntPtr(run.ExitCode))
		doc.provenance("Client state", run.ClientState)
	case "agent":
		agent, ok := findAgent(m.snapshot.Status.Agents, id)
		if !ok {
			return m.missingEntity("agent", id)
		}
		doc.title("Agent", agent.Name, "")
		doc.section("Facts")
		doc.field("Role", agent.Role)
		doc.field("Profile", agent.Profile)
		doc.section("Instructions")
		doc.body(agent.Instructions)
		doc.section("Sessions")
		sessions := make([]string, 0)
		for _, session := range m.snapshot.Status.Sessions {
			if session.AgentID == id {
				sessions = append(sessions, session.ID+" · "+session.LifecycleState+" · "+session.CurrentRunID)
			}
		}
		doc.bullets(sessions)
		doc.section("Provenance")
		doc.provenance("ID", agent.ID)
	case "service":
		service, ok := findService(m.snapshot.Services, id)
		if !ok {
			return m.missingEntity("service", id)
		}
		doc.title("Service", service.Name, service.State)
		doc.section("Facts")
		doc.field("Worktree", service.WorktreeID)
		doc.field("CWD", service.CWD)
		doc.field("Pane", service.PaneID)
		doc.field("Exit code", formatIntPtr(service.ExitCode))
		doc.section("Command")
		doc.body(strings.Join(service.Argv, " "))
		doc.section("Provenance")
		doc.provenance("ID", service.ID)
	case "artifact":
		artifact, ok := findArtifact(m.snapshot.Status.Workspace.Artifacts, id)
		if !ok {
			return m.missingEntity("artifact", id)
		}
		doc.title("Artifact", artifact.Name, "")
		doc.action("Enter to preview")
		doc.section("Facts")
		doc.field("Kind", artifact.Kind)
		doc.field("Size", fmt.Sprint(artifact.Size))
		doc.section("Provenance")
		doc.provenance("ID", artifact.ID)
		doc.provenance("Digest", artifact.Digest)
		doc.provenance("Source handoff", artifact.SourceHandoff)
		doc.provenance("Task / session / run", artifact.TaskID+" / "+artifact.SessionID+" / "+artifact.RunID)
		doc.provenance("Created", formatTime(artifact.CreatedAt))
	case "handoff":
		handoff, ok := findHandoff(m.snapshot.Handoffs, id)
		if !ok {
			return m.missingEntity("handoff", id)
		}
		state := handoff.State
		if handoff.Stale {
			state = "stale · " + state
		}
		doc.title("Handoff", handoff.Outcome, state)
		doc.section("Facts")
		doc.field("Task / attempt", handoff.TaskID+" / "+fmt.Sprint(handoff.Attempt))
		doc.field("From / to Agent", handoff.FromAgent+" / "+handoff.ToAgent)
		doc.field("From / to Session", handoff.FromSession+" / "+handoff.ToSession)
		doc.section("Summary")
		doc.body(handoff.Summary)
		doc.section("Risks")
		doc.bullets(handoff.Risks)
		if handoff.Feedback != "" {
			doc.section("Feedback")
			doc.body(handoff.Feedback)
		}
		doc.section("Artifacts")
		doc.bullets(handoff.ArtifactIDs)
		doc.section("Provenance")
		doc.provenance("ID", handoff.ID)
	case "check":
		check, ok := findCheck(m.snapshot.Checks, id)
		if !ok {
			return m.missingEntity("check", id)
		}
		doc.title("Check", check.ID, check.State)
		doc.action("Enter to preview captured output")
		doc.section("Facts")
		doc.field("Exit code", fmt.Sprint(check.ExitCode))
		doc.field("Task / attempt", check.TaskID+" / "+fmt.Sprint(check.Attempt))
		doc.section("Command")
		doc.body(strings.Join(check.Argv, " "))
		doc.section("Provenance")
		doc.provenance("Head", check.Head)
		doc.provenance("End head", check.EndHead)
		doc.provenance("Digest", check.Digest)
	case "decision":
		decision, ok := findDecision(m.snapshot.Status.Workspace.Decisions, m.snapshot.Status.Workspace.PendingDecision, id)
		if !ok {
			return m.missingEntity("decision", id)
		}
		doc.title("Decision", decision.Question, firstNonempty(decision.Answer, "pending"))
		doc.section("Facts")
		doc.field("Kind", decision.Kind)
		doc.section("Options")
		doc.bullets(decision.Options)
		if decision.Answer != "" {
			doc.section("Answer")
			doc.body(decision.Answer)
			doc.field("Reason", decision.Reason)
		}
		doc.section("Provenance")
		doc.provenance("ID", decision.ID)
	case "change_request":
		request, ok := findChangeRequest(m.snapshot.Status.Workspace.ChangeRequests, id)
		if !ok {
			return m.missingEntity("change request", id)
		}
		doc.title("Change request", request.Title, request.State)
		doc.section("Facts")
		doc.field("Branch", request.Branch)
		doc.field("Target", request.Target)
		doc.field("Worktree", request.WorktreeID)
		doc.section("Body")
		doc.body(request.Body)
		doc.section("URL")
		doc.body(request.URL)
		doc.section("Provenance")
		doc.provenance("ID", request.ID)
		doc.provenance("Head", request.HeadCommit)
	case "runtime":
		return m.runtimeContent()
	default:
		doc.title(m.collectionTitle(), "", "")
		doc.provenance("ID", id)
	}
	return doc.render()
}

func (m *Model) issueContent() string {
	doc := newDetailDoc(m.palette, m.detailWidth())
	if m.issue.ID == "" {
		if m.issuePending {
			doc.title("Issue", m.route.EntityID, "loading")
			doc.body("Loading durable Issue details…")
		} else {
			return m.missingEntity("Issue", m.route.EntityID)
		}
		return doc.render()
	}
	doc.title("Issue", m.issue.Title, m.issue.Status)
	doc.action("a create a linked workspace · Esc back")
	doc.section("Facts")
	doc.field("ID", m.issue.ID)
	doc.field("Project", m.issue.ProjectID)
	doc.field("Revision", fmt.Sprint(m.issue.Revision))
	doc.field("Digest", m.issue.Digest)
	doc.field("Source", firstNonempty(m.issue.Source, "manual intake"))
	doc.field("Updated", formatTime(m.issue.UpdatedAt))
	if m.issue.StatusReason != "" {
		doc.field("Status reason", m.issue.StatusReason)
	}
	doc.section("Description")
	doc.body(m.issue.Body)
	doc.section("Linked workspaces")
	if len(m.issue.LinkedWorkspaces) == 0 {
		doc.body("No workspace has been created from this Issue.")
	} else {
		for _, link := range m.issue.LinkedWorkspaces {
			doc.bullet(fmt.Sprintf("%s · %s · revision %d", firstNonempty(link.Title, link.WorkspaceID), link.Status, link.IssueRevision))
		}
	}
	doc.section("Provenance")
	doc.provenance("Created", formatTime(m.issue.CreatedAt))
	if m.issue.RetrievedAt != nil {
		doc.provenance("Retrieved", formatTime(*m.issue.RetrievedAt))
	}
	if m.issue.LastCheckedAt != nil {
		doc.provenance("Last checked", formatTime(*m.issue.LastCheckedAt))
	}
	return doc.render()
}

func (m *Model) dispatcherContent() string {
	doc := newDetailDoc(m.palette, m.detailWidth())
	d := m.project.Dispatcher
	doc.title("Project Dispatcher", "", d.State)
	doc.action("a start or stop Dispatcher · r refresh")
	doc.section("Facts")
	doc.field("State", firstNonempty(d.State, "never_started"))
	doc.field("Role", firstNonempty(d.Role, "dispatcher"))
	doc.field("Profile", d.Profile)
	doc.field("Agent", d.AgentID)
	doc.field("Session", d.SessionID)
	doc.field("Current run", d.CurrentRunID)
	doc.field("Last run", d.LastRunID)
	doc.field("Runs", fmt.Sprint(d.RunCount))
	doc.field("Stop requested", fmt.Sprintf("%t", d.StopRequested))
	if d.Error != "" {
		doc.section("Error")
		doc.warning(d.Error)
	}
	doc.section("Scope")
	doc.body("The Dispatcher is project-scoped. It may intake and route Issues and create linked Workspaces, but it does not implement code or advance Workspace workflow state.")
	doc.section("Provenance")
	doc.provenance("Project", m.projectID)
	doc.provenance("Updated", formatTime(d.UpdatedAt))
	return doc.render()
}

func (m *Model) previewContent() string {
	if m.previewPending {
		return "Loading preview…"
	}
	doc := newDetailDoc(m.palette, m.detailWidth())
	doc.title("Preview", m.preview.Name, "")
	if m.loadError != "" {
		doc.warning("Preview unavailable")
		doc.body(m.loadError)
		doc.action("Esc returns to the resource.")
		return doc.render()
	}
	if m.preview.ResourceID == "" {
		doc.body("Preview unavailable.")
		return doc.render()
	}
	doc.field("Size", fmt.Sprintf("%d bytes", m.preview.Size))
	if m.preview.Digest != "" {
		doc.provenance("Digest", m.preview.Digest)
	}
	if m.preview.Binary {
		doc.warning("Binary file · text preview is unavailable.")
	}
	if m.preview.Truncated {
		doc.warning("Preview truncated at 256 KiB")
	}
	if m.preview.Warning != "" {
		doc.warning(m.preview.Warning)
	}
	if m.preview.Text != "" {
		doc.section("Content")
		doc.body(strings.ReplaceAll(m.preview.Text, "\t", "    "))
	}
	return doc.render()
}

// runtimeRow is one comparison row shared by the wide table and the compact
// stacked fallback so both presentations show the same topology.
type runtimeRow struct {
	Window, Pane, Kind, Owner, Run, State string
}

func (m *Model) runtimeRows() []runtimeRow {
	topology := m.runtime.Topology
	seen := make(map[string]bool, len(topology.Panes))
	rows := make([]runtimeRow, 0, len(topology.Panes))
	for _, window := range topology.Windows {
		panes := make([]core.Pane, 0, 2)
		for _, pane := range topology.Panes {
			if pane.WindowID == window.ID {
				panes = append(panes, pane)
			}
		}
		if len(panes) == 0 {
			rows = append(rows, runtimeRow{
				Window: sanitizeLine(window.ID),
				Pane:   "-",
				Kind:   sanitizeLine(firstNonempty(window.Kind, "window")),
				Owner:  "-",
				Run:    "-",
				State:  windowState(window),
			})
			continue
		}
		for _, pane := range panes {
			seen[pane.ID] = true
			rows = append(rows, m.runtimePaneRow(window, pane))
		}
	}
	for _, pane := range topology.Panes {
		if seen[pane.ID] {
			continue
		}
		rows = append(rows, m.runtimePaneRow(core.TmuxWindow{}, pane))
	}
	return rows
}

func (m *Model) runtimePaneRow(window core.TmuxWindow, pane core.Pane) runtimeRow {
	state := paneState(pane.Dead)
	if pane.RunID != "" {
		if run, ok := m.run(pane.RunID); ok && run.State != "" {
			state = run.State
		}
	}
	return runtimeRow{
		Window: sanitizeLine(firstNonempty(firstNonempty(window.ID, pane.WindowID), "-")),
		Pane:   sanitizeLine(firstNonempty(pane.ID, "-")),
		Kind:   sanitizeLine(firstNonempty(firstNonempty(pane.Kind, window.Kind), "-")),
		Owner:  sanitizeLine(firstNonempty(shortID(pane.SessionID), "-")),
		Run:    sanitizeLine(firstNonempty(shortID(pane.RunID), "-")),
		State:  sanitizeLine(firstNonempty(state, "-")),
	}
}

func windowState(window core.TmuxWindow) string {
	if window.Active {
		return "active"
	}
	return "idle"
}

func (m *Model) runtimeTableStyles() table.Styles {
	if m.palette.noColor {
		plain := lipgloss.NewStyle()
		return table.Styles{Header: plain, Cell: plain, Selected: plain}
	}
	return table.Styles{
		Header:   m.palette.labelStyle(),
		Cell:     m.palette.metaStyle(),
		Selected: m.palette.selectedStyle(1),
	}
}

// runtimeTable renders the wide topology comparison. Column widths stay within
// the supported wide breakpoint so the viewport never wraps a row.
func (m *Model) runtimeTable(rows []runtimeRow) []string {
	columns := []table.Column{
		{Title: "Window", Width: 10},
		{Title: "Pane", Width: 8},
		{Title: "Kind", Width: 10},
		{Title: "Owner", Width: 12},
		{Title: "Run", Width: 12},
		{Title: "State", Width: 12},
	}
	values := make([]table.Row, len(rows))
	for i, row := range rows {
		values[i] = table.Row{row.Window, row.Pane, row.Kind, row.Owner, row.Run, row.State}
	}
	view := table.New(
		table.WithColumns(columns),
		table.WithRows(values),
		table.WithStyles(m.runtimeTableStyles()),
		table.WithFocused(false),
		table.WithWidth(max(66, m.width-4)),
		table.WithHeight(len(rows)+1),
	)
	return strings.Split(view.View(), "\n")
}

// runtimeStacked is the compact, color-independent equivalent of the table.
func runtimeStacked(rows []runtimeRow) []string {
	if len(rows) == 0 {
		return []string{"No tmux windows were observed."}
	}
	lines := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("Window %s · pane %s · %s", row.Window, row.Pane, row.Kind))
		lines = append(lines, fmt.Sprintf("  owner %s · run %s · %s", row.Owner, row.Run, row.State))
	}
	return lines
}

func (m *Model) runtimeContent() string {
	doc := newDetailDoc(m.palette, m.detailWidth())
	doc.title("Runtime", "", "")
	doc.section("Facts")
	doc.field("Workspace tmux", m.runtime.State)
	doc.field("Supervisor", m.runtime.SupervisorState)
	if m.uiError != "" {
		doc.warning("Managed interface status error: " + m.uiError)
	} else {
		doc.field("Managed interface", m.uiStatus.State)
		doc.field("Desired", fmt.Sprintf("%t", m.uiStatus.Desired))
		if m.uiStatus.PaneID != "" || m.uiStatus.WindowID != "" {
			doc.field("UI pane / window", m.uiStatus.PaneID+" / "+m.uiStatus.WindowID)
		}
		if m.uiStatus.LastError != "" {
			doc.warning("Managed interface error: " + m.uiStatus.LastError)
		}
		if m.uiStatus.NextRetryAt != nil {
			doc.field("Managed interface retry", formatTime(*m.uiStatus.NextRetryAt))
		}
	}
	if m.runtimeError != "" {
		doc.warning("Runtime error: " + m.runtimeError)
	}
	if m.runtime.SupervisorError != "" {
		doc.warning("Supervisor error: " + m.runtime.SupervisorError)
	}
	doc.field("Observed", formatTime(m.runtime.ObservedAt))
	doc.field("Session", m.runtime.Topology.SessionName)
	rows := m.runtimeRows()
	if len(rows) == 0 {
		doc.section("Topology")
		doc.body("No tmux windows were observed.")
		return doc.render()
	}
	doc.section("Topology")
	if layoutFor(m.width, m.height) == layoutWide {
		doc.raw(m.runtimeTable(rows)...)
	} else {
		doc.raw(runtimeStacked(rows)...)
	}
	return doc.render()
}

func (m *Model) orchestratorContent() string {
	session, run := m.orchestrator()
	doc := newDetailDoc(m.palette, m.detailWidth())
	if session == nil {
		doc.title("Orchestrator", "", "")
		doc.body("No orchestrator session is recorded.")
		return doc.render()
	}
	doc.title("Orchestrator", session.AgentSnapshot.Name, session.LifecycleState)
	doc.section("Facts")
	doc.field("Workspace", m.workspaceID)
	doc.field("Directory", m.snapshot.Status.Directory)
	doc.field("Session", session.ID)
	doc.field("Current run", session.CurrentRunID)
	doc.field("Last run", session.LastRunID)
	if run != nil {
		doc.section("Run")
		doc.field("State", statusBadge(run.State))
		doc.field("Model", run.Route.Model)
		doc.field("Provider", run.Route.Provider)
		doc.field("Pane / window", run.PaneID+" / "+run.WindowID)
	}
	uiState := m.uiStatus.State
	if uiState == "" {
		uiState = "unknown"
	}
	doc.section("Managed UI")
	doc.field("State", statusBadge(uiState))
	doc.field("Desired", fmt.Sprintf("%t · generation %d", m.uiStatus.Desired, m.uiStatus.Generation))
	if m.uiStatus.PaneID != "" || m.uiStatus.WindowID != "" {
		doc.field("Pane / window", m.uiStatus.PaneID+" / "+m.uiStatus.WindowID)
	}
	if m.uiError != "" {
		doc.warning("Status error: " + m.uiError)
	} else if m.uiStatus.LastError != "" {
		doc.warning("Error: " + m.uiStatus.LastError)
	}
	doc.action("g jumps to a verified live orchestrator pane")
	return doc.render()
}

func (m *Model) quickDetails(kind, id string) []string {
	switch kind {
	case "task":
		task, ok := m.task(id)
		if ok {
			return []string{"Goal: " + task.Goal, "Attempt: " + fmt.Sprint(task.Attempt), "Worktree: " + task.WorktreeID, "Session: " + task.SessionID}
		}
	case "worktree":
		worktree, ok := findWorktree(m.snapshot.Status.Worktrees, id)
		if ok {
			return []string{"Path: " + worktree.Path, "Branch: " + worktree.Branch}
		}
	case "session":
		session, ok := findSession(m.snapshot.Status.Sessions, id)
		if ok {
			return []string{"Task: " + session.TaskID, "Current run: " + session.CurrentRunID, "Model: " + session.Route.Model}
		}
	case "run":
		run, ok := m.run(id)
		if ok {
			return []string{"Model: " + run.Route.Model, "Pane: " + run.PaneID, "Error: " + run.Error}
		}
	case "artifact":
		artifact, ok := findArtifact(m.snapshot.Status.Workspace.Artifacts, id)
		if ok {
			return []string{"Size: " + fmt.Sprint(artifact.Size), "Digest: " + artifact.Digest, "Run: " + artifact.RunID}
		}
	case "handoff":
		handoff, ok := findHandoff(m.snapshot.Handoffs, id)
		if ok {
			return []string{"Task: " + handoff.TaskID, "Outcome: " + handoff.Outcome}
		}
	case "check":
		check, ok := findCheck(m.snapshot.Checks, id)
		if ok {
			return []string{"Exit code: " + fmt.Sprint(check.ExitCode), "Command: " + strings.Join(check.Argv, " ")}
		}
	}
	return nil
}

func safeContent(lines []string) string {
	for i := range lines {
		lines[i] = sanitize(lines[i])
	}
	return strings.Join(lines, "\n")
}

func (m *Model) missingEntity(kind, id string) string {
	label := strings.Title(kind)
	doc := newDetailDoc(m.palette, m.detailWidth())
	doc.title(label, "", "")
	doc.warning(label + " no longer exists")
	doc.provenance("ID", id)
	doc.body("The selection has changed. Press Esc to return.")
	doc.action("Press r to refresh.")
	return doc.render()
}

func findWorktree(values []core.Worktree, id string) (core.Worktree, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return core.Worktree{}, false
}
func findSession(values []core.Session, id string) (core.Session, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return core.Session{}, false
}
func findAgent(values []core.Agent, id string) (core.Agent, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return core.Agent{}, false
}
func findService(values []core.BackgroundService, id string) (core.BackgroundService, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return core.BackgroundService{}, false
}
func findArtifact(values []core.Artifact, id string) (core.Artifact, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return core.Artifact{}, false
}
func findHandoff(values []core.Handoff, id string) (core.Handoff, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return core.Handoff{}, false
}
func findCheck(values []core.CheckReceipt, id string) (core.CheckReceipt, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return core.CheckReceipt{}, false
}
func findDecision(values []core.Decision, pending *core.Decision, id string) (core.Decision, bool) {
	if pending != nil && pending.ID == id {
		return *pending, true
	}
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return core.Decision{}, false
}
func findChangeRequest(values []core.ChangeRequest, id string) (core.ChangeRequest, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return core.ChangeRequest{}, false
}
func worktreeRelation(values []core.WorktreeRelation, id string) (core.WorktreeRelation, bool) {
	for _, value := range values {
		if value.WorktreeID == id {
			return value, true
		}
	}
	return core.WorktreeRelation{}, false
}
func sessionRelation(values []core.SessionRelation, id string) (core.SessionRelation, bool) {
	for _, value := range values {
		if value.SessionID == id {
			return value, true
		}
	}
	return core.SessionRelation{}, false
}
func taskRelation(values []core.TaskRelation, id string) (core.TaskRelation, bool) {
	for _, value := range values {
		if value.TaskID == id {
			return value, true
		}
	}
	return core.TaskRelation{}, false
}
func formatTime(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}
func formatTimePtr(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return formatTime(*value)
}
func formatIntPtr(value *int) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprint(*value)
}
func paneState(dead bool) string {
	if dead {
		return "dead"
	}
	return "live"
}
