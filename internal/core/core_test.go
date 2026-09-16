package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

type fakeRuntime struct {
	launches     int
	panes        map[string]Pane
	inspectError error
}

func (r *fakeRuntime) Launch(_ context.Context, l Launch) (Pane, error) {
	r.launches++
	p := Pane{ID: fmt.Sprintf("%%%d", r.launches), WindowID: "@1", SessionID: l.SessionID, RunID: l.RunID, WorkspaceID: l.WorkspaceID}
	r.panes[p.ID] = p
	return p, nil
}
func (r *fakeRuntime) Inspect(_ context.Context, id string) (Pane, error) {
	if r.inspectError != nil {
		return Pane{}, r.inspectError
	}
	p, ok := r.panes[id]
	if !ok {
		return Pane{}, fail("pane_missing", "missing pane")
	}
	return p, nil
}
func (r *fakeRuntime) Stop(_ context.Context, id string) error { delete(r.panes, id); return nil }
func (r *fakeRuntime) StopWorkspace(_ context.Context, workspaceID string) error {
	for id, pane := range r.panes {
		if pane.WorkspaceID == workspaceID {
			delete(r.panes, id)
		}
	}
	return nil
}
func (r *fakeRuntime) Attach(context.Context, string, string) error { return nil }

func fixture(t *testing.T) (*Service, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
		if _, err := git(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := InitProject(ctx, dir); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Root: dir, Runtime: &fakeRuntime{panes: map[string]Pane{}}, Executable: exe}
	configure(t, s, exe)
	w, err := s.Create(ctx, CreateOptions{Title: "Fix checkout", Input: "A reproducible issue", Workflow: "issue-resolution"})
	if err != nil {
		t.Fatal(err)
	}
	return s, w.Workspace.ID
}
func configure(t *testing.T, s *Service, exe string) {
	t.Helper()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clients = map[string]Client{"test": {Adapter: "command", LaunchArgv: []string{exe, "-test.run=TestWorkerProcess", "--", "{prompt_file}"}}}
	cfg.Profiles = map[string]Profile{}
	cfg.Defaults.OrchestratorProfile = "frontier"
	for _, name := range []string{"frontier", "implementation", "live-testing"} {
		cfg.Profiles[name] = Profile{Routes: []Route{{ID: name + "-a", Client: "test", Provider: "a", Model: "test-model", MaxConcurrency: 8}}}
	}
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
}
func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("wanted %s, got %v", code, err)
	}
}
func worker(t *testing.T, s *Service, id, name string) (Agent, Worktree) {
	t.Helper()
	ctx := context.Background()
	a, err := s.CreateAgent(ctx, id, AgentOptions{Name: name, Role: "planner", Instructions: "Investigate checkout retries"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.CreateWorktree(ctx, id, WorktreeOptions{Name: name, Purpose: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	return a, w
}

func TestInitPreservesConfigAndTemplates(t *testing.T) {
	s, _ := fixture(t)
	before, err := os.ReadFile(filepath.Join(s.Root, ".workspace", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tpl := filepath.Join(s.Root, ".workspace", "templates", "WORKSPACE.md.tmpl")
	if err := os.WriteFile(tpl, []byte("custom"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InitProject(context.Background(), s.Root); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, ".workspace", "config.yaml"))
	if !bytes.Equal(before, after) {
		t.Fatal("init overwrote config")
	}
	b, _ := os.ReadFile(tpl)
	if string(b) != "custom" {
		t.Fatal("init overwrote customized template")
	}
	if _, err := git(context.Background(), s.Root, "check-ignore", ".workspace/templates/WORKSPACE.md.tmpl"); err == nil {
		t.Fatal("templates must remain versionable")
	}
}

func TestInitRepairsMissingTemplatesWithoutReplacingOverrides(t *testing.T) {
	s, _ := fixture(t)
	path := filepath.Join(s.Root, ".workspace", "templates", "worker.AGENTS.md.tmpl")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	override := filepath.Join(s.Root, ".workspace", "templates", "orchestrator.AGENTS.md.tmpl")
	if err := atomicWrite(override, []byte("Project-specific instructions")); err != nil {
		t.Fatal(err)
	}
	if _, err := InitProject(context.Background(), s.Root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("missing template not restored", err)
	}
	b, err := os.ReadFile(override)
	if err != nil || string(b) != "Project-specific instructions" {
		t.Fatal("project instructions overwritten")
	}
}
func TestCreateIdempotencyAndWorkflowSelection(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	opt := CreateOptions{Title: "Second issue", Input: "Details", OperationKey: "issue-2"}
	a, err := s.Create(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	if a.Workspace.ID != b.Workspace.ID {
		t.Fatal("duplicate workspace")
	}
	if a.Workspace.Workflow != nil || a.Workspace.Status != "needs_workflow" {
		t.Fatal("implicit workflow selection")
	}
	opt.Input = "changed"
	_, err = s.Create(ctx, opt)
	expectCode(t, err, "operation_conflict")
	c, err := s.SelectWorkflow(ctx, a.Workspace.ID, "issue-resolution")
	if err != nil {
		t.Fatal(err)
	}
	if c.Workspace.Workflow == nil || c.Workspace.Status != "active" {
		t.Fatal("workflow not selected")
	}
	_, err = s.Create(ctx, CreateOptions{Source: "https://tracker.example/1"})
	expectCode(t, err, "tracker_unavailable")
}
func TestConcurrentLaunchIsIdempotentAndWorktreeExclusive(t *testing.T) {
	s, id := fixture(t)
	a, w := worker(t, s, id, "planner")
	ctx := context.Background()
	opt := SessionOptions{Agent: a.ID, Worktree: w.ID, OperationKey: "plan:1"}
	var wg sync.WaitGroup
	results := make(chan Session, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); session, err := s.StartSession(ctx, id, opt); results <- session; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	first := ""
	for r := range results {
		if first == "" {
			first = r.ID
		}
		if r.ID != first {
			t.Fatal("duplicate concrete session")
		}
	}
	if s.Runtime.(*fakeRuntime).launches != 1 {
		t.Fatal("duplicate tmux launch")
	}
	_, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	expectCode(t, err, "agent_busy")
	a2, err := s.CreateAgent(ctx, id, AgentOptions{Name: "another-planner", Role: "planner"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.StartSession(ctx, id, SessionOptions{Agent: a2.ID, Worktree: w.ID})
	expectCode(t, err, "worktree_busy")
	opt.Profile = "implementation"
	_, err = s.StartSession(ctx, id, opt)
	expectCode(t, err, "operation_conflict")
}
func TestConcreteSessionCarriesIdentityAndPreservesPersona(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, id, "planner")
	first, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := s.ExecuteSession(ctx, id, first.ID, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("%v: %s", err, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("%v: %s", err, stdout.String())
	}
	if result["agent"] != a.ID || result["session"] != first.ID || result["run"] != first.CurrentRunID || result["parent"] != first.ParentAgentID || result["worktree"] != w.ID {
		t.Fatalf("identity not delivered: %v", result)
	}
	if !strings.Contains(result["prompt"], "Investigate checkout retries") {
		t.Fatal("persona instructions absent")
	}
	status, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if status.Sessions[0].State != "exited" || status.Workspace.Workflow.Phase != "planning" {
		t.Fatal("process exit must not advance workflow")
	}
	second, err := s.ResumeAgent(ctx, id, a.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.CurrentRunID == first.CurrentRunID || second.AgentID != first.AgentID {
		t.Fatal("resume must create a new Run in the same logical Session")
	}
	if second.AgentSnapshot != first.AgentSnapshot {
		t.Fatal("persona changed")
	}
	err = s.ExecuteSession(ctx, id, first.CurrentRunID, strings.NewReader(""), io.Discard, io.Discard)
	expectCode(t, err, "stale_run")
}

// Executed in a child process by the command adapter, never in the normal suite.
func TestWorkerProcess(t *testing.T) {
	if os.Getenv("WORKSPACE_SESSION_ID") == "" {
		return
	}
	path := os.Args[len(os.Args)-1]
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result := map[string]string{"agent": os.Getenv("WORKSPACE_AGENT_ID"), "session": os.Getenv("WORKSPACE_SESSION_ID"), "run": os.Getenv("WORKSPACE_RUN_ID"), "parent": os.Getenv("WORKSPACE_PARENT_AGENT_ID"), "worktree": os.Getenv("WORKSPACE_WORKTREE_ID"), "prompt": string(b)}
	data, _ := json.Marshal(result)
	if err := atomicWrite(filepath.Join("work-products", "identity.json"), data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
	os.Exit(0)
}
func TestRecoveryAndExternalEditDetection(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	status, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	err = s.With(ctx, id, func(d *Document) error {
		before := d.Registry.WorkspaceDigest
		d.State.Revision++
		d.State.Status = "paused"
		b, err := encodeDocument(d)
		if err != nil {
			return err
		}
		d.Registry.WorkspaceDigest = digest(b)
		if err := writeJSON(filepath.Join(d.Dir, ".runtime", "pending.json"), pendingWrite{BeforeDigest: before, Document: b, Registry: d.Registry}); err != nil {
			return err
		}
		// Simulate power loss after document replace, before runtime index replacement.
		return atomicWrite(filepath.Join(d.Dir, "WORKSPACE.md"), b)
	})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Workspace.Status != "paused" || recovered.Workspace.Revision != status.Workspace.Revision+1 {
		t.Fatal("pending transaction was not recovered")
	}
	path := filepath.Join(status.Directory, "WORKSPACE.md")
	b, _ := os.ReadFile(path)
	b = append(b, []byte("\nmanual edit\n")...)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateAgent(ctx, id, AgentOptions{Name: "unexpected", Role: "planner"})
	expectCode(t, err, "revision_conflict")
}
func TestRoutingBalancesProvidersAcrossWorkspaces(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profiles["frontier"] = Profile{Routes: []Route{{ID: "a1", Client: "test", Provider: "a", Model: "model-1", MaxConcurrency: 8}, {ID: "a2", Client: "test", Provider: "a", Model: "model-2", MaxConcurrency: 8}, {ID: "b1", Client: "test", Provider: "b", Model: "model-3", MaxConcurrency: 8}}}
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	first, err := s.StartOrchestrator(ctx, id, "orch:1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, CreateOptions{Title: "Other", Input: "Another issue", Workflow: "issue-resolution"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.StartOrchestrator(ctx, other.Workspace.ID, "orch:1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Route.Provider != "a" || second.Route.Provider != "b" {
		t.Fatalf("providers not balanced: %s %s", first.Route.Provider, second.Route.Provider)
	}
}
func TestPauseAndReconcileDoNotLoseWork(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, id, "planner")
	session, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPaused(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	_, err = s.StartOrchestrator(ctx, id, "")
	expectCode(t, err, "workspace_paused")
	delete(s.Runtime.(*fakeRuntime).panes, session.PaneID)
	status, err := s.Reconcile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if status.Sessions[0].State != "interrupted" {
		t.Fatal("dead pane did not interrupt session")
	}
	if _, err := os.Stat(w.Path); err != nil {
		t.Fatal("reconcile removed worktree")
	}
}
func TestWorktreeFrozenBaseAndPathValidation(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	before, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, s.Root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "advance"); err != nil {
		t.Fatal(err)
	}
	w, err := s.CreateWorktree(ctx, id, WorktreeOptions{Name: "frozen"})
	if err != nil {
		t.Fatal(err)
	}
	if w.BaseCommit != before.Workspace.Base.Commit {
		t.Fatal("default base moved with HEAD")
	}
	_, err = s.CreateWorktree(ctx, id, WorktreeOptions{Name: "../escape"})
	expectCode(t, err, "invalid_name")
}

func TestRuntimeOutageDoesNotReleaseReservation(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, id, "planner")
	session, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	s.Runtime.(*fakeRuntime).inspectError = fail("tmux_error", "server unavailable")
	_, err = s.Reconcile(ctx, id)
	expectCode(t, err, "tmux_error")
	status, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if status.Sessions[0].ID != session.ID || !status.Sessions[0].Active() {
		t.Fatal("runtime outage released an uncertain active session")
	}
}
