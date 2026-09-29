package tui

import (
	"strings"
	"testing"
	"time"

	"workspace/internal/core"
)

func TestSlugIDsRenderUntruncatedInRows(t *testing.T) {
	const (
		longWorkspace = "ws_really-long-workspace-slug"
		longAgent     = "agent_really-long-agent-slug"
		longSession   = "sess_really-long-session-slug"
		longTask      = "task_really-long-task-slug"
		ulidRun       = "run_01M3Q8DTG5C14ZW5KSCQT4SQXF"
	)
	m := New(Config{ProjectFound: true, WorkspaceID: longWorkspace, NoColor: true})
	m.project = core.ProjectOverview{Workspaces: []core.WorkspaceSummary{
		{ID: longWorkspace, Title: "Long slug workspace", Status: "active", Phase: "planning", CreatedAt: time.Now()},
	}}
	m.snapshot = core.WorkspaceSnapshot{ObservedAt: time.Now(), Status: core.Status{
		Workspace: core.Workspace{
			ID: longWorkspace, Title: "Long slug workspace", Status: "active", OrchestratorAgentID: longAgent,
			Tasks: []core.Task{{ID: longTask, TaskSpec: core.TaskSpec{Title: "Long slug task"}, State: "running", Attempt: 1}},
		},
		Agents: []core.Agent{{ID: longAgent, Name: "Persona", Role: "planner", Profile: "worker"}},
		Sessions: []core.Session{{
			ID: longSession, AgentID: longAgent, AgentSnapshot: core.Agent{Name: "Persona"},
			CurrentRunID: ulidRun, LifecycleState: "active", State: "running",
		}},
	}}

	cases := []struct{ page, id string }{
		{"project", longWorkspace},
		{"tasks", longTask},
		{"sessions", longSession},
		{"agents", longAgent},
	}
	for _, tc := range cases {
		m.route = route{Page: tc.page}
		found := false
		for _, item := range m.allItems() {
			if strings.Contains(item.Subtitle, tc.id) || strings.Contains(item.Title, tc.id) {
				found = true
			}
			if strings.Contains(item.Subtitle, tc.id[:12]+"…") || strings.Contains(item.Title, tc.id[:12]+"…") {
				t.Errorf("%s row truncated %q: title=%q subtitle=%q", tc.page, tc.id, item.Title, item.Subtitle)
			}
		}
		if !found {
			t.Errorf("%s rows do not show the full ID %q", tc.page, tc.id)
		}
	}
}

func TestRowIDKeepsRunIDCompact(t *testing.T) {
	if got := rowID("workspace", "ws_really-long-workspace-slug"); got != "ws_really-long-workspace-slug" {
		t.Fatalf("workspace row id truncated: %q", got)
	}
	if got := rowID("task", "task_really-long-task-slug"); got != "task_really-long-task-slug" {
		t.Fatalf("task row id truncated: %q", got)
	}
	if got := rowID("agent", "agent_really-long-agent-slug"); got != "agent_really-long-agent-slug" {
		t.Fatalf("agent row id truncated: %q", got)
	}
	if got := rowID("session", "sess_really-long-session-slug"); got != "sess_really-long-session-slug" {
		t.Fatalf("session row id truncated: %q", got)
	}
	run := "run_01M3Q8DTG5C14ZW5KSCQT4SQXF"
	if got := rowID("run", run); got != run[:12]+"…" {
		t.Fatalf("run row id = %q, want the compact form", got)
	}
}
