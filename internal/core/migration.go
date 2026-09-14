package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type RevisionOptions struct {
	Input            string
	Source           string
	Reason           string
	ExpectedRevision int
	OperationKey     string
}

func invalidateRevision(d *Document, reason string) {
	for i := range d.State.Tasks {
		t := &d.State.Tasks[i]
		t.Attempt++
		t.State = "pending"
		t.AcceptedHandoff = ""
		t.SessionID = ""
		t.RunID = ""
		t.InputDigest = ""
		t.Reason = reason
	}
	for i := range d.State.ChangeRequests {
		d.State.ChangeRequests[i].State = "outdated"
	}
	d.State.Integration = nil
	d.State.LiveTest = LiveTest{}
	d.State.PendingDecision = nil
	if d.State.Workflow != nil {
		d.State.Workflow.Phase = "planning"
	}
}
func revisionGate(s *Service, d *Document, revision int, reason string) error {
	if err := s.requireOrchestrator(d); err != nil {
		return err
	}
	if d.State.Revision != revision {
		return fail("revision_conflict", "expected revision %d, current %d", revision, d.State.Revision)
	}
	if strings.TrimSpace(reason) == "" {
		return fail("reason_required", "explain the revision change")
	}
	if d.State.Status != "paused" || d.State.Release.UserConfirmed {
		return fail("workspace_not_paused", "pause unreleased work before revising its inputs or workflow")
	}
	for _, p := range d.Registry.Sessions {
		if p.Active() && p.AgentSnapshot.Role != "orchestrator" {
			return fail("task_busy", "stop worker %s before revising inputs", p.ID)
		}
	}
	for _, p := range d.Registry.Services {
		if p.Active() {
			return fail("service_active", "stop service %s before revising workspace", p.ID)
		}
	}
	return nil
}
func (s *Service) UpdateInput(ctx context.Context, selector string, opt RevisionOptions) (Status, error) {
	var out Status
	if strings.TrimSpace(opt.Input) == "" {
		return out, fail("input_required", "provide new issue contents")
	}
	err := mutate(s, ctx, selector, []string{opt.OperationKey}, []any{"input.update", opt}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := revisionGate(s, d, opt.ExpectedRevision, opt.Reason); err != nil {
			return err
		}
		old, err := os.ReadFile(filepath.Join(d.Dir, d.State.Input.Snapshot))
		if err != nil {
			return err
		}
		id := ID("revision")
		prefix := filepath.Join("history", id)
		state, err := encodeDocument(d)
		if err != nil {
			return err
		}
		d.PendingFiles = map[string][]byte{
			filepath.Join(prefix, "WORKSPACE.md"): state,
			filepath.Join(prefix, "issue.md"):     old,
			filepath.Join(prefix, "REASON.md"):    []byte(opt.Reason),
			d.State.Input.Snapshot:                []byte(opt.Input),
		}
		d.State.Input.Source = opt.Source
		invalidateRevision(d, opt.Reason)
		d.Body += "\n\nInput revised: " + opt.Reason + ". Previous state: " + filepath.ToSlash(prefix) + "/WORKSPACE.md\n"
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}

func (s *Service) MigrateWorkflow(ctx context.Context, selector string, opt RevisionOptions) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, []string{opt.OperationKey}, []any{"workflow.migrate", opt}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := revisionGate(s, d, opt.ExpectedRevision, opt.Reason); err != nil {
			return err
		}
		if d.State.Workflow == nil {
			return fail("workflow_required", "select a workflow first")
		}
		// Render in memory, then commit all changed snapshots and state through
		// the same write-ahead record. Existing Session prompts remain immutable.
		paths := map[string]string{"AGENTS.md": "orchestrator.AGENTS.md.tmpl", "prompts/worker.AGENTS.md": "worker.AGENTS.md.tmpl", "WORKFLOW.md": "workflows/issue-resolution/WORKFLOW.md.tmpl"}
		for _, name := range []string{"orchestrator", "planning", "implementation", "integration", "live-testing"} {
			paths["prompts/"+name+".md.tmpl"] = "workflows/issue-resolution/prompts/" + name + ".md.tmpl"
		}
		id := ID("revision")
		prefix := filepath.Join("history", id)
		files := map[string][]byte{}
		changes := map[string]map[string]string{}
		for target, source := range paths {
			var next []byte
			var err error
			if strings.HasSuffix(target, ".tmpl") {
				next, err = os.ReadFile(filepath.Join(s.Root, ".workspace", "templates", filepath.FromSlash(source)))
			} else {
				next, err = s.render(source, d.State)
			}
			if err != nil {
				return err
			}
			old, err := os.ReadFile(filepath.Join(d.Dir, filepath.FromSlash(target)))
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if digest(old) == digest(next) {
				continue
			}
			files[filepath.FromSlash(target)] = next
			files[filepath.Join(prefix, filepath.FromSlash(target))] = old
			changes[target] = map[string]string{"before": digest(old), "after": digest(next)}
		}
		if len(changes) == 0 {
			out = d.Status()
			return nil
		}
		state, err := encodeDocument(d)
		if err != nil {
			return err
		}
		files[filepath.Join(prefix, "WORKSPACE.md")] = state
		manifest, _ := json.MarshalIndent(map[string]any{"reason": opt.Reason, "changes": changes}, "", "  ")
		files[filepath.Join(prefix, "migration.json")] = manifest
		d.PendingFiles = files
		d.State.Workflow.Version++
		if w, ok := files["WORKFLOW.md"]; ok {
			d.State.Workflow.TemplateDigest = digest(w)
		}
		invalidateRevision(d, opt.Reason)
		d.Body += "\n\nWorkflow migrated: " + opt.Reason + ". Change manifest: " + filepath.ToSlash(prefix) + "/migration.json\n"
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}
