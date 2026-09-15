package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"workspace/internal/core"
)

func (m *Model) beginRefresh() tea.Cmd {
	if m.backend == nil {
		return nil
	}
	gen := m.generation
	if m.route.Page == "project" || m.workspaceID == "" {
		if m.projectPending {
			return nil
		}
		m.projectPending = true
		backend := m.backend
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			value, err := backend.ProjectOverview(ctx)
			return projectMsg{generation: gen, value: value, err: err}
		}
	}
	var cmds []tea.Cmd
	if !m.snapshotPending {
		m.snapshotPending = true
		backend, id := m.backend, m.workspaceID
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			value, err := backend.WorkspaceSnapshot(ctx, id)
			return snapshotMsg{generation: gen, value: value, err: err}
		})
	}
	if !m.runtimePending {
		m.runtimePending = true
		backend, id := m.backend, m.workspaceID
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			value, err := backend.ObserveWorkspaceRuntime(ctx, id)
			return runtimeMsg{generation: gen, value: value, err: err}
		})
	}
	if backend, ok := m.backend.(UIStatusBackend); ok && !m.uiPending {
		m.uiPending = true
		id, gen := m.workspaceID, gen
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			value, err := backend.UIPaneStatus(ctx, id)
			return uiStatusMsg{generation: gen, value: value, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (m *Model) scheduleRefresh() tea.Cmd {
	if m.closed || m.mutationPending || m.previewPending || m.worktreePending {
		return nil
	}
	delay := 5 * time.Second
	if m.workspaceID != "" && m.route.Page != "project" {
		delay = 2 * time.Second
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return refreshTimerMsg{} })
}

func (m *Model) inspectWorktree(id string) tea.Cmd {
	if m.backend == nil || id == "" {
		return nil
	}
	m.worktreePending = true
	backend, workspace, gen := m.backend, m.workspaceID, m.generation
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		value, err := backend.InspectWorktree(ctx, workspace, id)
		return worktreeMsg{generation: gen, id: id, value: value, err: err}
	}
}

func (m *Model) readPreview(kind core.PreviewKind, id string) tea.Cmd {
	if m.backend == nil || m.workspaceID == "" || id == "" {
		return nil
	}
	m.previewPending = true
	backend, workspace, gen := m.backend, m.workspaceID, m.generation
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		value, err := backend.ReadPreview(ctx, workspace, kind, id)
		return previewMsg{generation: gen, value: value, err: err}
	}
}

func (m *Model) rebuildViewport() {
	if m.width < 1 || m.height < 1 {
		return
	}
	m.viewport.Width = max(1, m.width-4)
	m.viewport.Height = max(1, m.contentHeight())
	content := m.detailContent()
	if content == "" {
		content = m.dashboardContent()
	}
	m.viewport.SetContent(content)
}

func (m *Model) contentHeight() int {
	return max(1, m.height-5)
}
