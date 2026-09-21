package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type pauseRunTarget struct {
	SessionID string `json:"session_id"`
	RunID     string `json:"run_id"`
}

type pauseInterruptPlan struct {
	SchemaVersion int              `json:"schema_version"`
	WorkspaceID   string           `json:"workspace_id"`
	RequestDigest string           `json:"request_digest"`
	ServiceIDs    []string         `json:"service_ids"`
	Runs          []pauseRunTarget `json:"runs"`
}

// PauseInterruptGuarded pauses the workspace and stops only the exact Runs and
// services the user confirmed. Its durable plan lets retries finish individual
// steps without adopting newly started work.
func (s *Service) PauseInterruptGuarded(ctx context.Context, selector, key string, guard MutationGuard) (Status, error) {
	if key == "" {
		return Status{}, fail("invalid_option", "pause and interrupt requires an operation key")
	}
	request := struct {
		Action string
		Guard  MutationGuard
	}{"pause-interrupt", guard}
	return effect(s, ctx, selector, key, request, s.requireOrchestrator, func() (Status, error) {
		return s.pauseInterrupt(ctx, selector, key, request, guard)
	})
}

func (s *Service) pauseInterrupt(ctx context.Context, selector, key string, request any, guard MutationGuard) (Status, error) {
	dir, err := s.resolve(selector)
	if err != nil {
		return Status{}, err
	}
	planPath := filepath.Join(dir, ".runtime", "pause-interrupt", strings.TrimPrefix(digest([]byte(key)), "sha256:")+".json")
	requestDigest := payloadDigest(request)
	plan, exists, err := readPauseInterruptPlan(planPath)
	if err != nil {
		return Status{}, err
	}
	if exists {
		if plan.WorkspaceID == "" || plan.RequestDigest != requestDigest {
			return Status{}, fail("operation_conflict", "stored pause-interrupt plan does not match this operation")
		}
	} else {
		plan.SchemaVersion = 1
		plan.RequestDigest = requestDigest
		if err := s.With(ctx, selector, func(d *Document) error {
			if err := s.requireOrchestrator(d); err != nil {
				return err
			}
			if guard.ExpectedRevision != 0 && d.State.Revision != guard.ExpectedRevision {
				return fail("revision_conflict", "workspace changed while the action was being confirmed")
			}
			if d.State.Status == "completed" {
				return fail("workspace_completed", "completed workspace cannot be paused; use workspace reopen for new work")
			}
			if d.State.Status == "archived" {
				return fail("workspace_archived", "archived workspace cannot be paused")
			}
			if d.State.NeedsWorkflow() {
				return decisionRequired("select a workflow before pausing", exampleWorkflow)
			}
			plan.WorkspaceID = d.State.ID
			plan.ServiceIDs = activeServiceIDs(d.Registry.Services)
			plan.Runs = activePauseRuns(d, s.Actor)
			if guard.ExpectedRunIDs != nil && !sameIDs(guard.ExpectedRunIDs, pauseRunIDs(plan.Runs)) {
				return fail("target_changed", "active Runs changed while the action was being confirmed")
			}
			if guard.ExpectedServiceIDs != nil && !sameIDs(guard.ExpectedServiceIDs, plan.ServiceIDs) {
				return fail("target_changed", "active services changed while the action was being confirmed")
			}
			// The plan is durable before the first domain mutation. A retry can
			// finish the exact saved target set after a crash at any later step.
			if err := writePauseInterruptPlan(planPath, plan); err != nil {
				return err
			}
			if d.State.Status != "paused" {
				d.State.Status = "paused"
				if err := saveDocument(d); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return Status{}, err
		}
	}

	// A saved plan is authoritative on retry. Ensure the pause still holds
	// before completing its recorded stop steps.
	if err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if d.State.Status == "completed" {
			return fail("workspace_completed", "completed workspace cannot continue a pending pause")
		}
		if d.State.Status == "archived" {
			return fail("workspace_archived", "archived workspace cannot continue a pending pause")
		}
		if d.State.Status != "paused" {
			d.State.Status = "paused"
			return saveDocument(d)
		}
		return nil
	}); err != nil {
		return Status{}, err
	}

	for _, serviceID := range plan.ServiceIDs {
		stepKey := "pause-interrupt:" + digest([]byte(key+"\x00service\x00"+serviceID))
		if _, err := s.StopService(ctx, selector, serviceID, stepKey); err != nil {
			return Status{}, err
		}
	}
	for _, target := range plan.Runs {
		stepKey := "pause-interrupt:" + digest([]byte(key+"\x00run\x00"+target.RunID))
		if _, err := s.StopRun(ctx, selector, target.SessionID, target.RunID, stepKey); err != nil {
			return Status{}, err
		}
	}
	return s.Status(ctx, selector)
}

func readPauseInterruptPlan(path string) (pauseInterruptPlan, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return pauseInterruptPlan{}, false, nil
	}
	if err != nil {
		return pauseInterruptPlan{}, false, err
	}
	var plan pauseInterruptPlan
	if err := json.Unmarshal(b, &plan); err != nil {
		return pauseInterruptPlan{}, true, fail("operation_state_invalid", "cannot decode %s: %v", path, err)
	}
	if plan.SchemaVersion != 1 {
		return pauseInterruptPlan{}, true, fail("operation_state_invalid", "unsupported pause-interrupt plan schema %d", plan.SchemaVersion)
	}
	return plan, true, nil
}

func writePauseInterruptPlan(path string, plan pauseInterruptPlan) error {
	b, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(b, '\n'))
}

func activeServiceIDs(services []BackgroundService) []string {
	ids := make([]string, 0, len(services))
	for _, service := range services {
		if service.Active() {
			ids = append(ids, service.ID)
		}
	}
	return ids
}

func activePauseRuns(d *Document, actor Actor) []pauseRunTarget {
	targets := make([]pauseRunTarget, 0, len(d.Registry.Sessions))
	callerIndex := -1
	for _, session := range d.Registry.Sessions {
		if !session.Active() || session.CurrentRunID == "" {
			continue
		}
		run, err := findRun(d, session.CurrentRunID)
		if err != nil || !run.Active() {
			continue
		}
		isCaller := session.ID == actor.SessionID || session.CurrentRunID == actor.RunID && actor.RunID != "" || session.CurrentRunID == actor.SessionID && actor.SessionID != ""
		targets = append(targets, pauseRunTarget{SessionID: session.ID, RunID: run.ID})
		if isCaller {
			callerIndex = len(targets) - 1
		}
	}
	if callerIndex >= 0 && callerIndex != len(targets)-1 {
		caller := targets[callerIndex]
		copy(targets[callerIndex:], targets[callerIndex+1:])
		targets[len(targets)-1] = caller
	}
	return targets
}

func pauseRunIDs(targets []pauseRunTarget) []string {
	ids := make([]string, len(targets))
	for i, target := range targets {
		ids[i] = target.RunID
	}
	return ids
}

func sameIDs(left, right []string) bool {
	left = slices.Clone(left)
	right = slices.Clone(right)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}
