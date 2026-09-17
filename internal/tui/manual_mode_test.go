package tui

import (
	"strings"
	"testing"
	"time"

	"workspace/internal/core"
)

func workspaceState(id, status string) *Model {
	m := New(Config{ProjectFound: true, WorkspaceID: id, NoColor: true})
	m.width, m.height = 120, 32
	m.snapshot = core.WorkspaceSnapshot{ObservedAt: time.Now(), Status: core.Status{
		Workspace: core.Workspace{ID: id, Title: "Workspace", Status: status, Revision: 3},
	}}
	return m
}

func actionValues(t *testing.T, m *Model) map[string]bool {
	t.Helper()
	options, _ := m.availableActions()
	values := make(map[string]bool, len(options))
	for _, option := range options {
		values[option.Value] = true
	}
	return values
}

// A manual workspace shows the stable "manual" label in the header and offers
// lifecycle controls instead of workflow selection.
func TestManualWorkspaceHeaderAndActions(t *testing.T) {
	m := workspaceState("ws_manual", "active")
	if view := m.View(); !strings.Contains(view, "manual") {
		t.Fatalf("manual label missing from the header:\n%s", view)
	}
	values := actionValues(t, m)
	for _, action := range []string{"pause", "pause_interrupt"} {
		if !values[action] {
			t.Fatalf("manual actions missing %q: %+v", action, values)
		}
	}
	if values["select_workflow"] {
		t.Fatalf("manual actions offered workflow selection: %+v", values)
	}

	m.snapshot.Status.Workspace.Status = "paused"
	values = actionValues(t, m)
	if !values["resume_workspace"] || values["pause"] {
		t.Fatalf("paused manual actions: %+v", values)
	}
}

// A pending creation choice still offers workflow selection and is not labeled
// as manual.
func TestPendingWorkspaceOffersSelectionNotManual(t *testing.T) {
	m := workspaceState("ws_pending", "needs_workflow")
	values := actionValues(t, m)
	if !values["select_workflow"] {
		t.Fatalf("pending actions lost workflow selection: %+v", values)
	}
	if view := m.View(); strings.Contains(view, "manual") {
		t.Fatalf("pending workspace was labeled manual:\n%s", view)
	}
}

// The project picker row exposes the manual mode through the workspace phase
// label supplied by core.
func TestProjectRowShowsManualMode(t *testing.T) {
	m := New(Config{ProjectFound: true, NoColor: true})
	m.width, m.height = 120, 32
	m.project = core.ProjectOverview{ObservedAt: time.Now(), Workspaces: []core.WorkspaceSummary{
		{ID: "ws_manual", Title: "Manual", Status: "active", Phase: "manual"},
	}}
	m.navigate(route{Page: "project"})
	if view := m.View(); !strings.Contains(view, "manual") {
		t.Fatalf("project row omitted the manual mode:\n%s", view)
	}
}
