package core

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

type BackgroundService struct {
	ID         string   `json:"id" yaml:"id"`
	Name       string   `json:"name" yaml:"name"`
	WorktreeID string   `json:"worktree_id" yaml:"worktree_id"`
	CWD        string   `json:"cwd" yaml:"cwd"`
	Argv       []string `json:"argv" yaml:"argv"`
	PaneID     string   `json:"pane_id" yaml:"pane_id"`
	State      string   `json:"state" yaml:"state"`
	ExitCode   *int     `json:"exit_code,omitempty" yaml:"exit_code,omitempty"`
}

func (p BackgroundService) Active() bool { return p.State == "starting" || p.State == "running" }

type ServiceOptions struct {
	Name, Worktree, OperationKey string
	Argv                         []string
}

func findService(d *Document, id string) (*BackgroundService, error) {
	for i := range d.Registry.Services {
		p := &d.Registry.Services[i]
		if p.ID == id || p.Name == id {
			return p, nil
		}
	}
	return nil, fail("service_not_found", "unknown service %s", id)
}
func (s *Service) serviceRole(d *Document, worktree string) error {
	p, err := s.actor(d)
	if err != nil {
		return err
	}
	if p != nil && p.AgentID != d.State.OrchestratorAgentID && p.WorktreeID != worktree {
		return fail("forbidden", "service must belong to your assigned worktree")
	}
	return nil
}
func (s *Service) StartService(ctx context.Context, selector string, opt ServiceOptions) (BackgroundService, error) {
	var out BackgroundService
	if err := validateName(opt.Name); err != nil {
		return out, err
	}
	if len(opt.Argv) == 0 {
		return out, fail("command_required", "provide service command after --")
	}
	err := s.With(ctx, selector, func(d *Document) error {
		w, err := findWorktree(d, opt.Worktree)
		if err != nil {
			return err
		}
		if err := s.serviceRole(d, w.ID); err != nil {
			return err
		}
		id, err := d.previous(opt.OperationKey, opt)
		if err != nil {
			return err
		}
		if found, err := replayResource(d, opt.OperationKey, &out); found || err != nil {
			return err
		}
		if id != "" {
			p, err := findService(d, id)
			if err != nil {
				return err
			}
			out = *p
			return nil
		}
		if err := rejectNewWorkspaceWork(d, "starting services"); err != nil {
			return err
		}
		if d.State.Status != "active" {
			return fail("workspace_paused", "service start requires an active workspace")
		}
		if err := verifyWorktree(ctx, w); err != nil {
			return err
		}
		for _, p := range d.Registry.Services {
			if p.Name == opt.Name && p.Active() {
				return fail("service_busy", "service %s is already active", p.ID)
			}
		}
		argv := append([]string(nil), opt.Argv...)
		argv[0], err = exec.LookPath(argv[0])
		if err != nil {
			return err
		}
		out = BackgroundService{ID: ID("service"), Name: opt.Name, WorktreeID: w.ID, CWD: w.Path, Argv: argv, State: "starting"}
		d.Registry.Services = append(d.Registry.Services, out)
		d.remember(opt.OperationKey, opt, out.ID)
		if err := saveDocument(d); err != nil {
			return err
		}
		pane, launchErr := s.Runtime.Launch(ctx, Launch{Service: true, ProjectRoot: s.Root, ProjectID: d.State.ProjectID, WorkspaceID: d.State.ID, WorkspaceDir: d.Dir, SessionID: out.ID, WorktreeID: w.ID, WorktreeName: w.Name, CWD: w.Path, Executable: s.Executable})
		if launchErr == nil {
			out.PaneID = pane.ID
		} else {
			var ce *Error
			if !errors.As(launchErr, &ce) || ce.Code != "launch_uncertain" {
				out.State = "failed"
			}
		}
		d.Registry.Services[len(d.Registry.Services)-1] = out
		if err := saveResource(d, opt.OperationKey, out); err != nil {
			return err
		}
		return launchErr
	})
	return out, err
}
func (s *Service) ExecuteService(ctx context.Context, selector, id string, in io.Reader, out, errOut io.Writer) error {
	var p BackgroundService
	err := s.With(ctx, selector, func(d *Document) error {
		r, err := findService(d, id)
		if err != nil {
			return err
		}
		if r.State != "starting" {
			return fail("service_claimed", "service already executed")
		}
		if r.PaneID == "" {
			if rt, ok := s.Runtime.(interface {
				Recover(context.Context, Launch) (Pane, error)
			}); ok {
				w, err := findWorktree(d, r.WorktreeID)
				if err != nil {
					return err
				}
				pane, err := rt.Recover(ctx, Launch{Service: true, ProjectRoot: s.Root, ProjectID: d.State.ProjectID, WorkspaceID: d.State.ID, WorkspaceDir: d.Dir, SessionID: r.ID, WorktreeID: w.ID, WorktreeName: w.Name, CWD: w.Path, Executable: s.Executable})
				if err != nil {
					return err
				}
				r.PaneID = pane.ID
			} else {
				return fail("launch_uncertain", "service pane not recorded")
			}
		}
		r.State = "running"
		p = *r
		return saveDocument(d)
	})
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, p.Argv[0], p.Argv[1:]...)
	cmd.Dir = p.CWD
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut
	// Auxiliary processes do not inherit agent authority from tmux's environment.
	cmd.Env = replaceEnv(os.Environ(), "WORKSPACE_AGENT_ID", "")
	cmd.Env = replaceEnv(cmd.Env, "WORKSPACE_SESSION_ID", "")
	cmd.Env = replaceEnv(cmd.Env, "WORKSPACE_RUN_ID", "")
	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		code = 1
		var e *exec.ExitError
		if errors.As(runErr, &e) {
			code = e.ExitCode()
		}
	}
	err = s.With(context.Background(), selector, func(d *Document) error {
		r, err := findService(d, id)
		if err != nil {
			return err
		}
		if r.State != "running" {
			return nil
		}
		r.State = "exited"
		if runErr != nil {
			r.State = "failed"
		}
		r.ExitCode = &code
		return saveDocument(d)
	})
	return errors.Join(runErr, err)
}
func (s *Service) StopService(ctx context.Context, selector, id string, keys ...string) (BackgroundService, error) {
	if key := mutationKey(keys); key != "" {
		return effect(s, ctx, selector, key, []any{"service.stop", id}, func(d *Document) error {
			p, err := findService(d, id)
			if err != nil {
				return err
			}
			return s.serviceRole(d, p.WorktreeID)
		}, func() (BackgroundService, error) { return s.StopService(ctx, selector, id) })
	}
	var out BackgroundService
	err := s.With(ctx, selector, func(d *Document) error {
		p, err := findService(d, id)
		if err != nil {
			return err
		}
		if err := s.serviceRole(d, p.WorktreeID); err != nil {
			return err
		}
		out = *p
		if !p.Active() {
			return nil
		}
		pane, err := s.Runtime.Inspect(ctx, p.PaneID)
		if err != nil {
			var ce *Error
			if !errors.As(err, &ce) || ce.Code != "pane_missing" {
				return err
			}
		} else if pane.SessionID != p.ID {
			return fail("pane_mismatch", "service no longer owns pane")
		} else if err := s.Runtime.Stop(ctx, p.PaneID); err != nil {
			return err
		}
		p.State = "stopped"
		out = *p
		return saveDocument(d)
	})
	return out, err
}
