package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type CheckReceipt struct {
	ID           string     `json:"id" yaml:"id"`
	SessionID    string     `json:"session_id" yaml:"session_id"`
	RunID        string     `json:"run_id" yaml:"run_id"`
	TaskID       string     `json:"task_id" yaml:"task_id"`
	Attempt      int        `json:"attempt" yaml:"attempt"`
	Argv         []string   `json:"argv" yaml:"argv"`
	Head         string     `json:"head" yaml:"head"`
	EndHead      string     `json:"end_head" yaml:"end_head"`
	Clean        bool       `json:"clean" yaml:"clean"`
	State        string     `json:"state" yaml:"state"`
	ExitCode     int        `json:"exit_code" yaml:"exit_code"`
	ExpectedExit *int       `json:"expected_exit,omitempty" yaml:"expected_exit,omitempty"`
	Output       string     `json:"output" yaml:"output"`
	Digest       string     `json:"digest" yaml:"digest"`
	StartedAt    time.Time  `json:"started_at" yaml:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
}
type CheckOptions struct {
	Session, OperationKey string
	Argv                  []string
	ExpectedExit          *int
}

func findCheck(d *Document, id string) (*CheckReceipt, error) {
	for i := range d.Registry.Checks {
		if d.Registry.Checks[i].ID == id {
			return &d.Registry.Checks[i], nil
		}
	}
	return nil, fail("check_not_found", "unknown check %s", id)
}

type checkOutput struct {
	mu   sync.Mutex
	file *os.File
	size int64
}

func (w *checkOutput) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(b)) > maxArtifactSize {
		return 0, fail("output_limit", "check output exceeds 16 MiB")
	}
	n, err := w.file.Write(b)
	w.size += int64(n)
	return n, err
}

func (s *Service) RunCheck(ctx context.Context, selector string, opt CheckOptions) (CheckReceipt, error) {
	var out CheckReceipt
	var cwd, workspaceDir string
	replay := false
	if len(opt.Argv) == 0 || opt.Argv[0] == "" {
		return out, fail("command_required", "provide a command after --")
	}
	err := s.With(ctx, selector, func(d *Document) error {
		actor, err := s.actor(d)
		if err != nil {
			return err
		}
		id, err := d.previous(opt.OperationKey, opt)
		if err != nil {
			return err
		}
		if found, err := replayResource(d, opt.OperationKey, &out); found || err != nil {
			replay = found
			return err
		}
		if err := rejectNewWorkspaceWork(d, "running checks"); err != nil {
			return err
		}
		if id != "" {
			r, err := findCheck(d, id)
			if err != nil {
				return err
			}
			out = *r
			replay = true
			return nil
		}
		sessionID := opt.Session
		if sessionID == "" && actor != nil {
			sessionID = actor.ID
		}
		p, err := findSession(d, sessionID)
		if err != nil {
			return err
		}
		if actor != nil && actor.ID != p.ID {
			return fail("forbidden", "checks must run in your assigned session")
		}
		if !p.Active() || p.TaskID == "" {
			return fail("task_required", "check requires an active task-bound session")
		}
		run, err := currentRun(d, p)
		if err != nil {
			return err
		}
		if run.ConversationOnly {
			return fail("conversation_only", "conversation-only Runs cannot create task checks; reopen the workspace before doing work")
		}
		for _, r := range d.Registry.Checks {
			if r.RunID == run.ID && r.State == "running" {
				return fail("check_busy", "check %s is still running or needs reconciliation", r.ID)
			}
		}
		w, err := findWorktree(d, p.WorktreeID)
		if err != nil {
			return err
		}
		if err := verifyWorktree(ctx, w); err != nil {
			return err
		}
		head, err := git(ctx, w.Path, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		dirty, err := git(ctx, w.Path, "status", "--porcelain")
		if err != nil {
			return err
		}
		out = CheckReceipt{ID: ID("check"), SessionID: p.ID, RunID: run.ID, TaskID: p.TaskID, Attempt: p.TaskAttempt, Argv: append([]string(nil), opt.Argv...), ExpectedExit: opt.ExpectedExit, Head: head, Clean: dirty == "", State: "running", ExitCode: -1, StartedAt: nowUTC()}
		out.Output = filepath.ToSlash(filepath.Join("work-products", "checks", out.ID+".log"))
		cwd = w.Path
		workspaceDir = d.Dir
		d.Registry.Checks = append(d.Registry.Checks, out)
		d.remember(opt.OperationKey, opt, out.ID)
		return saveDocument(d)
	})
	if err != nil || replay {
		return out, err
	}
	// Any failure after reservation must release the logical check slot. A
	// crashed process remains running until its Session is reconciled instead.
	defer func() {
		_ = s.With(context.Background(), selector, func(d *Document) error {
			r, err := findCheck(d, out.ID)
			if err != nil {
				return err
			}
			if r.State != "running" {
				return nil
			}
			r.State = "failed"
			r.ExitCode = -1
			now := nowUTC()
			r.FinishedAt = &now
			return saveDocument(d)
		})
	}()
	// Execute outside the project lock so other agents and inbox delivery continue.
	path := filepath.Join(cwd, out.Output)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return out, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return out, err
	}
	writer := &checkOutput{file: f}
	cmd := exec.CommandContext(ctx, opt.Argv[0], opt.Argv[1:]...)
	cmd.Dir = cwd
	cmd.Stdout = writer
	cmd.Stderr = writer
	runErr := cmd.Run()
	syncErr := f.Sync()
	closeErr := f.Close()
	out.ExitCode = 0
	if runErr != nil {
		out.ExitCode = 1
		var exit *exec.ExitError
		if errors.As(runErr, &exit) {
			out.ExitCode = exit.ExitCode()
		}
	}
	b, _, readErr := readArtifact(cwd, out.Output)
	out.Digest = digest(b)
	out.EndHead, _ = git(context.Background(), cwd, "rev-parse", "HEAD")
	dirty, statusErr := git(context.Background(), cwd, "status", "--porcelain")
	out.Clean = out.Clean && dirty == "" && statusErr == nil && out.Head == out.EndHead
	out.State = "completed"
	finished := nowUTC()
	out.FinishedAt = &finished
	if readErr != nil || syncErr != nil || closeErr != nil {
		out.State = "failed"
		out.ExitCode = -1
	}
	// Keep the captured bytes in the workspace as well; worktree edits cannot
	// rewrite the evidence used for subsequent handoff validation.
	if readErr == nil {
		if err := atomicWrite(filepath.Join(workspaceDir, ".runtime", "checks", out.ID+".log"), b); err != nil {
			return out, err
		}
	}
	err = s.With(context.Background(), selector, func(d *Document) error {
		r, err := findCheck(d, out.ID)
		if err != nil {
			return err
		}
		*r = out
		return saveResource(d, opt.OperationKey, out)
	})
	return out, errors.Join(err, readErr, syncErr, closeErr)
}

func checkCommand(argv []string) string { b, _ := json.Marshal(argv); return string(b) }

var _ io.Writer = (*checkOutput)(nil)
