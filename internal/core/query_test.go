package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceSnapshotAndPreviewAreSeparateFromPublicStatus(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	snapshot, err := s.WorkspaceSnapshot(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ObservedAt.IsZero() || snapshot.Status.SchemaVersion != 2 || snapshot.Status.Workspace.ID != id {
		t.Fatalf("invalid snapshot metadata: %+v", snapshot)
	}
	if snapshot.Body == "" || snapshot.Metrics.TaskTotal != len(snapshot.Status.Workspace.Tasks) {
		t.Fatalf("snapshot omitted workspace state: %+v", snapshot)
	}
	status, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != 2 || len(status.Workspace.Tasks) != len(snapshot.Status.Workspace.Tasks) {
		t.Fatalf("public status contract changed: %+v", status)
	}
	preview, err := s.ReadPreview(ctx, id, PreviewDocument, "input")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Text != "A reproducible issue" || preview.Binary || preview.Truncated {
		t.Fatalf("unexpected input preview: %+v", preview)
	}
	if _, err := s.ReadPreview(ctx, id, PreviewDocument, "../../outside"); err == nil {
		t.Fatal("accepted an arbitrary document path")
	}
}

func TestProjectOverviewKeepsHealthyWorkspaceVisibleBesideCorruption(t *testing.T) {
	s, id := fixture(t)
	badDir := filepath.Join(s.Root, ".workspace", "ws_corrupt")
	if err := os.MkdirAll(badDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "WORKSPACE.md"), []byte("not frontmatter\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	overview, err := s.ProjectOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Workspaces) != 2 {
		t.Fatalf("got %d workspaces: %+v", len(overview.Workspaces), overview.Workspaces)
	}
	var sawHealthy, sawError bool
	for _, workspace := range overview.Workspaces {
		if workspace.ID == id && workspace.Error == "" {
			sawHealthy = true
		}
		if workspace.ID == "ws_corrupt" && workspace.Error != "" {
			sawError = true
		}
	}
	if !sawHealthy || !sawError {
		t.Fatalf("healthy/corrupt rows not represented: %+v", overview.Workspaces)
	}
	if _, err := s.WorkspaceSnapshot(ctx, id); err != nil {
		t.Fatalf("healthy workspace could not be opened beside damaged sibling: %v", err)
	}
	if _, err := s.List(ctx); err == nil {
		t.Fatal("legacy List no longer fails fast on a damaged workspace")
	}
}

func TestWorkspaceRelationsKeepAttemptAndArtifactLineage(t *testing.T) {
	d := &Document{
		State: Workspace{Tasks: []Task{{ID: "task_a", Attempt: 2, SessionID: "sess_new", WorktreeID: "wt_new"}}},
		Registry: Registry{
			Worktrees: []Worktree{{ID: "wt_old"}, {ID: "wt_new"}},
			Sessions: []Session{
				{ID: "sess_old", TaskID: "task_a", TaskAttempt: 1, WorktreeID: "wt_old"},
				{ID: "sess_new", TaskID: "task_a", TaskAttempt: 2, WorktreeID: "wt_new"},
			},
			Runs: []Run{{ID: "run_old", SessionID: "sess_old"}, {ID: "run_new", SessionID: "sess_new"}},
		},
	}
	d.State.Artifacts = []Artifact{{ID: "artifact_unknown", TaskID: "task_a"}, {ID: "artifact_old", TaskID: "task_a", SessionID: "sess_old", RunID: "run_old"}}
	relations := buildWorkspaceRelations(d)
	if len(relations.Tasks) != 1 || len(relations.Tasks[0].CurrentSessionIDs) != 1 || relations.Tasks[0].CurrentSessionIDs[0] != "sess_new" {
		t.Fatalf("current session lineage lost: %+v", relations.Tasks)
	}
	if len(relations.Tasks[0].HistoricalSessionIDs) != 1 || relations.Tasks[0].HistoricalSessionIDs[0] != "sess_old" {
		t.Fatalf("historical session lineage lost: %+v", relations.Tasks[0])
	}
	if len(relations.Tasks[0].ArtifactIDs) != 1 || relations.Tasks[0].ArtifactIDs[0] != "artifact_old" {
		t.Fatalf("unknown artifact lineage was attached by TaskID alone: %+v", relations.Tasks[0])
	}
	if len(relations.Worktrees[0].SessionIDs) != 1 || relations.Worktrees[0].SessionIDs[0] != "sess_old" {
		t.Fatalf("historical worktree relation lost: %+v", relations.Worktrees)
	}
}

type topologyRuntime struct {
	fakeRuntime
	topology TmuxTopology
	err      error
}

func (r *topologyRuntime) ObserveTopology(context.Context, string) (TmuxTopology, error) {
	return r.topology, r.err
}

func TestSupervisorObservationClassifiesAllHealthStates(t *testing.T) {
	runtime := Tmux{Socket: "expected"}
	cases := []struct {
		name, socket, state, message string
		runtime                      Runtime
		err                          error
	}{
		{name: "running", socket: "expected", state: "running", runtime: runtime},
		{name: "stopped", state: "stopped", runtime: runtime, err: os.ErrNotExist},
		{name: "conflict", socket: "other", state: "conflict", message: "supervisor is using a different tmux server", runtime: runtime},
		{name: "unavailable", state: "unavailable", message: "connection refused", runtime: runtime, err: errors.New("connection refused")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observation := classifySupervisorObservation(tc.runtime, SupervisorInfo{TmuxSocket: tc.socket}, tc.err)
			if observation.State != tc.state || observation.ObservedAt.IsZero() {
				t.Fatalf("observation = %+v, want state %q", observation, tc.state)
			}
			if tc.message != "" && observation.Error != tc.message {
				t.Fatalf("observation error = %q, want %q", observation.Error, tc.message)
			}
			if tc.message == "" && observation.Error != "" {
				t.Fatalf("unexpected observation error: %q", observation.Error)
			}
		})
	}
}

func TestObserveSupervisorIsReadOnlyAndReportsMissingSocketAsStopped(t *testing.T) {
	s, _ := fixture(t)
	observation, err := s.ObserveSupervisor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.State != "stopped" || observation.Error != "" || observation.ObservedAt.IsZero() {
		t.Fatalf("unexpected missing-supervisor observation: %+v", observation)
	}
}

func TestWorkspaceRuntimeAndNavigationUseVerifiedOwnership(t *testing.T) {
	s, id := fixture(t)
	runtime := &topologyRuntime{
		fakeRuntime: fakeRuntime{panes: map[string]Pane{}},
		topology:    TmuxTopology{WorkspaceID: id, SessionName: TmuxName(id), SessionExists: true, Windows: []TmuxWindow{{ID: "@1", Kind: "worktree"}}, Panes: []Pane{{ID: "%1", WindowID: "@1", Kind: "agent", WorkspaceID: id, SessionID: "sess_current", RunID: "run_current"}}},
	}
	s.Runtime = runtime
	ctx := context.Background()
	err := s.With(ctx, id, func(d *Document) error {
		d.Registry.Sessions = append(d.Registry.Sessions, Session{ID: "sess_current", AgentID: "agent_current", CurrentRunID: "run_current", State: "running", LifecycleState: "active"})
		d.Registry.Runs = append(d.Registry.Runs, Run{ID: "run_current", SessionID: "sess_current", State: "running", PaneID: "%1", WindowID: "@1"})
		return saveDocument(d)
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := s.ResolveNavigationTarget(ctx, id, EntityRef{Kind: "session", ID: "sess_current"})
	if err != nil {
		t.Fatal(err)
	}
	if target.PaneID != "%1" || target.RunID != "run_current" || target.SessionName != TmuxName(id) {
		t.Fatalf("unexpected resolved target: %+v", target)
	}
	observation, err := s.ObserveWorkspaceRuntime(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if observation.State != "present" || observation.SupervisorState != "stopped" || observation.ObservedAt.IsZero() || observation.SupervisorAt.IsZero() || observation.SupervisorError != "" {
		t.Fatalf("unexpected runtime observation: %+v", observation)
	}
	runtime.topology.Panes[0].RunID = "run_foreign"
	if _, err := s.ResolveNavigationTarget(ctx, id, EntityRef{Kind: "session", ID: "sess_current"}); err == nil {
		t.Fatal("resolver accepted a pane owned by a different run")
	} else {
		var ce *Error
		if !errors.As(err, &ce) || ce.Code != "pane_missing" {
			t.Fatalf("unexpected ownership error: %v", err)
		}
	}
}

func TestReadPreviewRejectsNonRegularFilesAndOutOfTreeSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := readPreviewFile(link, root); err == nil {
		t.Fatal("read preview followed a symlink outside the allowed directory")
	} else {
		var ce *Error
		if !errors.As(err, &ce) || ce.Code != "unsafe_path" {
			t.Fatalf("unexpected symlink error: %v", err)
		}
	}
}

func TestReadPreviewBoundsLargeFilesAndMarksBinaryContent(t *testing.T) {
	root := t.TempDir()
	largePath := filepath.Join(root, "large.txt")
	large := make([]byte, MaxPreviewBytes*4)
	for i := range large {
		large[i] = 'x'
	}
	if err := os.WriteFile(largePath, large, 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := readPreviewFile(largePath, root)
	if err != nil {
		t.Fatal(err)
	}
	if preview.size != int64(len(large)) || !preview.truncated || preview.binary || len(preview.text) != MaxPreviewBytes {
		t.Fatalf("large preview was not bounded at the read limit: size=%d truncated=%t binary=%t text=%d", preview.size, preview.truncated, preview.binary, len(preview.text))
	}

	binaryPath := filepath.Join(root, "binary.dat")
	if err := os.WriteFile(binaryPath, []byte{0x41, 0x00, 0x42}, 0600); err != nil {
		t.Fatal(err)
	}
	preview, err = readPreviewFile(binaryPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.binary || preview.text != "" || preview.size != 3 {
		t.Fatalf("binary preview was rendered as text: %+v", preview)
	}

	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := readPreviewFile(directory, root); err == nil {
		t.Fatal("directory was accepted as a preview file")
	} else {
		var ce *Error
		if !errors.As(err, &ce) || ce.Code != "unsafe_path" {
			t.Fatalf("unexpected non-regular preview error: %v", err)
		}
	}
}
