package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestThreeRestartsReuseLogicalSessionAndFenceOldRun(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	client := cfg.Clients["test"]
	client.ResumeArgv = append([]string(nil), client.LaunchArgv...)
	cfg.Clients["test"] = client
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	a, w := worker(t, s, ws, "planner")
	first, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindThread(ctx, ws, first.ID, "native-thread-1"); err != nil {
		t.Fatal(err)
	}
	oldRun := first.CurrentRunID
	if _, err := s.StopSession(ctx, ws, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.ResumeAgent(ctx, ws, a.ID, "resume:1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, second.ID); err != nil {
		t.Fatal(err)
	}
	third, err := s.ResumeAgent(ctx, ws, a.ID, "resume:2")
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Sessions) != 1 || len(status.Runs) != 3 || third.ID != first.ID || status.Sessions[0].RunCount != 3 {
		t.Fatalf("logical/run split: sessions=%+v runs=%+v", status.Sessions, status.Runs)
	}
	if third.ClientThreadID != "native-thread-1" || status.Runs[1].ClientThreadID != "native-thread-1" || status.Runs[2].ClientThreadID != "native-thread-1" {
		t.Fatal("native thread binding was not carried into successor runs")
	}
	zombie := *s
	zombie.Actor = Actor{AgentID: a.ID, SessionID: first.ID, RunID: oldRun}
	_, err = zombie.SendMessage(ctx, ws, MessageOptions{To: "orchestrator", Body: "late write"})
	expectCode(t, err, "stale_actor")
}

func TestClientThreadCannotBelongToTwoActiveSessions(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	a1, w1 := worker(t, s, ws, "planner-a")
	a2, w2 := worker(t, s, ws, "planner-b")
	p1, err := s.StartSession(ctx, ws, SessionOptions{Agent: a1.ID, Worktree: w1.ID, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.StartSession(ctx, ws, SessionOptions{Agent: a2.ID, Worktree: w2.ID, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindThread(ctx, ws, p1.ID, "same-native-thread"); err != nil {
		t.Fatal(err)
	}
	_, err = s.BindThread(ctx, ws, p2.ID, "same-native-thread")
	expectCode(t, err, "thread_conflict")
}

func TestLegacySessionMigrationPreservesProvenanceAndIsIdempotent(t *testing.T) {
	now := nowUTC()
	later := now.Add(time.Minute)
	d := &Document{State: Workspace{Tasks: []Task{{ID: "task", SessionID: "sess_old_2"}}}, Registry: Registry{
		Sessions: []Session{
			{ID: "sess_old_1", AgentID: "agent", WorktreeID: "wt", TaskID: "task", TaskAttempt: 1, InputDigest: "digest", ClientSnapshot: Client{Adapter: "codex"}, ClientThreadID: "thread", State: "interrupted", CreatedAt: now},
			{ID: "sess_old_2", AgentID: "agent", WorktreeID: "wt", TaskID: "task", TaskAttempt: 1, InputDigest: "digest", ClientSnapshot: Client{Adapter: "codex"}, ClientThreadID: "thread", State: "running", CreatedAt: later},
		},
		Checks:     []CheckReceipt{{ID: "check", SessionID: "sess_old_2"}},
		Handoffs:   []Handoff{{ID: "handoff", FromSession: "sess_old_1"}},
		Messages:   []Message{{ID: "message", FromSession: "sess_old_1", DeliveredSessionID: "sess_old_2"}},
		Operations: map[string]Operation{"legacy-start": {ResourceID: "sess_old_2", Result: []byte(`{"id":"sess_old_2"}`)}},
	}}
	d.State.Artifacts = []Artifact{{ID: "artifact", SessionID: "sess_old_1"}}
	if !migrateRegistryV2(d) {
		t.Fatal("legacy registry was not migrated")
	}
	if migrateRegistryV2(d) {
		t.Fatal("migration is not idempotent")
	}
	if len(d.Registry.Sessions) != 1 || len(d.Registry.Runs) != 2 {
		t.Fatalf("unexpected grouping: %+v %+v", d.Registry.Sessions, d.Registry.Runs)
	}
	sid := d.Registry.Sessions[0].ID
	if d.Registry.Runs[0].ID != "sess_old_1" || d.Registry.Runs[1].ID != "sess_old_2" || d.Registry.Runs[1].SessionID != sid {
		t.Fatal("legacy run aliases were not preserved")
	}
	if d.Registry.Checks[0].SessionID != sid || d.Registry.Checks[0].RunID != "sess_old_2" ||
		d.Registry.Handoffs[0].FromSession != sid || d.Registry.Handoffs[0].FromRun != "sess_old_1" ||
		d.State.Artifacts[0].SessionID != sid || d.State.Artifacts[0].RunID != "sess_old_1" ||
		d.Registry.Messages[0].DeliveredRunID != "sess_old_2" || d.State.Tasks[0].RunID != "sess_old_2" {
		t.Fatal("migration lost exact execution provenance")
	}
	if d.Registry.Operations["legacy-start"].ResourceID != sid || len(d.Registry.Operations["legacy-start"].Result) != 0 {
		t.Fatal("session operation replay was not migrated to the logical resource")
	}
	legacy := Service{Actor: Actor{AgentID: "agent", SessionID: "sess_old_2"}}
	if _, err := legacy.actor(d); err != nil {
		t.Fatalf("active legacy runtime was not kept compatible: %v", err)
	}
}

func TestMigrationFencesDuplicateActiveNativeThreadOwners(t *testing.T) {
	now := nowUTC()
	d := &Document{Registry: Registry{Sessions: []Session{
		{ID: "sess_a", AgentID: "a", TaskID: "one", ClientSnapshot: Client{Adapter: "opencode"}, ClientThreadID: "shared", State: "running", CreatedAt: now},
		{ID: "sess_b", AgentID: "b", TaskID: "two", ClientSnapshot: Client{Adapter: "opencode"}, ClientThreadID: "shared", State: "running", CreatedAt: now.Add(time.Minute)},
	}}}
	migrateRegistryV2(d)
	active := 0
	for _, run := range d.Registry.Runs {
		if run.Active() {
			active++
		}
	}
	if len(d.Registry.Sessions) != 2 || active != 1 || d.Registry.Runs[0].State != "interrupted" || d.Registry.Runs[1].State != "running" {
		t.Fatalf("duplicate binding was not fenced: sessions=%+v runs=%+v", d.Registry.Sessions, d.Registry.Runs)
	}
}

func TestOperationReplayDoesNotCreateAnotherRun(t *testing.T) {
	s, ws := fixture(t)
	a, w := worker(t, s, ws, "planner")
	opt := SessionOptions{Agent: a.ID, Worktree: w.ID, OperationKey: "start-once"}
	first, err := s.StartSession(context.Background(), ws, opt)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := s.StartSession(context.Background(), ws, opt)
	if err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(context.Background(), ws)
	if replayed.ID != first.ID || replayed.CurrentRunID != first.CurrentRunID || len(status.Runs) != 1 {
		t.Fatalf("operation replay launched another run: %+v", status.Runs)
	}
}

func TestAttemptChangeCreatesNewLogicalSession(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "attempt-lineage", "planner", nil)
	p, _ := startTask(t, s, ws, task)
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetryTask(ctx, ws, task.ID, "new input lineage", "retry-lineage"); err != nil {
		t.Fatal(err)
	}
	next, err := s.ResumeAgent(ctx, ws, p.AgentID, "resume-new-attempt")
	if err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx, ws)
	if next.ID == p.ID || len(status.Sessions) != 2 || len(status.Runs) != 2 {
		t.Fatalf("attempt change reused logical session: %+v", status.Sessions)
	}
}

func TestNewWorktreeAndNativeThreadCreateNewLogicalSession(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	a, w1 := worker(t, s, ws, "context-one")
	w2, err := s.CreateWorktree(ctx, ws, WorktreeOptions{Name: "context-two", Purpose: "planning"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w1.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindThread(ctx, ws, first.ID, "thread-one"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindThread(ctx, ws, second.ID, "thread-two"); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status(ctx, ws)
	if first.ID == second.ID || len(status.Sessions) != 2 || status.Sessions[0].ClientThreadID == status.Sessions[1].ClientThreadID {
		t.Fatalf("distinct worktree/thread lineage was merged: %+v", status.Sessions)
	}
}

func TestClosedLogicalSessionCannotResume(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, ws, "closable")
	p, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	closed, err := s.CloseSession(ctx, ws, p.ID, "completed", "close-once")
	if err != nil {
		t.Fatal(err)
	}
	if closed.LifecycleState != "closed" || closed.ClosedAt == nil {
		t.Fatal("logical session was not closed")
	}
	_, err = s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID, ResumeSession: p.ID})
	expectCode(t, err, "session_closed")
}

func TestProjectionKeepsActiveCurrentRunWhenHistoryIsNewer(t *testing.T) {
	now := nowUTC()
	later := now.Add(time.Minute)
	finished := later
	d := &Document{Registry: Registry{
		Sessions: []Session{{ID: "sess", CurrentRunID: "run-current"}},
		Runs: []Run{
			{ID: "run-newer-exited", SessionID: "sess", State: "exited", CreatedAt: later, FinishedAt: &finished},
			{ID: "run-current", SessionID: "sess", State: "running", CreatedAt: now},
		},
	}}
	d.syncSessions()
	p := d.Registry.Sessions[0]
	if !p.Active() || p.CurrentRunID != "run-current" || p.LastRunID != "run-newer-exited" || p.RunState != "running" {
		t.Fatalf("projection selected the wrong execution: %+v", p)
	}
}

func TestRunOnlyActorCanStopOwnedSession(t *testing.T) {
	s, ws := fixture(t)
	a, w := worker(t, s, ws, "run-only-owner")
	p, err := s.StartSession(context.Background(), ws, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	owner := *s
	owner.Actor = Actor{AgentID: a.ID, RunID: p.CurrentRunID}
	stopped, err := owner.StopSession(context.Background(), ws, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Active() || stopped.State != "stopped" {
		t.Fatalf("run-only actor could not stop its logical session: %+v", stopped)
	}
}

func TestResumeUsesSnapshottedAgentDefinition(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, ws, "snapshot-owner")
	first, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		current, err := findAgent(d, a.ID)
		if err != nil {
			return err
		}
		for i := range d.Registry.Agents {
			if d.Registry.Agents[i].ID == current.ID {
				d.Registry.Agents[i].Instructions = "changed live instructions"
			}
		}
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.StartSession(ctx, ws, SessionOptions{ResumeSession: first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.AgentSnapshot.Instructions != a.Instructions {
		t.Fatalf("resume rewrote the agent snapshot: %+v", resumed.AgentSnapshot)
	}
	prompt, err := os.ReadFile(resumed.PromptFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(prompt), "changed live instructions") || !strings.Contains(string(prompt), a.Instructions) {
		t.Fatalf("resume prompt did not use the snapshotted agent: %s", prompt)
	}
}
