package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
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
		Adapter:     "opencode",
		LaunchArgv:  []string{exe, "-test.run=TestOpenCodeProcess", "--", "{prompt_file}"},
		ResumeArgv:  []string{exe, "-test.run=TestOpenCodeProcess", "--session", "{thread_id}", "--", "{prompt_file}"},
		DeliverArgv: []string{"echo", `{"accepted":true}`},
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
		return []openCodeSession{{ID: nativeID, Directory: session.CWD, Created: time.Now().UnixMilli(), Updated: time.Now().UnixMilli()}}, nil
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
