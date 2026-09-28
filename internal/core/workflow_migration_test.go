package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateLegacyWorkflowRewritesStateAndSnapshotsHistory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "WORKSPACE.md"), []byte("legacy state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "WORKFLOW.md"), []byte("legacy workflow"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "implementation.md.tmpl"), []byte("legacy prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := &Document{Dir: dir, Body: "body", State: Workspace{
		SchemaVersion:     1,
		Status:            "active",
		Base:              Base{Ref: "main"},
		Workflow:          &Workflow{ID: "issue-resolution", Version: 2, Phase: "live_test_offer", Capabilities: []string{"release"}},
		Integration:       &Integration{BaseCommit: "base"},
		PendingDecision:   &Decision{ID: "decision", Kind: "live-testing"},
		ChangeRequestMode: "integrated",
	}}
	migrated, err := migrateLegacyWorkflow(d)
	if err != nil {
		t.Fatal(err)
	}
	if !migrated || d.State.Workflow.ID != "plan-first" || d.State.Workflow.Phase != "integration" {
		t.Fatalf("migration state: migrated=%v workflow=%+v", migrated, d.State.Workflow)
	}
	if d.State.Integration.Target != "main" || d.State.Integration.Strategy != "merge" {
		t.Fatalf("integration defaults: %+v", d.State.Integration)
	}
	if d.State.PendingDecision != nil || len(d.State.Decisions) != 1 || !d.State.Decisions[0].Superseded {
		t.Fatalf("pending decision was not superseded: pending=%+v decisions=%+v", d.State.PendingDecision, d.State.Decisions)
	}
	if d.State.ChangeRequestMode != "" || !strings.Contains(d.Body, "Workflow auto-migrated") {
		t.Fatalf("migration note or mode missing: body=%q mode=%q", d.Body, d.State.ChangeRequestMode)
	}
	var manifestPath string
	for name := range d.PendingFiles {
		if strings.HasSuffix(name, "/migration.json") {
			manifestPath = name
		}
	}
	if manifestPath == "" || string(d.PendingFiles[manifestPath]) == "" {
		t.Fatal("migration manifest was not staged")
	}
	if string(d.PendingFiles[filepath.Join(filepath.Dir(manifestPath), "WORKSPACE.md")]) != "legacy state" {
		t.Fatal("legacy WORKSPACE.md was not preserved")
	}
	second, err := migrateLegacyWorkflow(d)
	if err != nil {
		t.Fatal(err)
	}
	if second {
		t.Fatal("second migration was not a no-op")
	}
}
