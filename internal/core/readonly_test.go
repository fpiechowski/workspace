package core

import (
	"context"
	"testing"
)

func TestReadOnlySessionsShareWorktreeWithoutReleasingWriter(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	writer, w := worker(t, s, ws, "writer")
	wp, err := s.StartSession(ctx, ws, SessionOptions{Agent: writer.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "reader", Role: "planner"})
	if err != nil {
		t.Fatal(err)
	}
	rp, err := s.StartSession(ctx, ws, SessionOptions{Agent: reader.ID, Worktree: w.ID, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rp.ReadOnly || rp.WorktreeID != wp.WorktreeID {
		t.Fatal("reader not assigned to shared worktree")
	}
	another, err := s.CreateAgent(ctx, ws, AgentOptions{Name: "another", Role: "planner"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.StartSession(ctx, ws, SessionOptions{Agent: another.ID, Worktree: w.ID})
	expectCode(t, err, "worktree_busy")
	if _, err := s.StopSession(ctx, ws, rp.ID); err != nil {
		t.Fatal(err)
	}
	rp, err = s.ResumeAgent(ctx, ws, reader.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !rp.ReadOnly {
		t.Fatal("resume acquired write rights")
	}
}
