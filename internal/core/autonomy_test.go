package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ensureDeliverable upgrades every configured client with a delivery argv so
// the resolved orchestrator profile advertises the deliver capability. The
// generic test fixture intentionally has no delivery transport.
func ensureDeliverable(t *testing.T, s *Service) {
	t.Helper()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	for name, client := range cfg.Clients {
		if len(client.DeliverArgv) == 0 {
			client.DeliverArgv = []string{"deliver-wrapper", "{message_id}"}
		}
		cfg.Clients[name] = client
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
}

func lastDecision(t *testing.T, status Status, kind string) Decision {
	t.Helper()
	for i := len(status.Workspace.Decisions) - 1; i >= 0; i-- {
		if status.Workspace.Decisions[i].Kind == kind {
			return status.Workspace.Decisions[i]
		}
	}
	t.Fatalf("no decision of kind %s in %+v", kind, status.Workspace.Decisions)
	return Decision{}
}

func TestAutonomyCreatePersistsRunningAndReplaysIdempotently(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()

	opt := CreateOptions{Title: "Autonomous plan-first", Input: "run unattended", Autonomous: true, OperationKey: "autonomy-create"}
	first, err := s.Create(ctx, opt)
	if err != nil {
		t.Fatalf("autonomous create failed: %v", err)
	}
	auto := first.Workspace.Autonomy
	if auto == nil || auto.State != "running" || auto.Source != "create" || auto.Mode != "autonomous" || auto.EnabledRevision != 1 {
		t.Fatalf("autonomy state after create: %+v", auto)
	}
	if first.Workspace.Status != "active" || first.Workspace.Workflow == nil {
		t.Fatalf("autonomous plan-first create state: %+v", first.Workspace)
	}

	replay, err := s.Create(ctx, opt)
	if err != nil {
		t.Fatalf("autonomous create replay failed: %v", err)
	}
	if replay.Workspace.ID != first.Workspace.ID || replay.Workspace.Revision != first.Workspace.Revision {
		t.Fatalf("replay did not return the committed result: first=%s/%d replay=%s/%d", first.Workspace.ID, first.Workspace.Revision, replay.Workspace.ID, replay.Workspace.Revision)
	}

	changed := opt
	changed.Autonomous = false
	if _, err := s.Create(ctx, changed); err == nil {
		t.Fatal("changed flag under the same operation key was accepted")
	} else {
		expectCode(t, err, "operation_conflict")
	}

	manual, err := s.Create(ctx, CreateOptions{Title: "Autonomous manual", Input: "manual unattended", NoWorkflow: true, Autonomous: true, OperationKey: "autonomy-create-manual"})
	if err != nil {
		t.Fatalf("manual autonomous create failed: %v", err)
	}
	if !manual.Workspace.Manual() || manual.Workspace.Autonomy == nil || manual.Workspace.Autonomy.State != "running" || manual.Workspace.Autonomy.Source != "create" {
		t.Fatalf("manual autonomy state: %+v", manual.Workspace)
	}

	status, err := s.Status(ctx, first.Workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Workspace.Autonomy == nil || status.Workspace.Autonomy.State != "running" {
		t.Fatalf("status did not expose autonomy: %+v", status.Workspace.Autonomy)
	}
}

func TestAutonomyCreateRejectsWithoutDeliverableOrchestrator(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()

	if _, err := s.Create(ctx, CreateOptions{Title: "No delivery", Input: "unattended", Autonomous: true}); err == nil {
		t.Fatal("autonomous plan-first create succeeded without a deliver-capable route")
	} else {
		expectCode(t, err, "autonomy_unsupported")
	}
	if _, err := s.Create(ctx, CreateOptions{Title: "No delivery manual", Input: "unattended", NoWorkflow: true, Autonomous: true}); err == nil {
		t.Fatal("autonomous manual create succeeded without a deliver-capable route")
	} else {
		expectCode(t, err, "autonomy_unsupported")
	}

	created, err := s.Create(ctx, CreateOptions{Title: "Manual", Input: "interactive", NoWorkflow: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.EnableAutonomy(ctx, created.Workspace.ID, AutonomyEnableOptions{Reason: "user approved", ExpectedRevision: created.Workspace.Revision}, "enable-no-delivery")
	expectCode(t, err, "autonomy_unsupported")
}

func TestAutonomyNonAutonomousSerializationIsUnchanged(t *testing.T) {
	b, err := json.Marshal(CreateOptions{Title: "t", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "utonomous") {
		t.Fatalf("non-autonomous CreateOptions changed its JSON shape: %s", b)
	}

	workspaceYAML, err := yaml.Marshal(Workspace{ID: "ws_x", Status: "active", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(workspaceYAML), "autonomy") {
		t.Fatalf("non-autonomous Workspace changed its YAML shape: %s", workspaceYAML)
	}

	decisionJSON, err := json.Marshal(Decision{ID: "decision_1", Kind: "live-testing", Question: "q", Options: []string{"run"}, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"resolved_by", "autonomous", "subject", "evidence", "session_id", "run_id", "decided_at"} {
		if strings.Contains(string(decisionJSON), field) {
			t.Fatalf("existing Decision gained field %q: %s", field, decisionJSON)
		}
	}

	// A created non-autonomous document must not carry the new front matter.
	s, ws := fixture(t)
	status, err := s.Status(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(status.Directory, "WORKSPACE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "autonomy:") {
		t.Fatalf("non-autonomous WORKSPACE.md contains autonomy front matter:\n%s", raw)
	}
}

func TestAutonomyDispatcherCannotCreateAutonomousWorkspace(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()
	issue, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: "Route unattended", Body: "Run without the user"})
	if err != nil {
		t.Fatal(err)
	}

	dispatcher := *s
	dispatcher.Actor = Actor{AgentID: "agent_dispatcher", SessionID: "sess_dispatcher", RunID: "run_dispatcher", Scope: "project"}
	_, err = dispatcher.DispatchIssue(ctx, IssueDispatchOptions{IssueID: issue.ID, NoWorkflow: true, Autonomous: true, OperationKey: "dispatch-autonomous"})
	expectCode(t, err, "forbidden")

	dispatched, err := s.DispatchIssue(ctx, IssueDispatchOptions{IssueID: issue.ID, NoWorkflow: true, Autonomous: true, OperationKey: "dispatch-autonomous"})
	if err != nil {
		t.Fatalf("user dispatch failed: %v", err)
	}
	if dispatched.Workspace.Workspace.Autonomy == nil || dispatched.Workspace.Workspace.Autonomy.State != "running" {
		t.Fatalf("user dispatch did not create an autonomous workspace: %+v", dispatched.Workspace.Workspace.Autonomy)
	}
}

func TestAutonomyEnableGuardsRevisionReceiptAndDecision(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Manual", Input: "interactive", NoWorkflow: true})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID

	orch, err := s.StartOrchestrator(ctx, ws, "autonomy-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	agentService := *s
	agentService.Actor = Actor{AgentID: orch.AgentID, SessionID: orch.ID, RunID: orch.CurrentRunID}
	revision := currentRevision(t, s, ws)
	_, err = agentService.EnableAutonomy(ctx, ws, AutonomyEnableOptions{Reason: "agent tries", ExpectedRevision: revision}, "enable-agent")
	expectCode(t, err, "user_decision_required")

	_, err = s.EnableAutonomy(ctx, ws, AutonomyEnableOptions{Reason: "stale", ExpectedRevision: revision + 1}, "enable-stale")
	expectCode(t, err, "revision_conflict")

	enabled, err := s.EnableAutonomy(ctx, ws, AutonomyEnableOptions{Reason: "user approved unattended", ExpectedRevision: revision}, "enable-ok")
	if err != nil {
		t.Fatalf("enable failed: %v", err)
	}
	if enabled.Workspace.Autonomy == nil || enabled.Workspace.Autonomy.State != "running" || enabled.Workspace.Autonomy.Source != "enable" {
		t.Fatalf("enable state: %+v", enabled.Workspace.Autonomy)
	}
	if got := lastDecision(t, enabled, "autonomy.enabled"); got.ResolvedBy != "user" || got.Reason != "user approved unattended" || got.Subject != "workspace:"+ws {
		t.Fatalf("autonomy.enabled decision: %+v", got)
	}

	replay, err := s.EnableAutonomy(ctx, ws, AutonomyEnableOptions{Reason: "user approved unattended", ExpectedRevision: revision}, "enable-ok")
	if err != nil || replay.Workspace.Revision != enabled.Workspace.Revision {
		t.Fatalf("enable replay changed the result: %+v %v", replay, err)
	}
	_, err = s.EnableAutonomy(ctx, ws, AutonomyEnableOptions{Reason: "different", ExpectedRevision: revision}, "enable-ok")
	expectCode(t, err, "operation_conflict")
}

func TestAutonomyEnableRefusals(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()
	activate := func(status string, pending *Decision) string {
		created, err := s.Create(ctx, CreateOptions{Title: "Manual", Input: "interactive", NoWorkflow: true})
		if err != nil {
			t.Fatal(err)
		}
		ws := created.Workspace.ID
		if err := s.With(ctx, ws, func(d *Document) error {
			d.State.Status = status
			d.State.PendingDecision = pending
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
		return ws
	}

	for _, tc := range []struct {
		name    string
		status  string
		pending *Decision
		code    string
	}{
		{"needs workflow", "needs_workflow", nil, "needs_workflow"},
		{"completed", "completed", nil, "workspace_completed"},
		{"archived", "archived", nil, "workspace_archived"},
		{"pending decision", "active", &Decision{ID: "decision_pending", Kind: "live-testing", Revision: 1}, "decision_pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := activate(tc.status, tc.pending)
			revision := currentRevision(t, s, ws)
			_, err := s.EnableAutonomy(ctx, ws, AutonomyEnableOptions{Reason: "r", ExpectedRevision: revision}, "enable-"+tc.name)
			expectCode(t, err, tc.code)
		})
	}
}

func TestAutonomyDisableByUserAndOrchestrator(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()

	created, err := s.Create(ctx, CreateOptions{Title: "Autonomous", Input: "unattended", NoWorkflow: true, Autonomous: true, OperationKey: "auto-disable-user"})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID

	orch, err := s.StartOrchestrator(ctx, ws, "autonomy-disable-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	agentService := *s
	agentService.Actor = Actor{AgentID: orch.AgentID, SessionID: orch.ID, RunID: orch.CurrentRunID}
	revision := currentRevision(t, s, ws)
	_, err = agentService.DisableAutonomy(ctx, ws, AutonomyDisableOptions{Reason: "agent", ExpectedRevision: revision}, "disable-agent")
	expectCode(t, err, "user_decision_required")

	disabled, err := s.DisableAutonomy(ctx, ws, AutonomyDisableOptions{Reason: "user took control", ExpectedRevision: revision}, "disable-user")
	if err != nil {
		t.Fatalf("user disable failed: %v", err)
	}
	if disabled.Workspace.Autonomy.State != "disabled" || disabled.Workspace.Autonomy.DisabledReason != "user took control" || disabled.Workspace.Autonomy.DisabledAt == nil {
		t.Fatalf("disable state: %+v", disabled.Workspace.Autonomy)
	}
	if got := lastDecision(t, disabled, "autonomy.disabled"); got.ResolvedBy != "user" || got.Reason != "user took control" {
		t.Fatalf("autonomy.disabled decision: %+v", got)
	}

	replay, err := s.DisableAutonomy(ctx, ws, AutonomyDisableOptions{Reason: "user took control", ExpectedRevision: revision}, "disable-user")
	if err != nil || replay.Workspace.Revision != disabled.Workspace.Revision {
		t.Fatalf("disable replay changed the result: %+v %v", replay, err)
	}
	_, err = s.DisableAutonomy(ctx, ws, AutonomyDisableOptions{Reason: "different", ExpectedRevision: revision}, "disable-user")
	expectCode(t, err, "operation_conflict")

	// A fresh autonomous workspace can be disabled by the orchestrator with
	// the user's attestation.
	second, err := s.Create(ctx, CreateOptions{Title: "Autonomous second", Input: "unattended", NoWorkflow: true, Autonomous: true})
	if err != nil {
		t.Fatal(err)
	}
	ws2 := second.Workspace.ID
	orch2, err := s.StartOrchestrator(ctx, ws2, "autonomy-disable-orchestrator-2")
	if err != nil {
		t.Fatal(err)
	}
	agentService2 := *s
	agentService2.Actor = Actor{AgentID: orch2.AgentID, SessionID: orch2.ID, RunID: orch2.CurrentRunID}
	revision2 := currentRevision(t, s, ws2)
	confirmed, err := agentService2.DisableAutonomy(ctx, ws2, AutonomyDisableOptions{Reason: "user asked", ExpectedRevision: revision2, UserConfirmed: true}, "disable-orchestrator")
	if err != nil {
		t.Fatalf("attested orchestrator disable failed: %v", err)
	}
	if confirmed.Workspace.Autonomy.State != "disabled" {
		t.Fatalf("orchestrator disable state: %+v", confirmed.Workspace.Autonomy)
	}
	if got := lastDecision(t, confirmed, "autonomy.disabled"); got.ResolvedBy != "orchestrator" {
		t.Fatalf("orchestrator disable decision: %+v", got)
	}
}

func TestAutonomyReopenClearsAndPreservesHistory(t *testing.T) {
	s, _ := fixture(t)
	ensureDeliverable(t, s)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateOptions{Title: "Autonomous", Input: "unattended", NoWorkflow: true, Autonomous: true})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	completed, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{Reason: "done", ExpectedRevision: currentRevision(t, s, ws)}, "complete-autonomy")
	if err != nil {
		t.Fatalf("completion failed: %v", err)
	}
	reopened, err := s.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "follow-up", ExpectedRevision: completed.Workspace.Revision}, "reopen-autonomy")
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	if reopened.Workspace.Autonomy != nil {
		t.Fatalf("reopen did not clear autonomy: %+v", reopened.Workspace.Autonomy)
	}

	entries, err := os.ReadDir(filepath.Join(reopened.Directory, "history"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("reopen history missing: entries=%v err=%v", entries, err)
	}
	history, err := os.ReadFile(filepath.Join(reopened.Directory, "history", entries[0].Name(), "WORKSPACE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(history), "autonomy:") || !strings.Contains(string(history), "state: running") {
		t.Fatalf("history copy did not retain autonomy:\n%s", history)
	}
}
