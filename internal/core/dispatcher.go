package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const dispatcherSchemaVersion = 1

type DispatcherSummary struct {
	State         string    `json:"state" yaml:"state"`
	AgentID       string    `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`
	Role          string    `json:"role,omitempty" yaml:"role,omitempty"`
	Profile       string    `json:"profile,omitempty" yaml:"profile,omitempty"`
	SessionID     string    `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	CurrentRunID  string    `json:"current_run_id,omitempty" yaml:"current_run_id,omitempty"`
	LastRunID     string    `json:"last_run_id,omitempty" yaml:"last_run_id,omitempty"`
	RunCount      int       `json:"run_count" yaml:"run_count"`
	StopRequested bool      `json:"stop_requested" yaml:"stop_requested"`
	UpdatedAt     time.Time `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	Error         string    `json:"error,omitempty" yaml:"error,omitempty"`
}

type DispatcherStatus struct {
	SchemaVersion int               `json:"schema_version" yaml:"schema_version"`
	ProjectID     string            `json:"project_id" yaml:"project_id"`
	ProjectRoot   string            `json:"project_root" yaml:"project_root"`
	Initialized   bool              `json:"initialized" yaml:"initialized"`
	State         string            `json:"state" yaml:"state"`
	StopRequested bool              `json:"stop_requested" yaml:"stop_requested"`
	Agent         Agent             `json:"agent" yaml:"agent"`
	Sessions      []Session         `json:"sessions" yaml:"sessions"`
	Runs          []Run             `json:"runs" yaml:"runs"`
	Messages      []Message         `json:"messages,omitempty" yaml:"messages,omitempty"`
	ObservedAt    time.Time         `json:"observed_at" yaml:"observed_at"`
	Runtime       DispatcherRuntime `json:"runtime" yaml:"runtime"`
}

type DispatcherRuntime struct {
	SessionName string    `json:"session_name,omitempty" yaml:"session_name,omitempty"`
	SessionID   string    `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	RunID       string    `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	PaneID      string    `json:"pane_id,omitempty" yaml:"pane_id,omitempty"`
	WindowID    string    `json:"window_id,omitempty" yaml:"window_id,omitempty"`
	Verified    bool      `json:"verified" yaml:"verified"`
	Error       string    `json:"error,omitempty" yaml:"error,omitempty"`
	ObservedAt  time.Time `json:"observed_at" yaml:"observed_at"`
}

type dispatcherState struct {
	SchemaVersion int                          `json:"schema_version"`
	ProjectID     string                       `json:"project_id"`
	Agent         Agent                        `json:"agent"`
	Sessions      []Session                    `json:"sessions"`
	Runs          []Run                        `json:"runs"`
	Messages      []Message                    `json:"messages,omitempty"`
	Deliveries    []DeliveryAttempt            `json:"deliveries,omitempty"`
	Operations    map[string]Operation         `json:"operations,omitempty"`
	Receipts      map[string]dispatcherReceipt `json:"receipts,omitempty"`
	StopRequested bool                         `json:"stop_requested"`
	UpdatedAt     time.Time                    `json:"updated_at"`
}

type dispatcherReceipt struct {
	ID       string          `json:"id"`
	Digest   string          `json:"digest"`
	State    string          `json:"state"`
	Result   json.RawMessage `json:"result,omitempty"`
	Revision int             `json:"revision,omitempty"`
}

func saveDispatcherReceipt(root string, state dispatcherState, key string, receipt dispatcherReceipt, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	receipt.State, receipt.Result, receipt.Revision = "completed", b, len(state.Runs)
	if state.Receipts == nil {
		state.Receipts = map[string]dispatcherReceipt{}
	}
	state.Receipts[key] = receipt
	return saveDispatcherState(root, state)
}

func dispatcherRoot(root string) string { return filepath.Join(root, ".workspace", "dispatcher") }
func dispatcherStatePath(root string) string {
	return filepath.Join(dispatcherRoot(root), "state.json")
}
func dispatcherPromptDir(root string) string { return filepath.Join(dispatcherRoot(root), "prompts") }

func defaultDispatcherState(projectID string) dispatcherState {
	return dispatcherState{SchemaVersion: dispatcherSchemaVersion, ProjectID: projectID, Sessions: []Session{}, Runs: []Run{}, Messages: []Message{}, Deliveries: []DeliveryAttempt{}, Operations: map[string]Operation{}, Receipts: map[string]dispatcherReceipt{}}
}

func loadDispatcherState(root string, projectID string) (dispatcherState, bool, error) {
	state := defaultDispatcherState(projectID)
	b, err := os.ReadFile(dispatcherStatePath(root))
	if errors.Is(err, os.ErrNotExist) {
		return state, false, nil
	}
	if err != nil {
		return state, true, err
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return state, true, fail("dispatcher_state_invalid", "cannot decode Dispatcher state: %v", err)
	}
	if state.SchemaVersion != dispatcherSchemaVersion || state.ProjectID != projectID || state.Agent.ID == "" || state.Agent.Role != "dispatcher" || state.Agent.Scope != "project" {
		return state, true, fail("dispatcher_state_invalid", "Dispatcher state identity or schema is invalid")
	}
	if state.Operations == nil {
		state.Operations = map[string]Operation{}
	}
	if state.Receipts == nil {
		state.Receipts = map[string]dispatcherReceipt{}
	}
	return state, true, nil
}

func saveDispatcherState(root string, state dispatcherState) error {
	state.SchemaVersion = dispatcherSchemaVersion
	state.UpdatedAt = nowUTC()
	if state.Operations == nil {
		state.Operations = map[string]Operation{}
	}
	if state.Receipts == nil {
		state.Receipts = map[string]dispatcherReceipt{}
	}
	return writeJSON(dispatcherStatePath(root), state)
}

func dispatcherSummaryLocked(root, projectID string) DispatcherSummary {
	state, exists, err := loadDispatcherState(root, projectID)
	if err != nil {
		return DispatcherSummary{State: "error", Error: err.Error()}
	}
	if !exists {
		return DispatcherSummary{State: "never_started"}
	}
	out := DispatcherSummary{State: "idle", AgentID: state.Agent.ID, Role: state.Agent.Role, Profile: state.Agent.Profile, StopRequested: state.StopRequested, UpdatedAt: state.UpdatedAt}
	if state.StopRequested {
		out.State = "stopped"
	}
	var latest *Session
	for i := range state.Sessions {
		candidate := &state.Sessions[i]
		if latest == nil || candidate.LastActiveAt.After(latest.LastActiveAt) || candidate.LastActiveAt.Equal(latest.LastActiveAt) && candidate.ID > latest.ID {
			latest = candidate
		}
	}
	if latest != nil {
		out.SessionID, out.CurrentRunID, out.LastRunID, out.RunCount = latest.ID, latest.CurrentRunID, latest.LastRunID, latest.RunCount
		if latest.Active() {
			out.State = "running"
		}
	}
	if out.State != "running" {
		for _, run := range sortedDispatcherRuns(state.Runs) {
			if run.SessionID != out.SessionID {
				continue
			}
			if run.State == "failed" || run.State == "interrupted" {
				out.State, out.Error = run.State, run.Error
			}
			break
		}
	}
	return out
}

func dispatcherStatusFromState(root string, state dispatcherState, runtime DispatcherRuntime) DispatcherStatus {
	out := DispatcherStatus{SchemaVersion: state.SchemaVersion, ProjectID: state.ProjectID, ProjectRoot: root, Initialized: true, State: "idle", StopRequested: state.StopRequested, Agent: state.Agent, Sessions: append([]Session(nil), state.Sessions...), Runs: append([]Run(nil), state.Runs...), Messages: append([]Message(nil), state.Messages...), ObservedAt: nowUTC(), Runtime: runtime}
	if state.StopRequested {
		out.State = "stopped"
	}
	for _, session := range state.Sessions {
		if session.Active() {
			out.State = "running"
			break
		}
	}
	if out.State != "running" {
		for _, run := range sortedDispatcherRuns(state.Runs) {
			if run.State == "failed" || run.State == "interrupted" {
				out.State, out.Runtime.Error = run.State, run.Error
				break
			}
		}
	}
	return out
}

func findDispatcherSession(state *dispatcherState, id string) (*Session, error) {
	for i := range state.Sessions {
		if state.Sessions[i].ID == id {
			return &state.Sessions[i], nil
		}
	}
	return nil, fail("session_not_found", "unknown Dispatcher Session %q", id)
}

func findDispatcherRun(state *dispatcherState, id string) (*Run, error) {
	for i := range state.Runs {
		if state.Runs[i].ID == id {
			return &state.Runs[i], nil
		}
	}
	return nil, fail("run_not_found", "unknown Dispatcher Run %q", id)
}

func syncDispatcherSession(state *dispatcherState, session *Session) {
	var latest, current *Run
	session.RunCount = 0
	for i := range state.Runs {
		run := &state.Runs[i]
		if run.SessionID != session.ID {
			continue
		}
		session.RunCount++
		if latest == nil || run.CreatedAt.After(latest.CreatedAt) || run.CreatedAt.Equal(latest.CreatedAt) && run.ID > latest.ID {
			latest = run
		}
		if run.ID == session.CurrentRunID {
			current = run
		}
	}
	if latest != nil {
		session.LastRunID = latest.ID
	}
	if current == nil || !current.Active() {
		session.CurrentRunID = ""
	}
	selected := latest
	if current != nil && current.Active() {
		selected = current
	}
	if selected == nil {
		session.State, session.LifecycleState = "idle", "idle"
		return
	}
	session.Profile, session.ReasoningEffort, session.Route, session.RoutingDecision = selected.Profile, selected.ReasoningEffort, selected.Route, selected.RoutingDecision
	session.Argv, session.CWD, session.PromptFile = append([]string(nil), selected.Argv...), selected.CWD, selected.PromptFile
	session.RunState = selected.State
	session.State = projectSessionState(selected.State)
	session.PaneID, session.WindowID = selected.PaneID, selected.WindowID
	session.FinishedAt, session.ExitCode, session.Error, session.ClientState = selected.FinishedAt, selected.ExitCode, selected.Error, selected.ClientState
	session.ClientThreadID = selected.ClientThreadID
	session.LastActiveAt = selected.CreatedAt
	if selected.FinishedAt != nil {
		session.LastActiveAt = *selected.FinishedAt
	}
	if current != nil && current.Active() {
		session.LifecycleState = "active"
	} else {
		session.LifecycleState = "idle"
	}
}

func latestDispatcherSession(state dispatcherState) *Session {
	var latest *Session
	for i := range state.Sessions {
		candidate := &state.Sessions[i]
		if candidate.DeletedAt != nil {
			continue
		}
		if latest == nil || candidate.LastActiveAt.After(latest.LastActiveAt) || candidate.LastActiveAt.Equal(latest.LastActiveAt) && candidate.ID > latest.ID {
			latest = candidate
		}
	}
	return latest
}

func dispatcherRunActive(state dispatcherState) bool {
	for _, session := range state.Sessions {
		if session.AgentID == state.Agent.ID && session.Active() {
			return true
		}
	}
	return false
}

func sortedDispatcherRuns(runs []Run) []Run {
	out := append([]Run(nil), runs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func normalizeDispatcherProfile(cfg Config, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if cfg.Defaults.DispatcherProfile != "" {
		return cfg.Defaults.DispatcherProfile
	}
	return cfg.Defaults.OrchestratorProfile
}

func (s *Service) DispatcherStatus(ctx context.Context) (DispatcherStatus, error) {
	var out DispatcherStatus
	err := withProjectReadLock(ctx, s.Root, func() error {
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		state, exists, err := loadDispatcherState(s.Root, cfg.ProjectID)
		if err != nil {
			return err
		}
		if !exists {
			out = DispatcherStatus{ProjectID: cfg.ProjectID, ProjectRoot: s.Root, State: "never_started", ObservedAt: nowUTC()}
			return nil
		}
		out = dispatcherStatusFromState(s.Root, state, DispatcherRuntime{SessionName: DispatcherTmuxName(cfg.ProjectID), ObservedAt: nowUTC()})
		return nil
	})
	if err == nil && out.Initialized {
		if reader, ok := s.Runtime.(interface {
			ObserveProjectTopology(context.Context, string) (TmuxTopology, error)
		}); ok {
			topology, observeErr := reader.ObserveProjectTopology(ctx, out.ProjectID)
			out.Runtime.SessionName, out.Runtime.ObservedAt = topology.SessionName, topology.ObservedAt
			if observeErr != nil {
				out.Runtime.Error = observeErr.Error()
			} else {
				for _, pane := range topology.Panes {
					if !pane.Dead && pane.Kind == "dispatcher" && pane.Scope == "project" && pane.ProjectID == out.ProjectID {
						out.Runtime.Verified = true
						out.Runtime.SessionID, out.Runtime.RunID, out.Runtime.PaneID, out.Runtime.WindowID = pane.SessionID, pane.RunID, pane.ID, pane.WindowID
						break
					}
				}
			}
		}
	}
	return out, err
}
