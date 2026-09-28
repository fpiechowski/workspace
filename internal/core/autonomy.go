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

// isAgentActor reports whether the current actor is an agent session rather
// than the terminal user (whose actor is empty).
func (s *Service) isAgentActor() bool {
	return s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != ""
}

// requireGateRationale enforces the rationale contract for an orchestrator-level
// gate that a running autonomous run resolves without the user. A terminal user
// and an interactive workspace are unaffected.
func (s *Service) requireGateRationale(d *Document, rationale string) error {
	if !d.State.AutonomyRunning() || !s.isAgentActor() {
		return nil
	}
	if strings.TrimSpace(rationale) == "" {
		return fail("rationale_required", "autonomous run in progress: record --rationale for this gate")
	}
	return nil
}

// autonomyDecisionQuestion is the durable question recorded with an autonomous
// gate decision.
func autonomyDecisionQuestion(kind string) string {
	switch kind {
	case "autonomous.plan_acceptance":
		return "Accept the planner result under autonomy"
	case "autonomous.handoff_accept":
		return "Accept the task result under autonomy"
	case "autonomous.handoff_reject":
		return "Reject the task result under autonomy"
	case "autonomous.phase_advance":
		return "Advance the workflow phase under autonomy"
	case "autonomous.retry":
		return "Retry the task under autonomy"
	case "autonomous.retire":
		return "Retire the task under autonomy"
	case "autonomous.assumption":
		return "Record an autonomous assumption"
	case "autonomous.question_answer":
		return "Answer a worker question under autonomy"
	default:
		return ""
	}
}

// appendGateDecision records the orchestrator's autonomous resolution of a gate
// atomically with the gate mutation. It is a no-op unless an agent is resolving
// a gate while the run is running.
func (s *Service) appendGateDecision(d *Document, kind, subject, rationale string, evidence []string) {
	if !d.State.AutonomyRunning() || !s.isAgentActor() {
		return
	}
	now := nowUTC()
	d.State.Decisions = append(d.State.Decisions, Decision{
		ID:         ID("decision"),
		Kind:       kind,
		Question:   autonomyDecisionQuestion(kind),
		Subject:    subject,
		Reason:     strings.TrimSpace(rationale),
		Evidence:   append([]string(nil), evidence...),
		Revision:   d.State.Revision,
		ResolvedBy: "orchestrator",
		Autonomous: true,
		SessionID:  s.Actor.SessionID,
		RunID:      s.Actor.RunID,
		AnsweredAt: &now,
		DecidedAt:  &now,
	})
	line := kind + " " + subject
	if reason := strings.TrimSpace(rationale); reason != "" {
		line += " — " + reason
	}
	d.Body += "\n\n## Autonomous decisions\n\n" + line + "\n"
}

// autonomyDecisionCount counts prior autonomous orchestrator decisions of a
// kind for a subject within the current orchestrator Run. It backs the fixed
// rejection and retry bounds.
func (s *Service) autonomyDecisionCount(d *Document, kind, subject string) int {
	count := 0
	for _, dec := range d.State.Decisions {
		if dec.Kind == kind && dec.Subject == subject && dec.Autonomous && dec.RunID == s.Actor.RunID {
			count++
		}
	}
	return count
}

// requireAutonomyBound refuses a gate that exceeds the fixed per-task-per-run
// bound so the orchestrator must deliver a blocked report instead of looping.
func (s *Service) requireAutonomyBound(d *Document, kind, subject string, limit int, what string) error {
	if !d.State.AutonomyRunning() || !s.isAgentActor() {
		return nil
	}
	if s.autonomyDecisionCount(d, kind, subject) >= limit {
		return fail("autonomy_bound_exceeded", "task %s reached the autonomous %s bound of %d for this run", strings.TrimPrefix(subject, "task:"), what, limit)
	}
	return nil
}

// DecisionRecordOptions is an orchestrator judgment with no gate mutation, such
// as an assumption taken while resolving an ambiguity.
type DecisionRecordOptions struct {
	Kind     string   `json:"kind"`
	Subject  string   `json:"subject"`
	Reason   string   `json:"reason"`
	Evidence []string `json:"evidence,omitempty"`
}

// RecordDecision records an autonomous assumption or worker-question answer.
// It is orchestrator-only and allowed only while the run is running.
func (s *Service) RecordDecision(ctx context.Context, selector string, opt DecisionRecordOptions, key string) (Status, error) {
	var out Status
	switch opt.Kind {
	case "autonomous.assumption", "autonomous.question_answer":
	default:
		return out, fail("invalid_decision", "decision record supports autonomous.assumption and autonomous.question_answer")
	}
	if strings.TrimSpace(opt.Subject) == "" {
		return out, fail("subject_required", "record the decision subject")
	}
	err := mutate(s, ctx, selector, []string{key}, []any{"decision.record", opt}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if !s.isAgentActor() {
			return fail("forbidden", "decision record is reserved for the orchestrator during an autonomous run")
		}
		if !d.State.AutonomyRunning() {
			return fail("autonomy_not_running", "decision record is available only while an autonomous run is running")
		}
		if err := rejectNewWorkspaceWork(d, "recording decisions"); err != nil {
			return err
		}
		if strings.TrimSpace(opt.Reason) == "" {
			return fail("rationale_required", "record a rationale for the autonomous decision")
		}
		s.appendGateDecision(d, opt.Kind, opt.Subject, opt.Reason, opt.Evidence)
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
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
