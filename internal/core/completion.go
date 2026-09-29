package core

import (
	"context"
	"strings"
	"time"
)

// CompleteOptions carries the explicit user attestation and the optimistic
// revision guard for completing a manual workspace or a landed plan-first
// workflow. NoEvaluationLaunch suppresses the post-commit evaluation launch so
// tests and users can complete a linked workspace without starting the
// orchestrator; the evaluation stays pending and workspace start retries it.
type CompleteOptions struct {
	Reason             string
	UserConfirmed      bool
	ExpectedRevision   int
	NoEvaluationLaunch bool
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
	if err != nil {
		return out, err
	}
	s.launchIssueEvaluation(ctx, selector, opt, &out)
	return out, nil
}

// launchIssueEvaluation performs the best-effort post-commit launch of the
// conversation-only orchestrator Run that evaluates the linked Issue. The
// committed completion is never rolled back: a launch failure only records
// issue_evaluation.launch_error so status and menu can surface the recovery.
func (s *Service) launchIssueEvaluation(ctx context.Context, selector string, opt CompleteOptions, out *Status) {
	if opt.NoEvaluationLaunch {
		return
	}
	eval := out.Workspace.IssueEvaluation
	if eval == nil || eval.State != "pending" {
		return
	}
	if launchErr := s.startIssueEvaluationRun(ctx, selector, eval); launchErr != nil {
		updated, recordErr := s.recordIssueEvaluationLaunchError(ctx, selector, eval.ID, launchErr)
		if recordErr == nil {
			*out = updated
		}
	}
}

// startIssueEvaluationRun is idempotent: it skips the launch when the
// orchestrator already has an active Session (an earlier launch, a user
// conversation, or a replay of the completion) and otherwise starts the
// evaluation Run under the stable "issue-evaluation:<ieval id>" operation key.
func (s *Service) startIssueEvaluationRun(ctx context.Context, selector string, eval *IssueEvaluation) error {
	active := false
	if err := s.With(ctx, selector, func(d *Document) error {
		for _, session := range d.Registry.Sessions {
			if session.AgentID == d.State.OrchestratorAgentID && session.Active() {
				active = true
				break
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if active {
		return nil
	}
	_, err := s.StartOrchestrator(ctx, selector, "issue-evaluation:"+eval.ID)
	return err
}

// recordIssueEvaluationLaunchError stores the launch failure on the still
// pending evaluation. It is a small independent mutation because the
// completion itself has already committed and must not be undone.
func (s *Service) recordIssueEvaluationLaunchError(ctx context.Context, selector, evalID string, launchErr error) (Status, error) {
	var out Status
	err := s.With(ctx, selector, func(d *Document) error {
		eval := d.State.IssueEvaluation
		if eval != nil && eval.ID == evalID && eval.State == "pending" {
			eval.LaunchError = launchErr.Error()
			if err := saveDocument(d); err != nil {
				return err
			}
		}
		out = d.Status()
		return nil
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
	markIssueEvaluationPending(d, "Manual completion")
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
	markIssueEvaluationPending(d, "Completion")
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
