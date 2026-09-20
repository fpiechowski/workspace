package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ReopenOptions is the explicit authorization and optimistic-concurrency
// contract for turning a completed workspace back into active work.
type ReopenOptions struct {
	Reason           string `json:"reason"`
	ExpectedRevision int    `json:"expected_revision"`
	UserConfirmed    bool   `json:"user_confirmed,omitempty"`
}

type reopenRecord struct {
	ID               string `json:"id"`
	Reason           string `json:"reason"`
	PreviousStatus   string `json:"previous_status"`
	PreviousPhase    string `json:"previous_phase,omitempty"`
	PreviousRevision int    `json:"previous_revision"`
	BaseCommit       string `json:"base_commit"`
}

// ReopenWorkspace is deliberately separate from workspace resume. It records
// the completed document before changing it and invalidates only state that
// describes the prior completion/release cycle.
func (s *Service) ReopenWorkspace(ctx context.Context, selector string, opt ReopenOptions, key string) (Status, error) {
	var out Status
	request := []any{"workspace.reopen", opt}
	err := mutate(s, ctx, selector, []string{key}, request, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		switch d.State.Status {
		case "completed":
			// Continue below.
		case "archived":
			return fail("workspace_archived", "archived workspaces cannot be reopened")
		case "active":
			return fail("workspace_active", "workspace is already active; reopen applies only to completed workspaces")
		case "paused":
			return fail("workspace_paused", "paused workspaces use workspace resume; reopen applies only to completed workspaces")
		case "blocked":
			return fail("workspace_blocked", "blocked workspaces must be repaired or resumed; reopen applies only to completed workspaces")
		case "needs_attention":
			return fail("workspace_needs_attention", "workspaces needing attention must be resolved; reopen applies only to completed workspaces")
		case "needs_workflow":
			return fail("workspace_needs_workflow", "select a workflow before reopening a workspace")
		default:
			return fail("workspace_not_completed", "workspace status %q cannot be reopened", d.State.Status)
		}
		if strings.TrimSpace(opt.Reason) == "" {
			return fail("reason_required", "explain why the completed workspace should be reopened")
		}
		if opt.ExpectedRevision <= 0 {
			return fail("revision_required", "reopen requires the exact current workspace revision")
		}
		if d.State.Revision != opt.ExpectedRevision {
			return fail("revision_conflict", "expected revision %d, current %d", opt.ExpectedRevision, d.State.Revision)
		}
		if s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "" {
			if !opt.UserConfirmed {
				return fail("user_decision_required", "record the user's explicit follow-up authorization with --user-confirmed")
			}
		}
		for _, session := range d.Registry.Sessions {
			if !session.Active() {
				continue
			}
			if session.AgentID != d.State.OrchestratorAgentID && session.AgentSnapshot.Role != "orchestrator" {
				return fail("session_active", "stop active worker session %s before reopening", session.ID)
			}
		}
		for _, service := range d.Registry.Services {
			if service.Active() {
				return fail("service_active", "stop active service %s before reopening", service.ID)
			}
		}

		before, err := os.ReadFile(filepath.Join(d.Dir, "WORKSPACE.md"))
		if err != nil {
			return err
		}
		id := ID("reopen")
		prefix := filepath.Join("history", id)
		record := reopenRecord{ID: id, Reason: strings.TrimSpace(opt.Reason), PreviousStatus: d.State.Status, PreviousRevision: d.State.Revision, BaseCommit: d.State.Base.Commit}
		if d.State.Workflow != nil {
			record.PreviousPhase = d.State.Workflow.Phase
		}
		recordJSON, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		d.PendingFiles = map[string][]byte{
			filepath.Join(prefix, "WORKSPACE.md"): []byte(string(before)),
			filepath.Join(prefix, "REASON.md"):    []byte(record.Reason + "\n"),
			filepath.Join(prefix, "REOPEN.json"):  append(recordJSON, '\n'),
		}

		d.State.Status = "active"
		if d.State.Workflow != nil {
			d.State.Workflow.Phase = "planning"
		}
		d.State.PendingDecision = nil
		d.State.Integration = nil
		d.State.LiveTest = LiveTest{}
		d.State.Release = Release{}
		for i := range d.State.ChangeRequests {
			d.State.ChangeRequests[i].State = "outdated"
		}
		d.Body += "\n\n## Workspace reopened\n\n" + record.Reason + ". Previous completed state: " + filepath.ToSlash(filepath.Join(prefix, "WORKSPACE.md")) + "\n"
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}
