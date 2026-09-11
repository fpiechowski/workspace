package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
)

type StatePatch struct {
	Title  *string `yaml:"title" json:"title"`
	Body   *string `yaml:"body" json:"body"`
	Status *string `yaml:"status" json:"status"`
	Phase  *string `yaml:"phase" json:"phase"`
}

func ParseStatePatch(b []byte) (StatePatch, error) {
	var p StatePatch
	err := strictYAML(b, &p)
	return p, err
}
func (s *Service) UpdateState(ctx context.Context, selector string, expected int, patch StatePatch, keys ...string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, keys, []any{"state.update", expected, patch}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if d.State.Revision != expected {
			return fail("revision_conflict", "expected %d, current %d", expected, d.State.Revision)
		}
		if d.State.Status == "completed" || d.State.Status == "archived" {
			return fail("workspace_closed", "workspace is closed")
		}
		if patch.Title != nil {
			d.State.Title = *patch.Title
		}
		if patch.Body != nil {
			d.Body = *patch.Body
		}
		if patch.Status != nil {
			switch *patch.Status {
			case "active", "paused", "blocked", "needs_attention":
				d.State.Status = *patch.Status
			default:
				return fail("invalid_state", "use workflow/release commands for terminal states")
			}
		}
		if patch.Phase != nil {
			if err := advance(ctx, d, *patch.Phase); err != nil {
				return err
			}
		}
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}

// EditState imports a paused user's edited Markdown, validating its semantic
// changes through the same gates as state update. Runtime fields are immutable.
func (s *Service) EditState(ctx context.Context, selector string, expected int, content []byte, keys ...string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, keys, []any{"state.edit", expected, content}, &out, s.requireUser, func(d *Document) error {
		if err := s.requireUser(d); err != nil {
			return err
		}
		if d.State.Status != "paused" {
			return fail("workspace_not_paused", "pause workspace before editing")
		}
		if expected != d.State.Revision {
			return fail("revision_conflict", "workspace changed during editing")
		}
		edited := &Document{}
		if err := decodeDocument(content, edited); err != nil {
			return err
		}
		// Structured workflow, results and identities are changed through dedicated commands.
		before := d.State
		after := edited.State
		after.Title = before.Title
		if !reflect.DeepEqual(before, after) {
			return fail("invalid_state", "edit may change narrative and title only; use task/workflow commands for structured fields")
		}
		d.State.Title = edited.State.Title
		d.Body = edited.Body
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}
func (s *Service) StateDocument(ctx context.Context, selector string) ([]byte, int, error) {
	var out []byte
	var revision int
	err := s.With(ctx, selector, func(d *Document) error {
		var err error
		out, err = os.ReadFile(filepath.Join(d.Dir, "WORKSPACE.md"))
		revision = d.State.Revision
		return err
	})
	return out, revision, err
}
