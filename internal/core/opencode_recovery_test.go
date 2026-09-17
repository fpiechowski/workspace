package core

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"
)

func resumeOpenCodeTestSession(t *testing.T, s *Service, workspace string, session Session) (Session, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// ResumeSession deliberately refuses to start a tmux supervisor on
		// Windows. StartSession exercises the same core resume path for the
		// fake runtime used by these tests.
		return s.StartSession(context.Background(), workspace, SessionOptions{ResumeSession: session.ID})
	}
	return s.ResumeSession(context.Background(), workspace, session.ID, "resume:"+session.ID)
}

func stopOpenCodeTestRun(t *testing.T, s *Service, workspace string, session Session) Session {
	t.Helper()
	stopped, err := s.StopSession(context.Background(), workspace, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	return stopped
}

func TestOpenCodeFirstBindingIsReusedByTheNextRun(t *testing.T) {
	s, workspace := fixture(t)
	configureOpenCodeTest(t, s)
	agent, worktree := worker(t, s, workspace, "opencode-resume")
	nativeID := "ses_first_binding"
	var calls int
	s.openCodeSessionLister = func(_ context.Context, session Session) ([]openCodeSession, error) {
		calls++
		if calls == 1 {
			return nil, nil
		}
		now := time.Now().UnixMilli()
		return []openCodeSession{{ID: nativeID, Directory: session.CWD, Created: now, Updated: now}}, nil
	}

	first, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		runDone <- s.ExecuteSession(ctx, workspace, first.ID, strings.NewReader(""), io.Discard, io.Discard)
	}()
	waitForOpenCodeThread(t, s, workspace, first.ID, nativeID)
	cancel()
	_ = <-runDone

	resumed, err := resumeOpenCodeTestSession(t, s, workspace, first)
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID != first.ID || resumed.ClientThreadID != nativeID || resumed.RunCount != 2 || resumed.CurrentRunID == first.CurrentRunID {
		t.Fatalf("logical Session/Run lineage was not preserved: first=%+v resumed=%+v", first, resumed)
	}
	if len(status.Runs) != 2 || status.Runs[0].ClientThreadID != nativeID || status.Runs[1].ClientThreadID != nativeID || !hasArg(status.Runs[1].Argv, "--session") || !hasArg(status.Runs[1].Argv, nativeID) {
		t.Fatalf("successor did not use the original OpenCode thread: %+v", status.Runs)
	}
}

func TestOpenCodeResumeRepairsOldEmptyBindingByEarliestRun(t *testing.T) {
	s, workspace := fixture(t)
	configureOpenCodeTest(t, s)
	agent, worktree := worker(t, s, workspace, "opencode-repair")
	ready := false
	s.openCodeSessionLister = func(_ context.Context, _ Session) ([]openCodeSession, error) {
		if !ready {
			return nil, nil
		}
		return nil, nil
	}

	first, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	first = stopOpenCodeTestRun(t, s, workspace, first)
	second, err := s.StartSession(context.Background(), workspace, SessionOptions{ResumeSession: first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if hasArg(second.Argv, "--session") {
		t.Fatalf("empty historical listing unexpectedly selected a resume argv: %+v", second.Argv)
	}
	second = stopOpenCodeTestRun(t, s, workspace, second)
	third, err := s.StartSession(context.Background(), workspace, SessionOptions{ResumeSession: second.ID})
	if err != nil {
		t.Fatal(err)
	}
	if hasArg(third.Argv, "--session") {
		t.Fatalf("empty historical listing unexpectedly selected a resume argv: %+v", third.Argv)
	}
	third = stopOpenCodeTestRun(t, s, workspace, third)

	base := time.Now().UTC().Add(-10 * time.Minute)
	var runs []Run
	if err := s.With(context.Background(), workspace, func(d *Document) error {
		for i := range d.Registry.Runs {
			created := base.Add(time.Duration(i) * time.Minute)
			finished := created.Add(20 * time.Second)
			d.Registry.Runs[i].CreatedAt = created
			d.Registry.Runs[i].FinishedAt = &finished
			d.Registry.Runs[i].State = "stopped"
			d.Registry.Runs[i].ClientThreadID = ""
		}
		if err := saveDocument(d); err != nil {
			return err
		}
		runs = append(runs, d.Registry.Runs...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected three historical Runs, got %d", len(runs))
	}

	ready = true
	s.openCodeSessionLister = func(_ context.Context, session Session) ([]openCodeSession, error) {
		return []openCodeSession{
			{ID: "ses_original", Directory: session.CWD, Created: base.Add(4 * time.Second).UnixMilli(), Updated: base.Add(4 * time.Second).UnixMilli()},
			{ID: "ses_later", Directory: session.CWD, Created: base.Add(time.Minute + 4*time.Second).UnixMilli(), Updated: base.Add(time.Minute + 4*time.Second).UnixMilli()},
			{ID: "ses_other_cwd", Directory: workspace, Created: base.Add(3 * time.Second).UnixMilli(), Updated: base.Add(3 * time.Second).UnixMilli()},
		}, nil
	}
	resumed, err := resumeOpenCodeTestSession(t, s, workspace, third)
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID != first.ID || resumed.ClientThreadID != "ses_original" || resumed.RunCount != 4 {
		t.Fatalf("repair did not preserve the logical Session: %+v", resumed)
	}
	if status.Runs[0].ClientThreadID != "ses_original" || status.Runs[3].ClientThreadID != "ses_original" {
		t.Fatalf("binding was not copied to the original and successor Runs: %+v", status.Runs)
	}
	if !hasArg(status.Runs[3].Argv, "--session") || !hasArg(status.Runs[3].Argv, "ses_original") {
		t.Fatalf("successor argv did not resume the repaired thread: %+v", status.Runs[3].Argv)
	}
}

func TestOpenCodeResumeFailsClosedWhenHistoricalCorrelationIsAmbiguous(t *testing.T) {
	s, workspace := fixture(t)
	configureOpenCodeTest(t, s)
	agent, worktree := worker(t, s, workspace, "opencode-ambiguous")
	first, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	first = stopOpenCodeTestRun(t, s, workspace, first)
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	run := status.Runs[0]
	s.openCodeSessionLister = func(_ context.Context, session Session) ([]openCodeSession, error) {
		created := run.CreatedAt.Add(2 * time.Second).UnixMilli()
		return []openCodeSession{
			{ID: "ses_ambiguous_a", Directory: session.CWD, Created: created, Updated: created},
			{ID: "ses_ambiguous_b", Directory: session.CWD, Created: created + 1, Updated: created + 1},
		}, nil
	}

	_, err = s.StartSession(context.Background(), workspace, SessionOptions{ResumeSession: first.ID})
	expectCode(t, err, "opencode_thread_ambiguous")
	if !strings.Contains(err.Error(), "workspace session bind-thread") {
		t.Fatalf("ambiguity error lacks manual recovery instruction: %v", err)
	}
	after, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Runs) != 1 || after.Sessions[0].ClientThreadID != "" {
		t.Fatalf("ambiguous repair launched or bound a Run: %+v", after)
	}
}

func TestOpenCodeResumeFailsClosedWhenHistoricalListingIsUnavailable(t *testing.T) {
	s, workspace := fixture(t)
	configureOpenCodeTest(t, s)
	agent, worktree := worker(t, s, workspace, "opencode-unavailable")
	first, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	first = stopOpenCodeTestRun(t, s, workspace, first)
	s.openCodeSessionLister = func(context.Context, Session) ([]openCodeSession, error) {
		return nil, errors.New("session store unavailable")
	}

	_, err = s.StartSession(context.Background(), workspace, SessionOptions{ResumeSession: first.ID})
	expectCode(t, err, "opencode_thread_recovery")
	if !strings.Contains(err.Error(), "workspace session bind-thread") {
		t.Fatalf("unavailable-list error lacks manual recovery instruction: %v", err)
	}
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Runs) != 1 {
		t.Fatalf("unavailable recovery launched a new Run: %+v", status.Runs)
	}
}

func TestOpenCodeRepairExcludesThreadOwnedByAnotherActiveSession(t *testing.T) {
	s, workspace := fixture(t)
	configureOpenCodeTest(t, s)
	agent, worktree := worker(t, s, workspace, "opencode-owned")
	first, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	first = stopOpenCodeTestRun(t, s, workspace, first)
	otherAgent, otherWorktree := worker(t, s, workspace, "opencode-other")
	other, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: otherAgent.ID, Worktree: otherWorktree.ID, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindThread(context.Background(), workspace, other.ID, "ses_claimed"); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	run := status.Runs[0]
	s.openCodeSessionLister = func(_ context.Context, session Session) ([]openCodeSession, error) {
		created := run.CreatedAt.Add(2 * time.Second).UnixMilli()
		return []openCodeSession{{ID: "ses_claimed", Directory: session.CWD, Created: created, Updated: created}}, nil
	}

	resumed, err := s.StartSession(context.Background(), workspace, SessionOptions{ResumeSession: first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ClientThreadID != "" || hasArg(resumed.Argv, "--session") {
		t.Fatalf("thread owned by another active Session was reused: %+v", resumed)
	}
}

func TestOpenCodeRepairPreservesAwaitingReviewProvenance(t *testing.T) {
	s, workspace := fixture(t)
	configureOpenCodeTest(t, s)
	task := plannedTask(t, s, workspace, "opencode-review", "planner", nil)
	p, worktree := startTask(t, s, workspace, task)
	handoff := submitPlan(t, s, workspace, p, worktree)
	if _, err := s.StopSession(context.Background(), workspace, p.ID); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	var runCreated time.Time
	for _, candidate := range status.Runs {
		if candidate.ID == handoff.FromRun {
			runCreated = candidate.CreatedAt
			break
		}
	}
	if runCreated.IsZero() {
		t.Fatalf("handoff Run %s not found", handoff.FromRun)
	}
	s.openCodeSessionLister = func(_ context.Context, session Session) ([]openCodeSession, error) {
		created := runCreated.Add(2 * time.Second).UnixMilli()
		return []openCodeSession{{ID: "ses_review_repair", Directory: session.CWD, Created: created, Updated: created}}, nil
	}

	resumed, err := resumeOpenCodeTestSession(t, s, workspace, p)
	if err != nil {
		t.Fatal(err)
	}
	var storedTask Task
	var storedHandoff Handoff
	if err := s.With(context.Background(), workspace, func(d *Document) error {
		var err error
		foundTask, err := findTask(d, task.ID)
		if err != nil {
			return err
		}
		storedTask = *foundTask
		foundHandoff, err := findHandoff(d, handoff.ID)
		if err != nil {
			return err
		}
		storedHandoff = *foundHandoff
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if resumed.ClientThreadID != "ses_review_repair" || storedTask.State != "awaiting_review" || storedTask.RunID != handoff.FromRun || storedHandoff.FromRun != handoff.FromRun || storedHandoff.State != "submitted" {
		t.Fatalf("resume changed review provenance: resumed=%+v task=%+v handoff=%+v", resumed, storedTask, storedHandoff)
	}
}
