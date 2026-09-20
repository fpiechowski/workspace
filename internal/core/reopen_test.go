package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCompletedManualAndWorkflowWorkspacesResumeTheOrchestratorConversationally(t *testing.T) {
	t.Run("manual", func(t *testing.T) {
		s, err := fixtureServiceForManual(t)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		statuses, err := s.List(ctx)
		if err != nil || len(statuses) != 1 {
			t.Fatalf("manual fixture status: %v", err)
		}
		status := statuses[0]
		orch, err := s.StartOrchestrator(ctx, status.Workspace.ID, "manual-orchestrator-start")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.StopSession(ctx, status.Workspace.ID, orch.ID); err != nil {
			t.Fatal(err)
		}
		status, err = s.Status(ctx, status.Workspace.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.CompleteWorkspace(ctx, status.Workspace.ID, CompleteOptions{Reason: "manual result delivered", ExpectedRevision: status.Workspace.Revision}, "manual-complete"); err != nil {
			t.Fatal(err)
		}
		resumed, err := s.StartOrchestrator(ctx, status.Workspace.ID, "manual-conversation")
		if err != nil {
			t.Fatal(err)
		}
		if resumed.ID != orch.ID {
			t.Fatalf("start created a new logical session: got %s want %s", resumed.ID, orch.ID)
		}
		assertConversationRun(t, s, status.Workspace.ID, resumed.ID)
	})

	t.Run("workflow", func(t *testing.T) {
		s, ws := fixture(t)
		ctx := context.Background()
		orch, err := s.StartOrchestrator(ctx, ws, "workflow-orchestrator-start")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.StopSession(ctx, ws, orch.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.With(ctx, ws, func(d *Document) error {
			d.State.Status = "completed"
			d.State.Workflow.Phase = "completed"
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
		resumed, err := s.StartOrchestrator(ctx, ws, "workflow-conversation")
		if err != nil {
			t.Fatal(err)
		}
		if resumed.ID != orch.ID {
			t.Fatalf("start created a new logical session: got %s want %s", resumed.ID, orch.ID)
		}
		assertConversationRun(t, s, ws, resumed.ID)
	})
}

func TestAcceptedWorkerResumeIsConsultationOnlyAndPreservesTaskProvenance(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, ws, "accepted-worker")
	task, err := s.CreateTask(ctx, ws, TaskSpec{Name: "accepted-plan", Title: "Accepted plan", Goal: "Discuss the accepted plan", Role: "planner", AcceptanceCriteria: []string{"The plan is recorded"}}, "accepted-task")
	if err != nil {
		t.Fatal(err)
	}
	prior, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID, Task: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, prior.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		t, err := findTask(d, task.ID)
		if err != nil {
			return err
		}
		t.State = "accepted"
		d.State.Status = "completed"
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	before, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	var taskBefore Task
	for _, candidate := range before.Workspace.Tasks {
		if candidate.ID == task.ID {
			taskBefore = candidate
		}
	}
	var resumed Session
	if runtime.GOOS == "windows" {
		// ResumeSession also starts the tmux supervisor, which is intentionally
		// unavailable to the Windows core test binary. Exercise the same exact
		// logical-session path directly; Linux/WSL covers the supervised entry
		// point.
		resumed, err = s.StartSession(ctx, ws, SessionOptions{Agent: prior.AgentID, Worktree: prior.WorktreeID, Parent: prior.ParentAgentID, ParentSessionID: prior.ParentSessionID, Profile: prior.Profile, Task: prior.TaskID, ResumeSession: prior.ID, ReadOnly: prior.ReadOnly, OperationKey: "accepted-consultation"})
	} else {
		resumed, err = s.ResumeSession(ctx, ws, prior.ID, "accepted-consultation")
	}
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	var taskAfter Task
	for _, candidate := range after.Workspace.Tasks {
		if candidate.ID == task.ID {
			taskAfter = candidate
		}
	}
	if !reflect.DeepEqual(taskBefore, taskAfter) {
		t.Fatalf("consultation changed accepted task provenance:\nbefore: %+v\nafter:  %+v", taskBefore, taskAfter)
	}
	if resumed.ID != prior.ID || len(after.Sessions) != 1 || len(after.Runs) != 2 {
		t.Fatalf("resume did not reuse the logical session: sessions=%d runs=%d resumed=%s prior=%s", len(after.Sessions), len(after.Runs), resumed.ID, prior.ID)
	}
	run, err := findRunInStatus(after, resumed.CurrentRunID)
	if err != nil {
		t.Fatal(err)
	}
	if !run.ConversationOnly {
		t.Fatal("accepted worker resume was not marked conversation_only")
	}
	prompt, err := os.ReadFile(run.PromptFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "This Run is conversation-only") || !strings.Contains(string(prompt), "workspace reopen") {
		t.Fatalf("conversation notice missing from generated prompt: %s", prompt)
	}
}

func TestArchivedWorkspaceRejectsConversationAndReopen(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Status = "archived"
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.StartOrchestrator(ctx, ws, "archived-start")
	expectCode(t, err, "workspace_archived")
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "follow-up", ExpectedRevision: status.Workspace.Revision}, "archived-reopen")
	expectCode(t, err, "workspace_archived")
}

func TestReopenGuardsAndHistoryAreAtomicAndIdempotent(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Status = "completed"
		d.State.Workflow.Phase = "completed"
		d.State.Integration = &Integration{WorktreeID: "wt-old", HeadCommit: "head-old"}
		d.State.LiveTest = LiveTest{Choice: "run", HeadCommit: "head-old", AcceptedHandoff: "handoff-test"}
		d.State.Release = Release{UserConfirmed: true, Reference: "release-1", HeadCommit: "head-old"}
		d.State.PendingDecision = &Decision{ID: "decision-old", Kind: "live-testing", Question: "old", Revision: d.State.Revision}
		d.State.ChangeRequests = []ChangeRequest{{ID: "cr-old", State: "merged", HeadCommit: "head-old"}}
		d.State.Artifacts = []Artifact{{ID: "artifact-old", Path: "artifacts/artifact-old/report.md"}}
		d.Registry.Handoffs = []Handoff{{ID: "handoff-old", TaskID: "task-old", State: "accepted"}}
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	beforeDoc, beforeRevision, err := s.StateDocument(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "User requested a follow-up discussion", ExpectedRevision: beforeRevision}, "reopen-once")
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if after.Workspace.Status != "active" || after.Workspace.Workflow == nil || after.Workspace.Workflow.Phase != "planning" {
		t.Fatalf("workflow was not reopened into active planning: %+v", after.Workspace)
	}
	if after.Workspace.Integration != nil || after.Workspace.PendingDecision != nil || after.Workspace.LiveTest != (LiveTest{}) || after.Workspace.Release != (Release{}) {
		t.Fatalf("terminal derived state was not invalidated: %+v", after.Workspace)
	}
	if after.Workspace.ChangeRequests[0].State != "outdated" || len(after.Workspace.Artifacts) != len(before.Workspace.Artifacts) || len(after.Workspace.Tasks) != len(before.Workspace.Tasks) {
		t.Fatalf("reopen did not preserve immutable history while invalidating CRs: %+v", after.Workspace)
	}
	var handoffs []Handoff
	if err := s.With(ctx, ws, func(d *Document) error {
		handoffs = append(handoffs, d.Registry.Handoffs...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 1 {
		t.Fatal("accepted handoff history was lost")
	}
	entries, err := os.ReadDir(filepath.Join(after.Directory, "history"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("reopen history directory missing: entries=%v err=%v", entries, err)
	}
	historyDoc, err := os.ReadFile(filepath.Join(after.Directory, "history", entries[0].Name(), "WORKSPACE.md"))
	if err != nil || !reflect.DeepEqual(historyDoc, beforeDoc) {
		t.Fatalf("pre-reopen WORKSPACE.md was not preserved: err=%v", err)
	}
	reason, err := os.ReadFile(filepath.Join(after.Directory, "history", entries[0].Name(), "REASON.md"))
	if err != nil || !strings.Contains(string(reason), "follow-up discussion") {
		t.Fatalf("reopen reason was not recorded: %v %s", err, reason)
	}
	replayed, err := s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "User requested a follow-up discussion", ExpectedRevision: beforeRevision}, "reopen-once")
	if err != nil || replayed.Workspace.Revision != after.Workspace.Revision {
		t.Fatalf("reopen replay changed the result: %+v %v", replayed, err)
	}
	_, err = s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "different authorization", ExpectedRevision: beforeRevision}, "reopen-once")
	expectCode(t, err, "operation_conflict")
	_, err = s.Archive(ctx, ws)
	expectCode(t, err, "release_required")
}

func TestReopenManualWorkspaceReturnsToActiveManualState(t *testing.T) {
	s, err := fixtureServiceForManual(t)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	statuses, err := s.List(ctx)
	if err != nil || len(statuses) != 1 {
		t.Fatalf("manual fixture status: %v", err)
	}
	ws := statuses[0].Workspace.ID
	if err := setCompletedForReopenTest(s, ws); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "User requested manual follow-up", ExpectedRevision: status.Workspace.Revision}, "manual-reopen")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Workspace.Status != "active" || !reopened.Workspace.Manual() || reopened.Workspace.Workflow != nil {
		t.Fatalf("manual workspace did not reopen as active manual workspace: %+v", reopened.Workspace)
	}
}

func TestReopenRequiresCurrentRevisionReasonConfirmationAndIdleWorkers(t *testing.T) {
	t.Run("revision and reason", func(t *testing.T) {
		s, ws := fixture(t)
		ctx := context.Background()
		if err := setCompletedForReopenTest(s, ws); err != nil {
			t.Fatal(err)
		}
		status, _ := s.Status(ctx, ws)
		_, err := s.ReopenWorkspace(ctx, ws, ReopenOptions{ExpectedRevision: status.Workspace.Revision}, "missing-reason")
		expectCode(t, err, "reason_required")
		_, err = s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "follow-up", ExpectedRevision: status.Workspace.Revision - 1}, "stale-revision")
		expectCode(t, err, "revision_conflict")
	})

	t.Run("agent confirmation", func(t *testing.T) {
		s, ws := fixture(t)
		ctx := context.Background()
		orch, err := s.StartOrchestrator(ctx, ws, "active-orchestrator")
		if err != nil {
			t.Fatal(err)
		}
		if err := setCompletedForReopenTest(s, ws); err != nil {
			t.Fatal(err)
		}
		status, _ := s.Status(ctx, ws)
		s.Actor = Actor{AgentID: orch.AgentID, SessionID: orch.ID, RunID: orch.CurrentRunID}
		_, err = s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "follow-up", ExpectedRevision: status.Workspace.Revision}, "agent-reopen")
		expectCode(t, err, "user_decision_required")
		if _, err := s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "follow-up", ExpectedRevision: status.Workspace.Revision, UserConfirmed: true}, "agent-reopen-ok"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("active worker", func(t *testing.T) {
		s, ws := fixture(t)
		ctx := context.Background()
		a, w := worker(t, s, ws, "active-worker")
		if _, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID}); err != nil {
			t.Fatal(err)
		}
		if err := setCompletedForReopenTest(s, ws); err != nil {
			t.Fatal(err)
		}
		status, _ := s.Status(ctx, ws)
		_, err := s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "follow-up", ExpectedRevision: status.Workspace.Revision}, "worker-reopen")
		expectCode(t, err, "session_active")
	})

	t.Run("active service", func(t *testing.T) {
		s, ws := fixture(t)
		ctx := context.Background()
		_, w := worker(t, s, ws, "active-service")
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.StartService(ctx, ws, ServiceOptions{Name: "dev", Worktree: w.ID, Argv: []string{exe, "-test.run=TestWorkerProcess"}}); err != nil {
			t.Fatal(err)
		}
		if err := setCompletedForReopenTest(s, ws); err != nil {
			t.Fatal(err)
		}
		status, _ := s.Status(ctx, ws)
		_, err = s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "follow-up", ExpectedRevision: status.Workspace.Revision}, "service-reopen")
		expectCode(t, err, "service_active")
	})
}

func TestSupervisorDoesNotRestartCompletedOrchestrator(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	orch, err := s.StartOrchestrator(ctx, ws, "supervisor-completed-orch")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, orch.ID); err != nil {
		t.Fatal(err)
	}
	if err := setCompletedForReopenTest(s, ws); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Runs) != 1 || status.Sessions[0].Active() {
		t.Fatalf("completed orchestrator was automatically restarted: sessions=%+v runs=%+v", status.Sessions, status.Runs)
	}
}

func fixtureServiceForManual(t *testing.T) (*Service, error) {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
		if _, err := git(ctx, dir, args...); err != nil {
			return nil, err
		}
	}
	if _, err := InitProject(ctx, dir); err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	s := &Service{Root: dir, Runtime: &fakeRuntime{panes: map[string]Pane{}}, Executable: exe}
	configure(t, s, exe)
	_, err = s.Create(ctx, CreateOptions{Title: "Manual", Input: "A manual result", NoWorkflow: true})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func setCompletedForReopenTest(s *Service, ws string) error {
	return s.With(context.Background(), ws, func(d *Document) error {
		d.State.Status = "completed"
		if d.State.Workflow != nil {
			d.State.Workflow.Phase = "completed"
		}
		return saveDocument(d)
	})
}

func assertConversationRun(t *testing.T, s *Service, ws, sessionID string) {
	t.Helper()
	status, err := s.Status(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	var run Run
	for _, candidate := range status.Runs {
		if candidate.SessionID == sessionID && candidate.ID == status.Sessions[0].CurrentRunID {
			run = candidate
		}
	}
	if run.ID == "" || !run.ConversationOnly {
		t.Fatalf("missing conversation-only run: %+v", status.Runs)
	}
}
