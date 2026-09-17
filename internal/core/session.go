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
type promptData struct{ WorkspaceID, WorkspaceDir, AgentID, SessionID, RunID, ParentAgentID, BaseCommit string }

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
		requestOpt := opt
		id, err := d.previous(opt.OperationKey, requestOpt)
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
		var resumePrior *Session
		if opt.ResumeSession != "" {
			resumePrior, err = findSession(d, opt.ResumeSession)
			if err != nil {
				return err
			}
		}
		a, err := findAgent(d, opt.Agent)
		if err != nil {
			if resumePrior == nil || resumePrior.AgentSnapshot.ID == "" {
				return err
			}
			a = resumePrior.AgentSnapshot
		} else if resumePrior != nil && resumePrior.AgentSnapshot.ID != "" {
			// Agent definitions are snapshotted into a logical Session. A later
			// edit to the live persona must not rewrite its resumed prompt.
			a = resumePrior.AgentSnapshot
		}
		if resumePrior != nil {
			if opt.Worktree == "" {
				opt.Worktree = resumePrior.WorktreeID
			}
			if opt.Parent == "" {
				opt.Parent = resumePrior.ParentAgentID
			}
			if opt.Task == "" {
				opt.Task = resumePrior.TaskID
			}
			if opt.Profile == "" {
				opt.Profile = resumePrior.Profile
			}
			if resumePrior.ReadOnly {
				opt.ReadOnly = true
			}
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
			return decisionRequired("select a workflow before delegation", "plan-first")
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
		if opt.Task != "" {
			task, err := findTask(d, opt.Task)
			if err != nil {
				return err
			}
			opt.Task = task.ID
			if profile == "" {
				profile = task.Profile
			}
		}
		if profile == "" {
			profile = a.Profile
			if a.Role == "orchestrator" {
				profile = workflowProfile(cfg, d, a.Role, profile)
			}
		}
		thread := ""
		var logical *Session
		if opt.ResumeSession != "" {
			prior := resumePrior
			if prior.AgentID != a.ID || prior.Active() {
				return fail("invalid_resume", "resume requires a terminated session of the same agent")
			}
			if prior.ClosedAt != nil {
				return fail("session_closed", "logical session is closed")
			}
			if prior.WorktreeID != wtID || prior.TaskID != opt.Task || prior.ParentAgentID != parent || prior.ReadOnly != opt.ReadOnly {
				return fail("invalid_resume", "resume context differs from the logical session")
			}
			if prior.TaskID != "" {
				task, taskErr := findTask(d, prior.TaskID)
				if taskErr != nil {
					return taskErr
				}
				currentInput, inputErr := taskInputDigest(d, task)
				if inputErr != nil {
					return inputErr
				}
				if task.Attempt != prior.TaskAttempt || task.InputDigest != prior.InputDigest || currentInput != prior.InputDigest {
					return fail("invalid_resume", "task attempt or input lineage changed; start a new logical session")
				}
			}
			logical = prior
			if prior.ClientSnapshot.Adapter == "opencode" && prior.ClientThreadID == "" {
				thread, err = s.recoverOpenCodeBinding(ctx, d, prior)
				if err != nil {
					return err
				}
			}
			// A logical Session owns its client snapshot. Keep the client identity
			// stable across Runs even if the project config has since changed; a
			// missing executable still fails at the normal LookPath gate.
			if prior.Route.Client != "" && prior.ClientSnapshot.Adapter != "" {
				if cfg.Clients == nil {
					cfg.Clients = map[string]Client{}
				}
				if cfg.Profiles == nil {
					cfg.Profiles = map[string]Profile{}
				}
				priorClient := prior.ClientSnapshot
				// Older registries predate native OpenCode delivery. Upgrade the
				// default snapshot on resume; an explicit generic delivery wrapper
				// remains backward-compatible.
				if priorClient.Adapter == "opencode" && !priorClient.NativeDelivery && len(priorClient.DeliverArgv) == 0 {
					priorClient.NativeDelivery = true
				}
				cfg.Clients[prior.Route.Client] = priorClient
				profileCfg := cfg.Profiles[profile]
				if prior.ClientThreadID != "" {
					profileCfg.Routes = []Route{prior.Route}
				} else {
					filtered := make([]Route, 0, len(profileCfg.Routes))
					for _, candidate := range profileCfg.Routes {
						if candidate.Client == prior.Route.Client {
							filtered = append(filtered, candidate)
						}
					}
					if len(filtered) == 0 {
						filtered = []Route{prior.Route}
					}
					profileCfg.Routes = filtered
				}
				cfg.Profiles[profile] = profileCfg
			}
			if prior.ClientThreadID != "" {
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
		if thread != "" {
			for i := range d.Registry.Sessions {
				other := &d.Registry.Sessions[i]
				if (logical == nil || other.ID != logical.ID) && other.Active() && other.ClientSnapshot.Adapter == client.Adapter && other.ClientThreadID == thread {
					return fail("thread_conflict", "client thread is already bound to active session %s", other.ID)
				}
			}
		}
		if logical == nil {
			id = ID("sess")
		} else {
			id = logical.ID
		}
		runID := ID("run")
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
		if err = t.Execute(&prompt, promptData{d.State.ID, d.Dir, a.ID, id, runID, parent, base}); err != nil {
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
		promptFile := filepath.Join(d.Dir, "prompts", runID+".md")
		if err := atomicWrite(promptFile, prompt.Bytes()); err != nil {
			return err
		}
		launchArgv := client.LaunchArgv
		if thread != "" && client.Adapter != "codex" {
			if len(client.ResumeArgv) == 0 {
				if client.Adapter == "opencode" {
					return fail("opencode_resume_unavailable", "OpenCode session %s has a native thread but no resume_argv; configure the client before retrying", logical.ID)
				}
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
		openCodeEndpoint := ""
		if client.Adapter == "opencode" && client.NativeDelivery {
			openCodeEndpoint, err = allocateOpenCodeEndpoint()
			if err != nil {
				return err
			}
			argv = withOpenCodeServerFlags(argv, openCodeEndpoint)
		}
		argv[0], err = exec.LookPath(argv[0])
		if err != nil {
			return err
		}
		created := time.Now().UTC()
		if logical == nil {
			out = Session{ID: id, AgentID: a.ID, AgentSnapshot: a, ParentAgentID: parent, WorktreeID: wtID, ClientSnapshot: client, ClientThreadID: thread, ReadOnly: opt.ReadOnly, CreatedAt: created, LifecycleState: "idle"}
			logical = &out
		} else {
			out = *logical
			// A client without resume support receives a fresh bootstrap; do not
			// leave the previous native thread attached to the logical context.
			out.ClientThreadID = thread
		}
		// bindTask validates the checkout before the Run is appended; expose the
		// pending run snapshot through the compatibility projection for that gate.
		out.Profile, out.Route, out.RoutingDecision = profile, route, &decision
		out.Argv, out.CWD, out.PromptFile, out.State, out.OpenCodeEndpoint = argv, cwd, promptFile, "starting", openCodeEndpoint
		bindingMode := taskClaim
		if resumePrior != nil {
			bindingMode = taskResume
		}
		binding, err := bindTask(ctx, d, &out, opt.Task, bindingMode)
		if err != nil {
			return err
		}
		run := Run{ID: runID, SessionID: out.ID, Generation: out.RunCount + 1, Profile: profile, Route: route, RoutingDecision: &decision, Argv: argv, CWD: cwd, PromptFile: promptFile, State: "starting", ClientThreadID: thread, OpenCodeEndpoint: openCodeEndpoint, CreatedAt: created}
		out.CurrentRunID, out.LastRunID = run.ID, run.ID
		if logical == &out {
			d.Registry.Sessions = append(d.Registry.Sessions, out)
		} else {
			*logical = out
		}
		d.Registry.Runs = append(d.Registry.Runs, run)
		if out.TaskID != "" && !binding.preserveRunProvenance {
			task, _ := findTask(d, out.TaskID)
			task.RunID = run.ID
		}
		d.remember(opt.OperationKey, requestOpt, out.ID)
		if err := saveDocument(d); err != nil {
			return err
		}
		launch := Launch{ProjectRoot: s.Root, ProjectID: d.State.ProjectID, WorkspaceID: d.State.ID, WorkspaceDir: d.Dir, SessionID: id, RunID: runID, WorktreeID: wtID, WorktreeName: wtName, CWD: cwd, Executable: s.Executable, Orchestrator: a.Role == "orchestrator"}
		if launch.Orchestrator {
			launch.PreferredOrchestratorWindowID, launch.ReplacePaneID, launch.ReplaceSessionID, launch.ReplaceRunID = priorOrchestratorPane(d, runID)
		}
		pane, launchErr := s.Runtime.Launch(ctx, launch)
		if launchErr != nil {
			run.State = "failed"
			var ce *Error
			if errors.As(launchErr, &ce) && ce.Code == "launch_uncertain" {
				run.State = "starting"
			}
			run.Error = launchErr.Error()
			if out.TaskID != "" && run.State == "failed" && !binding.preserveRunProvenance {
				task, _ := findTask(d, out.TaskID)
				task.State = "blocked"
				task.Reason = launchErr.Error()
			}
		} else {
			run.PaneID = pane.ID
			run.WindowID = pane.WindowID
		}
		d.Registry.Runs[len(d.Registry.Runs)-1] = run
		p, _ := findSession(d, out.ID)
		d.syncSession(p)
		out = *p
		if err := saveResource(d, opt.OperationKey, out); err != nil {
			return err
		}
		return launchErr
	})
	if err == nil && out.AgentSnapshot.Role == "orchestrator" {
		if _, managed := s.Runtime.(ManagedUIRuntime); managed {
			// The orchestrator is already durable and running. A UI startup error
			// is recorded by ReconcileInterface and must not undo that operation.
			_ = s.ReconcileInterface(ctx, selector)
		}
	}
	return out, err
}
func priorOrchestratorPane(d *Document, newRunID string) (windowID, paneID, sessionID, runID string) {
	var latest *Run
	for i := range d.Registry.Runs {
		run := &d.Registry.Runs[i]
		if run.ID == newRunID || run.Active() || run.WindowID == "" || run.PaneID == "" {
			continue
		}
		session, err := findSession(d, run.SessionID)
		if err != nil || session.AgentSnapshot.Role != "orchestrator" {
			continue
		}
		if latest == nil || run.CreatedAt.After(latest.CreatedAt) || run.CreatedAt.Equal(latest.CreatedAt) && run.ID > latest.ID {
			latest = run
			sessionID = session.ID
		}
	}
	if latest == nil {
		return "", "", "", ""
	}
	return latest.WindowID, latest.PaneID, sessionID, latest.ID
}

func (s *Service) StartOrchestrator(ctx context.Context, selector, key string) (Session, error) {
	return s.StartSession(ctx, selector, SessionOptions{Agent: "orchestrator", OperationKey: key})
}

// ExecuteSession is invoked by tmux with a concrete Run ID. The claim and the
// session's current_run pointer fence replayed and superseded runners.
func (s *Service) ExecuteSession(ctx context.Context, selector, id string, in io.Reader, out, errOut io.Writer) error {
	var session Session
	var run Run
	var state Workspace
	var dir string
	err := s.With(ctx, selector, func(d *Document) error {
		r, err := findRun(d, id)
		if err != nil {
			p, sessionErr := findSession(d, id)
			if sessionErr != nil || p.CurrentRunID == "" {
				return err
			}
			r, err = findRun(d, p.CurrentRunID)
			if err != nil {
				return err
			}
			id = r.ID
		}
		p, err := findSession(d, r.SessionID)
		if err != nil {
			return err
		}
		if p.CurrentRunID != r.ID {
			return fail("stale_run", "run %s no longer owns session %s", r.ID, p.ID)
		}
		if r.State != "starting" {
			return fail("run_claimed", "run %s is %s", id, r.State)
		}
		if r.PaneID == "" {
			pane, err := s.recoverPane(ctx, d, *p, *r)
			if err != nil {
				return err
			}
			r.PaneID, r.WindowID = pane.ID, pane.WindowID
		}
		r.State = "running"
		run = *r
		d.syncSession(p)
		session = *p
		state = d.State
		dir = d.Dir
		return saveDocument(d)
	})
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, run.Argv[0], run.Argv[1:]...)
	cmd.Dir = run.CWD
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "WORKSPACE_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "WORKSPACE_PROJECT_DIR="+s.Root, "WORKSPACE_PROJECT_ID="+state.ProjectID, "WORKSPACE_ID="+state.ID, "WORKSPACE_DIR="+dir, "WORKSPACE_AGENT_ID="+session.AgentID, "WORKSPACE_SESSION_ID="+session.ID, "WORKSPACE_RUN_ID="+run.ID, "WORKSPACE_ORCHESTRATOR_ID="+state.OrchestratorAgentID, "WORKSPACE_PARENT_AGENT_ID="+session.ParentAgentID, "WORKSPACE_WORKTREE_ID="+session.WorktreeID, "WORKSPACE_ROLE="+session.AgentSnapshot.Role)
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
	} else if session.ClientSnapshot.Adapter == "opencode" && (session.ClientSnapshot.NativeDelivery || run.ClientThreadID == "") {
		runErr = s.runOpenCode(ctx, selector, session, cmd, out, errOut)
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
		r, err := findRun(d, id)
		if err != nil {
			return err
		}
		p, err := findSession(d, r.SessionID)
		if err != nil {
			return err
		}
		if p.CurrentRunID != r.ID || r.State != "running" {
			return nil
		}
		now := time.Now().UTC()
		r.FinishedAt = &now
		r.ExitCode = &code
		r.State = "exited"
		if runErr != nil {
			r.State = "failed"
			r.Error = runErr.Error()
		}
		p.CurrentRunID = ""
		if p.TaskID != "" {
			task, err := findTask(d, p.TaskID)
			if err != nil {
				return err
			}
			if task.Attempt == p.TaskAttempt && task.SessionID == p.ID && task.RunID == r.ID && task.State == "running" {
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
	return s.stopSession(ctx, selector, id, "", keys...)
}

// StopRun stops only the concrete Run the user confirmed. The workspace lock
// rechecks ownership immediately before the stop, so a newly current Run is
// never affected by a stale confirmation form.
func (s *Service) StopRun(ctx context.Context, selector, sessionID, expectedRunID, key string) (Session, error) {
	if expectedRunID == "" {
		return Session{}, fail("target_changed", "a current Run ID is required")
	}
	return s.stopSession(ctx, selector, sessionID, expectedRunID, key)
}

func (s *Service) stopSession(ctx context.Context, selector, id, expectedRunID string, keys ...string) (Session, error) {
	if key := mutationKey(keys); key != "" {
		request := []any{"session.stop", id}
		if expectedRunID != "" {
			request = []any{"session.stop", id, "expected_run", expectedRunID}
		}
		return effect(s, ctx, selector, key, request, func(d *Document) error { return s.requireSessionOwner(d, id) }, func() (Session, error) { return s.stopSession(ctx, selector, id, expectedRunID) })
	}
	var out Session
	err := s.With(ctx, selector, func(d *Document) error {
		p, err := findSession(d, id)
		if err != nil {
			return err
		}
		if p.DeletedAt != nil {
			return fail("session_deleted", "session %s was deleted", p.ID)
		}
		if err := s.requireSessionOwner(d, p.ID); err != nil {
			return err
		}
		if expectedRunID != "" && p.CurrentRunID != expectedRunID {
			prior, priorErr := findRun(d, expectedRunID)
			if priorErr == nil && !prior.Active() {
				out = *p
				return nil
			}
			return fail("target_changed", "session %s no longer owns run %s", p.ID, expectedRunID)
		}
		out = *p
		if !p.Active() {
			return nil
		}
		r, err := currentRun(d, p)
		if err != nil {
			return err
		}
		if r.PaneID == "" {
			return fail("launch_uncertain", "no pane recorded; inspect tmux and run reconcile")
		}
		pane, err := s.Runtime.Inspect(ctx, r.PaneID)
		if err != nil {
			var ce *Error
			if !errors.As(err, &ce) || ce.Code != "pane_missing" {
				return err
			}
		} else if !paneOwns(pane, p.ID, r.ID) {
			return fail("pane_mismatch", "pane is not owned by the current run")
		} else if err := s.Runtime.Stop(ctx, r.PaneID); err != nil {
			return err
		}
		now := time.Now().UTC()
		r.State = "stopped"
		r.FinishedAt = &now
		p.CurrentRunID = ""
		blockInterruptedTask(d, p, r)
		d.syncSession(p)
		out = *p
		return saveDocument(d)
	})
	return out, err
}

func (s *Service) CloseSession(ctx context.Context, selector, id, reason string, keys ...string) (Session, error) {
	var out Session
	err := mutate(s, ctx, selector, keys, []any{"session.close", id, reason}, &out, func(d *Document) error {
		return s.requireSessionOwner(d, id)
	}, func(d *Document) error {
		p, err := findSession(d, id)
		if err != nil {
			return err
		}
		if p.DeletedAt != nil {
			return fail("session_deleted", "session %s was deleted", p.ID)
		}
		if p.Active() {
			return fail("session_active", "stop the current run before closing the logical session")
		}
		if p.ClosedAt == nil {
			now := nowUTC()
			p.ClosedAt, p.CloseReason = &now, strings.TrimSpace(reason)
		}
		d.syncSession(p)
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
		var latest *Session
		for i := range d.Registry.Sessions {
			session := &d.Registry.Sessions[i]
			if session.DeletedAt != nil {
				continue
			}
			if session.AgentID == a.ID && (latest == nil || session.LastActiveAt.After(latest.LastActiveAt) || session.LastActiveAt.Equal(latest.LastActiveAt) && session.ID > latest.ID) {
				latest = session
			}
		}
		if latest != nil {
			session := *latest
			opt.Worktree = session.WorktreeID
			opt.Parent = session.ParentAgentID
			opt.Profile = session.Profile
			opt.Task = session.TaskID
			opt.ResumeSession = session.ID
			opt.ReadOnly = session.ReadOnly
			if session.ClosedAt != nil {
				opt.ResumeSession = ""
			}
			if session.TaskID != "" {
				t, taskErr := findTask(d, session.TaskID)
				if taskErr != nil {
					opt.ResumeSession = ""
				} else {
					currentInput, inputErr := taskInputDigest(d, t)
					if inputErr != nil {
						return inputErr
					}
					if t.Attempt != session.TaskAttempt || t.InputDigest != session.InputDigest || currentInput != session.InputDigest {
						opt.ResumeSession = ""
					}
				}
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
				worktree, findErr := findWorktree(d, p.WorktreeID)
				if findErr != nil {
					return findErr
				}
				pane, err = rt.Recover(ctx, Launch{Service: true, ProjectRoot: s.Root, ProjectID: d.State.ProjectID, WorkspaceID: d.State.ID, WorkspaceDir: d.Dir, SessionID: p.ID, WorktreeID: worktree.ID, WorktreeName: worktree.Name, CWD: worktree.Path, Executable: s.Executable})
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
		for i := range d.Registry.Runs {
			run := &d.Registry.Runs[i]
			if !run.Active() {
				continue
			}
			p, findErr := findSession(d, run.SessionID)
			if findErr != nil {
				return findErr
			}
			if p.CurrentRunID != run.ID {
				run.State = "interrupted"
				run.Error = "run superseded by current session owner"
				changed = true
				continue
			}
			if run.PaneID == "" {
				if _, ok := s.Runtime.(interface {
					Recover(context.Context, Launch) (Pane, error)
				}); !ok {
					continue
				}
			}
			var pane Pane
			var err error
			if run.PaneID == "" {
				pane, err = s.recoverPane(ctx, d, *p, *run)
				if err == nil {
					run.PaneID, run.WindowID = pane.ID, pane.WindowID
					changed = true
				}
			} else {
				pane, err = s.Runtime.Inspect(ctx, run.PaneID)
			}
			if err != nil {
				var runtimeError *Error
				if !errors.As(err, &runtimeError) || runtimeError.Code != "pane_missing" {
					return err // An unavailable runtime is not proof that its client stopped.
				}
			}
			if err != nil || pane.Dead || !paneOwns(pane, p.ID, run.ID) {
				run.State = "interrupted"
				run.Error = "pane absent, dead or no longer owned; inspect local work before retry"
				now := time.Now().UTC()
				run.FinishedAt = &now
				p.CurrentRunID = ""
				blockInterruptedTask(d, p, run)
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
			run, err := findRun(d, r.RunID)
			if err == nil && !run.Active() {
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

func (s *Service) recoverPane(ctx context.Context, d *Document, p Session, run Run) (Pane, error) {
	r, ok := s.Runtime.(interface {
		Recover(context.Context, Launch) (Pane, error)
	})
	if !ok {
		return Pane{}, fail("launch_uncertain", "runtime cannot recover an unrecorded pane")
	}
	launch := Launch{ProjectRoot: s.Root, ProjectID: d.State.ProjectID, WorkspaceID: d.State.ID, WorkspaceDir: d.Dir, SessionID: p.ID, RunID: run.ID, WorktreeID: p.WorktreeID, CWD: run.CWD, Executable: s.Executable, Orchestrator: p.AgentSnapshot.Role == "orchestrator"}
	if launch.Orchestrator {
		launch.PreferredOrchestratorWindowID, launch.ReplacePaneID, launch.ReplaceSessionID, launch.ReplaceRunID = priorOrchestratorPane(d, run.ID)
	}
	return r.Recover(ctx, launch)
}
func blockInterruptedTask(d *Document, p *Session, run *Run) {
	if p.TaskID == "" {
		return
	}
	t, err := findTask(d, p.TaskID)
	if err == nil && t.State == "running" && t.SessionID == p.ID && t.RunID == run.ID && t.Attempt == p.TaskAttempt {
		t.State, t.Reason = "blocked", "session interrupted; inspect local work before retry"
	}
}
