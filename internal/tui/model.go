package tui

import (
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"workspace/internal/core"
)

type route struct {
	Page         string
	EntityID     string
	ParentID     string
	Tab          string
	Query        string
	SelectedID   string
	StatusFilter string
	Sort         string
}

type routeKey struct {
	WorkspaceID string
	Page        string
	EntityID    string
	ParentID    string
	Tab         string
}

type routeMemory struct {
	Query, SelectedID, StatusFilter, Sort string
	ViewportOffset                        int
}

type collectionItem struct {
	ID, Kind, Title, Subtitle, State string
	At                               time.Time
}

type Model struct {
	backend   Backend
	navigator Navigator

	projectRoot  string
	projectID    string
	cwd          string
	workspaceID  string
	projectFound bool
	initialError string

	palette         palette
	width, height   int
	viewport        viewport.Model
	helpViewport    viewport.Model
	keys            keyMap
	filterInput     textinput.Model
	filtering       bool
	filterOriginal  string
	filterSelection string
	showHelp        bool
	form            *huh.Form
	formMode        string
	formChoice      string
	formWorkflow    string
	formConfirm     bool
	formReason      string
	formTitle       string
	formInput       string
	formTyped       string
	formAction      ActionCall
	formTargetID    string
	actionPending   bool
	actionFailure   bool
	actionCompleted bool
	lastAction      *ActionCall
	notice          string
	quit            bool

	route       route
	stack       []route
	routeMemory map[routeKey]routeMemory

	project            core.ProjectOverview
	snapshot           core.WorkspaceSnapshot
	runtime            core.RuntimeObservation
	uiStatus           core.UIStatus
	worktreeInspection map[string]core.WorktreeObservation
	preview            core.Preview
	loadError          string
	runtimeError       string
	uiError            string
	lastSuccess        time.Time
	lastFailure        time.Time
	generation         uint64
	managed            bool
	hidePending        bool
	hideKey            string
	projectPending     bool
	snapshotPending    bool
	runtimePending     bool
	uiPending          bool
	previewPending     bool
	navigationPending  bool
	worktreePending    bool
	mutationPending    bool
	closed             bool
	spinner            spinner.Model
	animating          bool
	externalProcess    *ExternalProcessRequest
	resumeCmd          tea.Cmd
}

// ExternalProcessRequest is a prepared interactive process which must run
// after Bubble Tea has fully restored the terminal. Keeping this handoff
// outside tea.ExecProcess avoids Bubble Tea's renderer stop/start race
// (https://github.com/charmbracelet/bubbletea/issues/1778).
type ExternalProcessRequest struct {
	Cmd            *exec.Cmd
	Ref            core.EntityRef
	AfterReconcile bool
	Generation     uint64
}

type projectMsg struct {
	generation uint64
	value      core.ProjectOverview
	err        error
}
type snapshotMsg struct {
	generation uint64
	value      core.WorkspaceSnapshot
	err        error
}
type runtimeMsg struct {
	generation uint64
	value      core.RuntimeObservation
	err        error
}
type uiStatusMsg struct {
	generation uint64
	value      core.UIStatus
	err        error
}
type workflowNamesMsg struct {
	generation uint64
	action     string
	names      []string
	err        error
}
type worktreeMsg struct {
	generation uint64
	id         string
	value      core.WorktreeObservation
	err        error
}
type previewMsg struct {
	generation uint64
	value      core.Preview
	err        error
}
type refreshTimerMsg struct{}
type animationMsg struct{}
type navigationTargetMsg struct {
	ref            core.EntityRef
	afterReconcile bool
	generation     uint64
	target         core.NavigationTarget
	err            error
}
type navigationResultMsg struct {
	ref            core.EntityRef
	afterReconcile bool
	generation     uint64
	err            error
}
type actionResultMsg struct {
	generation uint64
	call       ActionCall
	err        error
}
type hideResultMsg struct {
	generation uint64
	err        error
}

func New(config Config) *Model {
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "filter by name or ID"
	input.CharLimit = 0
	input.Width = 32
	m := &Model{
		backend:      config.Backend,
		navigator:    config.Navigator,
		projectRoot:  config.ProjectRoot,
		projectID:    config.ProjectID,
		cwd:          config.CWD,
		workspaceID:  config.WorkspaceID,
		projectFound: config.ProjectFound,
		initialError: sanitizeLine(config.InitialError),
		managed:      config.Managed,
		hideKey:      core.ID("tuihide"),
		palette:      makePalette(config.Theme, config.NoColor),
		width:        80, height: 24,
		keys:               defaultKeyMap(),
		filterInput:        input,
		worktreeInspection: make(map[string]core.WorktreeObservation),
		routeMemory:        make(map[routeKey]routeMemory),
	}
	m.viewport = viewport.New(76, 16)
	m.helpViewport = viewport.New(76, 16)
	m.spinner = spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(m.palette.spinnerStyle()),
	)
	if m.workspaceID == "" {
		m.route = route{Page: "project"}
	} else {
		m.route = route{Page: "tasks"}
	}
	if m.initialError != "" {
		m.route = route{Page: "error"}
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	if !m.projectFound || m.initialError != "" || m.backend == nil {
		return nil
	}
	resume := m.resumeCmd
	m.resumeCmd = nil
	// beginRefresh marks the first load as pending; only then does the spinner
	// animation loop arm, so an idle interface issues no ticks.
	return tea.Batch(m.beginRefresh(), m.ensureAnimation(), resume)
}

// TakeExternalProcessRequest transfers a prepared process to the CLI runner.
// A nil result means that the program exited normally.
func (m *Model) TakeExternalProcessRequest() *ExternalProcessRequest {
	request := m.externalProcess
	m.externalProcess = nil
	return request
}

// ResumeExternalProcess applies the result of an external process and prepares
// the command that the next, fresh tea.Program must run during Init.
func (m *Model) ResumeExternalProcess(request *ExternalProcessRequest, err error) {
	m.quit = false
	_, m.resumeCmd = m.Update(navigationResultMsg{
		generation:     m.generation,
		err:            err,
		ref:            request.Ref,
		afterReconcile: request.AfterReconcile,
	})
}

func (m *Model) activeWorkspace() string { return m.workspaceID }

func (m *Model) push(next route) {
	m.rememberRoute()
	m.stack = append(m.stack, m.route)
	m.activateRoute(next)
}

func (m *Model) navigate(next route) {
	m.rememberRoute()
	m.activateRoute(next)
}

func (m *Model) activateRoute(next route) {
	if memory, ok := m.routeMemory[m.routeKey(next)]; ok {
		next.Query = memory.Query
		next.SelectedID = memory.SelectedID
		next.StatusFilter = memory.StatusFilter
		next.Sort = memory.Sort
	}
	m.route = next
	m.resetView()
	if memory, ok := m.routeMemory[m.routeKey(next)]; ok {
		m.viewport.SetYOffset(memory.ViewportOffset)
	}
}

func (m *Model) routeKey(route route) routeKey {
	return routeKey{
		WorkspaceID: m.workspaceID,
		Page:        route.Page,
		EntityID:    route.EntityID,
		ParentID:    route.ParentID,
		Tab:         route.Tab,
	}
}

func (m *Model) rememberRoute() {
	if m.routeMemory == nil {
		m.routeMemory = make(map[routeKey]routeMemory)
	}
	m.routeMemory[m.routeKey(m.route)] = routeMemory{
		Query:          m.route.Query,
		SelectedID:     m.route.SelectedID,
		StatusFilter:   m.route.StatusFilter,
		Sort:           m.route.Sort,
		ViewportOffset: m.viewport.YOffset,
	}
}

func (m *Model) pop() {
	m.rememberRoute()
	if len(m.stack) == 0 {
		if m.workspaceID != "" {
			m.workspaceID = ""
			m.generation++
			m.activateRoute(route{Page: "project"})
			return
		}
		m.quit = true
		return
	}
	next := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	m.activateRoute(next)
}

func (m *Model) resetView() {
	m.viewport.GotoTop()
	m.rebuildViewport()
}

func (m *Model) setWorkspace(id string) tea.Cmd {
	m.rememberRoute()
	m.workspaceID = id
	m.generation++
	m.stack = nil
	m.snapshot = core.WorkspaceSnapshot{}
	m.runtime = core.RuntimeObservation{}
	m.uiStatus = core.UIStatus{}
	m.preview = core.Preview{}
	m.worktreeInspection = make(map[string]core.WorktreeObservation)
	m.lastSuccess = time.Time{}
	m.loadError = ""
	m.runtimeError = ""
	m.uiError = ""
	m.snapshotPending = false
	m.runtimePending = false
	m.uiPending = false
	m.previewPending = false
	m.worktreePending = false
	m.projectPending = false
	m.activateRoute(route{Page: "tasks"})
	return m.beginRefresh()
}
