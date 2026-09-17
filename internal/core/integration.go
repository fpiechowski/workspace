package core

import (
	"context"
	"path/filepath"
	"sort"
)

type IntegrationOptions struct {
	Tasks              []string
	Base, OperationKey string
}

func integrationInputs(d *Document, ids []string) ([]string, []string, error) {
	if len(ids) == 0 {
		for _, t := range d.State.Tasks {
			if t.Role == "implementer" {
				ids = append(ids, t.ID)
			}
		}
	}
	if len(ids) == 0 {
		return nil, nil, fail("tasks_required", "select accepted implementation tasks")
	}
	normalized := []string{}
	headsByID := map[string]string{}
	for _, id := range ids {
		t, err := findTask(d, id)
		if err != nil {
			return nil, nil, err
		}
		if t.Role != "implementer" || t.State != "accepted" {
			return nil, nil, fail("task_not_accepted", "task %s is not an accepted implementation", id)
		}
		h, err := findHandoff(d, t.AcceptedHandoff)
		if err != nil {
			return nil, nil, err
		}
		if err := validateHandoff(d, h, t); err != nil {
			return nil, nil, err
		}
		if _, ok := headsByID[t.ID]; ok {
			return nil, nil, fail("invalid_tasks", "duplicate integration task")
		}
		normalized = append(normalized, t.ID)
		headsByID[t.ID] = h.HeadCommit
	}
	// Existing dependency edges are respected; independent tasks have stable ID order.
	sort.Strings(normalized)
	ordered := []string{}
	done := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if done[id] {
			return
		}
		done[id] = true
		t, _ := findTask(d, id)
		for _, dep := range t.DependsOn {
			if _, ok := headsByID[dep]; ok {
				visit(dep)
			}
		}
		ordered = append(ordered, id)
	}
	for _, id := range normalized {
		visit(id)
	}
	heads := []string{}
	for _, id := range ordered {
		heads = append(heads, headsByID[id])
	}
	return ordered, heads, nil
}
func (s *Service) PrepareIntegration(ctx context.Context, selector string, opt IntegrationOptions) (Integration, error) {
	var out Integration
	var name, requestDigest string
	var reused bool
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := requireWorkflowOperation(d.State, "integration"); err != nil {
			return err
		}
		if _, err := d.previous(opt.OperationKey, opt); err != nil {
			return err
		}
		if found, err := replayResource(d, opt.OperationKey, &out); found || err != nil {
			reused = found
			return err
		}
		if d.State.Workflow == nil || d.State.Workflow.Phase != "integrating" {
			return fail("workflow_gate", "integration can be prepared in the integrating phase")
		}
		ids, heads, err := integrationInputs(d, opt.Tasks)
		if err != nil {
			return err
		}
		base := d.State.Base.Commit
		if opt.Base != "" {
			base, err = git(ctx, s.Root, "rev-parse", "--verify", "--end-of-options", opt.Base+"^{commit}")
			if err != nil {
				return err
			}
		}
		requestDigest = payloadDigest(struct {
			Tasks, Heads []string
			Base         string
		}{ids, heads, base})
		previous, err := d.previous(opt.OperationKey, opt)
		if err != nil {
			return err
		}
		if previous != "" {
			if d.State.Integration == nil || d.State.Integration.InputDigest != previous {
				return fail("integration_changed", "previous integration was superseded")
			}
			out = *d.State.Integration
			reused = true
			return nil
		}
		if d.State.Integration != nil && d.State.Integration.InputDigest == requestDigest {
			out = *d.State.Integration
			reused = true
			if opt.OperationKey != "" {
				d.remember(opt.OperationKey, opt, requestDigest)
				return saveResource(d, opt.OperationKey, out)
			}
			return nil
		}
		out = Integration{TaskIDs: ids, Heads: heads, BaseCommit: base, InputDigest: requestDigest}
		name = "integration"
		for _, w := range d.Registry.Worktrees {
			if w.Name == name {
				name = "integration-" + requestDigest[7:19]
				break
			}
		}
		return nil
	})
	if err != nil || reused {
		return out, err
	}
	w, err := s.CreateWorktree(ctx, selector, WorktreeOptions{Name: name, Base: out.BaseCommit, Purpose: "integration", OperationKey: "integration-worktree:" + requestDigest})
	if err != nil {
		return out, err
	}
	if w.State != "ready" {
		return out, fail("worktree_not_ready", "integration worktree requires reconcile")
	}
	out.WorktreeID = w.ID
	err = s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		ids, heads, err := integrationInputs(d, opt.Tasks)
		if err != nil {
			return err
		}
		if payloadDigest(struct {
			Tasks, Heads []string
			Base         string
		}{ids, heads, out.BaseCommit}) != requestDigest {
			return fail("revision_conflict", "accepted implementation changed during preparation")
		}
		if err := writeJSON(filepath.Join(d.Dir, "integration", "manifest.json"), out); err != nil {
			return err
		}
		d.State.Integration = &out
		d.State.LiveTest = LiveTest{}
		d.remember(opt.OperationKey, opt, requestDigest)
		return saveResource(d, opt.OperationKey, out)
	})
	return out, err
}
