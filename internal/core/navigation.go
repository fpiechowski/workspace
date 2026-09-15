package core

import (
	"context"
	"strings"
)

type EntityRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

// NavigationTarget contains only IDs and ownership fields resolved from the
// workspace registry and a fresh runtime observation. It never accepts a
// caller-supplied tmux target string.
type NavigationTarget struct {
	WorkspaceID string `json:"workspace_id"`
	SessionName string `json:"session_name"`
	Socket      string `json:"socket,omitempty"`
	WindowID    string `json:"window_id,omitempty"`
	PaneID      string `json:"pane_id,omitempty"`
	Kind        string `json:"kind"`
	SessionID   string `json:"session_id,omitempty"`
	RunID       string `json:"run_id,omitempty"`
	ServiceID   string `json:"service_id,omitempty"`
	UIID        string `json:"ui_id,omitempty"`
}

// ResolveNavigationTarget validates the requested entity against both the
// durable workspace and current tmux ownership before returning a target.
func (s *Service) ResolveNavigationTarget(ctx context.Context, selector string, ref EntityRef) (NavigationTarget, error) {
	snapshot, err := s.WorkspaceSnapshot(ctx, selector)
	if err != nil {
		return NavigationTarget{}, err
	}
	reader, ok := s.Runtime.(RuntimeTopologyReader)
	if !ok {
		return NavigationTarget{}, fail("runtime_unsupported", "runtime does not support navigation")
	}
	topology, err := reader.ObserveTopology(ctx, snapshot.Status.Workspace.ID)
	if err != nil {
		return NavigationTarget{}, err
	}
	if !topology.SessionExists {
		return NavigationTarget{}, fail("pane_missing", "tmux session %s does not exist", TmuxName(snapshot.Status.Workspace.ID))
	}
	target := NavigationTarget{
		WorkspaceID: snapshot.Status.Workspace.ID,
		SessionName: TmuxName(snapshot.Status.Workspace.ID),
		Kind:        strings.ToLower(strings.TrimSpace(ref.Kind)),
	}
	if tmux, ok := s.Runtime.(Tmux); ok {
		target.Socket = tmux.Socket
	}

	switch target.Kind {
	case "workspace":
		if ref.ID != "" && ref.ID != target.WorkspaceID {
			return NavigationTarget{}, fail("workspace_not_found", "workspace %s is not selected", ref.ID)
		}
		window, err := uniqueWindow(topology, func(w TmuxWindow) bool { return w.Kind == "orchestrator" }, "orchestrator")
		if err != nil {
			return NavigationTarget{}, err
		}
		if window != nil {
			target.WindowID = window.ID
		}
		return target, nil
	case "orchestrator":
		window, err := uniqueWindow(topology, func(w TmuxWindow) bool { return w.Kind == "orchestrator" }, "orchestrator")
		if err != nil {
			return NavigationTarget{}, err
		}
		if window != nil {
			target.WindowID = window.ID
		}
		session, run, err := currentOrchestrator(snapshot.Status, ref.ID)
		if err != nil {
			return NavigationTarget{}, err
		}
		if run == nil || !run.Active() {
			return NavigationTarget{}, fail("pane_missing", "orchestrator has no live run")
		}
		pane, err := verifiedPane(topology, run.PaneID, session.ID, run.ID, snapshot.Status.Workspace.ID, "agent")
		if err != nil {
			return NavigationTarget{}, err
		}
		return setPaneTarget(target, pane, session.ID, run.ID), nil
	case "worktree":
		worktree, err := findWorktreeInStatus(snapshot.Status, ref.ID)
		if err != nil {
			return NavigationTarget{}, err
		}
		window, err := uniqueWindow(topology, func(w TmuxWindow) bool { return w.Kind == "worktree" && w.WorktreeID == worktree.ID }, "worktree")
		if err != nil {
			return NavigationTarget{}, err
		}
		if window != nil {
			target.WindowID = window.ID
			return target, nil
		}
		var matches []Pane
		for _, pane := range topology.Panes {
			if pane.Dead || pane.Kind == "tui" || pane.WorkspaceID != "" && pane.WorkspaceID != target.WorkspaceID {
				continue
			}
			if pane.Kind == "service" {
				for _, service := range snapshot.Services {
					if service.ID == pane.SessionID && service.WorktreeID == worktree.ID && service.Active() {
						matches = append(matches, pane)
					}
				}
				continue
			}
			for _, session := range snapshot.Status.Sessions {
				if session.WorktreeID != worktree.ID || session.CurrentRunID == "" {
					continue
				}
				run, e := findRunInStatus(snapshot.Status, session.CurrentRunID)
				if e == nil && run.Active() && paneOwns(pane, session.ID, run.ID) {
					matches = append(matches, pane)
				}
			}
		}
		pane, err := uniquePane(matches, "worktree has multiple live panes")
		if err != nil {
			return NavigationTarget{}, err
		}
		if pane == nil {
			return NavigationTarget{}, fail("pane_missing", "worktree %s has no live tmux window", worktree.ID)
		}
		target.WindowID = pane.WindowID
		target.PaneID = pane.ID
		return target, nil
	case "session":
		session, err := findSessionInStatus(snapshot.Status, ref.ID)
		if err != nil {
			return NavigationTarget{}, err
		}
		if session.CurrentRunID == "" {
			return NavigationTarget{}, fail("run_inactive", "session %s has no current run", session.ID)
		}
		run, err := findRunInStatus(snapshot.Status, session.CurrentRunID)
		if err != nil || !run.Active() {
			return NavigationTarget{}, fail("run_inactive", "session %s has no active current run", session.ID)
		}
		pane, err := verifiedPane(topology, run.PaneID, session.ID, run.ID, target.WorkspaceID, "agent")
		if err != nil {
			return NavigationTarget{}, err
		}
		return setPaneTarget(target, pane, session.ID, run.ID), nil
	case "run":
		run, err := findRunInStatus(snapshot.Status, ref.ID)
		if err != nil {
			return NavigationTarget{}, err
		}
		session, err := findSessionInStatus(snapshot.Status, run.SessionID)
		if err != nil || session.CurrentRunID != run.ID || !run.Active() {
			return NavigationTarget{}, fail("run_inactive", "run %s is historical or no longer current", run.ID)
		}
		pane, err := verifiedPane(topology, run.PaneID, session.ID, run.ID, target.WorkspaceID, "agent")
		if err != nil {
			return NavigationTarget{}, err
		}
		return setPaneTarget(target, pane, session.ID, run.ID), nil
	case "service":
		service, err := findServiceInSnapshot(snapshot.Services, ref.ID)
		if err != nil {
			return NavigationTarget{}, err
		}
		if !service.Active() {
			return NavigationTarget{}, fail("pane_missing", "service %s is not active", service.ID)
		}
		pane, err := verifiedPane(topology, service.PaneID, service.ID, "", target.WorkspaceID, "service")
		if err != nil {
			return NavigationTarget{}, err
		}
		target = setPaneTarget(target, pane, "", "")
		target.ServiceID = service.ID
		return target, nil
	default:
		return NavigationTarget{}, fail("invalid_entity", "unsupported navigation kind %q", ref.Kind)
	}
}

func uniqueWindow(topology TmuxTopology, match func(TmuxWindow) bool, kind string) (*TmuxWindow, error) {
	var found *TmuxWindow
	for i := range topology.Windows {
		if !match(topology.Windows[i]) {
			continue
		}
		if found != nil {
			return nil, fail("navigation_ambiguous", "multiple %s windows are available", kind)
		}
		found = &topology.Windows[i]
	}
	return found, nil
}

func uniquePane(panes []Pane, message string) (*Pane, error) {
	if len(panes) > 1 {
		return nil, fail("navigation_ambiguous", "%s", message)
	}
	if len(panes) == 0 {
		return nil, nil
	}
	return &panes[0], nil
}

func verifiedPane(topology TmuxTopology, expectedPane, sessionID, runID, workspaceID, kind string) (*Pane, error) {
	var matches []Pane
	for _, pane := range topology.Panes {
		if pane.Dead || pane.Kind != kind || pane.WorkspaceID != "" && pane.WorkspaceID != workspaceID {
			continue
		}
		if expectedPane != "" && pane.ID != expectedPane {
			continue
		}
		if kind == "agent" && !paneOwns(pane, sessionID, runID) {
			continue
		}
		if kind == "service" && pane.SessionID != sessionID {
			continue
		}
		matches = append(matches, pane)
	}
	pane, err := uniquePane(matches, "entity has multiple verified live panes")
	if err != nil {
		return nil, err
	}
	if pane == nil {
		return nil, fail("pane_missing", "no verified live pane for %s", sessionID)
	}
	return pane, nil
}

func setPaneTarget(target NavigationTarget, pane *Pane, sessionID, runID string) NavigationTarget {
	target.WindowID, target.PaneID = pane.WindowID, pane.ID
	target.SessionID, target.RunID = sessionID, runID
	return target
}

func currentOrchestrator(status Status, requested string) (*Session, *Run, error) {
	agentID := status.Workspace.OrchestratorAgentID
	if requested != "" && requested != agentID {
		return nil, nil, fail("agent_not_found", "unknown orchestrator %s", requested)
	}
	var selected *Session
	for i := range status.Sessions {
		session := &status.Sessions[i]
		if session.AgentID != agentID {
			continue
		}
		if selected == nil || session.LastActiveAt.After(selected.LastActiveAt) {
			selected = session
		}
	}
	if selected == nil {
		return nil, nil, fail("agent_not_found", "workspace has no orchestrator session")
	}
	if selected.CurrentRunID == "" {
		return selected, nil, nil
	}
	run, err := findRunInStatus(status, selected.CurrentRunID)
	if err != nil {
		return selected, nil, err
	}
	return selected, run, nil
}

func findWorktreeInStatus(status Status, id string) (*Worktree, error) {
	for i := range status.Worktrees {
		if status.Worktrees[i].ID == id {
			return &status.Worktrees[i], nil
		}
	}
	return nil, fail("worktree_not_found", "unknown worktree %s", id)
}

func findSessionInStatus(status Status, id string) (*Session, error) {
	for i := range status.Sessions {
		if status.Sessions[i].ID == id {
			return &status.Sessions[i], nil
		}
	}
	return nil, fail("session_not_found", "unknown session %s", id)
}

func findRunInStatus(status Status, id string) (*Run, error) {
	for i := range status.Runs {
		if status.Runs[i].ID == id {
			return &status.Runs[i], nil
		}
	}
	return nil, fail("run_not_found", "unknown run %s", id)
}

func findServiceInSnapshot(services []BackgroundService, id string) (*BackgroundService, error) {
	for i := range services {
		if services[i].ID == id || services[i].Name == id {
			return &services[i], nil
		}
	}
	return nil, fail("service_not_found", "unknown service %s", id)
}
