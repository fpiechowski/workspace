package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

type Error struct {
	Options []string `json:"options,omitempty" yaml:"options,omitempty"`
	Code    string   `json:"code" yaml:"code"`
	Message string   `json:"message" yaml:"message"`
}

func decisionRequired(message string, options ...string) error {
	return &Error{Code: "decision_required", Message: message, Options: options}
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func fail(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
func ID(prefix string) string { return prefix + "_" + ulid.Make().String() }
func nowUTC() time.Time       { return time.Now().UTC() }

type Config struct {
	WorkspacesDir string                    `yaml:"workspaces_dir,omitempty" json:"workspaces_dir,omitempty"`
	Tracker       TrackerConfig             `yaml:"tracker,omitempty" json:"tracker,omitempty"`
	SchemaVersion int                       `yaml:"schema_version" json:"schema_version"`
	ProjectID     string                    `yaml:"project_id" json:"project_id"`
	Runtime       string                    `yaml:"runtime" json:"runtime"`
	Clients       map[string]Client         `yaml:"clients" json:"clients"`
	Profiles      map[string]Profile        `yaml:"profiles" json:"profiles"`
	Defaults      DefaultsConfig            `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Forge         ForgeConfig               `yaml:"forge,omitempty" json:"forge,omitempty"`
	Workflows     map[string]WorkflowConfig `yaml:"workflows,omitempty" json:"workflows,omitempty"`
}
type DefaultsConfig struct {
	OrchestratorProfile string `yaml:"orchestrator_profile,omitempty" json:"orchestrator_profile,omitempty"`
}
type ForgeConfig struct {
	Adapter     string   `yaml:"adapter" json:"adapter"`
	Remote      string   `yaml:"remote" json:"remote"`
	CommandArgv []string `yaml:"command_argv,omitempty" json:"command_argv,omitempty"`
	Publication string   `yaml:"publication,omitempty" json:"publication,omitempty"`
}
type WorkflowConfig struct {
	Profiles         map[string]string `yaml:"profiles" json:"profiles"`
	MaxParallelTasks int               `yaml:"max_parallel_tasks" json:"max_parallel_tasks"`
	ChangeRequests   string            `yaml:"change_requests" json:"change_requests"`
}
type Client struct {
	Adapter        string         `yaml:"adapter" json:"adapter"`
	LaunchArgv     []string       `yaml:"launch_argv" json:"launch_argv"`
	ResumeArgv     []string       `yaml:"resume_argv,omitempty" json:"resume_argv,omitempty"`
	DeliverArgv    []string       `yaml:"deliver_argv,omitempty" json:"deliver_argv,omitempty"`
	ThreadParams   map[string]any `yaml:"thread_params,omitempty" json:"thread_params,omitempty"`
	Capabilities   []string       `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	NativeDelivery bool           `yaml:"native_delivery,omitempty" json:"native_delivery,omitempty"`
}
type Profile struct {
	RequiredCapabilities []string           `yaml:"required_capabilities,omitempty" json:"required_capabilities,omitempty"`
	CooldownSeconds      int                `yaml:"cooldown_seconds,omitempty" json:"cooldown_seconds,omitempty"`
	ProviderWeights      map[string]float64 `yaml:"provider_weights,omitempty" json:"provider_weights,omitempty"`
	ReasoningEffort      string             `yaml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
	Routes               []Route            `yaml:"routes" json:"routes"`
	Strategy             string             `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}
type Route struct {
	MaxLaunches24h int    `yaml:"max_launches_24h,omitempty" json:"max_launches_24h,omitempty"`
	ID             string `yaml:"id" json:"id"`
	Client         string `yaml:"client" json:"client"`
	Provider       string `yaml:"provider" json:"provider"`
	Model          string `yaml:"model" json:"model"`
	MaxConcurrency int    `yaml:"max_concurrency" json:"max_concurrency"`
}
type Workspace struct {
	ChangeRequestMode   string          `yaml:"change_request_mode,omitempty" json:"change_request_mode,omitempty"`
	ProjectRoot         string          `yaml:"project_root,omitempty" json:"project_root,omitempty"`
	SchemaVersion       int             `yaml:"schema_version" json:"schema_version"`
	ID                  string          `yaml:"id" json:"id"`
	ProjectID           string          `yaml:"project_id" json:"project_id"`
	Title               string          `yaml:"title" json:"title"`
	Revision            int             `yaml:"revision" json:"revision"`
	Status              string          `yaml:"status" json:"status"`
	Workflow            *Workflow       `yaml:"workflow" json:"workflow"`
	Input               Input           `yaml:"input" json:"input"`
	Base                Base            `yaml:"base" json:"base"`
	OrchestratorAgentID string          `yaml:"orchestrator_agent_id" json:"orchestrator_agent_id"`
	CreatedAt           time.Time       `yaml:"created_at" json:"created_at"`
	Tasks               []Task          `yaml:"tasks" json:"tasks"`
	Artifacts           []Artifact      `yaml:"artifacts" json:"artifacts"`
	Decisions           []Decision      `yaml:"decisions" json:"decisions"`
	PendingDecision     *Decision       `yaml:"pending_decision" json:"pending_decision"`
	Integration         *Integration    `yaml:"integration" json:"integration"`
	ChangeRequests      []ChangeRequest `yaml:"change_requests" json:"change_requests"`
	LiveTest            LiveTest        `yaml:"live_test" json:"live_test"`
	Release             Release         `yaml:"release" json:"release"`
}

// workspaceMode classifies the durable workspace mode. It is the single source
// of truth behind Manual, NeedsWorkflow and WorkflowSelected so creation,
// delegation and lifecycle code never repeat the fragile nil/status test.
type workspaceMode int

const (
	modeWorkflow workspaceMode = iota
	modeNeedsWorkflow
	modeManual
)

func (w Workspace) mode() workspaceMode {
	switch {
	case w.Workflow != nil:
		return modeWorkflow
	case w.Status == "needs_workflow":
		return modeNeedsWorkflow
	default:
		return modeManual
	}
}

// Manual reports whether the workspace intentionally has no workflow. A nil
// Workflow alone is not enough: an omitted creation choice stays in
// needs_workflow until the user selects a workflow.
func (w Workspace) Manual() bool { return w.mode() == modeManual }

// NeedsWorkflow reports whether creation omitted a workflow and the workspace is
// still waiting for the user to select one.
func (w Workspace) NeedsWorkflow() bool { return w.mode() == modeNeedsWorkflow }

// WorkflowSelected reports whether the workspace is driven by a selected
// workflow snapshot.
func (w Workspace) WorkflowSelected() bool { return w.mode() == modeWorkflow }

// PhaseLabel is the stable presentation label for the workflow phase or mode. It
// never synthesizes a Workflow object: an intentional manual workspace reads
// "manual", a selected workflow shows its phase, and a pending creation choice
// has no label yet.
func (w Workspace) PhaseLabel() string {
	switch w.mode() {
	case modeWorkflow:
		return w.Workflow.Phase
	case modeManual:
		return "manual"
	default:
		return ""
	}
}

type Workflow struct {
	ID             string `yaml:"id" json:"id"`
	Version        int    `yaml:"version" json:"version"`
	TemplateDigest string `yaml:"template_digest" json:"template_digest"`
	Phase          string `yaml:"phase" json:"phase"`
}
type Input struct {
	Source   string `yaml:"source,omitempty" json:"source,omitempty"`
	Snapshot string `yaml:"snapshot" json:"snapshot"`
}
type Base struct {
	Ref    string `yaml:"ref" json:"ref"`
	Commit string `yaml:"commit" json:"commit"`
}

// Agent is a persona definition. A Session is a durable logical conversation;
// each concrete client/tmux execution is recorded as a Run.
type Agent struct {
	ID             string `json:"id" yaml:"id"`
	Name           string `json:"name" yaml:"name"`
	Role           string `json:"role" yaml:"role"`
	Profile        string `json:"profile" yaml:"profile"`
	PromptTemplate string `json:"prompt_template" yaml:"prompt_template"`
	Instructions   string `json:"instructions" yaml:"instructions"`
}
type Worktree struct {
	ID         string `json:"id" yaml:"id"`
	Name       string `json:"name" yaml:"name"`
	Path       string `json:"path" yaml:"path"`
	Branch     string `json:"branch" yaml:"branch"`
	BaseCommit string `json:"base_commit" yaml:"base_commit"`
	Purpose    string `json:"purpose" yaml:"purpose"`
	State      string `json:"state" yaml:"state"`
}
type Session struct {
	ID              string     `json:"id" yaml:"id"`
	AgentID         string     `json:"agent_id" yaml:"agent_id"`
	AgentSnapshot   Agent      `json:"agent_snapshot" yaml:"agent_snapshot"`
	ParentAgentID   string     `json:"parent_agent_id,omitempty" yaml:"parent_agent_id,omitempty"`
	ParentSessionID string     `json:"parent_session_id,omitempty" yaml:"parent_session_id,omitempty"`
	WorktreeID      string     `json:"worktree_id,omitempty" yaml:"worktree_id,omitempty"`
	TaskID          string     `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	TaskAttempt     int        `json:"task_attempt,omitempty" yaml:"task_attempt,omitempty"`
	InputDigest     string     `json:"input_digest,omitempty" yaml:"input_digest,omitempty"`
	ClientSnapshot  Client     `json:"client_snapshot" yaml:"client_snapshot"`
	ClientThreadID  string     `json:"client_thread_id,omitempty" yaml:"client_thread_id,omitempty"`
	ReadOnly        bool       `json:"read_only" yaml:"read_only"`
	CreatedAt       time.Time  `json:"created_at" yaml:"created_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty" yaml:"deleted_at,omitempty"`
	ClosedAt        *time.Time `json:"closed_at,omitempty" yaml:"closed_at,omitempty"`
	CloseReason     string     `json:"close_reason,omitempty" yaml:"close_reason,omitempty"`
	LifecycleState  string     `json:"lifecycle_state" yaml:"lifecycle_state"`
	CurrentRunID    string     `json:"current_run_id,omitempty" yaml:"current_run_id,omitempty"`
	LastRunID       string     `json:"last_run_id,omitempty" yaml:"last_run_id,omitempty"`
	RunCount        int        `json:"run_count" yaml:"run_count"`
	LastActiveAt    time.Time  `json:"last_active_at" yaml:"last_active_at"`

	// The fields below are a compatibility/status projection of current_run (or
	// last_run when idle). They are never the source of runtime ownership.
	RoutingDecision  *RoutingDecision `json:"routing_decision,omitempty" yaml:"routing_decision,omitempty"`
	Profile          string           `json:"profile" yaml:"profile"`
	ReasoningEffort  string           `json:"reasoning_effort,omitempty" yaml:"reasoning_effort,omitempty"`
	Route            Route            `json:"route" yaml:"route"`
	Argv             []string         `json:"argv" yaml:"argv"`
	CWD              string           `json:"cwd" yaml:"cwd"`
	PromptFile       string           `json:"prompt_file" yaml:"prompt_file"`
	RunState         string           `json:"run_state" yaml:"run_state"`
	State            string           `json:"state" yaml:"state"`
	PaneID           string           `json:"pane_id,omitempty" yaml:"pane_id,omitempty"`
	WindowID         string           `json:"window_id,omitempty" yaml:"window_id,omitempty"`
	FinishedAt       *time.Time       `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	ExitCode         *int             `json:"exit_code,omitempty" yaml:"exit_code,omitempty"`
	Error            string           `json:"error,omitempty" yaml:"error,omitempty"`
	ClientState      string           `json:"client_state,omitempty" yaml:"client_state,omitempty"`
	OpenCodeEndpoint string           `json:"opencode_endpoint,omitempty" yaml:"opencode_endpoint,omitempty"`
}

func (s Session) Active() bool {
	state := s.State
	if state == "" {
		state = s.RunState
	}
	return s.CurrentRunID != "" && (state == "starting" || state == "running")
}

type Run struct {
	ID               string           `json:"id" yaml:"id"`
	SessionID        string           `json:"session_id" yaml:"session_id"`
	Generation       int              `json:"generation" yaml:"generation"`
	ConversationOnly bool             `json:"conversation_only,omitempty" yaml:"conversation_only,omitempty"`
	Profile          string           `json:"profile" yaml:"profile"`
	ReasoningEffort  string           `json:"reasoning_effort,omitempty" yaml:"reasoning_effort,omitempty"`
	Route            Route            `json:"route" yaml:"route"`
	RoutingDecision  *RoutingDecision `json:"routing_decision,omitempty" yaml:"routing_decision,omitempty"`
	Argv             []string         `json:"argv" yaml:"argv"`
	CWD              string           `json:"cwd" yaml:"cwd"`
	PromptFile       string           `json:"prompt_file" yaml:"prompt_file"`
	State            string           `json:"state" yaml:"state"`
	PaneID           string           `json:"pane_id,omitempty" yaml:"pane_id,omitempty"`
	WindowID         string           `json:"window_id,omitempty" yaml:"window_id,omitempty"`
	ClientState      string           `json:"client_state,omitempty" yaml:"client_state,omitempty"`
	ClientThreadID   string           `json:"client_thread_id,omitempty" yaml:"client_thread_id,omitempty"`
	CreatedAt        time.Time        `json:"created_at" yaml:"created_at"`
	FinishedAt       *time.Time       `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	ExitCode         *int             `json:"exit_code,omitempty" yaml:"exit_code,omitempty"`
	Error            string           `json:"error,omitempty" yaml:"error,omitempty"`
	OpenCodeEndpoint string           `json:"opencode_endpoint,omitempty" yaml:"opencode_endpoint,omitempty"`
}

type DeliveryAttempt struct {
	MessageID string    `json:"message_id" yaml:"message_id"`
	RunID     string    `json:"run_id" yaml:"run_id"`
	Phase     string    `json:"phase" yaml:"phase"`
	Error     string    `json:"error,omitempty" yaml:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

func (r Run) Active() bool { return r.State == "starting" || r.State == "running" }

type Operation struct {
	Result     json.RawMessage `json:"result,omitempty"`
	ID         string          `json:"id,omitempty"`
	Revision   int             `json:"revision,omitempty"`
	Digest     string          `json:"digest"`
	ResourceID string          `json:"resource_id"`
}
type Registry struct {
	SchemaVersion   int                        `json:"schema_version,omitempty"`
	Mutations       map[string]MutationReceipt `json:"mutations,omitempty"`
	Services        []BackgroundService        `json:"services"`
	Checks          []CheckReceipt             `json:"checks"`
	WorkspaceDigest string                     `json:"workspace_digest"`
	Agents          []Agent                    `json:"agents"`
	Worktrees       []Worktree                 `json:"worktrees"`
	Sessions        []Session                  `json:"sessions"`
	Runs            []Run                      `json:"runs"`
	Operations      map[string]Operation       `json:"operations"`
	Messages        []Message                  `json:"messages"`
	Deliveries      []DeliveryAttempt          `json:"deliveries,omitempty"`
	Handoffs        []Handoff                  `json:"handoffs"`
}
type Document struct {
	deferSave    bool
	PendingFiles map[string][]byte
	Dir          string
	State        Workspace
	Body         string
	Registry     Registry
}
type Status struct {
	SchemaVersion int        `json:"schema_version" yaml:"schema_version"`
	Directory     string     `json:"directory" yaml:"directory"`
	Workspace     Workspace  `json:"workspace" yaml:"workspace"`
	Agents        []Agent    `json:"agents" yaml:"agents"`
	Worktrees     []Worktree `json:"worktrees" yaml:"worktrees"`
	Sessions      []Session  `json:"sessions" yaml:"sessions"`
	Runs          []Run      `json:"runs" yaml:"runs"`
}

func (d *Document) Status() Status {
	d.syncSessions()
	return Status{SchemaVersion: 2, Directory: d.Dir, Workspace: d.State, Agents: d.Registry.Agents, Worktrees: d.Registry.Worktrees, Sessions: d.Registry.Sessions, Runs: d.Registry.Runs}
}
