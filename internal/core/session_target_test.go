package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestSessionTargetRoutingDoesNotUseLatestAgentSession(t *testing.T) {
	s, workspace := fixture(t)
	ctx := context.Background()
	agent, firstWorktree := worker(t, s, workspace, "targeted")
	first, err := s.StartSession(ctx, workspace, SessionOptions{Agent: agent.ID, Worktree: firstWorktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, workspace, first.ID); err != nil {
		t.Fatal(err)
	}
	secondWorktree, err := s.CreateWorktree(ctx, workspace, WorktreeOptions{Name: "targeted-second"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.StartSession(ctx, workspace, SessionOptions{Agent: agent.ID, Worktree: secondWorktree.ID})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.SendMessage(ctx, workspace, MessageOptions{To: agent.ID, Body: "ambiguous"})
	expectCode(t, err, "ambiguous_recipient")

	targeted, err := s.SendMessage(ctx, workspace, MessageOptions{ToSession: first.ID, Body: "old session"})
	if err != nil {
		t.Fatal(err)
	}
	visible, err := s.InboxForSession(ctx, workspace, first.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0].ID != targeted.ID || visible[0].ToSession != first.ID {
		t.Fatalf("exact Session inbox was not isolated: %+v", visible)
	}

	secondService := *s
	secondService.Actor = Actor{AgentID: agent.ID, SessionID: second.ID, RunID: second.CurrentRunID}
	visible, err = secondService.Inbox(ctx, workspace, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("current Session received a sibling message: %+v", visible)
	}
	_, err = secondService.ReadMessage(ctx, workspace, targeted.ID, true)
	expectCode(t, err, "forbidden")

	if _, err := s.CloseSession(ctx, workspace, first.ID, "historical", "close:first"); err != nil {
		t.Fatal(err)
	}
	_, err = s.SendMessage(ctx, workspace, MessageOptions{ToSession: first.ID, Body: "closed"})
	var coreErr *Error
	if !errors.As(err, &coreErr) || coreErr.Code != "session_closed" {
		t.Fatalf("closed exact target was rerouted: %v", err)
	}
}

func TestDelegatedSessionCapturesExactParentAndHandoffTarget(t *testing.T) {
	s, workspace := fixture(t)
	ctx := context.Background()
	orchestrator, err := s.StartOrchestrator(ctx, workspace, "orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	agent, worktree := worker(t, s, workspace, "child")
	task, err := s.CreateTask(ctx, workspace, TaskSpec{Name: "child-task", Title: "Child task", Goal: "Produce a result", Role: "planner", AcceptanceCriteria: []string{"result"}}, "task:child")
	if err != nil {
		t.Fatal(err)
	}

	delegator := *s
	delegator.Actor = Actor{AgentID: orchestrator.AgentID, SessionID: orchestrator.ID, RunID: orchestrator.CurrentRunID}
	child, err := delegator.StartSession(ctx, workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID, Task: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentAgentID != orchestrator.AgentID || child.ParentSessionID != orchestrator.ID {
		t.Fatalf("delegation lost exact parent lineage: %+v", child)
	}

	if err := atomicWrite(filepath.Join(worktree.Path, "work-products", "PLAN.md"), []byte("plan")); err != nil {
		t.Fatal(err)
	}
	workerService := *s
	workerService.Actor = Actor{AgentID: child.AgentID, SessionID: child.ID, RunID: child.CurrentRunID}
	handoff, err := workerService.SubmitHandoff(ctx, workspace, HandoffOptions{Session: child.ID, Summary: "Result", Artifacts: []string{"work-products/PLAN.md"}, OperationKey: "result:child"})
	if err != nil {
		t.Fatal(err)
	}
	if handoff.ToAgent != orchestrator.AgentID || handoff.ToSession != orchestrator.ID {
		t.Fatalf("handoff was not addressed to parent Session: %+v", handoff)
	}
	if err := s.With(ctx, workspace, func(d *Document) error {
		for _, message := range d.Registry.Messages {
			if message.HandoffID == handoff.ID {
				if message.ToSession != orchestrator.ID {
					t.Fatalf("handoff message lost target Session: %+v", message)
				}
				return nil
			}
		}
		t.Fatalf("handoff message not persisted")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCodeEndpointRecoveryAcceptsOnlyLoopbackFlags(t *testing.T) {
	argv := withOpenCodeServerFlags([]string{"opencode", "--hostname=localhost", "--port=1"}, "http://127.0.0.1:43123")
	if endpoint, err := openCodeEndpointFromArgv(argv); err != nil || endpoint != "http://127.0.0.1:43123" {
		t.Fatalf("server flag replacement produced an invalid endpoint: argv=%v endpoint=%q err=%v", argv, endpoint, err)
	}
	endpoint, err := openCodeEndpointFromArgv([]string{"opencode", "--hostname", "127.0.0.1", "--port", "43123"})
	if err != nil || endpoint != "http://127.0.0.1:43123" {
		t.Fatalf("valid loopback endpoint was not recovered: endpoint=%q err=%v", endpoint, err)
	}
	if endpoint, err := openCodeEndpointFromArgv([]string{"opencode"}); err != nil || endpoint != "" {
		t.Fatalf("missing flags should require restart rather than inventing an endpoint: endpoint=%q err=%v", endpoint, err)
	}
	for _, argv := range [][]string{
		{"opencode", "--hostname", "localhost", "--port", "43123"},
		{"opencode", "--hostname", "192.0.2.1", "--port", "43123"},
		{"opencode", "--hostname", "127.0.0.1"},
		{"opencode", "--hostname", "127.0.0.1", "--port", "0"},
	} {
		if _, err := openCodeEndpointFromArgv(argv); err == nil {
			t.Fatalf("invalid OpenCode flags were accepted: %v", argv)
		}
	}
}

func TestSessionTargetMigrationPreservesAmbiguityAndUsesReceipts(t *testing.T) {
	agent := Agent{ID: "agent_worker", Name: "worker", Role: "planner"}
	d := &Document{Registry: Registry{
		SchemaVersion: 4,
		Agents:        []Agent{agent},
		Sessions: []Session{
			{ID: "sess_old", AgentID: agent.ID},
			{ID: "sess_new", AgentID: agent.ID},
		},
		Messages: []Message{
			{ID: "msg_ambiguous", ToAgent: agent.ID, Body: "unknown historical target"},
			{ID: "msg_receipt", ToAgent: agent.ID, DeliveredSessionID: "sess_old", Body: "known historical target"},
		},
	}}
	if !migrateRegistry(d) {
		t.Fatal("schema-v4 registry was not migrated")
	}
	if d.Registry.Messages[0].ToSession != "" {
		t.Fatalf("ambiguous legacy message was assigned to %s", d.Registry.Messages[0].ToSession)
	}
	if d.Registry.Messages[1].ToSession != "sess_old" {
		t.Fatalf("receipt-backed message target = %q", d.Registry.Messages[1].ToSession)
	}
}
