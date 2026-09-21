package core

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestStateMutationReplaysOriginalResultAndRejectsChangedPayload(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	initial, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	title := "first revision"
	patch := StatePatch{Title: &title}
	first, err := s.UpdateState(ctx, ws, initial.Workspace.Revision, patch, "edit-1")
	if err != nil {
		t.Fatal(err)
	}
	laterTitle := "later revision"
	later, err := s.UpdateState(ctx, ws, first.Workspace.Revision, StatePatch{Title: &laterTitle}, "edit-2")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.UpdateState(ctx, ws, initial.Workspace.Revision, patch, "edit-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, replay) {
		t.Fatal("replay did not preserve the original response")
	}
	current, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current, later) {
		t.Fatal("replay changed current state")
	}
	_, err = s.UpdateState(ctx, ws, initial.Workspace.Revision, StatePatch{Title: &laterTitle}, "edit-1")
	expectCode(t, err, "operation_conflict")
	_, err = s.SetPaused(ctx, ws, true, "edit-1")
	expectCode(t, err, "operation_conflict")
	_, err = s.CreateAgent(ctx, ws, AgentOptions{Name: "planner", Role: "planner", OperationKey: "edit-1"})
	expectCode(t, err, "operation_conflict")
}

func TestMutationRollbackAndConcurrentReplay(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	before, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	var out Status
	err = mutate(s, ctx, ws, []string{"rollback"}, "rollback-test", &out, s.requireOrchestrator, func(d *Document) error {
		d.State.Title = "must not persist"
		if err := saveDocument(d); err != nil {
			return err
		}
		return fail("test_error", "simulated failure after staged save")
	})
	expectCode(t, err, "test_error")
	after, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed mutation committed")
	}
	title := "concurrent edit"
	var wg sync.WaitGroup
	results := make([]Status, 2)
	errors := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errors[i] = s.UpdateState(ctx, ws, before.Workspace.Revision, StatePatch{Title: &title}, "parallel")
		}(i)
	}
	wg.Wait()
	for _, err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(results[0], results[1]) || results[0].Workspace.Revision != before.Workspace.Revision+1 {
		t.Fatal("concurrent mutation was not applied exactly once")
	}
}

func TestExternalMutationRecoversAndReplaysWithoutRepeatingEffects(t *testing.T) {
	s, ws, _ := integratedWorkflow(t)
	ctx := context.Background()
	f := &fakeForge{uncertain: true}
	s.Forge = f
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.PublishChangeRequest(ctx, ws, cr.ID, false, "publish")
	expectCode(t, err, "forge_error")
	_, err = s.PublishChangeRequest(ctx, ws, cr.ID, true, "publish")
	expectCode(t, err, "operation_conflict")
	first, err := s.PublishChangeRequest(ctx, ws, cr.ID, false, "publish")
	if err != nil {
		t.Fatal(err)
	}
	f.duringLookup = func() { t.Fatal("completed replay reached the forge") }
	replayed, err := s.PublishChangeRequest(ctx, ws, cr.ID, false, "publish")
	if err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 || !reflect.DeepEqual(first, replayed) {
		t.Fatal("publication did not replay exactly once")
	}
}

func TestPauseReplayDoesNotPauseResumedWorkspace(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	first, err := s.Pause(ctx, ws, false, "pause")
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := s.SetPaused(ctx, ws, false, "resume")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := s.Pause(ctx, ws, false, "pause")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, replayed) {
		t.Fatal("pause result changed on replay")
	}
	current, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed, current) {
		t.Fatal("replayed pause changed resumed state")
	}
	_, err = s.Pause(ctx, ws, true, "pause")
	expectCode(t, err, "operation_conflict")
}

func TestStopRecoversMissingPane(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, ws, "planner")
	p, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	delete(s.Runtime.(*fakeRuntime).panes, p.PaneID)
	stopped, err := s.StopSession(ctx, ws, p.ID, "stop")
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != "stopped" {
		t.Fatal("missing pane retained active reservation")
	}
	replayed, err := s.StopSession(ctx, ws, p.ID, "stop")
	if err != nil || !reflect.DeepEqual(stopped, replayed) {
		t.Fatal("stop did not replay", err)
	}
}

func TestMutationReceiptRecoversWithPendingStateWrite(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	before, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	title := "recovered change"
	patch := StatePatch{Title: &title}
	var expected Status
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Title = title
		d.State.Revision++
		expected = d.Status()
		result, err := json.Marshal(expected)
		if err != nil {
			return err
		}
		d.Registry.Mutations = map[string]MutationReceipt{"recover": {ID: ID("op"), Digest: payloadDigest([]any{"state.update", before.Workspace.Revision, patch}), Revision: d.State.Revision, Result: result, State: "completed"}}
		b, err := encodeDocument(d)
		if err != nil {
			return err
		}
		previous := d.Registry.WorkspaceDigest
		d.Registry.WorkspaceDigest = digest(b)
		return writeJSON(filepath.Join(d.Dir, ".runtime", "pending.json"), pendingWrite{BeforeDigest: previous, Document: b, Registry: d.Registry})
	}); err != nil {
		t.Fatal(err)
	}
	replayed, err := s.UpdateState(ctx, ws, before.Workspace.Revision, patch, "recover")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, replayed) {
		t.Fatal("receipt and state were not recovered together")
	}
	metadata, err := s.OperationMetadata(ctx, ws, "recover")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ID == "" || metadata.Revision != expected.Workspace.Revision {
		t.Fatal("lost operation metadata")
	}
}

func TestProjectMutationKeyConflictsAcrossCommands(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	first, err := InitProject(ctx, s.Root, "setup")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := InitProject(ctx, s.Root, "setup")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, replayed) {
		t.Fatal("project initialization result changed on replay")
	}
	_, err = s.InstallSkill(ctx, "codex", "setup")
	expectCode(t, err, "operation_conflict")
	metadata, err := s.OperationMetadata(ctx, "", "setup")
	if err != nil || metadata.ID == "" {
		t.Fatal("missing project operation ID", err)
	}
}

func TestClosedWorkspaceReplaysResourcesButRejectsNewWork(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	aOpt := AgentOptions{Name: "planner", Role: "planner", OperationKey: "agent"}
	wOpt := WorktreeOptions{Name: "planning", Purpose: "planning", OperationKey: "worktree"}
	spec := TaskSpec{Name: "planning", Title: "Plan", Goal: "Investigate issue", Role: "planner", AcceptanceCriteria: []string{"A concrete plan"}}
	a, err := s.CreateAgent(ctx, ws, aOpt)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.CreateWorktree(ctx, ws, wOpt)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(ctx, ws, spec, "task")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Status = "completed"
		d.State.Tasks[0].State = "accepted"
		d.Registry.Worktrees[0].State = "removed"
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	aReplay, err := s.CreateAgent(ctx, ws, aOpt)
	if err != nil || aReplay.ID != a.ID {
		t.Fatal("agent replay failed", err)
	}
	wReplay, err := s.CreateWorktree(ctx, ws, wOpt)
	if err != nil || !reflect.DeepEqual(wReplay, w) {
		t.Fatal("worktree replay failed", err)
	}
	taskReplay, err := s.CreateTask(ctx, ws, spec, "task")
	if err != nil || !reflect.DeepEqual(taskReplay, task) {
		t.Fatal("task replay failed", err)
	}
	aOpt.Name, aOpt.OperationKey = "another", "another-agent"
	_, err = s.CreateAgent(ctx, ws, aOpt)
	expectCode(t, err, "workspace_completed")
	wOpt.Name, wOpt.OperationKey = "another", "another-worktree"
	_, err = s.CreateWorktree(ctx, ws, wOpt)
	expectCode(t, err, "workspace_completed")
	spec.Name = "another"
	_, err = s.CreateTask(ctx, ws, spec, "another-task")
	expectCode(t, err, "workspace_completed")
}

func TestSessionStartReplayPreservesResponseAfterStop(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, ws, "planner")
	opt := SessionOptions{Agent: a.ID, Worktree: w.ID, OperationKey: "start"}
	first, err := s.StartSession(ctx, ws, opt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, first.ID); err != nil {
		t.Fatal(err)
	}
	replayed, err := s.StartSession(ctx, ws, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, replayed) {
		t.Fatal("session start result changed on replay")
	}
	current, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if current.Sessions[0].State != "stopped" || s.Runtime.(*fakeRuntime).launches != 1 {
		t.Fatal("start replay relaunched a stopped session")
	}
}
