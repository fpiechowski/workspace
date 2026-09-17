package tui

import (
	"context"
	"os"
	"time"

	keybind "github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"workspace/internal/core"
)

// Update processes one message and then rearms the animation loop if, and only
// if, pending work or a visible live run still needs it. An idle model never
// schedules another animation tick. While a modal form owns input the loop is
// left untouched so form commands are returned unwrapped.
func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(message)
	if m.form != nil {
		return model, cmd
	}
	if animation := m.ensureAnimation(); animation != nil {
		cmd = tea.Batch(cmd, animation)
	}
	return model, cmd
}

func (m *Model) update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case animationMsg:
		m.animating = false
		if m.quit || m.closed || !m.animationNeeded() {
			return m, nil
		}
		updated, _ := m.spinner.Update(m.spinner.Tick())
		m.spinner = updated
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
		m.filterInput.Width = max(8, min(40, m.width-10))
		m.rebuildViewport()
		return m, nil
	case projectMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.projectPending = false
		if msg.err != nil {
			m.loadError = sanitizeLine(msg.err.Error())
			m.lastFailure = time.Now()
		} else {
			m.project = msg.value
			m.loadError = ""
			m.lastSuccess = time.Now()
			m.completeAction()
			m.validateSelection()
		}
		m.rebuildViewport()
		return m, m.scheduleRefresh()
	case snapshotMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.snapshotPending = false
		if msg.err != nil {
			m.loadError = sanitizeLine(msg.err.Error())
			m.lastFailure = time.Now()
		} else {
			m.snapshot = msg.value
			m.loadError = ""
			m.lastSuccess = time.Now()
			m.completeAction()
			m.validateSelection()
		}
		m.rebuildViewport()
		return m, m.scheduleIfIdle()
	case runtimeMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.runtimePending = false
		if msg.err != nil {
			m.runtimeError = sanitizeLine(msg.err.Error())
		} else {
			m.runtime = msg.value
			m.runtimeError = sanitizeLine(msg.value.Error)
		}
		m.rebuildViewport()
		return m, m.scheduleIfIdle()
	case uiStatusMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.uiPending = false
		if msg.err != nil {
			m.uiError = sanitizeLine(msg.err.Error())
		} else {
			m.uiStatus = msg.value
			m.uiError = ""
		}
		m.rebuildViewport()
		return m, m.scheduleIfIdle()
	case workflowNamesMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.actionPending = false
		if msg.err != nil {
			m.loadError = sanitizeLine(msg.err.Error())
			m.notice = "Could not load workflows. Press a to try again."
			m.rebuildViewport()
			return m, nil
		}
		m.loadError = ""
		if msg.action == "create_workspace" {
			return m, m.openCreateWorkspaceForm(msg.names)
		}
		if len(msg.names) == 0 {
			m.notice = "No workflows are configured for this project."
			m.rebuildViewport()
			return m, nil
		}
		m.formWorkflow = msg.names[0]
		options := make([]huh.Option[string], 0, len(msg.names))
		for _, name := range msg.names {
			options = append(options, huh.NewOption(sanitizeLine(name), name))
		}
		m.formMode = "workflow"
		m.form = huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Key("workflow").Title("Choose a workflow").Options(options...).Value(&m.formWorkflow),
		)).WithWidth(m.dialogWidth()).WithHeight(m.formHeight(4)).WithTheme(huhTheme(m.palette))
		return m, m.form.Init()
	case worktreeMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.worktreePending = false
		if msg.err != nil {
			m.worktreeInspection[msg.id] = core.WorktreeObservation{WorkspaceID: m.workspaceID, WorktreeID: msg.id, Error: sanitizeLine(msg.err.Error()), CheckedAt: time.Now()}
		} else {
			m.worktreeInspection[msg.id] = msg.value
		}
		m.rebuildViewport()
		return m, m.scheduleIfIdle()
	case previewMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.previewPending = false
		if msg.err != nil {
			m.loadError = sanitizeLine(msg.err.Error())
		} else {
			m.preview = msg.value
			m.loadError = ""
		}
		m.rebuildViewport()
		return m, m.scheduleIfIdle()
	case navigationTargetMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.navigationPending = false
		if msg.err != nil {
			return m, m.navigationFailure(msg.err, msg.ref, msg.afterReconcile)
		}
		if m.navigator == nil {
			m.loadError = "terminal navigation is unavailable"
			m.rebuildViewport()
			return m, nil
		}
		if os.Getenv("TMUX") != "" {
			navigator, target, gen := m.navigator, msg.target, m.generation
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				return navigationResultMsg{generation: gen, ref: msg.ref, afterReconcile: msg.afterReconcile, err: navigator.Select(ctx, target)}
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cmd, err := m.navigator.PrepareAttach(ctx, msg.target)
		cancel()
		if err != nil {
			return m, m.navigationFailure(err, msg.ref, msg.afterReconcile)
		}
		// Do not use tea.ExecProcess here. Bubble Tea v1.3.10 can restart its
		// renderer before the old renderer goroutine stops, leaving animation
		// updates in the model but no longer flushing frames (upstream #1778).
		m.externalProcess = &ExternalProcessRequest{
			Cmd: cmd, Ref: msg.ref, AfterReconcile: msg.afterReconcile, Generation: m.generation,
		}
		m.quit = true
		return m, tea.Quit
	case navigationResultMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		if msg.err != nil {
			return m, m.navigationFailure(msg.err, msg.ref, msg.afterReconcile)
		}
		m.loadError = ""
		return m, m.beginRefresh()
	case actionResultMsg:
		return m, m.finishAction(msg)
	case hideResultMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.hidePending = false
		if msg.err != nil {
			m.loadError = sanitizeLine(msg.err.Error())
			m.notice = "Could not save the hide request. The interface is still open; press q to retry."
			m.rebuildViewport()
			return m, nil
		}
		m.loadError = ""
		m.quit = true
		return m, tea.Quit
	case refreshTimerMsg:
		if m.snapshotPending || m.runtimePending || m.uiPending || m.projectPending {
			return m, m.scheduleRefresh()
		}
		return m, m.beginRefresh()
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	if m.form != nil {
		return m.updateForm(message)
	}
	if m.isDetailPage() {
		updated, cmd := m.viewport.Update(message)
		m.viewport = updated
		return m, cmd
	}
	return m, nil
}

func (m *Model) scheduleIfIdle() tea.Cmd {
	if m.projectPending || m.snapshotPending || m.runtimePending || m.uiPending || m.worktreePending || m.previewPending {
		return nil
	}
	return m.scheduleRefresh()
}

func (m *Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.hidePending {
		return m, nil
	}
	if m.form != nil {
		if msg.Type == tea.KeyEsc || key == "ctrl+c" {
			m.form = nil
			m.formMode = ""
			m.formConfirm = false
			m.notice = "Action cancelled."
			m.rebuildViewport()
			return m, nil
		}
		return m.updateForm(msg)
	}
	if m.actionPending {
		m.notice = "An action is still in progress. Wait for its result before leaving this workspace."
		return m, nil
	}
	if m.managed && key == "ctrl+c" {
		m.filtering = false
		m.filterInput.Blur()
		return m, m.hideManaged()
	}
	if m.filtering {
		switch key {
		case "esc":
			m.cancelFilter()
			return m, nil
		case "enter":
			m.filtering = false
			m.filterInput.Blur()
			m.route.Query = m.filterInput.Value()
			m.route.SelectedID = ""
			m.rebuildViewport()
			return m, nil
		case "ctrl+c":
			m.cancelFilter()
			return m, nil
		}
		updated, cmd := m.filterInput.Update(msg)
		m.filterInput = updated
		m.route.Query = m.filterInput.Value()
		m.route.SelectedID = ""
		m.rebuildViewport()
		return m, cmd
	}
	if key == "y" && m.actionFailure && m.lastAction != nil {
		return m, m.runAction(*m.lastAction)
	}
	if key == "esc" && m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if key == "ctrl+c" || keybind.Matches(msg, m.keys.Exit) {
		if m.managed {
			return m, m.hideManaged()
		}
		m.quit = true
		return m, tea.Quit
	}
	if key == "esc" {
		if m.route.Query != "" {
			m.route.Query, m.route.SelectedID = "", ""
			m.filterInput.SetValue("")
			m.rebuildViewport()
			return m, nil
		}
		if m.route.StatusFilter != "" {
			m.route.StatusFilter = ""
			m.validateSelection()
			m.rebuildViewport()
			return m, nil
		}
		m.pop()
		return m, nil
	}
	if keybind.Matches(msg, m.keys.Help) {
		m.showHelp = !m.showHelp
		if m.showHelp {
			m.helpViewport.GotoTop()
		}
		return m, nil
	}
	if m.showHelp {
		// Scroll keys are owned by the help viewport; any other command
		// dismisses help and keeps working as documented.
		switch {
		case keybind.Matches(msg, m.keys.Up):
			m.helpViewport.LineUp(1)
			return m, nil
		case keybind.Matches(msg, m.keys.Down):
			m.helpViewport.LineDown(1)
			return m, nil
		case keybind.Matches(msg, m.keys.PageUp):
			m.helpViewport.PageUp()
			return m, nil
		case keybind.Matches(msg, m.keys.PageDown):
			m.helpViewport.PageDown()
			return m, nil
		case keybind.Matches(msg, m.keys.Top):
			m.helpViewport.GotoTop()
			return m, nil
		case keybind.Matches(msg, m.keys.Bottom):
			m.helpViewport.GotoBottom()
			return m, nil
		}
		m.showHelp = false
	}
	if keybind.Matches(msg, m.keys.Filter) && m.isCollectionPage() {
		m.filtering = true
		m.filterOriginal, m.filterSelection = m.route.Query, m.route.SelectedID
		m.filterInput.SetValue(m.route.Query)
		cmd := m.filterInput.Focus()
		return m, cmd
	}
	if keybind.Matches(msg, m.keys.Refresh) {
		m.generation++
		m.projectPending, m.snapshotPending, m.runtimePending, m.uiPending = false, false, false, false
		m.loadError, m.runtimeError, m.uiError = "", "", ""
		return m, m.beginRefresh()
	}
	if keybind.Matches(msg, m.keys.Sort) && m.isCollectionPage() {
		switch m.route.Sort {
		case "":
			m.route.Sort = "name"
		case "name":
			m.route.Sort = "recent"
		default:
			m.route.Sort = ""
		}
		m.rebuildViewport()
		return m, nil
	}
	if keybind.Matches(msg, m.keys.Terminal) {
		return m, m.openTerminal()
	}
	if keybind.Matches(msg, m.keys.WorkspacePicker) {
		m.push(route{Page: "project"})
		return m, m.beginRefresh()
	}
	if keybind.Matches(msg, m.keys.Orchestrator) && m.workspaceID != "" {
		m.push(route{Page: "orchestrator"})
		return m, nil
	}
	if keybind.Matches(msg, m.keys.FilterNext) && m.isCollectionPage() {
		options := m.statusFilterOptions()
		if len(options) <= 1 {
			m.notice = "No status or history filter is available for this collection."
			m.rebuildViewport()
			return m, nil
		}
		index := 0
		for i, option := range options {
			if option == m.route.StatusFilter {
				index = i
				break
			}
		}
		m.route.StatusFilter = options[(index+1)%len(options)]
		m.route.SelectedID = ""
		m.notice = ""
		m.validateSelection()
		m.rebuildViewport()
		return m, nil
	}
	if keybind.Matches(msg, m.keys.Focus) || keybind.Matches(msg, m.keys.FocusPrev) {
		delta := 1
		if keybind.Matches(msg, m.keys.FocusPrev) {
			delta = -1
		}
		switch m.route.Page {
		case "results":
			tabs := []string{"artifacts", "handoffs", "checks"}
			index := 0
			for i, tab := range tabs {
				if tab == m.route.Tab {
					index = i
				}
			}
			index = (index + delta + len(tabs)) % len(tabs)
			m.navigate(route{Page: "results", Tab: tabs[index]})
			return m, nil
		}
	}
	if m.route.Page == "task" && (key == "1" || key == "2" || key == "3") {
		switch key {
		case "1":
			m.push(route{Page: "sessions", ParentID: m.route.EntityID})
		case "2":
			m.push(route{Page: "worktrees", ParentID: m.route.EntityID})
		case "3":
			m.push(route{Page: "results", ParentID: m.route.EntityID, Tab: "handoffs"})
		}
		return m, nil
	}
	switch {
	case keybind.Matches(msg, m.keys.Primary[0]):
		m.navigate(route{Page: "tasks"})
	case keybind.Matches(msg, m.keys.Primary[1]):
		m.navigate(route{Page: "sessions"})
	case keybind.Matches(msg, m.keys.Primary[2]):
		m.navigate(route{Page: "worktrees"})
	case keybind.Matches(msg, m.keys.Primary[3]):
		m.navigate(route{Page: "results", Tab: "artifacts"})
	case keybind.Matches(msg, m.keys.Primary[4]):
		m.navigate(route{Page: "more"})
	case keybind.Matches(msg, m.keys.Up):
		if m.isCollectionPage() {
			m.moveSelection(-1)
		} else {
			m.viewport.LineUp(1)
		}
	case keybind.Matches(msg, m.keys.Down):
		if m.isCollectionPage() {
			m.moveSelection(1)
		} else {
			m.viewport.LineDown(1)
		}
	case keybind.Matches(msg, m.keys.PageUp):
		if m.isCollectionPage() {
			m.moveSelection(-max(1, m.contentHeight()-2))
		} else {
			m.viewport.PageUp()
		}
	case keybind.Matches(msg, m.keys.PageDown):
		if m.isCollectionPage() {
			m.moveSelection(max(1, m.contentHeight()-2))
		} else {
			m.viewport.PageDown()
		}
	case keybind.Matches(msg, m.keys.Top):
		if m.isCollectionPage() {
			items := m.filteredItems()
			if len(items) > 0 {
				m.route.SelectedID = items[0].ID
			}
		} else {
			m.viewport.GotoTop()
		}
	case keybind.Matches(msg, m.keys.Bottom):
		if m.isCollectionPage() {
			items := m.filteredItems()
			if len(items) > 0 {
				m.route.SelectedID = items[len(items)-1].ID
			}
		} else {
			m.viewport.GotoBottom()
		}
	case keybind.Matches(msg, m.keys.Open):
		if m.route.Page == "session" {
			m.push(route{Page: "runs", ParentID: m.route.EntityID})
			return m, nil
		}
		if m.route.Page == "artifact" {
			m.push(route{Page: "preview", EntityID: m.route.EntityID, ParentID: "artifact"})
			return m, m.readPreview(core.PreviewArtifact, m.route.EntityID)
		}
		if m.route.Page == "check" {
			m.push(route{Page: "preview", EntityID: m.route.EntityID, ParentID: "check"})
			return m, m.readPreview(core.PreviewCheck, m.route.EntityID)
		}
		if m.route.Page == "agent" {
			m.push(route{Page: "sessions", ParentID: m.route.EntityID})
			return m, nil
		}
		return m.openSelection()
	case keybind.Matches(msg, m.keys.Jump):
		return m, m.jumpSelected()
	case keybind.Matches(msg, m.keys.Actions):
		return m, m.openActionMenu()
	case keybind.Matches(msg, m.keys.Back):
		m.pop()
	}
	m.rebuildViewport()
	return m, nil
}

func (m *Model) cancelFilter() {
	m.filtering = false
	m.filterInput.Blur()
	m.filterInput.SetValue(m.filterOriginal)
	m.route.Query, m.route.SelectedID = m.filterOriginal, m.filterSelection
	m.rebuildViewport()
}

func (m *Model) hideManaged() tea.Cmd {
	hider, ok := m.backend.(ManagedUIHider)
	if !ok {
		m.loadError = "managed interface hide is unavailable"
		m.notice = "The hide request could not be saved; the interface remains open."
		m.rebuildViewport()
		return nil
	}
	if m.workspaceID == "" {
		m.loadError = "managed interface has no workspace"
		m.rebuildViewport()
		return nil
	}
	m.hidePending = true
	m.notice = "Saving hide request…"
	m.loadError = ""
	m.rebuildViewport()
	hider, workspace, key, gen := hider, m.workspaceID, m.hideKey, m.generation
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return hideResultMsg{generation: gen, err: hider.HideManagedUI(ctx, workspace, key)}
	}
}

func (m *Model) moveSelection(delta int) {
	_, index, items := m.selectedItem()
	if len(items) == 0 {
		return
	}
	index = (index + delta) % len(items)
	if index < 0 {
		index += len(items)
	}
	m.route.SelectedID = items[index].ID
}

func (m *Model) validateSelection() {
	if m.isCollectionPage() {
		items := m.filteredItems()
		found := false
		for _, item := range items {
			if item.ID == m.route.SelectedID {
				found = true
				break
			}
		}
		if !found && len(items) > 0 {
			m.route.SelectedID = items[0].ID
		} else if len(items) == 0 {
			m.route.SelectedID = ""
		}
	}
}

func (m *Model) isCollectionPage() bool {
	switch m.route.Page {
	case "project", "tasks", "worktrees", "results", "more", "sessions", "runs", "agents", "services", "decisions", "change_requests", "attention", "activity", "documents":
		return true
	default:
		return false
	}
}

func (m *Model) isDetailPage() bool {
	switch m.route.Page {
	case "task", "worktree", "session", "run", "agent", "service", "artifact", "handoff", "check", "decision", "change_request", "preview", "orchestrator", "runtime", "error":
		return true
	default:
		return false
	}
}

func (m *Model) openSelection() (tea.Model, tea.Cmd) {
	item, _, items := m.selectedItem()
	if len(items) == 0 {
		return m, nil
	}
	if item.State == "error" && item.Kind == "workspace" {
		m.loadError = sanitizeLine("This workspace could not be read. " + item.Subtitle)
		return m, nil
	}
	if item.Kind == "workspace" {
		return m, m.setWorkspace(item.ID)
	}
	if item.Kind == "page" {
		m.push(route{Page: item.ID})
		return m, nil
	}
	page := item.Kind
	if page == "document" {
		m.push(route{Page: "preview", EntityID: item.ID})
		return m, m.readPreview(core.PreviewDocument, item.ID)
	}
	m.push(route{Page: page, EntityID: item.ID})
	if page == "worktree" {
		return m, m.inspectWorktree(item.ID)
	}
	if page == "artifact" {
		return m, nil
	}
	if page == "check" {
		return m, nil
	}
	if page == "session" {
		return m, nil
	}
	return m, nil
}

func (m *Model) jumpSelected() tea.Cmd {
	item, _, items := m.selectedItem()
	if m.route.Page == "orchestrator" {
		return m.jump(core.EntityRef{Kind: "orchestrator"})
	}
	if m.route.Page == "worktree" || m.route.Page == "session" || m.route.Page == "run" || m.route.Page == "service" {
		return m.jump(core.EntityRef{Kind: m.route.Page, ID: m.route.EntityID})
	}
	if len(items) == 0 {
		return nil
	}
	if item.Kind == "run" {
		return m.jump(core.EntityRef{Kind: "run", ID: item.ID})
	}
	if item.Kind == "workspace" || item.Kind == "worktree" || item.Kind == "session" || item.Kind == "service" {
		return m.jump(core.EntityRef{Kind: item.Kind, ID: item.ID})
	}
	return m.jump(core.EntityRef{Kind: item.Kind, ID: item.ID})
}

func (m *Model) jump(ref core.EntityRef) tea.Cmd {
	return m.jumpAttempt(ref, false)
}

func (m *Model) jumpAttempt(ref core.EntityRef, afterReconcile bool) tea.Cmd {
	if m.backend == nil || m.navigator == nil || m.navigationPending {
		return nil
	}
	m.navigationPending = true
	workspace := m.workspaceID
	if workspace == "" && ref.Kind == "workspace" {
		workspace = ref.ID
	}
	backend, gen := m.backend, m.generation
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		target, err := backend.ResolveNavigationTarget(ctx, workspace, ref)
		return navigationTargetMsg{generation: gen, ref: ref, afterReconcile: afterReconcile, target: target, err: err}
	}
}
