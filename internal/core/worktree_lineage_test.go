package core

import (
	"context"
	"testing"
)

func TestWorktreeCreationFreezesLineage(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	create := func(name, base string) Worktree {
		t.Helper()
		w, err := s.CreateWorktree(ctx, id, WorktreeOptions{Name: name, Base: base, OperationKey: name})
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	parent := create("plan", "")
	if parent.ParentWorktreeID != "" {
		t.Fatal(parent)
	}
	// Explicit branch provenance is meaningful even at a shared initial commit.
	child := create("implementation", parent.Branch)
	if child.ParentWorktreeID != parent.ID || child.BaseRef != parent.Branch {
		t.Fatal(child)
	}
	if _, err := git(ctx, child.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "advance"); err != nil {
		t.Fatal(err)
	}
	head, err := git(ctx, child.Path, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	grandchild := create("tests", head)
	if grandchild.ParentWorktreeID != child.ID || grandchild.BaseCommit != head {
		t.Fatal(grandchild)
	}
	// Two tips now share this revision: do not choose an arbitrary parent.
	if ambiguous := create("ambiguous", head); ambiguous.ParentWorktreeID != "" {
		t.Fatal(ambiguous)
	}
	if _, err := git(ctx, child.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "advance again"); err != nil {
		t.Fatal(err)
	}
	if replay := create("tests", head); replay.ParentWorktreeID != child.ID {
		t.Fatal(replay)
	}
	if sibling := create("independent", ""); sibling.ParentWorktreeID != "" {
		t.Fatal(sibling)
	}
	status, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if status.Worktrees[2].ParentWorktreeID != child.ID {
		t.Fatal("lineage did not survive reload")
	}
}
