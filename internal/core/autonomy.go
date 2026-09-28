package core

import (
	"context"
	"slices"
	"strings"
)

// resolvedOrchestratorProfile mirrors workflowProfile for the orchestrator
// role without needing a live Document. A manual (or not-yet-selected)
// workspace uses defaults.orchestrator_profile; a selected workflow may
// override it in workflows.<id>.profiles.orchestrator.
func resolvedOrchestratorProfile(cfg Config, workflowID string, manual bool) string {
	if !manual && workflowID != "" {
		if name := cfg.Workflows[workflowID].Profiles["orchestrator"]; name != "" {
			return name
		}
	}
	return cfg.Defaults.OrchestratorProfile
}

// requireAutonomySupport refuses an autonomous run when the resolved
// orchestrator profile has no route whose client declares the deliver
// capability: without delivery the run would stall waiting for the user's next
// turn.
func requireAutonomySupport(cfg Config, workflowID string, manual bool) error {
	profile := resolvedOrchestratorProfile(cfg, workflowID, manual)
	if profile == "" {
		return fail("autonomy_unsupported", "autonomy requires a configured orchestrator profile with a deliver-capable route")
	}
	p, ok := cfg.Profiles[profile]
	if !ok {
		return fail("autonomy_unsupported", "orchestrator profile %q is not configured", profile)
	}
	for _, route := range p.Routes {
		if slices.Contains(cfg.Clients[route.Client].Capabilities, "deliver") {
			return nil
		}
	}
	return fail("autonomy_unsupported", "orchestrator profile %q has no client route with the deliver capability", profile)
}

// appendAutonomyDecision records one lifecycle decision atomically with the
// owning mutation. The caller has already started the mutate transaction.
func (s *Service) appendAutonomyDecision(d *Document, kind, question, reason, resolvedBy string) {
	now := nowUTC()
	d.State.Decisions = append(d.State.Decisions, Decision{
		ID:         ID("decision"),
		Kind:       kind,
		Question:   question,
		Subject:    "workspace:" + d.State.ID,
		Reason:     strings.TrimSpace(reason),
		Revision:   d.State.Revision,
		ResolvedBy: resolvedBy,
		SessionID:  s.Actor.SessionID,
		RunID:      s.Actor.RunID,
		AnsweredAt: &now,
		DecidedAt:  &now,
	})
}

// AutonomyEnableOptions is the explicit user decision that hands the
// orchestrator-level gates to the autonomous run.
type AutonomyEnableOptions struct {
	Reason           string `json:"reason"`
	ExpectedRevision int    `json:"expected_revision"`
}

// AutonomyDisableOptions records who disabled the run and why. An agent
// orchestrator must attest the user's decision with UserConfirmed.
type AutonomyDisableOptions struct {
	Reason           string `json:"reason"`
	ExpectedRevision int    `json:"expected_revision"`
	UserConfirmed    bool   `json:"user_confirmed,omitempty"`
}

// EnableAutonomy turns an existing workspace into an autonomous run. Only the
// terminal user may call it: agents are refused even with an attestation,
// because enabling hands over the user's gates.
func (s *Service) EnableAutonomy(ctx context.Context, selector string, opt AutonomyEnableOptions, key string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, []string{key}, []any{"autonomy.enable", opt}, &out, s.requireUser, func(d *Document) error {
		if err := s.requireUser(d); err != nil {
			return err
		}
		if d.State.AutonomyRunning() {
			out = d.Status()
			return nil
		}
		if d.State.NeedsWorkflow() {
			return fail("needs_workflow", "select a workflow before enabling autonomy")
		}
		switch d.State.Status {
		case "completed":
			return fail("workspace_completed", "autonomy cannot be enabled for a completed workspace")
		case "archived":
			return fail("workspace_archived", "autonomy cannot be enabled for an archived workspace")
		}
		if d.State.PendingDecision != nil {
			return fail("decision_pending", "answer or refresh the pending decision before enabling autonomy")
		}
		if opt.ExpectedRevision <= 0 {
			return fail("revision_required", "autonomy enable requires the exact current workspace revision")
		}
		if d.State.Revision != opt.ExpectedRevision {
			return fail("revision_conflict", "expected revision %d, current %d", opt.ExpectedRevision, d.State.Revision)
		}
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		workflowID := ""
		if d.State.Workflow != nil {
			workflowID = d.State.Workflow.ID
		}
		if err := requireAutonomySupport(cfg, workflowID, d.State.Manual()); err != nil {
			return err
		}
		now := nowUTC()
		d.State.Autonomy = &Autonomy{Mode: "autonomous", State: "running", EnabledAt: &now, EnabledRevision: d.State.Revision, Source: "enable"}
		s.appendAutonomyDecision(d, "autonomy.enabled", "Enable autonomous mode for this workspace", opt.Reason, "user")
		d.Body += "\n\n## Autonomous mode enabled\n\n" + strings.TrimSpace(opt.Reason) + "\n"
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}

// DisableAutonomy makes the workspace interactive again. The user may always
// disable, and an orchestrator agent may with --user-confirmed, because the
// change only narrows the run.
func (s *Service) DisableAutonomy(ctx context.Context, selector string, opt AutonomyDisableOptions, key string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, []string{key}, []any{"autonomy.disable", opt}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		agent := s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != ""
		if agent && !opt.UserConfirmed {
			return fail("user_decision_required", "record the user's explicit confirmation with --user-confirmed")
		}
		if d.State.Autonomy == nil {
			return fail("autonomy_not_enabled", "this workspace has no autonomy record")
		}
		if strings.TrimSpace(opt.Reason) == "" {
			return fail("reason_required", "record why autonomy is being disabled")
		}
		if d.State.Autonomy.State == "disabled" {
			out = d.Status()
			return nil
		}
		if opt.ExpectedRevision <= 0 {
			return fail("revision_required", "autonomy disable requires the exact current workspace revision")
		}
		if d.State.Revision != opt.ExpectedRevision {
			return fail("revision_conflict", "expected revision %d, current %d", opt.ExpectedRevision, d.State.Revision)
		}
		now := nowUTC()
		d.State.Autonomy.State = "disabled"
		d.State.Autonomy.DisabledAt = &now
		d.State.Autonomy.DisabledReason = strings.TrimSpace(opt.Reason)
		resolvedBy := "user"
		if agent {
			resolvedBy = "orchestrator"
		}
		s.appendAutonomyDecision(d, "autonomy.disabled", "Disable autonomous mode for this workspace", opt.Reason, resolvedBy)
		d.Body += "\n\n## Autonomous mode disabled\n\n" + strings.TrimSpace(opt.Reason) + "\n"
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}
