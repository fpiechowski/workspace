package core

import (
	"context"
	"os"
	"path/filepath"
)

// DeleteWorkspace permanently removes either an empty workspace or an archived
// workspace whose worktrees have already been cleaned. The receipt is stored at
// project scope so a transport retry still succeeds after the directory is gone.
func (s *Service) DeleteWorkspace(ctx context.Context, selector, key string, expectedRevision int) error {
	if key == "" {
		return fail("operation_key_required", "workspace deletion requires an operation key")
	}
	_, err := projectEffect(ctx, s.Root, key, struct {
		Workspace string
		Revision  int
	}{selector, expectedRevision}, func() (bool, error) {
		unlock, err := lockProject(ctx, s.Root)
		if err != nil {
			return false, err
		}
		defer unlock()
		dir, err := s.resolve(selector)
		if err != nil {
			return false, err
		}
		d, err := loadDocument(dir)
		if err != nil {
			return false, err
		}
		if err := s.requireUser(d); err != nil {
			return false, err
		}
		if expectedRevision != 0 && d.State.Revision != expectedRevision {
			return false, fail("revision_conflict", "workspace changed while deletion was being confirmed")
		}
		if err := workspaceDeletionAllowed(d); err != nil {
			return false, err
		}
		cfg, err := s.Config()
		if err != nil {
			return false, err
		}
		storage, err := s.storageRoot(cfg)
		if err != nil {
			return false, err
		}
		cleanStorage, err := filepath.Abs(storage)
		if err != nil {
			return false, err
		}
		cleanDir, err := filepath.Abs(dir)
		if err != nil {
			return false, err
		}
		if filepath.Base(cleanDir) != d.State.ID || !contained(cleanStorage, cleanDir) || cleanDir == cleanStorage {
			return false, fail("unsafe_path", "workspace directory is outside the configured storage root")
		}
		if err := os.RemoveAll(cleanDir); err != nil {
			return false, err
		}
		if err := syncDirectory(filepath.Dir(cleanDir)); err != nil {
			return false, err
		}
		return true, nil
	})
	return err
}

func workspaceDeletionAllowed(d *Document) error {
	for _, session := range d.Registry.Sessions {
		if session.Active() {
			return fail("workspace_delete_refused", "stop active session %s before deleting the workspace", session.ID)
		}
	}
	for _, service := range d.Registry.Services {
		if service.Active() {
			return fail("workspace_delete_refused", "stop active service %s before deleting the workspace", service.ID)
		}
	}
	empty := len(d.Registry.Agents) == 1 && len(d.State.Tasks) == 0 && len(d.State.Artifacts) == 0 && len(d.State.Decisions) == 0 &&
		d.State.PendingDecision == nil && d.State.Integration == nil && len(d.State.ChangeRequests) == 0 &&
		len(d.Registry.Worktrees) == 0 && len(d.Registry.Sessions) == 0 && len(d.Registry.Runs) == 0 &&
		len(d.Registry.Services) == 0 && len(d.Registry.Checks) == 0 && len(d.Registry.Handoffs) == 0 &&
		len(d.Registry.Messages) == 0
	if empty {
		return nil
	}
	if d.State.Status != "archived" {
		return fail("workspace_delete_refused", "only an empty or archived workspace can be deleted")
	}
	for _, worktree := range d.Registry.Worktrees {
		if worktree.State != "removed" {
			return fail("workspace_delete_refused", "clean worktree %s before deleting the archived workspace", worktree.ID)
		}
	}
	return nil
}

// DeleteTask records an auditable tombstone. It refuses to erase dependencies,
// accepted results or active work. Associated inactive sessions without durable
// results are tombstoned with the task.
func (s *Service) DeleteTask(ctx context.Context, selector, id, key string, guard MutationGuard) (Task, error) {
	var out Task
	request := struct {
		Task  string
		Guard MutationGuard
	}{id, guard}
	err := mutate(s, ctx, selector, []string{key}, request, &out, s.requireUser, func(d *Document) error {
		if guard.ExpectedRevision != 0 && d.State.Revision != guard.ExpectedRevision {
			return fail("revision_conflict", "workspace changed while deletion was being confirmed")
		}
		task, err := findTask(d, id)
		if err != nil {
			return err
		}
		if guard.ExpectedAttempt != 0 && task.Attempt != guard.ExpectedAttempt {
			return fail("target_changed", "task %s moved from attempt %d to %d", task.ID, guard.ExpectedAttempt, task.Attempt)
		}
		if task.DeletedAt != nil {
			out = *task
			return nil
		}
		for _, candidate := range d.State.Tasks {
			if candidate.DeletedAt == nil && containsString(candidate.DependsOn, task.ID) {
				return fail("task_delete_refused", "task %s is required by %s", task.ID, candidate.ID)
			}
		}
		if task.AcceptedHandoff != "" || task.State == "accepted" {
			return fail("task_delete_refused", "accepted task results must remain in workspace history")
		}
		if d.State.Integration != nil && containsString(d.State.Integration.TaskIDs, task.ID) {
			return fail("task_delete_refused", "task is referenced by the integration record")
		}
		for _, request := range d.State.ChangeRequests {
			if request.TaskID == task.ID {
				return fail("task_delete_refused", "task is referenced by change request %s", request.ID)
			}
		}
		for _, handoff := range d.Registry.Handoffs {
			if handoff.TaskID == task.ID {
				return fail("task_delete_refused", "task has recorded handoff %s", handoff.ID)
			}
		}
		for _, artifact := range d.State.Artifacts {
			if artifact.TaskID == task.ID {
				return fail("task_delete_refused", "task has recorded artifact %s", artifact.ID)
			}
		}
		for _, check := range d.Registry.Checks {
			if check.TaskID == task.ID {
				return fail("task_delete_refused", "task has recorded check %s", check.ID)
			}
		}
		for _, session := range d.Registry.Sessions {
			if session.TaskID == task.ID && session.Active() {
				return fail("task_delete_refused", "stop session %s before deleting the task", session.ID)
			}
		}
		now := nowUTC()
		task.DeletedAt, task.State, task.Reason = &now, "deleted", "deleted by user"
		task.SessionID, task.RunID = "", ""
		for i := range d.Registry.Sessions {
			session := &d.Registry.Sessions[i]
			if session.TaskID != task.ID || session.DeletedAt != nil {
				continue
			}
			session.DeletedAt = &now
			if session.ClosedAt == nil {
				session.ClosedAt = &now
			}
			session.CloseReason = "task deleted by user"
		}
		out = *task
		return saveDocument(d)
	})
	return out, err
}

// DeleteSession records an auditable tombstone for an inactive non-orchestrator
// session. Durable results and message delivery references must be removed by
// deleting their owning task or retained as history instead.
func (s *Service) DeleteSession(ctx context.Context, selector, id, key string, guard MutationGuard) (Session, error) {
	var out Session
	request := struct {
		Session string
		Guard   MutationGuard
	}{id, guard}
	err := mutate(s, ctx, selector, []string{key}, request, &out, s.requireUser, func(d *Document) error {
		if guard.ExpectedRevision != 0 && d.State.Revision != guard.ExpectedRevision {
			return fail("revision_conflict", "workspace changed while deletion was being confirmed")
		}
		session, err := findSession(d, id)
		if err != nil {
			return err
		}
		if guard.ExpectedRunID != "" && session.LastRunID != guard.ExpectedRunID {
			return fail("target_changed", "session history changed while deletion was being confirmed")
		}
		if session.DeletedAt != nil {
			out = *session
			return nil
		}
		if session.AgentID == d.State.OrchestratorAgentID {
			return fail("session_delete_refused", "orchestrator sessions are workspace history and cannot be deleted individually")
		}
		if session.Active() {
			return fail("session_delete_refused", "stop the current run before deleting the session")
		}
		if err := sessionDeletionReferences(d, session.ID); err != nil {
			return err
		}
		now := nowUTC()
		session.DeletedAt = &now
		if session.ClosedAt == nil {
			session.ClosedAt = &now
		}
		session.CloseReason = "deleted by user"
		if task, taskErr := findTask(d, session.TaskID); taskErr == nil && task.DeletedAt == nil && task.Attempt == session.TaskAttempt && task.SessionID == session.ID {
			if task.State == "accepted" {
				return fail("session_delete_refused", "session belongs to an accepted task")
			}
			task.SessionID, task.RunID, task.WorktreeID = "", "", ""
			task.State, task.Reason = "pending", "session deleted by user"
		}
		out = *session
		return saveDocument(d)
	})
	return out, err
}

func sessionDeletionReferences(d *Document, id string) error {
	for _, handoff := range d.Registry.Handoffs {
		if handoff.FromSession == id {
			return fail("session_delete_refused", "session has recorded handoff %s", handoff.ID)
		}
	}
	for _, artifact := range d.State.Artifacts {
		if artifact.SessionID == id {
			return fail("session_delete_refused", "session has recorded artifact %s", artifact.ID)
		}
	}
	for _, check := range d.Registry.Checks {
		if check.SessionID == id {
			return fail("session_delete_refused", "session has recorded check %s", check.ID)
		}
	}
	for _, message := range d.Registry.Messages {
		if message.FromSession == id || message.DeliveredSessionID == id || message.NotifiedSessionID == id {
			return fail("session_delete_refused", "session is referenced by message %s", message.ID)
		}
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
