package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstClassIssueIntakeRevisionAndFrozenWorkspace(t *testing.T) {
	s, _ := fixture(t)
	fetcher := &fakeIssueFetcher{}
	s.IssueFetcher = fetcher
	ctx := context.Background()

	issue, err := s.IntakeIssue(ctx, IssueCreateOptions{Source: "https://tracker.example/issues/42", OperationKey: "intake:42"})
	if err != nil {
		t.Fatal(err)
	}
	if issue.ID == "" || !strings.HasPrefix(issue.ID, "issue_") || issue.Revision != 1 {
		t.Fatalf("unexpected Issue: %+v", issue)
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".workspace", "issues", issue.ID, "ISSUE.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.IntakeIssue(ctx, IssueCreateOptions{Source: issue.Source, OperationKey: "intake:42:replay"}); err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != 2 {
		t.Fatalf("new operation should recheck tracker, calls=%d", fetcher.calls)
	}
	if _, err := s.IntakeIssue(ctx, IssueCreateOptions{Source: issue.Source, OperationKey: "intake:42:replay"}); err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != 2 {
		t.Fatalf("completed unchanged intake operation fetched again, calls=%d", fetcher.calls)
	}
	replay, err := s.IntakeIssue(ctx, IssueCreateOptions{Source: issue.Source, OperationKey: "intake:42"})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != issue.ID || replay.Revision != issue.Revision || fetcher.calls != 2 {
		t.Fatalf("intake replay changed Issue or fetched again: %+v calls=%d", replay, fetcher.calls)
	}

	oldDigest := issue.Digest

	// A manual update with the same source proves changed canonical content is
	// revised without relying on a live tracker implementation.
	changed, err := s.IntakeIssue(ctx, IssueCreateOptions{Source: issue.Source, Title: "Checkout retry changed", Body: "A changed body."})
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID != issue.ID || changed.Revision != issue.Revision+1 || changed.Digest == oldDigest {
		t.Fatalf("changed source did not create a new revision: old=%+v new=%+v", issue, changed)
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".workspace", "issues", issue.ID, "history", "revision-000001", "ISSUE.md")); err != nil {
		t.Fatal("prior Issue revision was not preserved", err)
	}

	workspace, err := s.CreateFromIssue(ctx, issue.ID, CreateOptions{OperationKey: "workspace:from-issue"})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Workspace.Input.IssueID != issue.ID || workspace.Workspace.Input.IssueRevision != changed.Revision || workspace.Workspace.Input.IssueDigest != changed.Digest {
		t.Fatalf("Workspace did not capture exact Issue relationship: %+v", workspace.Workspace.Input)
	}
	input, err := os.ReadFile(filepath.Join(workspace.Directory, workspace.Workspace.Input.Snapshot))
	if err != nil || !strings.Contains(string(input), "Revision: 2") || !strings.Contains(string(input), "A changed body.") {
		t.Fatalf("Workspace Issue snapshot was not frozen: %q %v", input, err)
	}
	newIssue, err := s.IntakeIssue(ctx, IssueCreateOptions{Source: issue.Source, Title: "Checkout retry latest", Body: "Latest body."})
	if err != nil {
		t.Fatal(err)
	}
	if newIssue.Revision != changed.Revision+1 {
		t.Fatalf("expected third revision, got %d", newIssue.Revision)
	}
	frozen, err := os.ReadFile(filepath.Join(workspace.Directory, workspace.Workspace.Input.Snapshot))
	if err != nil || !strings.Contains(string(frozen), "A changed body.") || strings.Contains(string(frozen), "Latest body.") {
		t.Fatalf("existing Workspace input changed after Issue refresh: %q", frozen)
	}
}

func TestManualIssuesRemainDistinct(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	one, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: "Same title", Body: "Same body"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: "Same title", Body: "Same body"})
	if err != nil {
		t.Fatal(err)
	}
	if one.ID == two.ID || one.Revision != 1 || two.Revision != 1 {
		t.Fatalf("manual Issues were unexpectedly deduplicated: one=%+v two=%+v", one, two)
	}
}

func TestDispatchIssueCreatesLinkedWorkspaceAndReplays(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	issue, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: "Route checkout", Body: "Route this durable Issue"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.DispatchIssue(ctx, IssueDispatchOptions{IssueID: issue.ID, NoWorkflow: true, OperationKey: "dispatch:checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Workspace.Workspace.Input.IssueID != issue.ID || first.Workspace.Workspace.Input.IssueRevision != issue.Revision || first.Started {
		t.Fatalf("unexpected dispatch result: %+v", first)
	}
	replayed, err := s.DispatchIssue(ctx, IssueDispatchOptions{IssueID: issue.ID, NoWorkflow: true, OperationKey: "dispatch:checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Workspace.Workspace.ID != first.Workspace.Workspace.ID {
		t.Fatalf("dispatch replay created a duplicate Workspace: first=%s replay=%s", first.Workspace.Workspace.ID, replayed.Workspace.Workspace.ID)
	}
}

func TestIssueStatusGuardAndDamagedSiblingTolerance(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	issue, err := s.IntakeIssue(ctx, IssueCreateOptions{Title: "Manual", Body: "Description", OperationKey: "manual:1"})
	if err != nil {
		t.Fatal(err)
	}
	deferred, err := s.UpdateIssue(ctx, issue.ID, IssueUpdateOptions{Status: "deferred", Reason: "Waiting", ExpectedRevision: issue.Revision, OperationKey: "status:1"})
	if err != nil || deferred.Status != "deferred" {
		t.Fatal(err)
	}
	replayed, err := s.UpdateIssue(ctx, issue.ID, IssueUpdateOptions{Status: "deferred", Reason: "Waiting", ExpectedRevision: issue.Revision, OperationKey: "status:1"})
	if err != nil || replayed.Revision != deferred.Revision {
		t.Fatalf("status replay was not idempotent: %+v / %+v / %v", deferred, replayed, err)
	}
	_, err = s.UpdateIssue(ctx, issue.ID, IssueUpdateOptions{Status: "closed", ExpectedRevision: deferred.Revision})
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != "reason_required" {
		t.Fatalf("missing close reason returned %v", err)
	}

	damaged := filepath.Join(s.Root, ".workspace", "issues", "issue_damaged")
	if err := os.MkdirAll(damaged, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(damaged, "ISSUE.md"), []byte("not frontmatter"), 0600); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListIssues(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var healthy, broken bool
	for _, row := range rows {
		healthy = healthy || row.ID == issue.ID && row.Error == ""
		broken = broken || row.ID == "issue_damaged" && row.Error != ""
	}
	if !healthy || !broken {
		t.Fatalf("damaged sibling hid healthy Issue: %+v", rows)
	}
}
