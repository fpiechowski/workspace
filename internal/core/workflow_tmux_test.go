package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// This deterministic client performs the same CLI operations a model would use.
// It exercises processes and mailboxes without consuming model-provider credits.
func TestWorkflowAgentProcess(t *testing.T) {
	if os.Getenv("WORKSPACE_SESSION_ID") == "" || os.Args[len(os.Args)-1] != "workflow-client" {
		return
	}
	call := func(args ...string) json.RawMessage {
		b, err := exec.Command("workspace", append([]string{"--json"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("CLI %v: %v: %s", args, err, b)
		}
		var response struct {
			OK   bool
			Data json.RawMessage
		}
		if err := json.Unmarshal(b, &response); err != nil || !response.OK {
			t.Fatalf("response: %s (%v)", b, err)
		}
		return response.Data
	}
	role := os.Getenv("WORKSPACE_ROLE")
	if role == "orchestrator" {
		for deadline := time.Now().Add(50 * time.Second); time.Now().Before(deadline); {
			var messages []Message
			if err := json.Unmarshal(call("inbox", "list"), &messages); err != nil {
				t.Fatal(err)
			}
			for _, m := range messages {
				if m.HandoffID != "" {
					call("handoff", "accept", m.HandoffID)
				} else {
					call("inbox", "ack", m.ID)
				}
			}
			var status Status
			if err := json.Unmarshal(call("status"), &status); err != nil {
				t.Fatal(err)
			}
			if status.Workspace.Status == "completed" {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("orchestrator timeout")
	}
	cwd, _ := os.Getwd()
	ctx := context.Background()
	if role == "implementer" {
		if err := os.WriteFile("fix.txt", []byte("fixed by actual worker process\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := git(ctx, cwd, "add", "fix.txt"); err != nil {
			t.Fatal(err)
		}
		if _, err := git(ctx, cwd, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fix issue"); err != nil {
			t.Fatal(err)
		}
	}
	if role == "integrator" {
		var manifest Integration
		if err := readJSON(filepath.Join(os.Getenv("WORKSPACE_DIR"), "integration", "manifest.json"), &manifest); err != nil {
			t.Fatal(err)
		}
		for _, head := range manifest.Heads {
			if _, err := git(ctx, cwd, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "merge", "--no-ff", "--no-edit", head); err != nil {
				t.Fatal(err)
			}
		}
	}
	if role == "tester" {
		b, err := os.ReadFile("fix.txt")
		if err != nil || string(b) != "fixed by actual worker process\n" {
			t.Fatal("integrated code missing")
		}
	}
	name := map[string]string{"planner": "PLAN.md", "implementer": "IMPLEMENTATION.md", "integrator": "INTEGRATION.md", "tester": "LIVE_TEST.md"}[role]
	if name == "" {
		t.Fatal("unexpected role", role)
	}
	if err := atomicWrite(filepath.Join("work-products", name), []byte("Result from "+role+" process\n")); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join("work-products", "SUMMARY.md"), []byte("Task completed")); err != nil {
		t.Fatal(err)
	}
	var receipt CheckReceipt
	if err := json.Unmarshal(call("check", "run", "--operation-key", "check:"+os.Getenv("WORKSPACE_SESSION_ID"), "--", "git", "diff", "--exit-code", "HEAD"), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.ExitCode != 0 {
		t.Fatal("check failed")
	}
	args := []string{"handoff", "submit", "--to-session", os.Getenv("WORKSPACE_PARENT_SESSION_ID"), "--task", os.Getenv("WORKSPACE_TASK_ID"), "--summary-file", "work-products/SUMMARY.md", "--artifact", "work-products/" + name, "--check", receipt.ID, "--operation-key", "result:" + os.Getenv("WORKSPACE_SESSION_ID")}
	var h1, h2 Handoff
	if err := json.Unmarshal(call(args...), &h1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(call(args...), &h2); err != nil {
		t.Fatal(err)
	}
	if h1.ID != h2.ID {
		t.Fatal("handoff replay duplicated result")
	}
	fmt.Println("handoff", h1.ID)
}

func TestTmuxCompleteIssueWorkflow(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getenv("WORKSPACE_TMUX_TEST") != "1" {
		t.Skip("requires opt-in Linux/macOS tmux")
	}
	s, ws := fixture(t)
	ctx := context.Background()
	socket := "workflow-" + ID("test")
	rt := Tmux{Socket: socket}
	s.Runtime = rt
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	bin := filepath.Join(t.TempDir(), "workspace")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/workspace")
	cmd.Dir = filepath.Join("..", "..")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, b)
	}
	s.Executable = bin
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	cfg.Clients["test"] = Client{Adapter: "command", LaunchArgv: []string{exe, "-test.run=TestWorkflowAgentProcess", "--", "{prompt_file}", "workflow-client"}}
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSupervisor(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.StopSupervisor(ctx) })
	orch, err := s.StartOrchestrator(ctx, ws, "orch")
	if err != nil {
		t.Fatal(err)
	}
	wait := func(taskID, sessionID string) {
		t.Helper()
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
			v, err := s.Status(ctx, ws)
			if err != nil {
				t.Fatal(err)
			}
			accepted := false
			ended := false
			for _, task := range v.Workspace.Tasks {
				if task.ID == taskID {
					accepted = task.State == "accepted"
				}
			}
			for _, p := range v.Sessions {
				if p.ID == sessionID {
					ended = !p.Active()
					if p.State == "failed" {
						output, _ := rt.call(ctx, "capture-pane", "-p", "-t", p.PaneID, "-S", "-")
						t.Fatalf("worker failed: %s %s", p.Error, output)
					}
				}
			}
			if accepted && ended {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		output, _ := rt.call(ctx, "capture-pane", "-p", "-t", orch.PaneID, "-S", "-")
		t.Fatalf("task timeout %s; orchestrator %s", taskID, output)
	}
	plan := plannedTask(t, s, ws, "planning", "planner", nil)
	pp, _ := startTask(t, s, ws, plan)
	wait(plan.ID, pp.ID)
	advancePhase(t, s, ws, "plan_review")
	impl := plannedTask(t, s, ws, "implementation", "implementer", []string{plan.ID})
	advancePhase(t, s, ws, "implementing")
	ip, _ := startTask(t, s, ws, impl)
	wait(impl.ID, ip.ID)
	advancePhase(t, s, ws, "integrating")
	i, err := s.PrepareIntegration(ctx, ws, IntegrationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	it := plannedTask(t, s, ws, "integration", "integrator", []string{impl.ID})
	ia, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "integrator", Role: "integrator"})
	if err != nil {
		t.Fatal(err)
	}
	is, err := s.StartSession(ctx, ws, SessionOptions{Agent: ia.ID, Task: it.ID, Worktree: i.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	wait(it.ID, is.ID)
	advancePhase(t, s, ws, "change_requests")
	s.Forge = &fakeForge{}
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishChangeRequest(ctx, ws, cr.ID, false); err != nil {
		t.Fatal(err)
	}
	advancePhase(t, s, ws, "live_test_offer")
	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	d := menu.PendingDecision
	if _, err := s.AnswerDecision(ctx, ws, DecisionAnswer{ID: d.ID, Answer: "run", ExpectedRevision: d.Revision, Environment: "local fixture checkout"}); err != nil {
		t.Fatal(err)
	}
	test := plannedTask(t, s, ws, "live-test", "tester", []string{it.ID})
	ta, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "tester", Role: "tester"})
	if err != nil {
		t.Fatal(err)
	}
	ts, err := s.StartSession(ctx, ws, SessionOptions{Agent: ta.ID, Task: test.ID, Worktree: i.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	wait(test.ID, ts.ID)
	if ts.WindowID != is.WindowID || ts.ID == is.ID {
		t.Fatal("tester must use separate session in integrated worktree window")
	}
	advancePhase(t, s, ws, "awaiting_release")
	v, err := s.ConfirmRelease(ctx, ws, "fixture-release-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if v.Workspace.Status != "completed" || len(v.Workspace.Tasks) != 4 {
		t.Fatal("workflow not complete")
	}
}
