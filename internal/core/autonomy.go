package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// autonomyRunNotice is the binary-owned guidance appended to an orchestrator Run
// prompt while the workspace is in an autonomous run. It cannot go stale with a
// project's customized templates and carries the judgment rules the orchestrator
// must apply plus the operations that stay reserved for the user.
const autonomyRunNotice = `Autonomous mode notice (owned by the workspace binary; project templates cannot change it):
This workspace is in an autonomous run and this Run is the orchestrator. Resolve the
orchestrator-level gates yourself instead of asking the user, and record every decision:
- Plan acceptance and task/integrator result acceptance: accept only when core validation
  passes, the artifact and commit stay within the task scope, the check evidence covers the
  declared verification with passing outcomes, and every acceptance criterion maps to
  evidence. Otherwise reject with actionable feedback. Pass --rationale and --evidence; the
  gate and its audit decision commit together.
- Phase advance (planning -> plan_review -> implementing -> integration): advance only when
  the core gates pass and the implementation tasks derive from the accepted plan.
- Worker questions: answer from the issue, accepted plan and repository docs. For a product
  choice pick the most conservative, minimal-scope reading and record it with
  ` + "`workspace decision record --kind autonomous.assumption`" + `.
- Retries are bounded to 2 rejections and 1 retry per task per run. When the bound is
  exceeded, stop and deliver the report with outcome blocked.
Do not resolve these (the binary refuses them for an agent while running): integration land,
complete, change-request publish/resolve, release confirm, live-testing decision answer,
reopen, archive/clean/delete, and state edit. Finish the run with
` + "`workspace autonomy report --summary-file <path> --outcome ready_to_land|ready_to_complete|blocked|failed`" + `;
that sets the autonomy state to delivered and leaves the excluded operations reserved for the user.`

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

// Autonomy report outcomes. They describe what the user may do next, not what
// the run itself did.
const (
	autonomyOutcomeLand     = "ready_to_land"
	autonomyOutcomeComplete = "ready_to_complete"
	autonomyOutcomeBlocked  = "blocked"
	autonomyOutcomeFailed   = "failed"
)

// rejectAutonomousAttestation is the code-level safety boundary for operations
// with external, local-branch, lifecycle-terminal or authorization effects.
// While an autonomous run is running, an agent actor may not perform them even
// with an attestation; only the terminal user (empty actor) may. After the run
// is delivered or disabled the ordinary attestation contract applies.
func (s *Service) rejectAutonomousAttestation(d *Document, operation string) error {
	if d == nil || !d.State.AutonomyRunning() || !s.isAgentActor() {
		return nil
	}
	return fail("autonomy_excluded", "%s is reserved for the user; deliver the autonomy report first", operation)
}

// requireUserNotAutonomous is the authorize guard for terminal-only operations
// that are also excluded from autonomous resolution: an agent actor sees
// autonomy_excluded while the run is running, and user_decision_required
// otherwise.
func (s *Service) requireUserNotAutonomous(d *Document, operation string) error {
	if err := s.rejectAutonomousAttestation(d, operation); err != nil {
		return err
	}
	return s.requireUser(d)
}

// AutonomyReportOptions is the orchestrator's durable final report in place of a
// user question.
type AutonomyReportOptions struct {
	Outcome          string   `json:"outcome"`
	Recommendation   string   `json:"recommendation,omitempty"`
	SummaryFile      string   `json:"summary_file,omitempty"`
	Artifacts        []string `json:"artifacts,omitempty"`
	Pending          []string `json:"pending,omitempty"`
	ExpectedRevision int      `json:"expected_revision,omitempty"`
}

// autonomyWorkAccepted reports whether every live task is accepted. Retired and
// deleted tasks are history and do not block a ready_to_complete report.
func autonomyWorkAccepted(d *Document) bool {
	for _, t := range d.State.Tasks {
		if retiredTask(t) {
			continue
		}
		if t.State != "accepted" {
			return false
		}
	}
	return true
}

// autonomySuggestedOutcome picks the report outcome the current state implies.
func autonomySuggestedOutcome(d *Document) string {
	if d.State.Manual() {
		return autonomyOutcomeComplete
	}
	if workflowHasCapability(d, capLanding) && (landingLanded(d) || !hasLiveImplementer(d.State.Tasks)) {
		return autonomyOutcomeComplete
	}
	return autonomyOutcomeLand
}

// autonomyNextCommands lists the exact user commands the report recommends.
func autonomyNextCommands(d *Document, outcome string) []string {
	switch outcome {
	case autonomyOutcomeLand:
		return []string{
			fmt.Sprintf("workspace integration land --target %s --expected-revision %d --user-confirmed", integrationTarget(d), d.State.Revision),
			"workspace complete --user-confirmed",
		}
	case autonomyOutcomeComplete:
		return []string{fmt.Sprintf("workspace complete --user-confirmed --expected-revision %d", d.State.Revision)}
	default:
		return nil
	}
}

// readAutonomyArtifact reads a regular file the orchestrator wants to preserve
// as an immutable report artifact. The stored document keeps the digest, so the
// source path is only used once.
func readAutonomyArtifact(path string) ([]byte, string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, "", fail("artifact_invalid", "empty artifact path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	before, err := os.Stat(abs)
	if err != nil {
		return nil, "", err
	}
	if !before.Mode().IsRegular() {
		return nil, "", fail("artifact_invalid", "autonomy report artifact must be a regular file: %s", path)
	}
	if before.Size() > maxArtifactSize {
		return nil, "", fail("artifact_invalid", "artifact exceeds 16 MiB")
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > maxArtifactSize {
		return nil, "", fail("artifact_invalid", "artifact exceeds 16 MiB")
	}
	return b, filepath.Base(abs), nil
}

// validateAutonomyReportOutcome refuses a report whose outcome contradicts the
// durable state. A ready_to_land report re-runs the same integration validation
// as landing, so an integration that changed after acceptance returns
// integration_changed instead of being recommended.
func (s *Service) validateAutonomyReportOutcome(ctx context.Context, d *Document, opt AutonomyReportOptions) error {
	switch opt.Outcome {
	case autonomyOutcomeLand:
		if d.State.Status != "active" || d.State.Workflow == nil || !workflowHasCapability(d, capLanding) || d.State.Workflow.Phase != integrationPhase(d) {
			return fail("workflow_gate", "ready_to_land requires an active landing workflow in the integration phase")
		}
		if _, err := acceptedRole(d, "integrator"); err != nil {
			return err
		}
		if err := validateIntegration(ctx, d); err != nil {
			return err
		}
	case autonomyOutcomeComplete:
		if d.State.Manual() {
			// A manual workspace completes on the accepted task set alone.
		} else if d.State.Workflow != nil && workflowHasCapability(d, capLanding) {
			if d.State.Status != "active" || d.State.Workflow.Phase != integrationPhase(d) {
				return fail("workflow_gate", "ready_to_complete for a landing workflow requires the active integration phase")
			}
			if hasLiveImplementer(d.State.Tasks) && !landingLanded(d) {
				return fail("workflow_gate", "the accepted integration has not landed; report ready_to_land instead")
			}
		} else {
			return fail("workflow_gate", "ready_to_complete is not available for this workflow")
		}
		if !autonomyWorkAccepted(d) {
			return fail("workflow_gate", "every live task must be accepted before a ready_to_complete report")
		}
	case autonomyOutcomeBlocked, autonomyOutcomeFailed:
		if len(opt.Pending) == 0 {
			return fail("pending_required", "a %s report must list the pending items", opt.Outcome)
		}
	default:
		return fail("invalid_outcome", "outcome must be %s, %s, %s or %s", autonomyOutcomeLand, autonomyOutcomeComplete, autonomyOutcomeBlocked, autonomyOutcomeFailed)
	}
	return nil
}

// requireNoUnreviewedHandoffs refuses a final report while a submitted result is
// still awaiting review.
func (s *Service) requireNoUnreviewedHandoffs(d *Document) error {
	for _, h := range d.Registry.Handoffs {
		if h.State == "submitted" {
			return fail("handoff_pending", "review submitted result %s before delivering the report", h.ID)
		}
	}
	return nil
}

// requireNoActiveWorkerRuns refuses a final report while a worker Run is still
// executing. The orchestrator's own Run is allowed.
func (s *Service) requireNoActiveWorkerRuns(d *Document) error {
	for _, p := range d.Registry.Sessions {
		if !p.Active() {
			continue
		}
		if p.ID == s.Actor.SessionID || p.AgentSnapshot.Role == "orchestrator" {
			continue
		}
		return fail("session_active", "stop worker session %s before delivering the report", p.ID)
	}
	return nil
}

// appendFinalReportDecision records the autonomy final report as an audit
// decision atomically with the delivered state.
func (s *Service) appendFinalReportDecision(d *Document, report *AutonomyReport, reason string) {
	now := nowUTC()
	d.State.Decisions = append(d.State.Decisions, Decision{
		ID:         ID("decision"),
		Kind:       "autonomous.final_report",
		Question:   "Deliver the autonomous final report",
		Subject:    "workspace:" + d.State.ID,
		Reason:     strings.TrimSpace(reason),
		Evidence:   append([]string(nil), report.ArtifactIDs...),
		Revision:   d.State.Revision,
		ResolvedBy: "orchestrator",
		Autonomous: true,
		SessionID:  s.Actor.SessionID,
		RunID:      s.Actor.RunID,
		AnsweredAt: &now,
		DecidedAt:  &now,
	})
}

// ReportAutonomy ends an autonomous run with a durable, immutable final report
// instead of a question. It is orchestrator-only, allowed only while the run is
// running, validates the outcome against the state, stores the summary as an
// immutable artifact, sets delivered and replays idempotently under one key.
func (s *Service) ReportAutonomy(ctx context.Context, selector string, opt AutonomyReportOptions, key string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, []string{key}, []any{"autonomy.report", opt}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if !d.State.AutonomyRunning() {
			return fail("autonomy_not_running", "autonomy report is available only while an autonomous run is running")
		}
		switch d.State.Status {
		case "active", "paused":
		default:
			return fail("workspace_not_active", "autonomy report requires an active or paused workspace")
		}
		if opt.ExpectedRevision != 0 && d.State.Revision != opt.ExpectedRevision {
			return fail("revision_conflict", "workspace changed while the report was being prepared")
		}
		if strings.TrimSpace(opt.SummaryFile) == "" {
			return fail("summary_required", "provide --summary-file for the autonomy report")
		}
		if err := s.requireNoUnreviewedHandoffs(d); err != nil {
			return err
		}
		if err := s.requireNoActiveWorkerRuns(d); err != nil {
			return err
		}
		if err := s.validateAutonomyReportOutcome(ctx, d, opt); err != nil {
			return err
		}
		sources := append([]string{opt.SummaryFile}, opt.Artifacts...)
		names := map[string]bool{}
		records := []Artifact{}
		blobs := [][]byte{}
		for _, source := range sources {
			b, name, err := readAutonomyArtifact(source)
			if err != nil {
				return err
			}
			if names[name] {
				return fail("artifact_invalid", "duplicate artifact filename %s", name)
			}
			names[name] = true
			id := ID("art")
			records = append(records, Artifact{ID: id, Name: name, Kind: "report", Path: filepath.ToSlash(filepath.Join("artifacts", id, name)), Digest: digest(b), Size: int64(len(b)), AgentID: s.Actor.AgentID, SessionID: s.Actor.SessionID, RunID: s.Actor.RunID, CreatedAt: nowUTC()})
			blobs = append(blobs, b)
		}
		if d.PendingFiles == nil {
			d.PendingFiles = map[string][]byte{}
		}
		artifactIDs := []string{}
		for i, a := range records {
			d.PendingFiles[a.Path] = blobs[i]
			artifactIDs = append(artifactIDs, a.ID)
		}
		d.State.Artifacts = append(d.State.Artifacts, records...)

		phase := d.State.PhaseLabel()
		integrationHead := ""
		if d.State.Integration != nil {
			integrationHead = d.State.Integration.HeadCommit
		}
		pending := append([]string(nil), opt.Pending...)
		if len(pending) == 0 {
			pending = autonomyNextCommands(d, opt.Outcome)
		}
		now := nowUTC()
		report := &AutonomyReport{
			Outcome:         opt.Outcome,
			Recommendation:  strings.TrimSpace(opt.Recommendation),
			ArtifactIDs:     artifactIDs,
			Phase:           phase,
			IntegrationHead: integrationHead,
			Pending:         pending,
			Revision:        d.State.Revision,
			CreatedAt:       &now,
		}
		d.State.Autonomy.State = "delivered"
		d.State.Autonomy.Report = report
		reason := report.Recommendation
		if reason == "" {
			reason = "outcome " + report.Outcome
		}
		s.appendFinalReportDecision(d, report, reason)
		d.Body += "\n\n## Autonomous report\n\n" + report.Outcome + " — " + reason + "\n"
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
