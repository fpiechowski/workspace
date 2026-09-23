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
	for _, name := range spec.RequiredArtifacts {
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			return out, fail("invalid_task", "required artifacts must be filenames")
		}
	}
	err := s.With(ctx, selector, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if d.State.WorkflowSelected() {
			if err := requireWorkflowRole(d, spec.Role); err != nil {
				return err
			}
		}
		id, err := d.previous(key, spec)
		if err != nil {
			return err
		}
		if found, err := replayResource(d, key, &out); found || err != nil {
			return err
		}
		if id == "" {
			if err := rejectNewWorkspaceWork(d, "creating tasks"); err != nil {
				return err
			}
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
		if spec.Role == "implementer" && d.State.WorkflowSelected() && workflowHasCapability(d, capPlannerDepends) {
			linked := false
			for _, candidate := range d.State.Tasks {
				if candidate.Role == "planner" && !retiredTask(candidate) {
					for _, depID := range normalized.DependsOn {
						if depID == candidate.ID {
							linked = true
						}
					}
				}
			}
			if !linked {
				return fail("workflow_gate", "implementation task must depend on a planner task; the planner must be accepted before implementation starts")
			}
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
func taskDependenciesReady(d *Document, t *Task) error {
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
func taskReady(d *Document, t *Task) error {
	if t.State != "pending" && t.State != "needs_changes" && t.State != "blocked" {
		return fail("task_not_ready", "task is %s", t.State)
	}
	return taskDependenciesReady(d, t)
}

// RetireTask records a terminal decision for work that will not be run. It is
// intentionally separate from RetryTask: retry creates a new attempt, while
// retirement preserves the current attempt and its explanation in history.
func (s *Service) RetireTask(ctx context.Context, selector, id, state, reason, key string, guard MutationGuard) (Task, error) {
	var out Task
	if state != "cancelled" && state != "abandoned" && state != "superseded" {
		return out, fail("invalid_task_state", "retirement state must be cancelled, abandoned or superseded")
	}
	if strings.TrimSpace(reason) == "" {
		return out, fail("reason_required", "give a reason for retiring the task")
	}
	request := struct {
		Task, State, Reason string
		Guard               MutationGuard
	}{id, state, reason, guard}
	err := mutate(s, ctx, selector, []string{key}, request, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if err := rejectNewWorkspaceWork(d, "retiring tasks"); err != nil {
			return err
		}
		if guard.ExpectedRevision != 0 && d.State.Revision != guard.ExpectedRevision {
			return fail("revision_conflict", "workspace changed while retiring the task")
		}
		t, err := findTask(d, id)
		if err != nil {
			return err
		}
		if retiredTask(*t) {
			if t.State != state || t.Reason != reason {
				return fail("task_retired", "task %s is already %s", t.ID, t.State)
			}
			out = *t
			return nil
		}
		for _, session := range d.Registry.Sessions {
			if session.TaskID == t.ID && session.Active() {
				return fail("task_busy", "stop session %s before retiring task %s", session.ID, t.ID)
			}
		}
		// Keep AcceptedHandoff and the original RunID intact when a result is
		// retired. The task leaves active workflow gates, while its accepted
		// evidence remains addressable as history.
		t.State, t.Reason = state, reason
		out = *t
		return saveDocument(d)
	})
	return out, err
}

func (s *Service) CancelTask(ctx context.Context, selector, id, reason, key string) (Task, error) {
	return s.RetireTask(ctx, selector, id, "cancelled", reason, key, MutationGuard{})
}

func (s *Service) AbandonTask(ctx context.Context, selector, id, reason, key string) (Task, error) {
	return s.RetireTask(ctx, selector, id, "abandoned", reason, key, MutationGuard{})
}

func (s *Service) SupersedeTask(ctx context.Context, selector, id, reason, key string) (Task, error) {
	return s.RetireTask(ctx, selector, id, "superseded", reason, key, MutationGuard{})
}

func (s *Service) RetryTask(ctx context.Context, selector, id, reason, key string) (Task, error) {
	return s.retryTask(ctx, selector, id, reason, key, MutationGuard{})
}

// RetryTaskGuarded includes the visible revision and attempt in the operation
// digest and validates them under the same project lock as the retry.
func (s *Service) RetryTaskGuarded(ctx context.Context, selector, id, reason, key string, guard MutationGuard) (Task, error) {
	return s.retryTask(ctx, selector, id, reason, key, guard)
}

func (s *Service) retryTask(ctx context.Context, selector, id, reason, key string, guard MutationGuard) (Task, error) {
	var out Task
	var request any = struct{ Task, Reason string }{id, reason}
	if !guard.empty() {
		request = struct {
			Task, Reason string
			Guard        MutationGuard
		}{id, reason, guard}
	}
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
		if guard.ExpectedRevision != 0 && d.State.Revision != guard.ExpectedRevision {
			return fail("revision_conflict", "workspace changed while the action was being confirmed")
		}
		t, err := findTask(d, id)
		if err != nil {
			return err
		}
		if t.DeletedAt != nil {
			return fail("task_deleted", "task %s was deleted", t.ID)
		}
		if retiredTask(*t) {
			return fail("task_retired", "task %s is %s; create a replacement task instead of retrying it", t.ID, t.State)
		}
		if guard.ExpectedAttempt != 0 && t.Attempt != guard.ExpectedAttempt {
			return fail("target_changed", "task %s moved from attempt %d to %d", id, guard.ExpectedAttempt, t.Attempt)
		}
		if err := rejectNewWorkspaceWork(d, "retrying tasks"); err != nil {
			return err
		}
		if d.State.Release.UserConfirmed {
			return fail("workspace_completed", "released work requires workspace reopen before retrying tasks")
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
			candidate.RunID = ""
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

type taskBindingMode uint8

const (
	taskClaim taskBindingMode = iota
	taskResume
	taskConsultation
)

// taskBinding makes the provenance decision explicit to the caller that
// appends the concrete Run. A review-safe resume must not replace the Run that
// identifies the pending handoff.
type taskBinding struct {
	preserveRunProvenance bool
}

// bindTask validates a current attempt, then records its logical Session. The
// caller records the concrete Run after reserving it. A resume of an idle
// logical Session whose task is awaiting review validates the existing binding
// but deliberately leaves the task state and result provenance unchanged.
func bindTask(ctx context.Context, d *Document, p *Session, taskID string, mode taskBindingMode) (taskBinding, error) {
	var binding taskBinding
	if taskID == "" {
		return binding, nil
	}
	t, err := findTask(d, taskID)
	if err != nil {
		return binding, err
	}
	if t.Role != p.AgentSnapshot.Role {
		return binding, fail("role_mismatch", "task and agent roles differ")
	}
	reviewResume := mode == taskResume && t.State == "awaiting_review"
	consultationResume := mode == taskConsultation && t.State == "accepted"
	if mode == taskConsultation && !consultationResume {
		return binding, fail("conversation_only", "completed workspace Runs may consult only accepted task results")
	}
	if consultationResume {
		if t.SessionID != p.ID || t.WorktreeID != p.WorktreeID || t.Attempt != p.TaskAttempt || t.InputDigest != p.InputDigest || t.RunID == "" {
			return binding, fail("invalid_resume", "accepted task provenance differs from the logical session")
		}
		run, runErr := findRun(d, t.RunID)
		if runErr != nil || run.SessionID != p.ID {
			return binding, fail("invalid_resume", "accepted task provenance is missing or inconsistent")
		}
		input, inputErr := taskInputDigest(d, t)
		if inputErr != nil {
			return binding, inputErr
		}
		if input != t.InputDigest {
			return binding, fail("invalid_resume", "task attempt or input lineage changed; start a new logical session")
		}
		if t.BaseCommit != "" {
			head, headErr := git(ctx, p.CWD, "rev-parse", "HEAD")
			if headErr != nil {
				return binding, headErr
			}
			if _, headErr = git(ctx, p.CWD, "merge-base", "--is-ancestor", t.BaseCommit, head); headErr != nil {
				return binding, fail("base_mismatch", "worktree does not contain task base")
			}
		}
		binding.preserveRunProvenance = true
		return binding, nil
	}
	if reviewResume {
		if t.SessionID != p.ID || t.WorktreeID != p.WorktreeID || t.Attempt != p.TaskAttempt || t.InputDigest != p.InputDigest || t.RunID == "" {
			return binding, fail("invalid_resume", "task binding differs from the logical session")
		}
		run, runErr := findRun(d, t.RunID)
		if runErr != nil || run.SessionID != p.ID {
			return binding, fail("invalid_resume", "pending review provenance is missing or inconsistent")
		}
		if err := taskDependenciesReady(d, t); err != nil {
			return binding, err
		}
		binding.preserveRunProvenance = true
	} else if err := taskReady(d, t); err != nil {
		return binding, err
	}
	if t.BaseCommit != "" {
		head, err := git(ctx, p.CWD, "rev-parse", "HEAD")
		if err != nil {
			return binding, err
		}
		if _, err := git(ctx, p.CWD, "merge-base", "--is-ancestor", t.BaseCommit, head); err != nil {
			return binding, fail("base_mismatch", "worktree does not contain task base")
		}
	}
	for _, depID := range t.DependsOn {
		dep, _ := findTask(d, depID)
		if dep.Role == "planner" || t.Role == "integrator" {
			continue
		}
		h, err := findHandoff(d, dep.AcceptedHandoff)
		if err != nil {
			return binding, err
		}
		if _, err := git(ctx, p.CWD, "merge-base", "--is-ancestor", h.HeadCommit, "HEAD"); err != nil {
			head, _ := git(ctx, p.CWD, "rev-parse", "HEAD")
			return binding, fail("base_mismatch", "worktree at %s is missing accepted dependency %s head %s", head, depID, h.HeadCommit)
		}
	}
	input, err := taskInputDigest(d, t)
	if err != nil {
		return binding, err
	}
	if reviewResume {
		if input != t.InputDigest {
			return binding, fail("invalid_resume", "task attempt or input lineage changed; start a new logical session")
		}
		return binding, nil
	}
	p.TaskID = t.ID
	p.TaskAttempt = t.Attempt
	p.InputDigest = input
	t.InputDigest = input
	t.SessionID = p.ID
	t.WorktreeID = p.WorktreeID
	t.State = "running"
	return binding, nil
}
