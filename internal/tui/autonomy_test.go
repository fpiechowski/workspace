package tui

import (
	"strings"
	"testing"
	"time"

	"workspace/internal/core"
)

// autonomyFixture builds a workspace-scoped model with the given autonomy
// state and, when applicable, a delivered report.
func autonomyFixture(state string, report *core.AutonomyReport) *Model {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_auto", NoColor: true})
	m.width, m.height = 120, 40
	now := time.Now()
	m.snapshot = core.WorkspaceSnapshot{ObservedAt: now, Status: core.Status{Workspace: core.Workspace{
		ID: "ws_auto", Title: "Autonomy demo", Status: "active", Revision: 7,
		Autonomy: &core.Autonomy{
			Mode: "autonomous", State: state, Source: "create", EnabledRevision: 3,
			EnabledAt: &now, Report: report,
		},
	}}}
	m.navigate(route{Page: "tasks"})
	m.validateSelection()
	return m
}

func deliveredReport() *core.AutonomyReport {
	now := time.Now()
	return &core.AutonomyReport{
		Outcome: "ready_to_land", Recommendation: "Land the accepted integration.",
		ArtifactIDs: []string{"art_1"}, Phase: "integration", IntegrationHead: "abcdef1234567890",
		Pending:  []string{"workspace integration land", "workspace complete"},
		Revision: 9, CreatedAt: &now,
	}
}

// The badge must name the state in both color and --no-color modes, and be
// absent for an interactive workspace.
func TestAutonomyBadgeStatesColorAndNoColor(t *testing.T) {
	for _, state := range []string{"running", "delivered", "disabled"} {
		for _, noColor := range []bool{true, false} {
			m := New(Config{ProjectFound: true, WorkspaceID: "ws_auto", NoColor: noColor})
			m.snapshot.Status.Workspace.Autonomy = &core.Autonomy{State: state}
			badge := m.autonomyBadge()
			if !strings.Contains(badge, state) {
				t.Fatalf("no-color=%t badge %q does not name state %q", noColor, badge, state)
			}
			if noColor && strings.Contains(badge, "\x1b") {
				t.Fatalf("--no-color badge carries an escape sequence: %q", badge)
			}
		}
	}
	plain := New(Config{ProjectFound: true, WorkspaceID: "ws_auto", NoColor: true})
	if badge := plain.autonomyBadge(); badge != "" {
		t.Fatalf("interactive workspace rendered an autonomy badge: %q", badge)
	}
}

// The workspace header always surfaces the current autonomy state.
func TestAutonomyBadgeRendersInWorkspaceHeader(t *testing.T) {
	for _, state := range []string{"running", "delivered", "disabled"} {
		m := autonomyFixture(state, nil)
		m.width, m.height = 160, 40
		m.rebuildViewport()
		view := m.View()
		if !strings.Contains(view, "autonomous "+state) {
			t.Fatalf("workspace header does not show the %q autonomy badge:\n%s", state, view)
		}
	}
}

// A delivered report is a needs-attention entry and opens the report view.
func TestDeliveredReportAppearsInAttentionAndOpensReportView(t *testing.T) {
	m := autonomyFixture("delivered", deliveredReport())
	m.navigate(route{Page: "attention"})
	m.validateSelection()

	items := m.filteredItems()
	found := false
	for _, item := range items {
		if item.Kind == "autonomy" {
			found = true
			m.route.SelectedID = item.ID
			if !strings.Contains(item.Title, "delivered") {
				t.Fatalf("attention report title is unclear: %q", item.Title)
			}
		}
	}
	if !found {
		t.Fatalf("delivered report is missing from Needs attention: %+v", items)
	}
	if _, cmd := m.openSelection(); cmd != nil || m.route.Page != "autonomy" {
		t.Fatalf("opening the attention entry did not reach the report view: page=%q cmd=%v", m.route.Page, cmd)
	}
	content := m.detailContent()
	for _, want := range []string{"ready_to_land", "Land the accepted integration.", "art_1", "workspace integration land"} {
		if !strings.Contains(content, want) {
			t.Fatalf("report view is missing %q:\n%s", want, content)
		}
	}
}

// More exposes the report view so a delivered report stays reachable after the
// attention row is gone; an interactive workspace has no entry.
func TestMoreExposesAutonomyOnlyWhenEnabled(t *testing.T) {
	m := autonomyFixture("delivered", deliveredReport())
	m.navigate(route{Page: "more"})
	ids := map[string]bool{}
	for _, item := range m.filteredItems() {
		ids[item.ID] = true
	}
	if !ids["autonomy"] {
		t.Fatalf("More omitted the autonomy report view: %+v", ids)
	}

	plain := New(Config{ProjectFound: true, WorkspaceID: "ws_plain", NoColor: true})
	plain.snapshot = core.WorkspaceSnapshot{ObservedAt: time.Now(), Status: core.Status{Workspace: core.Workspace{ID: "ws_plain", Status: "active"}}}
	plain.navigate(route{Page: "more"})
	for _, item := range plain.filteredItems() {
		if item.ID == "autonomy" {
			t.Fatal("interactive workspace offered an autonomy page")
		}
	}
}

// The disable action carries the revision guard and a tui_<ULID> key, and is
// available while running or delivered but never after disabling.
func TestDisableAutonomyActionUsesRevisionGuardAndTuiKey(t *testing.T) {
	for _, state := range []string{"running", "delivered"} {
		m := autonomyFixture(state, deliveredReport())
		m.route = route{Page: "orchestrator"}
		values := actionValues(t, m)
		if !values["disable_autonomy"] {
			t.Fatalf("state %q did not offer the disable action: %+v", state, values)
		}
		if cmd := m.beginAction("disable_autonomy", ""); cmd == nil || m.formMode != "reason" {
			t.Fatalf("state %q disable did not open a reason form: mode=%q", state, m.formMode)
		}
		if m.formAction.ExpectedRevision != 7 {
			t.Fatalf("disable lost its revision guard: %+v", m.formAction)
		}
		if !strings.HasPrefix(m.formAction.Key, "tui_") {
			t.Fatalf("disable operation key is not tui_<ULID>: %q", m.formAction.Key)
		}
		if !strings.Contains(actionCaption(m.formAction), "Disable the autonomous run") {
			t.Fatalf("disable caption is unclear: %q", actionCaption(m.formAction))
		}
	}

	disabled := autonomyFixture("disabled", deliveredReport())
	disabled.route = route{Page: "orchestrator"}
	if actionValues(t, disabled)["disable_autonomy"] {
		t.Fatal("disabled workspace still offered the disable action")
	}
}

// The reason step hands the same guarded request, including its key, to the
// confirmation form.
func TestDisableAutonomyReasonKeepsGuardedRequest(t *testing.T) {
	m := autonomyFixture("running", nil)
	m.route = route{Page: "autonomy"}
	if cmd := m.beginAction("disable_autonomy", ""); cmd == nil {
		t.Fatal("disable did not start")
	}
	key := m.formAction.Key
	m.formAction.Reason = "user resumed interactive control"
	if cmd := m.openConfirm("disable_autonomy"); cmd == nil {
		t.Fatal("disable did not reach confirmation")
	}
	if m.formMode != "confirm" || m.formAction.Key != key || m.formAction.ExpectedRevision != 7 || m.formAction.Reason == "" {
		t.Fatalf("confirmation lost the guarded request: mode=%q call=%+v", m.formMode, m.formAction)
	}
}

// The TUI never offers autonomy enable or an automatic result acceptance.
func TestTUIDoesNotOfferAutonomyEnableOrResultAcceptance(t *testing.T) {
	m := autonomyFixture("delivered", deliveredReport())
	for _, page := range []string{"orchestrator", "runtime", "autonomy"} {
		m.route = route{Page: page}
		for _, option := range mustActionValues(t, m) {
			if option == "enable_autonomy" || strings.Contains(option, "accept") || strings.Contains(option, "handoff") {
				t.Fatalf("page %q offered reserved action %q", page, option)
			}
		}
	}
}

// Decisions resolved under autonomy carry the ResolvedBy marker in both the
// collection and the detail document.
func TestAutonomyDecisionShowsResolvedByAndEvidence(t *testing.T) {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_auto", NoColor: true})
	m.width, m.height = 120, 40
	now := time.Now()
	m.snapshot = core.WorkspaceSnapshot{ObservedAt: now, Status: core.Status{Workspace: core.Workspace{
		ID: "ws_auto", Title: "Autonomy demo", Status: "active",
		Decisions: []core.Decision{{
			ID: "decision_1", Kind: "autonomous.handoff_accept",
			Question: "Accept the task result under autonomy", Answer: "accept",
			Reason: "core validation passed", ResolvedBy: "orchestrator", Autonomous: true,
			Subject: "task:task_1", Evidence: []string{"art_1", "check_1"},
			RunID: "run_1", DecidedAt: &now,
		}},
	}}}
	m.navigate(route{Page: "decisions"})
	m.validateSelection()

	items := m.filteredItems()
	if len(items) != 1 {
		t.Fatalf("decision collection lost its row: %+v", items)
	}
	for _, want := range []string{"by orchestrator", "autonomous"} {
		if !strings.Contains(items[0].Subtitle, want) {
			t.Fatalf("decision subtitle %q is missing %q", items[0].Subtitle, want)
		}
	}

	m.route = route{Page: "decision", EntityID: "decision_1"}
	m.rebuildViewport()
	content := m.detailContent()
	for _, want := range []string{"Resolved by", "orchestrator", "Autonomous", "yes", "task:task_1", "art_1", "check_1"} {
		if !strings.Contains(content, want) {
			t.Fatalf("decision detail is missing %q:\n%s", want, content)
		}
	}
}

// The create form accepts the autonomous toggle together with both the named
// workflow and the explicit manual choice.
func TestCreateFormAutonomyCompatibleWithBothWorkflowChoices(t *testing.T) {
	named := createOptions(ActionCall{Action: "create_workspace", TargetName: "Auto", Input: "intent", Workflow: "plan-first", Autonomous: true, Key: "tui:1"})
	if !named.Autonomous || named.Workflow != "plan-first" || named.NoWorkflow {
		t.Fatalf("named workflow + autonomy did not reach CreateOptions: %+v", named)
	}
	manual := createOptions(ActionCall{Action: "create_workspace", TargetName: "Auto manual", Input: "intent", NoWorkflow: true, Autonomous: true, Key: "tui:2"})
	if !manual.Autonomous || !manual.NoWorkflow || manual.Workflow != "" {
		t.Fatalf("manual + autonomy did not reach CreateOptions: %+v", manual)
	}
}

func mustActionValues(t *testing.T, m *Model) []string {
	t.Helper()
	options, _ := m.availableActions()
	values := make([]string, 0, len(options))
	for _, option := range options {
		values = append(values, option.Value)
	}
	return values
}
