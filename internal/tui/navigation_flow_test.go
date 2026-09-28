package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"workspace/internal/core"
	"workspace/internal/terminal"
)

type navigationFlowBackend struct {
	Backend
	targets      []core.NavigationTarget
	resolveErrs  []error
	resolveCalls []core.EntityRef
	actionCalls  []ActionCall
}

func (b *navigationFlowBackend) ResolveNavigationTarget(_ context.Context, _ string, ref core.EntityRef) (core.NavigationTarget, error) {
	b.resolveCalls = append(b.resolveCalls, ref)
	index := len(b.resolveCalls) - 1
	if index < len(b.resolveErrs) && b.resolveErrs[index] != nil {
		return core.NavigationTarget{}, b.resolveErrs[index]
	}
	if len(b.targets) == 0 {
		return core.NavigationTarget{}, nil
	}
	if index >= len(b.targets) {
		index = len(b.targets) - 1
	}
	return b.targets[index], nil
}

func (b *navigationFlowBackend) PerformAction(_ context.Context, _ string, call ActionCall) error {
	b.actionCalls = append(b.actionCalls, call)
	return nil
}

func (b *navigationFlowBackend) WorkspaceSnapshot(context.Context, string) (core.WorkspaceSnapshot, error) {
	return core.WorkspaceSnapshot{}, nil
}

func (b *navigationFlowBackend) ObserveWorkspaceRuntime(context.Context, string) (core.RuntimeObservation, error) {
	return core.RuntimeObservation{}, nil
}

type navigationFlowNavigator struct {
	clients []terminal.Client
	listErr error
	jumpErr error
	listed  []core.NavigationTarget
	jumps   []selectedNavigation
}

type selectedNavigation struct {
	target core.NavigationTarget
	client terminal.Client
}

func (n *navigationFlowNavigator) ListClients(_ context.Context, target core.NavigationTarget) ([]terminal.Client, error) {
	n.listed = append(n.listed, target)
	return append([]terminal.Client(nil), n.clients...), n.listErr
}

func (n *navigationFlowNavigator) Jump(_ context.Context, target core.NavigationTarget, client terminal.Client) error {
	n.jumps = append(n.jumps, selectedNavigation{target: target, client: client})
	return n.jumpErr
}

type memoryClientPreferences struct {
	values  map[string]terminal.Client
	loads   int
	saves   []selectedPreference
	loadErr error
	saveErr error
}

type selectedPreference struct {
	socket string
	client terminal.Client
}

func (p *memoryClientPreferences) LoadLastUsed(socket string) (terminal.Client, bool, error) {
	p.loads++
	client, ok := p.values[socket]
	return client, ok, p.loadErr
}

func (p *memoryClientPreferences) SaveLastUsed(socket string, client terminal.Client) error {
	p.saves = append(p.saves, selectedPreference{socket: socket, client: client})
	return p.saveErr
}

func flowTarget() core.NavigationTarget {
	return core.NavigationTarget{
		WorkspaceID: "ws_demo", SessionName: "workspace-ws_demo", Socket: "private",
		WindowID: "@8", PaneID: "%9", Kind: "run", SessionID: "sess_worker", RunID: "run_worker",
	}
}

func flowClients() []terminal.Client {
	return []terminal.Client{
		{TTY: "/dev/pts/9", Session: "other", SessionID: "$3", WindowID: "@2", PaneID: "%3", PID: "200", Name: "terminal-b", Created: "20"},
		{TTY: "/dev/pts/2", Session: "workspace", SessionID: "$1", WindowID: "@1", PaneID: "%1", PID: "100", Name: "terminal-a", Created: "10"},
	}
}

func openNavigationPicker(t *testing.T, clients []terminal.Client, prefs ClientPreferenceStore) (*Model, *navigationFlowBackend, *navigationFlowNavigator, navigationTargetMsg) {
	t.Helper()
	target := flowTarget()
	backend := &navigationFlowBackend{targets: []core.NavigationTarget{target}}
	navigator := &navigationFlowNavigator{clients: clients}
	model := workFixture()
	model.backend, model.navigator, model.clientPreferences = backend, navigator, prefs
	model.navigate(route{Page: "sessions"})
	model.route.SelectedID = "sess_worker"
	command := model.jumpSelected()
	if command == nil {
		t.Fatal("g did not resolve the jump-capable selection")
	}
	resolved, ok := command().(navigationTargetMsg)
	if !ok {
		t.Fatalf("g returned %T, want a target-resolution message", command())
	}
	_, discover := model.handleNavigationTarget(resolved)
	if discover == nil {
		t.Fatal("target resolution did not start client discovery")
	}
	clientsMessage, ok := discover().(navigationClientsMsg)
	if !ok {
		t.Fatalf("client discovery returned %T", discover())
	}
	model.Update(clientsMessage)
	if model.form == nil || model.formMode != "navigation_client" {
		t.Fatalf("g did not open the client picker: form=%v mode=%q", model.form != nil, model.formMode)
	}
	return model, backend, navigator, resolved
}

func TestJumpAlwaysShowsClientPickerAndJumpsChosenClient(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(map[int]string{1: "one client", 2: "multiple clients"}[count], func(t *testing.T) {
			clients := flowClients()[:count]
			prefs := &memoryClientPreferences{values: map[string]terminal.Client{}}
			model, backend, navigator, _ := openNavigationPicker(t, clients, prefs)
			if prefs.loads != 1 {
				t.Fatalf("picker did not read the socket-scoped preference: %d", prefs.loads)
			}
			if len(navigator.listed) != 1 || navigator.listed[0] != flowTarget() {
				t.Fatalf("client discovery used a different target: %+v", navigator.listed)
			}
			if backend.resolveCalls[0] != (core.EntityRef{Kind: "session", ID: "sess_worker"}) {
				t.Fatalf("resolved wrong selection: %+v", backend.resolveCalls)
			}
			chosenIndex := count - 1
			if chosenIndex > 0 {
				_, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
			}
			if selected := model.formClient; selected != strings.TrimSpace(string(rune('0'+chosenIndex))) {
				t.Fatalf("picker selection = %q, want %d", selected, chosenIndex)
			}
			jumpCommand := pressFormKey(model, tea.KeyMsg{Type: tea.KeyEnter})
			if jumpCommand == nil || model.navigationClient == nil || *model.navigationClient != model.navigationClients[chosenIndex] {
				t.Fatalf("confirm did not retain the selected client: client=%+v command=%v", model.navigationClient, jumpCommand != nil)
			}
			result, ok := findNavigationResult(jumpCommand)
			if !ok || result.err != nil {
				t.Fatalf("jump result = %#v", result)
			}
			if len(navigator.jumps) != 1 || navigator.jumps[0].client != model.navigationClients[chosenIndex] || navigator.jumps[0].target != flowTarget() {
				t.Fatalf("jump did not use the exact chosen client and target: %+v", navigator.jumps)
			}
			if len(backend.resolveCalls) != 2 {
				t.Fatalf("selected target was not rechecked before switching: %+v", backend.resolveCalls)
			}
			model.Update(result)
			if len(prefs.saves) != 1 || prefs.saves[0].client != navigator.jumps[0].client || prefs.saves[0].socket != flowTarget().Socket {
				t.Fatalf("successful jump did not save the chosen client: %+v", prefs.saves)
			}
		})
	}
}

func findNavigationResult(command tea.Cmd) (navigationResultMsg, bool) {
	if command == nil {
		return navigationResultMsg{}, false
	}
	switch message := command().(type) {
	case navigationResultMsg:
		return message, true
	case tea.BatchMsg:
		for _, child := range message {
			if result, ok := findNavigationResult(child); ok {
				return result, true
			}
		}
	}
	return navigationResultMsg{}, false
}

// pressFormKey lets Huh's next-field and next-group messages pass through the
// TUI update loop, as Bubble Tea does between key events.
func pressFormKey(model *Model, key tea.KeyMsg) tea.Cmd {
	_, command := model.Update(key)
	for steps := 0; model.form != nil && command != nil && steps < 8; steps++ {
		message := command()
		if message == nil {
			return nil
		}
		_, command = model.Update(message)
	}
	return command
}

func TestLastUsedClientIsOrderedMarkedAndPreselected(t *testing.T) {
	clients := flowClients()
	last := clients[0]
	prefs := &memoryClientPreferences{values: map[string]terminal.Client{"private": last}}
	model, _, _, _ := openNavigationPicker(t, clients, prefs)
	if len(model.navigationClients) != 2 || model.navigationClients[0] != last || model.formClient != "0" {
		t.Fatalf("last-used client was not first and selected: clients=%+v choice=%q", model.navigationClients, model.formClient)
	}
	if !strings.Contains(model.form.View(), "last used") {
		t.Fatalf("last-used row marker is missing from the picker:\n%s", model.form.View())
	}
}

func TestDisconnectedLastClientHasNoMarkerAndUsesDeterministicDefault(t *testing.T) {
	clients := flowClients()
	last := terminal.Client{TTY: "/dev/pts/77", Session: "gone", PID: "77", Created: "77"}
	prefs := &memoryClientPreferences{values: map[string]terminal.Client{"private": last}}
	model, _, _, _ := openNavigationPicker(t, clients, prefs)
	if model.navigationClients[0].TTY != "/dev/pts/2" || model.formClient != "0" {
		t.Fatalf("deterministic live-client default changed: %+v, choice=%q", model.navigationClients, model.formClient)
	}
	if strings.Contains(model.form.View(), "last used") {
		t.Fatalf("stale last-used marker remained visible:\n%s", model.form.View())
	}
}

func TestPreferenceReadFailureIsVisibleAndUsesDefaultOrder(t *testing.T) {
	clients := flowClients()
	prefs := &memoryClientPreferences{
		values:  map[string]terminal.Client{"private": clients[0]},
		loadErr: errors.New("permission denied"),
	}
	model, _, _, _ := openNavigationPicker(t, clients, prefs)
	if model.navigationClients[0].TTY != "/dev/pts/2" || model.formClient != "0" {
		t.Fatalf("preference read failure did not use deterministic order: %+v, choice=%q", model.navigationClients, model.formClient)
	}
	if strings.Contains(model.form.View(), "last used") || !strings.Contains(model.notice, "permission denied") {
		t.Fatalf("preference read error or fallback was hidden: notice=%q form=%q", model.notice, model.form.View())
	}
}

func TestJumpCancellationAndFailureNeverSavePreference(t *testing.T) {
	for _, key := range []string{"esc", "ctrl+c"} {
		t.Run("cancel "+key, func(t *testing.T) {
			prefs := &memoryClientPreferences{values: map[string]terminal.Client{}}
			model, _, navigator, _ := openNavigationPicker(t, flowClients()[:1], prefs)
			msg := tea.KeyMsg{Type: tea.KeyEsc}
			if key == "ctrl+c" {
				msg = tea.KeyMsg{Type: tea.KeyCtrlC}
			}
			model.updateKey(msg)
			if model.navigationPending || model.form != nil || len(navigator.jumps) != 0 || len(prefs.saves) != 0 {
				t.Fatalf("cancel had tmux or preference effects: pending=%t jumps=%+v saves=%+v", model.navigationPending, navigator.jumps, prefs.saves)
			}
		})
	}
	t.Run("failed jump", func(t *testing.T) {
		prefs := &memoryClientPreferences{values: map[string]terminal.Client{}}
		model, _, navigator, _ := openNavigationPicker(t, flowClients()[:1], prefs)
		navigator.jumpErr = errors.New("tmux switch failed")
		model.form = nil
		model.formMode = ""
		command := model.acceptNavigationClient("0")
		result := command().(navigationResultMsg)
		model.Update(result)
		if result.err == nil || len(prefs.saves) != 0 {
			t.Fatalf("failed jump replaced last-used preference: err=%v saves=%+v", result.err, prefs.saves)
		}
	})
}

func TestPreferenceWriteFailureReportsSuccessfulJumpSeparately(t *testing.T) {
	prefs := &memoryClientPreferences{values: map[string]terminal.Client{}, saveErr: errors.New("disk full")}
	model, _, _, _ := openNavigationPicker(t, flowClients()[:1], prefs)
	model.form = nil
	model.formMode = ""
	result := model.acceptNavigationClient("0")().(navigationResultMsg)
	if result.err != nil || result.preferenceErr == nil {
		t.Fatalf("preference failure was confused with jump failure: %#v", result)
	}
	model.Update(result)
	if !strings.Contains(model.notice, "Jump completed") || !strings.Contains(model.notice, "disk full") {
		t.Fatalf("preference failure was not visible without misreporting the jump: %q", model.notice)
	}
}

func TestTargetChangeWhilePickerOpenRequiresFreshChoice(t *testing.T) {
	first := flowTarget()
	changed := first
	changed.PaneID = "%new"
	backend := &navigationFlowBackend{targets: []core.NavigationTarget{changed}}
	navigator := &navigationFlowNavigator{clients: flowClients()[:1]}
	prefs := &memoryClientPreferences{values: map[string]terminal.Client{}}
	model := workFixture()
	model.backend, model.navigator, model.clientPreferences = backend, navigator, prefs
	model.navigationPending = true
	model.navigationSequence = 3
	model.navigationGeneration = model.generation
	model.navigationTarget = first
	model.navigationRef = core.EntityRef{Kind: "run", ID: "run_worker"}
	model.navigationClients = navigator.clients
	command := model.acceptNavigationClient("0")
	result := command().(navigationResultMsg)
	if result.err == nil || !isNavigationError(result.err, "navigation_target_changed") || len(navigator.jumps) != 0 || len(prefs.saves) != 0 {
		t.Fatalf("changed target was jumped without a new choice: %#v", result)
	}
}

func TestNoClientsAndDiscoveryErrorsAreVisibleWithoutJump(t *testing.T) {
	for _, tc := range []struct {
		name    string
		clients []terminal.Client
		err     error
		want    string
	}{
		{name: "no attached clients", want: "no attached tmux clients"},
		{name: "discovery error", err: &core.Error{Code: "client_discovery_failed", Message: "permission denied"}, want: "permission denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := workFixture()
			navigator := &navigationFlowNavigator{clients: tc.clients}
			model.navigator = navigator
			model.navigationPending = true
			model.navigationSequence = 1
			model.navigationGeneration = model.generation
			model.navigationTarget = flowTarget()
			model.navigationRef = core.EntityRef{Kind: "run", ID: "run_worker"}
			model.Update(navigationClientsMsg{
				generation: model.generation, sequence: 1, ref: model.navigationRef,
				target: flowTarget(), clients: tc.clients, err: tc.err,
			})
			if model.navigationPending || model.form != nil || len(navigator.jumps) != 0 || !strings.Contains(model.notice, tc.want) {
				t.Fatalf("discovery outcome was not reported cleanly: pending=%t form=%v notice=%q jumps=%+v", model.navigationPending, model.form != nil, model.notice, navigator.jumps)
			}
		})
	}
}

func TestRemovedTerminalKeyHasNoNavigationEffect(t *testing.T) {
	model := workFixture()
	navigator := &navigationFlowNavigator{clients: flowClients()}
	model.navigator = navigator
	model.backend = &navigationFlowBackend{targets: []core.NavigationTarget{flowTarget()}}
	model.navigate(route{Page: "sessions"})
	model.route.SelectedID = "sess_worker"
	_, command := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if command != nil || model.navigationPending || model.form != nil || len(navigator.listed) != 0 || len(navigator.jumps) != 0 {
		t.Fatalf("removed t action still caused navigation: command=%v pending=%t form=%v listed=%+v jumps=%+v", command != nil, model.navigationPending, model.form != nil, navigator.listed, navigator.jumps)
	}
}

func TestTmuxClientLabelsAreSanitizedBeforePickerUse(t *testing.T) {
	label := clientLabel(terminal.Client{TTY: "/dev/pts/7\x1b]52;c;bad\a", Session: "workspace\nmalicious", PID: "42", Name: "xterm"})
	if strings.ContainsAny(label, "\x1b\n\a") || !strings.Contains(label, "/dev/pts/7") || !strings.Contains(label, "session workspace malicious") {
		t.Fatalf("unsafe or incomplete tmux client label: %q", label)
	}
}

func TestDetachedSelectedClientIsReportedAndNotReplaced(t *testing.T) {
	prefs := &memoryClientPreferences{values: map[string]terminal.Client{}}
	model, _, navigator, _ := openNavigationPicker(t, flowClients(), prefs)
	chosen := model.navigationClients[1]
	navigator.jumpErr = &core.Error{Code: "client_gone", Message: "the selected tmux client detached or restarted"}
	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	command := pressFormKey(model, tea.KeyMsg{Type: tea.KeyEnter})
	result, ok := findNavigationResult(command)
	if !ok || result.err == nil {
		t.Fatalf("detached client was not reported: %#v", result)
	}
	model.Update(result)
	if len(navigator.jumps) != 1 || navigator.jumps[0].client != chosen || len(prefs.saves) != 0 || !strings.Contains(model.notice, "Press g") {
		t.Fatalf("detached client was replaced or saved: jumps=%+v saves=%+v notice=%q", navigator.jumps, prefs.saves, model.notice)
	}
}

func TestStaleNavigationResponsesCannotOpenPickerOrFinishAnotherFlow(t *testing.T) {
	model := workFixture()
	model.navigationPending = true
	model.navigationSequence = 9
	model.generation = 2
	model.Update(navigationTargetMsg{generation: 1, sequence: 9, ref: core.EntityRef{Kind: "run"}, target: flowTarget()})
	model.Update(navigationClientsMsg{generation: 2, sequence: 8, clients: flowClients(), target: flowTarget()})
	if model.form != nil || !model.navigationPending || model.navigationSequence != 9 {
		t.Fatalf("stale response changed active navigation flow: form=%v pending=%t sequence=%d", model.form != nil, model.navigationPending, model.navigationSequence)
	}
}

func TestReconcileRetryKeepsChosenClientAndRunsOnlyOnce(t *testing.T) {
	backend := &navigationFlowBackend{targets: []core.NavigationTarget{flowTarget()}}
	navigator := &navigationFlowNavigator{}
	model := workFixture()
	model.backend, model.navigator = backend, navigator
	ref := core.EntityRef{Kind: "run", ID: "run_worker"}
	chosen := flowClients()[1]
	model.navigationFailure(&core.Error{Code: "pane_missing", Message: "pane disappeared"}, ref, false, &chosen)
	if model.form == nil || model.formAction.NavigationClient == nil || *model.formAction.NavigationClient != chosen {
		t.Fatal("reconcile confirmation did not retain the selected client")
	}
	call := model.formAction
	model.form = nil
	_, retryBatch := model.Update(actionResultMsg{generation: model.generation, call: call})
	var targetMsg *navigationTargetMsg
	for _, message := range collectTargetMessages(retryBatch) {
		targetMsg = &message
	}
	if targetMsg == nil || !targetMsg.afterReconcile || targetMsg.client == nil || *targetMsg.client != chosen {
		t.Fatalf("reconcile retry lost the exact client: %#v", targetMsg)
	}
	newTarget := flowTarget()
	newTarget.PaneID = "%new"
	_, jumpCommand := model.Update(navigationTargetMsg{
		generation: model.generation, sequence: targetMsg.sequence, ref: ref,
		afterReconcile: true, client: &chosen, target: newTarget,
	})
	result := jumpCommand().(navigationResultMsg)
	if result.err != nil || len(navigator.jumps) != 1 || navigator.jumps[0].client != chosen || navigator.jumps[0].target != newTarget || len(navigator.listed) != 0 {
		t.Fatalf("reconcile did not retry exact selection on exact client: result=%#v jumps=%+v listed=%+v", result, navigator.jumps, navigator.listed)
	}
	model.Update(navigationResultMsg{generation: model.generation, sequence: targetMsg.sequence, ref: ref, afterReconcile: true, client: &chosen, err: &core.Error{Code: "pane_missing", Message: "still missing"}})
	if model.form != nil || !strings.Contains(model.notice, "Reconcile finished") {
		t.Fatalf("second failure offered another reconcile prompt: form=%v notice=%q", model.form != nil, model.notice)
	}
}

func collectTargetMessages(command tea.Cmd) []navigationTargetMsg {
	if command == nil {
		return nil
	}
	switch message := command().(type) {
	case navigationTargetMsg:
		return []navigationTargetMsg{message}
	case tea.BatchMsg:
		var all []navigationTargetMsg
		for _, child := range message {
			all = append(all, collectTargetMessages(child)...)
		}
		return all
	default:
		return nil
	}
}

func TestFileClientPreferenceIsAtomicSocketScopedAndReadOnly(t *testing.T) {
	root := t.TempDir()
	store := FileClientPreferenceStore{ProjectRoot: root}
	if _, found, err := store.LoadLastUsed("private"); err != nil || found {
		t.Fatalf("missing preference load = found %t, err %v", found, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".workspace")); !os.IsNotExist(err) {
		t.Fatalf("read created project preference data: %v", err)
	}
	client := flowClients()[0]
	if err := store.SaveLastUsed("private", client); err != nil {
		t.Fatal(err)
	}
	restarted := FileClientPreferenceStore{ProjectRoot: root}
	got, found, err := restarted.LoadLastUsed("private")
	if err != nil || !found || got != client {
		t.Fatalf("preference did not persist across store instances: %+v, found=%t, err=%v", got, found, err)
	}
	if _, found, err := restarted.LoadLastUsed("other"); err != nil || found {
		t.Fatalf("socket-scoped preference leaked across sockets: found=%t err=%v", found, err)
	}
}
