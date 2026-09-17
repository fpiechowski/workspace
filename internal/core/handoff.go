package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxArtifactSize int64 = 16 * 1024 * 1024

// Snapshot only explicit, regular files within a worktree. File identity checks
// detect replacement while reading, and the stored bytes are hashed, not the source path.
func readArtifact(root, path string) ([]byte, string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	if !contained(root, abs) {
		return nil, "", fail("artifact_invalid", "artifact escapes assigned worktree")
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, "", err
	}
	if !contained(root, real) {
		return nil, "", fail("artifact_invalid", "artifact symlink escapes assigned worktree")
	}
	before, err := os.Stat(real)
	if err != nil {
		return nil, "", err
	}
	if !before.Mode().IsRegular() || before.Size() > maxArtifactSize {
		return nil, "", fail("artifact_invalid", "expected regular file up to 16 MiB")
	}
	f, err := os.Open(real)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !os.SameFile(before, opened) {
		return nil, "", fail("artifact_changed", "source replaced while opening")
	}
	b, err := io.ReadAll(io.LimitReader(f, maxArtifactSize+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > maxArtifactSize {
		return nil, "", fail("artifact_invalid", "artifact exceeds 16 MiB")
	}
	after, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if after.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime()) {
		return nil, "", fail("artifact_changed", "source changed while reading")
	}
	return b, filepath.Base(abs), nil
}

type HandoffOptions struct {
	CheckIDs                                          []string
	To, Task, Session, Outcome, Summary, OperationKey string
	Artifacts                                         []string
	Checks                                            []Check
	Risks                                             []string
}

func (s *Service) SubmitHandoff(ctx context.Context, selector string, opt HandoffOptions) (Handoff, error) {
	var out Handoff
	if opt.Outcome == "" {
		opt.Outcome = "succeeded"
	}
	if opt.Outcome != "succeeded" && opt.Outcome != "blocked" && opt.Outcome != "failed" {
		return out, fail("invalid_handoff", "invalid outcome")
	}
	if strings.TrimSpace(opt.Summary) == "" {
		return out, fail("invalid_handoff", "summary is required")
	}
	if len(opt.Artifacts)+len(opt.CheckIDs) > 32 {
		return out, fail("artifact_invalid", "at most 32 explicit artifacts per handoff")
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
			return err
		}
		if id != "" {
			h, err := findHandoff(d, id)
			if err != nil {
				return err
			}
			out = *h
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
			return fail("forbidden", "cannot submit on behalf of another session")
		}
		if p.TaskID == "" {
			return fail("task_required", "handoff requires a task-bound session")
		}
		run, err := provenanceRun(d, p)
		if err != nil {
			return err
		}
		t, err := findTask(d, p.TaskID)
		if err != nil {
			return err
		}
		if opt.Task != "" && opt.Task != t.ID && opt.Task != t.Name {
			return fail("task_mismatch", "session is assigned to another task")
		}
		to := opt.To
		if to == "" {
			to = p.ParentAgentID
		}
		a, err := findAgent(d, to)
		if err != nil {
			return err
		}
		if a.ID != p.ParentAgentID && a.ID != d.State.OrchestratorAgentID {
			return fail("invalid_recipient", "send task results to the delegating agent or orchestrator")
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
		prompt, err := os.ReadFile(p.PromptFile)
		if err != nil {
			return err
		}
		out = Handoff{ID: ID("handoff"), FromAgent: p.AgentID, FromSession: p.ID, FromRun: run.ID, ToAgent: a.ID, TaskID: t.ID, Attempt: p.TaskAttempt, InputDigest: p.InputDigest, Outcome: opt.Outcome, Summary: opt.Summary, State: "submitted", BaseCommit: w.BaseCommit, HeadCommit: head, Dirty: dirty != "", Checks: append([]Check(nil), opt.Checks...), Risks: opt.Risks, CreatedAt: time.Now().UTC()}
		out.Stale = p.TaskAttempt != t.Attempt || p.InputDigest != t.InputDigest || t.RunID != run.ID || t.State == "accepted"
		artifacts := []Artifact{}
		bytes := [][]byte{}
		names := map[string]string{}
		for _, path := range opt.Artifacts {
			b, name, err := readArtifact(w.Path, path)
			if err != nil {
				return err
			}
			if names[name] != "" {
				return fail("artifact_invalid", "duplicate artifact filename %s", name)
			}
			id := ID("art")
			names[name] = id
			artifact := Artifact{ID: id, Name: name, Kind: "report", Path: filepath.ToSlash(filepath.Join("artifacts", id, name)), Digest: digest(b), Size: int64(len(b)), SourceHandoff: out.ID, AgentID: p.AgentID, SessionID: p.ID, RunID: run.ID, TaskID: t.ID, Model: run.Route.Model, PromptDigest: digest(prompt), BaseCommit: w.BaseCommit, HeadCommit: head, CreatedAt: out.CreatedAt}
			artifacts = append(artifacts, artifact)
			bytes = append(bytes, b)
			out.ArtifactIDs = append(out.ArtifactIDs, id)
		}
		for i, check := range out.Checks {
			if strings.TrimSpace(check.Command) == "" || names[check.Evidence] == "" {
				return fail("invalid_checks", "each check must reference an included evidence filename")
			}
			out.Checks[i].Evidence = names[check.Evidence]
		}
		for _, checkID := range opt.CheckIDs {
			r, err := findCheck(d, checkID)
			if err != nil {
				return err
			}
			if r.SessionID != p.ID || r.RunID != run.ID || r.TaskID != t.ID || r.Attempt != p.TaskAttempt || r.State != "completed" || !r.Clean || r.Head != head || r.EndHead != head {
				return fail("invalid_checks", "check %s does not verify this clean task revision", checkID)
			}
			b, _, err := readArtifact(d.Dir, filepath.Join(".runtime", "checks", r.ID+".log"))
			if err != nil {
				return err
			}
			if digest(b) != r.Digest {
				return fail("artifact_changed", "captured check output changed")
			}
			id := ID("art")
			name := r.ID + ".log"
			artifact := Artifact{ID: id, Name: name, Kind: "check-output", Path: filepath.ToSlash(filepath.Join("artifacts", id, name)), Digest: r.Digest, Size: int64(len(b)), SourceHandoff: out.ID, AgentID: p.AgentID, SessionID: p.ID, RunID: run.ID, TaskID: t.ID, Model: run.Route.Model, PromptDigest: digest(prompt), BaseCommit: w.BaseCommit, HeadCommit: head, CreatedAt: out.CreatedAt}
			artifacts = append(artifacts, artifact)
			bytes = append(bytes, b)
			out.ArtifactIDs = append(out.ArtifactIDs, id)
			out.Checks = append(out.Checks, Check{Command: checkCommand(r.Argv), ExitCode: r.ExitCode, Evidence: id})
		}
		// All validation precedes any copy. Only registered immutable files are exposed.
		for i, a := range artifacts {
			if err := atomicWrite(filepath.Join(d.Dir, a.Path), bytes[i]); err != nil {
				return err
			}
		}
		d.State.Artifacts = append(d.State.Artifacts, artifacts...)
		d.Registry.Handoffs = append(d.Registry.Handoffs, out)
		addMessage(d, p.AgentID, p.ID, a.ID, "handoff", out.Summary, out.ID, "")
		if !out.Stale {
			t.State = "awaiting_review"
		}
		d.remember(opt.OperationKey, opt, out.ID)
		return saveResource(d, opt.OperationKey, out)
	})
	return out, err
}
func verifyArtifact(d *Document, a *Artifact) error {
	b, _, err := readArtifact(d.Dir, a.Path)
	if err != nil {
		return err
	}
	if digest(b) != a.Digest {
		return fail("artifact_changed", "immutable artifact %s has been modified", a.ID)
	}
	return nil
}
func validateHandoff(d *Document, h *Handoff, t *Task) error {
	if h.Stale || h.Attempt != t.Attempt || h.InputDigest != t.InputDigest || h.FromRun == "" || h.FromRun != t.RunID {
		return fail("stale_handoff", "result belongs to an earlier attempt/input")
	}
	run, err := findRun(d, h.FromRun)
	if err != nil || run.SessionID != h.FromSession {
		return fail("stale_handoff", "result execution provenance is missing or inconsistent")
	}
	current, err := taskInputDigest(d, t)
	if err != nil {
		return err
	}
	if current != h.InputDigest {
		return fail("stale_handoff", "input or accepted dependencies changed")
	}
	if h.Outcome != "succeeded" {
		return fail("handoff_failed", "only succeeded results can be accepted")
	}
	names := map[string]bool{}
	for _, id := range h.ArtifactIDs {
		a, err := findArtifact(d, id)
		if err != nil {
			return err
		}
		if err := verifyArtifact(d, a); err != nil {
			return err
		}
		if a.SessionID != h.FromSession || a.RunID != h.FromRun {
			return fail("artifact_changed", "artifact provenance does not match handoff execution")
		}
		names[a.Name] = true
	}
	for _, name := range t.RequiredArtifacts {
		if !names[name] {
			return fail("artifact_required", "required artifact %s is missing", name)
		}
	}
	if t.Role != "planner" && h.Dirty {
		return fail("dirty_worktree", "implementation/integration/test result has uncommitted changes")
	}
	if t.RequireChecks && len(h.Checks) == 0 {
		return fail("checks_required", "verification evidence is required")
	}
	for _, check := range h.Checks {
		if check.ExitCode != 0 {
			return fail("checks_failed", "check %s failed", check.Command)
		}
		a, err := findArtifact(d, check.Evidence)
		if err != nil {
			return err
		}
		if err := verifyArtifact(d, a); err != nil {
			return err
		}
	}
	return nil
}

// promoteRejectedHandoffProvenance makes a successor Run the new source of
// truth only after the pending handoff has been rejected. While the handoff is
// still reviewable, its original Run must remain in Task.RunID so acceptance
// can validate the submitted result.
func promoteRejectedHandoffProvenance(d *Document, h *Handoff, t *Task) {
	if t.RunID == "" || t.RunID != h.FromRun || t.SessionID != h.FromSession || t.State != "needs_changes" {
		return
	}
	p, err := findSession(d, h.FromSession)
	if err != nil || p.DeletedAt != nil || p.ClosedAt != nil || p.AgentSnapshot.Role != t.Role || p.TaskID != t.ID || p.TaskAttempt != t.Attempt || p.InputDigest != t.InputDigest || p.WorktreeID != t.WorktreeID {
		return
	}
	r, err := currentRun(d, p)
	if err != nil || r.ID == h.FromRun {
		return
	}
	prior, err := findRun(d, h.FromRun)
	if err != nil || r.Generation <= prior.Generation {
		return
	}
	t.RunID = r.ID
}

func (s *Service) ReviewHandoff(ctx context.Context, selector, id string, accept bool, feedback string, keys ...string) (Handoff, error) {
	var out Handoff
	err := mutate(s, ctx, selector, keys, []any{"handoff.review", id, accept, feedback}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		h, err := findHandoff(d, id)
		if err != nil {
			return err
		}
		t, err := findTask(d, h.TaskID)
		if err != nil {
			return err
		}
		if accept && h.State == "accepted" {
			if err := validateHandoff(d, h, t); err != nil {
				return err
			}
			out = *h
			return nil
		}
		if h.State != "submitted" {
			return fail("handoff_reviewed", "result is already %s", h.State)
		}
		if h.Stale || t.Attempt != h.Attempt || t.InputDigest != h.InputDigest {
			return fail("stale_handoff", "cannot change task state using an old result")
		}
		if accept {
			if t.State == "accepted" {
				return fail("task_accepted", "another handoff is already accepted")
			}
			if err := validateHandoff(d, h, t); err != nil {
				return err
			}
			if t.Role == "tester" && (d.State.Integration == nil || h.HeadCommit != d.State.Integration.HeadCommit || d.State.LiveTest.Choice != "run") {
				return fail("test_revision_mismatch", "live test is not for the selected integrated revision")
			}
			if t.Role == "integrator" {
				i := d.State.Integration
				if i == nil || i.WorktreeID != t.WorktreeID {
					return fail("integration_required", "integrator must use the prepared integration worktree")
				}
				for _, head := range i.Heads {
					if _, err := git(ctx, s.Root, "merge-base", "--is-ancestor", head, h.HeadCommit); err != nil {
						return fail("integration_incomplete", "integrated commit is missing accepted task head %s", head)
					}
				}
				i.HeadCommit = h.HeadCommit
			}
			h.State = "accepted"
			t.State = "accepted"
			t.AcceptedHandoff = h.ID
			if t.Role == "tester" {
				d.State.LiveTest.AcceptedHandoff = h.ID
				d.State.LiveTest.HeadCommit = h.HeadCommit
			}
		} else {
			if strings.TrimSpace(feedback) == "" {
				return fail("feedback_required", "give a reason for rejection")
			}
			h.State = "rejected"
			t.State = "needs_changes"
			t.Reason = feedback
			promoteRejectedHandoffProvenance(d, h, t)
		}
		h.Feedback = feedback
		out = *h
		for i := range d.Registry.Messages {
			m := &d.Registry.Messages[i]
			if m.HandoffID == h.ID && m.ToAgent == d.State.OrchestratorAgentID {
				now := time.Now().UTC()
				m.AcknowledgedAt = &now
			}
		}
		fromSession := s.Actor.SessionID
		if actor, actorErr := s.actor(d); actorErr == nil && actor != nil {
			fromSession = actor.ID
		}
		addMessage(d, d.State.OrchestratorAgentID, fromSession, h.FromAgent, "review", "Handoff "+h.ID+" "+h.State+". "+feedback, h.ID, "")
		return saveDocument(d)
	})
	return out, err
}
