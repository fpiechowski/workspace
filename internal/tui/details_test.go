package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"workspace/internal/core"
)

// longGoal is a single unbreakable token: wrapping must hard-break it rather
// than let it overflow or silently drop characters.
var longGoal = strings.Repeat("goal-", 200)

func detailFixture() *Model {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_detail", NoColor: true})
	now := time.Now()
	m.snapshot = core.WorkspaceSnapshot{
		ObservedAt: now,
		Status: core.Status{
			Workspace: core.Workspace{
				ID: "ws_detail", Title: "Detail demo", Status: "active",
				OrchestratorAgentID: "agent_orch",
				Tasks: []core.Task{{
					TaskSpec: core.TaskSpec{
						Title:              "Retry failed payments",
						Goal:               longGoal,
						Role:               "implementer",
						Profile:            "worker",
						AcceptanceCriteria: []string{"Payments retried", "Receipts persisted"},
						DependsOn:          []string{"task_done"},
						RequiredArtifacts:  []string{"payments.patch"},
					},
					ID: "task_work", State: "running", Attempt: 2,
					WorktreeID: "wt_a", SessionID: "sess_worker", AcceptedHandoff: "handoff_a",
				}},
				Artifacts: []core.Artifact{{
					ID: "artifact_a", Name: "payments.patch", Kind: "patch", Size: 2048,
					Digest: "sha256:abc", SourceHandoff: "handoff_a",
					TaskID: "task_work", SessionID: "sess_worker", RunID: "run_worker", CreatedAt: now,
				}},
				Decisions: []core.Decision{{
					ID: "decision_a", Kind: "choice", Question: "Choose a refund policy",
					Options: []string{"Refund to the original card", "Issue store credit"},
				}},
				ChangeRequests: []core.ChangeRequest{{
					ID: "cr_a", Title: "Retry payments", Branch: "workspace/ws_detail/impl",
					Target: "main", WorktreeID: "wt_a", HeadCommit: "abc123", State: "open",
					Body: strings.Repeat("This is a long change request body line.\n", 20),
					URL:  "https://example.invalid/change/1",
				}},
			},
			Sessions: []core.Session{
				{
					ID: "sess_orch", AgentID: "agent_orch",
					AgentSnapshot: core.Agent{Name: "Coordinator", Role: "orchestrator"},
					CurrentRunID:  "run_orch", LastRunID: "run_orch",
					LifecycleState: "active", LastActiveAt: now,
				},
				{
					ID: "sess_worker", AgentID: "agent_worker",
					AgentSnapshot:  core.Agent{Name: "Payments worker", Role: "implementer"},
					ClientSnapshot: core.Client{Adapter: "codex"}, Route: core.Route{Model: "coder"},
					TaskID: "task_work", TaskAttempt: 2, WorktreeID: "wt_a",
					CurrentRunID: "run_worker", LastRunID: "run_worker",
					ClientThreadID: "thread-1", LifecycleState: "active",
				},
			},
			Runs: []core.Run{
				{ID: "run_orch", SessionID: "sess_orch", State: "running", Route: core.Route{Provider: "openai", Model: "planner", Client: "codex"}},
				{ID: "run_worker", SessionID: "sess_worker", State: "running", Route: core.Route{Provider: "openai", Model: "coder", Client: "codex"}, PaneID: "%1", WindowID: "@1", ClientState: "ready", CreatedAt: now},
			},
			Agents: []core.Agent{{
				ID: "agent_worker", Name: "Payments worker", Role: "implementer", Profile: "worker",
				Instructions: strings.Repeat("Follow the repository instructions.\n", 20),
			}},
			Worktrees: []core.Worktree{{
				ID: "wt_a", Name: "payments", Path: "/tmp/ws/wt_a",
				Branch: "workspace/ws_detail/impl", Purpose: "retry payments", State: "ready",
			}},
		},
		Handoffs: []core.Handoff{{
			ID: "handoff_a", Outcome: "submitted", State: "submitted", TaskID: "task_work", Attempt: 2,
			FromAgent: "agent_worker", ToAgent: "agent_orch",
			Summary:  strings.Repeat("summary text ", 60),
			Risks:    []string{"Risk one", "Risk two"},
			Feedback: "Feedback body", ArtifactIDs: []string{"artifact_a"},
		}},
		Checks: []core.CheckReceipt{{
			ID: "check_a", TaskID: "task_work", Attempt: 2, State: "passed", ExitCode: 0,
			Argv: []string{"go", "test", "-count=1", "./..."}, Head: "aaaa", EndHead: "bbbb", Digest: "sha256:def",
		}},
		Services: []core.BackgroundService{{
			ID: "svc_a", Name: "payments-api", WorktreeID: "wt_a", CWD: "/tmp/ws/wt_a",
			Argv: []string{"go", "run", "./cmd/api"}, PaneID: "%2", State: "running",
		}},
	}
	m.runtime = core.RuntimeObservation{
		State: "present", SupervisorState: "running", ObservedAt: now,
		Topology: core.TmuxTopology{
			SessionName: "workspace-ws_detail", SessionExists: true,
			Windows: []core.TmuxWindow{{ID: "@1", Name: "orchestrator", Kind: "orchestrator", Active: true}},
			Panes:   []core.Pane{{ID: "%1", WindowID: "@1", Kind: "agent", SessionID: "sess_worker", RunID: "run_worker"}},
		},
	}
	return m
}

// detailPages lists every detail route together with the entity it needs.
func detailPages() []route {
	return []route{
		{Page: "task", EntityID: "task_work"},
		{Page: "worktree", EntityID: "wt_a"},
		{Page: "session", EntityID: "sess_worker"},
		{Page: "run", EntityID: "run_worker"},
		{Page: "agent", EntityID: "agent_worker"},
		{Page: "service", EntityID: "svc_a"},
		{Page: "artifact", EntityID: "artifact_a"},
		{Page: "handoff", EntityID: "handoff_a"},
		{Page: "check", EntityID: "check_a"},
		{Page: "decision", EntityID: "decision_a"},
		{Page: "change_request", EntityID: "cr_a"},
		{Page: "runtime"},
		{Page: "orchestrator"},
	}
}

// TestDetailDocumentHasConsistentHierarchy proves identity/status come first,
// then facts, narrative, related resources and finally provenance.
func TestDetailDocumentHasConsistentHierarchy(t *testing.T) {
	m := detailFixture()
	m.width, m.height = 120, 40
	m.route = route{Page: "task", EntityID: "task_work"}
	m.rebuildViewport()

	content := m.detailContent()
	if !strings.HasPrefix(content, "Task") {
		t.Fatalf("detail document does not start with the identity:\n%s", content)
	}
	if !strings.Contains(content, "● running") {
		t.Fatalf("detail document does not show the status badge:\n%s", content)
	}
	order := []string{"Facts", "Goal", "Acceptance criteria", "Depends on", "Required artifacts", "Related resources", "Provenance"}
	previous := -1
	for _, heading := range order {
		index := strings.Index(content, heading)
		if index < 0 {
			t.Fatalf("detail document is missing the %q section:\n%s", heading, content)
		}
		if index <= previous {
			t.Fatalf("section %q is out of hierarchy order:\n%s", heading, content)
		}
		previous = index
	}
	if !strings.Contains(content, "ID: task_work") {
		t.Fatalf("provenance does not carry the resource ID:\n%s", content)
	}
}

func TestLiveSessionPresentationKeepsOperationalRunAndClientFacts(t *testing.T) {
	m := detailFixture()
	for i := range m.snapshot.Status.Sessions {
		if m.snapshot.Status.Sessions[i].ID == "sess_worker" {
			m.snapshot.Status.Sessions[i].State = "running"
			m.snapshot.Status.Sessions[i].RunState = "running"
			m.snapshot.Status.Sessions[i].ClientState = "idle"
		}
	}
	m.snapshot.Status.Runs[1].ClientState = "idle"
	m.width, m.height = 120, 40
	m.route = route{Page: "session", EntityID: "sess_worker"}
	m.rebuildViewport()
	sessionDetail := m.detailContent()
	for _, want := range []string{"● running", "Lifecycle: ● active", "Run state: ● running", "Client state: idle"} {
		if !strings.Contains(sessionDetail, want) {
			t.Fatalf("Session detail missing %q:\n%s", want, sessionDetail)
		}
	}
	rows := m.runtimeRows()
	foundRunningPane := false
	for _, row := range rows {
		if row.Run == "run_worker" {
			foundRunningPane = row.State == "running"
		}
	}
	if !foundRunningPane {
		t.Fatalf("runtime pane did not show the active operational state: %+v", rows)
	}
	m.snapshot.Status.Sessions[0].State = "running"
	m.snapshot.Status.Sessions[0].RunState = "running"
	m.snapshot.Status.Sessions[0].ClientState = "idle"
	m.snapshot.Status.Runs[0].ClientState = "idle"
	orchestratorDetail := m.orchestratorContent()
	for _, want := range []string{"Operational state: ● running", "Lifecycle: ● active", "Run state: ● running", "Client state: idle"} {
		if !strings.Contains(orchestratorDetail, want) {
			t.Fatalf("Orchestrator detail missing %q:\n%s", want, orchestratorDetail)
		}
	}
}

// TestDetailDocReusableBlocks covers the shared renderers directly: sections,
// fields, bullets, warnings, links and provenance.
func TestDetailDocReusableBlocks(t *testing.T) {
	doc := newDetailDoc(palette{noColor: true}, 24)
	doc.title("Task", "Retry failed payments", "running")
	doc.action("Enter details")
	doc.section("Facts")
	doc.field("Attempt", "2")
	doc.field("Path", "/a/very/long/path/that/must/wrap/cleanly")
	doc.section("Goal")
	doc.body("first line\nsecond line")
	doc.section("Acceptance criteria")
	doc.bullets([]string{"One", "Two"})
	doc.section("Depends on")
	doc.bullets(nil)
	doc.warning("something went wrong and needs attention")
	doc.link("1 Sessions · 2 Worktrees")
	doc.provenance("ID", "task_work")

	content := doc.render()
	// Wrapping may split multi-word expectations across lines, so compare a
	// whitespace-free projection as well as the literal markers.
	compact := strings.NewReplacer("\n", "", " ", "").Replace(content)
	for _, want := range []string{"Task", "Retry failed payments", "running", "Facts", "Attempt", "Goal", "first line", "second line", "Acceptance criteria", "Depends on", "  —", "[!] something", "1 Sessions · 2 Worktrees", "ID: task_work"} {
		if !strings.Contains(content, want) && !strings.Contains(compact, strings.NewReplacer(" ", "").Replace(want)) {
			t.Fatalf("document block output is missing %q:\n%s", want, content)
		}
	}
	for i, line := range strings.Split(content, "\n") {
		if width := ansi.StringWidth(line); width > 24 {
			t.Fatalf("block line %d rendered %d columns: %q", i, width, line)
		}
	}
}

// TestLongDetailContentWrapsWithoutOverflow wraps a long unbreakable goal and a
// long Unicode goal to the viewport and keeps every frame line in bounds.
func TestLongDetailContentWrapsWithoutOverflow(t *testing.T) {
	longUnicode := strings.Repeat("漢字", 120)
	for _, size := range [][2]int{{40, 12}, {60, 24}, {80, 18}, {100, 24}, {120, 32}} {
		m := detailFixture()
		m.snapshot.Status.Workspace.Tasks[0].Goal = longGoal
		m.width, m.height = size[0], size[1]
		m.route = route{Page: "task", EntityID: "task_work"}
		m.rebuildViewport()

		content := m.detailContent()
		compact := strings.NewReplacer("\n", "", " ", "").Replace(content)
		if !strings.Contains(compact, longGoal) {
			t.Fatalf("%dx%d dropped part of the long goal instead of wrapping it", size[0], size[1])
		}
		for i, line := range strings.Split(content, "\n") {
			if width := ansi.StringWidth(line); width > size[0] {
				t.Fatalf("%dx%d detail line %d rendered %d columns: %q", size[0], size[1], i, width, line)
			}
		}

		m.snapshot.Status.Workspace.Tasks[0].Goal = longUnicode
		m.rebuildViewport()
		unicodeContent := m.detailContent()
		unicodeCompact := strings.NewReplacer("\n", "", " ", "").Replace(unicodeContent)
		if !strings.Contains(unicodeCompact, longUnicode) {
			t.Fatalf("%dx%d dropped part of the long Unicode goal", size[0], size[1])
		}
	}
}

// TestDetailSanitizesExternalContentBeforeStyling proves escape sequences in
// external values are removed while the document's own styling survives.
func TestDetailSanitizesExternalContentBeforeStyling(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(previous)

	m := New(Config{ProjectFound: true, WorkspaceID: "ws_detail", Theme: "dark"})
	m.palette = darkPalette()
	m.snapshot = core.WorkspaceSnapshot{ObservedAt: time.Now(), Status: core.Status{Workspace: core.Workspace{
		ID: "ws_detail", Title: "Detail demo", Status: "active",
		Tasks: []core.Task{{ID: "task_x", TaskSpec: core.TaskSpec{
			Title: "Injected\x1b[31m title",
			Goal:  "safe red\x1b[31mINK\x1b]52;c;clipboard-secret\a\x1bP1;2;dcs-secret\x1b\\end",
		}, State: "running"}},
	}}}
	m.width, m.height = 120, 40
	m.route = route{Page: "task", EntityID: "task_x"}
	m.rebuildViewport()

	content := m.detailContent()
	if strings.Contains(content, "clipboard-secret") || strings.Contains(content, "dcs-secret") || strings.Contains(content, "\x1b]") {
		t.Fatalf("terminal control payload survived sanitization:\n%q", content)
	}
	if !strings.Contains(content, "redINK") || !strings.Contains(content, "end") {
		t.Fatalf("sanitization removed ordinary text:\n%s", content)
	}
	if !strings.Contains(content, "\x1b[") {
		t.Fatalf("document styling was stripped, so sanitization ran after styling:\n%q", content)
	}
}

// TestPreviewPreservesExplicitNewlinesAndWarns covers preview metadata, warning
// callouts, tab expansion and explicit newlines.
func TestPreviewPreservesExplicitNewlinesAndWarns(t *testing.T) {
	m := detailFixture()
	m.width, m.height = 100, 24
	m.route = route{Page: "preview", EntityID: "artifact_a"}
	m.preview = core.Preview{
		ResourceID: "artifact_a", Name: "payments.patch", Size: 1234, Digest: "sha256:abc",
		Binary: true, Truncated: true, Warning: "custom warning",
		Text: "alpha\nbeta\tgamma",
	}
	m.rebuildViewport()

	content := m.detailContent()
	for _, want := range []string{
		"[!] Binary file · text preview is unavailable.",
		"[!] Preview truncated at 256 KiB",
		"[!] custom warning",
		"alpha\nbeta    gamma",
		"Digest: sha256:abc",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("preview is missing %q:\n%s", want, content)
		}
	}
	for i, line := range strings.Split(content, "\n") {
		if width := ansi.StringWidth(line); width > m.width {
			t.Fatalf("preview line %d rendered %d columns: %q", i, width, line)
		}
	}
}

// TestDetailScrollIndicatorOnlyOnOverflow keeps the indicator honest: it
// appears with overflowing content and stays absent when everything fits.
func TestDetailScrollIndicatorOnlyOnOverflow(t *testing.T) {
	m := detailFixture()
	m.width, m.height = 80, 24
	m.route = route{Page: "task", EntityID: "task_work"}
	m.rebuildViewport()
	if m.scrollPosition() == "" {
		t.Fatal("long detail content did not report a scroll position")
	}
	view := m.View()
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[len(lines)-2], "line ") {
		t.Fatalf("status row does not show the scroll indicator: %q", lines[len(lines)-2])
	}

	short := detailFixture()
	short.snapshot.Status.Workspace.Tasks[0].Goal = "short"
	short.snapshot.Status.Workspace.Tasks[0].AcceptanceCriteria = nil
	short.snapshot.Status.Workspace.Tasks[0].DependsOn = nil
	short.snapshot.Status.Workspace.Tasks[0].RequiredArtifacts = nil
	short.width, short.height = 120, 60
	short.route = route{Page: "task", EntityID: "task_work"}
	short.rebuildViewport()
	if position := short.scrollPosition(); position != "" {
		t.Fatalf("fits-in-viewport detail still reported %q", position)
	}
}

// TestDetailScrollOffsetRestoresOnRouteReturn proves route memory restores a
// detail viewport offset, not just previews.
func TestDetailScrollOffsetRestoresOnRouteReturn(t *testing.T) {
	m := detailFixture()
	m.width, m.height = 80, 24
	m.navigate(route{Page: "task", EntityID: "task_work"})
	m.viewport.SetYOffset(6)
	if m.viewport.YOffset != 6 {
		t.Fatalf("could not scroll the detail viewport: %d", m.viewport.YOffset)
	}
	m.navigate(route{Page: "run", EntityID: "run_worker"})
	m.navigate(route{Page: "task", EntityID: "task_work"})
	if m.viewport.YOffset != 6 {
		t.Fatalf("returning to the task detail lost its scroll position: got %d, want 6", m.viewport.YOffset)
	}
}

// TestDetailArrowKeysScrollViewport proves arrow keys scroll a detail document
// and update the overflow indicator.
func TestDetailArrowKeysScrollViewport(t *testing.T) {
	m := detailFixture()
	m.width, m.height = 80, 24
	m.route = route{Page: "task", EntityID: "task_work"}
	m.rebuildViewport()

	if _, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyDown}); m.viewport.YOffset == 0 {
		t.Fatal("Down did not scroll the detail viewport")
	}
	top := m.viewport.YOffset
	if _, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyUp}); m.viewport.YOffset >= top {
		t.Fatalf("Up did not scroll the detail viewport back: %d -> %d", top, m.viewport.YOffset)
	}
}

// TestMissingEntityRecovery gives a removed selection an explicit recovery path.
func TestMissingEntityRecovery(t *testing.T) {
	m := detailFixture()
	m.width, m.height = 100, 24
	m.route = route{Page: "task", EntityID: "task_gone"}
	m.rebuildViewport()

	content := m.detailContent()
	for _, want := range []string{"[!] Task no longer exists", "ID: task_gone", "The selection has changed. Press Esc to return.", "Press r to refresh."} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing-entity document is missing %q:\n%s", want, content)
		}
	}
}

// TestDetailPagesFitAtSupportedSizes renders every detail document across the
// supported frame matrix without overflow.
func TestDetailPagesFitAtSupportedSizes(t *testing.T) {
	sizes := [][2]int{{40, 12}, {60, 24}, {80, 18}, {100, 24}, {120, 32}}
	for _, page := range detailPages() {
		for _, size := range sizes {
			m := detailFixture()
			m.width, m.height = size[0], size[1]
			m.route = page
			m.rebuildViewport()
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) != size[1] {
				t.Fatalf("%s %dx%d rendered %d rows", page.Page, size[0], size[1], len(lines))
			}
			for i, line := range lines {
				if width := ansi.StringWidth(line); width > size[0] {
					t.Fatalf("%s %dx%d line %d rendered %d columns: %q", page.Page, size[0], size[1], i, width, line)
				}
			}
		}
	}
}
