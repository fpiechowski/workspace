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
	if runtime.GOOS == "windows" {
		return fail("runtime_unsupported", "run the Linux build in WSL for tmux sessions")
	}
	rt, ok := s.Runtime.(Tmux)
	if !ok {
		return nil
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
	workspaces, err := s.List(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, status := range workspaces {
		if err := s.tickWorkspace(ctx, status); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", status.Workspace.ID, err))
		}
	}
	return errors.Join(failures...)
}
func (s *Service) tickWorkspace(ctx context.Context, status Status) error {
	if status.Workspace.Status == "archived" {
		return nil
	}
	if _, err := s.Reconcile(ctx, status.Workspace.ID); err != nil {
		return err
	}
	var messages []Message
	latest := map[string]Session{}
	if err := s.With(ctx, status.Workspace.ID, func(d *Document) error {
		for _, p := range d.Registry.Sessions {
			if prior, ok := latest[p.AgentID]; !ok || p.LastActiveAt.After(prior.LastActiveAt) || p.LastActiveAt.Equal(prior.LastActiveAt) && p.ID > prior.ID {
				latest[p.AgentID] = p
			}
		}
		messages = append(messages, d.Registry.Messages...)
		return nil
	}); err != nil {
		return err
	}
	if err := s.autoBindOpenCodeThreads(ctx, status.Workspace.ID, latest); err != nil {
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
		latest[orch.AgentID] = resumed
	}
	for _, message := range messages {
		if message.AcknowledgedAt != nil {
			continue
		}
		p, ok := latest[message.ToAgent]
		if !ok {
			continue
		}
		if p.Active() && p.ClientSnapshot.Adapter == "codex" {
			continue
		} // The bridge owns this native connection.
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
		if message.NotifiedRunID == p.CurrentRunID || p.PaneID == "" {
			continue
		}
		if rt, ok := s.Runtime.(Tmux); ok {
			pane, err := rt.Inspect(ctx, p.PaneID)
			if err != nil {
				continue
			}
			if !paneOwns(pane, p.ID, p.CurrentRunID) {
				continue
			}
			if _, err := rt.call(ctx, "display-message", "-t", p.PaneID, "workspace: pending "+message.Kind+" "+message.ID+" (read inbox)"); err != nil {
				return err
			}
			if err := s.With(ctx, status.Workspace.ID, func(d *Document) error {
				m, err := findMessage(d, message.ID)
				if err != nil {
					return err
				}
				m.NotifiedSessionID = p.ID
				m.NotifiedRunID = p.CurrentRunID
				return saveDocument(d)
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) autoBindOpenCodeThreads(ctx context.Context, selector string, latest map[string]Session) error {
	byExecutable := make(map[string][]openCodeSession)
	failedExecutables := make(map[string]struct{})
	claimed := make(map[string]struct{})
	for _, session := range latest {
		if session.Active() && session.ClientSnapshot.Adapter == "opencode" && session.ClientThreadID != "" {
			claimed[session.ClientThreadID] = struct{}{}
		}
	}
	for agentID, session := range latest {
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
		latest[agentID] = session
	}
	return nil
}
