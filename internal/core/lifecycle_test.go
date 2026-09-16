package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPauseInterruptAndArchiveGates(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, id, "planner")
	p, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Pause(ctx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Sessions[0].Active() {
		t.Fatal("pause killed active work")
	}
	v, err = s.Pause(ctx, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if v.Sessions[0].State != "stopped" {
		t.Fatal("interrupt did not stop", p.ID)
	}
	_, err = s.Archive(ctx, id)
	expectCode(t, err, "release_required")
	if err := s.With(ctx, id, func(d *Document) error {
		d.State.Status = "completed"
		d.State.Release.UserConfirmed = true
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	_, err = s.SetPaused(ctx, id, false)
	expectCode(t, err, "workspace_closed")
	completed, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ArchiveGuarded(ctx, id, "archive-stale", completed.Workspace.Revision-1)
	expectCode(t, err, "revision_conflict")
	if _, err = s.ArchiveGuarded(ctx, id, "archive-current", completed.Workspace.Revision); err != nil {
		t.Fatal(err)
	}
}

func TestCleanPreservesUnpublishedCommitsAndIgnoredFiles(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	_, w := worker(t, s, id, "work")
	if err := os.WriteFile(filepath.Join(w.Path, "fix.txt"), []byte("important fix"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, w.Path, "add", "fix.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, w.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fix"); err != nil {
		t.Fatal(err)
	}
	if err := s.With(ctx, id, func(d *Document) error {
		d.State.Status = "archived"
		d.State.Release.UserConfirmed = true
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	items, err := s.Clean(ctx, id, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Allowed {
		t.Fatalf("unprotected commit allowed: %+v", items)
	}
	_, err = s.Clean(ctx, id, false, false)
	expectCode(t, err, "clean_refused")
	ignored := filepath.Join(w.Path, "work-products", "lost.md")
	if err := os.MkdirAll(filepath.Dir(ignored), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignored, []byte("unsaved analysis"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = s.Clean(ctx, id, false, true)
	expectCode(t, err, "clean_refused")
	if err := os.Remove(ignored); err != nil {
		t.Fatal(err)
	}
	items, err = s.Clean(ctx, id, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !items[0].Removed || items[0].Backup == "" {
		t.Fatal("worktree not backed up and removed")
	}
	if _, err := os.Stat(w.Path); !os.IsNotExist(err) {
		t.Fatal("worktree still exists")
	}
	if _, err := git(ctx, s.Root, "bundle", "verify", items[0].Backup); err != nil {
		t.Fatal(err)
	}
	items, err = s.Clean(ctx, id, false, true)
	if err != nil || len(items) != 0 {
		t.Fatal("clean replay failed", err)
	}
}

func TestReconcileFinishesInterruptedRemoval(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	_, w := worker(t, s, ws, "removed")
	if err := s.With(ctx, ws, func(d *Document) error { wt, _ := findWorktree(d, w.ID); wt.State = "removing"; return saveDocument(d) }); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, s.Root, "worktree", "remove", "--", w.Path); err != nil {
		t.Fatal(err)
	}
	v, err := s.Reconcile(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if v.Worktrees[0].State != "removed" {
		t.Fatal("removal was not reconciled")
	}
}
