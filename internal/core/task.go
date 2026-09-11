package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func ParseTaskSpec(b []byte) (TaskSpec, error) {
	var spec TaskSpec
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if strings.HasPrefix(text, "---\n") {
		head, body, ok := strings.Cut(text[4:], "\n---\n")
		if !ok {
			return spec, fail("invalid_task", "unclosed task frontmatter")
		}
		if err := strictYAML([]byte(head), &spec); err != nil {
			return spec, err
		}
		if strings.TrimSpace(body) != "" {
			spec.Goal += "\n\n" + strings.TrimSpace(body)
		}
	} else if err := strictYAML(b, &spec); err != nil {
		return spec, err
	}
	return spec, nil
}
func findTask(d *Document, id string) (*Task, error) {
	for i := range d.State.Tasks {
		t := &d.State.Tasks[i]
		if t.ID == id || t.Name == id {
			return t, nil
		}
	}
	return nil, fail("task_not_found", "unknown task %q", id)
}
func findHandoff(d *Document, id string) (*Handoff, error) {
	for i := range d.Registry.Handoffs {
		if d.Registry.Handoffs[i].ID == id {
			return &d.Registry.Handoffs[i], nil
		}
	}
	return nil, fail("handoff_not_found", "unknown handoff %q", id)
}
func findArtifact(d *Document, id string) (*Artifact, error) {
	for i := range d.State.Artifacts {
		if d.State.Artifacts[i].ID == id {
			return &d.State.Artifacts[i], nil
		}
	}
	return nil, fail("artifact_not_found", "unknown artifact %q", id)
}
func taskInputDigest(d *Document, t *Task) (string, error) {
	b, err := os.ReadFile(filepath.Join(d.Dir, d.State.Input.Snapshot))
	if err != nil {
		return "", err
	}
	deps := []string{}
	for _, id := range t.DependsOn {
		dep, err := findTask(d, id)
		if err != nil {
			return "", err
		}
		deps = append(deps, dep.ID+":"+dep.AcceptedHandoff)
	}
	return payloadDigest(struct {
		Spec         TaskSpec
		Input, Base  string
		Dependencies []string
	}{t.TaskSpec, digest(b), d.State.Base.Commit, deps}), nil
}
func (s *Service) CreateTask(ctx context.Context, selector string, spec TaskSpec, key string) (Task, error) {
	var out Task
	if strings.TrimSpace(spec.Title) == "" || strings.TrimSpace(spec.Goal) == "" || len(spec.AcceptanceCriteria) == 0 {
		return out, fail("invalid_task", "title, goal and acceptance_criteria are required")
	}
	defaults := map[string][2]string{"planner": {"frontier", "PLAN.md"}, "implementer": {"implementation", "IMPLEMENTATION.md"}, "integrator": {"implementation", "INTEGRATION.md"}, "tester": {"live-testing", "LIVE_TEST.md"}}
	def, ok := defaults[spec.Role]
	if !ok {
		return out, fail("invalid_task", "unsupported task role")
	}
	if spec.Name != "" {
		if err := validateName(spec.Name); err != nil {
			return out, err
		}
	}
	if len(spec.RequiredArtifacts) == 0 {
		spec.RequiredArtifacts = []string{def[1]}
	}
	if spec.Role == "implementer" || spec.Role == "integrator" || spec.Role == "tester" {
		spec.RequireChecks = true
	}
	for _, name := range spec.RequiredArtifacts {
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			return out, fail("invalid_task", "required artifacts must be filenames")
		}
	}
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		id, err := d.previous(key, spec)
		if err != nil {
			return err
		}
		if found, err := replayResource(d, key, &out); found || err != nil {
			return err
		}
		if id == "" && (d.State.Status == "completed" || d.State.Status == "archived") {
			return fail("workspace_closed", "workspace is closed")
		}
		if id != "" {
			t, err := findTask(d, id)
			if err != nil {
				return err
			}
			out = *t
			return nil
		}
		if spec.Name != "" {
			for _, t := range d.State.Tasks {
				if t.Name == spec.Name {
					return fail("task_exists", "task name already exists")
				}
			}
		}
		seen := map[string]bool{}
		normalized := spec
		if normalized.Profile == "" {
			cfg, err := s.Config()
			if err != nil {
				return err
			}
			normalized.Profile = workflowProfile(cfg, d, spec.Role, def[0])
		}
		normalized.DependsOn = append([]string(nil), spec.DependsOn...)
		for i, depID := range spec.DependsOn {
			dep, err := findTask(d, depID)
			if err != nil {
				return err
			}
			if seen[dep.ID] {
				return fail("invalid_task", "duplicate dependency")
			}
			seen[dep.ID] = true
			normalized.DependsOn[i] = dep.ID
		}
		if spec.BaseCommit != "" {
			base, err := git(ctx, s.Root, "rev-parse", "--verify", "--end-of-options", spec.BaseCommit+"^{commit}")
			if err != nil {
				return err
			}
			normalized.BaseCommit = base
		}
		out = Task{TaskSpec: normalized, ID: ID("task"), State: "pending", Attempt: 1}
		out.InputDigest, err = taskInputDigest(d, &out)
		if err != nil {
			return err
		}
		b, err := yaml.Marshal(out.TaskSpec)
		if err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(d.Dir, "tasks", out.ID+".md"), []byte("---\n"+string(b)+"---\n")); err != nil {
			return err
		}
		d.State.Tasks = append(d.State.Tasks, out)
		d.remember(key, spec, out.ID)
		return saveResource(d, key, out)
	})
	return out, err
}
func taskReady(d *Document, t *Task) error {
	if t.State != "pending" && t.State != "needs_changes" && t.State != "blocked" {
		return fail("task_not_ready", "task is %s", t.State)
	}
	for _, id := range t.DependsOn {
		dep, err := findTask(d, id)
		if err != nil {
			return err
		}
		if dep.State != "accepted" {
			return fail("dependency_pending", "dependency %s is %s", id, dep.State)
		}
	}
	return nil
}
func (s *Service) RetryTask(ctx context.Context, selector, id, reason, key string) (Task, error) {
	var out Task
	request := struct{ Task, Reason string }{id, reason}
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		previous, err := d.previous(key, request)
		if err != nil {
			return err
		}
		if found, err := replayResource(d, key, &out); found || err != nil {
			return err
		}
		if previous != "" {
			t, err := findTask(d, previous)
			if err != nil {
				return err
			}
			out = *t
			return nil
		}
		t, err := findTask(d, id)
		if err != nil {
			return err
		}
		if d.State.Release.UserConfirmed {
			return fail("workspace_closed", "released work cannot be silently reopened")
		}
		affected := map[string]bool{t.ID: true}
		changed := true
		for changed {
			changed = false
			for _, candidate := range d.State.Tasks {
				for _, dep := range candidate.DependsOn {
					if affected[dep] && !affected[candidate.ID] {
						affected[candidate.ID] = true
						changed = true
					}
				}
			}
		}
		for _, p := range d.Registry.Sessions {
			if p.Active() && affected[p.TaskID] {
				return fail("task_busy", "stop session %s before retrying affected tasks", p.ID)
			}
		}
		for i := range d.State.Tasks {
			candidate := &d.State.Tasks[i]
			if !affected[candidate.ID] {
				continue
			}
			candidate.Attempt++
			candidate.State = "pending"
			candidate.AcceptedHandoff = ""
			candidate.SessionID = ""
			candidate.Reason = reason
		}
		if d.State.Workflow != nil {
			switch t.Role {
			case "planner":
				d.State.Workflow.Phase = "planning"
			case "implementer":
				d.State.Workflow.Phase = "implementing"
			case "integrator":
				d.State.Workflow.Phase = "integrating"
			case "tester":
				d.State.Workflow.Phase = "live_testing"
			}
		}
		if t.Role == "planner" || t.Role == "implementer" {
			d.State.Integration = nil
		}
		if t.Role == "integrator" && d.State.Integration != nil {
			d.State.Integration.HeadCommit = ""
		}
		if t.Role == "tester" {
			d.State.LiveTest.AcceptedHandoff = ""
		} else {
			d.State.LiveTest = LiveTest{}
		}
		d.State.PendingDecision = nil
		if t.Role != "tester" {
			for i := range d.State.ChangeRequests {
				d.State.ChangeRequests[i].State = "outdated"
			}
		}
		d.remember(key, request, t.ID)
		out = *t
		return saveResource(d, key, out)
	})
	return out, err
}

// bindTask validates a current attempt, then records its concrete Session.
func bindTask(ctx context.Context, d *Document, p *Session, taskID string) error {
	if taskID == "" {
		return nil
	}
	t, err := findTask(d, taskID)
	if err != nil {
		return err
	}
	if t.Role != p.AgentSnapshot.Role {
		return fail("role_mismatch", "task and agent roles differ")
	}
	if err := taskReady(d, t); err != nil {
		return err
	}
	if t.BaseCommit != "" {
		head, err := git(ctx, p.CWD, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		if _, err := git(ctx, p.CWD, "merge-base", "--is-ancestor", t.BaseCommit, head); err != nil {
			return fail("base_mismatch", "worktree does not contain task base")
		}
	}
	for _, depID := range t.DependsOn {
		dep, _ := findTask(d, depID)
		if dep.Role == "planner" || t.Role == "integrator" {
			continue
		}
		h, err := findHandoff(d, dep.AcceptedHandoff)
		if err != nil {
			return err
		}
		if _, err := git(ctx, p.CWD, "merge-base", "--is-ancestor", h.HeadCommit, "HEAD"); err != nil {
			return fail("base_mismatch", "worktree is missing accepted dependency %s", depID)
		}
	}
	input, err := taskInputDigest(d, t)
	if err != nil {
		return err
	}
	p.TaskID = t.ID
	p.TaskAttempt = t.Attempt
	p.InputDigest = input
	t.InputDigest = input
	t.SessionID = p.ID
	t.WorktreeID = p.WorktreeID
	t.State = "running"
	return nil
}
