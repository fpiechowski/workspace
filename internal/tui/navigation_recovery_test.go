package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"os"
	"os/exec"
	"strings"
	"testing"
	"workspace/internal/core"
)

type externalNavigator struct{}

func (externalNavigator) Select(context.Context, core.NavigationTarget) error { return nil }
func (externalNavigator) PrepareAttach(context.Context, core.NavigationTarget) (*exec.Cmd, error) {
	return exec.Command("echo", "attached"), nil
}

func TestExternalNavigationRequestsProcessAndQuitsProgram(t *testing.T) {
	t.Setenv("TMUX", "")
	m := workFixture()
	m.navigator = externalNavigator{}
	ref := core.EntityRef{Kind: "run", ID: "run_worker"}
	_, cmd := m.Update(navigationTargetMsg{
		generation: m.generation,
		ref:        ref,
		target:     core.NavigationTarget{SessionName: "workspace"},
	})
	if cmd == nil || cmd() != (tea.QuitMsg{}) {
		t.Fatal("external navigation must quit the current program")
	}
	request := m.TakeExternalProcessRequest()
	if request == nil || request.Cmd == nil || request.Ref != ref || !m.quit {
		t.Fatalf("invalid external process request: %#v", request)
	}
	if _, err := os.Stat(request.Cmd.Path); err != nil {
		t.Fatalf("prepared command is not executable: %v", err)
	}
}

func TestMissingPaneOffersReconcileWithoutExecutingIt(t *testing.T) {
	for _, duringSelect := range []bool{false, true} {
		m := workFixture()
		backend := &actionHarness{}
		m.backend = backend
		ref := core.EntityRef{Kind: "run", ID: "run_worker"}
		failure := fmt.Errorf("navigation: %w", &core.Error{Code: "pane_missing", Message: "no live pane"})
		var msg tea.Msg = navigationTargetMsg{generation: m.generation, ref: ref, err: failure}
		if duringSelect {
			msg = navigationResultMsg{generation: m.generation, ref: ref, err: failure}
		}
		m.Update(msg)
		if m.form == nil || m.formAction.Action != "reconcile" || m.formAction.NavigationRef == nil || *m.formAction.NavigationRef != ref {
			t.Fatal("missing reconcile confirmation")
		}
		if len(backend.calls) != 0 || m.actionPending {
			t.Fatal("reconcile executed without consent")
		}
		m.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
		if m.form != nil || len(backend.calls) != 0 {
			t.Fatal("cancel ran reconcile")
		}
	}
}

func TestReconcileRecoveryRetainsTargetAndDoesNotLoop(t *testing.T) {
	m := workFixture()
	m.backend = &actionHarness{}
	m.navigator = unusedNavigator{}
	ref := core.EntityRef{Kind: "run", ID: "run_worker"}
	m.navigationFailure(&core.Error{Code: "pane_missing", Message: "missing"}, ref, false)
	call := m.formAction
	if call.Key == "" {
		t.Fatal("missing operation key")
	}
	m.form = nil
	_, cmd := m.Update(actionResultMsg{generation: m.generation, call: call})
	if cmd == nil || !m.navigationPending {
		t.Fatal("successful reconcile did not schedule navigation")
	}
	m.Update(navigationTargetMsg{generation: m.generation, ref: ref, afterReconcile: true, err: &core.Error{Code: "run_inactive", Message: "historical run"}})
	if m.form != nil || !strings.Contains(m.notice, "use t") {
		t.Fatal("unavailable historical run must not reopen reconcile")
	}
}

func TestNavigationFailuresDoNotOfferUnrelatedOrStaleRecovery(t *testing.T) {
	for _, code := range []string{"run_inactive", "pane_mismatch", "tmux_server_mismatch", "navigation_ambiguous", "runtime_unsupported"} {
		m := workFixture()
		m.backend = &actionHarness{}
		m.navigationFailure(&core.Error{Code: code, Message: code}, core.EntityRef{Kind: "run", ID: "run_worker"}, false)
		if m.form != nil {
			t.Fatalf("offered reconcile for %s", code)
		}
	}
	m := workFixture()
	m.backend = &actionHarness{}
	m.generation = 2
	m.Update(navigationTargetMsg{generation: 1, ref: core.EntityRef{Kind: "run", ID: "old"}, err: &core.Error{Code: "pane_missing", Message: "old"}})
	if m.form != nil {
		t.Fatal("stale workspace response opened confirmation")
	}
	m.navigationFailure(&core.Error{Code: "pane_missing", Message: "missing"}, core.EntityRef{Kind: "run", ID: "run_worker"}, true)
	if m.form != nil {
		t.Fatal("second failure offered another reconcile")
	}
}

func TestListSeparatorsKeepSelectedEntryVisible(t *testing.T) {
	m := workFixture()
	m.navigate(route{Page: "sessions"})
	items := m.filteredItems()
	m.route.SelectedID = items[len(items)-1].ID
	for _, height := range []int{1, 3, 4, 5, 8, 12} {
		rows := m.renderItems(items, 40, height)
		if len(rows) > height {
			t.Fatalf("%d rows exceeds height %d", len(rows), height)
		}
		joined := strings.Join(rows, "\n")
		if !strings.Contains(joined, "› ") {
			t.Fatal("selection disappeared")
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > 40 {
				t.Fatal("row overflow")
			}
		}
		if height >= 5 && !strings.Contains(joined, strings.Repeat("─", 40)) {
			t.Fatal("entries have no separator")
		}
	}
}
