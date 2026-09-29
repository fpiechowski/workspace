package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- slug generation and validation -----------------------------------------

func TestSlugifyNormalizationTable(t *testing.T) {
	cases := []struct{ source, want string }{
		{"Fix checkout", "fix-checkout"},
		{"Hello, World!", "hello-world"},
		{"  Trims -- hyphens  ", "trims-hyphens"},
		{"Zażółć gęślą jaźń", "zazolc-gesla-jazn"},
		{"Straße", "strasse"},
		{"Ærøskøbing", "aeroskobing"},
		{"Łódź", "lodz"},
		{"123 abc", "123-abc"},
		{"already-slug", "already-slug"},
		{"/* punctuation only */", "punctuation-only"},
	}
	for _, tc := range cases {
		if got := slugify(tc.source, slugBaseMax); got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestSlugFallbacksAndReservedShapes(t *testing.T) {
	if got := slugify("😀😀", slugBaseMax); got != "" {
		t.Fatalf("emoji-only slugify = %q, want empty before fallback", got)
	}
	for kind, want := range map[string]string{"ws": "workspace", "agent": "agent", "sess": "session", "task": "task"} {
		if got := baseSlug(kind, "😀😀"); got != want {
			t.Errorf("baseSlug(%q, emoji) = %q, want %q", kind, got, want)
		}
		if got := baseSlug(kind, "   "); got != want {
			t.Errorf("baseSlug(%q, spaces) = %q, want %q", kind, got, want)
		}
	}
	if got := baseSlug("sess", "01arz3ndektsv4rrffq69g5fav"); got != "session" {
		t.Fatalf("reserved lowercase ULID not replaced: %q", got)
	}
	if got := baseSlug("agent", "0123456789abcdef01234567"); got != "agent" {
		t.Fatalf("reserved migrated-hex shape not replaced: %q", got)
	}
}

func TestSlugTruncationAtHyphenBoundary(t *testing.T) {
	got := slugify(strings.Repeat("alpha-", 10)+"omega", slugBaseMax)
	if len(got) > slugBaseMax {
		t.Fatalf("slug length %d exceeds %d: %q", len(got), slugBaseMax, got)
	}
	if strings.HasSuffix(got, "-") || strings.Contains(got, "--") {
		t.Fatalf("truncated slug has a dangling or doubled hyphen: %q", got)
	}
	if got := slugify(strings.Repeat("a", 50), slugBaseMax); got != strings.Repeat("a", 40) {
		t.Fatalf("word-only truncation = %q", got)
	}
}

func TestParseExplicitID(t *testing.T) {
	valid := []struct{ kind, value, want string }{
		{"ws", "custom", "custom"},
		{"ws", "ws_custom", "custom"},
		{"ws", "  ws_custom  ", "custom"},
		{"agent", "agent_planner-2", "planner-2"},
		{"task", "task_123abc", "123abc"},
		{"sess", "my-session", "my-session"},
	}
	for _, tc := range valid {
		got, err := parseExplicitID(tc.kind, tc.value)
		if err != nil || got != tc.want {
			t.Errorf("parseExplicitID(%q, %q) = %q, %v; want %q", tc.kind, tc.value, got, err, tc.want)
		}
	}
	invalid := []struct{ kind, value string }{
		{"ws", ""},
		{"ws", "agent_foo"},
		{"ws", "ws_Foo"},
		{"ws", "ws_foo_bar"},
		{"ws", "ws_foo/bar"},
		{"ws", "ws_.."},
		{"ws", strings.Repeat("a", 49)},
		{"ws", "01arz3ndektsv4rrffq69g5fav"},
		{"ws", "0123456789abcdef01234567"},
		{"ws", "-leading"},
		{"ws", "trailing-"},
	}
	for _, tc := range invalid {
		if _, err := parseExplicitID(tc.kind, tc.value); err == nil {
			t.Errorf("parseExplicitID(%q, %q) accepted an invalid id", tc.kind, tc.value)
		} else {
			expectCode(t, err, "invalid_id")
		}
	}
}

// --- reservation ledger ------------------------------------------------------

func TestAllocatorCollisionsAreDeterministic(t *testing.T) {
	storage := t.TempDir()
	alloc := idAllocator{storage: storage, kind: "agent", projectID: "prj", workspaceID: "ws_a"}
	first, err := alloc.allocate("agent", "planner", nil)
	if err != nil || first != "agent_planner" {
		t.Fatalf("first = %q, %v", first, err)
	}
	second, err := alloc.allocate("agent", "planner", nil)
	if err != nil || second != "agent_planner-2" {
		t.Fatalf("second = %q, %v", second, err)
	}
	third, err := alloc.allocate("agent", "planner", nil)
	if err != nil || third != "agent_planner-3" {
		t.Fatalf("third = %q, %v", third, err)
	}
	if first == second || second == third {
		t.Fatal("collisions were not suffixed")
	}
}

func TestAllocatorKeyedReuseAndDifferentKeySuffix(t *testing.T) {
	storage := t.TempDir()
	keyed := idAllocator{storage: storage, kind: "ws", projectID: "prj", operation: reservationOperation("prj", "", "ws", "key-1")}
	first, err := keyed.allocate("ws", "retry", nil)
	if err != nil || first != "ws_retry" {
		t.Fatalf("first = %q, %v", first, err)
	}
	replay, err := keyed.allocate("ws", "retry", nil)
	if err != nil || replay != first {
		t.Fatalf("same key did not reuse the reservation: %q, %v", replay, err)
	}
	other := idAllocator{storage: storage, kind: "ws", projectID: "prj", operation: reservationOperation("prj", "", "ws", "key-2")}
	next, err := other.allocate("ws", "retry", nil)
	if err != nil || next != "ws_retry-2" {
		t.Fatalf("different key = %q, %v; want ws_retry-2", next, err)
	}
}

func TestAllocatorExplicitIDExists(t *testing.T) {
	storage := t.TempDir()
	alloc := idAllocator{storage: storage, kind: "ws", projectID: "prj"}
	id, err := alloc.allocateExplicit("ws", "custom", nil)
	if err != nil || id != "ws_custom" {
		t.Fatalf("explicit = %q, %v", id, err)
	}
	if _, err := alloc.allocateExplicit("ws", "custom", nil); err == nil {
		t.Fatal("duplicate explicit id accepted")
	} else {
		expectCode(t, err, "id_exists")
	}
	if _, err := alloc.allocateExplicit("ws", "other", func(string) bool { return true }); err == nil {
		t.Fatal("taken callback ignored")
	} else {
		expectCode(t, err, "id_exists")
	}
}

func TestAllocatorConcurrentGoroutines(t *testing.T) {
	storage := t.TempDir()
	const n = 32
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			alloc := idAllocator{storage: storage, kind: "agent", projectID: "prj", workspaceID: "ws_x"}
			ids[i], errs[i] = alloc.allocate("agent", "worker", nil)
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if seen[ids[i]] {
			t.Fatalf("duplicate id %q", ids[i])
		}
		seen[ids[i]] = true
	}
}

func TestAllocatorDoesNotReuseReservedShape(t *testing.T) {
	storage := t.TempDir()
	alloc := idAllocator{storage: storage, kind: "agent", projectID: "prj"}
	id, err := alloc.allocate("agent", "01arz3ndektsv4rrffq69g5fav", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(id, "01arz3ndektsv4rrffq69g5fav") {
		t.Fatalf("reserved shape returned un-suffixed: %q", id)
	}
}

// --- Service-level adoption --------------------------------------------------

func TestWorkspaceSlugDerivationAndNoReuseAfterDelete(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	if ws != "ws_fix-checkout" {
		t.Fatalf("workspace id = %q, want ws_fix-checkout", ws)
	}
	if err := s.DeleteWorkspace(ctx, ws, "delete:fixture", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".workspace", ws)); !os.IsNotExist(err) {
		t.Fatalf("workspace directory survived deletion: %v", err)
	}
	again, err := s.Create(ctx, CreateOptions{Title: "Fix checkout", Input: "again", Workflow: "extended"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Workspace.ID != "ws_fix-checkout-2" {
		t.Fatalf("recreated workspace id = %q, want ws_fix-checkout-2 (reservations must never be reused)", again.Workspace.ID)
	}
}

func TestAgentSessionTaskSlugsAcrossWorkspaces(t *testing.T) {
	s, ws1 := fixture(t)
	ctx := context.Background()
	a1, w1 := worker(t, s, ws1, "planner")
	if a1.ID != "agent_planner" {
		t.Fatalf("agent id = %q, want agent_planner", a1.ID)
	}
	p1, err := s.StartSession(ctx, ws1, SessionOptions{Agent: a1.ID, Worktree: w1.ID, OperationKey: "sess:1"})
	if err != nil {
		t.Fatal(err)
	}
	if p1.ID != "sess_planner" {
		t.Fatalf("session id = %q, want sess_planner", p1.ID)
	}

	second, err := s.Create(ctx, CreateOptions{Title: "Second workspace", Input: "x", Workflow: "extended"})
	if err != nil {
		t.Fatal(err)
	}
	ws2 := second.Workspace.ID
	a2, err := s.CreateAgent(ctx, ws2, AgentOptions{Name: "planner", Role: "planner", Instructions: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if a2.ID != "agent_planner-2" {
		t.Fatalf("second workspace agent id = %q, want agent_planner-2", a2.ID)
	}
	w2, err := s.CreateWorktree(ctx, ws2, WorktreeOptions{Name: "planner", Purpose: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.StartSession(ctx, ws2, SessionOptions{Agent: a2.ID, Worktree: w2.ID, OperationKey: "sess:2"})
	if err != nil {
		t.Fatal(err)
	}
	if p2.ID != "sess_planner-2" {
		t.Fatalf("second workspace session id = %q, want sess_planner-2", p2.ID)
	}
}

func TestTaskSlugDoesNotReuseSoftDeletedID(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	spec := TaskSpec{Title: "Alpha task", Goal: "Bounded", Role: "planner", AcceptanceCriteria: []string{"result"}}
	first, err := s.CreateTask(ctx, ws, spec, "task:alpha:1")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "task_alpha-task" {
		t.Fatalf("task id = %q, want task_alpha-task", first.ID)
	}
	if _, err := s.DeleteTask(ctx, ws, first.ID, "delete:alpha", MutationGuard{}); err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateTask(ctx, ws, spec, "task:alpha:2")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != "task_alpha-task-2" {
		t.Fatalf("task after soft delete = %q, want task_alpha-task-2", second.ID)
	}
}

func TestExplicitWorkspaceAndAgentIDs(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	ws, err := s.Create(ctx, CreateOptions{Title: "Custom", Input: "x", Workflow: "extended", ID: "custom-ws"})
	if err != nil {
		t.Fatal(err)
	}
	if ws.Workspace.ID != "ws_custom-ws" {
		t.Fatalf("explicit workspace id = %q", ws.Workspace.ID)
	}
	if _, err := s.Create(ctx, CreateOptions{Title: "Dup", Input: "x", Workflow: "extended", ID: "ws_custom-ws"}); err == nil {
		t.Fatal("duplicate explicit workspace accepted")
	} else {
		expectCode(t, err, "id_exists")
	}
	if _, err := s.Create(ctx, CreateOptions{Title: "Bad", Input: "x", Workflow: "extended", ID: "agent_bad"}); err == nil {
		t.Fatal("wrong explicit prefix accepted")
	} else {
		expectCode(t, err, "invalid_id")
	}

	agent, err := s.CreateAgent(ctx, ws.Workspace.ID, AgentOptions{Name: "worker-one", Role: "planner", Instructions: "x", ID: "custom-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if agent.ID != "agent_custom-agent" {
		t.Fatalf("explicit agent id = %q", agent.ID)
	}
	if _, err := s.CreateAgent(ctx, ws.Workspace.ID, AgentOptions{Name: "worker-two", Role: "planner", Instructions: "x", ID: "custom-agent"}); err == nil {
		t.Fatal("duplicate explicit agent accepted")
	} else {
		expectCode(t, err, "id_exists")
	}
}

func TestExplicitTaskAndSessionIDs(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	spec := TaskSpec{Title: "Explicit task", Goal: "Bounded", Role: "planner", AcceptanceCriteria: []string{"result"}}
	task, err := s.CreateTaskWithID(ctx, ws, spec, "custom-task", "task:custom:1")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "task_custom-task" {
		t.Fatalf("explicit task id = %q", task.ID)
	}
	if _, err := s.CreateTaskWithID(ctx, ws, spec, "task_custom-task", "task:custom:2"); err == nil {
		t.Fatal("duplicate explicit task accepted")
	} else {
		expectCode(t, err, "id_exists")
	}

	a, w := worker(t, s, ws, "planner")
	session, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID, ID: "custom-sess", OperationKey: "sess:custom"})
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != "sess_custom-sess" {
		t.Fatalf("explicit session id = %q", session.ID)
	}
	// Terminate the run so the session can be resumed; an explicit id must then
	// be refused because it cannot apply to an existing logical session.
	if err := s.With(ctx, ws, func(d *Document) error {
		p, _ := findSession(d, session.ID)
		r, _ := findRun(d, p.CurrentRunID)
		r.State = "exited"
		now := nowUTC()
		r.FinishedAt = &now
		p.CurrentRunID = ""
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID, ResumeSession: session.ID, ID: "other", OperationKey: "sess:resume"}); err == nil {
		t.Fatal("explicit id accepted on resume")
	} else {
		expectCode(t, err, "invalid_id")
	}
}

func TestKeyedWorkspaceCreateReusesReservationAfterFailure(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	template := filepath.Join(s.Root, ".workspace", "templates", "orchestrator.AGENTS.md.tmpl")
	original, err := os.ReadFile(template)
	if err != nil {
		t.Fatal(err)
	}
	opt := CreateOptions{Title: "Reserved retry", Input: "x", Workflow: "extended", OperationKey: "reserve-retry"}
	if err := os.Remove(template); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, opt); err == nil {
		t.Fatal("expected the create to fail after reserving the id")
	}
	if err := os.WriteFile(template, original, 0600); err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	if v.Workspace.ID != "ws_reserved-retry" {
		t.Fatalf("keyed retry id = %q, want ws_reserved-retry (same id after a reserved failure)", v.Workspace.ID)
	}
	next, err := s.Create(ctx, CreateOptions{Title: "Reserved retry", Input: "x", Workflow: "extended", OperationKey: "reserve-retry-2"})
	if err != nil {
		t.Fatal(err)
	}
	if next.Workspace.ID != "ws_reserved-retry-2" {
		t.Fatalf("different key id = %q, want ws_reserved-retry-2", next.Workspace.ID)
	}
}

func TestSharedStorageProjectsGetDistinctSlugs(t *testing.T) {
	a, _ := fixture(t)
	b, _ := fixture(t)
	ctx := context.Background()
	root := t.TempDir()
	services := []*Service{a, b}
	for _, s := range services {
		cfg, err := s.Config()
		if err != nil {
			t.Fatal(err)
		}
		cfg.WorkspacesDir = root
		data, err := yaml.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), data); err != nil {
			t.Fatal(err)
		}
	}
	// Both projects allocate the same slug concurrently against one external
	// storage root; O_EXCL must hand out distinct ids without errors.
	ids := make([]string, len(services))
	errs := make([]error, len(services))
	var wg sync.WaitGroup
	for i, s := range services {
		wg.Add(1)
		go func(i int, s *Service) {
			defer wg.Done()
			v, err := s.Create(ctx, CreateOptions{Title: "Same title", Input: "x", Workflow: "extended"})
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = v.Workspace.ID
		}(i, s)
	}
	wg.Wait()
	for i := range services {
		if errs[i] != nil {
			t.Fatalf("service %d: %v", i, errs[i])
		}
	}
	if ids[0] == ids[1] {
		t.Fatalf("shared storage returned a duplicate id %q", ids[0])
	}
	got := append([]string(nil), ids...)
	sort.Strings(got)
	if got[0] != "ws_same-title" || got[1] != "ws_same-title-2" {
		t.Fatalf("shared storage ids = %v, want ws_same-title and ws_same-title-2", got)
	}
}

func TestNewOptionFieldsDoNotChangeExistingDigests(t *testing.T) {
	agentJSON, _ := json.Marshal(AgentOptions{Name: "a", Role: "planner", PromptTemplate: "planning", Instructions: "i"})
	if strings.Contains(string(agentJSON), `"ID"`) {
		t.Fatalf("empty AgentOptions.ID leaked into the digest payload: %s", agentJSON)
	}
	sessionJSON, _ := json.Marshal(SessionOptions{Agent: "a"})
	if strings.Contains(string(sessionJSON), `"ID"`) {
		t.Fatalf("empty SessionOptions.ID leaked into the digest payload: %s", sessionJSON)
	}
	createJSON, _ := json.Marshal(CreateOptions{Title: "t", Input: "i"})
	if strings.Contains(string(createJSON), `"ID":`) {
		t.Fatalf("empty CreateOptions.ID leaked into the digest payload: %s", createJSON)
	}
}

// --- legacy compatibility ----------------------------------------------------

func installLegacyWorkspace(t *testing.T, s *Service) (wsID, agentID, taskID, sessionID, runID string) {
	t.Helper()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	storage, err := s.storageRoot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	base, err := git(context.Background(), s.Root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	wsID = ID("ws")
	agentID = ID("agent")
	taskID = ID("task")
	sessionID = migratedSessionID("legacy-fixture")
	runID = ID("run")
	dir := filepath.Join(storage, wsID)
	for _, sub := range []string{"inputs", "prompts", "tasks", "artifacts", "worktrees", ".runtime"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := atomicWrite(filepath.Join(dir, "inputs", "issue.md"), []byte("Legacy issue")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"orchestrator", "planning", "implementation", "integration", "live-testing"} {
		if err := atomicWrite(filepath.Join(dir, "prompts", name+".md.tmpl"), []byte("Legacy prompt")); err != nil {
			t.Fatal(err)
		}
	}
	now := nowUTC()
	agent := Agent{ID: agentID, Name: "orchestrator", Role: "orchestrator", PromptTemplate: "orchestrator", Scope: "workspace"}
	d := &Document{Dir: dir, Body: "# Legacy\n"}
	d.State = Workspace{
		SchemaVersion: 1, ID: wsID, ProjectID: cfg.ProjectID, ProjectRoot: s.Root,
		Title: "Legacy workspace", Revision: 1, Status: "active",
		Input:               Input{Snapshot: "inputs/issue.md"},
		Base:                Base{Ref: "master", Commit: base},
		CreatedAt:           now,
		OrchestratorAgentID: agentID,
		Tasks: []Task{{
			TaskSpec: TaskSpec{Name: "legacy-task", Title: "Legacy task", Goal: "Bounded", Role: "planner", AcceptanceCriteria: []string{"result"}},
			ID:       taskID, State: "pending", Attempt: 1,
		}},
	}
	d.Registry = Registry{
		SchemaVersion: registrySchemaVersion,
		Agents:        []Agent{agent},
		Sessions: []Session{{
			ID: sessionID, AgentID: agentID, AgentSnapshot: agent,
			CreatedAt: now, LifecycleState: "idle",
		}},
		Runs:       []Run{{ID: runID, SessionID: sessionID, State: "exited", CreatedAt: now}},
		Operations: map[string]Operation{},
	}
	d.Registry.WorkspaceDigest = digest(mustEncodeDocument(t, d))
	if err := atomicWrite(filepath.Join(dir, "WORKSPACE.md"), mustEncodeDocument(t, d)); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, ".runtime", "index.json"), d.Registry); err != nil {
		t.Fatal(err)
	}
	return wsID, agentID, taskID, sessionID, runID
}

func mustEncodeDocument(t *testing.T, d *Document) []byte {
	t.Helper()
	b, err := encodeDocument(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLegacyWorkspaceCompatibility(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	wsID, agentID, taskID, sessionID, runID := installLegacyWorkspace(t, s)

	status, err := s.Status(ctx, wsID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Workspace.ID != wsID || status.Workspace.OrchestratorAgentID != agentID {
		t.Fatalf("legacy workspace did not resolve: %+v", status.Workspace)
	}
	var beforeTask Task
	if err := s.With(ctx, wsID, func(d *Document) error {
		if a, err := findAgent(d, agentID); err != nil || a.ID != agentID {
			return fail("test", "agent did not resolve by legacy id: %v", err)
		}
		if got, err := findTask(d, taskID); err != nil || got.ID != taskID {
			return fail("test", "task did not resolve by legacy id: %v", err)
		}
		if got, err := findSession(d, sessionID); err != nil || got.ID != sessionID {
			return fail("test", "session did not resolve by legacy id: %v", err)
		}
		if got, err := findSession(d, runID); err != nil || got.ID != sessionID {
			return fail("test", "session did not resolve by Run-ID alias: %v", err)
		}
		found, err := findTask(d, taskID)
		if err != nil {
			return err
		}
		beforeTask = *found
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	agent, err := s.CreateAgent(ctx, wsID, AgentOptions{Name: "worker-x", Role: "planner", Instructions: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if agent.ID != "agent_worker-x" {
		t.Fatalf("new agent in a legacy workspace = %q, want agent_worker-x", agent.ID)
	}
	task, err := s.CreateTask(ctx, wsID, TaskSpec{Title: "New slug task", Goal: "Bounded", Role: "planner", AcceptanceCriteria: []string{"result"}}, "legacy:new-task")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "task_new-slug-task" {
		t.Fatalf("new task in a legacy workspace = %q, want task_new-slug-task", task.ID)
	}

	if err := s.With(ctx, wsID, func(d *Document) error {
		got, err := findTask(d, taskID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(*got, beforeTask) {
			return fail("test", "pre-existing legacy task changed: before=%+v after=%+v", beforeTask, *got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// --- tmux integration --------------------------------------------------------

// Opt-in: a slug workspace gets a tmux session named workspace-<id>, an
// unrecorded pane is recovered inside that exact session, and stopping it never
// touches a live sibling whose ID merely shares the prefix.
func TestTmuxSlugWorkspaceIdentity(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getenv("WORKSPACE_TMUX_TEST") != "1" {
		t.Skip("requires opt-in Linux/macOS tmux")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatal(err)
	}
	s, first := fixture(t)
	ctx := context.Background()
	if first != "ws_fix-checkout" {
		t.Fatalf("fixture workspace id = %q", first)
	}
	second, err := s.Create(ctx, CreateOptions{Title: "Fix checkout", Input: "x", Workflow: "extended"})
	if err != nil {
		t.Fatal(err)
	}
	other := second.Workspace.ID
	if other != "ws_fix-checkout-2" {
		t.Fatalf("sibling workspace id = %q, want ws_fix-checkout-2", other)
	}

	socket := "slug-identity-" + ID("tmux")
	rt := Tmux{Socket: socket}
	s.Runtime = rt
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	bin := filepath.Join(t.TempDir(), "workspace")
	build := exec.Command("go", "build", "-o", bin, "./cmd/workspace")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	s.Executable = bin

	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clients["test"] = Client{Adapter: "command", LaunchArgv: []string{"sh", "-c", "sleep 30", "--", "{prompt_file}"}}
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}

	orch, err := s.StartOrchestrator(ctx, first, "slug-orch")
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := s.StartOrchestrator(ctx, other, "slug-orch-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.call(ctx, "has-session", "-t", "="+TmuxName(first)); err != nil {
		t.Fatalf("tmux session %s missing: %v", TmuxName(first), err)
	}
	if _, err := rt.call(ctx, "has-session", "-t", "="+TmuxName(other)); err != nil {
		t.Fatalf("tmux session %s missing: %v", TmuxName(other), err)
	}

	// Simulate an interrupted ledger write: the pane exists but the run lost its
	// recorded pane. Reconcile must recover the exact pane inside the session.
	if err := s.With(ctx, first, func(d *Document) error {
		p, findErr := findSession(d, orch.ID)
		if findErr != nil {
			return findErr
		}
		r, runErr := findRun(d, p.CurrentRunID)
		if runErr != nil {
			return runErr
		}
		r.State = "starting"
		r.PaneID, r.WindowID = "", ""
		p.CurrentRunID = r.ID
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Reconcile(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	var recoveredSession *Session
	for i := range recovered.Sessions {
		if recovered.Sessions[i].ID == orch.ID {
			recoveredSession = &recovered.Sessions[i]
		}
	}
	if recoveredSession == nil || recoveredSession.PaneID == "" {
		t.Fatalf("unrecorded pane was not recovered: %+v", recovered.Sessions)
	}
	pane, err := rt.Inspect(ctx, recoveredSession.PaneID)
	if err != nil || pane.SessionID != orch.ID || pane.WorkspaceID != first {
		t.Fatalf("recovered pane is not owned by the slug workspace: pane=%+v err=%v", pane, err)
	}
	if name := TmuxName(pane.WorkspaceID); name != TmuxName(first) {
		t.Fatalf("recovered pane session name = %q", name)
	}

	// Stopping the first workspace must not kill the live -2 sibling.
	if err := rt.StopWorkspace(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.call(ctx, "has-session", "-t", "="+TmuxName(first)); err == nil {
		t.Fatalf("stopping %s left its own session alive", first)
	}
	if _, err := rt.call(ctx, "has-session", "-t", "="+TmuxName(other)); err != nil {
		t.Fatalf("stopping %s also stopped live %s: %v", first, other, err)
	}
	if _, err := rt.Inspect(ctx, sibling.PaneID); err != nil {
		t.Fatalf("sibling pane was affected by stopping %s: %v", first, err)
	}
}
