package core

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestCodexNativeWakeupAndResume(t *testing.T) {
	s, ws := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	cfg.Clients["test"] = Client{Adapter: "codex", LaunchArgv: []string{exe, "-test.run=TestAppServerProcess", "--", "app-server-helper"}, ThreadParams: map[string]any{"effort": "client-default", "approvalPolicy": "never"}}
	profile := cfg.Profiles["frontier"]
	profile.ReasoningEffort = "configured-effort"
	cfg.Profiles["frontier"] = profile
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	p, err := s.StartOrchestrator(ctx, ws, "")
	if err != nil {
		t.Fatal(err)
	}
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	done := make(chan error, 1)
	go func() { done <- s.ExecuteSession(ctx, ws, p.ID, r, io.Discard, io.Discard) }()
	waitFor := func(fn func(Status) bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			v, err := s.Status(ctx, ws)
			if err != nil {
				t.Fatal(err)
			}
			if fn(v) {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatal("native client did not reach expected state")
	}
	waitFor(func(v Status) bool { return v.Sessions[0].ClientState == "idle" && v.Sessions[0].ClientThreadID != "" })
	if session := findSessionInStatusValue(t, s, ctx, ws, p.ID); session.State != "running" || session.RunState != "running" || session.ClientState != "idle" || session.LifecycleState != "active" || !session.Active() {
		t.Fatalf("Codex idle observation did not preserve the active operational Run projection: %+v", session)
	}
	startParams := readCodexThreadParams(t, filepath.Join(p.CWD, "work-products", "thread-thread-start.json"))
	if startParams["effort"] != "configured-effort" || startParams["approvalPolicy"] != "never" {
		t.Fatalf("thread/start did not receive the profile effort or other params: %#v", startParams)
	}
	m, err := s.SendMessage(ctx, ws, MessageOptions{To: p.AgentID, Body: "A worker has completed planning."})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(func(v Status) bool {
		messages, err := s.Inbox(ctx, ws, p.AgentID, true)
		if err != nil {
			return false
		}
		for _, message := range messages {
			if message.ID == m.ID && message.DeliveredSessionID == p.ID && message.DeliveredRunID == p.CurrentRunID {
				return true
			}
		}
		return false
	})
	if _, err := io.WriteString(w, "/quit\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("native client did not exit")
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Status = "completed"
		if d.State.Workflow != nil {
			d.State.Workflow.Phase = "completed"
		}
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.ResumeAgent(ctx, ws, p.AgentID, "")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ClientThreadID != "test-native-thread" {
		t.Fatal("native thread was not preserved")
	}
	r2, w2 := io.Pipe()
	defer r2.Close()
	defer w2.Close()
	go func() { done <- s.ExecuteSession(ctx, ws, resumed.ID, r2, io.Discard, io.Discard) }()
	waitFor(func(v Status) bool {
		return len(v.Sessions) == 1 && len(v.Runs) == 2 && v.Sessions[0].ClientState == "idle"
	})
	resumeParams := readCodexThreadParams(t, filepath.Join(p.CWD, "work-products", "thread-thread-resume.json"))
	if resumeParams["effort"] != "configured-effort" || resumeParams["approvalPolicy"] != "never" {
		t.Fatalf("thread/resume did not receive the profile effort or other params: %#v", resumeParams)
	}
	data, err := os.ReadFile(filepath.Join(p.CWD, "work-products", "thread-method.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "thread/resume" {
		t.Fatalf("expected native resume, got %s", data)
	}
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	currentRun, err := findRunInStatus(status, resumed.CurrentRunID)
	if err != nil {
		t.Fatal(err)
	}
	if !currentRun.ConversationOnly {
		t.Fatal("completed native resume was not marked conversation_only")
	}
	exact, err := s.SendMessage(ctx, ws, MessageOptions{To: p.AgentID, ToSession: resumed.ID, Body: "A user has a question about the completed result."})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(func(v Status) bool {
		messages, err := s.Inbox(ctx, ws, p.AgentID, true)
		if err != nil {
			return false
		}
		for _, message := range messages {
			if message.ID == exact.ID && message.DeliveredSessionID == resumed.ID && message.DeliveredRunID == resumed.CurrentRunID {
				return true
			}
		}
		return false
	})
	io.WriteString(w2, "/quit\n")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resumed client did not exit")
	}
}

func TestCodexClientActivityProjectionAcrossTurn(t *testing.T) {
	s, ws := fixture(t)
	agent, worktree := worker(t, s, ws, "codex-state")
	session, err := s.StartSession(context.Background(), ws, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		clientState string
	}{
		{clientState: "busy"},
		{clientState: "needs_input"},
		{clientState: "idle"},
	} {
		if err := s.With(context.Background(), ws, func(d *Document) error {
			p, err := findSession(d, session.ID)
			if err != nil {
				return err
			}
			r, err := currentRun(d, p)
			if err != nil {
				return err
			}
			r.State, r.ClientState = "running", test.clientState
			d.syncSession(p)
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
		status, err := s.Status(context.Background(), ws)
		if err != nil {
			t.Fatal(err)
		}
		got, err := findRunSession(status, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State != "running" || got.RunState != "running" || got.ClientState != test.clientState || !got.Active() {
			t.Fatalf("Codex client state %q projected as %+v", test.clientState, got)
		}
	}
}

func findSessionInStatusValue(t *testing.T, s *Service, ctx context.Context, workspace, id string) Session {
	t.Helper()
	status, err := s.Status(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range status.Sessions {
		if session.ID == id {
			return session
		}
	}
	t.Fatalf("session %s not found", id)
	return Session{}
}

func findRunSession(status Status, id string) (Session, error) {
	for _, session := range status.Sessions {
		if session.ID == id {
			return session, nil
		}
	}
	return Session{}, fail("session_not_found", "unknown session %s", id)
}

// A protocol fixture: no model calls, credentials or external account are used.
func TestAppServerProcess(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "app-server-helper" {
		return
	}
	encoder := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	for scanner.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		switch req.Method {
		case "initialize":
			encoder.Encode(map[string]any{"id": req.ID, "result": map[string]any{}})
		case "initialized":
		case "thread/start", "thread/resume":
			if err := atomicWrite(filepath.Join("work-products", "thread-method.txt"), []byte(req.Method)); err != nil {
				os.Exit(3)
			}
			params, _ := json.Marshal(req.Params)
			if err := atomicWrite(filepath.Join("work-products", "thread-"+strings.ReplaceAll(req.Method, "/", "-")+".json"), params); err != nil {
				os.Exit(3)
			}
			encoder.Encode(map[string]any{"id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "test-native-thread"}}})
		case "turn/start":
			encoder.Encode(map[string]any{"id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-1"}}})
			encoder.Encode(map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{"delta": "Fixture response"}})
			encoder.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"status": "completed"}}})
		default:
			if strings.TrimSpace(req.Method) != "" {
				encoder.Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -32601, "message": "unsupported"}})
			}
		}
	}
	os.Exit(0)
}

func readCodexThreadParams(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	if err := json.Unmarshal(b, &params); err != nil {
		t.Fatal(err)
	}
	return params
}
