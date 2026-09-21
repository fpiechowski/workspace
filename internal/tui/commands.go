package tui

import (
	"context"
	"fmt"
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
		var cmds []tea.Cmd
		if !m.projectPending {
			m.projectPending = true
			backend := m.backend
			cmds = append(cmds, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				value, err := backend.ProjectOverview(ctx)
				return projectMsg{generation: gen, value: value, err: err}
			})
		}
		if !m.supervisorPending {
			m.supervisorPending = true
			backend := m.backend
			cmds = append(cmds, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				value, err := backend.ObserveSupervisor(ctx)
				return supervisorMsg{generation: gen, value: value, err: err}
			})
		}
		return tea.Batch(cmds...)
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
	m.viewport.SetContent(m.detailContent())
}

func (m *Model) contentHeight() int {
	// Header, primary navigation, status/notice row, and key legend are always
	// reserved. Detail and parented routes add one breadcrumb row.
	rows := m.height - 4
	if m.breadcrumb() != "" {
		rows--
	}
	return max(1, rows)
}

// detailWidth is the wrapping width for detail and preview documents. It
// follows the content viewport so wrapped text never overflows it.
func (m *Model) detailWidth() int {
	if m.viewport.Width > 0 {
		return m.viewport.Width
	}
	return max(1, m.width-4)
}

// scrollPosition reports the visible window of an overflowing viewport. It
// returns "" when all content fits, so the indicator only appears on overflow.
func (m *Model) scrollPosition() string {
	total := m.viewport.TotalLineCount()
	visible := m.viewport.VisibleLineCount()
	if total <= visible {
		return ""
	}
	top := m.viewport.YOffset
	bottom := min(total, top+visible)
	return fmt.Sprintf("line %d–%d of %d", top+1, bottom, total)
}
