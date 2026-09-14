package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

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
		if err := s.clientState(discoveryCtx, selector, session.ID, thread, "idle"); err != nil {
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
