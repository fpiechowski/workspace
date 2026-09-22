package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id,omitempty"`
	Title         string    `json:"title,omitempty"`
	Directory     string    `json:"directory"`
	Status        string    `json:"status"`
	Phase         string    `json:"phase,omitempty"`
	InputSource   string    `json:"input_source,omitempty"`
	IssueID       string    `json:"issue_id,omitempty"`
	IssueTitle    string    `json:"issue_title,omitempty"`
	IssueRevision int       `json:"issue_revision,omitempty"`
	IssueDigest   string    `json:"issue_digest,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	ActiveRuns    int       `json:"active_runs"`
	Problems      int       `json:"problems"`
	Revision      int       `json:"revision"`
	Error         string    `json:"error,omitempty"`
}

type ProjectOverview struct {
	ProjectRoot string             `json:"project_root"`
	ProjectID   string             `json:"project_id"`
	Workspaces  []WorkspaceSummary `json:"workspaces"`
	Issues      []IssueSummary     `json:"issues"`
	Dispatcher  DispatcherSummary  `json:"dispatcher"`
	ObservedAt  time.Time          `json:"observed_at"`
}

// ProjectOverview tolerates a damaged workspace alongside healthy entries.
// List retains its existing fail-fast contract for CLI compatibility.
func (s *Service) ProjectOverview(ctx context.Context) (ProjectOverview, error) {
	var out ProjectOverview
	err := withProjectReadLock(ctx, s.Root, func() error {
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
			entries = nil
		} else if err != nil {
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
			row.Title, row.Status, row.CreatedAt, row.Revision = doc.State.Title, doc.State.Status, doc.State.CreatedAt, doc.State.Revision
			row.InputSource = doc.State.Input.Source
			row.IssueID, row.IssueRevision, row.IssueDigest = doc.State.Input.IssueID, doc.State.Input.IssueRevision, doc.State.Input.IssueDigest
			row.Phase = doc.State.PhaseLabel()
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
				IssueID:     status.Workspace.Input.IssueID, IssueRevision: status.Workspace.Input.IssueRevision, IssueDigest: status.Workspace.Input.IssueDigest,
				CreatedAt: status.Workspace.CreatedAt,
				Revision:  status.Workspace.Revision,
			}
			row.Phase = status.Workspace.PhaseLabel()
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
		if err := recoverIssueWrite(s.Root); err != nil {
			return err
		}
		issueEntries, err := os.ReadDir(issueRoot(s.Root))
		if errors.Is(err, os.ErrNotExist) {
			issueEntries = nil
		} else if err != nil {
			return err
		}
		for _, entry := range issueEntries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "issue_") {
				continue
			}
			row := IssueSummary{ID: entry.Name(), Status: "unknown"}
			file, openErr := os.Open(filepath.Join(issueDir(s.Root, entry.Name()), "ISSUE.md"))
			if openErr != nil {
				row.Error = openErr.Error()
				out.Issues = append(out.Issues, row)
				continue
			}
			i, parseErr := parseIssueHeader(file)
			_ = file.Close()
			if parseErr != nil {
				row.Error = parseErr.Error()
				out.Issues = append(out.Issues, row)
				continue
			}
			row = issueSummary(i)
			for _, workspace := range out.Workspaces {
				if workspace.Error == "" && workspace.IssueID == row.ID {
					row.LinkedWorkspaces = append(row.LinkedWorkspaces, IssueWorkspaceLink{WorkspaceID: workspace.ID, Title: workspace.Title, Status: workspace.Status, IssueRevision: workspace.IssueRevision, IssueDigest: workspace.IssueDigest})
				}
			}
			row.LinkedWorkspaceCount = len(row.LinkedWorkspaces)
			row.LinkedStateSummary = linkedStateSummary(row.LinkedWorkspaces)
			out.Issues = append(out.Issues, row)
		}
		issueTitles := make(map[string]string, len(out.Issues))
		for _, issue := range out.Issues {
			if issue.Error == "" {
				issueTitles[issue.ID] = issue.Title
			}
		}
		for i := range out.Workspaces {
			if out.Workspaces[i].IssueTitle == "" {
				out.Workspaces[i].IssueTitle = issueTitles[out.Workspaces[i].IssueID]
			}
		}
		sort.SliceStable(out.Issues, func(i, j int) bool {
			if !out.Issues[i].UpdatedAt.Equal(out.Issues[j].UpdatedAt) {
				return out.Issues[i].UpdatedAt.After(out.Issues[j].UpdatedAt)
			}
			return out.Issues[i].ID < out.Issues[j].ID
		})
		out.Dispatcher = dispatcherSummaryLocked(s.Root, cfg.ProjectID)
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

// withProjectReadLock preserves the read-only query contract for a fresh
// project: if no writer lock exists yet, observing state must not create the
// .workspace/.runtime directory merely to acquire a lock. Once a writer has
// established the lock, readers coordinate with it normally. An interrupted
// Issue write is treated as a recovery write and therefore takes the lock.
func withProjectReadLock(ctx context.Context, root string, fn func() error) error {
	lockPath := filepath.Join(root, ".workspace", ".runtime", "project.lock")
	if _, err := os.Stat(lockPath); errors.Is(err, os.ErrNotExist) {
		if _, pendingErr := os.Stat(issuePendingPath(root)); pendingErr == nil {
			return withProjectLock(ctx, root, fn)
		}
		return fn()
	} else if err != nil {
		return err
	}
	return withProjectLock(ctx, root, fn)
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
