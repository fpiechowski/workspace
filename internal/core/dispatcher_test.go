package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDispatcherLifecycleAndReplay(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	runtime := s.Runtime.(*fakeRuntime)

	started, err := s.StartDispatcher(ctx, "", "dispatcher-start")
	if err != nil {
		t.Fatal(err)
	}
	if started.Run.ID == "" || started.Run.Profile != "frontier" || started.Run.Scope != "project" || started.Run.ProjectID == "" {
		t.Fatalf("unexpected fallback Dispatcher Run: %+v", started.Run)
	}
	if runtime.launches != 1 {
		t.Fatalf("expected one launch, got %d", runtime.launches)
	}

	replayed, err := s.StartDispatcher(ctx, "", "dispatcher-start")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Run.ID != started.Run.ID || runtime.launches != 1 {
		t.Fatalf("start replay launched a duplicate: first=%s replay=%s launches=%d", started.Run.ID, replayed.Run.ID, runtime.launches)
	}

	_, err = s.StartDispatcher(ctx, "implementation", "another-start")
	expectCode(t, err, "agent_busy")

	if err := s.StopDispatcher(ctx, "dispatcher-stop"); err != nil {
		t.Fatal(err)
	}
	if err := s.StopDispatcher(ctx, "dispatcher-stop"); err != nil {
		t.Fatal("stop replay failed: ", err)
	}
	status, err := s.DispatcherStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "stopped" || !status.StopRequested || status.Runtime.Verified {
		t.Fatalf("unexpected stopped Dispatcher status: %+v", status)
	}

	resumed, err := s.StartDispatcher(ctx, "implementation", "dispatcher-resume")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Run.Profile != "implementation" || resumed.Run.ID == started.Run.ID {
		t.Fatalf("explicit profile did not create a new Dispatcher Run: %+v", resumed.Run)
	}
}

func TestDispatcherAndIssueQueriesDoNotCreateMissingStores(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"}} {
		if _, err := git(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := InitProject(ctx, dir); err != nil {
		t.Fatal(err)
	}
	s := &Service{Root: dir, Runtime: &fakeRuntime{panes: map[string]Pane{}}}
	issuePath := filepath.Join(s.Root, ".workspace", "issues")
	dispatcherPath := filepath.Join(s.Root, ".workspace", "dispatcher")
	if _, err := os.Stat(issuePath); !os.IsNotExist(err) {
		t.Fatalf("fixture unexpectedly created Issue store: %v", err)
	}
	if _, err := os.Stat(dispatcherPath); !os.IsNotExist(err) {
		t.Fatalf("fixture unexpectedly created Dispatcher store: %v", err)
	}
	if _, err := s.ListIssues(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := s.DispatcherStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "never_started" || status.Initialized {
		t.Fatalf("unexpected missing Dispatcher status: %+v", status)
	}
	overview, err := s.ProjectOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Dispatcher.State != "never_started" {
		t.Fatalf("overview did not expose missing Dispatcher state: %+v", overview.Dispatcher)
	}
	for _, path := range []string{issuePath, dispatcherPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("read-only query created %s: %v", path, err)
		}
	}
}

func TestProjectActorAuthorizationForIssueMutation(t *testing.T) {
	s, _ := fixture(t)
	s.Actor = Actor{AgentID: "agent_workspace", SessionID: "sess_workspace", RunID: "run_workspace", Scope: "workspace"}
	_, err := s.IntakeIssue(context.Background(), IssueCreateOptions{Title: "Forbidden", Body: "Mutation from workspace actor"})
	expectCode(t, err, "forbidden")
}

func TestProjectDispatcherCannotUseGenericWorkspaceMutations(t *testing.T) {
	s, _ := fixture(t)
	s.Actor = Actor{AgentID: "agent_dispatcher", SessionID: "sess_dispatcher", RunID: "run_dispatcher", Scope: "project"}
	ctx := context.Background()

	_, err := s.Create(ctx, CreateOptions{Title: "Unlinked", Input: "arbitrary Workspace input", NoWorkflow: true})
	expectCode(t, err, "forbidden")
	_, err = s.StartSupervisedOrchestrator(ctx, "ws_missing", "start-unlinked")
	expectCode(t, err, "forbidden")
}
