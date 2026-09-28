package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestResumeNativeThreadLimitedKeepsThreadAndGuides covers T6: a native-thread
// resume whose only route is limited fails with route_limited, keeps the client
// thread, and tells the caller to wait for the reset or start a new session.
func TestResumeNativeThreadLimitedKeepsThreadAndGuides(t *testing.T) {
	s, id := twoRouteFixture(t, nil)
	ctx := context.Background()
	a, w := worker(t, s, id, "resumed-thread")
	session, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindThread(ctx, id, session.ID, "thread-keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, id, session.ID); err != nil {
		t.Fatal(err)
	}
	reset := nowUTC().Add(45 * time.Minute).Truncate(time.Second)
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/" + session.Route.ID, Kind: "quota_exhausted", Scope: "route", Until: &reset}); err != nil {
		t.Fatal(err)
	}

	_, err = s.ResumeAgent(ctx, id, a.ID, "")
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != "route_limited" {
		t.Fatalf("expected route_limited, got %v", err)
	}
	if !strings.Contains(ce.Message, reset.Format(time.RFC3339)) || !strings.Contains(ce.Message, "new logical session") {
		t.Fatalf("resume message missing reset/new-session guidance: %q", ce.Message)
	}
	options := strings.Join(ce.Options, " ")
	if !strings.Contains(options, "wait_for_reset") || !strings.Contains(options, "new_session") {
		t.Fatalf("resume options missing wait/new-session: %v", ce.Options)
	}
	status, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Sessions) != 1 || status.Sessions[0].ClientThreadID != "thread-keep" || status.Sessions[0].ID != session.ID {
		t.Fatalf("failed resume changed the session or dropped the thread: %+v", status.Sessions)
	}
}

// TestResumeWithoutThreadExcludesLimitedSameClientRoutes covers T6: a
// non-thread resume excludes limited same-client routes and keeps the client
// thread empty.
func TestResumeWithoutThreadExcludesLimitedSameClientRoutes(t *testing.T) {
	s, id := twoRouteFixture(t, nil)
	ctx := context.Background()
	a, w := worker(t, s, id, "resumed-plain")
	session, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if session.Route.ID != "frontier-a" || session.ClientThreadID != "" {
		t.Fatalf("unexpected initial route/thread: %+v", session)
	}
	if _, err := s.StopSession(ctx, id, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "rate_limited", Scope: "route", For: time.Hour}); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.ResumeAgent(ctx, id, a.ID, "")
	if err != nil {
		t.Fatalf("resume should move to the open same-client route: %v", err)
	}
	if resumed.Route.ID != "frontier-b" || resumed.ClientThreadID != "" {
		t.Fatalf("resume did not exclude the limited route: %+v", resumed)
	}
}

// TestResumeAllSameClientRoutesLimitedGuides covers T6: when every same-client
// route is limited, a non-thread resume fails route_limited with the reset and
// the wait-or-new-session options.
func TestResumeAllSameClientRoutesLimitedGuides(t *testing.T) {
	s, id := twoRouteFixture(t, nil)
	ctx := context.Background()
	a, w := worker(t, s, id, "resumed-client")
	session, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, id, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "quota_exhausted", Scope: "client", For: time.Hour}); err != nil {
		t.Fatal(err)
	}
	_, err = s.ResumeAgent(ctx, id, a.ID, "")
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != "route_limited" {
		t.Fatalf("expected route_limited, got %v", err)
	}
	if !strings.Contains(ce.Message, "usage-limited") || !strings.Contains(ce.Message, "new logical session") {
		t.Fatalf("resume message missing shared guidance: %q", ce.Message)
	}
	if _, ok := routeLimitedReset(ce); !ok {
		t.Fatalf("resume error lost the reset option: %v", ce)
	}
}

// TestSupervisorDefersRecoveryWhileRouteLimited covers T6: supervisor
// orchestrator recovery skips retries until the reported reset, does not fail
// every tick, and resumes after the reset.
func TestSupervisorDefersRecoveryWhileRouteLimited(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	p, err := s.StartOrchestrator(ctx, id, "initial")
	if err != nil {
		t.Fatal(err)
	}
	rt := s.Runtime.(*fakeRuntime)
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "quota_exhausted", Scope: "route", For: time.Hour}); err != nil {
		t.Fatal(err)
	}
	delete(rt.panes, p.PaneID)
	if err := s.Tick(ctx); err != nil {
		t.Fatalf("limited recovery failed the tick: %v", err)
	}
	if rt.launches != 1 {
		t.Fatalf("recovery launched while limited: %d", rt.launches)
	}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if rt.launches != 1 {
		t.Fatalf("recovery retried every tick: %d", rt.launches)
	}
	if _, ok := s.recoveryLimitSkip[id]; !ok {
		t.Fatalf("deferral was not recorded: %+v", s.recoveryLimitSkip)
	}
	// A reset clears the in-memory deferral; the ledger record has expired, so
	// the next tick finishes recovery normally.
	if s.recoveryDeferred(id, nowUTC().Add(2*time.Hour)) {
		t.Fatal("deferral did not expire after the reset")
	}
	if _, err := s.ClearRouteLimit(ctx, "", RouteLimitClearOptions{Target: "frontier/frontier-a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if rt.launches != 2 {
		t.Fatalf("recovery did not resume after the reset: %d", rt.launches)
	}
}

// TestAutonomousRetryPrecheckFailsBeforeBoundWhenAllLimited covers T6: an
// autonomous RetryTask on an all-limited profile fails with route_limited
// before consuming the retry bound or mutating the task.
func TestAutonomousRetryPrecheckFailsBeforeBoundWhenAllLimited(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	task := plannedTask(t, s, ws, "retry-limited", "planner", nil)
	p, _ := startTask(t, s, ws, task)
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "quota_exhausted", Scope: "client", For: time.Hour}); err != nil {
		t.Fatal(err)
	}
	_, err := agent.RetryTaskAudited(ctx, ws, task.ID, "checks failed", "retry while the route is limited", nil, "retry-limited-1", MutationGuard{})
	expectCode(t, err, "route_limited")
	st := workspaceStatus(t, s, ws)
	if st.Workspace.Tasks[0].Attempt != 1 {
		t.Fatalf("retry bound or attempt was consumed: %+v", st.Workspace.Tasks[0])
	}
	for _, d := range st.Workspace.Decisions {
		if d.Kind == "autonomous.retry" {
			t.Fatalf("retry decision was recorded despite the pre-check: %+v", d)
		}
	}
}

// TestInteractiveRetryUnaffectedByRouteLimit covers T6: interactive retries keep
// their previous behavior and are not pre-checked.
func TestInteractiveRetryUnaffectedByRouteLimit(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	task := plannedTask(t, s, ws, "retry-interactive", "planner", nil)
	p, _ := startTask(t, s, ws, task)
	if _, err := s.StopSession(ctx, ws, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "quota_exhausted", Scope: "client", For: time.Hour}); err != nil {
		t.Fatal(err)
	}
	updated, err := s.RetryTask(ctx, ws, task.ID, "user retry", "retry-interactive-1")
	if err != nil {
		t.Fatalf("interactive retry was blocked by a route limit: %v", err)
	}
	if updated.Attempt != 2 {
		t.Fatalf("interactive retry did not advance the attempt: %+v", updated)
	}
}
