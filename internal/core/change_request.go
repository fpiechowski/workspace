package core

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
)

func findCR(d *Document, id string) (*ChangeRequest, error) {
	for i := range d.State.ChangeRequests {
		if d.State.ChangeRequests[i].ID == id {
			return &d.State.ChangeRequests[i], nil
		}
	}
	return nil, fail("change_request_not_found", "unknown change request %s", id)
}

type ChangeRequestOptions struct{ Worktree, Target, Title, Body, OperationKey string }

func (s *Service) PrepareChangeRequest(ctx context.Context, selector string, opt ChangeRequestOptions) (ChangeRequest, error) {
	var out ChangeRequest
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := requireWorkflowOperation(d.State, "change-request preparation"); err != nil {
			return err
		}
		if err := requireWorkflowCapability(d, capChangeRequest); err != nil {
			return err
		}
		previous, err := d.previous(opt.OperationKey, opt)
		if err != nil {
			return err
		}
		if found, err := replayResource(d, opt.OperationKey, &out); found || err != nil {
			return err
		}
		if err := rejectNewWorkspaceWork(d, "preparing change requests"); err != nil {
			return err
		}
		if previous != "" {
			cr, err := findCR(d, previous)
			if err != nil {
				return err
			}
			out = *cr
			return nil
		}
		if err := validateIntegration(ctx, d); err != nil {
			return err
		}
		if d.State.Workflow.Phase != "change_requests" {
			return fail("workflow_gate", "prepare change requests in the change_requests phase")
		}
		wtID := opt.Worktree
		if wtID == "" {
			if d.State.ChangeRequestMode == "per-task" {
				return fail("worktree_required", "per-task policy requires an implementation worktree")
			}
			wtID = d.State.Integration.WorktreeID
		}
		w, err := findWorktree(d, wtID)
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
		allowed := head == d.State.Integration.HeadCommit && d.State.ChangeRequestMode != "per-task"
		var selectedTask *Task
		if d.State.ChangeRequestMode == "per-task" {
			for n, h := range d.State.Integration.Heads {
				if h == head {
					task, err := findTask(d, d.State.Integration.TaskIDs[n])
					if err != nil {
						return err
					}
					allowed = task.WorktreeID == w.ID
					if allowed {
						selectedTask = task
						break
					}
				}
			}
		}
		if !allowed {
			return fail("revision_mismatch", "worktree does not match the configured change-request policy")
		}
		target := opt.Target
		if target == "" {
			target = d.State.Base.Ref
		}
		if strings.HasPrefix(target, "-") {
			return fail("invalid_target", "invalid target branch")
		}
		if _, err := git(ctx, s.Root, "check-ref-format", "--branch", target); err != nil {
			return err
		}
		if _, err := git(ctx, s.Root, "rev-parse", "--verify", "--end-of-options", target+"^{commit}"); err != nil {
			return err
		}
		diff, err := git(ctx, w.Path, "diff", target+"..."+head)
		if err != nil {
			return err
		}
		title, body := opt.Title, opt.Body
		if title == "" {
			title = d.State.Title
		}
		if body == "" {
			body = "Resolves the issue described in " + d.State.Input.Snapshot + ".\n\nIntegrated revision: " + head + ".\n\nSee preserved task handoffs and verification artifacts."
		}
		out = ChangeRequest{ID: ID("cr"), WorktreeID: w.ID, Title: title, Body: body, Branch: w.Branch, Target: target, HeadCommit: head, State: "prepared"}
		if selectedTask != nil {
			out.TaskID = selectedTask.ID
			out.MergeAfter, err = changeRequestDependencies(d, selectedTask)
			if err != nil {
				return err
			}
			if len(out.MergeAfter) > 0 {
				out.Body += "\n\nMerge after workspace change requests: " + strings.Join(out.MergeAfter, ", ") + ".\n"
			}
		}
		out.Body += "\n\n<!-- workspace-cr:" + out.ID + " -->\n"
		if err := atomicWrite(crBodyPath(d, out), []byte(out.Body)); err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(d.Dir, "change-requests", out.ID, "changes.diff"), []byte(diff)); err != nil {
			return err
		}
		d.State.ChangeRequests = append(d.State.ChangeRequests, out)
		d.remember(opt.OperationKey, opt, out.ID)
		return saveResource(d, opt.OperationKey, out)
	})
	return out, err
}

func changeRequestDependencies(d *Document, task *Task) ([]string, error) {
	var result []string
	for _, id := range task.DependsOn {
		dep, err := findTask(d, id)
		if err != nil {
			return nil, err
		}
		if dep.Role != "implementer" {
			continue
		}
		h, err := findHandoff(d, dep.AcceptedHandoff)
		if err != nil {
			return nil, err
		}
		found := ""
		for _, cr := range d.State.ChangeRequests {
			if cr.TaskID == dep.ID && cr.HeadCommit == h.HeadCommit && cr.State != "outdated" && cr.State != "closed" {
				found = cr.ID
			}
		}
		if found == "" {
			return nil, fail("change_request_dependency", "prepare current change request for task %s first", dep.ID)
		}
		result = append(result, found)
	}
	return result, nil
}
func validateForgeResult(result ForgeResult, cr ChangeRequest) error {
	u, err := url.Parse(result.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fail("forge_error", "forge returned invalid URL")
	}
	if result.ID == "" {
		return fail("forge_error", "forge returned no ID")
	}
	if result.HeadCommit != cr.HeadCommit {
		return fail("revision_mismatch", "remote change request points to a different commit")
	}
	if result.State != "open" && result.State != "merged" && result.State != "closed" {
		return fail("forge_error", "unknown remote state")
	}
	return nil
}
func (s *Service) PublishChangeRequest(ctx context.Context, selector, id string, userConfirmed bool, keys ...string) (ChangeRequest, error) {
	if key := mutationKey(keys); key != "" {
		return effect(s, ctx, selector, key, []any{"change-request.publish", id, userConfirmed}, s.requireOrchestrator, func() (ChangeRequest, error) { return s.PublishChangeRequest(ctx, selector, id, userConfirmed) })
	}
	var out ChangeRequest
	var body string
	var allowCreate bool
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := rejectNewWorkspaceWork(d, "publishing change requests"); err != nil {
			return err
		}
		if err := requireWorkflowOperation(d.State, "change-request publication"); err != nil {
			return err
		}
		if err := requireWorkflowCapability(d, capChangeRequest); err != nil {
			return err
		}
		if err := validateIntegration(ctx, d); err != nil {
			return err
		}
		cr, err := findCR(d, id)
		if err != nil {
			return err
		}
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		if cr.State == "outdated" || cr.State == "skipped" {
			return fail("change_request_outdated", "prepare a current change request")
		}
		if !cr.PublicationAllowed && cfg.Forge.Publication != "allowed" && (s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "") && !userConfirmed {
			return decisionRequired("review change-requests/"+id+"/changes.diff and DESCRIPTION.md, then record user approval with --user-confirmed", "publish", "revise", "skip")
		}
		cr.PublicationAllowed = true
		allowCreate = cr.State == "prepared"
		out = *cr
		body = crBodyPath(d, *cr)
		if allowCreate {
			cr.State = "publishing"
		}
		return saveDocument(d)
	})
	if err != nil {
		return out, err
	}
	forge, err := s.forgeAdapter()
	if err != nil {
		return out, err
	}
	result, externalErr := forge.Lookup(ctx, s.Root, out)
	if externalErr == nil && result == nil {
		if !allowCreate {
			externalErr = fail("publication_uncertain", "no request found; refusing duplicate creation after an uncertain attempt")
		} else {
			r, err := forge.Publish(ctx, s.Root, out, body)
			externalErr = err
			if err == nil {
				result = &r
			}
		}
	}
	if externalErr == nil && result != nil {
		externalErr = validateForgeResult(*result, out)
	}
	saveErr := s.With(context.Background(), selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		cr, err := findCR(d, id)
		if err != nil {
			return err
		}
		if cr.HeadCommit != out.HeadCommit {
			return fail("revision_conflict", "change request changed during publishing")
		}
		if externalErr != nil {
			if cr.State != "outdated" && cr.State != "skipped" {
				cr.State = "uncertain"
			}
		} else if result != nil {
			if cr.State != "outdated" && cr.State != "skipped" {
				cr.State = result.State
			}
			cr.ExternalID = result.ID
			cr.URL = result.URL
		}
		out = *cr
		return saveDocument(d)
	})
	if saveErr != nil {
		return out, saveErr
	}
	return out, externalErr
}
func (s *Service) SyncChangeRequests(ctx context.Context, selector string, keys ...string) ([]ChangeRequest, error) {
	if key := mutationKey(keys); key != "" {
		return effect(s, ctx, selector, key, "change-request.sync", s.requireOrchestrator, func() ([]ChangeRequest, error) { return s.SyncChangeRequests(ctx, selector) })
	}
	if err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := rejectNewWorkspaceWork(d, "syncing change requests"); err != nil {
			return err
		}
		if err := requireWorkflowOperation(d.State, "change-request sync"); err != nil {
			return err
		}
		return requireWorkflowCapability(d, capChangeRequest)
	}); err != nil {
		return nil, err
	}
	status, err := s.Status(ctx, selector)
	if err != nil {
		return nil, err
	}
	forge, err := s.forgeAdapter()
	if err != nil {
		return nil, err
	}
	for _, cr := range status.Workspace.ChangeRequests {
		if cr.State == "skipped" || cr.State == "prepared" || cr.State == "outdated" {
			continue
		}
		result, err := forge.Lookup(ctx, s.Root, cr)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, fail("publication_uncertain", "request %s not found; no new request was created", cr.ID)
		}
		if err := validateForgeResult(*result, cr); err != nil {
			return nil, err
		}
		if err := s.With(ctx, selector, func(d *Document) error {
			if err := s.requireOrchestrator(d); err != nil {
				return err
			}
			if err := rejectNewWorkspaceWork(d, "syncing change requests"); err != nil {
				return err
			}
			current, err := findCR(d, cr.ID)
			if err != nil {
				return err
			}
			if current.HeadCommit != cr.HeadCommit {
				return fail("revision_conflict", "request changed while syncing")
			}
			if current.State != "outdated" && current.State != "skipped" {
				current.State = result.State
			}
			current.URL = result.URL
			current.ExternalID = result.ID
			return saveDocument(d)
		}); err != nil {
			return nil, err
		}
	}
	status, err = s.Status(ctx, selector)
	return status.Workspace.ChangeRequests, err
}
func (s *Service) ResolveChangeRequest(ctx context.Context, selector, id, action, reference, reason string, userConfirmed bool, keys ...string) (ChangeRequest, error) {
	var out ChangeRequest
	err := mutate(s, ctx, selector, keys, []any{"change-request.resolve", id, action, reference, reason, userConfirmed}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := rejectNewWorkspaceWork(d, "resolving change requests"); err != nil {
			return err
		}
		if err := requireWorkflowOperation(d.State, "change-request resolution"); err != nil {
			return err
		}
		if err := requireWorkflowCapability(d, capChangeRequest); err != nil {
			return err
		}
		if (s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "") && !userConfirmed {
			return fail("user_decision_required", "record the user's explicit decision")
		}
		cr, err := findCR(d, id)
		if err != nil {
			return err
		}
		switch action {
		case "skip":
			if strings.TrimSpace(reason) == "" {
				return fail("reason_required", "give the user's reason for skipping publication")
			}
			cr.State = "skipped"
			cr.Body += "\nPublication skipped: " + reason
		case "link":
			u, err := url.Parse(reference)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
				return fail("invalid_url", "provide a change-request URL")
			}
			cr.URL = reference
			cr.ExternalID = reference
			cr.State = "open"
		case "retry":
			if cr.State != "uncertain" && cr.State != "publishing" {
				return fail("invalid_state", "only uncertain publications can be retried")
			}
			if strings.TrimSpace(reason) == "" {
				return fail("reason_required", "record how the user verified that no change request exists")
			}
			cr.State = "prepared"
		default:
			return fail("invalid_action", "choose skip, link or retry")
		}
		out = *cr
		return saveDocument(d)
	})
	return out, err
}
