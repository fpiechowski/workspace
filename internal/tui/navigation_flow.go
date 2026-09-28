package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"workspace/internal/core"
	"workspace/internal/terminal"
)

func (m *Model) jumpSelected() tea.Cmd {
	if m.route.Page == "orchestrator" {
		return m.jump(core.EntityRef{Kind: "orchestrator"})
	}
	if m.route.Page == "dispatcher" {
		if !m.dispatcherJumpReady() {
			return nil
		}
		return m.jump(core.EntityRef{Kind: "dispatcher"})
	}
	if jumpCapable(m.route.Page) && m.route.EntityID != "" {
		return m.jump(core.EntityRef{Kind: m.route.Page, ID: m.route.EntityID})
	}
	if !m.isCollectionPage() {
		return nil
	}
	item, _, items := m.selectedItem()
	if len(items) == 0 || !jumpCapable(item.Kind) {
		return nil
	}
	return m.jump(core.EntityRef{Kind: item.Kind, ID: item.ID})
}

func (m *Model) jump(ref core.EntityRef) tea.Cmd {
	return m.navigationAttempt(ref, false, nil)
}

func (m *Model) navigationAttempt(ref core.EntityRef, afterReconcile bool, client *terminal.Client) tea.Cmd {
	if m.navigationPending {
		return nil
	}
	if m.backend == nil || m.navigator == nil {
		m.notice = "Tmux navigation is unavailable for this TUI."
		m.rebuildViewport()
		return nil
	}
	m.navigationSequence++
	sequence := m.navigationSequence
	m.navigationPending = true
	m.navigationRef = ref
	m.navigationAfterReconcile = afterReconcile
	m.navigationClient = client
	m.navigationClients = nil
	m.navigationGeneration = m.generation
	m.notice = "Resolving the selected tmux target…"
	workspace := m.workspaceID
	if workspace == "" && ref.Kind == "workspace" {
		workspace = ref.ID
	}
	backend, generation := m.backend, m.generation
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		target, err := backend.ResolveNavigationTarget(ctx, workspace, ref)
		return navigationTargetMsg{
			generation: generation, sequence: sequence, ref: ref,
			afterReconcile: afterReconcile, client: cloneClient(client), target: target, err: err,
		}
	}
}

func cloneClient(client *terminal.Client) *terminal.Client {
	if client == nil {
		return nil
	}
	copy := *client
	return &copy
}

func (m *Model) handleNavigationTarget(msg navigationTargetMsg) (tea.Model, tea.Cmd) {
	if !m.navigationMessageCurrent(msg.generation, msg.sequence) {
		return m, nil
	}
	if msg.err != nil {
		m.endNavigationFlow()
		return m, m.navigationFailure(msg.err, msg.ref, msg.afterReconcile, msg.client)
	}
	if m.navigator == nil {
		m.endNavigationFlow()
		m.notice = "Tmux navigation is unavailable for this TUI."
		m.rebuildViewport()
		return m, nil
	}
	m.navigationTarget = msg.target
	m.navigationRef = msg.ref
	m.navigationAfterReconcile = msg.afterReconcile
	m.navigationClient = cloneClient(msg.client)
	m.navigationGeneration = msg.generation
	if msg.client != nil {
		return m, m.jumpClientCommand(msg.target, msg.ref, *msg.client, msg.afterReconcile, msg.generation, msg.sequence, false)
	}
	navigator, target, preferences, generation, sequence := m.navigator, msg.target, m.clientPreferences, msg.generation, msg.sequence
	m.notice = "Finding attached tmux clients…"
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		clients, err := navigator.ListClients(ctx, target)
		result := navigationClientsMsg{
			generation: generation, sequence: sequence, ref: msg.ref,
			afterReconcile: msg.afterReconcile, target: target, clients: clients, err: err,
		}
		if err == nil && len(clients) > 0 && preferences != nil {
			result.lastClient, result.hasLastClient, result.preferenceErr = preferences.LoadLastUsed(target.Socket)
		}
		return result
	}
}

func (m *Model) handleNavigationClients(msg navigationClientsMsg) (tea.Model, tea.Cmd) {
	if !m.navigationMessageCurrent(msg.generation, msg.sequence) {
		return m, nil
	}
	if msg.err != nil {
		m.endNavigationFlow()
		return m, m.navigationFailure(msg.err, msg.ref, msg.afterReconcile, nil)
	}
	if len(msg.clients) == 0 {
		m.endNavigationFlow()
		m.loadError = "no attached tmux clients are available on the selected socket"
		m.notice = "Jump unavailable: " + m.loadError
		m.rebuildViewport()
		return m, nil
	}
	if m.generation != m.navigationGeneration || msg.target != m.navigationTarget || msg.ref != m.navigationRef {
		m.endNavigationFlow()
		m.notice = "The selected workspace target changed before the client picker opened. Press g to try again."
		m.rebuildViewport()
		return m, nil
	}
	clients, marked := orderedClients(msg.clients, msg.lastClient, msg.hasLastClient && msg.preferenceErr == nil)
	options := make([]huh.Option[string], 0, len(clients))
	for index, client := range clients {
		label := clientLabel(client)
		if marked && index == 0 {
			label += " · last used"
		}
		options = append(options, huh.NewOption(sanitizeLine(label), strconv.Itoa(index)))
	}
	m.navigationClients = clients
	m.formClient = "0"
	m.formMode = "navigation_client"
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Key("client").Title("Choose a tmux client").Options(options...).Value(&m.formClient),
	)).WithWidth(m.dialogWidth()).WithHeight(m.formHeight(5)).WithTheme(huhTheme(m.palette))
	if msg.preferenceErr != nil {
		m.notice = "Could not read the last-used tmux client preference: " + sanitizeLine(msg.preferenceErr.Error()) + ". Using the default client order."
	} else {
		m.notice = "Choose the client to move to the verified workspace pane."
	}
	return m, m.form.Init()
}

func clientLabel(client terminal.Client) string {
	parts := []string{client.TTY}
	if client.Session != "" {
		parts = append(parts, "session "+client.Session)
	}
	if client.PID != "" {
		parts = append(parts, "PID "+client.PID)
	}
	if client.Name != "" {
		parts = append(parts, client.Name)
	}
	if client.TermName != "" {
		parts = append(parts, client.TermName)
	}
	return sanitizeLine(strings.Join(parts, " · "))
}

func (m *Model) acceptNavigationClient(value string) tea.Cmd {
	if m.navigationGeneration != m.generation {
		m.endNavigationFlow()
		m.notice = "The selected workspace changed while the client picker was open. Press g to choose again."
		m.rebuildViewport()
		return nil
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 0 || index >= len(m.navigationClients) {
		m.endNavigationFlow()
		m.notice = "The selected tmux client is no longer available in the picker. Press g to choose again."
		m.rebuildViewport()
		return nil
	}
	chosen := m.navigationClients[index]
	return m.jumpClientCommand(m.navigationTarget, m.navigationRef, chosen, m.navigationAfterReconcile, m.generation, m.navigationSequence, true)
}

func (m *Model) jumpClientCommand(target core.NavigationTarget, ref core.EntityRef, client terminal.Client, afterReconcile bool, generation, sequence uint64, recheckTarget bool) tea.Cmd {
	backend, navigator, preferences := m.backend, m.navigator, m.clientPreferences
	workspace := m.workspaceID
	if workspace == "" && ref.Kind == "workspace" {
		workspace = ref.ID
	}
	chosen := client
	m.navigationPending = true
	m.navigationClient = &chosen
	m.notice = "Verifying the selected client and jumping to the workspace pane…"
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		if recheckTarget {
			current, err := backend.ResolveNavigationTarget(ctx, workspace, ref)
			if err != nil {
				return navigationResultMsg{generation: generation, sequence: sequence, ref: ref, afterReconcile: afterReconcile, target: target, client: &chosen, err: err}
			}
			if current != target {
				err := &core.Error{Code: "navigation_target_changed", Message: "the selected tmux target changed while the picker was open; press g and choose again"}
				return navigationResultMsg{generation: generation, sequence: sequence, ref: ref, afterReconcile: afterReconcile, target: target, client: &chosen, err: err}
			}
		}
		if err := navigator.Jump(ctx, target, chosen); err != nil {
			return navigationResultMsg{generation: generation, sequence: sequence, ref: ref, afterReconcile: afterReconcile, target: target, client: &chosen, err: err}
		}
		var preferenceErr error
		if preferences != nil {
			preferenceErr = preferences.SaveLastUsed(target.Socket, chosen)
		}
		return navigationResultMsg{generation: generation, sequence: sequence, ref: ref, afterReconcile: afterReconcile, target: target, client: &chosen, preferenceErr: preferenceErr}
	}
}

func (m *Model) handleNavigationResult(msg navigationResultMsg) (tea.Model, tea.Cmd) {
	if !m.navigationMessageCurrent(msg.generation, msg.sequence) {
		return m, nil
	}
	m.endNavigationFlow()
	if msg.err != nil {
		return m, m.navigationFailure(msg.err, msg.ref, msg.afterReconcile, msg.client)
	}
	m.loadError = ""
	if msg.preferenceErr != nil {
		m.loadError = sanitizeLine(msg.preferenceErr.Error())
		m.notice = "Jump completed, but the last-used tmux client could not be saved: " + m.loadError
	} else {
		m.notice = "Jumped the selected tmux client to the verified workspace pane."
	}
	m.rebuildViewport()
	return m, m.beginRefresh()
}

func (m *Model) navigationMessageCurrent(generation, sequence uint64) bool {
	return m.navigationPending && generation == m.generation && sequence == m.navigationSequence
}

func (m *Model) endNavigationFlow() {
	m.navigationPending = false
	m.navigationSequence++
	m.navigationTarget = core.NavigationTarget{}
	m.navigationRef = core.EntityRef{}
	m.navigationClient = nil
	m.navigationAfterReconcile = false
	m.navigationClients = nil
	m.navigationGeneration = 0
}

func (m *Model) cancelNavigation() {
	m.endNavigationFlow()
	m.formClient = ""
}

func (m *Model) navigationFailure(err error, ref core.EntityRef, afterReconcile bool, client *terminal.Client) tea.Cmd {
	m.loadError = sanitizeLine(err.Error())
	var failure *core.Error
	_, canAct := m.backend.(ActionBackend)
	if !afterReconcile && canAct && m.workspaceID != "" && ref.Kind != "" && m.form == nil && !m.actionPending && err != nil &&
		errors.As(err, &failure) && failure.Code == "pane_missing" {
		m.formAction = ActionCall{
			Action: "reconcile", WorkspaceID: m.workspaceID, Key: core.ID("tui"),
			ExpectedRevision: m.snapshot.Status.Workspace.Revision,
			NavigationRef:    &ref,
			NavigationClient: cloneClient(client),
		}
		return m.openConfirm("reconcile")
	}
	if afterReconcile {
		if client != nil && (isNavigationError(err, "client_gone") || isNavigationError(err, "client_changed")) {
			m.notice = "Reconcile finished, but the chosen tmux client is gone or changed. Press g to choose a live client."
		} else {
			m.notice = "Reconcile finished, but this target is unavailable. Open Sessions (2) or the Orchestrator (o), then press g to choose a client."
		}
	} else if isNavigationError(err, "navigation_target_changed") {
		m.notice = m.loadError
	} else if isNavigationError(err, "client_gone") || isNavigationError(err, "client_changed") {
		m.notice = m.loadError + ". Press g to choose a live client."
	} else {
		m.notice = "Jump unavailable: " + m.loadError
	}
	m.rebuildViewport()
	return nil
}

func isNavigationError(err error, code string) bool {
	var failure *core.Error
	return errors.As(err, &failure) && failure.Code == code
}
