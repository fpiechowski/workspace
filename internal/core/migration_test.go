package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInputRevisionPreservesHistoryAndInvalidatesResults(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "planning", "planner", nil)
	p, w := startTask(t, s, ws, task)
	h := submitPlan(t, s, ws, p, w)
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	v, err := s.SetPaused(ctx, ws, true)
	if err != nil {
		t.Fatal(err)
	}
	opt := RevisionOptions{Input: "Changed acceptance criteria", Reason: "User clarified scope", ExpectedRevision: v.Workspace.Revision, OperationKey: "input:2"}
	updated, err := s.UpdateInput(ctx, ws, opt)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Workspace.Tasks[0].Attempt != 2 || updated.Workspace.Tasks[0].State != "pending" || updated.Workspace.Status != "paused" {
		t.Fatal("results not invalidated")
	}
	files, err := filepath.Glob(filepath.Join(v.Directory, "history", "*", "issue.md"))
	if err != nil || len(files) != 1 {
		t.Fatal("missing input history")
	}
	b, err := os.ReadFile(files[0])
	if err != nil || string(b) != "A reproducible issue" {
		t.Fatal("previous input lost")
	}
	if _, err := s.UpdateInput(ctx, ws, opt); err != nil {
		t.Fatal("replay failed", err)
	}
	_, err = s.ReviewHandoff(ctx, ws, h.ID, true, "")
	expectCode(t, err, "stale_handoff")
}

func TestWorkflowMigrationPreservesSessionPrompt(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, ws, "plan")
	p, err := s.StartSession(ctx, ws, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p.PromptFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	v, err := s.SetPaused(ctx, ws, true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Root, ".workspace", "templates", "workflows", "issue-resolution", "WORKFLOW.md.tmpl")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, []byte("\nAdditional acceptance requirement.\n")...)
	if err := atomicWrite(path, b); err != nil {
		t.Fatal(err)
	}
	updated, err := s.MigrateWorkflow(ctx, ws, RevisionOptions{Reason: "New workflow requirements", ExpectedRevision: v.Workspace.Revision, OperationKey: "migrate:2"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Workspace.Workflow.Version != 2 || updated.Workspace.Status != "paused" {
		t.Fatal("migration state incorrect")
	}
	after, err := os.ReadFile(p.PromptFile)
	if err != nil || digest(after) != digest(before) {
		t.Fatal("historical Session prompt changed")
	}
	files, _ := filepath.Glob(filepath.Join(v.Directory, "history", "*", "migration.json"))
	if len(files) != 1 {
		t.Fatal("migration manifest missing")
	}
}

func TestRecoveryCommitsPendingRevisionFiles(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	err := s.With(ctx, ws, func(d *Document) error {
		before := d.Registry.WorkspaceDigest
		d.State.Revision++
		d.Body += "\nUpdated input context\n"
		b, err := encodeDocument(d)
		if err != nil {
			return err
		}
		d.Registry.WorkspaceDigest = digest(b)
		return writeJSON(filepath.Join(d.Dir, ".runtime", "pending.json"), pendingWrite{BeforeDigest: before, Document: b, Registry: d.Registry, Files: map[string][]byte{"inputs/issue.md": []byte("recovered input"), "history/recovery/issue.md": []byte("original input")}})
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(v.Directory, "inputs", "issue.md"))
	if err != nil || string(b) != "recovered input" {
		t.Fatal("input not recovered")
	}
	b, err = os.ReadFile(filepath.Join(v.Directory, "history", "recovery", "issue.md"))
	if err != nil || string(b) != "original input" {
		t.Fatal("history not recovered")
	}
}
