package core

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// IssueEvaluationOptions is the orchestrator's judgment about whether the linked
// Issue's acceptance criteria were delivered by the completed Workspace.
type IssueEvaluationOptions struct {
	Outcome          string `json:"outcome"`
	Reason           string `json:"reason"`
	ExpectedRevision int    `json:"expected_revision"`
}

// issueEvaluationReasonLimit bounds the reason text stored in the Issue
// frontmatter so the linked Workspace names stay visible without growing the
// Issue unboundedly.
const issueEvaluationReasonLimit = 2000

// markIssueEvaluationPending records the durable pending Issue evaluation in the
// same revision as the completion. It is a no-op for an unlinked Workspace.
func markIssueEvaluationPending(d *Document, trigger string) *IssueEvaluation {
	if d.State.Input.IssueID == "" {
		return nil
	}
	eval := &IssueEvaluation{ID: ID("ieval"), IssueID: d.State.Input.IssueID, State: "pending", RequestedAt: nowUTC()}
	d.State.IssueEvaluation = eval
	d.Body += "\n\n## Issue evaluation\n\npending; the orchestrator will evaluate Issue " + d.State.Input.IssueID + " after " + strings.ToLower(trigger) + ".\n"
	return eval
}

func issueEvaluationReceiptKey(wsID, evalID string) string {
	return "issue-evaluation:" + wsID + ":" + evalID
}

func deliveredByWorkspacePrefix(wsID string) string {
	return "Delivered by workspace " + wsID + " (evaluation "
}

// issueClosedByWorkspace reports whether the Issue is closed by an earlier
// evaluation of this exact Workspace, identified by the workspace link that the
// evaluation writes into the visible status reason.
func issueClosedByWorkspace(i Issue, wsID string) bool {
	return i.Status == "closed" && strings.HasPrefix(i.StatusReason, deliveredByWorkspacePrefix(wsID))
}

func issueEvaluationReason(outcome, wsID, evalID, reason string) string {
	label := "Not delivered"
	if outcome == "delivered" {
		label = "Delivered"
	}
	return truncateIssueReason(fmt.Sprintf("%s by workspace %s (evaluation %s): %s", label, wsID, evalID, reason))
}

func truncateIssueReason(text string) string {
	if len(text) <= issueEvaluationReasonLimit {
		return text
	}
	cut := text[:issueEvaluationReasonLimit]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return strings.TrimRight(cut, " \t\n")
}

// authorizeIssueEvaluation narrows the operation to the completed Workspace's
// own orchestrator (including a conversation-only Run) or the user terminal as
// recovery. The project-scoped Dispatcher is rejected before actor resolution so
// it never gains linked-Workspace authority here.
func (s *Service) authorizeIssueEvaluation(d *Document) error {
	if err := s.requireWorkspaceScope(); err != nil {
		return err
	}
	return s.requireOrchestrator(d)
}

// RecordIssueEvaluation records the orchestrator's linked-Issue judgment. It
// runs inside one project-lock critical section: the Issue is updated through
// the lock-free updateIssueLocked helper and the Workspace evaluation is saved
// by the same mutation. Replaying the same operation key and payload never
// writes a second Issue revision; a crash between the Issue write and the
// Workspace save converges when the same key is retried.
func (s *Service) RecordIssueEvaluation(ctx context.Context, selector string, opt IssueEvaluationOptions, key string) (Status, error) {
	var out Status
	request := []any{"workspace.issue-evaluation", opt}
	err := mutate(s, ctx, selector, []string{key}, request, &out, s.authorizeIssueEvaluation, func(d *Document) error {
		return s.recordIssueEvaluationLocked(d, opt, &out)
	})
	return out, err
}

func (s *Service) recordIssueEvaluationLocked(d *Document, opt IssueEvaluationOptions, out *Status) error {
	switch d.State.Status {
	case "archived":
		return fail("workspace_archived", "archived workspace cannot record an Issue evaluation")
	case "completed":
	default:
		return fail("workspace_not_completed", "record the linked Issue evaluation only after the workspace is completed")
	}
	if strings.TrimSpace(d.State.Input.IssueID) == "" {
		return fail("operation_not_applicable", "this workspace is not linked to an Issue")
	}
	eval := d.State.IssueEvaluation
	if eval == nil {
		return fail("operation_not_applicable", "this workspace has no Issue evaluation to record")
	}
	outcome := strings.TrimSpace(opt.Outcome)
	if outcome != "delivered" && outcome != "not_delivered" {
		return fail("invalid_outcome", "use delivered or not_delivered")
	}
	reason := strings.TrimSpace(opt.Reason)
	if reason == "" {
		return fail("reason_required", "record why the linked Issue was or was not delivered")
	}
	if opt.ExpectedRevision != 0 && d.State.Revision != opt.ExpectedRevision {
		return fail("revision_conflict", "workspace changed while the Issue evaluation was being recorded")
	}
	if eval.State == "recorded" {
		if eval.Outcome == outcome && eval.Reason == reason {
			*out = d.Status()
			return nil
		}
		return fail("issue_evaluation_recorded", "this workspace already recorded a different Issue evaluation; reopen and complete the workspace again to change it")
	}
	current, _, err := readIssue(s.Root, d.State.Input.IssueID)
	if err != nil {
		return err
	}
	if outcome == "delivered" && current.Digest != d.State.Input.IssueDigest {
		return fail("issue_revised", "Issue content changed since this workspace's snapshot (revision %d, now %d); record not_delivered with the gap or reopen", d.State.Input.IssueRevision, current.Revision)
	}
	newStatus, action, changed, err := issueEvaluationTransition(current, outcome == "delivered", d.State.ID)
	if err != nil {
		return err
	}
	updated := current
	if changed {
		updated, err = s.updateIssueLocked(d.State.Input.IssueID, IssueUpdateOptions{
			Status:           newStatus,
			Reason:           issueEvaluationReason(outcome, d.State.ID, eval.ID, reason),
			ExpectedRevision: current.Revision,
			OperationKey:     issueEvaluationReceiptKey(d.State.ID, eval.ID),
		})
		if err != nil {
			return err
		}
	}
	now := nowUTC()
	eval.State = "recorded"
	eval.Outcome = outcome
	eval.Reason = reason
	eval.IssueAction = action
	eval.IssueRevision = updated.Revision
	eval.EvaluatedBy = s.issueEvaluationActor()
	eval.RunID = s.Actor.RunID
	eval.EvaluatedAt = &now
	d.Body += "\n\n## Issue evaluation\n\n" + issueEvaluationReason(outcome, d.State.ID, eval.ID, reason) + " (" + action + ")\n"
	if err := saveDocument(d); err != nil {
		return err
	}
	*out = d.Status()
	return nil
}

func (s *Service) issueEvaluationActor() string {
	if s.Actor.AgentID == "" {
		return "user"
	}
	return s.Actor.AgentID
}

// issueEvaluationTransition is the plan's outcomes table. changed reports
// whether the Issue file needs a write; unchanged_closed paths never touch it.
func issueEvaluationTransition(current Issue, delivered bool, wsID string) (status, action string, changed bool, err error) {
	switch current.Status {
	case "open":
		if delivered {
			return "closed", "closed", true, nil
		}
		return "open", "left_open", true, nil
	case "deferred":
		if delivered {
			return "closed", "closed", true, nil
		}
		return "open", "left_open", true, nil
	case "closed":
		if issueClosedByWorkspace(current, wsID) {
			if delivered {
				return "closed", "unchanged_closed", false, nil
			}
			return "open", "reopened", true, nil
		}
		return "closed", "unchanged_closed", false, nil
	default:
		return "", "", false, fail("invalid_issue", "unsupported Issue status %q", current.Status)
	}
}
