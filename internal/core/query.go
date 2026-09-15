package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// WorkspaceSnapshot is the typed, private read model shared by the CLI and
// interactive interfaces. It is deliberately separate from Status so the
// public JSON schema remains unchanged.
type WorkspaceSnapshot struct {
	Status     Status              `json:"status"`
	Body       string              `json:"body"`
	Services   []BackgroundService `json:"services"`
	Checks     []CheckReceipt      `json:"checks"`
	Handoffs   []Handoff           `json:"handoffs"`
	Messages   []Message           `json:"messages"`
	Relations  WorkspaceRelations  `json:"relations"`
	Metrics    WorkspaceMetrics    `json:"metrics"`
	ObservedAt time.Time           `json:"observed_at"`
}

// WorkspaceSnapshot reads the document and its runtime index once, under the
// project lock. It does not inspect tmux, start a supervisor, or expose mutable
// operation journals.
func (s *Service) WorkspaceSnapshot(ctx context.Context, selector string) (WorkspaceSnapshot, error) {
	var out WorkspaceSnapshot
	err := s.withReadableWorkspace(ctx, selector, func(d *Document) error {
		out = WorkspaceSnapshot{
			Status:     d.Status(),
			Body:       d.Body,
			Services:   append([]BackgroundService(nil), d.Registry.Services...),
			Checks:     append([]CheckReceipt(nil), d.Registry.Checks...),
			Handoffs:   append([]Handoff(nil), d.Registry.Handoffs...),
			Messages:   append([]Message(nil), d.Registry.Messages...),
			Relations:  buildWorkspaceRelations(d),
			Metrics:    workspaceMetrics(d),
			ObservedAt: nowUTC(),
		}
		return nil
	})
	return out, err
}

// WorkspaceSummary is a light project-picker row. Error is populated for an
// unreadable workspace directory without hiding healthy siblings.
type WorkspaceSummary struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id,omitempty"`
	Title       string    `json:"title,omitempty"`
	Directory   string    `json:"directory"`
	Status      string    `json:"status"`
	Phase       string    `json:"phase,omitempty"`
	InputSource string    `json:"input_source,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	ActiveRuns  int       `json:"active_runs"`
	Problems    int       `json:"problems"`
	Error       string    `json:"error,omitempty"`
}

type ProjectOverview struct {
	ProjectRoot string             `json:"project_root"`
	ProjectID   string             `json:"project_id"`
	Workspaces  []WorkspaceSummary `json:"workspaces"`
	ObservedAt  time.Time          `json:"observed_at"`
}

// ProjectOverview tolerates a damaged workspace alongside healthy entries.
// List retains its existing fail-fast contract for CLI compatibility.
func (s *Service) ProjectOverview(ctx context.Context) (ProjectOverview, error) {
	var out ProjectOverview
	err := withProjectLock(ctx, s.Root, func() error {
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		root, err := s.storageRoot(cfg)
		if err != nil {
			return err
		}
		out = ProjectOverview{ProjectRoot: s.Root, ProjectID: cfg.ProjectID, ObservedAt: nowUTC()}
		entries, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || len(entry.Name()) < 3 || entry.Name()[:3] != "ws_" {
				continue
			}
			dir := filepath.Join(root, entry.Name())
			row := WorkspaceSummary{ID: entry.Name(), Directory: dir, Status: "unknown"}
			frontmatter, readErr := os.ReadFile(filepath.Join(dir, "WORKSPACE.md"))
			if readErr != nil {
				row.Error = readErr.Error()
				out.Workspaces = append(out.Workspaces, row)
				continue
			}
			doc := &Document{Dir: dir}
			decodeErr := decodeDocument(frontmatter, doc)
			if decodeErr != nil {
				if doc.State.ProjectID != "" && doc.State.ProjectID != cfg.ProjectID {
					continue
				}
				if doc.State.ID != "" {
					row.ID = doc.State.ID
				}
				row.Error = decodeErr.Error()
				out.Workspaces = append(out.Workspaces, row)
				continue
			}
			if doc.State.ProjectID != cfg.ProjectID {
				continue
			}
			row.ID, row.ProjectID = doc.State.ID, doc.State.ProjectID
			row.Title, row.Status, row.CreatedAt = doc.State.Title, doc.State.Status, doc.State.CreatedAt
			row.InputSource = doc.State.Input.Source
			if doc.State.Workflow != nil {
				row.Phase = doc.State.Workflow.Phase
			}
			doc, err := loadDocument(dir)
			if err != nil {
				row.Error = err.Error()
				out.Workspaces = append(out.Workspaces, row)
				continue
			}
			status := doc.Status()
			row = WorkspaceSummary{
				ID:          status.Workspace.ID,
				ProjectID:   status.Workspace.ProjectID,
				Title:       status.Workspace.Title,
				Directory:   status.Directory,
				Status:      status.Workspace.Status,
				InputSource: status.Workspace.Input.Source,
				CreatedAt:   status.Workspace.CreatedAt,
			}
			if status.Workspace.Workflow != nil {
				row.Phase = status.Workspace.Workflow.Phase
			}
			for _, run := range status.Runs {
				if !run.Active() {
					continue
				}
				if session, findErr := findSession(doc, run.SessionID); findErr == nil && session.CurrentRunID == run.ID {
					row.ActiveRuns++
				}
			}
			row.Problems = workspaceMetrics(doc).ProblemCount
			out.Workspaces = append(out.Workspaces, row)
		}
		sort.Slice(out.Workspaces, func(i, j int) bool {
			a, b := out.Workspaces[i], out.Workspaces[j]
			if (a.Status == "archived") != (b.Status == "archived") {
				return b.Status == "archived"
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.After(b.CreatedAt)
			}
			return a.ID < b.ID
		})
		return nil
	})
	return out, err
}

func withProjectLock(ctx context.Context, root string, fn func() error) error {
	unlock, err := lockProject(ctx, root)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

func (s *Service) withReadableWorkspace(ctx context.Context, selector string, fn func(*Document) error) error {
	unlock, err := lockProject(ctx, s.Root)
	if err != nil {
		return err
	}
	defer unlock()
	dir, err := s.resolveReadableWorkspace(selector)
	if err != nil {
		return err
	}
	doc, err := loadDocument(dir)
	if err != nil {
		return err
	}
	return fn(doc)
}

// resolveReadableWorkspace skips malformed sibling records while resolving an
// explicit ID, allowing the TUI to open a healthy workspace next to a damaged
// one. List and the legacy With path keep their fail-fast semantics.
func (s *Service) resolveReadableWorkspace(selector string) (string, error) {
	cfg, err := s.Config()
	if err != nil {
		return "", err
	}
	root, err := s.storageRoot(cfg)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return "", fail("workspace_not_found", "unknown workspace %q", selector)
	}
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() || len(entry.Name()) < 3 || entry.Name()[:3] != "ws_" {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if entry.Name() == selector {
			b, readErr := os.ReadFile(filepath.Join(dir, "WORKSPACE.md"))
			if readErr != nil {
				return "", readErr
			}
			doc := &Document{}
			if decodeErr := decodeDocument(b, doc); decodeErr != nil {
				if doc.State.ProjectID != "" && doc.State.ProjectID != cfg.ProjectID {
					continue
				}
				return "", decodeErr
			}
			if doc.State.ProjectID == cfg.ProjectID {
				return dir, nil
			}
			continue
		}
		b, readErr := os.ReadFile(filepath.Join(dir, "WORKSPACE.md"))
		if readErr != nil {
			continue
		}
		doc := &Document{}
		if decodeDocument(b, doc) == nil && doc.State.ProjectID == cfg.ProjectID && doc.State.ID == selector {
			return dir, nil
		}
	}
	return "", fail("workspace_not_found", "unknown workspace %q", selector)
}
