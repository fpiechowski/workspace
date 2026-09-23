package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

var workflowPhases = []string{"planning", "plan_review", "implementing", "integrating", "change_requests", "live_test_offer", "live_testing", "awaiting_release", "completed"}

// requireWorkflowOperation rejects operations that only apply to a
// workflow-driven workspace with a stable, non-interactive error. It is a no-op
// for a selected workflow and for a pending creation choice, whose existing
// selection prompt stays authoritative.
func requireWorkflowOperation(w Workspace, operation string) error {
	if w.Manual() {
		return fail("operation_not_applicable", "%s is not available without a selected workflow", operation)
	}
	return nil
}

func advancePlanFirst(d *Document) error {
	phase := d.State.Workflow.Phase
	switch phase {
	case "planning":
		if _, err := acceptedRole(d, "planner"); err != nil {
			return err
		}
		d.State.Workflow.Phase = "plan_review"
	case "plan_review":
		plans, err := acceptedRole(d, "planner")
		if err != nil {
			return err
		}
		count := 0
		retired := 0
		for _, t := range d.State.Tasks {
			if t.Role != "implementer" {
				continue
			}
			if retiredTask(t) {
				retired++
				continue
			}
			count++
			linked := false
			for _, dep := range t.DependsOn {
				for _, p := range plans {
					if dep == p.ID {
						linked = true
					}
				}
			}
			if !linked {
				return fail("workflow_gate", "implementation task %s must reference an accepted plan", t.ID)
			}
		}
		if count == 0 && retired == 0 {
			return fail("workflow_gate", "create implementation tasks before advancing")
		}
		d.State.Workflow.Phase = "implementing"
	case "implementing":
		live := false
		for _, t := range d.State.Tasks {
			if t.Role == "implementer" && !retiredTask(t) {
				live = true
				break
			}
		}
		if !live {
			d.State.Workflow.Phase = "completed"
			d.State.Status = "completed"
			return nil
		}
		if _, err := acceptedRole(d, "implementer"); err != nil {
			return err
		}
		d.State.Workflow.Phase = "completed"
		d.State.Status = "completed"
	case "completed":
		return fail("workspace_completed", "workflow is completed")
	default:
		return fail("invalid_state", "unknown plan-first workflow phase %q", phase)
	}
	return nil
}

func acceptedRole(d *Document, role string) ([]Task, error) {
	result := []Task{}
	for _, t := range d.State.Tasks {
		if t.Role != role {
			continue
		}
		if retiredTask(t) {
			continue
		}
		if t.State != "accepted" {
			return nil, fail("workflow_gate", "%s task %s is %s", role, t.ID, t.State)
		}
		h, err := findHandoff(d, t.AcceptedHandoff)
		if err != nil {
			return nil, err
		}
		if err := validateHandoff(d, h, &t); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if len(result) == 0 {
		return nil, fail("workflow_gate", "at least one accepted %s task is required", role)
	}
	return result, nil
}

func retiredTask(t Task) bool {
	return t.State == "cancelled" || t.State == "abandoned" || t.State == "superseded" || t.State == "deleted" || t.DeletedAt != nil
}
func decisionBasis(d *Document) string {
	return payloadDigest(struct {
		Workflow    *Workflow
		Input       Input
		Base        Base
		Integration *Integration
		CR          []ChangeRequest
	}{d.State.Workflow, d.State.Input, d.State.Base, d.State.Integration, d.State.ChangeRequests})
}
func requestDecision(d *Document, kind, question string, options []string) (Decision, error) {
	if p := d.State.PendingDecision; p != nil {
		if p.Kind == kind && p.BasisDigest == decisionBasis(d) {
			return *p, nil
		}
		return Decision{}, fail("decision_pending", "answer or cancel pending decision %s first", p.ID)
	}
	decision := Decision{ID: ID("decision"), Kind: kind, Question: question, Options: options, Revision: d.State.Revision + 1, BasisDigest: decisionBasis(d)}
	d.State.PendingDecision = &decision
	return decision, nil
}
func validateIntegration(ctx context.Context, d *Document) error {
	if err := requireWorkflowCapability(d, capIntegration); err != nil {
		return err
	}
	i := d.State.Integration
	if i == nil || i.HeadCommit == "" {
		return fail("workflow_gate", "accepted integrated revision is missing")
	}
	w, err := findWorktree(d, i.WorktreeID)
	if err != nil {
		return err
	}
	if err := verifyWorktree(ctx, w); err != nil {
		return err
	}
	head, err := git(ctx, w.Path, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != i.HeadCommit {
		return fail("integration_changed", "integration HEAD changed; revalidate integration and tests")
	}
	status, err := git(ctx, w.Path, "status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		return fail("dirty_worktree", "integration has uncommitted changes")
	}
	for n, taskID := range i.TaskIDs {
		task, err := findTask(d, taskID)
		if err != nil {
			return err
		}
		if task.State != "accepted" {
			return fail("workflow_gate", "integrated task no longer accepted")
		}
		h, err := findHandoff(d, task.AcceptedHandoff)
		if err != nil {
			return err
		}
		if n >= len(i.Heads) || h.HeadCommit != i.Heads[n] {
			return fail("integration_changed", "accepted task heads changed")
		}
		if err := validateHandoff(d, h, task); err != nil {
			return err
		}
	}
	return nil
}
func advance(ctx context.Context, d *Document, target string) error {
	if d.State.NeedsWorkflow() {
		return decisionRequired("select a workflow", "plan-first")
	}
	if err := requireWorkflowOperation(d.State, "workflow advance"); err != nil {
		return err
	}
	if err := requireWorkflowCapability(d, capPhases); err != nil {
		return err
	}
	if d.State.Status == "completed" {
		return fail("workspace_completed", "workflow is completed; reopen the workspace before advancing")
	}
	if d.State.Status == "archived" {
		return fail("workspace_archived", "archived workspace cannot advance")
	}
	if d.State.Status != "active" {
		return fail("workflow_gate", "workspace is %s", d.State.Status)
	}
	// Workflows without the integration capability use the compact
	// plan/review/implementation/completed state machine. The capability
	// declaration, rather than a workflow ID, selects these gates.
	if !workflowHasCapability(d, capIntegration) {
		next := map[string]string{"planning": "plan_review", "plan_review": "implementing", "implementing": "completed"}[d.State.Workflow.Phase]
		if target != "" && target != next {
			return fail("workflow_gate", "next phase is %s; cannot skip to %s", next, target)
		}
		if err := advancePlanFirst(d); err != nil {
			return err
		}
		return nil
	}
	phase := d.State.Workflow.Phase
	next := ""
	switch phase {
	case "planning":
		if _, err := acceptedRole(d, "planner"); err != nil {
			return err
		}
		next = "plan_review"
	case "plan_review":
		plans, err := acceptedRole(d, "planner")
		if err != nil {
			return err
		}
		count := 0
		for _, t := range d.State.Tasks {
			if t.Role != "implementer" || retiredTask(t) {
				continue
			}
			count++
			linked := false
			for _, dep := range t.DependsOn {
				for _, p := range plans {
					if dep == p.ID {
						linked = true
					}
				}
			}
			if !linked {
				return fail("workflow_gate", "implementation task %s must reference an accepted plan", t.ID)
			}
		}
		if count == 0 {
			return fail("workflow_gate", "create implementation tasks before advancing")
		}
		next = "implementing"
	case "implementing":
		if _, err := acceptedRole(d, "implementer"); err != nil {
			return err
		}
		next = "integrating"
	case "integrating":
		if err := requireWorkflowCapability(d, capIntegration); err != nil {
			return err
		}
		if _, err := acceptedRole(d, "integrator"); err != nil {
			return err
		}
		if err := validateIntegration(ctx, d); err != nil {
			return err
		}
		next = "change_requests"
	case "change_requests":
		if err := requireWorkflowCapability(d, capChangeRequest); err != nil {
			return err
		}
		if err := validateIntegration(ctx, d); err != nil {
			return err
		}
		covered := map[string]bool{}
		for _, cr := range d.State.ChangeRequests {
			if cr.State == "open" || cr.State == "merged" || cr.State == "skipped" {
				covered[cr.HeadCommit] = true
			}
		}
		if d.State.ChangeRequestMode == "per-task" {
			for _, head := range d.State.Integration.Heads {
				if !covered[head] {
					return fail("workflow_gate", "publish, link or explicitly skip change requests covering all integrated changes")
				}
			}
		} else if !covered[d.State.Integration.HeadCommit] {
			return fail("workflow_gate", "publish, link or explicitly skip the integrated change request")
		}
		next = "live_test_offer"
	case "live_test_offer":
		if err := requireWorkflowCapability(d, capLiveTest); err != nil {
			return err
		}
		return decisionRequired("answer the live-testing decision", "run", "skip")
	case "live_testing":
		if err := requireWorkflowCapability(d, capLiveTest); err != nil {
			return err
		}
		if err := validateIntegration(ctx, d); err != nil {
			return err
		}
		if d.State.LiveTest.Choice != "run" || d.State.LiveTest.AcceptedHandoff == "" {
			return fail("workflow_gate", "live-testing result has not been accepted")
		}
		h, err := findHandoff(d, d.State.LiveTest.AcceptedHandoff)
		if err != nil {
			return err
		}
		t, err := findTask(d, h.TaskID)
		if err != nil {
			return err
		}
		if t.State != "accepted" {
			return fail("workflow_gate", "test task not accepted")
		}
		if err := validateHandoff(d, h, t); err != nil {
			return err
		}
		if h.HeadCommit != d.State.Integration.HeadCommit {
			return fail("test_revision_mismatch", "live test covers another revision")
		}
		next = "awaiting_release"
	case "awaiting_release":
		if err := requireWorkflowCapability(d, capRelease); err != nil {
			return err
		}
		return fail("user_decision_required", "use release confirm after the user confirms deployment or release")
	case "completed":
		return fail("workspace_completed", "workflow is completed")
	default:
		return fail("invalid_state", "unknown workflow phase %q", phase)
	}
	if target != "" && target != next {
		return fail("workflow_gate", "next phase is %s; cannot skip to %s", next, target)
	}
	d.State.Workflow.Phase = next
	if next == "live_test_offer" {
		if _, err := requestDecision(d, "live-testing", "Run live testing against the integrated revision?", []string{"run", "skip"}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) AdvanceWorkflow(ctx context.Context, selector, target, key string) (Status, error) {
	var out Status
	req := struct{ Action, Target string }{"workflow.advance", target}
	err := mutate(s, ctx, selector, []string{key}, req, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := advance(ctx, d, target); err != nil {
			return err
		}
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}

type DecisionAnswer struct {
	ID, Answer, Reason, Environment string
	ExpectedRevision                int
	UserConfirmed                   bool
}

func (s *Service) AnswerDecision(ctx context.Context, selector string, opt DecisionAnswer, keys ...string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, keys, []any{"decision.answer", opt}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := rejectNewWorkspaceWork(d, "answering decisions"); err != nil {
			return err
		}
		if err := requireWorkflowOperation(d.State, "decision answers"); err != nil {
			return err
		}
		if (s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "") && !opt.UserConfirmed {
			return fail("user_decision_required", "confirm that the user supplied this answer with --user-confirmed")
		}
		p := d.State.PendingDecision
		if p == nil {
			for _, dec := range d.State.Decisions {
				if dec.ID == opt.ID && !dec.Superseded && dec.Answer == opt.Answer && dec.Reason == opt.Reason && dec.Environment == opt.Environment {
					out = d.Status()
					return nil
				}
			}
			return fail("decision_not_found", "no matching pending decision")
		}
		if p.ID != opt.ID || p.Revision != opt.ExpectedRevision {
			return fail("revision_conflict", "answer must reference decision ID and revision %d", p.Revision)
		}
		if p.BasisDigest != decisionBasis(d) {
			return fail("decision_stale", "workflow inputs changed; refresh this decision")
		}
		allowed := false
		for _, choice := range p.Options {
			if choice == opt.Answer {
				allowed = true
			}
		}
		if !allowed {
			return fail("invalid_answer", "choose one of %s", strings.Join(p.Options, ", "))
		}
		switch p.Kind {
		case "live-testing":
			if err := requireWorkflowCapability(d, capLiveTest); err != nil {
				return err
			}
			if err := validateIntegration(ctx, d); err != nil {
				return err
			}
			if opt.Answer == "skip" && strings.TrimSpace(opt.Reason) == "" {
				return fail("reason_required", "record a reason for skipping live testing")
			}
			if opt.Answer == "run" && strings.TrimSpace(opt.Environment) == "" {
				return fail("environment_required", "specify the live-testing environment")
			}
			d.State.LiveTest = LiveTest{Choice: opt.Answer, Reason: opt.Reason, Environment: opt.Environment, HeadCommit: d.State.Integration.HeadCommit}
			if opt.Answer == "run" {
				d.State.Workflow.Phase = "live_testing"
			} else {
				d.State.Workflow.Phase = "awaiting_release"
			}
		default:
			return fail("invalid_decision", "unsupported decision kind %s", p.Kind)
		}
		now := time.Now().UTC()
		p.Answer = opt.Answer
		p.Reason = opt.Reason
		p.Environment = opt.Environment
		p.AnsweredAt = &now
		d.State.Decisions = append(d.State.Decisions, *p)
		d.State.PendingDecision = nil
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}
func (s *Service) ConfirmRelease(ctx context.Context, selector, reference string, userConfirmed bool, keys ...string) (Status, error) {
	var out Status
	if strings.TrimSpace(reference) == "" {
		return out, fail("reference_required", "record the release/deployment reference")
	}
	err := mutate(s, ctx, selector, keys, []any{"release.confirm", reference, userConfirmed}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if (s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "") && !userConfirmed {
			return fail("user_decision_required", "record the user's explicit confirmation with --user-confirmed")
		}
		if d.State.Release.UserConfirmed {
			if d.State.Release.Reference != reference {
				return fail("release_conflict", "another release is already recorded")
			}
			out = d.Status()
			return nil
		}
		if d.State.Status == "completed" {
			return fail("workspace_completed", "workflow is completed; reopen the workspace before confirming release")
		}
		if d.State.Status == "archived" {
			return fail("workspace_archived", "archived workspace cannot confirm release")
		}
		if err := requireWorkflowOperation(d.State, "release confirmation"); err != nil {
			return err
		}
		if err := requireWorkflowCapability(d, capRelease); err != nil {
			return err
		}
		if d.State.Workflow == nil || d.State.Workflow.Phase != "awaiting_release" || d.State.Status != "active" {
			return fail("workflow_gate", "workspace must be active and awaiting_release")
		}
		if err := validateIntegration(ctx, d); err != nil {
			return err
		}
		if d.State.LiveTest.Choice != "skip" {
			h, err := findHandoff(d, d.State.LiveTest.AcceptedHandoff)
			if err != nil {
				return err
			}
			if h.HeadCommit != d.State.Integration.HeadCommit {
				return fail("test_revision_mismatch", "live test does not cover release revision")
			}
			t, err := findTask(d, h.TaskID)
			if err != nil {
				return err
			}
			if t.State != "accepted" || t.AcceptedHandoff != h.ID {
				return fail("workflow_gate", "release requires the current accepted test result")
			}
			if err := validateHandoff(d, h, t); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		d.State.Release = Release{UserConfirmed: true, Reference: reference, HeadCommit: d.State.Integration.HeadCommit, ConfirmedAt: &now}
		d.State.Workflow.Phase = "completed"
		d.State.Status = "completed"
		d.Body += "\n\n## Release confirmation\n\n" + reference + " — " + d.State.Integration.HeadCommit + " at " + now.Format(time.RFC3339) + "\n"
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}

type MenuAction struct {
	ID      string `json:"id" yaml:"id"`
	Label   string `json:"label" yaml:"label"`
	Command string `json:"command" yaml:"command"`
}
type Menu struct {
	Revision        int          `json:"revision" yaml:"revision"`
	Phase           string       `json:"phase" yaml:"phase"`
	Actions         []MenuAction `json:"actions" yaml:"actions"`
	PendingDecision *Decision    `json:"pending_decision" yaml:"pending_decision"`
	FreeText        bool         `json:"free_text" yaml:"free_text"`
}

func (s *Service) Menu(ctx context.Context, selector string) (Menu, error) {
	var out Menu
	err := s.With(ctx, selector, func(d *Document) error {
		out = Menu{Revision: d.State.Revision, PendingDecision: d.State.PendingDecision, FreeText: true, Actions: []MenuAction{{"status", "Show progress", "status"}, {"sessions", "Open an agent session", "session list"}, {"artifacts", "Inspect results", "artifact list"}, {"inbox", "Read messages", "inbox list"}}}
		if d.State.NeedsWorkflow() {
			out.Actions = append(out.Actions, MenuAction{"workflow", "Select plan-first", "workflow select plan-first"})
			return nil
		}
		if d.State.Manual() {
			// Manual mode has no phase, selection or advance action. It exposes
			// ordinary inspection plus the guarded pause/resume and
			// complete/archive lifecycle.
			out.Phase = "manual"
			switch d.State.Status {
			case "completed":
				out.Actions = append(out.Actions, MenuAction{"conversation", "Start or resume conversation", "start"})
				out.Actions = append(out.Actions, MenuAction{"reopen", "Reopen completed workspace", "reopen --reason <reason> --expected-revision <revision>"})
				out.Actions = append(out.Actions, MenuAction{"archive", "Archive this workspace", "archive"})
			case "archived":
				out.Actions = append(out.Actions, MenuAction{"clean", "Inspect cleanup plan", "clean --dry-run"})
			case "paused":
				out.Actions = append(out.Actions, MenuAction{"resume", "Resume delegation", "resume"})
			default:
				out.Actions = append(out.Actions, MenuAction{"pause", "Pause delegation", "pause"})
				out.Actions = append(out.Actions, MenuAction{"complete", "Complete this manual workspace", "complete --user-confirmed"})
			}
			return nil
		}
		out.Phase = d.State.Workflow.Phase
		if d.State.Status == "completed" {
			out.Actions = append(out.Actions, MenuAction{"conversation", "Start or resume conversation", "start"})
			out.Actions = append(out.Actions, MenuAction{"reopen", "Reopen completed workspace", "reopen --reason <reason> --expected-revision <revision>"})
			out.Actions = append(out.Actions, MenuAction{"archive", "Archive this workspace", "archive"})
			return nil
		}
		if d.State.Status == "archived" {
			out.Actions = append(out.Actions, MenuAction{"clean", "Inspect cleanup plan", "clean --dry-run"})
			return nil
		}
		if d.State.Status == "paused" {
			out.Actions = append(out.Actions, MenuAction{"resume", "Resume delegation", "resume"})
			return nil
		}
		if d.State.PendingDecision != nil {
			if d.State.PendingDecision.BasisDigest != decisionBasis(d) {
				out.Actions = append(out.Actions, MenuAction{"refresh-decision", "Refresh a question after state changes", "decision refresh"})
				return nil
			}
			out.Actions = append(out.Actions, MenuAction{"decision", "Answer the pending question", fmt.Sprintf("decision answer %s --expected-revision %d", d.State.PendingDecision.ID, d.State.PendingDecision.Revision)})
		} else if out.Phase == "awaiting_release" {
			out.Actions = append(out.Actions, MenuAction{"release", "Confirm deployment or release", "release confirm --reference <reference>"})
		} else {
			out.Actions = append(out.Actions, MenuAction{"advance", "Check requirements and advance workflow", "workflow advance"})
		}
		out.Actions = append(out.Actions, MenuAction{"pause", "Pause delegation", "pause"})
		return nil
	})
	return out, err
}
