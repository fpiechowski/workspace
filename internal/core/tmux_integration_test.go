package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
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
	orch, err := s.StartOrchestrator(ctx, id, "tmux-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	waitExited(orch.ID)
	if orch.WorktreeID != "" || orch.CWD == w.Path {
		t.Fatal("orchestrator must run from workspace directory")
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
	if _, err := s.StopService(ctx, id, service.ID); err != nil {
		t.Fatal(err)
	}
}
