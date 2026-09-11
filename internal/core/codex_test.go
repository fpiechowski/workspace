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
	cfg.Clients["test"] = Client{Adapter: "codex", LaunchArgv: []string{exe, "-test.run=TestAppServerProcess", "--", "app-server-helper"}}
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
			if message.ID == m.ID && message.DeliveredSessionID == p.ID {
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
	waitFor(func(v Status) bool { return len(v.Sessions) == 2 && v.Sessions[1].ClientState == "idle" })
	data, err := os.ReadFile(filepath.Join(p.CWD, "work-products", "thread-method.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "thread/resume" {
		t.Fatalf("expected native resume, got %s", data)
	}
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
