package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestDecodeOpenCodeSessionStatus(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		thread  string
		want    string
		bad     bool
	}{
		{name: "idle", payload: `{"ses_idle":{"type":"idle"}}`, thread: "ses_idle", want: "idle"},
		{name: "busy", payload: `{"ses_busy":{"type":"busy"}}`, thread: "ses_busy", want: "busy"},
		{name: "retry", payload: `{"ses_retry":{"type":"retry","attempt":2,"message":"retrying","next":123}}`, thread: "ses_retry", want: "retry"},
		{name: "missing", payload: `{"other":{"type":"idle"}}`, thread: "ses_missing", bad: true},
		{name: "malformed", payload: `{"ses_bad":`, thread: "ses_bad", bad: true},
		{name: "unknown", payload: `{"ses_bad":{"type":"paused"}}`, thread: "ses_bad", bad: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeOpenCodeSessionStatus([]byte(test.payload), test.thread)
			if test.bad {
				if err == nil {
					t.Fatalf("invalid status unexpectedly decoded as %q", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("status = %q, err = %v; want %q", got, err, test.want)
			}
		})
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
