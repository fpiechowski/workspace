package core

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type managedFakeRuntime struct {
	fakeRuntime
	topology    TmuxTopology
	topologyErr error
	launchErr   error
	stopErr     error
	uiLaunches  int
	uiStops     int
}

func (r *managedFakeRuntime) ObserveTopology(context.Context, string) (TmuxTopology, error) {
	return r.topology, r.topologyErr
}

func (r *managedFakeRuntime) LaunchUI(_ context.Context, launch UILaunch) (Pane, error) {
	r.uiLaunches++
	if r.launchErr != nil {
		return Pane{}, r.launchErr
	}
	pane := Pane{
		ID: fmt.Sprintf("%%ui%d", r.uiLaunches), WindowID: launch.WindowID,
		Kind: "tui", WorkspaceID: launch.WorkspaceID, UIID: launch.UIID,
		UIGeneration: launch.Generation, UIToken: launch.Token,
		StartCommand: (Tmux{}).uiRunnerCommand(launch), SessionName: TmuxName(launch.WorkspaceID),
	}
	if r.fakeRuntime.panes == nil {
		r.fakeRuntime.panes = map[string]Pane{}
	}
	r.fakeRuntime.panes[pane.ID] = pane
	r.topology.Panes = append(r.topology.Panes, pane)
	return pane, nil
}

func (r *managedFakeRuntime) StopUI(_ context.Context, launch UILaunch, paneID string) error {
	if r.stopErr != nil {
		return r.stopErr
	}
	pane, ok := r.fakeRuntime.panes[paneID]
	uiID, generation, token, verified := uiPaneIdentity(pane, launch.WorkspaceID)
	if !ok || !verified || uiID != launch.UIID || generation != launch.Generation || token != launch.Token {
		return fail("pane_mismatch", "fake runtime refused an unowned UI pane")
	}
	delete(r.fakeRuntime.panes, paneID)
	kept := r.topology.Panes[:0]
	for _, candidate := range r.topology.Panes {
		if candidate.ID != paneID {
			kept = append(kept, candidate)
		}
	}
	r.topology.Panes = kept
	r.uiStops++
	return nil
}

func TestManagedInterfaceCleansVerifiedStaleGenerationsAndDuplicates(t *testing.T) {
	s, workspaceID, runtime := managedFixture(t)
	ctx := context.Background()
	status, err := s.SetUIPaneDesired(ctx, workspaceID, true, "show:generation-cleanup")
	if err != nil {
		t.Fatal(err)
	}
	oldPane := runtime.topology.Panes[len(runtime.topology.Panes)-1]
	if err := s.ClaimUIPane(ctx, workspaceID, status.UIID, status.Generation, oldPane.UIToken, oldPane.ID); err != nil {
		t.Fatal(err)
	}

	newLaunch := UILaunch{WorkspaceID: workspaceID, UIID: status.UIID, Generation: 2, Token: ID("uit")}
	current := Pane{ID: "%ui-current", WindowID: "@orch", Kind: "tui", WorkspaceID: workspaceID, UIID: newLaunch.UIID, UIGeneration: newLaunch.Generation, UIToken: newLaunch.Token, SessionName: TmuxName(workspaceID), StartCommand: (Tmux{}).uiRunnerCommand(newLaunch)}
	duplicate := current
	duplicate.ID = "%ui-duplicate"
	runtime.fakeRuntime.panes[current.ID] = current
	runtime.fakeRuntime.panes[duplicate.ID] = duplicate
	runtime.topology.Panes = append(runtime.topology.Panes, current, duplicate)

	// Simulate the recorded generation advancing after a crash left the old
	// generation behind. Strip the old pane metadata to exercise command proof.
	oldPane.Kind, oldPane.WorkspaceID, oldPane.UIID, oldPane.UIGeneration, oldPane.UIToken = "", "", "", 0, ""
	runtime.fakeRuntime.panes[oldPane.ID] = oldPane
	for i := range runtime.topology.Panes {
		if runtime.topology.Panes[i].ID == oldPane.ID {
			runtime.topology.Panes[i] = oldPane
		}
	}
	foreign := Pane{ID: "%foreign", WindowID: "@orch", Kind: "shell", WorkspaceID: workspaceID, SessionName: TmuxName(workspaceID), StartCommand: "bash"}
	runtime.fakeRuntime.panes[foreign.ID] = foreign
	runtime.topology.Panes = append(runtime.topology.Panes, foreign)

	if err := s.With(ctx, workspaceID, func(d *Document) error {
		record, _, err := readUIRecord(d.Dir, d.State.ID, d.State.ProjectID)
		if err != nil {
			return err
		}
		record.Generation, record.LaunchToken = newLaunch.Generation, newLaunch.Token
		record.PaneID, record.WindowID, record.State = current.ID, current.WindowID, "running"
		return writeUIRecord(d.Dir, record)
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileInterface(ctx, workspaceID); err != nil {
		t.Fatal(err)
	}
	status, err = s.UIPaneStatus(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "running" || status.Generation != 2 || status.PaneID != current.ID || runtime.uiStops != 2 {
		t.Fatalf("stale generation and duplicate cleanup changed the current runner: status=%+v stops=%d", status, runtime.uiStops)
	}
	if len(runtime.topology.Panes) != 3 {
		t.Fatalf("cleanup should retain orchestrator, one UI pane, and foreign shell: %+v", runtime.topology.Panes)
	}
	for _, pane := range runtime.topology.Panes {
		if pane.ID == oldPane.ID || pane.ID == duplicate.ID || pane.ID == foreign.ID {
			if pane.ID != foreign.ID {
				t.Fatalf("verified stale UI pane survived cleanup: %+v", pane)
			}
		}
	}
}

func TestUIPaneIdentityUsesExactRunnerCommandAndSession(t *testing.T) {
	launch := UILaunch{WorkspaceID: "ws_command_proof", UIID: "ui_command_proof", Generation: 4, Token: "uit_command_proof"}
	pane := Pane{SessionName: TmuxName(launch.WorkspaceID), StartCommand: (Tmux{}).uiRunnerCommand(launch)}
	uiID, generation, token, ok := uiPaneIdentity(pane, launch.WorkspaceID)
	if !ok || uiID != launch.UIID || generation != launch.Generation || token != launch.Token {
		t.Fatalf("runner command did not prove its exact launch identity: %q %q %d %q %t", pane.StartCommand, uiID, generation, token, ok)
	}
	pane.SessionName = TmuxName("ws_other")
	if _, _, _, ok := uiPaneIdentity(pane, launch.WorkspaceID); ok {
		t.Fatal("a runner command outside the workspace tmux session proved ownership")
	}
}

func managedFixture(t *testing.T) (*Service, string, *managedFakeRuntime) {
	t.Helper()
	s, workspaceID := fixture(t)
	now := nowUTC()
	runtime := &managedFakeRuntime{
		fakeRuntime: fakeRuntime{panes: map[string]Pane{}},
		topology: TmuxTopology{
			WorkspaceID: workspaceID, SessionName: TmuxName(workspaceID), SessionExists: true,
			Windows: []TmuxWindow{{ID: "@orch", Kind: "orchestrator", Width: 160, Height: 48}},
		},
	}
	s.Runtime = runtime
	if err := s.With(context.Background(), workspaceID, func(d *Document) error {
		sessionID, runID := "sess_orchestrator_ui_test", "run_orchestrator_ui_test"
		d.Registry.Sessions = append(d.Registry.Sessions, Session{
			ID: sessionID, AgentID: d.State.OrchestratorAgentID,
			AgentSnapshot: Agent{ID: d.State.OrchestratorAgentID, Role: "orchestrator"},
			CurrentRunID:  runID, LastRunID: runID, State: "running", LifecycleState: "active", LastActiveAt: now,
		})
		d.Registry.Runs = append(d.Registry.Runs, Run{ID: runID, SessionID: sessionID, State: "running", WindowID: "@orch", CreatedAt: now})
		runtime.topology.Panes = append(runtime.topology.Panes, Pane{
			ID: "%orch", WindowID: "@orch", Kind: "agent", WorkspaceID: workspaceID,
			SessionID: sessionID, RunID: runID, SessionName: TmuxName(workspaceID),
		})
		runtime.fakeRuntime.panes["%orch"] = runtime.topology.Panes[len(runtime.topology.Panes)-1]
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	return s, workspaceID, runtime
}

func TestUIPaneStatusDefaultsToDisabled(t *testing.T) {
	s, workspaceID, _ := managedFixture(t)
	status, err := s.UIPaneStatus(context.Background(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Desired || status.State != "disabled" || status.PaneID != "" {
		t.Fatalf("UI without an explicit show intent is not disabled: %+v", status)
	}
}

func TestManagedInterfaceReceiptsConcurrencyAndOwnership(t *testing.T) {
	s, workspaceID, runtime := managedFixture(t)
	ctx := context.Background()
	type outcome struct {
		status UIStatus
		err    error
	}
	outcomes := make(chan outcome, 2)
	for range 2 {
		go func() {
			status, err := s.SetUIPaneDesired(ctx, workspaceID, true, "show:stable")
			outcomes <- outcome{status, err}
		}()
	}
	first, second := <-outcomes, <-outcomes
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent show failed: %v / %v", first.err, second.err)
	}
	if first.status.OperationID == "" || first.status.OperationID != second.status.OperationID {
		t.Fatalf("same-key show did not replay its operation: %+v / %+v", first.status, second.status)
	}
	if runtime.uiLaunches != 1 || len(runtime.topology.Panes) != 2 {
		t.Fatalf("concurrent ensure created duplicate UI panes: launches=%d panes=%d", runtime.uiLaunches, len(runtime.topology.Panes))
	}

	status, err := s.UIPaneStatus(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "starting" || status.PaneID != "%ui1" || !status.Desired {
		t.Fatalf("unexpected launched status: %+v", status)
	}
	if err := s.ClaimUIPane(ctx, workspaceID, status.UIID, status.Generation, runtime.topology.Panes[1].UIToken, status.PaneID); err != nil {
		t.Fatalf("current runner could not claim the pane: %v", err)
	}
	if err := s.ClaimUIPane(ctx, workspaceID, status.UIID, status.Generation, runtime.topology.Panes[1].UIToken, status.PaneID); err == nil {
		t.Fatal("second runner claimed the same generation")
	} else {
		expectCode(t, err, "ui_claim_conflict")
	}

	replayed, err := s.SetUIPaneDesired(ctx, workspaceID, true, "show:stable")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.OperationID != first.status.OperationID || runtime.uiLaunches != 1 {
		t.Fatalf("receipt replay changed or launched another pane: %+v, launches=%d", replayed, runtime.uiLaunches)
	}
	if _, err := s.SetUIPaneDesired(ctx, workspaceID, false, "show:stable"); err == nil {
		t.Fatal("same operation key accepted a conflicting hide")
	} else {
		expectCode(t, err, "operation_conflict")
	}
}

func TestManagedInterfaceHideDoesNotRequireSupervisorCleanup(t *testing.T) {
	s, workspaceID, runtime := managedFixture(t)
	ctx := context.Background()
	status, err := s.SetUIPaneDesired(ctx, workspaceID, true, "show:hide-test")
	if err != nil {
		t.Fatal(err)
	}
	uiPane := runtime.topology.Panes[len(runtime.topology.Panes)-1]
	if err := s.ClaimUIPane(ctx, workspaceID, status.UIID, status.Generation, uiPane.UIToken, uiPane.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.HideManagedUIPane(ctx, workspaceID, "q:hide-test"); err != nil {
		t.Fatal(err)
	}
	var record uiRecord
	if err := s.withReadableWorkspace(ctx, workspaceID, func(d *Document) error {
		var err error
		record, _, err = readUIRecord(d.Dir, d.State.ID, d.State.ProjectID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if record.Desired || record.ExitRequested || record.State != "disabled" {
		t.Fatalf("q did not persist the user-owned hide intent: %+v", record)
	}
	if runtime.uiStops != 0 {
		t.Fatal("q unexpectedly cleaned the live TUI before terminal restoration")
	}
	if err := s.ReconcileInterface(ctx, workspaceID); err != nil {
		t.Fatal(err)
	}
	status, err = s.UIPaneStatus(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Desired || status.State != "disabled" || status.PaneID != "" || runtime.uiStops != 1 {
		t.Fatalf("explicit UI cleanup did not remove the hidden pane: status=%+v stops=%d", status, runtime.uiStops)
	}
}

func TestSupervisorDoesNotRestoreManagedInterface(t *testing.T) {
	s, workspaceID, runtime := managedFixture(t)
	ctx := context.Background()
	status, err := s.SetUIPaneDesired(ctx, workspaceID, true, "show:supervisor-no-restore")
	if err != nil {
		t.Fatal(err)
	}
	uiPane := runtime.topology.Panes[len(runtime.topology.Panes)-1]
	if err := s.ClaimUIPane(ctx, workspaceID, status.UIID, status.Generation, uiPane.UIToken, uiPane.ID); err != nil {
		t.Fatal(err)
	}
	delete(runtime.fakeRuntime.panes, uiPane.ID)
	kept := runtime.topology.Panes[:0]
	for _, pane := range runtime.topology.Panes {
		if pane.ID != uiPane.ID {
			kept = append(kept, pane)
		}
	}
	runtime.topology.Panes = kept
	launches := runtime.uiLaunches
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if runtime.uiLaunches != launches {
		t.Fatalf("supervisor restored the managed UI: launches before=%d after=%d", launches, runtime.uiLaunches)
	}
	if _, err := s.SetUIPaneDesired(ctx, workspaceID, true, "show:explicit-restore"); err != nil {
		t.Fatal(err)
	}
	if runtime.uiLaunches != launches+1 {
		t.Fatalf("explicit show did not restore the managed UI: launches before=%d after=%d", launches, runtime.uiLaunches)
	}
}

func TestManagedInterfaceBackoffAndTopologyErrorRetainOwnership(t *testing.T) {
	s, workspaceID, runtime := managedFixture(t)
	ctx := context.Background()
	status, err := s.SetUIPaneDesired(ctx, workspaceID, true, "show:recovery")
	if err != nil {
		t.Fatal(err)
	}
	uiPane := runtime.topology.Panes[len(runtime.topology.Panes)-1]
	if err := s.ClaimUIPane(ctx, workspaceID, status.UIID, status.Generation, uiPane.UIToken, uiPane.ID); err != nil {
		t.Fatal(err)
	}
	runtime.topologyErr = fail("tmux_error", "temporary server error")
	if err := s.ReconcileInterface(ctx, workspaceID); err == nil {
		t.Fatal("expected topology observation error")
	}
	status, err = s.UIPaneStatus(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "backoff" || status.Failures != 1 || status.PaneID != uiPane.ID || status.Generation != 1 {
		t.Fatalf("transient topology error lost the pane reservation: %+v", status)
	}

	runtime.topologyErr = nil
	runtime.topology.Panes = runtime.topology.Panes[:1]
	delete(runtime.fakeRuntime.panes, uiPane.ID)
	if err := s.ReconcileInterface(ctx, workspaceID); err != nil {
		t.Fatal(err)
	}
	status, err = s.UIPaneStatus(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "backoff" || status.Failures != 1 || status.PaneID != uiPane.ID || status.NextRetryAt == nil || status.NextRetryAt.Before(time.Now()) {
		t.Fatalf("transient topology error did not preserve retry reservation: %+v", status)
	}
	if runtime.uiLaunches != 1 {
		t.Fatalf("reconcile restarted inside backoff: launches=%d", runtime.uiLaunches)
	}

	if err := s.With(ctx, workspaceID, func(d *Document) error {
		record, _, err := readUIRecord(d.Dir, d.State.ID, d.State.ProjectID)
		if err != nil {
			return err
		}
		past := nowUTC().Add(-time.Second)
		record.NextRetryAt = &past
		return writeUIRecord(d.Dir, record)
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileInterface(ctx, workspaceID); err != nil {
		t.Fatal(err)
	}
	status, err = s.UIPaneStatus(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Failures != 1 || status.State != "starting" || status.Generation != 2 || runtime.uiLaunches != 2 {
		t.Fatalf("retry after backoff did not create one new generation: %+v launches=%d", status, runtime.uiLaunches)
	}
}

func TestManagedInterfaceArchiveCleansOnlyItsPane(t *testing.T) {
	s, workspaceID, runtime := managedFixture(t)
	ctx := context.Background()
	status, err := s.SetUIPaneDesired(ctx, workspaceID, true, "show:archive-test")
	if err != nil {
		t.Fatal(err)
	}
	uiPane := runtime.topology.Panes[len(runtime.topology.Panes)-1]
	if err := s.ClaimUIPane(ctx, workspaceID, status.UIID, status.Generation, uiPane.UIToken, uiPane.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, workspaceID, func(d *Document) error {
		d.State.Status = "archived"
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileInterface(ctx, workspaceID); err != nil {
		t.Fatal(err)
	}
	status, err = s.UIPaneStatus(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Desired || status.State != "disabled" || runtime.uiStops != 1 || len(runtime.topology.Panes) != 1 || runtime.topology.Panes[0].ID != "%orch" {
		t.Fatalf("archive cleanup affected the wrong runtime: status=%+v stops=%d panes=%+v", status, runtime.uiStops, runtime.topology.Panes)
	}
}
