package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const uiSchemaVersion = 1

type UIStatus struct {
	OperationID string     `json:"operation_id,omitempty"`
	WorkspaceID string     `json:"workspace_id"`
	Desired     bool       `json:"desired"`
	UIID        string     `json:"ui_id,omitempty"`
	Generation  int        `json:"generation"`
	State       string     `json:"state"`
	PaneID      string     `json:"pane_id,omitempty"`
	WindowID    string     `json:"window_id,omitempty"`
	AnchorRunID string     `json:"anchor_run_id,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	Failures    int        `json:"failures"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type uiReceipt struct {
	Digest    string    `json:"digest"`
	Operation string    `json:"operation_id"`
	Desired   bool      `json:"desired"`
	Result    UIStatus  `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

type uiRecord struct {
	SchemaVersion int                  `json:"schema_version"`
	WorkspaceID   string               `json:"workspace_id"`
	ProjectID     string               `json:"project_id"`
	Desired       bool                 `json:"desired"`
	UIID          string               `json:"ui_id"`
	Generation    int                  `json:"generation"`
	LaunchToken   string               `json:"launch_token,omitempty"`
	State         string               `json:"state"`
	PaneID        string               `json:"pane_id,omitempty"`
	WindowID      string               `json:"window_id,omitempty"`
	AnchorRunID   string               `json:"anchor_run_id,omitempty"`
	LastError     string               `json:"last_error,omitempty"`
	Failures      int                  `json:"failures"`
	NextRetryAt   *time.Time           `json:"next_retry_at,omitempty"`
	StartedAt     *time.Time           `json:"started_at,omitempty"`
	UpdatedAt     time.Time            `json:"updated_at"`
	ExitRequested bool                 `json:"exit_requested,omitempty"`
	Receipts      map[string]uiReceipt `json:"receipts,omitempty"`
}

type ManagedUIRuntime interface {
	LaunchUI(context.Context, UILaunch) (Pane, error)
	StopUI(context.Context, UILaunch, string) error
}

type UILaunch struct {
	ProjectRoot, ProjectID, WorkspaceID, WorkspaceDir string
	Executable, Socket, WindowID, UIID, Token         string
	Generation                                        int
}

func uiStatePath(workspaceDir string) string {
	return filepath.Join(workspaceDir, ".runtime", "ui.json")
}

func readUIRecord(dir, workspaceID, projectID string) (uiRecord, bool, error) {
	record := uiRecord{
		SchemaVersion: uiSchemaVersion,
		WorkspaceID:   workspaceID,
		ProjectID:     projectID,
		Desired:       false,
		State:         "disabled",
		UpdatedAt:     nowUTC(),
		Receipts:      map[string]uiReceipt{},
	}
	b, err := os.ReadFile(uiStatePath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return record, false, nil
	}
	if err != nil {
		return uiRecord{}, false, err
	}
	var stored uiRecord
	if err := json.Unmarshal(b, &stored); err != nil {
		return uiRecord{}, true, fail("ui_state_invalid", "cannot decode %s: %v", uiStatePath(dir), err)
	}
	if stored.SchemaVersion != uiSchemaVersion {
		return uiRecord{}, true, fail("ui_state_invalid", "unsupported UI state schema %d", stored.SchemaVersion)
	}
	if stored.WorkspaceID != workspaceID || stored.ProjectID != projectID || stored.UIID == "" || stored.Generation < 0 {
		return uiRecord{}, true, fail("ui_state_invalid", "UI state identity does not match workspace %s", workspaceID)
	}
	if stored.Receipts == nil {
		stored.Receipts = map[string]uiReceipt{}
	}
	return stored, true, nil
}

func writeUIRecord(dir string, record uiRecord) error {
	b, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(uiStatePath(dir), append(b, '\n'))
}

func uiStatus(record uiRecord) UIStatus {
	return UIStatus{
		WorkspaceID: record.WorkspaceID,
		Desired:     record.Desired,
		UIID:        record.UIID,
		Generation:  record.Generation,
		State:       record.State,
		PaneID:      record.PaneID,
		WindowID:    record.WindowID,
		AnchorRunID: record.AnchorRunID,
		LastError:   record.LastError,
		Failures:    record.Failures,
		NextRetryAt: record.NextRetryAt,
		StartedAt:   record.StartedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}

func (s *Service) UIPaneStatus(ctx context.Context, selector string) (UIStatus, error) {
	var out UIStatus
	err := s.withReadableWorkspace(ctx, selector, func(d *Document) error {
		projectID := d.State.ProjectID
		if projectID == "" {
			cfg, err := s.Config()
			if err != nil {
				return err
			}
			projectID = cfg.ProjectID
		}
		record, exists, err := readUIRecord(d.Dir, d.State.ID, projectID)
		if err != nil {
			return err
		}
		if !exists {
			if d.State.Status == "archived" {
				record.Desired, record.State = false, "disabled"
			}
		}
		out = uiStatus(record)
		return nil
	})
	return out, err
}

func (s *Service) SetUIPaneDesired(ctx context.Context, selector string, desired bool, key string) (UIStatus, error) {
	return s.setUIPaneDesired(ctx, selector, desired, key, false, false)
}

// HideManagedUIPane records q's durable hide intent. The managed process exits
// after Bubble Tea restores the terminal; tmux then closes its pane.
func (s *Service) HideManagedUIPane(ctx context.Context, selector, key string) error {
	_, err := s.setUIPaneDesired(ctx, selector, false, key, true, false)
	return err
}

func (s *Service) setUIPaneDesired(ctx context.Context, selector string, desired bool, key string, deferCleanup, exitRequested bool) (UIStatus, error) {
	if err := s.requireUserFromWorkspace(ctx, selector); err != nil {
		return UIStatus{}, err
	}
	if key == "" {
		key = ID("tuiop")
	}
	digestValue := payloadDigest(struct {
		Operation string
		Desired   bool
	}{"ui.desired", desired})
	var result UIStatus
	var replay bool
	err := withProjectLock(ctx, s.Root, func() error {
		dir, err := s.resolveReadableWorkspace(selector)
		if err != nil {
			return err
		}
		doc, err := loadDocument(dir)
		if err != nil {
			return err
		}
		if err := s.requireUser(doc); err != nil {
			return err
		}
		record, exists, err := readUIRecord(dir, doc.State.ID, doc.State.ProjectID)
		if err != nil {
			return err
		}
		if !exists {
			record.UIID = ID("ui")
		}
		if receipt, ok := record.Receipts[key]; ok {
			if receipt.Digest != digestValue || receipt.Desired != desired {
				return fail("operation_conflict", "operation key %q was already used with another UI intent", key)
			}
			result = receipt.Result
			if result.OperationID == "" {
				result.OperationID = receipt.Operation
			}
			replay = true
			return nil
		}
		record.Desired = desired
		record.ExitRequested = exitRequested
		if desired {
			record.Failures = 0
			record.NextRetryAt = nil
			record.LastError = ""
			if record.State == "disabled" || record.State == "backoff" {
				record.State = "waiting_for_runtime"
			}
		} else if !exitRequested {
			record.State = "disabled"
		}
		record.UpdatedAt = nowUTC()
		receipt := uiReceipt{Digest: digestValue, Operation: ID("op"), Desired: desired, CreatedAt: nowUTC()}
		receipt.Result = uiStatus(record)
		receipt.Result.OperationID = receipt.Operation
		record.Receipts[key] = receipt
		pruneUIReceipts(record.Receipts)
		if err := writeUIRecord(dir, record); err != nil {
			return err
		}
		result = receipt.Result
		return nil
	})
	if err != nil {
		return UIStatus{}, err
	}
	if replay {
		return result, nil
	}
	if !deferCleanup {
		if err := s.ReconcileInterface(ctx, selector); err != nil {
			result.LastError = err.Error()
		}
	}
	current, statusErr := s.UIPaneStatus(ctx, selector)
	if statusErr == nil {
		result = current
		// A receipt is a stable result of intent replay, separate from live status.
		result.OperationID = resultOperationID(ctx, s, selector, key)
	}
	if result.OperationID != "" {
		_ = s.storeUIReceiptResult(ctx, selector, key, result)
	}
	return result, statusErr
}

func (s *Service) requireUserFromWorkspace(ctx context.Context, selector string) error {
	return s.withReadableWorkspace(ctx, selector, func(d *Document) error { return s.requireUser(d) })
}

func (s *Service) storeUIReceiptResult(ctx context.Context, selector, key string, result UIStatus) error {
	return withProjectLock(ctx, s.Root, func() error {
		dir, err := s.resolveReadableWorkspace(selector)
		if err != nil {
			return err
		}
		doc, err := loadDocument(dir)
		if err != nil {
			return err
		}
		record, exists, err := readUIRecord(dir, doc.State.ID, doc.State.ProjectID)
		if err != nil || !exists {
			return err
		}
		receipt, ok := record.Receipts[key]
		if !ok {
			return nil
		}
		result.OperationID = receipt.Operation
		receipt.Result = result
		record.Receipts[key] = receipt
		return writeUIRecord(dir, record)
	})
}

func resultOperationID(ctx context.Context, s *Service, selector, key string) string {
	var operationID string
	_ = s.withReadableWorkspace(ctx, selector, func(d *Document) error {
		record, exists, err := readUIRecord(d.Dir, d.State.ID, d.State.ProjectID)
		if err != nil || !exists {
			return err
		}
		operationID = record.Receipts[key].Operation
		return nil
	})
	return operationID
}

func pruneUIReceipts(receipts map[string]uiReceipt) {
	if len(receipts) <= 64 {
		return
	}
	type entry struct {
		key string
		at  time.Time
	}
	items := make([]entry, 0, len(receipts))
	for key, receipt := range receipts {
		items = append(items, entry{key, receipt.CreatedAt})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].at.Before(items[j].at) })
	for _, item := range items[:len(items)-64] {
		delete(receipts, item.key)
	}
}

func hasOrchestratorHistory(d *Document) bool {
	if d.State.OrchestratorAgentID == "" {
		return false
	}
	for _, session := range d.Registry.Sessions {
		if session.AgentID == d.State.OrchestratorAgentID {
			return true
		}
	}
	return false
}

func (s *Service) ClaimUIPane(ctx context.Context, selector, uiID string, generation int, token, paneID string) error {
	if uiID == "" || token == "" || paneID == "" {
		return fail("ui_claim_invalid", "UI identity and current tmux pane are required")
	}
	if s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "" {
		return fail("forbidden", "managed UI runner must not inherit an agent actor")
	}
	return withProjectLock(ctx, s.Root, func() error {
		dir, err := s.resolveReadableWorkspace(selector)
		if err != nil {
			return err
		}
		doc, err := loadDocument(dir)
		if err != nil {
			return err
		}
		record, exists, err := readUIRecord(dir, doc.State.ID, doc.State.ProjectID)
		if err != nil {
			return err
		}
		if !exists || doc.State.Status == "archived" || !record.Desired || record.UIID != uiID || record.Generation != generation || record.LaunchToken != token {
			return fail("ui_claim_stale", "managed UI launch is no longer desired")
		}
		if record.State == "running" {
			return fail("ui_claim_conflict", "managed UI generation already has a claimant")
		}
		pane, err := s.Runtime.Inspect(ctx, paneID)
		if err != nil {
			return err
		}
		if pane.Dead || pane.ID != paneID || pane.Kind != "tui" || pane.WorkspaceID != doc.State.ID || pane.UIID != uiID || pane.UIGeneration != generation || pane.UIToken != token {
			return fail("pane_mismatch", "current pane does not match the managed UI launch identity")
		}
		record.State = "running"
		record.PaneID, record.WindowID = pane.ID, pane.WindowID
		record.LastError = ""
		record.UpdatedAt = nowUTC()
		started := record.UpdatedAt
		record.StartedAt = &started
		return writeUIRecord(dir, record)
	})
}
