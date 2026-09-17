package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const openCodeDeliveryTimeout = 8 * time.Second

func (s *Service) openCodeHTTP(ctx context.Context, endpoint, method, path string, body []byte) ([]byte, int, error) {
	if endpoint == "" {
		return nil, 0, fail("opencode_unavailable", "current Run has no OpenCode endpoint")
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(endpoint, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	if password := os.Getenv("OPENCODE_SERVER_PASSWORD"); password != "" {
		request.SetBasicAuth("opencode", password)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if readErr != nil {
		return nil, response.StatusCode, readErr
	}
	return data, response.StatusCode, nil
}

func (s *Service) openCodeReady(ctx context.Context, endpoint string) error {
	deadline := time.NewTimer(openCodeDeliveryTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, status, err := s.openCodeHTTP(ctx, endpoint, http.MethodGet, "/global/health", nil)
		if err == nil && status >= 200 && status < 300 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fail("opencode_unavailable", "OpenCode endpoint is not ready")
		case <-ticker.C:
		}
	}
}

func openCodeHistoryContains(data []byte, marker string) bool {
	return bytes.Contains(data, []byte(marker))
}

func (s *Service) openCodeHistory(ctx context.Context, endpoint, thread string) ([]byte, error) {
	data, status, err := s.openCodeHTTP(ctx, endpoint, http.MethodGet, "/session/"+thread+"/message", nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fail("opencode_rejected", "OpenCode history returned HTTP %d", status)
	}
	return data, nil
}

func (s *Service) deliverOpenCodeMessage(ctx context.Context, selector string, session Session, run Run, message Message) error {
	_ = session
	if run.OpenCodeEndpoint == "" || run.ClientThreadID == "" {
		deliveryErr := "active OpenCode Run has no Run-scoped endpoint or session; stop and resume the Session"
		if err := s.setDeliveryPhase(ctx, selector, message.ID, run.ID, "restart_required", deliveryErr); err != nil {
			return err
		}
		return fail("restart_required", "%s", deliveryErr)
	}
	marker := "[workspace-message-id:" + message.ID + "]"
	if err := s.setDeliveryPhase(ctx, selector, message.ID, run.ID, "checking", ""); err != nil {
		return err
	}
	if err := s.openCodeReady(ctx, run.OpenCodeEndpoint); err != nil {
		_ = s.setDeliveryPhase(context.Background(), selector, message.ID, run.ID, "retry", err.Error())
		return err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	history, historyErr := s.openCodeHistory(checkCtx, run.OpenCodeEndpoint, run.ClientThreadID)
	cancel()
	if historyErr == nil && openCodeHistoryContains(history, marker) {
		_ = s.setDeliveryPhase(ctx, selector, message.ID, run.ID, "confirmed", "")
		return s.markDelivered(ctx, selector, run.ID, []string{message.ID})
	}
	prompt := fmt.Sprintf("%s\nType: %s\nFrom: %s\nHandoff: %s\n\n%s\n\nRead the durable inbox record before acting: message_id=%s.", marker, message.Kind, message.FromAgent, message.HandoffID, message.Body, message.ID)
	body, _ := json.Marshal(map[string]any{"parts": []map[string]string{{"type": "text", "text": prompt}}})
	data, status, err := s.openCodeHTTP(ctx, run.OpenCodeEndpoint, http.MethodPost, "/session/"+run.ClientThreadID+"/prompt_async", body)
	if err != nil || status < 200 || status >= 300 {
		if err == nil {
			err = fail("opencode_rejected", "OpenCode prompt returned HTTP %d: %s", status, strings.TrimSpace(string(data)))
		}
		_ = s.setDeliveryPhase(context.Background(), selector, message.ID, run.ID, "retry", err.Error())
		return err
	}
	if err := s.setDeliveryPhase(ctx, selector, message.ID, run.ID, "submitted", ""); err != nil {
		return err
	}
	confirmCtx, cancel := context.WithTimeout(ctx, openCodeDeliveryTimeout)
	defer cancel()
	for {
		history, err := s.openCodeHistory(confirmCtx, run.OpenCodeEndpoint, run.ClientThreadID)
		if err == nil && openCodeHistoryContains(history, marker) {
			if err := s.setDeliveryPhase(confirmCtx, selector, message.ID, run.ID, "confirmed", ""); err != nil {
				return err
			}
			return s.markDelivered(confirmCtx, selector, run.ID, []string{message.ID})
		}
		select {
		case <-confirmCtx.Done():
			_ = s.setDeliveryPhase(context.Background(), selector, message.ID, run.ID, "uncertain", "prompt accepted but marker was not observed")
			return fail("delivery_unconfirmed", "OpenCode prompt was not observed in session history")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func allocateOpenCodeEndpoint() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fail("opencode_endpoint", "cannot allocate loopback endpoint: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return "http://" + address, nil
}

func withOpenCodeServerFlags(argv []string, endpoint string) []string {
	result := append([]string(nil), argv...)
	host, port, _ := strings.Cut(strings.TrimPrefix(endpoint, "http://"), ":")
	if host == "" {
		host = "127.0.0.1"
	}
	if i := argIndex(result, "--hostname"); i >= 0 && i+1 < len(result) {
		result[i+1] = host
	} else {
		result = append(result, "--hostname", host)
	}
	if i := argIndex(result, "--port"); i >= 0 && i+1 < len(result) {
		result[i+1] = port
	} else {
		result = append(result, "--port", port)
	}
	return result
}

func argIndex(argv []string, value string) int {
	for i, arg := range argv {
		if arg == value {
			return i
		}
	}
	return -1
}

const (
	openCodeDiscoveryTimeout  = 30 * time.Second
	openCodeDiscoveryInterval = 200 * time.Millisecond
	// A native session created by a Run should appear shortly after the Run
	// starts. The repair path is deliberately narrower than an arbitrary
	// historical search so an old conversation cannot be attached silently.
	openCodeRecoveryClockSkew = time.Second
	openCodeRecoveryWindow    = 30 * time.Second
)

type openCodeSession struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
	Created   int64  `json:"created"`
	Updated   int64  `json:"updated"`
}

// UnmarshalJSON normalizes the two OpenCode session-list shapes we need to
// consume. The run-scoped HTTP endpoint nests timestamps under time, while
// `opencode session list --format json` emits them at the top level.
func (s *openCodeSession) UnmarshalJSON(data []byte) error {
	var payload struct {
		ID        string `json:"id"`
		Directory string `json:"directory"`
		Created   int64  `json:"created"`
		Updated   int64  `json:"updated"`
		Time      *struct {
			Created int64 `json:"created"`
			Updated int64 `json:"updated"`
		} `json:"time"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	s.ID, s.Directory = payload.ID, payload.Directory
	s.Created, s.Updated = payload.Created, payload.Updated
	if payload.Time != nil {
		if s.Created == 0 {
			s.Created = payload.Time.Created
		}
		if s.Updated == 0 {
			s.Updated = payload.Time.Updated
		}
	}
	return nil
}

type openCodeSessionLister func(context.Context, Session) ([]openCodeSession, error)

// runOpenCode keeps the client's interactive process in the tmux pane while a
// short-lived discovery loop resolves the native OpenCode session it created.
// The native ID is persisted before the supervisor needs to deliver a message.
func (s *Service) runOpenCode(ctx context.Context, selector string, session Session, cmd *exec.Cmd, out, errOut io.Writer) error {
	startedAt := time.Now()
	existing, err := s.listOpenCodeSessions(ctx, session, cmd.Env)
	baselineOK := err == nil
	known := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		if sameOpenCodePath(item.Directory, session.CWD) && item.ID != "" {
			known[item.ID] = struct{}{}
		}
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	discoveryCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	processDone := make(chan struct{})
	discoveryDone := make(chan struct{})
	go func() {
		defer close(discoveryDone)
		thread, discoveryErr := s.waitForOpenCodeThread(discoveryCtx, session, cmd.Env, known, startedAt, baselineOK, processDone)
		if thread == "" {
			if discoveryErr != nil && !isContextError(discoveryErr) {
				fmt.Fprintf(errOut, "workspace: OpenCode session discovery failed: %v\n", discoveryErr)
			}
			return
		}
		if err := s.clientState(discoveryCtx, selector, session.CurrentRunID, thread, "idle"); err != nil {
			if !isContextError(err) {
				fmt.Fprintf(errOut, "workspace: cannot bind OpenCode session %s: %v\n", thread, err)
			}
			return
		}
		fmt.Fprintf(out, "\n[workspace] OpenCode session bound: %s\n", thread)
	}()

	waitErr := cmd.Wait()
	close(processDone)
	<-discoveryDone
	return waitErr
}

func (s *Service) waitForOpenCodeThread(ctx context.Context, session Session, env []string, known map[string]struct{}, startedAt time.Time, baselineOK bool, processDone <-chan struct{}) (string, error) {
	deadline := time.NewTimer(openCodeDiscoveryTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(openCodeDiscoveryInterval)
	defer ticker.Stop()
	var lastErr error
	for {
		items, err := s.listOpenCodeSessions(ctx, session, env)
		if err != nil {
			lastErr = err
		} else if thread := chooseOpenCodeThread(items, session.CWD, known, startedAt, baselineOK); thread != "" {
			return thread, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-processDone:
			return "", lastErr
		case <-deadline.C:
			if lastErr == nil {
				lastErr = fmt.Errorf("no new OpenCode session found for %s", session.CWD)
			}
			return "", lastErr
		case <-ticker.C:
		}
	}
}

func (s *Service) listOpenCodeSessions(ctx context.Context, session Session, env []string) ([]openCodeSession, error) {
	if s.openCodeSessionLister != nil {
		return s.openCodeSessionLister(ctx, session)
	}
	var endpointErr error
	if session.OpenCodeEndpoint != "" {
		data, status, err := s.openCodeHTTP(ctx, session.OpenCodeEndpoint, http.MethodGet, "/session", nil)
		if err == nil && status >= 200 && status < 300 {
			var sessions []openCodeSession
			if err := json.Unmarshal(data, &sessions); err == nil {
				return sessions, nil
			} else {
				endpointErr = fmt.Errorf("invalid OpenCode session list: %w", err)
			}
		} else if err != nil {
			endpointErr = fmt.Errorf("OpenCode session endpoint: %w", err)
		} else {
			endpointErr = fmt.Errorf("OpenCode session list returned HTTP %d", status)
		}
		// Do not turn cancellation into an executable invocation. During normal
		// discovery, however, an endpoint can be unready or already gone when
		// the recorded executable can still inspect the shared session store.
		if ctx.Err() != nil {
			return nil, endpointErr
		}
	}

	sessions, executableErr := s.listOpenCodeSessionsExecutable(ctx, session, env)
	if executableErr == nil {
		return sessions, nil
	}
	if endpointErr != nil {
		return nil, errors.Join(endpointErr, fmt.Errorf("OpenCode executable fallback: %w", executableErr))
	}
	return nil, executableErr
}

func (s *Service) listOpenCodeSessionsExecutable(ctx context.Context, session Session, env []string) ([]openCodeSession, error) {
	if len(session.Argv) == 0 || session.Argv[0] == "" {
		return nil, fmt.Errorf("OpenCode executable is not recorded")
	}
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(queryCtx, session.Argv[0], "session", "list", "--format", "json", "--max-count", "1000")
	// Run the metadata query from the project root. A worktree may contain an
	// OpenCode project config that keeps an interactive TUI busy; listing the
	// global session store does not require loading that worktree config.
	cmd.Dir = s.Root
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("%s", message)
	}
	var sessions []openCodeSession
	if err := json.Unmarshal(b, &sessions); err != nil {
		return nil, fmt.Errorf("invalid OpenCode session list: %w", err)
	}
	return sessions, nil
}

// recoverOpenCodeBinding repairs an idle logical Session created by versions
// that failed to persist the native ID. It runs while StartSession holds the
// workspace lock, so the binding and its historical Run are committed before
// a successor Run can be assembled.
func (s *Service) recoverOpenCodeBinding(ctx context.Context, d *Document, session *Session) (string, error) {
	if session.ClientSnapshot.Adapter != "opencode" || session.ClientThreadID != "" {
		return "", nil
	}

	history := make([]Run, 0)
	for _, run := range d.Registry.Runs {
		if run.SessionID == session.ID && run.ClientThreadID == "" && !run.Active() && !run.CreatedAt.IsZero() {
			history = append(history, run)
		}
	}
	if len(history) == 0 {
		// There is no previous execution to correlate. The next Run may start a
		// fresh OpenCode conversation and the normal discovery loop will bind it.
		return "", nil
	}

	items, err := s.listOpenCodeSessions(ctx, *session, os.Environ())
	if err != nil {
		return "", fail("opencode_thread_recovery", "cannot safely recover the OpenCode conversation for session %s: %v; use `workspace session bind-thread %s --thread-id <thread-id>` and retry", session.ID, err, session.ID)
	}
	thread, runID, err := chooseHistoricalOpenCodeThread(items, d, *session, history)
	if err != nil {
		return "", err
	}
	if thread == "" {
		// A successful, empty listing means no historical OpenCode process ever
		// created a native session (or it was outside the bounded correlation
		// window). Preserve the documented fresh-start behavior.
		return "", nil
	}

	r, err := findRun(d, runID)
	if err != nil {
		return "", err
	}
	if r.ClientThreadID != "" && r.ClientThreadID != thread {
		return "", fail("opencode_thread_conflict", "historical Run %s is already bound to a different OpenCode conversation", runID)
	}
	if session.ClientThreadID != "" && session.ClientThreadID != thread {
		return "", fail("opencode_thread_conflict", "session %s is already bound to a different OpenCode conversation", session.ID)
	}
	session.ClientThreadID = thread
	r.ClientThreadID = thread
	if err := saveDocument(d); err != nil {
		return "", err
	}
	return thread, nil
}

func chooseHistoricalOpenCodeThread(items []openCodeSession, d *Document, session Session, history []Run) (string, string, error) {
	sort.SliceStable(history, func(i, j int) bool {
		if history[i].Generation != history[j].Generation {
			return history[i].Generation < history[j].Generation
		}
		if !history[i].CreatedAt.Equal(history[j].CreatedAt) {
			return history[i].CreatedAt.Before(history[j].CreatedAt)
		}
		return history[i].ID < history[j].ID
	})

	claimed := make(map[string]string)
	for _, other := range d.Registry.Sessions {
		if other.ID == session.ID || !other.Active() || other.ClientSnapshot.Adapter != "opencode" {
			continue
		}
		if other.ClientThreadID != "" {
			claimed[other.ClientThreadID] = other.ID
		}
		if other.CurrentRunID != "" {
			if run, err := findRun(d, other.CurrentRunID); err == nil && run.ClientThreadID != "" {
				claimed[run.ClientThreadID] = other.ID
			}
		}
	}

	for _, run := range history {
		cwd := run.CWD
		if cwd == "" {
			cwd = session.CWD
		}
		matches := make([]openCodeSession, 0)
		seen := make(map[string]struct{})
		for _, item := range items {
			if item.ID == "" || item.Created == 0 || !sameOpenCodePath(item.Directory, cwd) {
				continue
			}
			if _, ok := claimed[item.ID]; ok {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			created := time.UnixMilli(item.Created)
			if created.Before(run.CreatedAt.Add(-openCodeRecoveryClockSkew)) || created.After(run.CreatedAt.Add(openCodeRecoveryWindow)) {
				continue
			}
			seen[item.ID] = struct{}{}
			matches = append(matches, item)
		}
		if len(matches) == 0 {
			continue
		}
		if len(matches) > 1 {
			sort.Slice(matches, func(i, j int) bool {
				if matches[i].Created != matches[j].Created {
					return matches[i].Created < matches[j].Created
				}
				return matches[i].ID < matches[j].ID
			})
			ids := make([]string, len(matches))
			for i, item := range matches {
				ids[i] = item.ID
			}
			return "", "", fail("opencode_thread_ambiguous", "cannot safely recover session %s: Run %s has multiple OpenCode conversations in its detection window (%s); use `workspace session bind-thread %s --thread-id <thread-id>` and retry", session.ID, run.ID, strings.Join(ids, ", "), session.ID)
		}
		return matches[0].ID, run.ID, nil
	}
	return "", "", nil
}

func chooseOpenCodeThread(items []openCodeSession, cwd string, known map[string]struct{}, startedAt time.Time, baselineOK bool) string {
	var best openCodeSession
	for _, item := range items {
		if item.ID == "" || !sameOpenCodePath(item.Directory, cwd) {
			continue
		}
		if _, exists := known[item.ID]; exists {
			continue
		}
		if !baselineOK && (item.Created == 0 || item.Created < startedAt.Add(-time.Second).UnixMilli()) {
			continue
		}
		if best.ID == "" || item.Updated > best.Updated || item.Updated == best.Updated && item.Created > best.Created {
			best = item
		}
	}
	return best.ID
}

func sameOpenCodePath(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	left = cleanOpenCodePath(left)
	right = cleanOpenCodePath(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func cleanOpenCodePath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
