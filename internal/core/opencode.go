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
