package tui

import (
	"strings"
	"testing"
	"time"

	"workspace/internal/core"
)

func TestProjectScopeIssuesAndDispatcherRoutes(t *testing.T) {
	m := New(Config{ProjectFound: true, ProjectID: "proj_test", NoColor: true})
	m.width, m.height = 100, 30
	m.project = core.ProjectOverview{
		ProjectID:  "proj_test",
		Issues:     []core.IssueSummary{{ID: "issue_1", Title: "Checkout retry", Status: "open", Revision: 2, LinkedWorkspaceCount: 1, LinkedStateSummary: "active 1", UpdatedAt: time.Now()}},
		Dispatcher: core.DispatcherSummary{State: "running", Role: "dispatcher", Profile: "frontier", CurrentRunID: "run_1", RunCount: 3},
	}

	m.route = route{Page: "issues"}
	items := m.allItems()
	if len(items) != 1 || items[0].Kind != "issue" || !strings.Contains(items[0].Subtitle, "revision 2") {
		t.Fatalf("unexpected Issue collection: %+v", items)
	}
	if !strings.Contains(m.tabs(), "2 Issues") || !strings.Contains(m.tabs(), "3 Dispatcher") {
		t.Fatalf("project tabs omit first-class routes: %q", m.tabs())
	}

	m.issue = core.IssueDetail{Issue: core.Issue{ID: "issue_1", ProjectID: "proj_test", Title: "Checkout retry", Body: "Second payment attempt fails.", Status: "open", Revision: 2, Digest: "sha256:test"}}
	m.route = route{Page: "issue", EntityID: "issue_1"}
	if content := m.detailContent(); !strings.Contains(content, "Second payment attempt fails.") || !strings.Contains(content, "Revision") {
		t.Fatalf("Issue detail omitted durable content: %q", content)
	}

	m.route = route{Page: "dispatcher"}
	if content := m.detailContent(); !strings.Contains(content, "Project Dispatcher") || !strings.Contains(content, "running") {
		t.Fatalf("Dispatcher detail omitted project state: %q", content)
	}
}
