package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DeleteWorkspace permanently discards a workspace regardless of workflow state.
// It stops the workspace runtime and removes its worktrees and local workspace
// branches before deleting durable state. The receipt is stored at project scope
// so a transport retry still succeeds after the directory is gone.
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
		if s.Runtime == nil {
			return false, fail("runtime_unavailable", "cannot stop workspace runtime before deletion")
		}
		if err := s.Runtime.StopWorkspace(ctx, d.State.ID); err != nil {
			return false, err
		}
		for _, worktree := range d.Registry.Worktrees {
			if err := s.discardWorkspaceWorktree(ctx, d, worktree); err != nil {
				return false, err
			}
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

func (s *Service) discardWorkspaceWorktree(ctx context.Context, d *Document, worktree Worktree) error {
	root := filepath.Join(d.Dir, "worktrees")
	cleanPath, err := filepath.Abs(worktree.Path)
	if err != nil {
		return err
	}
	if !contained(root, cleanPath) || cleanPath == root {
		return fail("unsafe_path", "worktree %s is outside the workspace", worktree.ID)
	}
	branchPrefix := "workspace/" + d.State.ID + "/"
	if !strings.HasPrefix(worktree.Branch, branchPrefix) {
		return fail("unsafe_path", "worktree %s uses an unmanaged branch", worktree.ID)
	}
	registered, err := s.registeredWorktree(ctx, cleanPath)
	if err != nil {
		return err
	}
	if registered {
		candidate := worktree
		candidate.Path = cleanPath
		if verifyWorktree(ctx, &candidate) == nil {
			if _, err := git(ctx, s.Root, "worktree", "remove", "--force", "--force", "--", cleanPath); err != nil {
				return err
			}
		} else {
			if err := os.RemoveAll(cleanPath); err != nil {
				return err
			}
			if err := s.removeBrokenWorktreeRegistration(ctx, cleanPath); err != nil {
				return err
			}
		}
	} else if err := os.RemoveAll(cleanPath); err != nil {
		return err
	}
	exists, err := localBranchExists(ctx, s.Root, worktree.Branch)
	if err != nil {
		return err
	}
	if exists {
		if _, err := git(ctx, s.Root, "branch", "-D", "--", worktree.Branch); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) registeredWorktree(ctx context.Context, target string) (bool, error) {
	listing, err := git(ctx, s.Root, "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(listing, "\n") {
		if strings.HasPrefix(line, "worktree ") && filepath.Clean(strings.TrimPrefix(line, "worktree ")) == filepath.Clean(target) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) removeBrokenWorktreeRegistration(ctx context.Context, target string) error {
	common, err := git(ctx, s.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	metadataRoot := filepath.Join(common, "worktrees")
	entries, err := os.ReadDir(metadataRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	targetGitDir := filepath.Clean(filepath.Join(target, ".git"))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metadataDir := filepath.Join(metadataRoot, entry.Name())
		b, err := os.ReadFile(filepath.Join(metadataDir, "gitdir"))
		if err != nil {
			continue
		}
		registeredGitDir := strings.TrimSpace(string(b))
		if !filepath.IsAbs(registeredGitDir) {
			registeredGitDir = filepath.Join(metadataDir, registeredGitDir)
		}
		registeredGitDir, err = filepath.Abs(registeredGitDir)
		if err != nil || filepath.Clean(registeredGitDir) != targetGitDir {
			continue
		}
		cleanMetadataDir, err := filepath.Abs(metadataDir)
		if err != nil {
			return err
		}
		cleanMetadataRoot, err := filepath.Abs(metadataRoot)
		if err != nil {
			return err
		}
		if cleanMetadataDir == cleanMetadataRoot || !contained(cleanMetadataRoot, cleanMetadataDir) {
			return fail("unsafe_path", "worktree metadata is outside the Git common directory")
		}
		if err := os.RemoveAll(cleanMetadataDir); err != nil {
			return err
		}
		return syncDirectory(cleanMetadataRoot)
	}
	return fail("worktree_metadata_missing", "could not locate Git metadata for broken worktree %s", target)
}

func localBranchExists(ctx context.Context, root, branch string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, fail("git_error", "could not inspect branch %s: %v", branch, err)
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
