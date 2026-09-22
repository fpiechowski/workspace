package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) replayWorkspaceCreate(ctx context.Context, opt CreateOptions) (Status, bool, error) {
	var out Status
	found := false
	err := withProjectLock(ctx, s.Root, func() error {
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		storage, err := s.storageRoot(cfg)
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(storage)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "ws_") {
				continue
			}
			d, loadErr := loadDocument(filepath.Join(storage, entry.Name()))
			if loadErr != nil {
				continue
			}
			if d.State.ProjectID != cfg.ProjectID {
				continue
			}
			id, previousErr := d.previous("create:"+opt.OperationKey, normalizedCreateReplayRequest(opt))
			if previousErr != nil {
				// A URL-only create is normalized into a first-class Issue before
				// the workspace receipt is written. Its durable payload contains
				// the fetched snapshot, while a retry intentionally contains only
				// the URL; the stable operation key is still the replay authority.
				if opt.Source == "" || strings.TrimSpace(opt.Input) != "" {
					return previousErr
				}
				id = d.Registry.Operations["create:"+opt.OperationKey].ResourceID
				if id == "" {
					return previousErr
				}
			}
			if id == "" {
				continue
			}
			if replayed, unmarshalErr := replayResource(d, "create:"+opt.OperationKey, &out); replayed {
				found = true
				return unmarshalErr
			}
			out = d.Status()
			found = true
			return nil
		}
		return nil
	})
	return out, found, err
}

func normalizedCreateReplayRequest(opt CreateOptions) CreateOptions {
	if opt.Title == "" {
		opt.Title = "Untitled issue"
	}
	if opt.Base == "" {
		opt.Base = "HEAD"
	}
	return opt
}

func issueWorkspaceSnapshot(i Issue) string {
	retrieved := ""
	if i.RetrievedAt != nil {
		retrieved = i.RetrievedAt.UTC().Format(time.RFC3339)
	} else {
		retrieved = i.UpdatedAt.UTC().Format(time.RFC3339)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", i.Title)
	if i.Source != "" {
		fmt.Fprintf(&b, "Source: %s\n", i.Source)
	}
	fmt.Fprintf(&b, "Retrieved: %s\nRevision: %d\nDigest: %s\n\n", retrieved, i.Revision, i.Digest)
	b.WriteString(normalizeIssueBody(i.Body))
	b.WriteByte('\n')
	return b.String()
}

// CreateFromIssue copies the exact durable Issue revision selected by the
// caller. Later Issue refreshes never alter the Workspace input.
func (s *Service) CreateFromIssue(ctx context.Context, issueID string, opt CreateOptions) (Status, error) {
	if err := s.authorizeProjectActor(ctx); err != nil {
		return Status{}, err
	}
	original := opt.OperationRequest
	if original == nil {
		original = normalizedCreateReplayRequest(opt)
	}
	i, err := s.ShowIssue(ctx, issueID)
	if err != nil {
		return Status{}, err
	}
	if opt.IssueRevision > 0 && opt.IssueRevision != i.Revision {
		return Status{}, fail("revision_conflict", "expected Issue revision %d, current %d", opt.IssueRevision, i.Revision)
	}
	if opt.IssueDigest != "" && opt.IssueDigest != i.Digest {
		return Status{}, fail("revision_conflict", "expected Issue digest %q, current %q", opt.IssueDigest, i.Digest)
	}
	if opt.Title == "" {
		opt.Title = i.Title
	}
	opt.Input = issueWorkspaceSnapshot(i.Issue)
	opt.Source = i.Source
	opt.IssueID = issueID
	opt.IssueRevision = i.Revision
	opt.IssueDigest = i.Digest
	opt.OperationRequest = original
	// Create is also the compatibility path for free-form workspaces; clear
	// the dispatch selector before entering that implementation.
	opt.FromIssue, opt.IssueID = "", ""
	return s.createWorkspace(ctx, opt, issueID, i.Revision, i.Digest)
}

// CreateFromIssueURL first resolves a URL to one durable Issue, then creates a
// frozen Workspace link. A supplied body is intentionally treated as an
// offline snapshot and does not invoke the tracker.
func (s *Service) CreateFromIssueURL(ctx context.Context, opt CreateOptions) (Status, error) {
	operationRequest := normalizedCreateReplayRequest(opt)
	intakeKey := ""
	if opt.OperationKey != "" {
		intakeKey = "workspace-create-issue:" + opt.OperationKey
	}
	if strings.TrimSpace(opt.Input) != "" {
		issue, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: opt.Title, Body: opt.Input, Source: opt.Source, OperationKey: intakeKey})
		if err != nil {
			return Status{}, err
		}
		opt.Title = firstNonempty(opt.Title, issue.Title)
		opt.Input = ""
		opt.OperationRequest = operationRequest
		return s.CreateFromIssue(ctx, issue.ID, opt)
	}
	issue, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: opt.Title, Source: opt.Source, OperationKey: intakeKey})
	if err != nil {
		return Status{}, err
	}
	opt.OperationRequest = operationRequest
	return s.CreateFromIssue(ctx, issue.ID, opt)
}

func (s *Service) createWorkspace(ctx context.Context, opt CreateOptions, issueID string, issueRevision int, issueDigest string) (Status, error) {
	opt.IssueID, opt.IssueRevision, opt.IssueDigest = issueID, issueRevision, issueDigest
	if issueID == "" {
		return s.Create(ctx, opt)
	}
	// Avoid the CreateFromIssue selector branch while retaining the existing
	// workspace creation idempotency and workflow/base semantics.
	opt.FromIssue, opt.IssueID = "", ""
	return s.createWorkspaceDocument(ctx, opt, issueID, issueRevision, issueDigest)
}

// createWorkspaceDocument is the pre-existing Create implementation with
// optional Issue relationship fields supplied by the first-class path.
func (s *Service) createWorkspaceDocument(ctx context.Context, opt CreateOptions, issueID string, issueRevision int, issueDigest string) (Status, error) {
	// The body of Create remains the compatibility implementation. This helper
	// is replaced below by a small trampoline so both paths share one writer.
	return s.createWorkspaceLegacy(ctx, opt, issueID, issueRevision, issueDigest)
}

func (s *Service) DispatchIssue(ctx context.Context, opt IssueDispatchOptions) (DispatchResult, error) {
	if opt.IssueID == "" {
		return DispatchResult{}, fail("issue_required", "provide an Issue ID")
	}
	if opt.NoWorkflow && opt.Workflow != "" {
		return DispatchResult{}, fail("invalid_option", "--workflow and --no-workflow are mutually exclusive")
	}
	if err := s.authorizeProjectActor(ctx); err != nil {
		return DispatchResult{}, err
	}
	if opt.OperationKey != "" {
		return projectEffect(ctx, s.Root, "issue.dispatch:"+opt.OperationKey, opt, func() (DispatchResult, error) {
			return s.dispatchIssue(ctx, opt)
		})
	}
	return s.dispatchIssue(ctx, opt)
}

func (s *Service) dispatchIssue(ctx context.Context, opt IssueDispatchOptions) (DispatchResult, error) {
	issue, err := s.ShowIssue(ctx, opt.IssueID)
	if err != nil {
		return DispatchResult{}, err
	}
	workspaceService := *s
	// The project actor is authorized for the aggregate operation. The narrow
	// Workspace use case is invoked as an internal user-scoped step, without
	// granting the Dispatcher access to general Workspace mutations.
	workspaceService.Actor = Actor{}
	createKey := ""
	if opt.OperationKey != "" {
		createKey = "dispatch:create:" + opt.OperationKey
	}
	workspace, err := workspaceService.CreateFromIssue(ctx, opt.IssueID, CreateOptions{Title: opt.Title, Workflow: opt.Workflow, NoWorkflow: opt.NoWorkflow, Base: opt.Base, OperationKey: createKey})
	if err != nil {
		return DispatchResult{}, err
	}
	result := DispatchResult{Issue: issueSummary(issue.Issue), Workspace: workspace, Started: false}
	if opt.Start {
		startKey := ""
		if opt.OperationKey != "" {
			startKey = "dispatch:start:" + opt.OperationKey
		}
		session, err := workspaceService.StartSupervisedOrchestrator(ctx, workspace.Workspace.ID, startKey)
		if err != nil {
			return result, err
		}
		result.Started, result.Orchestrator = true, session
	}
	return result, nil
}
