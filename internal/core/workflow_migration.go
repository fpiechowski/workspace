package core

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func migrateLegacyWorkflow(d *Document) (bool, error) {
	if d.State.Workflow == nil || d.State.Workflow.ID != "issue-resolution" {
		return false, nil
	}
	oldPhase := d.State.Workflow.Phase
	newPhase := oldPhase
	switch oldPhase {
	case "integrating", "change_requests", "live_test_offer", "live_testing", "awaiting_release":
		newPhase = "integration"
	}
	revisionID := ID("revision")
	prefix := filepath.Join("history", revisionID)
	files := map[string][]byte{}
	terminal := d.State.Status == "completed" || d.State.Status == "archived"
	if !terminal {
		if old, err := os.ReadFile(filepath.Join(d.Dir, "WORKSPACE.md")); err == nil {
			files[filepath.Join(prefix, "WORKSPACE.md")] = old
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		for _, name := range []string{"WORKFLOW.md", "AGENTS.md"} {
			if old, err := os.ReadFile(filepath.Join(d.Dir, name)); err == nil {
				files[filepath.Join(prefix, name)] = old
			} else if !errors.Is(err, os.ErrNotExist) {
				return false, err
			}
		}
		if entries, err := os.ReadDir(filepath.Join(d.Dir, "prompts")); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := filepath.Join("prompts", entry.Name())
				if old, readErr := os.ReadFile(filepath.Join(d.Dir, name)); readErr == nil {
					files[filepath.Join(prefix, name)] = old
				} else if !errors.Is(readErr, os.ErrNotExist) {
					return false, readErr
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	d.State.Workflow.ID = "plan-first"
	d.State.Workflow.Version++
	d.State.Workflow.Phase = newPhase
	d.State.Workflow.Capabilities = builtinWorkflowCapabilities("plan-first")
	d.State.ChangeRequestMode = ""
	if d.State.Integration != nil {
		if d.State.Integration.Target == "" {
			d.State.Integration.Target = d.State.Base.Ref
		}
		if d.State.Integration.Strategy == "" {
			d.State.Integration.Strategy = "merge"
		}
	}
	if p := d.State.PendingDecision; p != nil && p.Kind == "live-testing" {
		p.Superseded = true
		d.State.Decisions = append(d.State.Decisions, *p)
		d.State.PendingDecision = nil
	}
	templateState := "not_applicable"
	if !terminal {
		templateState = "embedded_fallback"
		if rendered, available := legacyWorkflowSnapshots(d, files); available {
			files = rendered
			d.State.Workflow.TemplateDigest = digest(files["WORKFLOW.md"])
		} else {
			templateState = "unavailable"
		}
	}
	manifest := map[string]any{
		"reason": "workflow_auto_migration", "from": "issue-resolution", "to": "plan-first",
		"phase_map": map[string]string{oldPhase: newPhase},
		"changes":   []string{"workflow id and capabilities", "workflow phase", "change request mode cleared", "legacy change-request, live-test, and release state preserved"},
		"templates": templateState,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return false, err
	}
	if !terminal {
		files[filepath.Join(prefix, "migration.json")] = manifestBytes
		d.PendingFiles = files
		d.Body += "\n\nWorkflow auto-migrated from issue-resolution to plan-first (" + oldPhase + " → " + newPhase + "). Manifest: " + filepath.ToSlash(filepath.Join(prefix, "migration.json")) + "\n"
	}
	return true, nil
}

func legacyWorkflowSnapshots(d *Document, files map[string][]byte) (map[string][]byte, bool) {
	svc := &Service{Root: d.State.ProjectRoot}
	workflowPath := "workflows/plan-first/WORKFLOW.md.tmpl"
	templateBytes, err := svc.workflowTemplate(workflowPath)
	if err != nil {
		return files, false
	}
	workflow, err := renderTemplate(workflowPath, templateBytes, d.State)
	if err != nil {
		return files, false
	}
	entries, err := fs.ReadDir(templates, "templates/workflows/plan-first/prompts")
	if err != nil {
		return files, false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md.tmpl") {
			continue
		}
		name := entry.Name()
		promptPath := filepath.Join(d.State.ProjectRoot, ".workspace", "templates", "workflows", "plan-first", "prompts", name)
		prompt, readErr := os.ReadFile(promptPath)
		if readErr != nil {
			prompt, readErr = templates.ReadFile("templates/workflows/plan-first/prompts/" + name)
		}
		if readErr != nil {
			return files, false
		}
		files[filepath.Join("prompts", name)] = prompt
	}
	files["WORKFLOW.md"] = workflow
	return files, true
}
