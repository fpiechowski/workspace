package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

type SessionOptions struct {
	Agent, Worktree, Parent, Profile, OperationKey, Task, PromptTemplate, ResumeSession string
	ReadOnly                                                                            bool
}
type promptData struct{ WorkspaceID, WorkspaceDir, AgentID, SessionID, ParentAgentID, BaseCommit string }

// Selection occurs under the project lock, including all workspace reservations.
func (s *Service) chooseRoute(cfg Config, profile string) (Route, error) {
	decision, err := s.assessRoutes(cfg, profile)
	if err != nil {
		return Route{}, err
	}
	for _, candidate := range decision.Candidates {
		if candidate.Route.ID == decision.Selected {
			return candidate.Route, nil
		}
	}
	return Route{}, fail("no_route", "no eligible route in profile %q; use workspace profile explain", profile)
}
func (s *Service) StartSession(ctx context.Context, selector string, opt SessionOptions) (Session, error) {
	var out Session
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
		if id != "" {
			session, err := findSession(d, id)
			if err != nil {
				return err
			}
			out = *session
			return nil
		}
		a, err := findAgent(d, opt.Agent)
		if err != nil {
			return err
		}
		if opt.ReadOnly && a.Role != "planner" {
			return fail("invalid_role", "read-only sessions require a planning/analysis persona")
		}
		if d.State.Status == "completed" || d.State.Status == "archived" {
			return fail("workspace_closed", "workspace is closed")
		}
		if d.State.Status == "paused" {
			return fail("workspace_paused", "resume workspace before starting sessions")
		}
		if a.Role != "orchestrator" && d.State.Workflow == nil {
			return decisionRequired("select a workflow before delegation", "issue-resolution")
		}
		parent := opt.Parent
		if parent == "" && a.Role != "orchestrator" {
			parent = d.State.OrchestratorAgentID
		}
		if parent != "" {
			p, err := findAgent(d, parent)
			if err != nil {
				return err
			}
			parent = p.ID
			if parent == a.ID {
				return fail("invalid_parent", "agent cannot delegate to itself")
			}
		}
		cwd, wtID, wtName, base := d.Dir, "", "", d.State.Base.Commit
		if a.Role != "orchestrator" {
			w, err := findWorktree(d, opt.Worktree)
			if err != nil {
				return err
			}
			if w.State != "ready" {
				return fail("worktree_not_ready", "worktree is %s; inspect/reconcile first", w.State)
			}
			if err := verifyWorktree(ctx, w); err != nil {
				return err
			}
			cwd, wtID, wtName, base = w.Path, w.ID, w.Name, w.BaseCommit
		} else if opt.Worktree != "" {
			return fail("invalid_worktree", "orchestrator works in workspace directory")
		}
		for _, session := range d.Registry.Sessions {
			if !session.Active() {
				continue
			}
			if session.AgentID == a.ID {
				return fail("agent_busy", "agent already has active session %s", session.ID)
			}
			if wtID != "" && session.WorktreeID == wtID && !session.ReadOnly && !opt.ReadOnly {
				return fail("worktree_busy", "worktree is owned by session %s", session.ID)
			}
		}
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		if a.Role != "orchestrator" && d.State.Workflow != nil {
			limit := cfg.Workflows[d.State.Workflow.ID].MaxParallelTasks
			if limit == 0 {
				limit = 3
			}
			active := 0
			for _, p := range d.Registry.Sessions {
				if p.Active() && p.AgentSnapshot.Role != "orchestrator" {
					active++
				}
			}
			if active >= limit {
				return fail("parallel_limit", "workflow already has %d active workers", active)
			}
		}
		profile := opt.Profile
		if opt.Task != "" && profile == "" {
			task, err := findTask(d, opt.Task)
			if err != nil {
				return err
			}
			profile = task.Profile
		}
		if profile == "" {
			profile = a.Profile
			if a.Role == "orchestrator" {
				profile = workflowProfile(cfg, d, a.Role, profile)
			}
		}
		thread := ""
		if opt.ResumeSession != "" {
			prior, err := findSession(d, opt.ResumeSession)
			if err != nil {
				return err
			}
			if prior.AgentID != a.ID || prior.Active() {
				return fail("invalid_resume", "resume requires a terminated session of the same agent")
			}
			if prior.ClientThreadID != "" {
				if _, ok := cfg.Clients[prior.Route.Client]; !ok {
					return fail("client_unavailable", "original client is no longer configured")
				}
				cfg.Profiles = map[string]Profile{profile: {Routes: []Route{prior.Route}}}
				thread = prior.ClientThreadID
			}
		}
		route, err := s.chooseRoute(cfg, profile)
		if err != nil {
			return err
		}
		decision, err := s.assessRoutes(cfg, profile)
		if err != nil {
			return err
		}
		client, ok := cfg.Clients[route.Client]
		if !ok {
			return fail("client_unavailable", "original client is no longer configured")
		}
		id = ID("sess")
		promptTemplate := a.PromptTemplate
		if opt.PromptTemplate != "" {
			if err := validateName(opt.PromptTemplate); err != nil {
				return err
			}
			promptTemplate = opt.PromptTemplate
		}
		b, err := os.ReadFile(filepath.Join(d.Dir, "prompts", promptTemplate+".md.tmpl"))
		if err != nil {
			return err
		}
		t, err := template.New(a.PromptTemplate).Option("missingkey=error").Parse(string(b))
		if err != nil {
			return err
		}
		var prompt bytes.Buffer
		if a.Role != "orchestrator" {
			instructions, err := os.ReadFile(filepath.Join(d.Dir, "prompts", "worker.AGENTS.md"))
			if err != nil {
				return err
			}
			prompt.Write(instructions)
			prompt.WriteString("\n\n")
		}
		if err = t.Execute(&prompt, promptData{d.State.ID, d.Dir, a.ID, id, parent, base}); err != nil {
			return err
		}
		prompt.WriteString("\n\nAgent definition instructions:\n" + a.Instructions + "\n")
		if opt.ReadOnly {
			prompt.WriteString("\nRead-only repository access: do not edit tracked files, index, branches or commits. Other sessions may change this checkout while you read it. Record observed commit IDs; request a separate checkout if a stable revision is required. Write your report only under work-products/" + id + "/ and submit those explicit paths. This is a cooperation contract, not an OS sandbox.\n")
		}
		if opt.Task != "" {
			task, err := findTask(d, opt.Task)
			if err != nil {
				return err
			}
			prompt.WriteString("\nAssigned task: " + task.ID + "\nRead " + filepath.Join(d.Dir, "tasks", task.ID+".md") + ".\n")
			for _, depID := range task.DependsOn {
				dep, _ := findTask(d, depID)
				if dep.AcceptedHandoff != "" {
					handoff, _ := findHandoff(d, dep.AcceptedHandoff)
					if handoff != nil {
						for _, aid := range handoff.ArtifactIDs {
							artifact, _ := findArtifact(d, aid)
							if artifact != nil {
								prompt.WriteString("Dependency artifact: " + filepath.Join(d.Dir, artifact.Path) + "\n")
							}
						}
					}
				}
			}
		}
		promptFile := filepath.Join(d.Dir, "prompts", id+".md")
		if err := atomicWrite(promptFile, prompt.Bytes()); err != nil {
			return err
		}
		launchArgv := client.LaunchArgv
		if thread != "" && client.Adapter != "codex" {
			if len(client.ResumeArgv) == 0 {
				thread = ""
			} else {
				launchArgv = client.ResumeArgv
			}
		}
		argv := make([]string, len(launchArgv))
		replace := strings.NewReplacer("{model}", route.Model, "{prompt_file}", promptFile, "{prompt}", prompt.String(), "{thread_id}", thread)
		for i, arg := range launchArgv {
			argv[i] = replace.Replace(arg)
		}
		argv[0], err = exec.LookPath(argv[0])
		if err != nil {
			return err
		}
		out = Session{ID: id, AgentID: a.ID, AgentSnapshot: a, ParentAgentID: parent, WorktreeID: wtID, Profile: profile, Route: route, Argv: argv, CWD: cwd, PromptFile: promptFile, State: "starting", CreatedAt: time.Now().UTC()}
		out.ClientSnapshot = client
		out.ReadOnly = opt.ReadOnly
		out.RoutingDecision = &decision
		out.ClientThreadID = thread
		if err := bindTask(ctx, d, &out, opt.Task); err != nil {
			return err
		}
		d.Registry.Sessions = append(d.Registry.Sessions, out)
		d.remember(opt.OperationKey, opt, out.ID)
		if err := saveDocument(d); err != nil {
			return err
		}
		pane, launchErr := s.Runtime.Launch(ctx, Launch{ProjectRoot: s.Root, WorkspaceID: d.State.ID, WorkspaceDir: d.Dir, SessionID: id, WorktreeName: wtName, CWD: cwd, Executable: s.Executable, Orchestrator: a.Role == "orchestrator"})
		if launchErr != nil {
			out.State = "failed"
			var ce *Error
			if errors.As(launchErr, &ce) && ce.Code == "launch_uncertain" {
				out.State = "starting"
			}
			out.Error = launchErr.Error()
			if out.TaskID != "" && out.State == "failed" {
				task, _ := findTask(d, out.TaskID)
				task.State = "blocked"
				task.Reason = launchErr.Error()
			}
		} else {
			out.PaneID = pane.ID
			out.WindowID = pane.WindowID
		}
		d.Registry.Sessions[len(d.Registry.Sessions)-1] = out
		if err := saveResource(d, opt.OperationKey, out); err != nil {
			return err
		}
		return launchErr
	})
	return out, err
}
func (s *Service) StartOrchestrator(ctx context.Context, selector, key string) (Session, error) {
	return s.StartSession(ctx, selector, SessionOptions{Agent: "orchestrator", OperationKey: key})
}

// ExecuteSession is invoked by tmux, not by the agent client. The claim prevents
// replaying the same concrete Session even if its runner command is executed twice.
func (s *Service) ExecuteSession(ctx context.Context, selector, id string, in io.Reader, out, errOut io.Writer) error {
	var session Session
	var state Workspace
	var dir string
	err := s.With(ctx, selector, func(d *Document) error {
		p, err := findSession(d, id)
		if err != nil {
			return err
		}
		if p.State != "starting" {
			return fail("session_claimed", "session %s is %s", id, p.State)
		}
		if p.PaneID == "" {
			pane, err := s.recoverPane(ctx, d, *p)
			if err != nil {
				return err
			}
			p.PaneID, p.WindowID = pane.ID, pane.WindowID
		}
		p.State = "running"
		session = *p
		state = d.State
		dir = d.Dir
		return saveDocument(d)
	})
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, session.Argv[0], session.Argv[1:]...)
	cmd.Dir = session.CWD
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "WORKSPACE_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "WORKSPACE_PROJECT_DIR="+s.Root, "WORKSPACE_PROJECT_ID="+state.ProjectID, "WORKSPACE_ID="+state.ID, "WORKSPACE_DIR="+dir, "WORKSPACE_AGENT_ID="+session.AgentID, "WORKSPACE_SESSION_ID="+id, "WORKSPACE_ORCHESTRATOR_ID="+state.OrchestratorAgentID, "WORKSPACE_PARENT_AGENT_ID="+session.ParentAgentID, "WORKSPACE_WORKTREE_ID="+session.WorktreeID, "WORKSPACE_ROLE="+session.AgentSnapshot.Role)
	cmd.Env = append(cmd.Env, "WORKSPACE_TASK_ID="+session.TaskID)
	if session.ReadOnly {
		cmd.Env = append(cmd.Env, "WORKSPACE_READ_ONLY=1")
	}
	if tmux, ok := s.Runtime.(Tmux); ok {
		cmd.Env = append(cmd.Env, "WORKSPACE_TMUX_SOCKET="+tmux.Socket)
	}
	// Make the same installed binary available when the agent invokes workspace.
	cmd.Env = replaceEnv(cmd.Env, "PATH", filepath.Dir(s.Executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut
	var runErr error
	if session.ClientSnapshot.Adapter == "codex" {
		runErr = s.runCodex(ctx, selector, session, cmd, in, out, errOut)
	} else {
		runErr = cmd.Run()
	}
	code := 0
	if runErr != nil {
		code = 1
		var exit *exec.ExitError
		if errors.As(runErr, &exit) {
			code = exit.ExitCode()
		}
	}
	finishErr := s.With(context.Background(), selector, func(d *Document) error {
		p, err := findSession(d, id)
		if err != nil {
			return err
		}
		if p.State != "running" {
			return nil
		}
		now := time.Now().UTC()
		p.FinishedAt = &now
		p.ExitCode = &code
		p.State = "exited"
		if runErr != nil {
			p.State = "failed"
			p.Error = runErr.Error()
		}
		if p.TaskID != "" {
			task, err := findTask(d, p.TaskID)
			if err != nil {
				return err
			}
			if task.Attempt == p.TaskAttempt && task.SessionID == p.ID && task.State == "running" {
				task.State = "blocked"
				task.Reason = "session ended without a handoff"
			}
		}
		return saveDocument(d)
	})
	if finishErr != nil {
		return finishErr
	}
	return runErr
}
func replaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := []string{}
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return append(out, prefix+value)
}
func (s *Service) StopSession(ctx context.Context, selector, id string, keys ...string) (Session, error) {
	if key := mutationKey(keys); key != "" {
		return effect(s, ctx, selector, key, []any{"session.stop", id}, func(d *Document) error { return s.requireSessionOwner(d, id) }, func() (Session, error) { return s.StopSession(ctx, selector, id) })
	}
	var out Session
	err := s.With(ctx, selector, func(d *Document) error {
		if s.Actor.SessionID != id {
			if err := s.requireOrchestrator(d); err != nil {
				return err
			}
		} else if _, err := s.actor(d); err != nil {
			return err
		}
		p, err := findSession(d, id)
		if err != nil {
			return err
		}
		out = *p
		if !p.Active() {
			return nil
		}
		if p.PaneID == "" {
			return fail("launch_uncertain", "no pane recorded; inspect tmux and run reconcile")
		}
		pane, err := s.Runtime.Inspect(ctx, p.PaneID)
		if err != nil {
			var ce *Error
			if !errors.As(err, &ce) || ce.Code != "pane_missing" {
				return err
			}
		} else if pane.SessionID != id {
			return fail("pane_mismatch", "pane is not owned by this session")
		} else if err := s.Runtime.Stop(ctx, p.PaneID); err != nil {
			return err
		}
		now := time.Now().UTC()
		p.State = "stopped"
		p.FinishedAt = &now
		blockInterruptedTask(d, p)
		out = *p
		return saveDocument(d)
	})
	return out, err
}
func (s *Service) ResumeAgent(ctx context.Context, selector, agent, key string) (Session, error) {
	var opt SessionOptions
	err := s.With(ctx, selector, func(d *Document) error {
		a, err := findAgent(d, agent)
		if err != nil {
			return err
		}
		opt = SessionOptions{Agent: a.ID, OperationKey: key}
		for _, session := range d.Registry.Sessions {
			if session.AgentID == a.ID {
				opt.Worktree = session.WorktreeID
				opt.Parent = session.ParentAgentID
				opt.Profile = session.Profile
				opt.Task = session.TaskID
				opt.ResumeSession = session.ID
				opt.ReadOnly = session.ReadOnly
			}
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return s.StartSession(ctx, selector, opt)
}
func (s *Service) Reconcile(ctx context.Context, selector string, keys ...string) (Status, error) {
	if key := mutationKey(keys); key != "" {
		return effect(s, ctx, selector, key, "reconcile", s.requireOrchestrator, func() (Status, error) { return s.Reconcile(ctx, selector) })
	}
	var out Status
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		changed := false
		for i := range d.Registry.Services {
			p := &d.Registry.Services[i]
			if !p.Active() {
				continue
			}
			var pane Pane
			var err error
			if p.PaneID != "" {
				pane, err = s.Runtime.Inspect(ctx, p.PaneID)
			} else if rt, ok := s.Runtime.(interface {
				Recover(context.Context, Launch) (Pane, error)
			}); ok {
				pane, err = rt.Recover(ctx, Launch{Service: true, ProjectRoot: s.Root, WorkspaceID: d.State.ID, SessionID: p.ID, Executable: s.Executable})
				if err == nil {
					p.PaneID = pane.ID
					changed = true
				}
			} else {
				continue
			}
			if err != nil {
				var ce *Error
				if !errors.As(err, &ce) || ce.Code != "pane_missing" {
					return err
				}
			}
			if err != nil || pane.Dead || pane.SessionID != p.ID {
				p.State = "interrupted"
				changed = true
			}
		}
		for i := range d.Registry.Sessions {
			p := &d.Registry.Sessions[i]
			if !p.Active() {
				continue
			}
			if p.PaneID == "" {
				if _, ok := s.Runtime.(interface {
					Recover(context.Context, Launch) (Pane, error)
				}); !ok {
					continue
				}
			}
			var pane Pane
			var err error
			if p.PaneID == "" {
				pane, err = s.recoverPane(ctx, d, *p)
				if err == nil {
					p.PaneID, p.WindowID = pane.ID, pane.WindowID
					changed = true
				}
			} else {
				pane, err = s.Runtime.Inspect(ctx, p.PaneID)
			}
			if err != nil {
				var runtimeError *Error
				if !errors.As(err, &runtimeError) || runtimeError.Code != "pane_missing" {
					return err // An unavailable runtime is not proof that its client stopped.
				}
			}
			if err != nil || pane.Dead || pane.SessionID != p.ID {
				p.State = "interrupted"
				p.Error = "pane absent, dead or no longer owned; inspect local work before retry"
				now := time.Now().UTC()
				p.FinishedAt = &now
				blockInterruptedTask(d, p)
				changed = true
			}
		}
		for i := range d.Registry.Worktrees {
			w := &d.Registry.Worktrees[i]
			if w.State == "removing" {
				if _, err := os.Lstat(w.Path); os.IsNotExist(err) {
					if !contained(filepath.Join(d.Dir, "worktrees"), w.Path) {
						return fail("unsafe_path", "removed worktree outside workspace")
					}
					listing, err := git(ctx, s.Root, "worktree", "list", "--porcelain")
					if err != nil {
						return err
					}
					registered := false
					for _, line := range strings.Split(listing, "\n") {
						if strings.HasPrefix(line, "worktree ") && filepath.Clean(strings.TrimPrefix(line, "worktree ")) == filepath.Clean(w.Path) {
							registered = true
						}
					}
					if registered {
						if _, err := git(ctx, s.Root, "worktree", "remove", "--force", "--", w.Path); err != nil {
							return err
						}
					}
					w.State = "removed"
					changed = true
				}
			}
			if w.State == "creating" && verifyWorktree(ctx, w) == nil {
				w.State = "ready"
				changed = true
			}
		}
		for i := range d.Registry.Checks {
			r := &d.Registry.Checks[i]
			if r.State != "running" {
				continue
			}
			p, err := findSession(d, r.SessionID)
			if err == nil && !p.Active() {
				r.State = "interrupted"
				r.ExitCode = -1
				now := nowUTC()
				r.FinishedAt = &now
				changed = true
			}
		}
		if changed {
			if err := saveDocument(d); err != nil {
				return err
			}
		}
		out = d.Status()
		return nil
	})
	return out, err
}

func (s *Service) recoverPane(ctx context.Context, d *Document, p Session) (Pane, error) {
	r, ok := s.Runtime.(interface {
		Recover(context.Context, Launch) (Pane, error)
	})
	if !ok {
		return Pane{}, fail("launch_uncertain", "runtime cannot recover an unrecorded pane")
	}
	return r.Recover(ctx, Launch{ProjectRoot: s.Root, WorkspaceID: d.State.ID, WorkspaceDir: d.Dir, SessionID: p.ID, CWD: p.CWD, Executable: s.Executable, Orchestrator: p.AgentSnapshot.Role == "orchestrator"})
}
func blockInterruptedTask(d *Document, p *Session) {
	if p.TaskID == "" {
		return
	}
	t, err := findTask(d, p.TaskID)
	if err == nil && t.State == "running" && t.SessionID == p.ID && t.Attempt == p.TaskAttempt {
		t.State, t.Reason = "blocked", "session interrupted; inspect local work before retry"
	}
}
