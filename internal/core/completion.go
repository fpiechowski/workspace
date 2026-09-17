package core

import (
	"context"
	"strings"
	"time"
)

// CompleteOptions carries the explicit user attestation and the optimistic
// revision guard for completing an intentionally manual workspace.
type CompleteOptions struct {
	Reason           string
	UserConfirmed    bool
	ExpectedRevision int
}

// CompleteWorkspace is the dedicated terminal operation for an intentional
// manual workspace. It is deliberately separate from workflow advance, state
// update and handoff acceptance so no process exit, empty task set or accepted
// handoff can complete manual work automatically. The operation is guarded by
// the visible revision and, when an operation key is supplied, replays
// idempotently with mutate's committed receipt.
//
// Contract:
//   - only an intentionally manual workspace can be completed;
//   - an agent session must attest the user's explicit confirmation;
//   - no active service, active session or non-accepted task may remain;
//   - archive then succeeds without a workflow release confirmation.
func (s *Service) CompleteWorkspace(ctx context.Context, selector string, opt CompleteOptions, keys ...string) (Status, error) {
	var out Status
	request := []any{"workspace.complete", opt}
	err := mutate(s, ctx, selector, keys, request, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if !d.State.Manual() {
			return fail("operation_not_applicable", "workspace completion is only available for an intentionally manual workspace")
		}
		if d.State.Status == "completed" || d.State.Status == "archived" {
			out = d.Status()
			return nil
		}
		if opt.ExpectedRevision != 0 && d.State.Revision != opt.ExpectedRevision {
			return fail("revision_conflict", "workspace changed while completion was being confirmed")
		}
		if (s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "") && !opt.UserConfirmed {
			return fail("user_decision_required", "record the user's explicit completion confirmation with --user-confirmed")
		}
		for _, service := range d.Registry.Services {
			if service.Active() {
				return fail("service_active", "stop service %s before completing", service.ID)
			}
		}
		for _, session := range d.Registry.Sessions {
			if session.Active() {
				return fail("session_active", "stop session %s before completing", session.ID)
			}
		}
		for _, task := range d.State.Tasks {
			if task.DeletedAt != nil {
				continue
			}
			if task.State != "accepted" {
				return fail("task_incomplete", "task %s is %s; accept or delete it before completing", task.ID, task.State)
			}
		}
		now := nowUTC()
		reason := strings.TrimSpace(opt.Reason)
		if reason == "" {
			reason = "user confirmed manual completion"
		}
		d.State.Status = "completed"
		d.Body += "\n\n## Manual completion\n\n" + reason + " at " + now.Format(time.RFC3339) + "\n"
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}
