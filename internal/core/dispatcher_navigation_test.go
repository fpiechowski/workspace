package core

import (
	"context"
	"errors"
	"testing"
)

type dispatcherNavigationRuntime struct {
	fakeRuntime
	topology TmuxTopology
}

func (r *dispatcherNavigationRuntime) ObserveProjectTopology(context.Context, string) (TmuxTopology, error) {
	return r.topology, nil
}

func projectIDOf(t *testing.T, s *Service) string {
	t.Helper()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	return cfg.ProjectID
}

func saveDispatcherNavigationState(t *testing.T, s *Service, state dispatcherState) {
	t.Helper()
	if err := saveDispatcherState(s.Root, state); err != nil {
		t.Fatal(err)
	}
}

func runningDispatcherState(projectID, sessionID, runID, paneID, windowID string) dispatcherState {
	state := defaultDispatcherState(projectID)
	state.Agent = Agent{ID: "agent_dispatcher", Role: "dispatcher", Scope: "project"}
	state.Sessions = []Session{{
		ID: sessionID, AgentID: "agent_dispatcher", CurrentRunID: runID,
		RunState: "running", State: "running", LifecycleState: "active",
		PaneID: paneID, WindowID: windowID,
	}}
	state.Runs = []Run{{ID: runID, SessionID: sessionID, State: "running", PaneID: paneID, WindowID: windowID}}
	return state
}

func dispatcherProjectTopology(projectID, sessionID, runID, paneID, windowID string) TmuxTopology {
	return TmuxTopology{
		ProjectID: projectID, Scope: "project", SessionName: DispatcherTmuxName(projectID), SessionExists: true,
		Windows: []TmuxWindow{{ID: windowID, Kind: "dispatcher"}},
		Panes: []Pane{{
			ID: paneID, WindowID: windowID, SessionID: sessionID, RunID: runID,
			Kind: "dispatcher", Scope: "project", ProjectID: projectID,
		}},
	}
}

func TestResolveDispatcherNavigationTarget(t *testing.T) {
	ctx := context.Background()
	const (
		sessionID = "sess_dispatch"
		runID     = "run_dispatch"
		paneID    = "%dispatch"
		windowID  = "@dispatch"
	)

	t.Run("verified running dispatcher", func(t *testing.T) {
		s, _ := fixture(t)
		projectID := projectIDOf(t, s)
		s.Runtime = &dispatcherNavigationRuntime{topology: dispatcherProjectTopology(projectID, sessionID, runID, paneID, windowID)}
		saveDispatcherNavigationState(t, s, runningDispatcherState(projectID, sessionID, runID, paneID, windowID))

		target, err := s.ResolveNavigationTarget(ctx, "", EntityRef{Kind: "dispatcher"})
		if err != nil {
			t.Fatal(err)
		}
		if target.Kind != "dispatcher" || target.ProjectID != projectID ||
			target.SessionName != DispatcherTmuxName(projectID) || target.SessionID != sessionID ||
			target.RunID != runID || target.PaneID != paneID || target.WindowID != windowID ||
			target.WorkspaceID != "" {
			t.Fatalf("unexpected dispatcher target: %+v", target)
		}
	})

	t.Run("never started", func(t *testing.T) {
		s, _ := fixture(t)
		s.Runtime = &dispatcherNavigationRuntime{topology: TmuxTopology{}}
		expectDispatcherPaneMissing(t, s, ctx)
	})

	t.Run("no live run", func(t *testing.T) {
		s, _ := fixture(t)
		projectID := projectIDOf(t, s)
		s.Runtime = &dispatcherNavigationRuntime{topology: dispatcherProjectTopology(projectID, sessionID, runID, paneID, windowID)}
		state := defaultDispatcherState(projectID)
		state.Agent = Agent{ID: "agent_dispatcher", Role: "dispatcher", Scope: "project"}
		state.Sessions = []Session{{ID: sessionID, AgentID: "agent_dispatcher", State: "idle", LifecycleState: "idle"}}
		saveDispatcherNavigationState(t, s, state)
		expectDispatcherPaneMissing(t, s, ctx)
	})

	t.Run("unverified pane", func(t *testing.T) {
		s, _ := fixture(t)
		projectID := projectIDOf(t, s)
		s.Runtime = &dispatcherNavigationRuntime{topology: TmuxTopology{}}
		saveDispatcherNavigationState(t, s, runningDispatcherState(projectID, sessionID, runID, paneID, windowID))
		expectDispatcherPaneMissing(t, s, ctx)
	})

	t.Run("ownership mismatch", func(t *testing.T) {
		s, _ := fixture(t)
		projectID := projectIDOf(t, s)
		s.Runtime = &dispatcherNavigationRuntime{topology: dispatcherProjectTopology(projectID, sessionID, "run_foreign", paneID, windowID)}
		saveDispatcherNavigationState(t, s, runningDispatcherState(projectID, sessionID, runID, paneID, windowID))
		expectDispatcherPaneMissing(t, s, ctx)
	})

	t.Run("workspace selector is ignored for dispatcher", func(t *testing.T) {
		s, _ := fixture(t)
		projectID := projectIDOf(t, s)
		s.Runtime = &dispatcherNavigationRuntime{topology: dispatcherProjectTopology(projectID, sessionID, runID, paneID, windowID)}
		saveDispatcherNavigationState(t, s, runningDispatcherState(projectID, sessionID, runID, paneID, windowID))
		if _, err := s.ResolveNavigationTarget(ctx, "ws_missing", EntityRef{Kind: "dispatcher"}); err != nil {
			t.Fatalf("dispatcher resolution rejected a non-empty selector: %v", err)
		}
	})
}

func expectDispatcherPaneMissing(t *testing.T, s *Service, ctx context.Context) {
	t.Helper()
	_, err := s.ResolveNavigationTarget(ctx, "", EntityRef{Kind: "dispatcher"})
	if err == nil {
		t.Fatal("dispatcher resolution unexpectedly succeeded")
	}
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != "pane_missing" {
		t.Fatalf("unexpected error: %v", err)
	}
}
