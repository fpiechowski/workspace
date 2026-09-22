package tui

import (
	"context"
	"os/exec"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"workspace/internal/core"
)

type intentNavigator struct {
	selects      int
	dedicated    int
	attaches     int
	dedicatedErr error
}

func (n *intentNavigator) Select(context.Context, core.NavigationTarget) error {
	n.selects++
	return nil
}

func (n *intentNavigator) OpenDedicated(context.Context, core.NavigationTarget) error {
	n.dedicated++
	return n.dedicatedErr
}

func (n *intentNavigator) PrepareAttach(context.Context, core.NavigationTarget) (*exec.Cmd, error) {
	n.attaches++
	return exec.Command("echo", "attached"), nil
}

func TestTerminalKeyUsesDedicatedIntentAndKeepsTUIRunning(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual", true: "managed"}[managed], func(t *testing.T) {
			if managed {
				t.Setenv("TMUX", "/tmp/tmux-1000/private,1,0")
			} else {
				t.Setenv("TMUX", "")
			}
			model := workFixture()
			model.managed = managed
			model.backend = &terminalHarness{}
			navigator := &intentNavigator{}
			model.navigator = navigator
			model.navigate(route{Page: "sessions"})
			model.route.SelectedID = "sess_worker"

			resolve := model.openTerminal()
			if resolve == nil {
				t.Fatal("t did not schedule target resolution")
			}
			message, ok := resolve().(navigationTargetMsg)
			if !ok || message.mode != NavigationModeDedicated {
				t.Fatalf("resolved navigation mode = %#v, want dedicated", message)
			}
			_, effect := model.update(message)
			if effect == nil {
				t.Fatal("dedicated navigation effect was not scheduled")
			}
			result, ok := navigationResultMessage(effect)
			if !ok || result.mode != NavigationModeDedicated {
				t.Fatalf("navigation result = %#v, want dedicated mode", result)
			}
			_, refresh := model.update(result)
			if refresh == nil || navigator.dedicated != 1 || navigator.selects != 0 || navigator.attaches != 0 {
				t.Fatalf("unexpected terminal routing: refresh=%v navigator=%+v", refresh != nil, navigator)
			}
			if model.quit || model.TakeExternalProcessRequest() != nil {
				t.Fatal("dedicated t action quit or created an external stdio request")
			}
			if model.navigationPending || model.notice != "Dedicated terminal opened or reused." {
				t.Fatalf("dedicated completion state: pending=%t notice=%q", model.navigationPending, model.notice)
			}
		})
	}
}

func TestReconcileRetryRetainsDedicatedNavigationMode(t *testing.T) {
	model := workFixture()
	model.backend = &navigationActionHarness{}
	model.navigator = &intentNavigator{}
	ref := core.EntityRef{Kind: "run", ID: "run_worker"}
	model.navigationFailure(&core.Error{Code: "pane_missing", Message: "missing"}, ref, false, NavigationModeDedicated)
	if model.formAction.NavigationMode != NavigationModeDedicated {
		t.Fatalf("reconcile action mode = %q, want dedicated", model.formAction.NavigationMode)
	}
	call := model.formAction
	model.form = nil
	_, command := model.update(actionResultMsg{generation: model.generation, call: call})
	if command == nil {
		t.Fatal("reconcile did not schedule retry")
	}
	for _, message := range navigationTargetMessages(command) {
		if message.mode != NavigationModeDedicated || message.ref != ref || !message.afterReconcile {
			t.Fatalf("reconcile retry lost intent: %#v", message)
		}
		return
	}
	t.Fatal("reconcile command did not resolve a navigation target")
}

func TestStaleDedicatedNavigationResultCannotRouteCurrentGeneration(t *testing.T) {
	model := workFixture()
	navigator := &intentNavigator{}
	model.navigator = navigator
	model.generation = 2
	model.navigationPending = true
	model.Update(navigationTargetMsg{
		generation: 1, mode: NavigationModeDedicated,
		target: core.NavigationTarget{SessionName: "stale"},
	})
	model.Update(navigationResultMsg{generation: 1, mode: NavigationModeDedicated})
	if navigator.dedicated != 0 || !model.navigationPending || model.quit {
		t.Fatalf("stale dedicated result changed current route: navigator=%+v pending=%t quit=%t", navigator, model.navigationPending, model.quit)
	}
}

type navigationActionHarness struct {
	terminalHarness
}

func (h *navigationActionHarness) PerformAction(context.Context, string, ActionCall) error {
	return nil
}

func (h *navigationActionHarness) WorkspaceSnapshot(context.Context, string) (core.WorkspaceSnapshot, error) {
	return core.WorkspaceSnapshot{}, nil
}

func (h *navigationActionHarness) ObserveWorkspaceRuntime(context.Context, string) (core.RuntimeObservation, error) {
	return core.RuntimeObservation{}, nil
}

func navigationResultMessage(command tea.Cmd) (navigationResultMsg, bool) {
	if command == nil {
		return navigationResultMsg{}, false
	}
	message := command()
	switch message := message.(type) {
	case navigationResultMsg:
		return message, true
	case tea.BatchMsg:
		for _, child := range message {
			if result, ok := navigationResultMessage(child); ok {
				return result, true
			}
		}
	}
	return navigationResultMsg{}, false
}

func navigationTargetMessages(command tea.Cmd) []navigationTargetMsg {
	if command == nil {
		return nil
	}
	message := command()
	switch message := message.(type) {
	case navigationTargetMsg:
		return []navigationTargetMsg{message}
	case tea.BatchMsg:
		var messages []navigationTargetMsg
		for _, child := range message {
			messages = append(messages, navigationTargetMessages(child)...)
		}
		return messages
	default:
		return nil
	}
}
