package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Executed as a child by the automatic OpenCode binding test. It stays alive
// until ExecuteSession cancels its context, matching an interactive TUI.
func TestOpenCodeProcess(t *testing.T) {
	if os.Getenv("WORKSPACE_SESSION_ID") == "" {
		return
	}
	time.Sleep(30 * time.Second)
}

func TestOpenCodeSessionPayloadsNormalizeTimes(t *testing.T) {
	nested := []byte(`[{"id":"ses_payload","directory":"/repo","time":{"created":1710000000123,"updated":1710000000456}}]`)
	flat := []byte(`[{"id":"ses_payload","directory":"/repo","created":1710000000123,"updated":1710000000456}]`)
	var fromEndpoint, fromCLI []openCodeSession
	if err := json.Unmarshal(nested, &fromEndpoint); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(flat, &fromCLI); err != nil {
		t.Fatal(err)
	}
	if len(fromEndpoint) != 1 || len(fromCLI) != 1 || fromEndpoint[0] != fromCLI[0] {
		t.Fatalf("payload shapes were not normalized equally: endpoint=%+v cli=%+v", fromEndpoint, fromCLI)
	}
}

func openCodeListCommand(t *testing.T, payload []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode-session-list")
	if runtime.GOOS == "windows" {
		path += ".cmd"
		if err := os.WriteFile(path, []byte("@echo off\r\necho "+string(payload)+"\r\n"), 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	quoted := strings.ReplaceAll(string(payload), "'", "'\"'\"'")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s' '"+quoted+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenCodeSessionListingFallsBackToExecutable(t *testing.T) {
	workspace := t.TempDir()
	now := time.Now().UnixMilli()
	payload, err := json.Marshal([]map[string]any{{
		"id":        "ses_cli_fallback",
		"directory": workspace,
		"created":   now,
		"updated":   now,
	}})
	if err != nil {
		t.Fatal(err)
	}
	command := openCodeListCommand(t, payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	s := &Service{Root: workspace}
	sessions, err := s.listOpenCodeSessions(context.Background(), Session{
		OpenCodeEndpoint: server.URL,
		Argv:             []string{command},
		CWD:              workspace,
	}, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != "ses_cli_fallback" || sessions[0].Created != now || sessions[0].Updated != now {
		t.Fatalf("fallback listing was not decoded: %+v", sessions)
	}
}

func configureOpenCodeTest(t *testing.T, s *Service) {
	t.Helper()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clients["test"] = Client{
		Adapter:    "opencode",
		LaunchArgv: []string{exe, "-test.run=TestOpenCodeProcess", "--", "{prompt_file}"},
		ResumeArgv: []string{exe, "-test.run=TestOpenCodeProcess", "--session", "{thread_id}", "--", "{prompt_file}"},
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
}

func waitForOpenCodeThread(t *testing.T, s *Service, workspace, sessionID, expected string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := s.Status(context.Background(), workspace)
		if err != nil {
			t.Fatal(err)
		}
		for _, session := range status.Sessions {
			if session.ID == sessionID && session.ClientThreadID == expected {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("OpenCode thread was not bound for session %s", sessionID)
}

func TestOpenCodeLaunchAutomaticallyBindsNativeThread(t *testing.T) {
	s, workspace := fixture(t)
	configureOpenCodeTest(t, s)
	agent, worktree := worker(t, s, workspace, "planner")
	nativeID := "ses_auto_launch_test"
	var mu sync.Mutex
	calls := 0
	s.openCodeSessionLister = func(_ context.Context, session Session) ([]openCodeSession, error) {
		mu.Lock()
		calls++
		call := calls
		mu.Unlock()
		if call == 1 {
			return nil, nil
		}
		now := time.Now().UnixMilli()
		payload, err := json.Marshal([]map[string]any{{
			"id":        nativeID,
			"directory": session.CWD,
			"time":      map[string]int64{"created": now, "updated": now},
		}})
		if err != nil {
			return nil, err
		}
		var sessions []openCodeSession
		if err := json.Unmarshal(payload, &sessions); err != nil {
			return nil, err
		}
		return sessions, nil
	}

	started, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.ExecuteSession(ctx, workspace, started.ID, strings.NewReader(""), io.Discard, io.Discard)
	}()
	waitForOpenCodeThread(t, s, workspace, started.ID, nativeID)
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if calls < 2 {
		t.Fatalf("session discovery was not retried: %d calls", calls)
	}
}

func TestSupervisorAutomaticallyBindsExistingOpenCodeThread(t *testing.T) {
	s, workspace := fixture(t)
	configureOpenCodeTest(t, s)
	agent, worktree := worker(t, s, workspace, "planner")
	nativeID := "ses_auto_reconcile_test"
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
		r.State = "running"
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	s.openCodeSessionLister = func(_ context.Context, got Session) ([]openCodeSession, error) {
		now := time.Now().UnixMilli()
		return []openCodeSession{{ID: nativeID, Directory: got.CWD, Created: now, Updated: now}}, nil
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForOpenCodeThread(t, s, workspace, session.ID, nativeID)
}
