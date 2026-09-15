package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Opt-in: launches a real agent helper through the compiled CLI in an isolated tmux server.
func TestTmuxEndToEnd(t *testing.T) {
	if os.Getenv("WORKSPACE_TMUX_TEST") != "1" {
		t.Skip("set WORKSPACE_TMUX_TEST=1 to run real tmux integration")
	}
	if runtime.GOOS == "windows" {
		t.Skip("tmux integration runs in Linux/WSL or macOS")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatal(err)
	}
	s, id := fixture(t)
	ctx := context.Background()
	socket := "workspace-test-" + ID("tmux")
	rt := Tmux{Socket: socket}
	s.Runtime = rt
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	bin := filepath.Join(t.TempDir(), "bin with 'quotes'", "workspace")
	if err := os.MkdirAll(filepath.Dir(bin), 0700); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/workspace")
	build.Dir = filepath.Join("..", "..")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, b)
	}
	s.Executable = bin
	if err := s.EnsureSupervisor(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.StopSupervisor(context.Background()) })
	info, err := s.SupervisorStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSupervisor(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := s.SupervisorStatus(ctx)
	if err != nil || info.PID != again.PID {
		t.Fatal("supervisor start is not idempotent", err)
	}
	a, w := worker(t, s, id, "planner")
	session, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID, OperationKey: "tmux-plan"})
	if err != nil {
		t.Fatal(err)
	}
	waitExited := func(sessionID string) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			status, err := s.Status(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range status.Sessions {
				if p.ID == sessionID && !p.Active() {
					if p.State != "exited" {
						t.Fatalf("session failed: %+v", p)
					}
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		output, _ := rt.call(ctx, "capture-pane", "-p", "-t", session.PaneID, "-S", "-")
		t.Fatalf("session did not exit; pane output: %s", output)
	}
	waitExited(session.ID)
	// Simulate an interrupted ledger write after tmux started the process.
	if err := s.With(ctx, id, func(d *Document) error {
		p, _ := findSession(d, session.ID)
		r, _ := findRun(d, session.CurrentRunID)
		r.State = "starting"
		r.PaneID = ""
		r.WindowID = ""
		p.CurrentRunID = r.ID
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.call(ctx, "set-option", "-pu", "-t", session.PaneID, "@workspace_session_id"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.call(ctx, "set-option", "-pu", "-t", session.PaneID, "@workspace_run_id"); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Reconcile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Sessions[0].PaneID != session.PaneID || recovered.Sessions[0].State != "interrupted" {
		t.Fatalf("unrecorded pane recovery failed: %+v", recovered.Sessions[0])
	}
	data, err := os.ReadFile(filepath.Join(w.Path, "work-products", "identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var identity map[string]string
	if err := json.Unmarshal(data, &identity); err != nil {
		t.Fatal(err)
	}
	if identity["agent"] != a.ID || identity["session"] != session.ID || identity["run"] != session.CurrentRunID || identity["parent"] != session.ParentAgentID {
		t.Fatalf("wrong identity: %v", identity)
	}
	replay, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID, OperationKey: "tmux-plan"})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != session.ID {
		t.Fatal("idempotency did not survive process execution")
	}
	resumed, err := s.ResumeAgent(ctx, id, a.ID, "tmux-resume")
	if err != nil {
		t.Fatal(err)
	}
	waitExited(resumed.ID)
	if resumed.ID != session.ID || resumed.CurrentRunID == session.CurrentRunID || resumed.WindowID != session.WindowID || resumed.AgentID != a.ID {
		t.Fatal("resume did not preserve persona/worktree window")
	}
	activeWindowBeforeOrchestrator := activeWindowID(t, rt, id)
	orch, err := s.StartOrchestrator(ctx, id, "tmux-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	waitExited(orch.ID)
	if orch.WorktreeID != "" || orch.CWD == w.Path {
		t.Fatal("orchestrator must run from workspace directory")
	}
	ui := waitUIStatus(t, s, id, func(status UIStatus) bool { return status.State == "running" && status.PaneID != "" })
	if ui.WindowID != orch.WindowID || activeWindowID(t, rt, id) != activeWindowBeforeOrchestrator {
		t.Fatalf("managed UI did not share the orchestrator window without stealing focus: ui=%+v orchestrator=%+v", ui, orch)
	}
	uiPane, err := rt.Inspect(ctx, ui.PaneID)
	if err != nil || uiPane.Kind != "tui" || uiPane.UIID != ui.UIID || uiPane.UIGeneration != ui.Generation {
		t.Fatalf("managed pane ownership was not recorded: pane=%+v err=%v", uiPane, err)
	}
	if activePaneID(t, rt, orch.WindowID) != orch.PaneID || uiPane.Active {
		t.Fatalf("managed split changed the active orchestrator pane: ui=%+v orchestrator=%+v", uiPane, orch)
	}
	frame := waitTUIFrame(t, rt, ui.PaneID, "Fix checkout")
	if !strings.Contains(frame, "Current work") {
		t.Fatalf("managed TUI did not render current work: %s", frame)
	}
	if _, err := rt.call(ctx, "send-keys", "-t", ui.PaneID, "q"); err != nil {
		t.Fatal(err)
	}
	hidden := waitUIStatus(t, s, id, func(status UIStatus) bool { return !status.Desired && status.State == "disabled" })
	if hidden.PaneID != "" {
		t.Fatalf("hidden managed UI retained a pane: %+v", hidden)
	}
	shown, err := s.SetUIPaneDesired(ctx, id, true, "tmux-ui-show")
	if err != nil {
		t.Fatal(err)
	}
	shown = waitUIStatus(t, s, id, func(status UIStatus) bool { return status.State == "running" && status.PaneID != "" })
	if shown.Generation <= ui.Generation || shown.PaneID == ui.PaneID || shown.WindowID != orch.WindowID {
		t.Fatalf("show did not restore one new managed pane beside the orchestrator: hidden=%+v shown=%+v", ui, shown)
	}
	if activePaneID(t, rt, orch.WindowID) != orch.PaneID {
		t.Fatal("show changed focus from the orchestrator pane")
	}
	if _, err := rt.call(ctx, "select-pane", "-t", shown.PaneID); err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, id, func(d *Document) error {
		current, findErr := findSession(d, orch.ID)
		if findErr != nil {
			return findErr
		}
		current.ClientSnapshot.LaunchArgv = []string{"sh", "-c", "sleep 30"}
		current.ClientSnapshot.ResumeArgv = nil
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	resumedOrchestrator, err := s.ResumeAgent(ctx, id, orch.AgentID, "tmux-orchestrator-resume")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pane, inspectErr := rt.Inspect(ctx, resumedOrchestrator.PaneID)
		if inspectErr == nil && !pane.Dead {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if pane, inspectErr := rt.Inspect(ctx, resumedOrchestrator.PaneID); inspectErr != nil || pane.Dead {
		t.Fatalf("long-running orchestrator did not become active: pane=%+v err=%v", pane, inspectErr)
	}
	if resumedOrchestrator.ID != orch.ID || resumedOrchestrator.CurrentRunID == orch.CurrentRunID || resumedOrchestrator.PaneID == shown.PaneID {
		t.Fatalf("orchestrator resume did not create a new run in its own pane: before=%+v after=%+v", orch, resumedOrchestrator)
	}
	if activePaneID(t, rt, orch.WindowID) != shown.PaneID {
		t.Fatal("resuming the orchestrator stole focus from the active TUI pane")
	}
	uiAfterResume, err := s.UIPaneStatus(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if uiAfterResume.PaneID != shown.PaneID || uiAfterResume.Generation != shown.Generation {
		t.Fatalf("resuming the orchestrator replaced the managed TUI: before=%+v after=%+v", shown, uiAfterResume)
	}
	foreignPane, err := rt.call(ctx, "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", orch.WindowID, "sleep 30")
	if err != nil {
		t.Fatal(err)
	}
	foreignPane = strings.TrimSpace(foreignPane)
	for _, option := range []string{"@workspace_kind", "@workspace_id", "@workspace_ui_id", "@workspace_ui_generation", "@workspace_ui_token"} {
		if _, err := rt.call(ctx, "set-option", "-pu", "-t", shown.PaneID, option); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rt.call(ctx, "set-option", "-p", "-t", shown.PaneID, "@workspace_kind", "damaged"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileInterface(ctx, id); err != nil {
		t.Fatal(err)
	}
	repairedUI := waitUIStatus(t, s, id, func(status UIStatus) bool {
		return status.State == "running" && status.PaneID == shown.PaneID && status.Generation == shown.Generation
	})
	repairedPane, err := rt.Inspect(ctx, repairedUI.PaneID)
	if err != nil || repairedPane.Kind != "tui" || repairedPane.WorkspaceID != id || repairedPane.UIID != repairedUI.UIID || repairedPane.UIGeneration != repairedUI.Generation {
		t.Fatalf("managed UI metadata was not repaired from its exact runner command: pane=%+v status=%+v err=%v", repairedPane, repairedUI, err)
	}
	if pane, err := rt.Inspect(ctx, foreignPane); err != nil || pane.Dead {
		t.Fatalf("UI metadata repair stopped the adjacent foreign shell: pane=%+v err=%v", pane, err)
	}
	if _, err := rt.call(ctx, "kill-pane", "-t", foreignPane); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.call(ctx, "kill-pane", "-t", shown.PaneID); err != nil {
		t.Fatal(err)
	}
	recoveredUI := waitUIStatus(t, s, id, func(status UIStatus) bool {
		return status.State == "running" && status.PaneID != "" && status.PaneID != shown.PaneID
	})
	if recoveredUI.Generation <= shown.Generation || activePaneID(t, rt, orch.WindowID) != resumedOrchestrator.PaneID {
		t.Fatalf("lost UI pane was not recovered without changing orchestrator focus: before=%+v after=%+v", shown, recoveredUI)
	}
	recoveredTopology, err := rt.ObserveTopology(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	uiPaneCount := 0
	for _, pane := range recoveredTopology.Panes {
		if pane.Kind == "tui" && pane.WorkspaceID == id {
			uiPaneCount++
		}
	}
	if uiPaneCount != 1 {
		t.Fatalf("UI recovery created %d owned panes, want one: %+v", uiPaneCount, recoveredTopology.Panes)
	}
	if frame := waitTUIFrame(t, rt, recoveredUI.PaneID, "Fix checkout"); !strings.Contains(frame, "Current work") {
		t.Fatalf("recovered TUI did not render current work: %s", frame)
	}
	statusBeforeWindowLoss, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	workerRunsBeforeWindowLoss := 0
	for _, run := range statusBeforeWindowLoss.Runs {
		if run.SessionID == session.ID {
			workerRunsBeforeWindowLoss++
		}
	}
	orchestratorRunBeforeWindowLoss := resumedOrchestrator.CurrentRunID
	oldOrchestratorWindow := resumedOrchestrator.WindowID
	if _, err := rt.call(ctx, "kill-window", "-t", oldOrchestratorWindow); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(12 * time.Second)
	var recoveredOrchestrator Session
	for time.Now().Before(deadline) {
		status, statusErr := s.Status(ctx, id)
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		for _, current := range status.Sessions {
			if current.ID == orch.ID && current.CurrentRunID != "" && current.CurrentRunID != orchestratorRunBeforeWindowLoss && current.WindowID != "" && current.WindowID != oldOrchestratorWindow {
				recoveredOrchestrator = current
			}
		}
		workerRuns := 0
		for _, run := range status.Runs {
			if run.SessionID == session.ID {
				workerRuns++
			}
		}
		if workerRuns != workerRunsBeforeWindowLoss {
			t.Fatalf("whole-window recovery changed unrelated worker run count: before=%d after=%d", workerRunsBeforeWindowLoss, workerRuns)
		}
		if recoveredOrchestrator.ID != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if recoveredOrchestrator.ID == "" {
		t.Fatal("supervisor did not recover the orchestrator after its whole tmux window was lost")
	}
	recoveredUI = waitUIStatus(t, s, id, func(status UIStatus) bool {
		return status.State == "running" && status.PaneID != "" && status.WindowID == recoveredOrchestrator.WindowID && status.PaneID != recoveredUI.PaneID
	})
	if frame := waitTUIFrame(t, rt, recoveredUI.PaneID, "Fix checkout"); !strings.Contains(frame, "Current work") {
		t.Fatalf("TUI did not return in the recovered orchestrator window: %s", frame)
	}
	recoveredTopology, err = rt.ObserveTopology(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	uiPaneCount = 0
	for _, pane := range recoveredTopology.Panes {
		if pane.Kind == "tui" && pane.WorkspaceID == id {
			uiPaneCount++
		}
	}
	if uiPaneCount != 1 {
		t.Fatalf("whole-window recovery created %d managed UI panes, want one: %+v", uiPaneCount, recoveredTopology.Panes)
	}
	service, err := s.StartService(ctx, id, ServiceOptions{Name: "app-server", Worktree: w.ID, Argv: []string{"sh", "-c", "printf 'service-ready'; sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	pane, err := rt.Inspect(ctx, service.PaneID)
	if err != nil {
		t.Fatal(err)
	}
	if pane.WindowID != session.WindowID {
		t.Fatal("service not in worktree window")
	}
	beforeInterrupt, err := s.WorkspaceSnapshot(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var activeRunIDs, activeServiceIDs []string
	for _, run := range beforeInterrupt.Status.Runs {
		if run.Active() {
			activeRunIDs = append(activeRunIDs, run.ID)
		}
	}
	for _, activeService := range beforeInterrupt.Services {
		if activeService.Active() {
			activeServiceIDs = append(activeServiceIDs, activeService.ID)
		}
	}
	if len(activeServiceIDs) != 1 || activeServiceIDs[0] != service.ID {
		t.Fatalf("unexpected active service targets before pause-interrupt: %v", activeServiceIDs)
	}
	if _, err := s.PauseInterruptGuarded(ctx, id, "tmux-pause-interrupt", MutationGuard{
		ExpectedRevision:   beforeInterrupt.Status.Workspace.Revision,
		ExpectedRunIDs:     activeRunIDs,
		ExpectedServiceIDs: activeServiceIDs,
	}); err != nil {
		t.Fatal(err)
	}
	pausedUI, err := s.UIPaneStatus(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if pausedUI.State != "running" || pausedUI.PaneID != recoveredUI.PaneID {
		t.Fatalf("pause-interrupt stopped or replaced the managed TUI: %+v", pausedUI)
	}
	afterInterrupt, err := s.WorkspaceSnapshot(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, remainingService := range afterInterrupt.Services {
		if remainingService.ID == service.ID && remainingService.Active() {
			t.Fatalf("pause-interrupt left the service running: %+v", remainingService)
		}
	}
	for _, size := range []struct {
		width, height int
		want          string
	}{
		{width: 210, height: 18, want: "Current work"},
		{width: 120, height: 40, want: "Current work"},
		{width: 60, height: 10, want: "q"},
	} {
		if _, err := rt.call(ctx, "resize-window", "-t", recoveredOrchestrator.WindowID, "-x", strconv.Itoa(size.width), "-y", strconv.Itoa(size.height)); err != nil {
			t.Fatalf("resize %dx%d: %v", size.width, size.height, err)
		}
		deadline := time.Now().Add(3 * time.Second)
		var frame string
		var paneWidth, paneHeight int
		for time.Now().Before(deadline) {
			frame, err = rt.call(ctx, "capture-pane", "-p", "-t", recoveredUI.PaneID, "-S", "-")
			if err == nil {
				dimensions, dimensionErr := rt.call(ctx, "display-message", "-p", "-t", recoveredUI.PaneID, "#{pane_width} #{pane_height}")
				fields := strings.Fields(dimensions)
				if dimensionErr == nil && len(fields) == 2 {
					paneWidth, _ = strconv.Atoi(fields[0])
					paneHeight, _ = strconv.Atoi(fields[1])
					if strings.Contains(frame, size.want) && paneWidth > 0 && paneHeight > 0 {
						break
					}
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !strings.Contains(frame, size.want) || paneWidth <= 0 || paneHeight <= 0 {
			t.Fatalf("TUI did not render after resize to %dx%d (pane %dx%d): %s", size.width, size.height, paneWidth, paneHeight, frame)
		}
		for lineNo, line := range strings.Split(frame, "\n") {
			if width := ansi.StringWidth(line); width > paneWidth {
				t.Fatalf("resized TUI row %d exceeds pane width %d with %d columns: %q", lineNo, paneWidth, width, line)
			}
		}
	}
}

func waitUIStatus(t *testing.T, service *Service, workspaceID string, ready func(UIStatus) bool) UIStatus {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var status UIStatus
	for time.Now().Before(deadline) {
		var err error
		status, err = service.UIPaneStatus(context.Background(), workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		if ready(status) {
			return status
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("managed TUI did not reach the expected state: %+v", status)
	return status
}

func waitTUIFrame(t *testing.T, runtime Tmux, paneID, title string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var frame string
	for time.Now().Before(deadline) {
		var err error
		frame, err = runtime.call(context.Background(), "capture-pane", "-p", "-t", paneID, "-S", "-")
		if err == nil && strings.Contains(frame, title) && strings.Contains(frame, "Current work") {
			return frame
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("managed TUI did not render the requested page: %s", frame)
	return frame
}

func activeWindowID(t *testing.T, runtime Tmux, workspaceID string) string {
	t.Helper()
	output, err := runtime.call(context.Background(), "list-windows", "-t", "="+TmuxName(workspaceID), "-F", "#{window_id}\t#{window_active}")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) == 2 && fields[1] == "1" {
			return fields[0]
		}
	}
	t.Fatalf("tmux has no active window: %q", output)
	return ""
}

func activePaneID(t *testing.T, runtime Tmux, windowID string) string {
	t.Helper()
	output, err := runtime.call(context.Background(), "list-panes", "-t", windowID, "-F", "#{pane_id}\t#{pane_active}")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) == 2 && fields[1] == "1" {
			return fields[0]
		}
	}
	t.Fatalf("tmux window %s has no active pane: %q", windowID, output)
	return ""
}
