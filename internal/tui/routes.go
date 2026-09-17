package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"workspace/internal/core"
)

func (m *Model) allItems() []collectionItem {
	s := m.snapshot
	items := []collectionItem{}
	switch m.route.Page {
	case "project":
		for _, workspace := range m.project.Workspaces {
			if m.route.StatusFilter == "active" && workspace.Status == "archived" || m.route.StatusFilter == "archived" && workspace.Status != "archived" {
				continue
			}
			title := workspace.Title
			if title == "" {
				title = workspace.ID
			}
			state := workspace.Status
			if workspace.Error != "" {
				state = "error"
			}
			items = append(items, collectionItem{ID: workspace.ID, Kind: "workspace", Title: title, Subtitle: fmt.Sprintf("%s · %s · active %d · problems %d", workspace.Phase, shortID(workspace.ID), workspace.ActiveRuns, workspace.Problems), State: state, At: workspace.CreatedAt})
		}
	case "more":
		items = []collectionItem{
			{ID: "agents", Kind: "page", Title: "Agents", Subtitle: "Persona definitions"},
			{ID: "services", Kind: "page", Title: "Services", Subtitle: "Background processes"},
			{ID: "decisions", Kind: "page", Title: "Decisions", Subtitle: "Pending and recorded decisions"},
			{ID: "change_requests", Kind: "page", Title: "Change requests", Subtitle: "Recorded integration proposals"},
			{ID: "runtime", Kind: "page", Title: "Runtime", Subtitle: "tmux and supervisor observation"},
			{ID: "attention", Kind: "page", Title: "Needs attention", Subtitle: "Durable issues and current failures"},
			{ID: "activity", Kind: "page", Title: "Recent recorded activity", Subtitle: "Runs, handoffs, artifacts and decisions"},
			{ID: "documents", Kind: "page", Title: "Documents", Subtitle: "Workspace, workflow and input snapshots"},
		}
	case "tasks":
		for _, task := range s.Status.Workspace.Tasks {
			if task.DeletedAt != nil {
				continue
			}
			if m.route.ParentID != "" && task.ID != m.route.ParentID {
				continue
			}
			items = append(items, m.taskItem(task))
		}
	case "worktrees":
		for _, worktree := range s.Status.Worktrees {
			if m.route.ParentID != "" {
				task, taskOK := m.task(m.route.ParentID)
				worktreeInTask, related := false, false
				if taskOK && task.WorktreeID == worktree.ID {
					worktreeInTask, related = true, true
				}
				if relation, ok := taskRelation(s.Relations.Tasks, m.route.ParentID); ok && containsID(relation.WorktreeIDs, worktree.ID) {
					worktreeInTask, related = true, true
				}
				if !related || !worktreeInTask {
					continue
				}
			}
			items = append(items, collectionItem{ID: worktree.ID, Kind: "worktree", Title: worktree.Name, Subtitle: worktree.Branch + " · " + worktree.Purpose, State: worktree.State})
		}
	case "results":
		tab := m.route.Tab
		if tab == "" {
			tab = "artifacts"
		}
		switch tab {
		case "artifacts":
			for _, artifact := range s.Status.Workspace.Artifacts {
				if m.route.ParentID != "" && artifact.TaskID != m.route.ParentID {
					continue
				}
				state, attempt := "recorded", ""
				if m.route.ParentID != "" {
					if task, ok := m.task(m.route.ParentID); ok {
						artifactAttempt, known := m.artifactAttempt(artifact)
						current := known && artifactAttempt == task.Attempt
						if current && m.route.StatusFilter == "history" || !current && m.route.StatusFilter != "history" {
							continue
						}
						if !known {
							state, attempt = "unknown", " · unknown lineage"
						} else if !current {
							state, attempt = "history", fmt.Sprintf(" · attempt %d", artifactAttempt)
						} else {
							attempt = fmt.Sprintf(" · attempt %d", artifactAttempt)
						}
					}
				}
				items = append(items, collectionItem{ID: artifact.ID, Kind: "artifact", Title: artifact.Name, Subtitle: artifact.Kind + attempt + " · " + artifact.Digest, State: state, At: artifact.CreatedAt})
			}
		case "handoffs":
			for _, handoff := range s.Handoffs {
				if m.route.ParentID != "" && handoff.TaskID != m.route.ParentID {
					continue
				}
				if task, ok := m.task(m.route.ParentID); ok {
					current := handoff.Attempt == task.Attempt && !handoff.Stale
					if current && m.route.StatusFilter == "history" || !current && m.route.StatusFilter != "history" {
						continue
					}
				}
				state := handoff.State
				if handoff.Stale {
					state = "stale"
				}
				items = append(items, collectionItem{ID: handoff.ID, Kind: "handoff", Title: handoff.Summary, Subtitle: fmt.Sprintf("%s · task %s · attempt %d", handoff.Outcome, shortID(handoff.TaskID), handoff.Attempt), State: state, At: handoff.CreatedAt})
			}
		case "checks":
			for _, check := range s.Checks {
				if m.route.ParentID != "" && check.TaskID != m.route.ParentID {
					continue
				}
				if task, ok := m.task(m.route.ParentID); ok {
					current := check.Attempt == task.Attempt
					if current && m.route.StatusFilter == "history" || !current && m.route.StatusFilter != "history" {
						continue
					}
				}
				items = append(items, collectionItem{ID: check.ID, Kind: "check", Title: strings.Join(check.Argv, " "), Subtitle: fmt.Sprintf("attempt %d · exit %d · %s", check.Attempt, check.ExitCode, check.State), State: check.State})
			}
		}
	case "sessions":
		for _, session := range s.Status.Sessions {
			if session.DeletedAt != nil {
				continue
			}
			if m.route.ParentID != "" {
				if task, ok := m.task(m.route.ParentID); ok {
					if session.TaskID != task.ID {
						continue
					}
					if m.route.StatusFilter == "history" && session.TaskAttempt == task.Attempt || m.route.StatusFilter != "history" && session.TaskAttempt != task.Attempt {
						continue
					}
				} else if strings.HasPrefix(m.route.ParentID, "agent_") {
					if session.AgentID != m.route.ParentID {
						continue
					}
				} else if session.TaskID != m.route.ParentID {
					continue
				}
			}
			if !m.isTask(m.route.ParentID) && (m.route.StatusFilter == "current" && session.LifecycleState == "closed" || m.route.StatusFilter == "history" && session.LifecycleState != "closed") {
				continue
			}
			items = append(items, collectionItem{ID: session.ID, Kind: "session", Title: session.AgentSnapshot.Name, Subtitle: fmt.Sprintf("%s · %s · attempt %d", session.LifecycleState, shortID(session.CurrentRunID), session.TaskAttempt), State: session.State, At: session.LastActiveAt})
		}
	case "runs":
		for _, run := range s.Status.Runs {
			if m.route.ParentID != "" && run.SessionID != m.route.ParentID {
				continue
			}
			items = append(items, collectionItem{ID: run.ID, Kind: "run", Title: run.ID, Subtitle: fmt.Sprintf("%s · %s / %s · %s", run.State, run.Route.Provider, run.Route.Model, run.ClientThreadID), State: run.State, At: run.CreatedAt})
		}
	case "agents":
		seen := make(map[string]bool)
		for _, agent := range s.Status.Agents {
			if seen[agent.ID] {
				continue
			}
			seen[agent.ID] = true
			items = append(items, collectionItem{ID: agent.ID, Kind: "agent", Title: agent.Name, Subtitle: agent.Role + " · " + agent.Profile, State: "defined"})
		}
	case "services":
		for _, service := range s.Services {
			items = append(items, collectionItem{ID: service.ID, Kind: "service", Title: service.Name, Subtitle: strings.Join(service.Argv, " "), State: service.State})
		}
	case "decisions":
		for _, decision := range s.Status.Workspace.Decisions {
			state := "answered"
			if decision.Answer == "" {
				state = "pending"
			}
			items = append(items, collectionItem{ID: decision.ID, Kind: "decision", Title: decision.Question, Subtitle: decision.Kind + " · " + state, State: state})
		}
		if decision := s.Status.Workspace.PendingDecision; decision != nil {
			found := false
			for _, item := range items {
				if item.ID == decision.ID {
					found = true
				}
			}
			if !found {
				items = append(items, collectionItem{ID: decision.ID, Kind: "decision", Title: decision.Question, Subtitle: decision.Kind + " · pending", State: "pending"})
			}
		}
	case "change_requests":
		for _, request := range s.Status.Workspace.ChangeRequests {
			items = append(items, collectionItem{ID: request.ID, Kind: "change_request", Title: request.Title, Subtitle: request.Branch + " · " + request.Target, State: request.State})
		}
	case "attention":
		items = m.attentionItems()
	case "activity":
		items = m.activityItems()
	case "documents":
		items = []collectionItem{
			{ID: "workspace", Kind: "document", Title: "WORKSPACE.md", Subtitle: "Current workspace document"},
			{ID: "workflow", Kind: "document", Title: "WORKFLOW.md", Subtitle: "Current workflow instructions"},
			{ID: "input", Kind: "document", Title: s.Status.Workspace.Input.Snapshot, Subtitle: "Saved input snapshot"},
		}
	}
	return items
}

func (m *Model) filteredItems() []collectionItem {
	query := strings.ToLower(strings.TrimSpace(m.route.Query))
	items := m.allItems()
	out := items[:0]
	for _, item := range items {
		if !m.statusFilterMatches(item) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.ID+" "+item.Title+" "+item.Subtitle), query) {
			continue
		}
		out = append(out, item)
	}
	m.sortItems(out)
	return out
}

func (m *Model) statusFilterOptions() []string {
	if m.route.Page == "project" {
		return []string{"", "active", "archived"}
	}
	if m.route.Page == "sessions" && !m.isTask(m.route.ParentID) {
		return []string{"", "current", "history"}
	}
	if m.isTask(m.route.ParentID) && (m.route.Page == "sessions" || m.route.Page == "results") {
		return []string{"", "history"}
	}
	states := make(map[string]struct{})
	for _, item := range m.allItems() {
		if item.State != "" {
			states[item.State] = struct{}{}
		}
	}
	if len(states) < 2 {
		return []string{""}
	}
	values := make([]string, 0, len(states)+1)
	values = append(values, "")
	for state := range states {
		values = append(values, state)
	}
	sort.Strings(values[1:])
	return values
}

func (m *Model) statusFilterMatches(item collectionItem) bool {
	filter := m.route.StatusFilter
	if filter == "" || m.route.Page == "project" || m.route.Page == "sessions" && !m.isTask(m.route.ParentID) || m.isTask(m.route.ParentID) && (m.route.Page == "sessions" || m.route.Page == "results") {
		return true
	}
	return strings.EqualFold(item.State, filter)
}

func (m *Model) selectedItem() (collectionItem, int, []collectionItem) {
	items := m.filteredItems()
	for i, item := range items {
		if item.ID == m.route.SelectedID {
			return item, i, items
		}
	}
	if len(items) > 0 {
		return items[0], 0, items
	}
	return collectionItem{}, -1, items
}

func shortID(id string) string {
	if len(id) > 15 {
		return id[:12] + "…"
	}
	return id
}

func (m *Model) orchestrator() (*core.Session, *core.Run) {
	status := m.snapshot.Status
	var selected *core.Session
	for i := range status.Sessions {
		candidate := &status.Sessions[i]
		if candidate.AgentID != status.Workspace.OrchestratorAgentID {
			continue
		}
		if selected == nil || candidate.LastActiveAt.After(selected.LastActiveAt) {
			selected = candidate
		}
	}
	if selected == nil {
		return nil, nil
	}
	for i := range status.Runs {
		if status.Runs[i].ID == selected.CurrentRunID || selected.CurrentRunID == "" && status.Runs[i].ID == selected.LastRunID {
			return selected, &status.Runs[i]
		}
	}
	return selected, nil
}

func (m *Model) attentionItems() []collectionItem {
	var items []collectionItem
	taskProblems := make(map[string]struct{})
	reportedTaskRuns := make(map[string]struct{})
	reportedRuns := make(map[string]struct{})
	if decision := m.snapshot.Status.Workspace.PendingDecision; decision != nil {
		items = append(items, collectionItem{ID: decision.ID, Kind: "decision", Title: "Pending decision: " + decision.Question, State: "pending"})
	}
	for _, task := range m.snapshot.Status.Workspace.Tasks {
		if task.State == "blocked" || task.State == "needs_changes" {
			taskProblems[task.ID] = struct{}{}
			items = append(items, collectionItem{ID: task.ID, Kind: "task", Title: task.Title, Subtitle: task.Reason, State: task.State})
		}
	}
	for _, handoff := range m.snapshot.Handoffs {
		if !handoff.Stale && handoff.State == "submitted" {
			if task, ok := m.task(handoff.TaskID); !ok || task.Attempt != handoff.Attempt {
				continue
			}
			items = append(items, collectionItem{ID: handoff.ID, Kind: "handoff", Title: "Review: " + handoff.Summary, Subtitle: shortID(handoff.TaskID), State: "submitted"})
		}
	}
	for _, session := range m.snapshot.Status.Sessions {
		runID := session.CurrentRunID
		if runID == "" {
			runID = session.LastRunID
		}
		if runID == "" {
			continue
		}
		if run, ok := m.run(runID); ok && (run.State == "failed" || run.State == "interrupted") {
			if session.TaskID != "" {
				if _, duplicate := taskProblems[session.TaskID]; duplicate {
					continue
				}
				if _, duplicate := reportedTaskRuns[session.TaskID]; duplicate {
					continue
				}
				reportedTaskRuns[session.TaskID] = struct{}{}
			}
			if _, duplicate := reportedRuns[run.ID]; duplicate {
				continue
			}
			reportedRuns[run.ID] = struct{}{}
			items = append(items, collectionItem{ID: run.ID, Kind: "run", Title: session.AgentSnapshot.Name + " run", Subtitle: run.Error, State: run.State, At: run.CreatedAt})
		}
	}
	if m.uiError != "" {
		items = append(items, collectionItem{ID: "managed_ui_error", Kind: "runtime", Title: "Managed interface status unavailable", Subtitle: m.uiError, State: "error"})
	} else if m.uiStatus.LastError != "" && m.uiStatus.Desired && m.uiStatus.State != "disabled" {
		items = append(items, collectionItem{ID: "managed_ui", Kind: "runtime", Title: "Managed interface " + m.uiStatus.State, Subtitle: m.uiStatus.LastError, State: "error"})
	}
	if m.runtimeError != "" {
		items = append(items, collectionItem{ID: "runtime_error", Kind: "runtime", Title: "Runtime unavailable", Subtitle: m.runtimeError, State: "error"})
	} else if m.runtime.SupervisorState == "unavailable" || m.runtime.SupervisorState == "conflict" {
		items = append(items, collectionItem{ID: "supervisor", Kind: "runtime", Title: "Supervisor " + m.runtime.SupervisorState, Subtitle: m.runtime.SupervisorError, State: "error"})
	}
	if m.loadError != "" {
		items = append(items, collectionItem{ID: "read_error", Kind: "runtime", Title: "Refresh failed", Subtitle: m.loadError, State: "error"})
	}
	sort.SliceStable(items, func(i, j int) bool {
		priority := func(item collectionItem) int {
			switch item.Kind {
			case "decision":
				return 0
			case "runtime":
				return 1
			case "task":
				return 2
			case "handoff":
				return 3
			default:
				return 4
			}
		}
		if priority(items[i]) != priority(items[j]) {
			return priority(items[i]) < priority(items[j])
		}
		return items[i].ID < items[j].ID
	})
	return items
}

func (m *Model) activityItems() []collectionItem {
	var recorded []activityItem
	for _, run := range m.snapshot.Status.Runs {
		recorded = append(recorded, activityItem{at: run.CreatedAt, item: collectionItem{ID: run.ID, Kind: "run", Title: run.State + " run " + shortID(run.ID), Subtitle: run.Route.Model, State: run.State}})
	}
	for _, handoff := range m.snapshot.Handoffs {
		recorded = append(recorded, activityItem{at: handoff.CreatedAt, item: collectionItem{ID: handoff.ID, Kind: "handoff", Title: handoff.Outcome + " handoff", Subtitle: handoff.Summary, State: handoff.State}})
	}
	for _, artifact := range m.snapshot.Status.Workspace.Artifacts {
		recorded = append(recorded, activityItem{at: artifact.CreatedAt, item: collectionItem{ID: artifact.ID, Kind: "artifact", Title: "Artifact " + artifact.Name, Subtitle: artifact.Kind, State: "recorded"}})
	}
	for _, decision := range m.snapshot.Status.Workspace.Decisions {
		if decision.AnsweredAt != nil {
			recorded = append(recorded, activityItem{at: *decision.AnsweredAt, item: collectionItem{ID: decision.ID, Kind: "decision", Title: "Answered decision", Subtitle: decision.Question, State: "completed"}})
		}
	}
	sort.SliceStable(recorded, func(i, j int) bool { return recorded[i].at.After(recorded[j].at) })
	if len(recorded) > 5 {
		recorded = recorded[:5]
	}
	items := make([]collectionItem, len(recorded))
	for i := range recorded {
		items[i] = recorded[i].item
	}
	return items
}

type activityItem struct {
	at   time.Time
	item collectionItem
}

func (m *Model) task(id string) (core.Task, bool) {
	for _, task := range m.snapshot.Status.Workspace.Tasks {
		if task.ID == id {
			return task, true
		}
	}
	return core.Task{}, false
}

func (m *Model) run(id string) (core.Run, bool) {
	for _, run := range m.snapshot.Status.Runs {
		if run.ID == id {
			return run, true
		}
	}
	return core.Run{}, false
}

func (m *Model) artifactAttempt(artifact core.Artifact) (int, bool) {
	if artifact.RunID != "" {
		if run, ok := m.run(artifact.RunID); ok {
			if session, found := findSession(m.snapshot.Status.Sessions, run.SessionID); found && session.TaskID == artifact.TaskID {
				return session.TaskAttempt, true
			}
		}
	}
	if artifact.SessionID != "" {
		if session, found := findSession(m.snapshot.Status.Sessions, artifact.SessionID); found && session.TaskID == artifact.TaskID {
			return session.TaskAttempt, true
		}
	}
	if artifact.SourceHandoff != "" {
		if handoff, found := findHandoff(m.snapshot.Handoffs, artifact.SourceHandoff); found && handoff.TaskID == artifact.TaskID {
			return handoff.Attempt, true
		}
	}
	return 0, false
}

func (m *Model) isTask(id string) bool {
	_, ok := m.task(id)
	return ok
}

func containsID(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
