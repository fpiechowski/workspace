package core

import "time"

type TaskSpec struct {
	Name               string   `yaml:"name" json:"name"`
	Title              string   `yaml:"title" json:"title"`
	Goal               string   `yaml:"goal" json:"goal"`
	Role               string   `yaml:"role" json:"role"`
	Profile            string   `yaml:"profile" json:"profile"`
	DependsOn          []string `yaml:"depends_on" json:"depends_on"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria" json:"acceptance_criteria"`
	RequiredArtifacts  []string `yaml:"required_artifacts" json:"required_artifacts"`
	RequireChecks      bool     `yaml:"require_checks" json:"require_checks"`
	BaseCommit         string   `yaml:"base_commit,omitempty" json:"base_commit,omitempty"`
}
type Task struct {
	TaskSpec        `yaml:",inline"`
	ID              string     `yaml:"id" json:"id"`
	State           string     `yaml:"state" json:"state"`
	Attempt         int        `yaml:"attempt" json:"attempt"`
	InputDigest     string     `yaml:"input_digest" json:"input_digest"`
	WorktreeID      string     `yaml:"worktree_id,omitempty" json:"worktree_id,omitempty"`
	SessionID       string     `yaml:"session_id,omitempty" json:"session_id,omitempty"`
	RunID           string     `yaml:"run_id,omitempty" json:"run_id,omitempty"`
	AcceptedHandoff string     `yaml:"accepted_handoff,omitempty" json:"accepted_handoff,omitempty"`
	Reason          string     `yaml:"reason,omitempty" json:"reason,omitempty"`
	DeletedAt       *time.Time `yaml:"deleted_at,omitempty" json:"deleted_at,omitempty"`
}
type Artifact struct {
	ID            string    `yaml:"id" json:"id"`
	Name          string    `yaml:"name" json:"name"`
	Kind          string    `yaml:"kind" json:"kind"`
	Path          string    `yaml:"path" json:"path"`
	Digest        string    `yaml:"digest" json:"digest"`
	Size          int64     `yaml:"size" json:"size"`
	SourceHandoff string    `yaml:"source_handoff" json:"source_handoff"`
	AgentID       string    `yaml:"agent_id" json:"agent_id"`
	SessionID     string    `yaml:"session_id" json:"session_id"`
	RunID         string    `yaml:"run_id" json:"run_id"`
	TaskID        string    `yaml:"task_id" json:"task_id"`
	Model         string    `yaml:"model" json:"model"`
	PromptDigest  string    `yaml:"prompt_digest" json:"prompt_digest"`
	BaseCommit    string    `yaml:"base_commit" json:"base_commit"`
	HeadCommit    string    `yaml:"head_commit" json:"head_commit"`
	CreatedAt     time.Time `yaml:"created_at" json:"created_at"`
}
type Check struct {
	Command  string `yaml:"command" json:"command"`
	ExitCode int    `yaml:"exit_code" json:"exit_code"`
	Evidence string `yaml:"evidence" json:"evidence"` // Artifact name at submission, immutable ID thereafter.
}
type Handoff struct {
	ID          string    `yaml:"id" json:"id"`
	FromAgent   string    `yaml:"from_agent" json:"from_agent"`
	FromSession string    `yaml:"from_session" json:"from_session"`
	FromRun     string    `yaml:"from_run" json:"from_run"`
	ToAgent     string    `yaml:"to_agent" json:"to_agent"`
	TaskID      string    `yaml:"task_id" json:"task_id"`
	Attempt     int       `yaml:"attempt" json:"attempt"`
	InputDigest string    `yaml:"input_digest" json:"input_digest"`
	Outcome     string    `yaml:"outcome" json:"outcome"`
	Summary     string    `yaml:"summary" json:"summary"`
	State       string    `yaml:"state" json:"state"`
	Stale       bool      `yaml:"stale" json:"stale"`
	BaseCommit  string    `yaml:"base_commit" json:"base_commit"`
	HeadCommit  string    `yaml:"head_commit" json:"head_commit"`
	Dirty       bool      `yaml:"dirty" json:"dirty"`
	ArtifactIDs []string  `yaml:"artifact_ids" json:"artifact_ids"`
	Checks      []Check   `yaml:"checks" json:"checks"`
	Risks       []string  `yaml:"risks" json:"risks"`
	Feedback    string    `yaml:"feedback,omitempty" json:"feedback,omitempty"`
	CreatedAt   time.Time `yaml:"created_at" json:"created_at"`
}
type Message struct {
	ID                 string     `yaml:"id" json:"id"`
	FromAgent          string     `yaml:"from_agent" json:"from_agent"`
	FromSession        string     `yaml:"from_session,omitempty" json:"from_session,omitempty"`
	FromRun            string     `yaml:"from_run,omitempty" json:"from_run,omitempty"`
	ToAgent            string     `yaml:"to_agent" json:"to_agent"`
	Kind               string     `yaml:"kind" json:"kind"`
	Body               string     `yaml:"body" json:"body"`
	HandoffID          string     `yaml:"handoff_id,omitempty" json:"handoff_id,omitempty"`
	ReplyTo            string     `yaml:"reply_to,omitempty" json:"reply_to,omitempty"`
	CreatedAt          time.Time  `yaml:"created_at" json:"created_at"`
	AcknowledgedAt     *time.Time `yaml:"acknowledged_at,omitempty" json:"acknowledged_at,omitempty"`
	DeliveredAt        *time.Time `yaml:"delivered_at,omitempty" json:"delivered_at,omitempty"`
	DeliveredSessionID string     `yaml:"delivered_session_id,omitempty" json:"delivered_session_id,omitempty"`
	DeliveredRunID     string     `yaml:"delivered_run_id,omitempty" json:"delivered_run_id,omitempty"`
	NotifiedSessionID  string     `yaml:"notified_session_id,omitempty" json:"notified_session_id,omitempty"`
	NotifiedRunID      string     `yaml:"notified_run_id,omitempty" json:"notified_run_id,omitempty"`
}
type Decision struct {
	Environment string     `yaml:"environment,omitempty" json:"environment,omitempty"`
	Superseded  bool       `yaml:"superseded,omitempty" json:"superseded,omitempty"`
	ID          string     `yaml:"id" json:"id"`
	Kind        string     `yaml:"kind" json:"kind"`
	Question    string     `yaml:"question" json:"question"`
	Options     []string   `yaml:"options" json:"options"`
	Revision    int        `yaml:"revision" json:"revision"`
	BasisDigest string     `yaml:"basis_digest" json:"basis_digest"`
	Answer      string     `yaml:"answer,omitempty" json:"answer,omitempty"`
	Reason      string     `yaml:"reason,omitempty" json:"reason,omitempty"`
	AnsweredAt  *time.Time `yaml:"answered_at,omitempty" json:"answered_at,omitempty"`
}
type Integration struct {
	WorktreeID  string   `yaml:"worktree_id" json:"worktree_id"`
	TaskIDs     []string `yaml:"task_ids" json:"task_ids"`
	Heads       []string `yaml:"heads" json:"heads"`
	BaseCommit  string   `yaml:"base_commit" json:"base_commit"`
	HeadCommit  string   `yaml:"head_commit" json:"head_commit"`
	InputDigest string   `yaml:"input_digest" json:"input_digest"`
}
type ChangeRequest struct {
	TaskID             string   `yaml:"task_id,omitempty" json:"task_id,omitempty"`
	MergeAfter         []string `yaml:"merge_after,omitempty" json:"merge_after,omitempty"`
	ID                 string   `yaml:"id" json:"id"`
	WorktreeID         string   `yaml:"worktree_id" json:"worktree_id"`
	Title              string   `yaml:"title" json:"title"`
	Body               string   `yaml:"body" json:"body"`
	Branch             string   `yaml:"branch" json:"branch"`
	Target             string   `yaml:"target" json:"target"`
	HeadCommit         string   `yaml:"head_commit" json:"head_commit"`
	State              string   `yaml:"state" json:"state"`
	ExternalID         string   `yaml:"external_id,omitempty" json:"external_id,omitempty"`
	URL                string   `yaml:"url,omitempty" json:"url,omitempty"`
	PublicationAllowed bool     `yaml:"publication_allowed" json:"publication_allowed"`
}
type LiveTest struct {
	Choice          string `yaml:"choice" json:"choice"`
	Reason          string `yaml:"reason" json:"reason"`
	Environment     string `yaml:"environment" json:"environment"`
	HeadCommit      string `yaml:"head_commit" json:"head_commit"`
	AcceptedHandoff string `yaml:"accepted_handoff,omitempty" json:"accepted_handoff,omitempty"`
}
type Release struct {
	UserConfirmed bool       `yaml:"user_confirmed" json:"user_confirmed"`
	Reference     string     `yaml:"reference" json:"reference"`
	HeadCommit    string     `yaml:"head_commit" json:"head_commit"`
	ConfirmedAt   *time.Time `yaml:"confirmed_at,omitempty" json:"confirmed_at,omitempty"`
}
