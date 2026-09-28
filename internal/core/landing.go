package core

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// LandingOptions carries the user approval and optimistic revision guard for
// landing an accepted integration on the target branch.
type LandingOptions struct {
	Target           string `json:"target,omitempty"`
	ExpectedRevision int    `json:"expected_revision,omitempty"`
	UserConfirmed    bool   `json:"user_confirmed,omitempty"`
}

// landingLanded reports whether the accepted integration has already been
// merged into the target branch.
func landingLanded(d *Document) bool {
	return d != nil && d.State.Integration != nil && d.State.Integration.Landing != nil && d.State.Integration.Landing.State == "landed"
}

// hasLiveImplementer reports whether any non-retired implementation task still
// has to be integrated.
func hasLiveImplementer(tasks []Task) bool {
	for _, t := range tasks {
		if t.Role == "implementer" && !retiredTask(t) {
			return true
		}
	}
	return false
}

// integrationTarget is the branch the accepted integration lands into.
func integrationTarget(d *Document) string {
	if d.State.Integration != nil && d.State.Integration.Target != "" {
		return d.State.Integration.Target
	}
	return d.State.Base.Ref
}

// LandingReadyToComplete reports whether a plan-first v2 workspace can be
// completed with workspace complete: either the accepted integration has
// landed or there is nothing to integrate.
func (w Workspace) LandingReadyToComplete() bool {
	if w.Workflow == nil || !slices.Contains(w.Workflow.Capabilities, capLanding) {
		return false
	}
	if w.Workflow.Phase != "integration" || w.Status != "active" {
		return false
	}
	if landingLanded(&Document{State: w}) {
		return true
	}
	return !hasLiveImplementer(w.Tasks)
}

// LandIntegration is the user-approved, fast-forward-only merge of the accepted
// integration HEAD into the target branch. It is local only and never pushes.
//
// Contract:
//   - the workspace must be an active landing workflow in the integration phase
//     with an accepted integrator and an unchanged accepted integration;
//   - an agent actor must attest the user's explicit approval;
//   - only a fast-forward is performed: a checked-out clean target is merged
//     with git merge --ff-only, otherwise the target ref is compare-and-swap
//     updated; a dirty, moved or diverged target is refused without changing
//     any ref;
//   - the pending intent is written before the ref update so an interrupted
//     land is completed by a retry without a second git effect.
func (s *Service) LandIntegration(ctx context.Context, selector string, opt LandingOptions, keys ...string) (Status, error) {
	if key := mutationKey(keys); key != "" {
		return effect(s, ctx, selector, key, []any{"integration.land", opt}, s.requireOrchestrator, func() (Status, error) {
			return s.landIntegration(ctx, selector, opt)
		})
	}
	return s.landIntegration(ctx, selector, opt)
}

func (s *Service) landIntegration(ctx context.Context, selector string, opt LandingOptions) (Status, error) {
	var out Status
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := s.rejectAutonomousAttestation(d, "integration land"); err != nil {
			return err
		}
		if err := rejectNewWorkspaceWork(d, "landing integration"); err != nil {
			return err
		}
		if err := requireWorkflowCapability(d, capLanding); err != nil {
			return err
		}
		phase := integrationPhase(d)
		if d.State.Workflow == nil || d.State.Workflow.Phase != phase || d.State.Status != "active" {
			return fail("workflow_gate", "workspace must be active in the %s phase to land", phase)
		}
		if d.State.PendingDecision != nil {
			return fail("decision_pending", "answer or cancel pending decision %s first", d.State.PendingDecision.ID)
		}
		if (s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "") && !opt.UserConfirmed {
			return fail("user_decision_required", "record the user's explicit approval with --user-confirmed")
		}
		if opt.ExpectedRevision != 0 && d.State.Revision != opt.ExpectedRevision {
			return fail("revision_conflict", "workspace changed while landing was being confirmed")
		}
		if _, err := acceptedRole(d, "integrator"); err != nil {
			return err
		}
		if err := validateIntegration(ctx, d); err != nil {
			return err
		}
		i := d.State.Integration
		target := integrationTarget(d)
		if opt.Target != "" {
			if i.Target != "" && opt.Target != i.Target {
				return fail("target_moved", "integration was prepared for target %s", i.Target)
			}
			target = opt.Target
		}
		if strings.HasPrefix(target, "-") {
			return fail("invalid_target", "invalid target branch")
		}
		if _, err := git(ctx, s.Root, "check-ref-format", "--branch", target); err != nil {
			return err
		}
		tip, err := git(ctx, s.Root, "rev-parse", "--verify", "--end-of-options", "refs/heads/"+target+"^{commit}")
		if err != nil {
			return err
		}
		after := i.HeadCommit
		landing := i.Landing
		if landing != nil && landing.State == "landed" {
			if tip != after {
				return fail("target_moved", "target %s no longer points at the landed integration", target)
			}
			out = d.Status()
			return nil
		}
		before := tip
		applied := false
		switch {
		case landing != nil && landing.After == after:
			before = landing.Before
			if tip == after {
				applied = true
			} else if tip != before {
				return fail("target_moved", "target %s moved to %s", target, tip)
			}
		case tip == after:
			applied = true
		case tip == i.BaseCommit:
			// Fast-forward from the prepared base.
		default:
			if _, err := git(ctx, s.Root, "merge-base", "--is-ancestor", tip, after); err != nil {
				return fail("target_moved", "target %s moved or diverged from the accepted integration", target)
			}
		}
		path, checkedOut, err := checkedOutWorktree(ctx, s.Root, target)
		if err != nil {
			return err
		}
		if checkedOut {
			status, err := git(ctx, path, "status", "--porcelain")
			if err != nil {
				return err
			}
			if status != "" {
				return fail("target_checkout_dirty", "target %s has a dirty checkout at %s", target, path)
			}
		}
		now := nowUTC()
		if !applied {
			// Record the intent before changing any ref. The pending landing is
			// written without advancing the visible revision, so a retry still
			// validates the caller's expected revision.
			i.Landing = &Landing{State: "pending", Target: target, Before: before, After: after, UserConfirmed: opt.UserConfirmed}
			if err := flushDocument(d); err != nil {
				return err
			}
			if checkedOut {
				if _, err := git(ctx, path, "merge", "--ff-only", after); err != nil {
					return fail("target_moved", "target %s could not fast-forward to %s", target, after)
				}
			} else if _, err := git(ctx, s.Root, "update-ref", "refs/heads/"+target, after, before); err != nil {
				return fail("target_moved", "target %s moved before landing", target)
			}
		}
		i.Landing = &Landing{State: "landed", Target: target, Before: before, After: after, UserConfirmed: opt.UserConfirmed, LandedAt: &now}
		d.Body += "\n\n## Integration accepted\n\n" + target + " now contains " + after + " (was " + before + ") at " + now.Format(time.RFC3339) + "\n"
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}

// checkedOutWorktree returns the worktree that has the branch checked out, when
// one exists.
func checkedOutWorktree(ctx context.Context, root, branch string) (string, bool, error) {
	listing, err := git(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return "", false, err
	}
	current := ""
	for _, line := range strings.Split(listing, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			current = filepath.Clean(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "branch "):
			if strings.TrimPrefix(line, "branch ") == "refs/heads/"+branch {
				return current, true, nil
			}
		}
	}
	return "", false, nil
}
