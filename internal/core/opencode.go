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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// OpenCode is local, but its HTTP handler can still stop responding while
	// the interactive TUI is busy. Keep each request and the complete delivery
	// attempt bounded so the sequential supervisor can continue with later
	// messages and workspaces.
	openCodeDeliveryTimeout              = 5 * time.Second
	openCodeRequestTimeout               = 750 * time.Millisecond
	openCodeReadinessTimeout             = 2 * time.Second
	openCodeRequestInterval              = 100 * time.Millisecond
	openCodeHistoryLimit                 = 100
	openCodePersistenceTimeout           = 1 * time.Second
	openCodeResponseLimit                = 4 << 20
	openCodeDeliveryPhaseChecking        = "checking"
	openCodeDeliveryPhaseRetry           = "retry"
	openCodeDeliveryPhaseAppended        = "appended"
	openCodeDeliveryPhaseSubmitted       = "submitted"
	openCodeDeliveryPhaseUncertainAppend = "uncertain_append"
	// "uncertain" was the phase used by the previous asynchronous transport
	// after acceptance without history confirmation. Keep it compatible and
	// treat it as an uncertain submit on retry.
	openCodeDeliveryPhaseUncertainSubmit = "uncertain"
	openCodeDeliveryPhaseConfirmed       = "confirmed"
)

func (s *Service) openCodeHTTP(ctx context.Context, endpoint, method, path string, body []byte) ([]byte, int, error) {
	if endpoint == "" {
		return nil, 0, fail("opencode_unavailable", "current Run has no OpenCode endpoint")
	}
	requestCtx, cancel := context.WithTimeout(ctx, openCodeRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, strings.TrimRight(endpoint, "/")+path, bytes.NewReader(body))
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
	data, readErr := io.ReadAll(io.LimitReader(response.Body, openCodeResponseLimit))
	if readErr != nil {
		return nil, response.StatusCode, readErr
	}
	return data, response.StatusCode, nil
}

func (s *Service) openCodeReady(ctx context.Context, endpoint string) error {
	deadline := time.NewTimer(openCodeReadinessTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(openCodeRequestInterval)
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
	path := fmt.Sprintf("/session/%s/message?limit=%d", url.PathEscape(thread), openCodeHistoryLimit)
	data, status, err := s.openCodeHTTP(ctx, endpoint, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fail("opencode_rejected", "OpenCode history returned HTTP %d", status)
	}
	return data, nil
}

type openCodeMutationError struct {
	operation string
	uncertain bool
	err       error
}

func (e *openCodeMutationError) Error() string {
	if e.err == nil {
		return "OpenCode mutation failed"
	}
	return e.err.Error()
}

func (e *openCodeMutationError) Unwrap() error { return e.err }

// openCodeTUIMutation calls one of OpenCode's TUI control routes. A response
// that is not a 2xx JSON boolean true is a rejected operation. A transport or
// malformed-response error is uncertain because the TUI may have applied the
// mutation before the response was lost.
func (s *Service) openCodeTUIMutation(ctx context.Context, endpoint, path string, body []byte, operation string) error {
	data, status, err := s.openCodeHTTP(ctx, endpoint, http.MethodPost, path, body)
	if err != nil {
		return &openCodeMutationError{operation: operation, uncertain: true, err: fmt.Errorf("OpenCode %s request failed: %w", operation, err)}
	}
	if status < 200 || status >= 300 {
		return &openCodeMutationError{operation: operation, err: fail("opencode_rejected", "OpenCode %s returned HTTP %d: %s", operation, status, strings.TrimSpace(string(data)))}
	}
	var accepted bool
	if err := json.Unmarshal(data, &accepted); err != nil {
		return &openCodeMutationError{operation: operation, uncertain: true, err: fmt.Errorf("OpenCode %s returned invalid boolean response: %w", operation, err)}
	}
	if !accepted {
		return &openCodeMutationError{operation: operation, err: fail("opencode_rejected", "OpenCode %s rejected the operation", operation)}
	}
	return nil
}

func (s *Service) openCodeDeliveryPhase(ctx context.Context, selector, messageID, runID string) (string, error) {
	var phase string
	err := s.With(ctx, selector, func(d *Document) error {
		run, err := findRun(d, runID)
		if err != nil {
			return err
		}
		p, err := findSession(d, run.SessionID)
		if err != nil {
			return err
		}
		if p.CurrentRunID != run.ID || !run.Active() {
			return fail("stale_run", "run no longer owns the session runtime")
		}
		for i := len(d.Registry.Deliveries) - 1; i >= 0; i-- {
			attempt := d.Registry.Deliveries[i]
			if attempt.MessageID == messageID && attempt.RunID == runID {
				phase = attempt.Phase
				break
			}
		}
		return nil
	})
	return phase, err
}

func openCodePartialDeliveryPhase(phase string) bool {
	switch phase {
	case openCodeDeliveryPhaseAppended, openCodeDeliveryPhaseSubmitted,
		openCodeDeliveryPhaseUncertainAppend, openCodeDeliveryPhaseUncertainSubmit,
		openCodeDeliveryPhaseConfirmed, "uncertain_submit", "needs_attention":
		return true
	default:
		return false
	}
}

func openCodeAppendUncertainPhase(phase string) bool {
	return phase == openCodeDeliveryPhaseUncertainAppend || phase == openCodeDeliveryPhaseConfirmed || phase == "needs_attention"
}

// persistOpenCodePhase retries a phase write with a short background context
// after supervisor cancellation. The external TUI mutation already happened
// at this point, so keeping the partial-delivery receipt is safer than
// allowing a canceled turn to cause an unconditional duplicate append.
func (s *Service) persistOpenCodePhase(ctx context.Context, selector, messageID, runID, phase, deliveryErr string) error {
	err := s.setDeliveryPhase(ctx, selector, messageID, runID, phase, deliveryErr)
	if err == nil || !isContextError(err) {
		return err
	}
	persistCtx, cancel := context.WithTimeout(context.Background(), openCodePersistenceTimeout)
	defer cancel()
	return s.setDeliveryPhase(persistCtx, selector, messageID, runID, phase, deliveryErr)
}

func (s *Service) persistOpenCodeDelivered(ctx context.Context, selector, runID, messageID string) error {
	err := s.markDelivered(ctx, selector, runID, []string{messageID})
	if err == nil || !isContextError(err) {
		return err
	}
	persistCtx, cancel := context.WithTimeout(context.Background(), openCodePersistenceTimeout)
	defer cancel()
	return s.markDelivered(persistCtx, selector, runID, []string{messageID})
}

func (s *Service) openCodeDeliveryFailure(ctx context.Context, selector, messageID, runID, phase string, err error) {
	if err == nil {
		return
	}
	failurePhase := openCodeDeliveryPhaseRetry
	if openCodePartialDeliveryPhase(phase) {
		failurePhase = phase
	}
	// A background, bounded write keeps errors visible even when the request
	// context was canceled by supervisor shutdown. A stale Run is rejected by
	// setDeliveryPhase and must not be resurrected.
	persistCtx, cancel := context.WithTimeout(context.Background(), openCodePersistenceTimeout)
	defer cancel()
	_ = s.setDeliveryPhase(persistCtx, selector, messageID, runID, failurePhase, err.Error())
}

func openCodePrompt(message Message) string {
	marker := "[workspace-message-id:" + message.ID + "]"
	return fmt.Sprintf("\n\n--- workspace handoff ---\n%s\nType: %s\nFrom: %s\nHandoff: %s\n\n%s\n\nRead the durable inbox record before acting: message_id=%s.\n--- end workspace handoff ---\n\n", marker, message.Kind, message.FromAgent, message.HandoffID, message.Body, message.ID)
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
	attemptCtx, cancel := context.WithTimeout(ctx, openCodeDeliveryTimeout)
	defer cancel()
	marker := "[workspace-message-id:" + message.ID + "]"
	phase, err := s.openCodeDeliveryPhase(attemptCtx, selector, message.ID, run.ID)
	if err != nil {
		return err
	}
	if phase == "" || !openCodePartialDeliveryPhase(phase) && phase != openCodeDeliveryPhaseConfirmed {
		if err := s.persistOpenCodePhase(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseChecking, ""); err != nil {
			return err
		}
		phase = openCodeDeliveryPhaseChecking
	}
	if err := s.openCodeReady(attemptCtx, run.OpenCodeEndpoint); err != nil {
		s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, phase, err)
		return err
	}
	history, historyErr := s.openCodeHistory(attemptCtx, run.OpenCodeEndpoint, run.ClientThreadID)
	if historyErr == nil && openCodeHistoryContains(history, marker) {
		if err := s.persistOpenCodePhase(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseConfirmed, ""); err != nil {
			s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseConfirmed, err)
			return err
		}
		return s.persistOpenCodeDelivered(attemptCtx, selector, run.ID, message.ID)
	}
	if historyErr != nil {
		s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, phase, historyErr)
		return historyErr
	}
	if openCodeAppendUncertainPhase(phase) {
		err := fail("delivery_uncertain", "OpenCode append outcome is uncertain; refusing to append the handoff again until history contains %s", marker)
		s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, phase, err)
		return err
	}
	selectBody, _ := json.Marshal(map[string]string{"sessionID": run.ClientThreadID})
	if err := s.openCodeTUIMutation(attemptCtx, run.OpenCodeEndpoint, "/tui/select-session", selectBody, "select-session"); err != nil {
		s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, phase, err)
		return err
	}

	if !openCodePartialDeliveryPhase(phase) {
		body, _ := json.Marshal(map[string]string{"text": openCodePrompt(message)})
		if err := s.openCodeTUIMutation(attemptCtx, run.OpenCodeEndpoint, "/tui/append-prompt", body, "append-prompt"); err != nil {
			var mutationErr *openCodeMutationError
			uncertain := errors.As(err, &mutationErr) && mutationErr.uncertain
			failurePhase := phase
			if uncertain {
				failurePhase = openCodeDeliveryPhaseUncertainAppend
			}
			s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, failurePhase, err)
			return err
		}
		if err := s.persistOpenCodePhase(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseAppended, ""); err != nil {
			// The append has succeeded but its receipt did not become durable.
			// Never fall back to a fresh append on the next tick.
			s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseUncertainAppend, err)
			return err
		}
	}
	if err := s.openCodeTUIMutation(attemptCtx, run.OpenCodeEndpoint, "/tui/submit-prompt", []byte("{}"), "submit-prompt"); err != nil {
		var mutationErr *openCodeMutationError
		if errors.As(err, &mutationErr) && mutationErr.uncertain {
			s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseUncertainSubmit, err)
		} else {
			failurePhase := phase
			if !openCodePartialDeliveryPhase(failurePhase) {
				failurePhase = openCodeDeliveryPhaseAppended
			}
			s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, failurePhase, err)
		}
		return err
	}
	if err := s.persistOpenCodePhase(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseSubmitted, ""); err != nil {
		s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseUncertainSubmit, err)
		return err
	}
	for {
		history, historyErr := s.openCodeHistory(attemptCtx, run.OpenCodeEndpoint, run.ClientThreadID)
		if historyErr == nil && openCodeHistoryContains(history, marker) {
			if err := s.persistOpenCodePhase(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseConfirmed, ""); err != nil {
				s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseConfirmed, err)
				return err
			}
			return s.persistOpenCodeDelivered(attemptCtx, selector, run.ID, message.ID)
		}
		select {
		case <-attemptCtx.Done():
			deliveryErr := fail("delivery_unconfirmed", "OpenCode prompt was submitted but marker %s was not observed in session history", marker)
			if historyErr != nil {
				deliveryErr = fmt.Errorf("%w (last history check: %v)", deliveryErr, historyErr)
			}
			s.openCodeDeliveryFailure(attemptCtx, selector, message.ID, run.ID, openCodeDeliveryPhaseUncertainSubmit, deliveryErr)
			return deliveryErr
		case <-time.After(openCodeRequestInterval):
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
)

type openCodeSession struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
	Created   int64  `json:"created"`
	Updated   int64  `json:"updated"`
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
	if session.OpenCodeEndpoint != "" {
		data, status, err := s.openCodeHTTP(ctx, session.OpenCodeEndpoint, http.MethodGet, "/session", nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("OpenCode session list returned HTTP %d", status)
		}
		var sessions []openCodeSession
		if err := json.Unmarshal(data, &sessions); err != nil {
			return nil, fmt.Errorf("invalid OpenCode session list: %w", err)
		}
		return sessions, nil
	}
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
