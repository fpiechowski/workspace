package core

import (
	"context"
	"errors"
	"testing"
)

type pauseFailRuntime struct {
	*fakeRuntime
	failPane string
	failOnce bool
}

func (r *pauseFailRuntime) Stop(ctx context.Context, paneID string) error {
	if r.failOnce && paneID == r.failPane {
		r.failOnce = false
		return errors.New("temporary stop failure")
	}
	return r.fakeRuntime.Stop(ctx, paneID)
}

func TestPauseInterruptGuardAndRetryUseSavedExactTargets(t *testing.T) {
	s, workspaceID := fixture(t)
	ctx := context.Background()
	workerAgent, worktree := worker(t, s, workspaceID, "pause-worker")
	workerSession, err := s.StartSession(ctx, workspaceID, SessionOptions{Agent: workerAgent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	runtime := s.Runtime.(*fakeRuntime)
	servicePaneID := "%service-pause"
	serviceID := "service_pause_test"
	runtime.panes[servicePaneID] = Pane{ID: servicePaneID, SessionID: serviceID}
	if err := s.With(ctx, workspaceID, func(d *Document) error {
		d.Registry.Services = append(d.Registry.Services, BackgroundService{
			ID: serviceID, Name: "watcher", WorktreeID: worktree.ID, PaneID: servicePaneID, State: "running",
		})
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	before, err := s.Status(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	guard := MutationGuard{
		ExpectedRevision:   before.Workspace.Revision,
		ExpectedRunIDs:     []string{workerSession.CurrentRunID},
		ExpectedServiceIDs: []string{serviceID},
	}
	stale := guard
	stale.ExpectedRunIDs = []string{"run_changed"}
	if _, err := s.PauseInterruptGuarded(ctx, workspaceID, "pause-stale-targets", stale); err == nil {
		t.Fatal("stale Run list was accepted")
	} else {
		expectCode(t, err, "target_changed")
	}
	unchanged, err := s.Status(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var workerStillActive bool
	for _, session := range unchanged.Sessions {
		if session.ID == workerSession.ID {
			workerStillActive = session.Active()
		}
	}
	if unchanged.Workspace.Status == "paused" || !workerStillActive {
		t.Fatalf("stale guard mutated the workspace or session: %+v", unchanged)
	}

	failingRuntime := &pauseFailRuntime{fakeRuntime: runtime, failPane: workerSession.PaneID, failOnce: true}
	s.Runtime = failingRuntime
	if _, err := s.PauseInterruptGuarded(ctx, workspaceID, "pause-retry-exact-targets", guard); err == nil {
		t.Fatal("expected an interrupted stop step")
	}
	partial, err := s.Status(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Workspace.Status != "paused" || partial.Sessions[0].CurrentRunID != workerSession.CurrentRunID {
		t.Fatalf("pause intent or failed exact step was not retained: %+v", partial)
	}
	service, err := findServiceInStatus(ctx, s, workspaceID, serviceID)
	if err != nil || service.Active() {
		t.Fatalf("first completed step was not durable: %+v %v", service, err)
	}

	newPaneID := "%new-run-after-pause"
	failingRuntime.panes[newPaneID] = Pane{ID: newPaneID, SessionID: "sess_after_pause", RunID: "run_after_pause"}
	if err := s.With(ctx, workspaceID, func(d *Document) error {
		d.Registry.Sessions = append(d.Registry.Sessions, Session{
			ID: "sess_after_pause", AgentID: "agent_after_pause", CurrentRunID: "run_after_pause", LastRunID: "run_after_pause",
			State: "running", LifecycleState: "active",
		})
		d.Registry.Runs = append(d.Registry.Runs, Run{ID: "run_after_pause", SessionID: "sess_after_pause", State: "running", PaneID: newPaneID})
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PauseInterruptGuarded(ctx, workspaceID, "pause-retry-exact-targets", guard); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	oldRun, err := findRunInStatus(status, workerSession.CurrentRunID)
	if err != nil || oldRun.Active() {
		t.Fatalf("confirmed Run was not stopped: %+v %v", oldRun, err)
	}
	newRun, err := findRunInStatus(status, "run_after_pause")
	if err != nil || !newRun.Active() {
		t.Fatalf("retry adopted and stopped a Run outside the saved plan: %+v %v", newRun, err)
	}
	replayed, err := s.PauseInterruptGuarded(ctx, workspaceID, "pause-retry-exact-targets", guard)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := findRunInStatus(replayed, "run_after_pause"); err != nil || !got.Active() {
		t.Fatalf("receipt replay touched a later Run: %+v %v", got, err)
	}
}

func findServiceInStatus(ctx context.Context, s *Service, selector, id string) (BackgroundService, error) {
	var result BackgroundService
	err := s.With(ctx, selector, func(d *Document) error {
		service, err := findService(d, id)
		if err == nil {
			result = *service
		}
		return err
	})
	return result, err
}
