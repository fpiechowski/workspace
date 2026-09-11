package core

import (
	"context"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"testing"
)

func TestExternalWorkspaceStorageAndDiscovery(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.WorkspacesDir = t.TempDir()
	b, _ := yaml.Marshal(cfg)
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(ctx, CreateOptions{Input: "External work", Workflow: "issue-resolution"})
	if err != nil {
		t.Fatal(err)
	}
	if !contained(cfg.WorkspacesDir, v.Directory) {
		t.Fatal("workspace stored in wrong location")
	}
	w, err := s.CreateWorktree(ctx, v.Workspace.ID, WorktreeOptions{Name: "external"})
	if err != nil {
		t.Fatal(err)
	}
	project, err := DiscoverProject(w.Path)
	if err != nil || project != s.Root {
		t.Fatal("cannot discover external workspace project", err)
	}
	if _, err := s.Status(ctx, v.Workspace.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSharedStorageKeepsProjectsIsolated(t *testing.T) {
	a, _ := fixture(t)
	b, _ := fixture(t)
	ctx := context.Background()
	root := t.TempDir()
	var workspaces []Status
	for _, s := range []*Service{a, b} {
		cfg, err := s.Config()
		if err != nil {
			t.Fatal(err)
		}
		cfg.WorkspacesDir = root
		data, err := yaml.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), data); err != nil {
			t.Fatal(err)
		}
		v, err := s.Create(ctx, CreateOptions{Input: "Shared storage issue", Workflow: "issue-resolution", OperationKey: "same-key"})
		if err != nil {
			t.Fatal(err)
		}
		workspaces = append(workspaces, v)
	}
	for i, s := range []*Service{a, b} {
		listed, err := s.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 1 || listed[0].Workspace.ID != workspaces[i].Workspace.ID {
			t.Fatal("foreign workspace leaked into project")
		}
		_, err = s.Status(ctx, workspaces[1-i].Workspace.ID)
		expectCode(t, err, "workspace_not_found")
	}
}
