package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrackerCommandProcess(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "https://tracker.example/142?text=a%20b" {
		return
	}
	_ = json.NewEncoder(os.Stdout).Encode(IssueContent{Title: "Fetched through wrapper", Body: "Actual JSON response"})
	os.Exit(0)
}

func TestConfiguredTrackerCommand(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f := CLITracker{Root: t.TempDir(), Config: TrackerConfig{Adapter: "command", CommandArgv: []string{exe, "-test.run=TestTrackerCommandProcess", "--", "{url}"}}}
	issue, err := f.Fetch(context.Background(), "https://tracker.example/142?text=a%20b")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Title != "Fetched through wrapper" || issue.Body != "Actual JSON response" {
		t.Fatalf("unexpected wrapper response: %+v", issue)
	}
	_, err = f.Fetch(context.Background(), "--malicious-flag")
	expectCode(t, err, "invalid_issue_url")
}

type fakeIssueFetcher struct {
	calls int
	err   error
}

func (f *fakeIssueFetcher) Fetch(context.Context, string) (IssueContent, error) {
	f.calls++
	return IssueContent{Title: "Checkout retry", Body: "The second payment attempt fails."}, f.err
}
func TestIssueSnapshotAndCreateReplay(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	fetcher := &fakeIssueFetcher{}
	s.IssueFetcher = fetcher
	opt := CreateOptions{Source: "https://tracker.example/issue/142", Workflow: "issue-resolution", OperationKey: "ticket"}
	v, err := s.Create(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	if v.Workspace.Title != "Checkout retry" {
		t.Fatal("fetched title not used")
	}
	b, err := os.ReadFile(filepath.Join(v.Directory, v.Workspace.Input.Snapshot))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{opt.Source, "Retrieved:", "second payment attempt"} {
		if !strings.Contains(string(b), want) {
			t.Fatal("missing snapshot context", want)
		}
	}
	fetcher.err = fail("tracker_unavailable", "offline")
	replay, err := s.Create(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Workspace.ID != v.Workspace.ID || fetcher.calls != 1 {
		t.Fatal("create replay fetched mutable issue again")
	}
	opt.OperationKey = "other"
	_, err = s.Create(ctx, opt)
	expectCode(t, err, "tracker_unavailable")
	opt.Input = "Supplied offline description"
	if _, err := s.Create(ctx, opt); err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != 2 {
		t.Fatal("explicit description invoked tracker")
	}
}
