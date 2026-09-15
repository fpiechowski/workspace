package core

import (
	"context"
	"errors"
	"os"
	"time"
)

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

	info, err := s.SupervisorStatus(ctx)
	out.SupervisorAt = nowUTC()
	if err == nil {
		out.SupervisorState = "running"
		if tmux, ok := s.Runtime.(Tmux); ok && info.TmuxSocket != tmux.Socket {
			out.SupervisorState = "conflict"
			out.SupervisorError = "supervisor is using a different tmux server"
		}
	} else if errors.Is(err, os.ErrNotExist) {
		out.SupervisorState = "stopped"
	} else {
		out.SupervisorState = "unavailable"
		out.SupervisorError = err.Error()
	}
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
