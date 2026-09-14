package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckCommandProcess(t *testing.T) {
	if os.Args[len(os.Args)-1] == "workspace-check-failure" {
		fmt.Fprintln(os.Stderr, "real failing check")
		os.Exit(3)
	}
	if os.Args[len(os.Args)-1] != "workspace-check-fixture" {
		return
	}
	fmt.Fprintln(os.Stdout, "real check stdout")
	fmt.Fprintln(os.Stderr, "real check stderr")
	os.Exit(0)
}
func TestCapturedCheckProvenanceAndHandoff(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	a, w := worker(t, s, id, "planner")
	task, err := s.CreateTask(ctx, id, TaskSpec{Title: "Investigate", Goal: "Diagnose issue", Role: "planner", AcceptanceCriteria: []string{"Evidence"}}, "task")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID, Task: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	opt := CheckOptions{Session: p.ID, OperationKey: "check:1", Argv: []string{exe, "-test.run=TestCheckCommandProcess", "--", "workspace-check-fixture"}}
	r, err := s.RunCheck(ctx, id, opt)
	if err != nil {
		t.Fatal(err)
	}
	if r.SessionID != p.ID || r.RunID != p.CurrentRunID || r.ExitCode != 0 || r.State != "completed" || !r.Clean || r.Head != r.EndHead {
		t.Fatalf("invalid receipt %+v", r)
	}
	replay, err := s.RunCheck(ctx, id, opt)
	if err != nil || replay.ID != r.ID {
		t.Fatal("check replay failed", err)
	}
	if err := os.WriteFile(filepath.Join(w.Path, r.Output), []byte("forged local output"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(w.Path, "work-products", "PLAN.md")
	if err := os.WriteFile(plan, []byte("analysis"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := s.SubmitHandoff(ctx, id, HandoffOptions{Session: p.ID, Task: task.ID, Summary: "Done", Artifacts: []string{plan}, CheckIDs: []string{r.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Checks) != 1 {
		t.Fatal("captured check missing")
	}
	if err := s.With(ctx, id, func(d *Document) error {
		artifact, _ := findArtifact(d, h.Checks[0].Evidence)
		b, err := os.ReadFile(filepath.Join(d.Dir, artifact.Path))
		if err != nil {
			return err
		}
		if digest(b) != r.Digest {
			t.Fatal("handoff used modified worktree output")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, w.Path, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "new revision"); err != nil {
		t.Fatal(err)
	}
	_, err = s.SubmitHandoff(ctx, id, HandoffOptions{Session: p.ID, Task: task.ID, Summary: "New revision", Artifacts: []string{plan}, CheckIDs: []string{r.ID}})
	expectCode(t, err, "invalid_checks")
	opt.OperationKey = "failed-check"
	opt.Argv[len(opt.Argv)-1] = "workspace-check-failure"
	failed, err := s.RunCheck(ctx, id, opt)
	if err != nil {
		t.Fatal(err)
	}
	if failed.ExitCode != 3 || failed.State != "completed" {
		t.Fatalf("failure lost: %+v", failed)
	}
	h, err = s.SubmitHandoff(ctx, id, HandoffOptions{Session: p.ID, Summary: "Checks failed", Artifacts: []string{plan}, CheckIDs: []string{failed.ID}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ReviewHandoff(ctx, id, h.ID, true, "")
	expectCode(t, err, "checks_failed")
}

func TestCheckSetupFailureReleasesReservation(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "plan", "planner", nil)
	p, w := startTask(t, s, ws, task)
	if err := os.WriteFile(filepath.Join(w.Path, "work-products"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	_, err := s.RunCheck(ctx, ws, CheckOptions{Session: p.ID, Argv: []string{exe, "-test.run=TestCheckCommandProcess", "--", "workspace-check-fixture"}})
	if err == nil {
		t.Fatal("setup unexpectedly succeeded")
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		if len(d.Registry.Checks) != 1 || d.Registry.Checks[0].State != "failed" {
			t.Fatal("check reservation leaked")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
