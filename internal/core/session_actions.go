package core

import (
	"context"
)

// StartSupervisedSession is the shared CLI/TUI entry point. The supervisor is
// ready before the domain operation reserves and launches a Run.
func (s *Service) StartSupervisedSession(ctx context.Context, selector string, opt SessionOptions) (Session, error) {
	if err := s.requireWorkspaceScope(); err != nil {
		return Session{}, err
	}
	if err := s.EnsureSupervisor(ctx); err != nil {
		return Session{}, err
	}
	return s.StartSession(ctx, selector, opt)
}

func (s *Service) StartSupervisedOrchestrator(ctx context.Context, selector, key string) (Session, error) {
	return s.StartSupervisedSession(ctx, selector, SessionOptions{Agent: "orchestrator", OperationKey: key})
}

func (s *Service) ResumeSupervisedAgent(ctx context.Context, selector, agent, key string) (Session, error) {
	if err := s.requireWorkspaceScope(); err != nil {
		return Session{}, err
	}
	if err := s.EnsureSupervisor(ctx); err != nil {
		return Session{}, err
	}
	return s.ResumeAgent(ctx, selector, agent, key)
}

// ResumeSession resumes exactly the selected logical Session. Run IDs,
// including legacy sess_* Run aliases, resolve to their owning Session here so
// terminal adapters and TUI use the same lineage rules as the CLI.
func (s *Service) ResumeSession(ctx context.Context, selector, sessionOrRunID, key string) (Session, error) {
	if err := s.requireWorkspaceScope(); err != nil {
		return Session{}, err
	}
	var opt SessionOptions
	err := s.With(ctx, selector, func(d *Document) error {
		session, err := findSession(d, sessionOrRunID)
		if err != nil {
			run, runErr := findRun(d, sessionOrRunID)
			if runErr != nil {
				return err
			}
			session, err = findSession(d, run.SessionID)
			if err != nil {
				return err
			}
		}
		if session.DeletedAt != nil {
			return fail("session_deleted", "session %s was deleted", session.ID)
		}
		opt = SessionOptions{
			Agent:           session.AgentID,
			Worktree:        session.WorktreeID,
			Parent:          session.ParentAgentID,
			ParentSessionID: session.ParentSessionID,
			Profile:         session.Profile,
			Task:            session.TaskID,
			ResumeSession:   session.ID,
			ReadOnly:        session.ReadOnly,
			OperationKey:    key,
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	if err := s.EnsureSupervisor(ctx); err != nil {
		return Session{}, err
	}
	return s.StartSession(ctx, selector, opt)
}

// ReconcileWorkspace applies domain runtime recovery for client panes.
func (s *Service) ReconcileWorkspace(ctx context.Context, selector string, keys ...string) (Status, error) {
	return s.Reconcile(ctx, selector, keys...)
}
