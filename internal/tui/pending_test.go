package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"workspace/internal/core"
)

func idleModel() *Model {
	return New(Config{ProjectFound: true, WorkspaceID: "ws_pending", NoColor: true})
}

// TestSpinnerActivatesForEveryPendingFlag proves that each asynchronous
// operation starts the animation loop exactly once.
func TestSpinnerActivatesForEveryPendingFlag(t *testing.T) {
	flags := []struct {
		name string
		set  func(*Model)
	}{
		{"project", func(m *Model) { m.projectPending = true }},
		{"snapshot", func(m *Model) { m.snapshotPending = true }},
		{"runtime", func(m *Model) { m.runtimePending = true }},
		{"ui status", func(m *Model) { m.uiPending = true }},
		{"preview", func(m *Model) { m.previewPending = true }},
		{"navigation", func(m *Model) { m.navigationPending = true }},
		{"worktree", func(m *Model) { m.worktreePending = true }},
		{"action", func(m *Model) { m.actionPending = true }},
		{"managed hide", func(m *Model) { m.hidePending = true }},
	}
	for _, flag := range flags {
		t.Run(flag.name, func(t *testing.T) {
			m := idleModel()
			if m.animationNeeded() {
				t.Fatal("idle model reported pending animation")
			}
			if cmd := m.ensureAnimation(); cmd != nil {
				t.Fatal("idle model armed an animation tick")
			}
			flag.set(m)
			if !m.animationNeeded() {
				t.Fatalf("%s did not activate animation", flag.name)
			}
			if cmd := m.ensureAnimation(); cmd == nil {
				t.Fatalf("%s did not arm an animation tick", flag.name)
			}
			if !m.pendingWork() {
				t.Fatalf("%s was not reported as pending work", flag.name)
			}
		})
	}
}

// TestIdleModelIssuesNoAnimationTick covers the headline requirement: no tick
// command may be issued when neither work nor a live run is visible.
func TestIdleModelIssuesNoAnimationTick(t *testing.T) {
	m := idleModel()
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("idle Init scheduled commands: %v", cmd)
	}
	// Emulate a previously armed loop that reaches its next frame.
	m.animating = true
	before := m.spinnerGlyph()
	_, cmd := m.Update(animationMsg{})
	if cmd != nil {
		t.Fatal("idle model issued another animation tick")
	}
	if m.animating {
		t.Fatal("idle model kept the animation loop armed")
	}
	if m.spinnerGlyph() != before {
		t.Fatal("idle frame advanced without pending work")
	}
}

// TestLiveRunKeepsSpinnerRunningAndStopsWhenGone proves the visible live-run
// branch of the animation gate, independent of pending flags.
func TestLiveRunKeepsSpinnerRunningAndStopsWhenGone(t *testing.T) {
	m := workFixture()
	if !m.animationNeeded() {
		t.Fatal("fixture live runs did not request animation")
	}
	before := m.spinnerGlyph()
	if _, cmd := m.Update(animationMsg{}); cmd == nil {
		t.Fatal("live run did not keep the animation loop running")
	}
	if m.spinnerGlyph() == before {
		t.Fatal("spinner frame did not advance with a live run")
	}
	if strings.Contains(m.header(), m.spinnerGlyph()) {
		t.Fatalf("live Run without a pending read created a refresh header icon: %q", m.header())
	}

	m.snapshot.Status.Sessions = nil
	m.snapshot.Status.Runs = nil
	if m.animationNeeded() {
		t.Fatal("removing live runs left animation active")
	}
	if _, cmd := m.Update(animationMsg{}); cmd != nil {
		t.Fatal("animation continued after the live run disappeared")
	}
}

// TestRunningGlyphFallsBackWhenIdle shows the running indicator is only animated
// while the loop is active; otherwise it degrades to a static marker.
func TestRunningGlyphFallsBackWhenIdle(t *testing.T) {
	m := idleModel()
	if got := m.stateLabel("running"); !strings.Contains(got, "●") {
		t.Fatalf("idle running label was not static: %q", got)
	}
	m.snapshotPending = true
	got := m.stateLabel("running")
	if strings.Contains(got, "●") {
		t.Fatalf("pending running label was not animated: %q", got)
	}
	if !strings.Contains(got, spinner.MiniDot.Frames[0]) {
		t.Fatalf("pending running label did not use the spinner frame: %q", got)
	}
}

// TestReadProgressUsesHeaderAndLeavesStatusRowAvailable proves background
// reads use the compact header frame while the reserved status row remains
// available for meaningful messages.
func TestReadProgressUsesHeaderAndLeavesStatusRowAvailable(t *testing.T) {
	m := idleModel()
	m.width = 80
	m.snapshotPending = true
	m.runtimePending = true
	frame := m.spinnerGlyph()
	if header := m.header(); !strings.Contains(header, frame) {
		t.Fatalf("pending read header omitted the current MiniDot frame %q: %q", frame, header)
	}
	if row := m.statusRow(); strings.Contains(row, frame) || strings.Contains(row, "Refreshing") {
		t.Fatalf("pending read commandeered the status row: %q", row)
	}

	m.snapshotPending = false
	if header := m.header(); !strings.Contains(header, frame) {
		t.Fatalf("header indicator disappeared before all reads completed: %q", header)
	}
	m.runtimePending = false
	if header := m.header(); strings.Contains(header, frame) {
		t.Fatalf("header indicator survived the final pending read: %q", header)
	}
}

// TestStatusRowPresentsStatesDistinctlyWithoutColor checks each state has its
// own wording and text marker so meaning does not depend on color.
func TestStatusRowPresentsStatesDistinctlyWithoutColor(t *testing.T) {
	spin := spinner.MiniDot.Frames[0]

	t.Run("refresh", func(t *testing.T) {
		m := idleModel()
		m.lastSuccess = time.Now()
		m.snapshotPending = true
		if header := m.header(); !strings.Contains(header, spin) {
			t.Fatalf("refresh header omitted the current MiniDot frame: %q", header)
		}
		row := m.statusRow()
		if strings.Contains(row, "Refreshing") || strings.Contains(row, spin) {
			t.Fatalf("refresh row still contains a read notification or spinner: %q", row)
		}
	})

	t.Run("mutation", func(t *testing.T) {
		m := idleModel()
		m.actionPending = true
		m.lastAction = &ActionCall{Action: "delete_workspace", TargetID: "ws_pending"}
		row := m.statusRow()
		if !strings.Contains(row, spin) || !strings.Contains(row, "Deleting workspace") || !strings.Contains(row, "keep this panel open") {
			t.Fatalf("mutation row unclear: %q", row)
		}
		if strings.Contains(row, "Refreshing") {
			t.Fatalf("mutation was presented as a refresh: %q", row)
		}
	})

	t.Run("failure", func(t *testing.T) {
		m := idleModel()
		m.actionFailure = true
		m.loadError = "boom"
		row := m.statusRow()
		if !strings.Contains(row, "[!] Action failed: boom") || !strings.Contains(row, "y retry") {
			t.Fatalf("failure row unclear: %q", row)
		}
	})

	t.Run("success", func(t *testing.T) {
		m := idleModel()
		m.actionCompleted = true
		m.notice = actionCompletedNotice
		m.snapshotPending = true
		row := m.statusRow()
		if !strings.Contains(row, "✓ Action completed.") || strings.Contains(row, "Refreshing") || strings.Contains(row, spin) {
			t.Fatalf("success row unclear: %q", row)
		}
	})

	t.Run("stale", func(t *testing.T) {
		m := idleModel()
		m.lastSuccess = time.Now()
		m.loadError = "network down"
		m.snapshotPending = true
		row := m.statusRow()
		if !strings.Contains(row, "[!] Stale data from") || !strings.Contains(row, "network down") || !strings.Contains(row, "r retry") {
			t.Fatalf("stale row unclear: %q", row)
		}
	})

	t.Run("notice", func(t *testing.T) {
		m := idleModel()
		m.notice = "Saved."
		m.snapshotPending = true
		if row := m.statusRow(); !strings.Contains(row, "· Saved.") || strings.Contains(row, spin) {
			t.Fatalf("notice row unclear: %q", row)
		}
	})

	t.Run("scroll position", func(t *testing.T) {
		m := detailFixture()
		m.width, m.height = 80, 24
		m.route = route{Page: "task", EntityID: "task_work"}
		m.rebuildViewport()
		m.snapshotPending = true
		row := m.statusRow()
		if !strings.Contains(row, "line ") || strings.Contains(row, spin) {
			t.Fatalf("scroll position was masked by read progress: %q", row)
		}
	})
}

func TestAnimationMsgAdvancesSpinnerFrame(t *testing.T) {
	m := idleModel()
	m.snapshotPending = true
	before := m.spinnerGlyph()
	m.animating = true
	_, _ = m.Update(animationMsg{})
	if m.spinnerGlyph() == before {
		t.Fatal("animation tick did not advance the bubbles spinner")
	}
}

// TestCompletedActionStatusClearsAfterRefresh keeps a transient success from
// masking later background refreshes.
func TestCompletedActionStatusClearsAfterRefresh(t *testing.T) {
	m := idleModel()
	m.actionCompleted = true
	m.notice = actionCompletedNotice
	if row := m.statusRow(); !strings.Contains(row, "✓") {
		t.Fatalf("success status missing before the refresh: %q", row)
	}
	_, _ = m.Update(snapshotMsg{generation: m.generation, value: core.WorkspaceSnapshot{}})
	if m.actionCompleted || m.notice != "" {
		t.Fatalf("success status survived the refresh: completed=%t notice=%q", m.actionCompleted, m.notice)
	}
	if row := m.statusRow(); strings.Contains(row, "✓") {
		t.Fatalf("stale success status remained: %q", row)
	}
}

func TestCommandStatusKeepsLastErrorForRetry(t *testing.T) {
	m := idleModel()
	m.actionFailure = true
	m.loadError = "task is required by task_child"
	_, cmd := m.Update(animationMsg{})
	if cmd != nil {
		t.Fatal("failure state should not animate when idle")
	}
	if !strings.Contains(m.statusRow(), "y retry") {
		t.Fatal("failure status lost the exact retry affordance")
	}
}
