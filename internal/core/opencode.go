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
	"sort"
	"strconv"
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

// reconcileOpenCodeRun restores transport metadata from the immutable Run
// argv immediately before delivery. This closes the gap where an unrelated
// Session mutation had persisted a Run without OpenCodeEndpoint while the
// server was still reachable at the recorded loopback flags.
func (s *Service) reconcileOpenCodeRun(ctx context.Context, selector, runID string) (Session, Run, error) {
	var session Session
	var run Run
	err := s.With(ctx, selector, func(d *Document) error {
		r, err := findRun(d, runID)
		if err != nil {
			return err
		}
		p, err := findSession(d, r.SessionID)
		if err != nil {
			return err
		}
		if p.CurrentRunID != r.ID || !r.Active() {
			return fail("stale_run", "run no longer owns the session runtime")
		}
		changed := false
		if r.ClientThreadID == "" && p.ClientThreadID != "" {
			r.ClientThreadID = p.ClientThreadID
			changed = true
		}
		if r.OpenCodeEndpoint != "" && !validOpenCodeEndpoint(r.OpenCodeEndpoint) {
			// Never use a persisted endpoint that is not a loopback HTTP server.
			r.OpenCodeEndpoint = ""
			changed = true
		}
		if r.OpenCodeEndpoint == "" {
			endpoint, deriveErr := openCodeEndpointFromArgv(r.Argv)
			if deriveErr == nil && endpoint != "" {
				r.OpenCodeEndpoint = endpoint
				changed = true
			}
		}
		if changed {
			d.syncSession(p)
			if err := saveDocument(d); err != nil {
				return err
			}
		}
		session, run = *p, *r
		return nil
	})
	return session, run, err
}

func (s *Service) deliverOpenCodeMessage(ctx context.Context, selector string, session Session, run Run, message Message) error {
	var err error
	session, run, err = s.reconcileOpenCodeRun(ctx, selector, run.ID)
	if err != nil {
		return err
	}
	if !messageAddressMatchesRun(message, session, run) {
		return fail("forbidden", "delivery recipient mismatch for message %s", message.ID)
	}
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
	parsed, err := url.Parse(endpoint)
	host, port := "", ""
	if err == nil {
		host, port = parsed.Hostname(), parsed.Port()
	}
	if host == "" {
		host = "127.0.0.1"
	}
	result := replaceOpenCodeFlag(argv, "--hostname", host)
	return replaceOpenCodeFlag(result, "--port", port)
}

func replaceOpenCodeFlag(argv []string, flag, value string) []string {
	result := append([]string(nil), argv...)
	for i, arg := range result {
		if arg == flag {
			if i+1 < len(result) && !strings.HasPrefix(result[i+1], "--") {
				result[i+1] = value
				return result
			}
			next := make([]string, 0, len(result)+1)
			next = append(next, result[:i+1]...)
			next = append(next, value)
			next = append(next, result[i+1:]...)
			return next
		}
		if strings.HasPrefix(arg, flag+"=") {
			result[i] = flag + "=" + value
			return result
		}
	}
	return append(result, flag, value)
}

// openCodeEndpointFromArgv recovers only the loopback server endpoint that
// workspace itself placed in an immutable Run argv. Missing flags are not an
// error (the caller may use the documented restart-required path); malformed,
// partial or non-loopback flags are rejected and never turned into a guessed
// endpoint.
func openCodeEndpointFromArgv(argv []string) (string, error) {
	var hostname, portText string
	hostFound, portFound := false, false
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if openCodeValueFlag(arg) {
			i++
			continue
		}
		for _, flag := range []string{"--hostname", "--port"} {
			if arg == flag {
				if i+1 >= len(argv) || strings.TrimSpace(argv[i+1]) == "" || strings.HasPrefix(argv[i+1], "--") {
					return "", fail("opencode_endpoint_invalid", "%s has no valid value in Run argv", flag)
				}
				if flag == "--hostname" {
					if hostFound {
						return "", fail("opencode_endpoint_invalid", "Run argv contains duplicate --hostname flags")
					}
					hostname, hostFound = argv[i+1], true
				} else {
					if portFound {
						return "", fail("opencode_endpoint_invalid", "Run argv contains duplicate --port flags")
					}
					portText, portFound = argv[i+1], true
				}
				i++
				break
			}
			prefix := flag + "="
			if strings.HasPrefix(arg, prefix) {
				value := strings.TrimPrefix(arg, prefix)
				if strings.TrimSpace(value) == "" {
					return "", fail("opencode_endpoint_invalid", "%s has no valid value in Run argv", flag)
				}
				if flag == "--hostname" {
					if hostFound {
						return "", fail("opencode_endpoint_invalid", "Run argv contains duplicate --hostname flags")
					}
					hostname, hostFound = value, true
				} else {
					if portFound {
						return "", fail("opencode_endpoint_invalid", "Run argv contains duplicate --port flags")
					}
					portText, portFound = value, true
				}
				break
			}
		}
	}
	if !hostFound && !portFound {
		return "", nil
	}
	if !hostFound || !portFound {
		return "", fail("opencode_endpoint_invalid", "Run argv must contain both --hostname and --port")
	}
	hostname = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(hostname, "]"), "["))
	ip := net.ParseIP(hostname)
	if ip == nil || !ip.IsLoopback() {
		return "", fail("opencode_endpoint_invalid", "OpenCode hostname %q is not loopback", hostname)
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil || port < 1 || port > 65535 {
		return "", fail("opencode_endpoint_invalid", "OpenCode port %q is invalid", portText)
	}
	return "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)), nil
}

func openCodeValueFlag(arg string) bool {
	switch arg {
	case "--model", "--prompt", "--session", "--agent", "--cwd":
		return true
	default:
		return false
	}
}

func validOpenCodeEndpoint(endpoint string) bool {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Hostname() == "" || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	ip := net.ParseIP(parsed.Hostname())
	port, err := strconv.Atoi(parsed.Port())
	return ip != nil && ip.IsLoopback() && err == nil && port >= 1 && port <= 65535
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

	env := os.Environ()
	var err error
	if session.ReasoningEffort != "" {
		env, err = withOpenCodeReasoningEffortEnv(env, session.Argv, session.ReasoningEffort)
		if err != nil {
			return "", err
		}
	}
	items, err := s.listOpenCodeSessions(ctx, *session, env)
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
