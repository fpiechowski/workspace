package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"workspace/internal/core"
	"workspace/internal/terminal"
)

func dispatcherFlowTarget() core.NavigationTarget {
	return core.NavigationTarget{
		ProjectID: "proj_test", SessionName: "workspace-dispatcher-proj_test", Socket: "private",
		WindowID: "@disp", PaneID: "%disp", Kind: "dispatcher", SessionID: "sess_disp", RunID: "run_disp",
	}
}

func dispatcherFixture(state string) *Model {
	m := workFixture()
	m.workspaceID = ""
	m.project = core.ProjectOverview{ProjectID: "proj_test", Dispatcher: core.DispatcherSummary{State: state, CurrentRunID: "run_disp"}}
	m.route = route{Page: "dispatcher"}
	return m
}

func TestDispatcherJumpResolvesVerifiedTargetThroughPicker(t *testing.T) {
	target := dispatcherFlowTarget()
	backend := &navigationFlowBackend{targets: []core.NavigationTarget{target}}
	navigator := &navigationFlowNavigator{clients: flowClients()}
	prefs := &memoryClientPreferences{values: map[string]terminal.Client{}}
	model := dispatcherFixture("running")
	model.backend, model.navigator, model.clientPreferences = backend, navigator, prefs

	command := model.jumpSelected()
	if command == nil {
		t.Fatal("g did not start Dispatcher navigation")
	}
	resolved, ok := command().(navigationTargetMsg)
	if !ok {
		t.Fatalf("g returned %T, want a target-resolution message", command())
	}
	_, discover := model.handleNavigationTarget(resolved)
	if discover == nil {
		t.Fatal("Dispatcher target resolution did not start client discovery")
	}
	clientsMessage, ok := discover().(navigationClientsMsg)
	if !ok {
		t.Fatalf("client discovery returned %T", discover())
	}
	model.Update(clientsMessage)
	if model.form == nil || model.formMode != "navigation_client" {
		t.Fatalf("g did not open the client picker: form=%v mode=%q", model.form != nil, model.formMode)
	}
	if backend.resolveCalls[0] != (core.EntityRef{Kind: "dispatcher"}) {
		t.Fatalf("resolved wrong ref: %+v", backend.resolveCalls)
	}
	if len(navigator.listed) != 1 || navigator.listed[0] != target {
		t.Fatalf("client discovery used a different target: %+v", navigator.listed)
	}

	jumpCommand := pressFormKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	result, ok := findNavigationResult(jumpCommand)
	if !ok || result.err != nil {
		t.Fatalf("dispatcher jump result = %#v", result)
	}
	if len(navigator.jumps) != 1 || navigator.jumps[0].target != target || navigator.jumps[0].client != model.navigationClients[0] {
		t.Fatalf("jump did not use the exact target and chosen client: %+v", navigator.jumps)
	}
	if len(backend.resolveCalls) != 2 {
		t.Fatalf("dispatcher target was not rechecked before switching: %+v", backend.resolveCalls)
	}
	model.Update(result)
	if len(prefs.saves) != 1 || prefs.saves[0].socket != target.Socket || prefs.saves[0].client != navigator.jumps[0].client {
		t.Fatalf("successful dispatcher jump did not save the chosen client: %+v", prefs.saves)
	}
}

func TestDispatcherJumpIsInertWithoutLiveRun(t *testing.T) {
	for _, state := range []string{"never_started", "idle", "stopped"} {
		t.Run(state, func(t *testing.T) {
			model := dispatcherFixture(state)
			model.backend = &navigationFlowBackend{}
			model.navigator = &navigationFlowNavigator{}
			if model.contextFlags().jump {
				t.Fatalf("jump advertised for %s Dispatcher", state)
			}
			if help := collapseHelp(model.helpView()); strings.Contains(help, "g jump") {
				t.Fatalf("full help advertises g jump for %s Dispatcher:\n%s", state, help)
			}
			if content := model.detailContent(); strings.Contains(content, "g jump") {
				t.Fatalf("page hint advertises g jump for %s Dispatcher:\n%s", state, content)
			}
			_, command := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
			if command != nil || model.navigationPending || model.form != nil {
				t.Fatalf("g had an effect for %s Dispatcher: command=%v pending=%t form=%v", state, command != nil, model.navigationPending, model.form != nil)
			}
		})
	}
}

func TestDispatcherAdvertisesJumpWhenRunning(t *testing.T) {
	model := dispatcherFixture("running")
	if !model.contextFlags().jump {
		t.Fatal("running Dispatcher did not advertise jump")
	}
	if help := collapseHelp(model.helpView()); !strings.Contains(help, "g jump") {
		t.Fatalf("full help omits g jump for running Dispatcher:\n%s", help)
	}
	if content := model.detailContent(); !strings.Contains(content, "g jump") {
		t.Fatalf("page hint omits g jump for running Dispatcher:\n%s", content)
	}
	options, _ := model.availableActions()
	for _, option := range options {
		if option.Value == "jump" {
			t.Fatalf("actions menu gained a jump entry: %+v", options)
		}
	}
}

func TestDispatcherTargetChangeWhilePickerOpenRequiresFreshChoice(t *testing.T) {
	first := dispatcherFlowTarget()
	changed := first
	changed.PaneID = "%new"
	backend := &navigationFlowBackend{targets: []core.NavigationTarget{changed}}
	navigator := &navigationFlowNavigator{clients: flowClients()[:1]}
	prefs := &memoryClientPreferences{values: map[string]terminal.Client{}}
	model := dispatcherFixture("running")
	model.backend, model.navigator, model.clientPreferences = backend, navigator, prefs
	model.navigationPending = true
	model.navigationSequence = 3
	model.navigationGeneration = model.generation
	model.navigationTarget = first
	model.navigationRef = core.EntityRef{Kind: "dispatcher"}
	model.navigationClients = navigator.clients

	command := model.acceptNavigationClient("0")
	result := command().(navigationResultMsg)
	if result.err == nil || !isNavigationError(result.err, "navigation_target_changed") || len(navigator.jumps) != 0 || len(prefs.saves) != 0 {
		t.Fatalf("changed dispatcher target was jumped without a new choice: %#v", result)
	}
}

func TestDispatcherDetachedClientIsReportedAndNotReplaced(t *testing.T) {
	chosen := flowClients()[0]
	backend := &navigationFlowBackend{targets: []core.NavigationTarget{dispatcherFlowTarget()}}
	navigator := &navigationFlowNavigator{clients: []terminal.Client{chosen}, jumpErr: &core.Error{Code: "client_gone", Message: "the selected tmux client detached or restarted"}}
	prefs := &memoryClientPreferences{values: map[string]terminal.Client{}}
	model := dispatcherFixture("running")
	model.backend, model.navigator, model.clientPreferences = backend, navigator, prefs
	model.navigationPending = true
	model.navigationSequence = 1
	model.navigationGeneration = model.generation
	model.navigationTarget = dispatcherFlowTarget()
	model.navigationRef = core.EntityRef{Kind: "dispatcher"}
	model.navigationClients = []terminal.Client{chosen}

	command := model.acceptNavigationClient("0")
	result := command().(navigationResultMsg)
	if result.err == nil {
		t.Fatal("detached dispatcher client was not reported")
	}
	model.Update(result)
	if len(navigator.jumps) != 1 || len(prefs.saves) != 0 || !strings.Contains(model.notice, "Press g") {
		t.Fatalf("detached dispatcher client was replaced or saved: jumps=%+v saves=%+v notice=%q", navigator.jumps, prefs.saves, model.notice)
	}
}
