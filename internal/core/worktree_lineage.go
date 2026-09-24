package core

import (
	"context"
	"strings"
)

// Record creation-time provenance, never infer it from branches on refresh.
// A named local branch identifies its source even when several branches share
// a commit. A raw revision identifies a source only at a unique worktree tip.
func (s *Service) worktreeParent(ctx context.Context, d *Document, ref, commit string) string {
	if ref == "" {
		return ""
	}
	branch, _ := git(ctx, s.Root, "rev-parse", "--symbolic-full-name", "--verify", "--end-of-options", ref)
	for _, w := range d.Registry.Worktrees {
		if strings.HasPrefix(branch, "refs/heads/") && branch == "refs/heads/"+w.Branch {
			return w.ID
		}
	}
	// The frozen workspace base is shared by independent tasks.
	if commit == d.State.Base.Commit {
		return ""
	}
	parent := ""
	for _, w := range d.Registry.Worktrees {
		head, err := git(ctx, s.Root, "rev-parse", "--verify", "--end-of-options", "refs/heads/"+w.Branch+"^{commit}")
		if err != nil || head != commit {
			continue
		}
		if parent != "" {
			return ""
		}
		parent = w.ID
	}
	return parent
}
