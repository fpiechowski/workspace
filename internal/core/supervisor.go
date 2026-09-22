package core

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

type SupervisorInfo struct {
	PID        int       `json:"pid" yaml:"pid"`
	Endpoint   string    `json:"endpoint" yaml:"endpoint"`
	Token      string    `json:"token" yaml:"-"`
	TmuxSocket string    `json:"tmux_socket" yaml:"tmux_socket"`
	StartedAt  time.Time `json:"started_at" yaml:"started_at"`
}

func currentRunFromStatus(status Status, id string) (Run, error) {
	for _, run := range status.Runs {
		if run.ID == id && run.Active() {
			return run, nil
		}
	}
	return Run{}, fail("stale_run", "current Run is unavailable")
}

func (s *Service) supervisorFile() string {
	return filepath.Join(s.Root, ".workspace", ".runtime", "supervisor.json")
}
func (s *Service) supervisorCall(ctx context.Context, method string) (SupervisorInfo, error) {
	var info SupervisorInfo
	if err := readJSON(s.supervisorFile(), &info); err != nil {
		return info, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", info.Endpoint)
	if err != nil {
		return info, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := json.NewEncoder(conn).Encode(map[string]string{"method": method, "token": info.Token}); err != nil {
		return info, err
	}
	var response struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return info, err
	}
	if !response.OK {
		return info, fail("supervisor_error", "supervisor rejected request")
	}
	info.Token = ""
	return info, nil
}
func (s *Service) SupervisorStatus(ctx context.Context) (SupervisorInfo, error) {
	return s.supervisorCall(ctx, "ping")
}
func (s *Service) StopSupervisor(ctx context.Context, keys ...string) error {
	if s.Actor.AgentID != "" || s.Actor.SessionID != "" || s.Actor.RunID != "" {
		return fail("forbidden", "stop supervision from a user terminal")
	}
	if key := mutationKey(keys); key != "" {
		_, err := projectEffect(ctx, s.Root, key, "server.stop", func() (bool, error) { return true, s.StopSupervisor(ctx) })
		return err
	}
	_, err := s.supervisorCall(ctx, "stop")
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (s *Service) EnsureSupervisor(ctx context.Context) error {
	rt, ok := s.Runtime.(Tmux)
	if !ok {
		return nil
	}
	if runtime.GOOS == "windows" {
		return fail("runtime_unsupported", "run the Linux build in WSL for tmux sessions")
	}
	if info, err := s.SupervisorStatus(ctx); err == nil {
		if info.TmuxSocket != rt.Socket {
			return fail("supervisor_conflict", "project supervisor is using a different tmux server")
		}
		return nil
	}
	path := filepath.Join(s.Root, ".workspace", ".runtime", "supervisor-start.lock")
	lock := flock.New(path)
	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	acquired, err := lock.TryLockContext(lockCtx, 50*time.Millisecond)
	if err != nil || !acquired {
		return fail("supervisor_busy", "cannot lock supervisor startup")
	}
	defer lock.Unlock()
	if info, err := s.SupervisorStatus(ctx); err == nil {
		if info.TmuxSocket != rt.Socket {
			return fail("supervisor_conflict", "project supervisor is using a different tmux server")
		}
		return nil
	}
	log, err := os.OpenFile(filepath.Join(s.Root, ".workspace", ".runtime", "supervisor.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(s.Executable, "--project", s.Root, "--tmux-socket", rt.Socket, "serve")
	cmd.Dir = s.Root
	cmd.Stdout = log
	cmd.Stderr = log
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "WORKSPACE_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	deadline := time.NewTimer(7 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := s.SupervisorStatus(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fail("supervisor_start_failed", "inspect .workspace/.runtime/supervisor.log")
		case <-ticker.C:
		}
	}
}
func (s *Service) Serve(ctx context.Context) error {
	if runtime.GOOS == "windows" {
		return fail("runtime_unsupported", "supervisor runtime requires Linux/macOS/WSL")
	}
	if _, err := s.Config(); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(s.Root, ".workspace", ".runtime", "supervisor.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !ok {
		return fail("supervisor_running", "a supervisor already owns this project")
	}
	defer lock.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	endpoint := filepath.Join(os.TempDir(), "workspace-"+payloadDigest(s.Root)[7:31]+".sock")
	if err := os.Remove(endpoint); err != nil && !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(endpoint)
	if err := os.Chmod(endpoint, 0600); err != nil {
		return err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	info := SupervisorInfo{PID: os.Getpid(), Endpoint: endpoint, Token: hex.EncodeToString(secret), StartedAt: nowUTC()}
	if rt, ok := s.Runtime.(Tmux); ok {
		info.TmuxSocket = rt.Socket
	}
	if err := writeJSON(s.supervisorFile(), info); err != nil {
		return err
	}
	defer os.Remove(s.supervisorFile())
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				var request struct{ Method, Token string }
				if json.NewDecoder(conn).Decode(&request) != nil {
					return
				}
				valid := subtle.ConstantTimeCompare([]byte(request.Token), []byte(info.Token)) == 1
				valid = valid && (request.Method == "ping" || request.Method == "stop")
				_ = json.NewEncoder(conn).Encode(map[string]bool{"ok": valid})
				if valid && request.Method == "stop" {
					cancel()
				}
			}()
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastError := ""
	for {
		if err := s.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
			if err.Error() != lastError {
				fmt.Fprintln(os.Stderr, err)
				lastError = err.Error()
			}
		} else {
			lastError = ""
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (s *Service) Tick(ctx context.Context) error {
	var failures []error
	overview, overviewErr := s.ProjectOverview(ctx)
	if overviewErr != nil {
		failures = append(failures, fmt.Errorf("project overview: %w", overviewErr))
	} else {
		for _, row := range overview.Workspaces {
			if row.Error != "" {
				failures = append(failures, fmt.Errorf("%s: %s", row.ID, row.Error))
				continue
			}
			status, err := s.Status(ctx, row.ID)
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", row.ID, err))
				continue
			}
			if err := s.tickWorkspace(ctx, status); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", status.Workspace.ID, err))
			}
		}
	}
	if err := s.tickDispatcher(ctx); err != nil {
		failures = append(failures, fmt.Errorf("dispatcher: %w", err))
	}
	return errors.Join(failures...)
}

// tickDispatcher reconciles the project-scoped Dispatcher independently from
// Workspace agents. Only a verified lost owned pane is eligible for resume;
// normal client exit, explicit stop and observation failures never trigger a
// blind duplicate launch.
func (s *Service) tickDispatcher(ctx context.Context) error {
	var projectID, runID, paneID, sessionID, agentID string
	var active bool
	err := withProjectLock(ctx, s.Root, func() error {
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		state, exists, err := loadDispatcherState(s.Root, cfg.ProjectID)
		if err != nil || !exists || state.StopRequested {
			return err
		}
		projectID = cfg.ProjectID
		for _, session := range state.Sessions {
			if session.AgentID != state.Agent.ID || !session.Active() || session.CurrentRunID == "" {
				continue
			}
			run, findErr := findDispatcherRun(&state, session.CurrentRunID)
			if findErr != nil {
				return findErr
			}
			active, runID, paneID, sessionID, agentID = true, run.ID, run.PaneID, session.ID, state.Agent.ID
			break
		}
		return nil
	})
	if err != nil || !active {
		return err
	}
	if paneID == "" {
		return nil
	}
	var inspectErr error
	if inspector, ok := s.Runtime.(interface {
		InspectProject(context.Context, string, string) (Pane, error)
	}); ok {
		pane, err := inspector.InspectProject(ctx, paneID, projectID)
		inspectErr = err
		if inspectErr == nil {
			owned := pane.Scope == "project" && pane.ProjectID == projectID && pane.AgentID == agentID && pane.SessionID == sessionID && pane.RunID == runID && pane.Role == "dispatcher"
			if !owned {
				return fail("dispatcher_pane_conflict", "live pane does not carry the current Dispatcher ownership metadata")
			}
			if !pane.Dead {
				return nil
			}
			inspectErr = fail("pane_missing", "Dispatcher pane is dead or ownership metadata does not match")
		}
	} else {
		return fail("runtime_unsupported", "runtime cannot verify project Dispatcher pane ownership")
	}
	if inspectErr == nil {
		return nil
	}
	var runtimeErr *Error
	if !errors.As(inspectErr, &runtimeErr) || runtimeErr.Code != "pane_missing" {
		return inspectErr
	}
	// Persist the interruption under the project lock and verify the same Run
	// still owns the Session before resuming it.
	err = withProjectLock(ctx, s.Root, func() error {
		state, exists, err := loadDispatcherState(s.Root, projectID)
		if err != nil || !exists || state.StopRequested {
			return err
		}
		session := latestDispatcherSession(state)
		if session == nil || session.CurrentRunID != runID {
			return nil
		}
		run, err := findDispatcherRun(&state, runID)
		if err != nil || !run.Active() {
			return err
		}
		run.State, run.Error = "interrupted", "Dispatcher pane is absent or no longer owned"
		now := nowUTC()
		run.FinishedAt = &now
		session.CurrentRunID = ""
		syncDispatcherSession(&state, session)
		return saveDispatcherState(s.Root, state)
	})
	if err != nil {
		return err
	}
	_, err = s.StartDispatcher(ctx, "", "dispatcher-recover:"+runID)
	return err
}
func (s *Service) tickWorkspace(ctx context.Context, status Status) error {
	var failures []error
	if status.Workspace.Status != "archived" {
		if err := s.tickWorkspaceAgents(ctx, status); err != nil {
			failures = append(failures, fmt.Errorf("agent runtime: %w", err))
		}
	}
	return errors.Join(failures...)
}

func (s *Service) tickWorkspaceAgents(ctx context.Context, status Status) error {
	if status.Workspace.Status == "archived" {
		return nil
	}
	if _, err := s.Reconcile(ctx, status.Workspace.ID); err != nil {
		return err
	}
	var messages []Message
	sessions := map[string]Session{}
	latest := map[string]Session{}
	if err := s.With(ctx, status.Workspace.ID, func(d *Document) error {
		for _, p := range d.Registry.Sessions {
			sessions[p.ID] = p
			if prior, ok := latest[p.AgentID]; !ok || p.LastActiveAt.After(prior.LastActiveAt) || p.LastActiveAt.Equal(prior.LastActiveAt) && p.ID > prior.ID {
				latest[p.AgentID] = p
			}
		}
		messages = append(messages, d.Registry.Messages...)
		return nil
	}); err != nil {
		return err
	}
	if err := s.autoBindOpenCodeThreads(ctx, status.Workspace.ID, sessions); err != nil {
		return err
	}
	// A verified lost pane can be replaced; silence or a transient tmux
	// error never triggers another execution. Stopped/failed clients require
	// an explicit retry, preventing unbounded restart loops.
	orch, exists := latest[status.Workspace.OrchestratorAgentID]
	if exists && orch.State == "interrupted" && (status.Workspace.Status == "active" || status.Workspace.Status == "needs_workflow") {
		resumed, err := s.ResumeAgent(ctx, status.Workspace.ID, orch.AgentID, "recover:"+orch.LastRunID)
		if err != nil {
			return err
		}
		sessions[resumed.ID] = resumed
		latest[orch.AgentID] = resumed
	}
	for _, message := range messages {
		if message.AcknowledgedAt != nil {
			continue
		}
		var p Session
		var ok bool
		if message.ToSession != "" {
			// Session-addressed messages never fall back to another Session of
			// the same Agent. An idle target simply remains pending.
			p, ok = sessions[message.ToSession]
		} else {
			// Explicit compatibility path for pre-migration agent-addressed
			// messages. New messages never enter this branch.
			p, ok = latest[message.ToAgent]
			if ok {
				if updated, exists := sessions[p.ID]; exists {
					p = updated
				}
			}
		}
		if !ok {
			continue
		}
		if p.DeletedAt != nil || p.ClosedAt != nil {
			continue
		}
		if p.Active() && p.ClientSnapshot.Adapter == "codex" {
			continue
		} // The bridge owns this native connection.
		if p.Active() && usesNativeOpenCodeDelivery(p.ClientSnapshot) {
			if message.DeliveredRunID == p.CurrentRunID {
				continue
			}
			run, err := currentRunFromStatus(status, p.CurrentRunID)
			if err != nil {
				continue
			}
			if err := s.deliverOpenCodeMessage(ctx, status.Workspace.ID, p, run, message); err != nil {
				// Delivery is at-least-once and must not stop supervision of other
				// sessions. The durable phase remains retryable on the next tick;
				// the notification is only a fallback and never marks transport
				// delivery successful.
				notifyCtx, cancel := context.WithTimeout(ctx, openCodePersistenceTimeout)
				_ = s.notifyPendingMessage(notifyCtx, status.Workspace.ID, p, message)
				cancel()
				continue
			}
			continue
		}
		if p.Active() && len(p.ClientSnapshot.DeliverArgv) > 0 && p.ClientThreadID != "" {
			if message.DeliveredRunID == p.CurrentRunID {
				continue
			}
			path := filepath.Join(status.Directory, ".runtime", "delivery", message.ID+".json")
			if err := writeJSON(path, message); err != nil {
				return err
			}
			argv := make([]string, len(p.ClientSnapshot.DeliverArgv))
			replace := strings.NewReplacer(
				"{project_dir}", s.Root,
				"{thread_id}", p.ClientThreadID,
				"{message_file}", path,
				"{message_id}", message.ID,
			)
			for i, arg := range p.ClientSnapshot.DeliverArgv {
				argv[i] = replace.Replace(arg)
			}
			callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			b, err := forgeCommand(callCtx, p.CWD, argv...)
			cancel()
			if err != nil {
				return err
			}
			var receipt struct {
				Accepted bool `json:"accepted"`
			}
			if err := json.Unmarshal(b, &receipt); err != nil {
				return err
			}
			if !receipt.Accepted {
				return fail("delivery_rejected", "client did not accept message %s", message.ID)
			}
			if err := s.markDelivered(ctx, status.Workspace.ID, p.CurrentRunID, []string{message.ID}); err != nil {
				return err
			}
			continue
		}
		if err := s.notifyPendingMessage(ctx, status.Workspace.ID, p, message); err != nil {
			return err
		}
	}
	return nil
}

// notifyPendingMessage is the tmux-only fallback for transports that cannot
// wake a client or whose native delivery is currently uncertain. It verifies
// the exact current Run before and after display-message so a successor Run
// cannot inherit a notification receipt from an older pane. The message stays
// undelivered and unacknowledged.
func (s *Service) notifyPendingMessage(ctx context.Context, selector string, session Session, message Message) error {
	if !session.Active() || session.CurrentRunID == "" || session.PaneID == "" {
		return nil
	}
	rt, ok := s.Runtime.(interface {
		Inspect(context.Context, string) (Pane, error)
		DisplayMessage(context.Context, string, string) error
	})
	if !ok {
		return nil
	}
	eligible := false
	if err := s.With(ctx, selector, func(d *Document) error {
		p, err := findSession(d, session.ID)
		if err != nil {
			return err
		}
		r, err := currentRun(d, p)
		if err != nil {
			return err
		}
		m, err := findMessage(d, message.ID)
		if err != nil {
			return err
		}
		if !messageAddressMatchesRun(*m, *p, *r) || p.CurrentRunID != session.CurrentRunID || r.ID != session.CurrentRunID || !r.Active() || m.DeliveredRunID == r.ID || m.NotifiedRunID == r.ID {
			return nil
		}
		eligible = true
		return nil
	}); err != nil {
		return err
	}
	if !eligible {
		return nil
	}
	pane, err := rt.Inspect(ctx, session.PaneID)
	if err != nil {
		// A disappearing pane is already covered by reconciliation; do not turn
		// a best-effort notification into a supervisor failure.
		return nil
	}
	if !paneOwns(pane, session.ID, session.CurrentRunID) {
		return nil
	}
	if err := rt.DisplayMessage(ctx, session.PaneID, "workspace: pending "+message.Kind+" "+message.ID+" (read inbox)"); err != nil {
		return err
	}
	return s.With(ctx, selector, func(d *Document) error {
		p, err := findSession(d, session.ID)
		if err != nil {
			return err
		}
		r, err := currentRun(d, p)
		if err != nil {
			return err
		}
		m, err := findMessage(d, message.ID)
		if err != nil {
			return err
		}
		if !messageAddressMatchesRun(*m, *p, *r) || p.CurrentRunID != session.CurrentRunID || r.ID != session.CurrentRunID || !r.Active() || m.DeliveredRunID == r.ID || m.NotifiedRunID == r.ID {
			return nil
		}
		m.NotifiedSessionID = p.ID
		m.NotifiedRunID = r.ID
		return saveDocument(d)
	})
}

func (s *Service) autoBindOpenCodeThreads(ctx context.Context, selector string, sessions map[string]Session) error {
	byExecutable := make(map[string][]openCodeSession)
	failedExecutables := make(map[string]struct{})
	claimed := make(map[string]struct{})
	for _, session := range sessions {
		if session.Active() && session.ClientSnapshot.Adapter == "opencode" && session.ClientThreadID != "" {
			claimed[session.ClientThreadID] = struct{}{}
		}
	}
	for _, session := range sessions {
		if session.State != "running" || session.ClientSnapshot.Adapter != "opencode" || session.ClientThreadID != "" {
			continue
		}
		if len(session.Argv) == 0 || session.Argv[0] == "" {
			continue
		}
		executable := session.Argv[0]
		if _, failed := failedExecutables[executable]; failed {
			continue
		}
		items, cached := byExecutable[executable]
		if !cached {
			var err error
			items, err = s.listOpenCodeSessions(ctx, session, os.Environ())
			if err != nil {
				failedExecutables[executable] = struct{}{}
				continue
			}
			byExecutable[executable] = items
		}
		thread := chooseOpenCodeThread(items, session.CWD, claimed, session.LastActiveAt, false)
		if thread == "" {
			continue
		}
		if err := s.clientState(ctx, selector, session.CurrentRunID, thread, "idle"); err != nil {
			return err
		}
		claimed[thread] = struct{}{}
		session.ClientThreadID = thread
		sessions[session.ID] = session
	}
	return nil
}
