package tui

import (
	"fmt"
	"strings"

	"workspace/internal/core"
)

// autonomyStateBadge is the canonical, color-independent badge text for a
// workspace autonomy state. The state word is always present so the badge
// survives --no-color mode. An empty string means the workspace is interactive.
func autonomyStateBadge(autonomy *core.Autonomy) string {
	if autonomy == nil || autonomy.State == "" {
		return ""
	}
	switch autonomy.State {
	case "running":
		return "● autonomous running"
	case "delivered":
		return "✓ autonomous delivered"
	case "disabled":
		return "○ autonomous disabled"
	default:
		return "● autonomous " + autonomy.State
	}
}

// autonomyBadge renders the workspace autonomy badge with its semantic color.
// In --no-color mode the palette drops the color and the plain text badge is
// returned, so the state stays readable.
func (m *Model) autonomyBadge() string {
	autonomy := m.snapshot.Status.Workspace.Autonomy
	if autonomy == nil {
		return ""
	}
	text := autonomyStateBadge(autonomy)
	if text == "" {
		return ""
	}
	color := m.palette.secondary
	switch autonomy.State {
	case "running":
		color = m.palette.accent
	case "delivered":
		color = m.palette.success
	}
	return m.palette.style(color, false).Render(text)
}

// autonomyNotice is the short status sentence used by attention and More rows.
func (m *Model) autonomyNotice() string {
	autonomy := m.snapshot.Status.Workspace.Autonomy
	if autonomy == nil {
		return ""
	}
	if autonomy.State == "delivered" && autonomy.Report != nil {
		outcome := firstNonempty(autonomy.Report.Outcome, "delivered")
		return "Autonomous run delivered: " + outcome
	}
	if autonomy.State == "delivered" {
		return "Autonomous run delivered"
	}
	if autonomy.State == "disabled" {
		return "Autonomous run disabled"
	}
	return "Autonomous run running"
}

// autonomyReportPending flattens the report's pending list for a one-line row.
func autonomyReportPending(report *core.AutonomyReport) string {
	if report == nil || len(report.Pending) == 0 {
		return ""
	}
	return strings.Join(report.Pending, " · ")
}

// autonomyContent renders the autonomy record and, when present, the durable
// final report. It is the report view reached from Needs attention or More.
func (m *Model) autonomyContent() string {
	doc := newDetailDoc(m.palette, m.detailWidth())
	autonomy := m.snapshot.Status.Workspace.Autonomy
	if autonomy == nil {
		doc.title("Autonomy", "", "")
		doc.body("This workspace has no autonomy record and follows the interactive contract.")
		doc.action("workspace autonomy enable --reason \"…\" --expected-revision N runs from the terminal.")
		return doc.render()
	}
	doc.title("Autonomy", firstNonempty(autonomy.Mode, "autonomous"), "")
	doc.raw(m.autonomyBadge())
	doc.section("Facts")
	doc.field("Source", firstNonempty(autonomy.Source, "unknown"))
	doc.field("Enabled revision", fmt.Sprint(autonomy.EnabledRevision))
	doc.field("Enabled", formatTimePtr(autonomy.EnabledAt))
	if autonomy.DisabledAt != nil {
		doc.field("Disabled", formatTimePtr(autonomy.DisabledAt))
	}
	if autonomy.DisabledReason != "" {
		doc.field("Disabled reason", autonomy.DisabledReason)
	}
	if autonomy.Report == nil {
		if autonomy.State == "running" {
			doc.section("Progress")
			doc.body("The orchestrator is resolving the orchestrator-level gates and will deliver a durable report instead of a question.")
		}
		return doc.render()
	}
	report := autonomy.Report
	doc.section("Report")
	doc.field("Outcome", firstNonempty(report.Outcome, "delivered"))
	doc.field("Phase", firstNonempty(report.Phase, "unknown"))
	doc.field("Integration head", shortRevision(report.IntegrationHead))
	doc.field("Report revision", fmt.Sprint(report.Revision))
	doc.field("Created", formatTimePtr(report.CreatedAt))
	if report.Recommendation != "" {
		doc.section("Recommendation")
		doc.body(report.Recommendation)
	}
	doc.section("Artifacts")
	doc.bullets(report.ArtifactIDs)
	doc.section("Pending")
	doc.bullets(report.Pending)
	doc.action("Disable the autonomous run from this page with a.")
	return doc.render()
}
