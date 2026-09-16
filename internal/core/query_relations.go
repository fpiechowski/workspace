package core

// The relation DTOs contain identifiers only. Interface code can project the
// aggregate without retaining pointers into a Document or inventing lineage.
type TaskRelation struct {
	TaskID               string   `json:"task_id"`
	CurrentSessionIDs    []string `json:"current_session_ids,omitempty"`
	HistoricalSessionIDs []string `json:"historical_session_ids,omitempty"`
	RunIDs               []string `json:"run_ids,omitempty"`
	WorktreeIDs          []string `json:"worktree_ids,omitempty"`
	HandoffIDs           []string `json:"handoff_ids,omitempty"`
	ArtifactIDs          []string `json:"artifact_ids,omitempty"`
	CheckIDs             []string `json:"check_ids,omitempty"`
}

type WorktreeRelation struct {
	WorktreeID string   `json:"worktree_id"`
	TaskIDs    []string `json:"task_ids,omitempty"`
	SessionIDs []string `json:"session_ids,omitempty"`
	RunIDs     []string `json:"run_ids,omitempty"`
	ServiceIDs []string `json:"service_ids,omitempty"`
}

type SessionRelation struct {
	SessionID string   `json:"session_id"`
	RunIDs    []string `json:"run_ids,omitempty"`
}

type WorkspaceRelations struct {
	Tasks     []TaskRelation     `json:"tasks"`
	Worktrees []WorktreeRelation `json:"worktrees"`
	Sessions  []SessionRelation  `json:"sessions"`
}

type WorkspaceMetrics struct {
	TaskTotal        int `json:"task_total"`
	TaskAccepted     int `json:"task_accepted"`
	TaskRunning      int `json:"task_running"`
	TaskBlocked      int `json:"task_blocked"`
	TaskNeedsChanges int `json:"task_needs_changes"`
	ActiveRuns       int `json:"active_runs"`
	ReadyWorktrees   int `json:"ready_worktrees"`
	ActiveServices   int `json:"active_services"`
	ProblemCount     int `json:"problem_count"`
}

func workspaceMetrics(d *Document) WorkspaceMetrics {
	var m WorkspaceMetrics
	for _, task := range d.State.Tasks {
		if task.DeletedAt != nil {
			continue
		}
		m.TaskTotal++
		switch task.State {
		case "accepted":
			m.TaskAccepted++
		case "running":
			m.TaskRunning++
		case "blocked":
			m.TaskBlocked++
		case "needs_changes":
			m.TaskNeedsChanges++
		}
	}
	for _, session := range d.Registry.Sessions {
		if !session.Active() {
			continue
		}
		run, err := findRun(d, session.CurrentRunID)
		if err == nil && run.Active() {
			m.ActiveRuns++
		}
	}
	for _, worktree := range d.Registry.Worktrees {
		if worktree.State == "ready" {
			m.ReadyWorktrees++
		}
	}
	for _, service := range d.Registry.Services {
		if service.Active() {
			m.ActiveServices++
		}
	}
	m.ProblemCount = len(attentionItems(d))
	return m
}

// attentionItems intentionally counts distinct durable issues rather than
// double-counting a task and the failed run attached to its current attempt.
func attentionItems(d *Document) []string {
	items := make(map[string]struct{})
	if d.State.PendingDecision != nil {
		items["decision:"+d.State.PendingDecision.ID] = struct{}{}
	}
	for _, task := range d.State.Tasks {
		if task.State == "blocked" || task.State == "needs_changes" {
			items["task:"+task.ID] = struct{}{}
		}
	}
	for _, handoff := range d.Registry.Handoffs {
		if !handoff.Stale && handoff.State == "submitted" {
			if task, err := findTask(d, handoff.TaskID); err != nil || task.Attempt != handoff.Attempt {
				continue
			}
			items["handoff:"+handoff.ID] = struct{}{}
		}
	}
	for _, run := range d.Registry.Runs {
		if run.State != "failed" && run.State != "interrupted" {
			continue
		}
		if run.SessionID == "" {
			items["run:"+run.ID] = struct{}{}
			continue
		}
		session, err := findSession(d, run.SessionID)
		if err != nil {
			items["run:"+run.ID] = struct{}{}
			continue
		}
		if session.DeletedAt != nil {
			continue
		}
		if session.CurrentRunID != run.ID && session.LastRunID != run.ID {
			continue
		}
		if session.TaskID != "" {
			if task, err := findTask(d, session.TaskID); err == nil && (task.State == "blocked" || task.State == "needs_changes") {
				continue
			}
		}
		items["run:"+run.ID] = struct{}{}
	}
	out := make([]string, 0, len(items))
	for id := range items {
		out = append(out, id)
	}
	return out
}

func buildWorkspaceRelations(d *Document) WorkspaceRelations {
	out := WorkspaceRelations{
		Tasks:     make([]TaskRelation, 0, len(d.State.Tasks)),
		Worktrees: make([]WorktreeRelation, 0, len(d.Registry.Worktrees)),
		Sessions:  make([]SessionRelation, 0, len(d.Registry.Sessions)),
	}
	taskIndex := make(map[string]int, len(d.State.Tasks))
	for _, task := range d.State.Tasks {
		taskIndex[task.ID] = len(out.Tasks)
		out.Tasks = append(out.Tasks, TaskRelation{TaskID: task.ID})
	}
	worktreeIndex := make(map[string]int, len(d.Registry.Worktrees))
	for _, worktree := range d.Registry.Worktrees {
		worktreeIndex[worktree.ID] = len(out.Worktrees)
		out.Worktrees = append(out.Worktrees, WorktreeRelation{WorktreeID: worktree.ID})
	}
	for _, task := range d.State.Tasks {
		if i, ok := worktreeIndex[task.WorktreeID]; ok && task.WorktreeID != "" {
			out.Worktrees[i].TaskIDs = appendUnique(out.Worktrees[i].TaskIDs, task.ID)
		}
		if i, ok := taskIndex[task.ID]; ok && task.AcceptedHandoff != "" {
			out.Tasks[i].HandoffIDs = appendUnique(out.Tasks[i].HandoffIDs, task.AcceptedHandoff)
		}
	}
	sessionIndex := make(map[string]int, len(d.Registry.Sessions))
	for _, session := range d.Registry.Sessions {
		sessionIndex[session.ID] = len(out.Sessions)
		out.Sessions = append(out.Sessions, SessionRelation{SessionID: session.ID})
		if session.TaskID != "" {
			if i, ok := taskIndex[session.TaskID]; ok {
				task := d.State.Tasks[i]
				if task.Attempt == session.TaskAttempt && task.SessionID == session.ID {
					out.Tasks[i].CurrentSessionIDs = append(out.Tasks[i].CurrentSessionIDs, session.ID)
				} else {
					out.Tasks[i].HistoricalSessionIDs = append(out.Tasks[i].HistoricalSessionIDs, session.ID)
				}
				if session.WorktreeID != "" {
					out.Tasks[i].WorktreeIDs = appendUnique(out.Tasks[i].WorktreeIDs, session.WorktreeID)
				}
			}
		}
		if i, ok := worktreeIndex[session.WorktreeID]; ok && session.WorktreeID != "" {
			out.Worktrees[i].SessionIDs = appendUnique(out.Worktrees[i].SessionIDs, session.ID)
		}
	}
	runIndex := make(map[string]Run, len(d.Registry.Runs))
	for _, run := range d.Registry.Runs {
		runIndex[run.ID] = run
		if i, ok := sessionIndex[run.SessionID]; ok {
			out.Sessions[i].RunIDs = append(out.Sessions[i].RunIDs, run.ID)
		}
		if session, err := findSession(d, run.SessionID); err == nil {
			if i, ok := worktreeIndex[session.WorktreeID]; ok && session.WorktreeID != "" {
				out.Worktrees[i].RunIDs = appendUnique(out.Worktrees[i].RunIDs, run.ID)
			}
			if session.TaskID != "" {
				if i, ok := taskIndex[session.TaskID]; ok {
					out.Tasks[i].RunIDs = appendUnique(out.Tasks[i].RunIDs, run.ID)
				}
			}
		}
	}
	for _, handoff := range d.Registry.Handoffs {
		if i, ok := taskIndex[handoff.TaskID]; ok {
			out.Tasks[i].HandoffIDs = appendUnique(out.Tasks[i].HandoffIDs, handoff.ID)
		}
	}
	for _, artifact := range d.State.Artifacts {
		if i, ok := taskIndex[artifact.TaskID]; ok {
			// An artifact is task-related only when at least one stored lineage ID
			// joins to the same aggregate. TaskID alone may describe stale history.
			_, runExists := runIndex[artifact.RunID]
			_, sessionExists := sessionIndex[artifact.SessionID]
			lineageKnown := artifact.RunID != "" && runExists || artifact.SessionID != "" && sessionExists
			if lineageKnown {
				out.Tasks[i].ArtifactIDs = appendUnique(out.Tasks[i].ArtifactIDs, artifact.ID)
			}
		}
	}
	for _, check := range d.Registry.Checks {
		if i, ok := taskIndex[check.TaskID]; ok {
			out.Tasks[i].CheckIDs = appendUnique(out.Tasks[i].CheckIDs, check.ID)
		}
	}
	for _, service := range d.Registry.Services {
		if i, ok := worktreeIndex[service.WorktreeID]; ok {
			out.Worktrees[i].ServiceIDs = appendUnique(out.Worktrees[i].ServiceIDs, service.ID)
		}
	}
	return out
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
