package tui

import (
	"fmt"
	"strings"
	"time"

	"workspace/internal/core"
)

func (m *Model) dashboardContent() string {
	if m.snapshot.ObservedAt.IsZero() {
		if m.loadError != "" {
			return strings.Join([]string{"Workspace snapshot unavailable", m.loadError, "Press r to retry."}, "\n")
		}
		return "Loading workspace…"
	}
	panels := m.dashboardPanels()
	lines := []string{m.dashboardSectionTitle(0, panels[0].Title)}
	lines = append(lines, panels[0].Lines...)
	lines = append(lines, "", m.dashboardSectionTitle(1, panels[1].Title))
	lines = append(lines, panels[1].Lines...)
	lines = append(lines, "", m.dashboardSectionTitle(2, panels[2].Title))
	lines = append(lines, panels[2].Lines...)
	lines = append(lines, "", m.dashboardSectionTitle(3, panels[3].Title))
	lines = append(lines, panels[3].Lines...)
	lines = append(lines, "", "Choose 1 Overview · 2 Tasks · 3 Worktrees · 4 Results · 5 More")
	if len(m.attentionItems()) > 4 {
		lines = append(lines, "v opens all attention items")
	}
	return safeContent(lines)
}

func (m *Model) dashboardSectionTitle(index int, title string) string {
	if m.focusedPanel == index {
		return "> " + title
	}
	return "  " + title
}

func (m *Model) focusDashboardPanel(delta int) {
	const panelCount = 4
	m.focusedPanel = (m.focusedPanel + delta + panelCount) % panelCount
	m.route.Query, m.route.SelectedID, m.route.StatusFilter = "", "", ""
	m.validateSelection()
	m.rebuildViewport()
	if layoutFor(m.width, m.height) != layoutCompact {
		return
	}
	panels := m.dashboardPanels()
	target := m.dashboardSectionTitle(m.focusedPanel, panels[m.focusedPanel].Title)
	for line, value := range strings.Split(m.dashboardContent(), "\n") {
		if strings.HasPrefix(value, target) {
			m.viewport.SetYOffset(line)
			return
		}
	}
}

func (m *Model) detailContent() string {
	if m.initialError != "" || m.route.Page == "error" {
		return safeContent([]string{"Could not load this workspace", m.initialError, "Press w to choose a workspace or q to quit."})
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
	id := m.route.EntityID
	lines := []string{}
	switch m.route.Page {
	case "task":
		task, ok := m.task(id)
		if !ok {
			return missingEntity("task", id)
		}
		lines = append(lines, "Task", task.Title, statusBadge(task.State), m.taskItem(task).Subtitle, "t open / resume terminal · 1 sessions · 3 results", "ID: "+task.ID, fmt.Sprintf("Attempt %d · profile %s · role %s", task.Attempt, task.Profile, task.Role), "", "Goal", task.Goal)
		if task.Reason != "" {
			lines = append(lines, "", "Reason", task.Reason)
		}
		lines = append(lines, "", "Acceptance criteria")
		lines = append(lines, bulletLines(task.AcceptanceCriteria)...)
		lines = append(lines, "", "Depends on")
		lines = append(lines, bulletLines(task.DependsOn)...)
		lines = append(lines, "", "Required artifacts")
		lines = append(lines, bulletLines(task.RequiredArtifacts)...)
		lines = append(lines, "", "Related sessions and results", "1 Sessions · 2 Worktrees · 3 Results · f toggles attempt history")
		if task.AcceptedHandoff != "" {
			lines = append(lines, "Accepted handoff: "+task.AcceptedHandoff)
		}
	case "worktree":
		worktree, ok := findWorktree(m.snapshot.Status.Worktrees, id)
		if !ok {
			return missingEntity("worktree", id)
		}
		lines = append(lines, "Worktree", worktree.Name, statusBadge(worktree.State), "ID: "+worktree.ID, "Path: "+worktree.Path, "Branch: "+worktree.Branch, "Purpose: "+worktree.Purpose)
		if observation, exists := m.worktreeInspection[id]; exists {
			lines = append(lines, "", "Git observation · "+formatTime(observation.CheckedAt))
			if observation.Error != "" {
				lines = append(lines, "Error: "+observation.Error)
			} else {
				dirty := "clean"
				if observation.Dirty {
					dirty = "dirty"
				}
				lines = append(lines, "HEAD: "+observation.Head+" · "+dirty)
			}
		} else if m.worktreePending {
			lines = append(lines, "", "Inspecting HEAD and working tree…")
		}
		if relation, ok := worktreeRelation(m.snapshot.Relations.Worktrees, id); ok {
			lines = append(lines, "", "Related task IDs", "  "+strings.Join(relation.TaskIDs, ", "), "Session IDs", "  "+strings.Join(relation.SessionIDs, ", "), "Service IDs", "  "+strings.Join(relation.ServiceIDs, ", "))
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
		lines = append(lines, "", "Active writer sessions", "  "+firstNonempty(strings.Join(writers, ", "), "none"), "Active read-only sessions", "  "+firstNonempty(strings.Join(readers, ", "), "none"), "Active services", "  "+firstNonempty(strings.Join(services, ", "), "none"))
	case "session":
		session, ok := findSession(m.snapshot.Status.Sessions, id)
		if !ok {
			return missingEntity("session", id)
		}
		lines = append(lines, "Session", session.AgentSnapshot.Name, statusBadge(session.LifecycleState), "ID: "+session.ID, "Role: "+session.AgentSnapshot.Role, "Profile: "+session.Profile, "Client: "+session.ClientSnapshot.Adapter+" · "+session.Route.Model, "Task: "+session.TaskID+" · attempt "+fmt.Sprint(session.TaskAttempt), "Worktree: "+session.WorktreeID, fmt.Sprintf("Read only: %t", session.ReadOnly), "Current run: "+session.CurrentRunID, "Last run: "+session.LastRunID, "Native thread: "+session.ClientThreadID, "", "Enter opens Run history · g jumps to the current Run")
	case "run":
		run, ok := m.run(id)
		if !ok {
			return missingEntity("run", id)
		}
		current := false
		for _, session := range m.snapshot.Status.Sessions {
			if session.ID == run.SessionID {
				current = session.CurrentRunID == run.ID
				break
			}
		}
		lines = append(lines, "Run", run.ID, statusBadge(run.State), "Session: "+run.SessionID, "Generation: "+fmt.Sprint(run.Generation), "Current: "+fmt.Sprint(current), "Provider / model: "+run.Route.Provider+" / "+run.Route.Model, "Client: "+run.Route.Client, "Pane / window: "+run.PaneID+" / "+run.WindowID, "Created: "+formatTime(run.CreatedAt), "Finished: "+formatTimePtr(run.FinishedAt), "Exit code: "+formatIntPtr(run.ExitCode), "Client state: "+run.ClientState)
		if run.Error != "" {
			lines = append(lines, "Error", run.Error)
		}
		if !current {
			lines = append(lines, "", "This run is historical. Jump is unavailable; open its current Session instead.")
		}
	case "agent":
		agent, ok := findAgent(m.snapshot.Status.Agents, id)
		if !ok {
			return missingEntity("agent", id)
		}
		lines = append(lines, "Agent", agent.Name, "ID: "+agent.ID, "Role: "+agent.Role, "Profile: "+agent.Profile, "", "Instructions", agent.Instructions, "", "Sessions")
		for _, session := range m.snapshot.Status.Sessions {
			if session.AgentID == id {
				lines = append(lines, "  "+session.ID+" · "+session.LifecycleState+" · "+session.CurrentRunID)
			}
		}
	case "service":
		service, ok := findService(m.snapshot.Services, id)
		if !ok {
			return missingEntity("service", id)
		}
		lines = append(lines, "Service", service.Name, statusBadge(service.State), "ID: "+service.ID, "Worktree: "+service.WorktreeID, "CWD: "+service.CWD, "Pane: "+service.PaneID, "Exit code: "+formatIntPtr(service.ExitCode), "", "Command", strings.Join(service.Argv, " "))
	case "artifact":
		artifact, ok := findArtifact(m.snapshot.Status.Workspace.Artifacts, id)
		if !ok {
			return missingEntity("artifact", id)
		}
		lines = append(lines, "Artifact", artifact.Name, "ID: "+artifact.ID, "Kind: "+artifact.Kind, "Size: "+fmt.Sprint(artifact.Size), "Digest: "+artifact.Digest, "Source handoff: "+artifact.SourceHandoff, "Task / session / run: "+artifact.TaskID+" / "+artifact.SessionID+" / "+artifact.RunID, "Created: "+formatTime(artifact.CreatedAt), "Enter to preview")
	case "handoff":
		handoff, ok := findHandoff(m.snapshot.Handoffs, id)
		if !ok {
			return missingEntity("handoff", id)
		}
		state := handoff.State
		if handoff.Stale {
			state = "stale · " + state
		}
		lines = append(lines, "Handoff", handoff.Outcome, statusBadge(state), "ID: "+handoff.ID, "Task / attempt: "+handoff.TaskID+" / "+fmt.Sprint(handoff.Attempt), "From / to: "+handoff.FromAgent+" / "+handoff.ToAgent, "", "Summary", handoff.Summary, "", "Risks")
		lines = append(lines, bulletLines(handoff.Risks)...)
		if handoff.Feedback != "" {
			lines = append(lines, "", "Feedback", handoff.Feedback)
		}
		lines = append(lines, "", "Artifacts", strings.Join(handoff.ArtifactIDs, ", "))
	case "check":
		check, ok := findCheck(m.snapshot.Checks, id)
		if !ok {
			return missingEntity("check", id)
		}
		lines = append(lines, "Check", check.ID, statusBadge(check.State), fmt.Sprintf("Exit code: %d", check.ExitCode), "Task / attempt: "+check.TaskID+" / "+fmt.Sprint(check.Attempt), "Head: "+check.Head, "End head: "+check.EndHead, "Digest: "+check.Digest, "Command: "+strings.Join(check.Argv, " "), "Enter to preview captured output")
	case "decision":
		decision, ok := findDecision(m.snapshot.Status.Workspace.Decisions, m.snapshot.Status.Workspace.PendingDecision, id)
		if !ok {
			return missingEntity("decision", id)
		}
		lines = append(lines, "Decision", decision.Question, statusBadge(firstNonempty(decision.Answer, "pending")), "ID: "+decision.ID, "Kind: "+decision.Kind, "Options")
		lines = append(lines, bulletLines(decision.Options)...)
		if decision.Answer != "" {
			lines = append(lines, "", "Answer", decision.Answer, "Reason", decision.Reason)
		}
	case "change_request":
		request, ok := findChangeRequest(m.snapshot.Status.Workspace.ChangeRequests, id)
		if !ok {
			return missingEntity("change request", id)
		}
		lines = append(lines, "Change request", request.Title, statusBadge(request.State), "ID: "+request.ID, "Branch: "+request.Branch, "Target: "+request.Target, "Worktree: "+request.WorktreeID, "Head: "+request.HeadCommit, "", "Body", request.Body, "", "URL", request.URL)
	case "runtime":
		return m.runtimeContent()
	default:
		return safeContent([]string{m.collectionTitle(), "ID: " + id})
	}
	return safeContent(lines)
}

func (m *Model) previewContent() string {
	if m.previewPending {
		return "Loading preview…"
	}
	if m.loadError != "" {
		return safeContent([]string{"Preview unavailable", m.loadError, "Esc returns to the resource."})
	}
	if m.preview.ResourceID == "" {
		return "Preview unavailable."
	}
	lines := []string{m.preview.Name, fmt.Sprintf("%d bytes", m.preview.Size)}
	if m.preview.Digest != "" {
		lines = append(lines, "Digest: "+m.preview.Digest)
	}
	if m.preview.Binary {
		lines = append(lines, "Binary file · text preview is unavailable.")
	}
	if m.preview.Truncated {
		lines = append(lines, "[!] Preview truncated at 256 KiB")
	}
	if m.preview.Warning != "" {
		lines = append(lines, "[!] "+m.preview.Warning)
	}
	if m.preview.Text != "" {
		lines = append(lines, "", strings.ReplaceAll(m.preview.Text, "\t", "    "))
	}
	return safeContent(lines)
}

func (m *Model) runtimeContent() string {
	lines := []string{"Runtime", "Workspace tmux: " + statusBadge(m.runtime.State), "Supervisor: " + statusBadge(m.runtime.SupervisorState)}
	if m.uiError != "" {
		lines = append(lines, "Managed interface status error: "+m.uiError)
	} else {
		lines = append(lines, "Managed interface: "+statusBadge(m.uiStatus.State)+fmt.Sprintf(" · desired %t", m.uiStatus.Desired))
		if m.uiStatus.PaneID != "" || m.uiStatus.WindowID != "" {
			lines = append(lines, "UI pane / window: "+m.uiStatus.PaneID+" / "+m.uiStatus.WindowID)
		}
		if m.uiStatus.LastError != "" {
			lines = append(lines, "Managed interface error: "+m.uiStatus.LastError)
		}
		if m.uiStatus.NextRetryAt != nil {
			lines = append(lines, "Managed interface retry: "+formatTime(*m.uiStatus.NextRetryAt))
		}
	}
	if m.runtimeError != "" {
		lines = append(lines, "Runtime error: "+m.runtimeError)
	}
	if m.runtime.SupervisorError != "" {
		lines = append(lines, "Supervisor error: "+m.runtime.SupervisorError)
	}
	lines = append(lines, "Observed: "+formatTime(m.runtime.ObservedAt), "Session: "+m.runtime.Topology.SessionName)
	for _, window := range m.runtime.Topology.Windows {
		lines = append(lines, "", "Window "+window.ID+" · "+window.Name+" · "+window.Kind+" · "+fmt.Sprintf("%dx%d", window.Width, window.Height))
		for _, pane := range m.runtime.Topology.Panes {
			if pane.WindowID == window.ID {
				lines = append(lines, "  "+pane.ID+" · "+pane.Kind+" · session "+shortID(pane.SessionID)+" · run "+shortID(pane.RunID)+" · "+paneState(pane.Dead))
			}
		}
	}
	if len(m.runtime.Topology.Windows) == 0 {
		lines = append(lines, "No tmux windows were observed.")
	}
	return safeContent(lines)
}

func (m *Model) orchestratorContent() string {
	session, run := m.orchestrator()
	if session == nil {
		return "Orchestrator\nNo orchestrator session is recorded."
	}
	lines := []string{"Orchestrator", session.AgentSnapshot.Name, statusBadge(session.LifecycleState), "Workspace: " + m.workspaceID, "Directory: " + m.snapshot.Status.Directory, "Session: " + session.ID, "Current run: " + session.CurrentRunID, "Last run: " + session.LastRunID}
	if run != nil {
		lines = append(lines, "", "Run", statusBadge(run.State), "Model: "+run.Route.Model, "Provider: "+run.Route.Provider, "Pane / window: "+run.PaneID+" / "+run.WindowID)
	}
	uiState := m.uiStatus.State
	if uiState == "" {
		uiState = "unknown"
	}
	lines = append(lines, "", "Managed UI", "State: "+statusBadge(uiState), fmt.Sprintf("Desired: %t · generation %d", m.uiStatus.Desired, m.uiStatus.Generation))
	if m.uiStatus.PaneID != "" || m.uiStatus.WindowID != "" {
		lines = append(lines, "Pane / window: "+m.uiStatus.PaneID+" / "+m.uiStatus.WindowID)
	}
	if m.uiError != "" {
		lines = append(lines, "Status error: "+m.uiError)
	} else if m.uiStatus.LastError != "" {
		lines = append(lines, "Error: "+m.uiStatus.LastError)
	}
	lines = append(lines, "g jumps to a verified live orchestrator pane")
	return safeContent(lines)
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

func bulletLines(values []string) []string {
	if len(values) == 0 {
		return []string{"  —"}
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = "  · " + value
	}
	return out
}

func missingEntity(kind, id string) string {
	return safeContent([]string{strings.Title(kind) + " no longer exists", "ID: " + id, "The selection has changed. Press Esc to return.", "Press r to refresh."})
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
