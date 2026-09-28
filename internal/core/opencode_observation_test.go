package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestDecodeOpenCodeSessionStatus(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		thread  string
		want    openCodeSessionStatus
		bad     bool
	}{
		{name: "idle", payload: `{"ses_idle":{"type":"idle"}}`, thread: "ses_idle", want: openCodeSessionStatus{Type: "idle"}},
		{name: "busy", payload: `{"ses_busy":{"type":"busy"}}`, thread: "ses_busy", want: openCodeSessionStatus{Type: "busy"}},
		{name: "retry", payload: `{"ses_retry":{"type":"retry","attempt":2,"message":"retrying","next":123}}`, thread: "ses_retry", want: openCodeSessionStatus{Type: "retry", Attempt: 2, Message: "retrying", Next: 123}},
		{name: "retry-ignores-unknown-fields", payload: `{"ses_retry":{"type":"retry","attempt":1,"message":"m","next":7,"futureField":{"x":1}},"other":{"type":"idle"}}`, thread: "ses_retry", want: openCodeSessionStatus{Type: "retry", Attempt: 1, Message: "m", Next: 7}},
		{name: "missing", payload: `{"other":{"type":"idle"}}`, thread: "ses_missing", bad: true},
		{name: "malformed", payload: `{"ses_bad":`, thread: "ses_bad", bad: true},
		{name: "unknown", payload: `{"ses_bad":{"type":"paused"}}`, thread: "ses_bad", bad: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeOpenCodeSessionStatus([]byte(test.payload), test.thread)
			if test.bad {
				if err == nil {
					t.Fatalf("invalid status unexpectedly decoded as %+v", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("status = %+v, err = %v; want %+v", got, err, test.want)
			}
		})
	}
}

func TestClassifyOpenCodeRetryMessage(t *testing.T) {
	for _, test := range []struct {
		message string
		kind    string
		ok      bool
	}{
		{message: "429 Too Many Requests", kind: "rate_limited", ok: true},
		{message: "Rate limit exceeded. Please retry later.", kind: "rate_limited", ok: true},
		{message: "You have hit your usage limit", kind: "quota_exhausted", ok: true},
		{message: "insufficient_quota", kind: "quota_exhausted", ok: true},
		{message: "RESOURCE_EXHAUSTED", kind: "quota_exhausted", ok: true},
		{message: "Quota exceeded for the month", kind: "quota_exhausted", ok: true},
		{message: "provider is overloaded, retrying", ok: false},
		{message: "network connection lost", ok: false},
		{message: "", ok: false},
	} {
		kind, ok := classifyOpenCodeRetryMessage(test.message)
		if ok != test.ok || kind != test.kind {
			t.Fatalf("classify(%q) = (%q, %v); want (%q, %v)", test.message, kind, ok, test.kind, test.ok)
		}
	}
}

func TestOpenCodeRetryLimitObservation(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	future := now.Add(30 * time.Minute).UnixMilli()
	past := now.Add(-time.Minute).UnixMilli()
	obs, ok := openCodeRetryLimitObservation(openCodeSessionStatus{Type: "retry", Message: "rate limit exceeded", Next: future}, now)
	if !ok || obs.Kind != "rate_limited" || obs.Source != "opencode_retry" || obs.ResetAt == nil || !obs.ResetAt.Equal(time.UnixMilli(future).UTC()) {
		t.Fatalf("future reset observation = %+v ok=%v", obs, ok)
	}
	obs, ok = openCodeRetryLimitObservation(openCodeSessionStatus{Type: "retry", Message: "quota exceeded", Next: past}, now)
	if !ok || obs.Kind != "quota_exhausted" || obs.ResetAt != nil {
		t.Fatalf("a past next must fall back to backoff: %+v ok=%v", obs, ok)
	}
	if _, ok := openCodeRetryLimitObservation(openCodeSessionStatus{Type: "retry", Message: "provider overloaded"}, now); ok {
		t.Fatal("a non-limit retry must not become a limit observation")
	}
}

func openCodeObservationFixture(t *testing.T, endpoint, thread, clientState string) (*Service, string, Session) {
	t.Helper()
	s, workspace := fixture(t)
	agent, worktree := worker(t, s, workspace, "opencode-observer")
	session, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.With(context.Background(), workspace, func(d *Document) error {
		p, err := findSession(d, session.ID)
		if err != nil {
			return err
		}
		r, err := currentRun(d, p)
		if err != nil {
			return err
		}
		p.ClientSnapshot = Client{Adapter: "opencode"}
		p.ClientThreadID = thread
		r.State, r.ClientState = "running", clientState
		r.ClientThreadID, r.OpenCodeEndpoint = thread, endpoint
		d.syncSession(p)
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range status.Sessions {
		if candidate.ID == session.ID {
			return s, workspace, candidate
		}
	}
	t.Fatalf("observation Session %s was not persisted", session.ID)
	return nil, "", Session{}
}

func openCodeStatusServer(t *testing.T, thread, state string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/status" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"%s":{"type":"%s"}}`, thread, state))
	}))
}

func TestOpenCodeSupervisorPersistsActivityObservation(t *testing.T) {
	for _, state := range []string{"idle", "busy", "retry"} {
		t.Run(state, func(t *testing.T) {
			thread := "ses_" + state
			server := openCodeStatusServer(t, thread, state)
			defer server.Close()
			s, workspace, session := openCodeObservationFixture(t, server.URL, thread, "busy")

			s.observeOpenCodeActivity(context.Background(), workspace, map[string]Session{session.ID: session})
			status, err := s.Status(context.Background(), workspace)
			if err != nil {
				t.Fatal(err)
			}
			got, err := findRunSession(status, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.ClientState != state || got.RunState != "running" || !got.Active() {
				t.Fatalf("OpenCode observation %q was not projected conservatively: %+v", state, got)
			}
			if got.State != "running" || got.LifecycleState != "active" {
				t.Fatalf("OpenCode observation %q changed Session projection incorrectly: %+v", state, got)
			}
		})
	}
}

func TestOpenCodeObservationFailuresPreserveLastKnownState(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		block  bool
	}{
		{name: "missing-thread", status: http.StatusOK, body: `{"other":{"type":"idle"}}`},
		{name: "malformed", status: http.StatusOK, body: `{"thread":`},
		{name: "non-2xx", status: http.StatusBadGateway, body: `rejected`},
		{name: "timeout", block: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread := "ses_preserve"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.block {
					<-r.Context().Done()
					return
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			s, workspace, session := openCodeObservationFixture(t, server.URL, thread, "busy")
			before, err := s.Status(context.Background(), workspace)
			if err != nil {
				t.Fatal(err)
			}

			s.observeOpenCodeActivity(context.Background(), workspace, map[string]Session{session.ID: session})
			after, err := s.Status(context.Background(), workspace)
			if err != nil {
				t.Fatal(err)
			}
			got, err := findRunSession(after, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.ClientState != "busy" || got.State != "running" || after.Workspace.Revision != before.Workspace.Revision {
				t.Fatalf("failed observation changed durable state: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestOpenCodeObservationUnchangedStateDoesNotBumpRevision(t *testing.T) {
	thread := "ses_unchanged"
	server := openCodeStatusServer(t, thread, "idle")
	defer server.Close()
	s, workspace, session := openCodeObservationFixture(t, server.URL, thread, "idle")
	before, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	s.observeOpenCodeActivity(context.Background(), workspace, map[string]Session{session.ID: session})
	after, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if after.Workspace.Revision != before.Workspace.Revision {
		t.Fatalf("unchanged OpenCode observation bumped revision: %d -> %d", before.Workspace.Revision, after.Workspace.Revision)
	}
}

func TestOpenCodeObservationDoesNotOverwriteSuccessorRun(t *testing.T) {
	thread := "ses_stale"
	var (
		s         *Service
		workspace string
		session   Session
		once      sync.Once
	)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			if err := s.With(context.Background(), workspace, func(d *Document) error {
				var p *Session
				for i := range d.Registry.Sessions {
					if d.Registry.Sessions[i].ClientThreadID == thread {
						p = &d.Registry.Sessions[i]
						break
					}
				}
				if p == nil {
					return fail("session_not_found", "stale observation fixture Session not found")
				}
				old, err := findRun(d, p.CurrentRunID)
				if err != nil {
					return err
				}
				old.State = "interrupted"
				newRun := Run{ID: "run_successor", SessionID: p.ID, State: "running", ClientState: "busy", ClientThreadID: "ses_successor", OpenCodeEndpoint: server.URL, CreatedAt: nowUTC()}
				d.Registry.Runs = append(d.Registry.Runs, newRun)
				p.CurrentRunID = newRun.ID
				d.syncSession(p)
				return saveDocument(d)
			}); err != nil {
				return
			}
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"%s":{"type":"idle"}}`, thread))
	}))
	defer server.Close()
	s, workspace, session = openCodeObservationFixture(t, server.URL, thread, "busy")
	s.observeOpenCodeActivity(context.Background(), workspace, map[string]Session{session.ID: session})
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	current, err := findRunSession(status, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRunID != "run_successor" || current.ClientState != "busy" || current.State != "running" {
		t.Fatalf("stale OpenCode observation overwrote successor Run: %+v", current)
	}
}

func openCodeRetryStatusServer(t *testing.T, thread, message string, next int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/status" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]any{thread: map[string]any{"type": "retry", "attempt": 2, "message": message, "next": next}})
		_, _ = w.Write(body)
	}))
}

func TestOpenCodeSupervisorRecordsRetryLimit(t *testing.T) {
	thread := "ses_limited"
	next := time.Now().Add(45 * time.Minute).UnixMilli()
	server := openCodeRetryStatusServer(t, thread, "429 rate limit exceeded", next)
	defer server.Close()
	s, workspace, session := openCodeObservationFixture(t, server.URL, thread, "busy")

	s.observeOpenCodeActivity(context.Background(), workspace, map[string]Session{session.ID: session})

	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	got, err := findRunSession(status, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientState != "retry" || got.State != "running" {
		t.Fatalf("limit retry observation not persisted conservatively: %+v", got)
	}
	limits, err := s.ListRouteLimits(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(limits.Limits) != 1 {
		t.Fatalf("want exactly one recorded limit, got %+v", limits)
	}
	rec := limits.Limits[0]
	if rec.Scope != "provider" || rec.Client != session.Route.Client || rec.Provider != session.Route.Provider ||
		rec.Kind != "rate_limited" || rec.Source != "opencode_retry" || rec.RunID != session.CurrentRunID || rec.WorkspaceID != workspace {
		t.Fatalf("retry limit not bound to the calling Run's provider route: %+v (route %+v)", rec, session.Route)
	}
	if rec.ResetAt == nil || !rec.ResetAt.Equal(time.UnixMilli(next).UTC()) {
		t.Fatalf("retry Next lower bound not applied: %+v", rec.ResetAt)
	}
}

func TestOpenCodeSupervisorIgnoresNonLimitStatus(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
	}{
		{name: "idle", payload: `{"ses_idle":{"type":"idle"}}`},
		{name: "busy", payload: `{"ses_busy":{"type":"busy"}}`},
		{name: "retry-non-limit", payload: `{"ses_retry":{"type":"retry","attempt":2,"message":"provider overloaded, retrying","next":123}}`},
		{name: "retry-empty", payload: `{"ses_retry":{"type":"retry","attempt":2,"message":"","next":123}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, test.payload)
			}))
			defer server.Close()
			s, workspace, session := openCodeObservationFixture(t, server.URL, "ses_"+test.name, "busy")
			s.observeOpenCodeActivity(context.Background(), workspace, map[string]Session{session.ID: session})
			limits, err := s.ListRouteLimits(context.Background(), true)
			if err != nil {
				t.Fatal(err)
			}
			if len(limits.Limits) != 0 {
				t.Fatalf("non-limit status %q recorded a limit: %+v", test.name, limits.Limits)
			}
		})
	}
}

func TestOpenCodeStaleRetryLimitIsNoOp(t *testing.T) {
	thread := "ses_stale_limit"
	var (
		s         *Service
		workspace string
		session   Session
		once      sync.Once
	)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			if err := s.With(context.Background(), workspace, func(d *Document) error {
				var p *Session
				for i := range d.Registry.Sessions {
					if d.Registry.Sessions[i].ClientThreadID == thread {
						p = &d.Registry.Sessions[i]
						break
					}
				}
				if p == nil {
					return fail("session_not_found", "stale observation fixture Session not found")
				}
				old, err := findRun(d, p.CurrentRunID)
				if err != nil {
					return err
				}
				old.State = "interrupted"
				newRun := Run{ID: "run_successor", SessionID: p.ID, State: "running", ClientState: "busy", ClientThreadID: "ses_successor", OpenCodeEndpoint: server.URL, CreatedAt: nowUTC()}
				d.Registry.Runs = append(d.Registry.Runs, newRun)
				p.CurrentRunID = newRun.ID
				d.syncSession(p)
				return saveDocument(d)
			}); err != nil {
				return
			}
		})
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]any{thread: map[string]any{"type": "retry", "attempt": 1, "message": "quota exceeded", "next": time.Now().Add(time.Hour).UnixMilli()}})
		_, _ = w.Write(body)
	}))
	defer server.Close()
	s, workspace, session = openCodeObservationFixture(t, server.URL, thread, "busy")
	s.observeOpenCodeActivity(context.Background(), workspace, map[string]Session{session.ID: session})
	limits, err := s.ListRouteLimits(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(limits.Limits) != 0 {
		t.Fatalf("stale Run retry limit was recorded: %+v", limits.Limits)
	}
}
