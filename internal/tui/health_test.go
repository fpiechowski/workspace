package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"workspace/internal/core"
)

func TestAggregateServiceHealthUsesLatestRecordPerName(t *testing.T) {
	cases := []struct {
		name           string
		services       []core.BackgroundService
		active, failed int
		label          string
	}{
		{name: "none", label: "services idle"},
		{
			name: "active",
			services: []core.BackgroundService{
				{Name: "api", State: "starting"},
				{Name: "worker", State: "running"},
			},
			active: 2, label: "services 2 active",
		},
		{
			name:     "failed",
			services: []core.BackgroundService{{Name: "api", State: "failed"}},
			failed:   1, label: "services 1 failed",
		},
		{
			name: "mixed",
			services: []core.BackgroundService{
				{Name: "api", State: "failed"},
				{Name: "worker", State: "running"},
			},
			active: 1, failed: 1, label: "services 1 failed · 1 active",
		},
		{
			name: "stopped and exited are idle",
			services: []core.BackgroundService{
				{Name: "api", State: "stopped"},
				{Name: "worker", State: "exited"},
			},
			label: "services idle",
		},
		{
			name: "restart supersedes failure",
			services: []core.BackgroundService{
				{Name: "api", State: "failed"},
				{Name: "api", State: "running"},
			},
			active: 1, label: "services 1 active",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := aggregateServiceHealth(tc.services)
			if got.active != tc.active || got.failed != tc.failed {
				t.Fatalf("health = %+v, want active=%d failed=%d", got, tc.active, tc.failed)
			}
			if label := got.indicator().label; label != tc.label {
				t.Fatalf("label = %q, want %q", label, tc.label)
			}
		})
	}
}

func TestHeaderShowsSupervisorAndServiceHealthInColorIndependentText(t *testing.T) {
	states := []string{"running", "stopped", "conflict", "unavailable"}
	for _, state := range states {
		t.Run(state, func(t *testing.T) {
			m := New(Config{ProjectFound: true, WorkspaceID: "ws_health", NoColor: true})
			m.width = 100
			m.route = route{Page: "tasks"}
			m.snapshot = core.WorkspaceSnapshot{
				ObservedAt: time.Now(),
				Status:     core.Status{Workspace: core.Workspace{ID: "ws_health", Title: "Health workspace", Status: "active"}},
				Services: []core.BackgroundService{
					{Name: "api", State: "failed"},
					{Name: "worker", State: "running"},
				},
			}
			m.supervisor = core.SupervisorObservation{State: state, ObservedAt: time.Now()}
			header := m.header()
			if !strings.Contains(header, "server "+state) {
				t.Fatalf("header omitted supervisor state %q: %q", state, header)
			}
			if !strings.Contains(header, "services 1 failed") || !strings.Contains(header, "1 active") {
				t.Fatalf("header omitted service counts: %q", header)
			}
			if !strings.Contains(header, "●") {
				t.Fatalf("header omitted the text health marker: %q", header)
			}
		})
	}
}

func TestProjectHeaderShowsSupervisorHealthAndCheckingDoesNotFlashRed(t *testing.T) {
	m := New(Config{ProjectFound: true, NoColor: true})
	m.route = route{Page: "project"}
	m.width = 80
	m.supervisorPending = true
	if header := m.header(); !strings.Contains(header, "○ server checking") || strings.Contains(header, "● server") {
		t.Fatalf("pending project header was not neutral checking: %q", header)
	}
	m.supervisorPending = false
	if header := m.header(); !strings.Contains(header, "○ server unknown") {
		t.Fatalf("unobserved project header was not explicit unknown: %q", header)
	}
	m.supervisor = core.SupervisorObservation{State: "running", ObservedAt: time.Now()}
	if header := m.header(); !strings.Contains(header, "● server running") {
		t.Fatalf("project header omitted running server: %q", header)
	}
}

func TestHealthHeaderFitsMinimumWidthWithEffectiveServiceCounts(t *testing.T) {
	for _, state := range []string{"running", "stopped", "conflict", "unavailable"} {
		m := New(Config{ProjectFound: true, WorkspaceID: "ws_health", NoColor: true})
		m.width, m.height = 40, 12
		m.route = route{Page: "tasks"}
		m.snapshot = core.WorkspaceSnapshot{
			ObservedAt: time.Now(),
			Status:     core.Status{Workspace: core.Workspace{ID: "ws_health", Title: "Health workspace", Status: "active"}},
			Services: []core.BackgroundService{
				{Name: "api", State: "failed"},
				{Name: "worker", State: "running"},
			},
		}
		m.supervisor = core.SupervisorObservation{State: state, ObservedAt: time.Now()}
		view := m.View()
		for lineNo, line := range strings.Split(view, "\n") {
			if width := ansi.StringWidth(line); width > m.width {
				t.Fatalf("%s header frame line %d is %d columns: %q", state, lineNo, width, line)
			}
		}
		if !strings.Contains(view, "server "+state) || !strings.Contains(view, "services 1 failed") || !strings.Contains(view, "1 active") {
			t.Fatalf("minimum frame hid health text for %s:\n%s", state, view)
		}
	}
}

func TestRefreshIndicatorFitsWithHealthAtSupportedSizes(t *testing.T) {
	modes := []struct {
		name    string
		theme   string
		noColor bool
	}{
		{name: "dark", theme: "dark"},
		{name: "light", theme: "light"},
		{name: "no-color", theme: "dark", noColor: true},
	}
	sizes := [][2]int{{40, 12}, {60, 24}, {100, 24}, {120, 32}}
	for _, mode := range modes {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s-workspace-%dx%d", mode.name, size[0], size[1]), func(t *testing.T) {
				m := shellFixture(mode.theme, mode.noColor)
				m.width, m.height = size[0], size[1]
				m.snapshotPending, m.runtimePending = true, true
				frame := m.spinnerGlyph()
				view := m.View()
				if header := strings.Split(view, "\n")[0]; !strings.Contains(header, frame) {
					t.Fatalf("workspace header omitted refresh frame %q: %q", frame, header)
				}
				for lineNo, line := range strings.Split(view, "\n") {
					if width := ansi.StringWidth(line); width > size[0] {
						t.Fatalf("workspace %dx%d line %d rendered %d columns: %q", size[0], size[1], lineNo, width, line)
					}
				}
			})

			t.Run(fmt.Sprintf("%s-project-%dx%d", mode.name, size[0], size[1]), func(t *testing.T) {
				m := New(Config{ProjectFound: true, Theme: mode.theme, NoColor: mode.noColor})
				m.route = route{Page: "project"}
				m.width, m.height = size[0], size[1]
				m.projectPending, m.supervisorPending = true, true
				frame := m.spinnerGlyph()
				view := m.View()
				if header := strings.Split(view, "\n")[0]; !strings.Contains(header, frame) {
					t.Fatalf("project header omitted refresh frame %q: %q", frame, header)
				}
				for lineNo, line := range strings.Split(view, "\n") {
					if width := ansi.StringWidth(line); width > size[0] {
						t.Fatalf("project %dx%d line %d rendered %d columns: %q", size[0], size[1], lineNo, width, line)
					}
				}
			})
		}
	}
}

func TestRefreshKeepsNarrowServiceOverflowReadable(t *testing.T) {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_health", NoColor: true})
	m.width, m.height = 40, 12
	m.route = route{Page: "tasks"}
	m.snapshot = core.WorkspaceSnapshot{
		ObservedAt: time.Now(),
		Status:     core.Status{Workspace: core.Workspace{ID: "ws_health", Title: "Health workspace", Status: "active"}},
		Services: []core.BackgroundService{
			{Name: "api", State: "failed"},
			{Name: "worker", State: "running"},
		},
	}
	m.supervisor = core.SupervisorObservation{State: "running", ObservedAt: time.Now()}
	m.snapshotPending, m.runtimePending = true, true
	frame := m.spinnerGlyph()
	view := m.View()
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[0], frame) {
		t.Fatalf("narrow header omitted refresh frame %q: %q", frame, lines[0])
	}
	if !strings.Contains(lines[len(lines)-2], "services 1 failed") || strings.Contains(lines[len(lines)-2], frame) {
		t.Fatalf("service overflow was masked by read progress: %q", lines[len(lines)-2])
	}
}

type healthBackend struct {
	Backend
	projectCalls, supervisorCalls, snapshotCalls, runtimeCalls int
	projectErr, supervisorErr, snapshotErr, runtimeErr         error
	project                                                    core.ProjectOverview
	supervisor                                                 core.SupervisorObservation
	snapshot                                                   core.WorkspaceSnapshot
	runtime                                                    core.RuntimeObservation
}

func (b *healthBackend) ProjectOverview(context.Context) (core.ProjectOverview, error) {
	b.projectCalls++
	return b.project, b.projectErr
}

func (b *healthBackend) ObserveSupervisor(context.Context) (core.SupervisorObservation, error) {
	b.supervisorCalls++
	return b.supervisor, b.supervisorErr
}

func (b *healthBackend) WorkspaceSnapshot(context.Context, string) (core.WorkspaceSnapshot, error) {
	b.snapshotCalls++
	return b.snapshot, b.snapshotErr
}

func (b *healthBackend) ObserveWorkspaceRuntime(context.Context, string) (core.RuntimeObservation, error) {
	b.runtimeCalls++
	return b.runtime, b.runtimeErr
}

func TestProjectAndWorkspaceRefreshKeepSupervisorReadIndependent(t *testing.T) {
	backend := &healthBackend{}
	m := New(Config{ProjectFound: true, Backend: backend})
	m.route = route{Page: "project"}
	if cmd := m.beginRefresh(); cmd == nil {
		t.Fatal("project refresh did not return commands")
	}
	if !m.projectPending || !m.supervisorPending {
		t.Fatalf("project refresh pending flags = project=%t supervisor=%t", m.projectPending, m.supervisorPending)
	}
	_, _ = m.Update(projectMsg{generation: m.generation, value: core.ProjectOverview{ObservedAt: time.Now()}})
	if !m.supervisorPending {
		t.Fatal("project response incorrectly cleared independent supervisor pending state")
	}

	m.workspaceID = "ws_health"
	m.route = route{Page: "tasks"}
	m.projectPending, m.supervisorPending = false, false
	cmd := m.beginRefresh()
	if cmd == nil || !m.snapshotPending || !m.runtimePending {
		t.Fatalf("workspace refresh did not schedule snapshot/runtime: cmd=%v snapshot=%t runtime=%t", cmd != nil, m.snapshotPending, m.runtimePending)
	}
	if m.supervisorPending {
		t.Fatal("workspace refresh scheduled a duplicate project supervisor read")
	}
	if backend.supervisorCalls != 0 {
		t.Fatalf("workspace refresh called project supervisor directly: %d", backend.supervisorCalls)
	}
}

func TestSupervisorRefreshGenerationAndLastGoodObservation(t *testing.T) {
	m := New(Config{ProjectFound: true, NoColor: true})
	m.route = route{Page: "project"}
	m.supervisor = core.SupervisorObservation{State: "running", ObservedAt: time.Now()}
	m.lastSuccess = time.Now()
	m.supervisorPending = true
	m.generation = 3
	_, _ = m.Update(supervisorMsg{generation: 2, value: core.SupervisorObservation{State: "stopped", ObservedAt: time.Now()}})
	if m.supervisor.State != "running" || !m.supervisorPending {
		t.Fatalf("stale supervisor response replaced current observation: %+v pending=%t", m.supervisor, m.supervisorPending)
	}

	_, _ = m.Update(supervisorMsg{generation: 3, err: errors.New("read locked")})
	if m.supervisor.State != "running" || m.supervisorReadError != "read locked" {
		t.Fatalf("failed read did not preserve last good observation: %+v error=%q", m.supervisor, m.supervisorReadError)
	}
	if status := m.statusRow(); !strings.Contains(status, "read locked") || !strings.Contains(status, "Stale") {
		t.Fatalf("failed supervisor read was not visible as stale: %q", status)
	}
}

func TestManualRefreshIncludesProjectSupervisorPendingState(t *testing.T) {
	backend := &healthBackend{}
	m := New(Config{ProjectFound: true, Backend: backend})
	m.route = route{Page: "project"}
	_, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !m.projectPending || !m.supervisorPending {
		t.Fatalf("manual refresh omitted pending reads: project=%t supervisor=%t", m.projectPending, m.supervisorPending)
	}
}
