package core

import (
	"context"
	"strings"
	"time"
)

// CompleteOptions carries the explicit user attestation and the optimistic
// revision guard for completing a manual workspace or a landed plan-first
// workflow.
type CompleteOptions struct {
	Reason           string
	UserConfirmed    bool
	ExpectedRevision int
}

// CompleteWorkspace is the dedicated terminal operation for an intentionally
// manual workspace and for a plan-first v2 workflow whose accepted integration
// has landed (or that has nothing to integrate). It is deliberately separate
// from workflow advance, state update and handoff acceptance so no process
// exit, empty task set or accepted handoff can complete work automatically. The
// operation is guarded by the visible revision and, when an operation key is
// supplied, replays idempotently with mutate's committed receipt.
//
// Contract:
//   - an intentionally manual workspace, or an active landing workflow in the
//     integration phase with a landed integration or nothing to integrate;
//   - an agent session must attest the user's explicit confirmation;
//   - no active service, active session or non-accepted (non-retired) task may
//     remain;
//   - archive then succeeds without a workflow release confirmation.
func (s *Service) CompleteWorkspace(ctx context.Context, selector string, opt CompleteOptions, keys ...string) (Status, error) {
	var out Status
	request := []any{"workspace.complete", opt}
	err := mutate(s, ctx, selector, keys, request, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := s.rejectAutonomousAttestation(d, "complete"); err != nil {
			return err
		}
		if d.State.Manual() {
			return s.completeManual(d, opt, &out)
		}
		if workflowHasCapability(d, capLanding) {
			return s.completeLanding(ctx, d, opt, &out)
		}
		return fail("operation_not_applicable", "workspace completion is only available for an intentionally manual workspace or a landed plan-first workflow")
	})
	return out, err
}

func (s *Service) completeManual(d *Document, opt CompleteOptions, out *Status) error {
	if d.State.Status == "archived" {
		return fail("workspace_archived", "archived workspace cannot be completed")
	}
	if d.State.Status == "completed" {
		*out = d.Status()
		return nil
	}
	if opt.ExpectedRevision != 0 && d.State.Revision != opt.ExpectedRevision {
		return fail("revision_conflict", "workspace changed while completion was being confirmed")
	}
	if (s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "") && !opt.UserConfirmed {
		return fail("user_decision_required", "record the user's explicit completion confirmation with --user-confirmed")
	}
	if err := completionRuntimeQuiet(d); err != nil {
		return err
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
	*out = d.Status()
	return nil
}

// completeLanding completes a plan-first v2 workspace. The integration phase is
// left only here and only when the approved integration landed or there was
// nothing to integrate.
func (s *Service) completeLanding(ctx context.Context, d *Document, opt CompleteOptions, out *Status) error {
	if d.State.Status == "archived" {
		return fail("workspace_archived", "archived workspace cannot be completed")
	}
	if d.State.Status == "completed" {
		*out = d.Status()
		return nil
	}
	if d.State.Workflow == nil || d.State.Workflow.Phase != integrationPhase(d) {
		return fail("workflow_gate", "workspace must be in the %s phase to complete", integrationPhase(d))
	}
	if opt.ExpectedRevision != 0 && d.State.Revision != opt.ExpectedRevision {
		return fail("revision_conflict", "workspace changed while completion was being confirmed")
	}
	if (s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "") && !opt.UserConfirmed {
		return fail("user_decision_required", "record the user's explicit completion confirmation with --user-confirmed")
	}
	if err := completionRuntimeQuiet(d); err != nil {
		return err
	}
	for _, task := range d.State.Tasks {
		if retiredTask(task) {
			continue
		}
		if task.State != "accepted" {
			return fail("task_incomplete", "task %s is %s; accept or delete it before completing", task.ID, task.State)
		}
	}
	reason := strings.TrimSpace(opt.Reason)
	if landingLanded(d) {
		target := d.State.Integration.Landing.Target
		if d.State.Integration.HeadCommit == "" {
			return fail("workflow_gate", "the landed integration has no recorded revision")
		}
		if _, err := git(ctx, s.Root, "merge-base", "--is-ancestor", d.State.Integration.HeadCommit, "refs/heads/"+target); err != nil {
			return fail("workflow_gate", "the landed integration is no longer reachable from %s", target)
		}
	} else if hasLiveImplementer(d.State.Tasks) {
		return fail("workflow_gate", "land the accepted integration before completing the workspace")
	} else if reason == "" {
		return fail("reason_required", "record why the workspace completed without an integration")
	}
	now := nowUTC()
	if reason == "" {
		reason = "user confirmed workflow completion"
	}
	d.State.Workflow.Phase = "completed"
	d.State.Status = "completed"
	d.Body += "\n\n## Completion\n\n" + reason + " at " + now.Format(time.RFC3339) + "\n"
	if err := saveDocument(d); err != nil {
		return err
	}
	*out = d.Status()
	return nil
}

func completionRuntimeQuiet(d *Document) error {
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
	return nil
}
