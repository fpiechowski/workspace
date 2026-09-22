package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const issueSchemaVersion = 1

// Issue is a durable project input. Body is the trusted local snapshot of the
// retrieved/manual description; Source is metadata only and is never used as
// an instruction to a client.
type Issue struct {
	SchemaVersion int        `json:"schema_version" yaml:"schema_version"`
	ID            string     `json:"id" yaml:"id"`
	ProjectID     string     `json:"project_id" yaml:"project_id"`
	Revision      int        `json:"revision" yaml:"revision"`
	Title         string     `json:"title" yaml:"title"`
	Source        string     `json:"source,omitempty" yaml:"source,omitempty"`
	Status        string     `json:"status" yaml:"status"`
	StatusReason  string     `json:"status_reason,omitempty" yaml:"status_reason,omitempty"`
	Digest        string     `json:"digest" yaml:"digest"`
	CreatedAt     time.Time  `json:"created_at" yaml:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" yaml:"updated_at"`
	RetrievedAt   *time.Time `json:"retrieved_at,omitempty" yaml:"retrieved_at,omitempty"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty" yaml:"last_checked_at,omitempty"`
	Body          string     `json:"body" yaml:"body"`
}

type IssueWorkspaceLink struct {
	WorkspaceID   string `json:"workspace_id" yaml:"workspace_id"`
	Title         string `json:"title" yaml:"title"`
	Status        string `json:"status" yaml:"status"`
	IssueRevision int    `json:"issue_revision" yaml:"issue_revision"`
	IssueDigest   string `json:"issue_digest" yaml:"issue_digest"`
}

// IssueSummary intentionally omits Body so periodic project refreshes remain
// bounded even when an external tracker returned a very large description.
type IssueSummary struct {
	ID                   string               `json:"id" yaml:"id"`
	ProjectID            string               `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Revision             int                  `json:"revision" yaml:"revision"`
	Title                string               `json:"title,omitempty" yaml:"title,omitempty"`
	Source               string               `json:"source,omitempty" yaml:"source,omitempty"`
	Status               string               `json:"status" yaml:"status"`
	StatusReason         string               `json:"status_reason,omitempty" yaml:"status_reason,omitempty"`
	Digest               string               `json:"digest,omitempty" yaml:"digest,omitempty"`
	CreatedAt            time.Time            `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt            time.Time            `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	RetrievedAt          *time.Time           `json:"retrieved_at,omitempty" yaml:"retrieved_at,omitempty"`
	LastCheckedAt        *time.Time           `json:"last_checked_at,omitempty" yaml:"last_checked_at,omitempty"`
	LinkedWorkspaceCount int                  `json:"linked_workspace_count" yaml:"linked_workspace_count"`
	LinkedWorkspaces     []IssueWorkspaceLink `json:"linked_workspaces,omitempty" yaml:"linked_workspaces,omitempty"`
	LinkedStateSummary   string               `json:"linked_state_summary,omitempty" yaml:"linked_state_summary,omitempty"`
	Error                string               `json:"error,omitempty" yaml:"error,omitempty"`
}

type IssueDetail struct {
	Issue
	LinkedWorkspaces []IssueWorkspaceLink `json:"linked_workspaces,omitempty" yaml:"linked_workspaces,omitempty"`
}

type IssueCreateOptions struct {
	Title        string
	Body         string
	Source       string
	OperationKey string
}

type IssueUpdateOptions struct {
	Status           string
	Reason           string
	ExpectedRevision int
	OperationKey     string
}

type IssueRefreshOptions struct {
	ExpectedRevision int
	OperationKey     string
}

type IssueDispatchOptions struct {
	IssueID      string
	Workflow     string
	NoWorkflow   bool
	Base         string
	Title        string
	Start        bool
	OperationKey string
}

type DispatchResult struct {
	Issue        IssueSummary `json:"issue" yaml:"issue"`
	Workspace    Status       `json:"workspace" yaml:"workspace"`
	Started      bool         `json:"started" yaml:"started"`
	Orchestrator Session      `json:"orchestrator,omitempty" yaml:"orchestrator,omitempty"`
}

type issueFrontmatter struct {
	SchemaVersion int        `yaml:"schema_version" json:"schema_version"`
	ID            string     `yaml:"id" json:"id"`
	ProjectID     string     `yaml:"project_id" json:"project_id"`
	Revision      int        `yaml:"revision" json:"revision"`
	Title         string     `yaml:"title" json:"title"`
	Source        string     `yaml:"source,omitempty" json:"source,omitempty"`
	Status        string     `yaml:"status" json:"status"`
	StatusReason  string     `yaml:"status_reason,omitempty" json:"status_reason,omitempty"`
	Digest        string     `yaml:"digest" json:"digest"`
	CreatedAt     time.Time  `yaml:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `yaml:"updated_at" json:"updated_at"`
	RetrievedAt   *time.Time `yaml:"retrieved_at,omitempty" json:"retrieved_at,omitempty"`
	LastCheckedAt *time.Time `yaml:"last_checked_at,omitempty" json:"last_checked_at,omitempty"`
}

type issuePendingWrite struct {
	BeforeDigest string            `json:"before_digest"`
	IssueID      string            `json:"issue_id"`
	Current      []byte            `json:"current"`
	History      map[string][]byte `json:"history,omitempty"`
}

type issueReceipt struct {
	ID         string          `json:"id"`
	Digest     string          `json:"digest"`
	State      string          `json:"state"`
	ResourceID string          `json:"resource_id,omitempty"`
	Revision   int             `json:"revision,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}

func issueRoot(root string) string            { return filepath.Join(root, ".workspace", "issues") }
func issueDir(root, id string) string         { return filepath.Join(issueRoot(root), id) }
func issueCurrentPath(root, id string) string { return filepath.Join(issueDir(root, id), "ISSUE.md") }
func issuePendingPath(root string) string {
	return filepath.Join(issueRoot(root), ".runtime", "pending.json")
}

func validateIssueID(id string) error {
	if !strings.HasPrefix(id, "issue_") || len(id) <= len("issue_") || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return fail("invalid_issue", "issue ID must use the issue_ prefix")
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return fail("invalid_issue", "Issue ID contains an unsafe character")
		}
	}
	return nil
}

func validateIssueSource(source string) error {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(source))
	if err != nil || u.Host == "" || (strings.ToLower(u.Scheme) != "https" && strings.ToLower(u.Scheme) != "http") || u.User != nil {
		return fail("invalid_issue_url", "provide an HTTP(S) issue URL without credentials")
	}
	return nil
}

func canonicalIssueDigest(title, body, source string) string {
	payload := struct {
		Title  string `json:"title"`
		Body   string `json:"body"`
		Source string `json:"source,omitempty"`
	}{strings.TrimSpace(title), normalizeIssueBody(body), strings.TrimSpace(source)}
	b, _ := json.Marshal(payload)
	return digest(b)
}

func normalizeIssueBody(body string) string {
	return strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
}

func issueFrontmatterFrom(i Issue) issueFrontmatter {
	return issueFrontmatter{SchemaVersion: i.SchemaVersion, ID: i.ID, ProjectID: i.ProjectID, Revision: i.Revision, Title: i.Title, Source: i.Source, Status: i.Status, StatusReason: i.StatusReason, Digest: i.Digest, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt, RetrievedAt: i.RetrievedAt, LastCheckedAt: i.LastCheckedAt}
}

func issueFromFrontmatter(f issueFrontmatter, body string) Issue {
	return Issue{SchemaVersion: f.SchemaVersion, ID: f.ID, ProjectID: f.ProjectID, Revision: f.Revision, Title: f.Title, Source: f.Source, Status: f.Status, StatusReason: f.StatusReason, Digest: f.Digest, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt, RetrievedAt: f.RetrievedAt, LastCheckedAt: f.LastCheckedAt, Body: body}
}

func encodeIssue(i Issue) ([]byte, error) {
	head, err := yaml.Marshal(issueFrontmatterFrom(i))
	if err != nil {
		return nil, err
	}
	body := normalizeIssueBody(i.Body)
	if body != "" {
		body += "\n"
	}
	return []byte("---\n" + string(head) + "---\n\n" + body), nil
}

func parseIssueBytes(b []byte) (Issue, error) {
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Issue{}, fail("invalid_issue", "ISSUE.md has no YAML frontmatter")
	}
	head, body, ok := strings.Cut(text[4:], "\n---\n")
	if !ok {
		return Issue{}, fail("invalid_issue", "unclosed Issue YAML frontmatter")
	}
	var f issueFrontmatter
	if err := strictYAML([]byte(head), &f); err != nil {
		return Issue{}, fail("invalid_issue", "%v", err)
	}
	if err := validateIssueFrontmatter(f); err != nil {
		return Issue{}, err
	}
	body = strings.TrimSpace(body)
	if canonicalIssueDigest(f.Title, body, f.Source) != f.Digest {
		return Issue{}, fail("invalid_issue", "Issue digest does not match its title, source, and body")
	}
	return issueFromFrontmatter(f, body), nil
}

func validateIssueFrontmatter(f issueFrontmatter) error {
	if f.SchemaVersion != issueSchemaVersion || validateIssueID(f.ID) != nil || f.ProjectID == "" || f.Revision < 1 || f.Title == "" || f.Digest == "" {
		return fail("invalid_issue", "unsupported schema or missing Issue identity/revision/digest")
	}
	if f.Status != "open" && f.Status != "deferred" && f.Status != "closed" {
		return fail("invalid_issue", "unsupported local Issue status %q", f.Status)
	}
	return validateIssueSource(f.Source)
}

func readIssue(root, id string) (Issue, []byte, error) {
	if err := validateIssueID(id); err != nil {
		return Issue{}, nil, err
	}
	b, err := os.ReadFile(issueCurrentPath(root, id))
	if err != nil {
		if os.IsNotExist(err) {
			return Issue{}, nil, fail("issue_not_found", "unknown Issue %q", id)
		}
		return Issue{}, nil, err
	}
	i, err := parseIssueBytes(b)
	if err != nil {
		return Issue{}, nil, err
	}
	return i, b, nil
}

func recoverIssueWrite(root string) error {
	path := issuePendingPath(root)
	var pending issuePendingWrite
	if err := readJSON(path, &pending); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := validateIssueID(pending.IssueID); err != nil {
		return err
	}
	currentPath := issueCurrentPath(root, pending.IssueID)
	current, err := os.ReadFile(currentPath)
	if errors.Is(err, os.ErrNotExist) {
		current = nil
	} else if err != nil {
		return err
	}
	if pending.BeforeDigest != "" && digest(current) != pending.BeforeDigest && digest(current) != digest(pending.Current) {
		return fail("revision_conflict", "Issue %s changed during an interrupted write", pending.IssueID)
	}
	for name, data := range pending.History {
		if filepath.IsAbs(name) || !contained(issueDir(root, pending.IssueID), filepath.Join(issueDir(root, pending.IssueID), name)) {
			return fail("unsafe_path", "invalid Issue history path %s", name)
		}
		if err := atomicWrite(filepath.Join(issueDir(root, pending.IssueID), name), data); err != nil {
			return err
		}
	}
	if err := atomicWrite(currentPath, pending.Current); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func writeIssue(root string, before Issue, after Issue, preserveHistory bool) error {
	current, err := encodeIssue(after)
	if err != nil {
		return err
	}
	pending := issuePendingWrite{IssueID: after.ID, Current: current, History: map[string][]byte{}}
	if before.ID != "" {
		old, err := encodeIssue(before)
		if err != nil {
			return err
		}
		pending.BeforeDigest = digest(old)
		if preserveHistory {
			name := filepath.Join("history", fmt.Sprintf("revision-%06d", before.Revision), "ISSUE.md")
			pending.History[name] = old
		}
	}
	if err := writeJSON(issuePendingPath(root), pending); err != nil {
		return err
	}
	return recoverIssueWrite(root)
}

func issueSummary(i Issue) IssueSummary {
	return IssueSummary{ID: i.ID, ProjectID: i.ProjectID, Revision: i.Revision, Title: i.Title, Source: i.Source, Status: i.Status, StatusReason: i.StatusReason, Digest: i.Digest, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt, RetrievedAt: i.RetrievedAt, LastCheckedAt: i.LastCheckedAt}
}

func issueOperationPath(root, key string) string {
	return filepath.Join(issueRoot(root), ".runtime", "operations", strings.TrimPrefix(digest([]byte(key)), "sha256:")+".json")
}

func readIssueReceipt(root, key string, request any) (issueReceipt, bool, error) {
	receipt, exists, err := readIssueReceiptFile(root, key)
	if err != nil || !exists {
		return receipt, exists, err
	}
	if receipt.Digest != payloadDigest(request) {
		return issueReceipt{}, true, fail("operation_conflict", "operation key %q was already used with another payload", key)
	}
	return receipt, true, nil
}

func readIssueReceiptFile(root, key string) (issueReceipt, bool, error) {
	if strings.TrimSpace(key) == "" {
		return issueReceipt{}, false, nil
	}
	var receipt issueReceipt
	if err := readJSON(issueOperationPath(root, key), &receipt); errors.Is(err, os.ErrNotExist) {
		return issueReceipt{}, false, nil
	} else if err != nil {
		return issueReceipt{}, true, err
	}
	return receipt, true, nil
}

func beginIssueReceipt(root, key string, request any) (issueReceipt, error) {
	receipt := issueReceipt{ID: ID("op"), Digest: payloadDigest(request), State: "pending"}
	return receipt, writeJSON(issueOperationPath(root, key), receipt)
}

func completeIssueReceipt(root, key string, receipt issueReceipt, value any, revision int) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	receipt.State, receipt.Result, receipt.Revision = "completed", b, revision
	return writeJSON(issueOperationPath(root, key), receipt)
}

func (s *Service) issueRootReady() string { return issueRoot(s.Root) }

func (s *Service) issueSource(ctx context.Context, source string) (IssueContent, error) {
	if err := validateIssueSource(source); err != nil {
		return IssueContent{}, err
	}
	fetcher := s.IssueFetcher
	if fetcher == nil {
		cfg, err := s.Config()
		if err != nil {
			return IssueContent{}, err
		}
		fetcher = CLITracker{Root: s.Root, Config: cfg.Tracker}
	}
	return fetcher.Fetch(ctx, source)
}

func (s *Service) findIssueBySourceLocked(source string) (Issue, error) {
	entries, err := os.ReadDir(issueRoot(s.Root))
	if errors.Is(err, os.ErrNotExist) {
		return Issue{}, nil
	}
	if err != nil {
		return Issue{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "issue_") {
			continue
		}
		i, _, readErr := readIssue(s.Root, entry.Name())
		if readErr != nil {
			continue
		}
		if i.Source == source {
			return i, nil
		}
	}
	return Issue{}, nil
}

func (s *Service) findIssueForOperationLocked(key string, request any) (Issue, bool, issueReceipt, error) {
	receipt, exists, err := readIssueReceipt(s.Root, key, request)
	if err != nil || !exists {
		return Issue{}, exists, receipt, err
	}
	if receipt.State == "completed" && len(receipt.Result) > 0 {
		var out Issue
		if err := json.Unmarshal(receipt.Result, &out); err == nil && out.ID != "" {
			return out, true, receipt, nil
		}
	}
	if receipt.ResourceID != "" {
		i, _, readErr := readIssue(s.Root, receipt.ResourceID)
		return i, true, receipt, readErr
	}
	return Issue{}, true, receipt, nil
}

// IntakeIssue creates or revises one durable Issue. Source URLs are deduped;
// source-less manual intake creates a distinct entity unless an operation key
// replays an earlier request.
func (s *Service) IntakeIssue(ctx context.Context, opt IssueCreateOptions) (Issue, error) {
	if err := s.authorizeProjectActor(ctx); err != nil {
		return Issue{}, err
	}
	opt.Source = strings.TrimSpace(opt.Source)
	opt.Title = strings.TrimSpace(opt.Title)
	opt.Body = normalizeIssueBody(opt.Body)
	if len(opt.Body) > 4*1024*1024 {
		return Issue{}, fail("input_too_large", "Issue description exceeds 4 MiB")
	}
	if err := validateIssueSource(opt.Source); err != nil {
		return Issue{}, err
	}
	if opt.Source == "" && opt.Body == "" {
		return Issue{}, fail("input_required", "provide an Issue description or a source URL")
	}
	request := opt
	if opt.OperationKey != "" {
		var replay Issue
		err := withProjectLock(ctx, s.Root, func() error {
			if err := recoverIssueWrite(s.Root); err != nil {
				return err
			}
			i, exists, receipt, err := s.findIssueForOperationLocked(opt.OperationKey, request)
			if err != nil {
				return err
			}
			if exists {
				if receipt.State == "completed" || receipt.ResourceID != "" {
					replay = i
					return nil
				}
			}
			return nil
		})
		if err != nil {
			return Issue{}, err
		}
		if replay.ID != "" {
			return replay, nil
		}
	}
	if opt.Body == "" && opt.Source != "" {
		content, err := s.issueSource(ctx, opt.Source)
		if err != nil {
			return Issue{}, err
		}
		if opt.Title == "" {
			opt.Title = strings.TrimSpace(content.Title)
		}
		opt.Body = normalizeIssueBody(content.Body)
		if opt.Title == "" && opt.Body == "" {
			return Issue{}, fail("input_required", "tracker returned no Issue description")
		}
	}
	if opt.Title == "" {
		opt.Title = "Untitled issue"
	}
	now := nowUTC()
	var out Issue
	err := withProjectLock(ctx, s.Root, func() error {
		if err := recoverIssueWrite(s.Root); err != nil {
			return err
		}
		if opt.OperationKey != "" {
			i, exists, receipt, err := s.findIssueForOperationLocked(opt.OperationKey, request)
			if err != nil {
				return err
			}
			if exists {
				if receipt.State == "completed" || receipt.ResourceID != "" {
					out = i
					return nil
				}
			} else if _, err := beginIssueReceipt(s.Root, opt.OperationKey, request); err != nil {
				return err
			}
		}
		var previous Issue
		if opt.Source != "" {
			var findErr error
			previous, findErr = s.findIssueBySourceLocked(opt.Source)
			if findErr != nil {
				return findErr
			}
		}
		if previous.ID == "" {
			out = Issue{SchemaVersion: issueSchemaVersion, ID: ID("issue"), ProjectID: mustProjectID(s), Revision: 1, Title: opt.Title, Source: opt.Source, Status: "open", Digest: canonicalIssueDigest(opt.Title, opt.Body, opt.Source), CreatedAt: now, UpdatedAt: now, Body: opt.Body}
			if opt.Source != "" {
				out.RetrievedAt, out.LastCheckedAt = timePtr(now), timePtr(now)
			}
			if err := writeIssue(s.Root, Issue{}, out, false); err != nil {
				return err
			}
		} else {
			out = previous
			digestValue := canonicalIssueDigest(opt.Title, opt.Body, opt.Source)
			if digestValue == previous.Digest {
				out.LastCheckedAt = timePtr(now)
				out.UpdatedAt = now
				if err := writeIssue(s.Root, previous, out, false); err != nil {
					return err
				}
			} else {
				out.Revision++
				out.Title, out.Body, out.Source, out.Digest = opt.Title, opt.Body, opt.Source, digestValue
				out.UpdatedAt = now
				if opt.Source != "" {
					out.RetrievedAt, out.LastCheckedAt = timePtr(now), timePtr(now)
				}
				if err := writeIssue(s.Root, previous, out, true); err != nil {
					return err
				}
			}
		}
		if opt.OperationKey != "" {
			receipt, exists, err := readIssueReceipt(s.Root, opt.OperationKey, request)
			if err != nil {
				return err
			}
			if !exists {
				receipt, err = beginIssueReceipt(s.Root, opt.OperationKey, request)
				if err != nil {
					return err
				}
			}
			receipt.ResourceID = out.ID
			if err := writeJSON(issueOperationPath(s.Root, opt.OperationKey), receipt); err != nil {
				return err
			}
			if err := completeIssueReceipt(s.Root, opt.OperationKey, receipt, out, out.Revision); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func mustProjectID(s *Service) string {
	cfg, err := s.Config()
	if err != nil {
		return ""
	}
	return cfg.ProjectID
}

func timePtr(value time.Time) *time.Time { return &value }

// ListIssues is a read-only query. It never creates the issues directory and
// keeps corrupt sibling records visible as error rows.
func (s *Service) ListIssues(ctx context.Context) ([]IssueSummary, error) {
	var out []IssueSummary
	err := withProjectReadLock(ctx, s.Root, func() error {
		if err := recoverIssueWrite(s.Root); err != nil {
			return err
		}
		entries, err := os.ReadDir(issueRoot(s.Root))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "issue_") {
				continue
			}
			row := IssueSummary{ID: entry.Name(), Status: "unknown"}
			i, _, err := readIssue(s.Root, entry.Name())
			if err != nil {
				row.Error = err.Error()
				out = append(out, row)
				continue
			}
			row = issueSummary(i)
			out = append(out, row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i := range out {
		links, err := s.linkedWorkspaces(ctx, out[i].ID)
		if err != nil {
			out[i].Error = firstNonempty(out[i].Error, err.Error())
			continue
		}
		out[i].LinkedWorkspaces = links
		out[i].LinkedWorkspaceCount = len(links)
		out[i].LinkedStateSummary = linkedStateSummary(links)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status < out[j].Status
		}
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func firstNonempty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func linkedStateSummary(links []IssueWorkspaceLink) string {
	counts := map[string]int{}
	for _, link := range links {
		counts[link.Status]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}

func (s *Service) linkedWorkspaces(ctx context.Context, issueID string) ([]IssueWorkspaceLink, error) {
	var links []IssueWorkspaceLink
	overview, err := s.ProjectOverview(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range overview.Workspaces {
		if row.Error != "" || row.IssueID != issueID {
			continue
		}
		links = append(links, IssueWorkspaceLink{WorkspaceID: row.ID, Title: row.Title, Status: row.Status, IssueRevision: row.IssueRevision, IssueDigest: row.IssueDigest})
	}
	return links, nil
}

func (s *Service) ShowIssue(ctx context.Context, selector string) (IssueDetail, error) {
	var out IssueDetail
	err := withProjectReadLock(ctx, s.Root, func() error {
		if err := recoverIssueWrite(s.Root); err != nil {
			return err
		}
		i, _, err := readIssue(s.Root, selector)
		if err != nil {
			return err
		}
		out.Issue = i
		return nil
	})
	if err != nil {
		return out, err
	}
	out.LinkedWorkspaces, err = s.linkedWorkspaces(ctx, selector)
	return out, err
}

func (s *Service) RefreshIssue(ctx context.Context, selector string, opt IssueRefreshOptions) (Issue, error) {
	if err := s.authorizeProjectActor(ctx); err != nil {
		return Issue{}, err
	}
	request := struct {
		IssueID          string
		ExpectedRevision int
	}{selector, opt.ExpectedRevision}
	if opt.OperationKey != "" {
		var replay Issue
		err := withProjectLock(ctx, s.Root, func() error {
			if err := recoverIssueWrite(s.Root); err != nil {
				return err
			}
			receipt, exists, err := readIssueReceipt(s.Root, opt.OperationKey, request)
			if err != nil || !exists {
				return err
			}
			if receipt.State == "completed" && len(receipt.Result) > 0 {
				return json.Unmarshal(receipt.Result, &replay)
			}
			if receipt.ResourceID != "" {
				i, _, readErr := readIssue(s.Root, receipt.ResourceID)
				if readErr != nil {
					return readErr
				}
				replay = i
			}
			if replay.ID == "" {
				intakeReceipt, intakeExists, readErr := readIssueReceiptFile(s.Root, "refresh-content:"+opt.OperationKey)
				if readErr != nil {
					return readErr
				}
				if intakeExists && intakeReceipt.State == "completed" && len(intakeReceipt.Result) > 0 {
					if readErr := json.Unmarshal(intakeReceipt.Result, &replay); readErr != nil {
						return readErr
					}
				} else if intakeExists && intakeReceipt.ResourceID != "" {
					replay, _, readErr = readIssue(s.Root, intakeReceipt.ResourceID)
					if readErr != nil {
						return readErr
					}
				}
			}
			if replay.ID != "" && exists {
				receipt.ResourceID, receipt.Revision = replay.ID, replay.Revision
				if writeErr := writeJSON(issueOperationPath(s.Root, opt.OperationKey), receipt); writeErr != nil {
					return writeErr
				}
				return completeIssueReceipt(s.Root, opt.OperationKey, receipt, replay, replay.Revision)
			}
			return nil
		})
		if err != nil {
			return Issue{}, err
		}
		if replay.ID != "" {
			return replay, nil
		}
	}
	current, err := s.ShowIssue(ctx, selector)
	if err != nil {
		return Issue{}, err
	}
	if opt.ExpectedRevision != current.Revision {
		return Issue{}, fail("revision_conflict", "expected %d, current %d", opt.ExpectedRevision, current.Revision)
	}
	if current.Source == "" {
		return Issue{}, fail("invalid_issue_url", "cannot refresh an Issue without an HTTP(S) source URL")
	}
	if opt.OperationKey != "" {
		if err := withProjectLock(ctx, s.Root, func() error {
			_, exists, err := readIssueReceipt(s.Root, opt.OperationKey, request)
			if err != nil {
				return err
			}
			if !exists {
				_, err = beginIssueReceipt(s.Root, opt.OperationKey, request)
			}
			return err
		}); err != nil {
			return Issue{}, err
		}
	}
	content, err := s.issueSource(ctx, current.Source)
	if err != nil {
		return Issue{}, err
	}
	title := firstNonempty(strings.TrimSpace(content.Title), current.Title)
	intakeKey := ""
	if opt.OperationKey != "" {
		intakeKey = "refresh-content:" + opt.OperationKey
	}
	out, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: title, Body: content.Body, Source: current.Source, OperationKey: intakeKey})
	if err != nil {
		return Issue{}, err
	}
	if opt.OperationKey != "" {
		err = withProjectLock(ctx, s.Root, func() error {
			receipt, exists, readErr := readIssueReceipt(s.Root, opt.OperationKey, request)
			if readErr != nil {
				return readErr
			}
			if !exists {
				receipt, readErr = beginIssueReceipt(s.Root, opt.OperationKey, request)
				if readErr != nil {
					return readErr
				}
			}
			receipt.ResourceID, receipt.Revision = out.ID, out.Revision
			if err := writeJSON(issueOperationPath(s.Root, opt.OperationKey), receipt); err != nil {
				return err
			}
			return completeIssueReceipt(s.Root, opt.OperationKey, receipt, out, out.Revision)
		})
	}
	return out, err
}

func (s *Service) UpdateIssue(ctx context.Context, selector string, opt IssueUpdateOptions) (Issue, error) {
	if opt.Status != "open" && opt.Status != "deferred" && opt.Status != "closed" {
		return Issue{}, fail("invalid_issue_status", "use open, deferred, or closed")
	}
	if (opt.Status == "deferred" || opt.Status == "closed") && strings.TrimSpace(opt.Reason) == "" {
		return Issue{}, fail("reason_required", "deferred and closed Issues require a non-empty reason")
	}
	if err := s.authorizeProjectActor(ctx); err != nil {
		return Issue{}, err
	}
	var out Issue
	err := withProjectLock(ctx, s.Root, func() error {
		if err := recoverIssueWrite(s.Root); err != nil {
			return err
		}
		request := struct {
			IssueID          string
			Status           string
			Reason           string
			ExpectedRevision int
		}{selector, opt.Status, strings.TrimSpace(opt.Reason), opt.ExpectedRevision}
		if opt.OperationKey != "" {
			receipt, exists, err := readIssueReceipt(s.Root, opt.OperationKey, request)
			if err != nil {
				return err
			}
			if exists && receipt.State == "completed" && len(receipt.Result) > 0 {
				return json.Unmarshal(receipt.Result, &out)
			}
			if exists && receipt.ResourceID != "" {
				replayed, _, readErr := readIssue(s.Root, receipt.ResourceID)
				if readErr != nil {
					return readErr
				}
				out = replayed
				return nil
			}
			if exists {
				// If the process wrote the new Issue but crashed before recording
				// the receipt's resource, reconcile the exact requested status
				// transition instead of treating the already-applied revision as
				// a stale guard conflict.
				candidate, _, readErr := readIssue(s.Root, selector)
				if readErr == nil && candidate.Revision == opt.ExpectedRevision+1 && candidate.Status == opt.Status && candidate.StatusReason == request.Reason {
					out = candidate
					receipt.ResourceID, receipt.Revision = out.ID, out.Revision
					if writeErr := writeJSON(issueOperationPath(s.Root, opt.OperationKey), receipt); writeErr != nil {
						return writeErr
					}
					return completeIssueReceipt(s.Root, opt.OperationKey, receipt, out, out.Revision)
				}
			}
			if !exists {
				if _, err := beginIssueReceipt(s.Root, opt.OperationKey, request); err != nil {
					return err
				}
			}
		}
		before, _, err := readIssue(s.Root, selector)
		if err != nil {
			return err
		}
		if before.Revision != opt.ExpectedRevision {
			return fail("revision_conflict", "expected %d, current %d", opt.ExpectedRevision, before.Revision)
		}
		out = before
		out.Revision++
		out.Status = opt.Status
		out.StatusReason = strings.TrimSpace(opt.Reason)
		out.UpdatedAt = nowUTC()
		if err := writeIssue(s.Root, before, out, true); err != nil {
			return err
		}
		if opt.OperationKey != "" {
			receipt, exists, err := readIssueReceipt(s.Root, opt.OperationKey, request)
			if err != nil {
				return err
			}
			if !exists {
				receipt, err = beginIssueReceipt(s.Root, opt.OperationKey, request)
				if err != nil {
					return err
				}
			}
			receipt.ResourceID, receipt.Revision = out.ID, out.Revision
			if err := writeJSON(issueOperationPath(s.Root, opt.OperationKey), receipt); err != nil {
				return err
			}
			if err := completeIssueReceipt(s.Root, opt.OperationKey, receipt, out, out.Revision); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func (s *Service) authorizeProjectActor(ctx context.Context) error {
	if s.Actor.AgentID == "" && s.Actor.SessionID == "" && s.Actor.RunID == "" {
		return nil
	}
	if s.Actor.Scope != "project" {
		return fail("forbidden", "workspace actors cannot mutate project Issues")
	}
	state, err := s.DispatcherStatus(ctx)
	if err != nil || state.Agent.ID == "" {
		return fail("stale_actor", "unknown Dispatcher actor")
	}
	for _, session := range state.Sessions {
		if session.ID != s.Actor.SessionID || session.AgentID != s.Actor.AgentID || session.CurrentRunID != s.Actor.RunID || !session.Active() {
			continue
		}
		for _, run := range state.Runs {
			if run.ID == s.Actor.RunID && run.SessionID == session.ID && run.Active() {
				return nil
			}
		}
	}
	return fail("stale_actor", "Dispatcher actor is not the active owner of this Run")
}

// parseIssueHeader is used by project overview code to avoid reading an entire
// multi-megabyte Issue body during a summary refresh.
func parseIssueHeader(r io.Reader) (Issue, error) {
	br := bufio.NewReader(io.LimitReader(r, 256*1024))
	var b bytes.Buffer
	for {
		line, err := br.ReadBytes('\n')
		b.Write(line)
		if b.Len() > 128*1024 {
			return Issue{}, fail("invalid_issue", "Issue frontmatter is too large")
		}
		if bytes.HasSuffix(b.Bytes(), []byte("\n---\n")) {
			break
		}
		if err != nil {
			return Issue{}, fail("invalid_issue", "unclosed Issue YAML frontmatter")
		}
	}
	text := strings.ReplaceAll(b.String(), "\r\n", "\n")
	head, _, ok := strings.Cut(text[4:], "\n---\n")
	if !ok {
		return Issue{}, fail("invalid_issue", "unclosed Issue YAML frontmatter")
	}
	var f issueFrontmatter
	if err := strictYAML([]byte(head), &f); err != nil {
		return Issue{}, fail("invalid_issue", "%v", err)
	}
	if err := validateIssueFrontmatter(f); err != nil {
		return Issue{}, err
	}
	// The header-only path intentionally leaves Body empty. Full reads validate
	// the canonical digest against the complete body; project overviews only
	// need bounded metadata and must not load multi-megabyte descriptions.
	return issueFromFrontmatter(f, ""), nil
}
