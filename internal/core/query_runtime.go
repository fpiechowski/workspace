package core

import (
	"context"
	"errors"
	"os"
	"time"
)

// SupervisorObservation is a best-effort, project-scoped view of the
// supervisor process. The observation captures failures as states so callers
// can keep rendering durable data while still showing why the health check is
// not healthy.
type SupervisorObservation struct {
	State      string    `json:"state"`
	Error      string    `json:"error,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

// RuntimeObservation is a best-effort view of external runtime state. Runtime
// failures are captured here so callers can still display the durable snapshot.
type RuntimeObservation struct {
	WorkspaceID     string       `json:"workspace_id"`
	State           string       `json:"state"`
	Error           string       `json:"error,omitempty"`
	SupervisorState string       `json:"supervisor_state"`
	SupervisorError string       `json:"supervisor_error,omitempty"`
	Topology        TmuxTopology `json:"topology"`
	ObservedAt      time.Time    `json:"observed_at"`
	SupervisorAt    time.Time    `json:"supervisor_observed_at"`
}

func classifySupervisorObservation(runtime Runtime, info SupervisorInfo, err error) SupervisorObservation {
	out := SupervisorObservation{ObservedAt: nowUTC()}
	switch {
	case err == nil:
		out.State = "running"
		if tmux, ok := runtime.(Tmux); ok && info.TmuxSocket != tmux.Socket {
			out.State = "conflict"
			out.Error = "supervisor is using a different tmux server"
		}
	case errors.Is(err, os.ErrNotExist):
		out.State = "stopped"
	default:
		out.State = "unavailable"
		out.Error = err.Error()
	}
	return out
}

// ObserveSupervisor reads the project supervisor descriptor and pings the
// supervisor without starting, stopping, or reconciling any runtime. Missing
// supervisor files or sockets are the normal stopped state; other failures are
// retained in the observation for display and attention views.
func (s *Service) ObserveSupervisor(ctx context.Context) (SupervisorObservation, error) {
	info, err := s.SupervisorStatus(ctx)
	return classifySupervisorObservation(s.Runtime, info, err), nil
}

// ObserveWorkspaceRuntime reads tmux and supervisor state without starting or
// reconciling either runtime. Errors are part of the observation, not fatal to
// the workspace's durable read model.
func (s *Service) ObserveWorkspaceRuntime(ctx context.Context, selector string) (RuntimeObservation, error) {
	var id string
	if err := s.withReadableWorkspace(ctx, selector, func(d *Document) error {
		id = d.State.ID
		return nil
	}); err != nil {
		return RuntimeObservation{}, err
	}
	out := RuntimeObservation{WorkspaceID: id, State: "unknown", SupervisorState: "unknown"}
	if reader, ok := s.Runtime.(RuntimeTopologyReader); ok {
		topology, err := reader.ObserveTopology(ctx, id)
		out.ObservedAt = nowUTC()
		out.Topology = topology
		if err != nil {
			out.Error = err.Error()
		} else if topology.SessionExists {
			out.State = "present"
		} else {
			out.State = "missing"
		}
	} else {
		out.Error = "runtime does not support topology inspection"
		out.ObservedAt = nowUTC()
	}

	supervisor, _ := s.ObserveSupervisor(ctx)
	out.SupervisorState = supervisor.State
	out.SupervisorError = supervisor.Error
	out.SupervisorAt = supervisor.ObservedAt
	return out, nil
}

type WorktreeObservation struct {
	WorkspaceID string    `json:"workspace_id"`
	WorktreeID  string    `json:"worktree_id"`
	Path        string    `json:"path"`
	Head        string    `json:"head,omitempty"`
	Dirty       bool      `json:"dirty"`
	Error       string    `json:"error,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

// InspectWorktree runs Git only on demand and outside the project lock.
func (s *Service) InspectWorktree(ctx context.Context, selector, worktreeID string) (WorktreeObservation, error) {
	var out WorktreeObservation
	var worktree Worktree
	if err := s.withReadableWorkspace(ctx, selector, func(d *Document) error {
		out.WorkspaceID = d.State.ID
		var err error
		found, err := findWorktree(d, worktreeID)
		if err != nil {
			return err
		}
		worktree = *found
		out.WorktreeID, out.Path = worktree.ID, worktree.Path
		return nil
	}); err != nil {
		return out, err
	}
	out.CheckedAt = nowUTC()
	if err := verifyWorktree(ctx, &worktree); err != nil {
		out.Error = err.Error()
		return out, nil
	}
	head, err := git(ctx, worktree.Path, "rev-parse", "HEAD")
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	out.Head = head
	changes, err := git(ctx, worktree.Path, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	out.Dirty = changes != ""
	return out, nil
}
