package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type AgentOptions struct{ Name, Role, Profile, PromptTemplate, Instructions, OperationKey string }

func (s *Service) CreateAgent(ctx context.Context, selector string, opt AgentOptions) (Agent, error) {
	var out Agent
	if err := validateName(opt.Name); err != nil {
		return out, err
	}
	defaults := map[string][2]string{
		"planner": {"frontier", "planning"}, "implementer": {"implementation", "implementation"},
		"integrator": {"implementation", "integration"}, "tester": {"live-testing", "live-testing"},
	}
	def, ok := defaults[opt.Role]
	if !ok {
		return out, fail("invalid_role", "supported worker roles: planner, implementer, integrator, tester")
	}
	if opt.PromptTemplate == "" {
		opt.PromptTemplate = def[1]
	}
	if err := validateName(opt.PromptTemplate); err != nil {
		return out, err
	}
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		id, err := d.previous(opt.OperationKey, opt)
		if err != nil {
			return err
		}
		if found, err := replayResource(d, opt.OperationKey, &out); found || err != nil {
			return err
		}
		if id == "" && (d.State.Status == "completed" || d.State.Status == "archived") {
			return fail("workspace_closed", "workspace is closed")
		}
		for _, a := range d.Registry.Agents {
			if a.ID == id {
				out = a
				return nil
			}
			if a.Name == opt.Name {
				return fail("agent_exists", "agent %q already exists", opt.Name)
			}
		}
		if _, err := os.Stat(filepath.Join(d.Dir, "prompts", opt.PromptTemplate+".md.tmpl")); err != nil {
			return fail("template_not_found", "%s", opt.PromptTemplate)
		}
		profile := opt.Profile
		if profile == "" {
			cfg, err := s.Config()
			if err != nil {
				return err
			}
			profile = workflowProfile(cfg, d, opt.Role, def[0])
		}
		out = Agent{ID("agent"), opt.Name, opt.Role, profile, opt.PromptTemplate, opt.Instructions}
		d.Registry.Agents = append(d.Registry.Agents, out)
		d.remember(opt.OperationKey, opt, out.ID)
		return saveResource(d, opt.OperationKey, out)
	})
	return out, err
}
func findAgent(d *Document, id string) (Agent, error) {
	for _, a := range d.Registry.Agents {
		if a.ID == id || a.Name == id {
			return a, nil
		}
	}
	return Agent{}, fail("agent_not_found", "unknown agent %q", id)
}
func findSession(d *Document, id string) (*Session, error) {
	for i := range d.Registry.Sessions {
		if d.Registry.Sessions[i].ID == id {
			return &d.Registry.Sessions[i], nil
		}
	}
	// Legacy runtimes and scripts may still address the old concrete session
	// identifier. Migration preserves it as Run.ID, making it a durable alias.
	if r, err := findRun(d, id); err == nil {
		for i := range d.Registry.Sessions {
			if d.Registry.Sessions[i].ID == r.SessionID {
				return &d.Registry.Sessions[i], nil
			}
		}
	}
	return nil, fail("session_not_found", "unknown session %q", id)
}

func findRun(d *Document, id string) (*Run, error) {
	for i := range d.Registry.Runs {
		if d.Registry.Runs[i].ID == id {
			return &d.Registry.Runs[i], nil
		}
	}
	return nil, fail("run_not_found", "unknown run %q", id)
}

func currentRun(d *Document, p *Session) (*Run, error) {
	if p.CurrentRunID == "" {
		return nil, fail("session_idle", "session %s has no active run", p.ID)
	}
	r, err := findRun(d, p.CurrentRunID)
	if err != nil || r.SessionID != p.ID || !r.Active() {
		return nil, fail("stale_run", "session %s no longer owns run %s", p.ID, p.CurrentRunID)
	}
	return r, nil
}

func provenanceRun(d *Document, p *Session) (*Run, error) {
	resolve := func(id string) (*Run, error) {
		r, err := findRun(d, id)
		if err != nil {
			return nil, err
		}
		if r.SessionID != p.ID {
			return nil, fail("stale_run", "run %s does not belong to session %s", id, p.ID)
		}
		return r, nil
	}
	if p.CurrentRunID != "" {
		return resolve(p.CurrentRunID)
	}
	if p.LastRunID != "" {
		return resolve(p.LastRunID)
	}
	return nil, fail("run_not_found", "session %s has no execution history", p.ID)
}

func (d *Document) syncSession(p *Session) {
	p.RunCount = 0
	var latest, current *Run
	for i := range d.Registry.Runs {
		r := &d.Registry.Runs[i]
		if r.SessionID != p.ID {
			continue
		}
		p.RunCount++
		if latest == nil || r.CreatedAt.After(latest.CreatedAt) || r.CreatedAt.Equal(latest.CreatedAt) && r.ID > latest.ID {
			latest = r
		}
		if r.ID == p.CurrentRunID {
			current = r
		}
	}
	if latest != nil {
		p.LastRunID = latest.ID
	} else {
		p.LastRunID = ""
	}
	selected := latest
	if current != nil && current.Active() {
		selected = current
	}
	if selected == nil {
		p.CurrentRunID = ""
		p.LifecycleState = "idle"
		return
	}
	if current == nil || !current.Active() {
		p.CurrentRunID = ""
	}
	p.Profile, p.Route, p.RoutingDecision = selected.Profile, selected.Route, selected.RoutingDecision
	p.Argv, p.CWD, p.PromptFile = append([]string(nil), selected.Argv...), selected.CWD, selected.PromptFile
	p.RunState, p.State, p.PaneID, p.WindowID = selected.State, selected.State, selected.PaneID, selected.WindowID
	p.FinishedAt, p.ExitCode, p.Error, p.ClientState = selected.FinishedAt, selected.ExitCode, selected.Error, selected.ClientState
	p.OpenCodeEndpoint = selected.OpenCodeEndpoint
	if selected.ClientThreadID != "" {
		p.ClientThreadID = selected.ClientThreadID
	}
	p.LastActiveAt = selected.CreatedAt
	if selected.FinishedAt != nil {
		p.LastActiveAt = *selected.FinishedAt
	}
	if p.ClosedAt != nil {
		p.LifecycleState = "closed"
	} else if current != nil && current.Active() {
		p.LifecycleState = "active"
	} else {
		p.CurrentRunID = ""
		p.LifecycleState = "idle"
	}
}

func (d *Document) syncSessions() {
	for i := range d.Registry.Sessions {
		d.syncSession(&d.Registry.Sessions[i])
	}
}
func findWorktree(d *Document, id string) (*Worktree, error) {
	for i := range d.Registry.Worktrees {
		w := &d.Registry.Worktrees[i]
		if w.ID == id || w.Name == id {
			return w, nil
		}
	}
	return nil, fail("worktree_not_found", "unknown worktree %q", id)
}

type WorktreeOptions struct{ Name, Base, Purpose, OperationKey string }

func (s *Service) CreateWorktree(ctx context.Context, selector string, opt WorktreeOptions) (Worktree, error) {
	var out Worktree
	if err := validateName(opt.Name); err != nil {
		return out, err
	}
	if opt.Purpose == "" {
		opt.Purpose = "implementation"
	}
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		id, err := d.previous(opt.OperationKey, opt)
		if err != nil {
			return err
		}
		if found, err := replayResource(d, opt.OperationKey, &out); found || err != nil {
			return err
		}
		if id == "" && (d.State.Status == "completed" || d.State.Status == "archived") {
			return fail("workspace_closed", "workspace is closed")
		}
		for _, w := range d.Registry.Worktrees {
			if w.ID == id {
				out = w
				return nil
			}
			if w.Name == opt.Name {
				return fail("worktree_exists", "worktree %q exists", opt.Name)
			}
		}
		base := d.State.Base.Commit
		if opt.Base != "" {
			base, err = git(ctx, s.Root, "rev-parse", "--verify", "--end-of-options", opt.Base+"^{commit}")
			if err != nil {
				return err
			}
		}
		path := filepath.Join(d.Dir, "worktrees", opt.Name)
		if _, err := os.Lstat(path); err == nil {
			return fail("path_exists", "%s already exists", path)
		} else if !os.IsNotExist(err) {
			return err
		}
		out = Worktree{ID("wt"), opt.Name, path, "workspace/" + d.State.ID + "/" + opt.Name, base, opt.Purpose, "creating"}
		d.Registry.Worktrees = append(d.Registry.Worktrees, out)
		d.remember(opt.OperationKey, opt, out.ID)
		// Reserve identity before invoking Git. A crash leaves a visible pending worktree.
		if err := saveDocument(d); err != nil {
			return err
		}
		_, err = git(ctx, s.Root, "worktree", "add", "-b", out.Branch, "--", path, base)
		if err != nil {
			out.State = "failed"
		} else {
			out.State = "ready"
		}
		d.Registry.Worktrees[len(d.Registry.Worktrees)-1] = out
		if saveErr := saveResource(d, opt.OperationKey, out); saveErr != nil {
			return saveErr
		}
		return err
	})
	return out, err
}

func verifyWorktree(ctx context.Context, w *Worktree) error {
	real, err := filepath.EvalSymlinks(w.Path)
	if err != nil {
		return fail("worktree_missing", "%s", w.Path)
	}
	abs, err := filepath.Abs(w.Path)
	if err != nil {
		return err
	}
	if real != abs {
		return fail("worktree_moved", "worktree path changed or became a symlink")
	}
	root, err := git(ctx, w.Path, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if filepath.Clean(root) != filepath.Clean(w.Path) {
		return fail("worktree_missing", "directory is no longer a worktree")
	}
	branch, err := git(ctx, w.Path, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(branch) != w.Branch {
		return fail("worktree_changed", "expected branch %s, found %s", w.Branch, branch)
	}
	return nil
}
