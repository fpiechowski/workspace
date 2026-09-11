package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) Pause(ctx context.Context, selector string, interrupt bool, keys ...string) (Status, error) {
	if key := mutationKey(keys); key != "" {
		return effect(s, ctx, selector, key, []any{"pause", interrupt}, s.requireOrchestrator, func() (Status, error) { return s.Pause(ctx, selector, interrupt) })
	}
	v, err := s.SetPaused(ctx, selector, true)
	if err != nil {
		return v, err
	}
	if interrupt {
		var services []BackgroundService
		if err := s.With(ctx, selector, func(d *Document) error { services = d.Registry.Services; return nil }); err != nil {
			return v, err
		}
		for _, p := range services {
			if p.Active() {
				if _, err := s.StopService(ctx, selector, p.ID); err != nil {
					return v, err
				}
			}
		}
		// Stop the caller last so delegated sessions cannot outlive this request.
		for _, p := range v.Sessions {
			if p.Active() && p.ID != s.Actor.SessionID {
				if _, err := s.StopSession(ctx, selector, p.ID); err != nil {
					return v, err
				}
			}
		}
		if s.Actor.SessionID != "" {
			if _, err := s.StopSession(ctx, selector, s.Actor.SessionID); err != nil {
				return v, err
			}
		}
	}
	return s.Status(ctx, selector)
}

func (s *Service) Archive(ctx context.Context, selector string, keys ...string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, keys, "archive", &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if d.State.Status == "archived" {
			out = d.Status()
			return nil
		}
		if d.State.Status != "completed" || !d.State.Release.UserConfirmed {
			return fail("release_required", "confirm release before archiving")
		}
		for _, p := range d.Registry.Services {
			if p.Active() {
				return fail("service_active", "stop service %s before archiving", p.ID)
			}
		}
		for _, p := range d.Registry.Sessions {
			if p.Active() {
				return fail("session_active", "stop session %s before archiving", p.ID)
			}
		}
		d.State.Status = "archived"
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}

type CleanItem struct {
	WorktreeID string `json:"worktree_id" yaml:"worktree_id"`
	Path       string `json:"path" yaml:"path"`
	Head       string `json:"head" yaml:"head"`
	Allowed    bool   `json:"allowed" yaml:"allowed"`
	Reason     string `json:"reason,omitempty" yaml:"reason,omitempty"`
	Backup     string `json:"backup,omitempty" yaml:"backup,omitempty"`
	Removed    bool   `json:"removed" yaml:"removed"`
}

func (s *Service) cleanItem(ctx context.Context, d *Document, w Worktree, backup bool) (CleanItem, error) {
	item := CleanItem{WorktreeID: w.ID, Path: w.Path}
	for _, p := range d.Registry.Services {
		if p.Active() && p.WorktreeID == w.ID {
			item.Reason = "active service " + p.ID
			return item, nil
		}
	}
	if d.State.Status != "archived" {
		item.Reason = "archive workspace before cleaning"
		return item, nil
	}
	for _, p := range d.Registry.Sessions {
		if p.Active() && p.WorktreeID == w.ID {
			item.Reason = "active session " + p.ID
			return item, nil
		}
	}
	if err := verifyWorktree(ctx, &w); err != nil {
		item.Reason = err.Error()
		return item, nil
	}
	if !contained(filepath.Join(d.Dir, "worktrees"), w.Path) {
		return item, fail("unsafe_path", "worktree path is outside workspace")
	}
	dirty, err := git(ctx, w.Path, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return item, err
	}
	if dirty != "" {
		item.Reason = "uncommitted or untracked files"
		return item, nil
	}
	item.Head, err = git(ctx, w.Path, "rev-parse", "HEAD")
	if err != nil {
		return item, err
	}
	// Ignored files can still be valuable. Remove them only when an identical
	// immutable handoff artifact exists; dependencies/caches need explicit cleanup.
	ignored, err := git(ctx, w.Path, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return item, err
	}
	for _, name := range strings.Split(ignored, "\x00") {
		if name == "" {
			continue
		}
		path := filepath.Join(w.Path, filepath.FromSlash(name))
		stat, err := os.Lstat(path)
		if err != nil {
			return item, err
		}
		if !stat.Mode().IsRegular() {
			item.Reason = "unpreserved ignored file: " + name
			return item, nil
		}
		if stat.Size() > maxArtifactSize {
			item.Reason = "unpreserved ignored file: " + name
			return item, nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return item, err
		}
		preserved := false
		for _, a := range d.State.Artifacts {
			if a.Digest == digest(b) {
				saved, err := os.ReadFile(filepath.Join(d.Dir, a.Path))
				if err == nil && digest(saved) == a.Digest {
					preserved = true
					break
				}
			}
		}
		if !preserved {
			item.Reason = "unpreserved ignored file: " + name
			return item, nil
		}
	}
	unprotected, err := git(ctx, w.Path, "rev-list", item.Head, "--not", "--remotes", "--exclude=workspace/*", "--branches")
	if err != nil {
		return item, err
	}
	if unprotected != "" {
		if !backup {
			item.Reason = "commits are not reachable outside workspace branches; use --backup"
			return item, nil
		}
		item.Backup = filepath.Join(d.Dir, "artifacts", "git-backups", w.ID+"-"+item.Head+".bundle")
	}
	item.Allowed = true
	return item, nil
}

func (s *Service) Clean(ctx context.Context, selector string, dryRun, backup bool, keys ...string) ([]CleanItem, error) {
	if key := mutationKey(keys); key != "" && !dryRun {
		return effect(s, ctx, selector, key, []any{"clean", backup}, s.requireOrchestrator, func() ([]CleanItem, error) {
			if _, err := s.Reconcile(ctx, selector); err != nil {
				return nil, err
			}
			return s.Clean(ctx, selector, dryRun, backup)
		})
	}
	var out []CleanItem
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		for _, w := range d.Registry.Worktrees {
			if w.State == "removed" {
				continue
			}
			item, err := s.cleanItem(ctx, d, w, backup)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		if dryRun {
			return nil
		}
		for _, item := range out {
			if !item.Allowed {
				return fail("clean_refused", "%s: %s", item.Path, item.Reason)
			}
		}
		for i := range out {
			item := &out[i]
			w, _ := findWorktree(d, item.WorktreeID)
			if item.Backup != "" {
				if err := os.MkdirAll(filepath.Dir(item.Backup), 0700); err != nil {
					return err
				}
				if _, err := os.Stat(item.Backup); os.IsNotExist(err) {
					if _, err := git(ctx, w.Path, "bundle", "create", item.Backup, w.Branch); err != nil {
						return err
					}
				}
				if _, err := git(ctx, w.Path, "bundle", "verify", item.Backup); err != nil {
					return err
				}
				refs, err := git(ctx, w.Path, "bundle", "list-heads", item.Backup, "refs/heads/"+w.Branch)
				if err != nil {
					return err
				}
				if !strings.HasPrefix(refs, item.Head+" ") {
					return fail("backup_invalid", "bundle does not preserve current worktree head")
				}
			}
			w.State = "removing"
			if err := saveDocument(d); err != nil {
				return err
			}
			if _, err := git(ctx, s.Root, "worktree", "remove", "--", w.Path); err != nil {
				return err
			}
			w.State = "removed"
			item.Removed = true
			if err := saveDocument(d); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}
