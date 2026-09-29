package core

import (
	"context"
	"strings"
	"testing"
)

// failLaunchRuntime forces the orchestrator pane launch to fail so the
// best-effort completion path can be exercised.
type failLaunchRuntime struct{ *fakeRuntime }

func (r *failLaunchRuntime) Launch(context.Context, Launch) (Pane, error) {
	return Pane{}, fail("launch_unavailable", "forced launch failure")
}

// activeEvaluationRuns returns the live conversation-only orchestrator Runs of a
// completed Workspace, which is exactly the set the automatic launch creates.
func activeEvaluationRuns(t *testing.T, s *Service, ws string) []Run {
	t.Helper()
	status, err := s.Status(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	var runs []Run
	for _, session := range status.Sessions {
		if !session.Active() || session.AgentSnapshot.Role != "orchestrator" {
			continue
		}
		for _, run := range status.Runs {
			if run.ID == session.CurrentRunID {
				runs = append(runs, run)
			}
		}
	}
	return runs
}

// Completing a linked Workspace starts exactly one conversation-only
// orchestrator Run. Replaying the same completion operation key does not start
// a second Run.
func TestCompleteLinkedWorkspaceLaunchesEvaluationOrchestrator(t *testing.T) {
	ctx := context.Background()
	s, _ := fixture(t)
	ws, _ := linkedManualWorkspace(t, s, "")
	opt := CompleteOptions{Reason: "delivered", ExpectedRevision: currentRevision(t, s, ws)}

	completed, err := s.CompleteWorkspace(ctx, ws, opt, "complete-eval-launch")
	if err != nil {
		t.Fatalf("completion failed: %v", err)
	}
	if completed.Workspace.Status != "completed" {
		t.Fatalf("completion status: %s", completed.Workspace.Status)
	}
	if eval := completed.Workspace.IssueEvaluation; eval == nil || eval.State != "pending" || eval.LaunchError != "" {
		t.Fatalf("completion evaluation: %+v", eval)
	}
	runs := activeEvaluationRuns(t, s, ws)
	if len(runs) != 1 || !runs[0].ConversationOnly {
		t.Fatalf("evaluation launch created %d conversation-only Runs: %+v", len(runs), runs)
	}

	replay, err := s.CompleteWorkspace(ctx, ws, opt, "complete-eval-launch")
	if err != nil {
		t.Fatalf("completion replay failed: %v", err)
	}
	if replay.Workspace.Status != "completed" {
		t.Fatalf("completion replay status: %s", replay.Workspace.Status)
	}
	if runs := activeEvaluationRuns(t, s, ws); len(runs) != 1 {
		t.Fatalf("completion replay started another Run: %+v", runs)
	}
}

// A launch failure never rolls back the committed completion: the evaluation
// stays pending with a recorded launch_error, and the menu offers both the
// evaluation action and a conversation start.
func TestCompleteLinkedWorkspaceLaunchFailureRecordsError(t *testing.T) {
	ctx := context.Background()
	s, _ := fixture(t)
	s.Runtime = &failLaunchRuntime{fakeRuntime: s.Runtime.(*fakeRuntime)}
	ws, _ := linkedManualWorkspace(t, s, "")

	completed, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{Reason: "delivered", ExpectedRevision: currentRevision(t, s, ws)})
	if err != nil {
		t.Fatalf("completion with a failing launch must succeed: %v", err)
	}
	if completed.Workspace.Status != "completed" {
		t.Fatalf("completion status: %s", completed.Workspace.Status)
	}
	eval := completed.Workspace.IssueEvaluation
	if eval == nil || eval.State != "pending" || strings.TrimSpace(eval.LaunchError) == "" {
		t.Fatalf("launch failure was not recorded: %+v", eval)
	}
	if runs := activeEvaluationRuns(t, s, ws); len(runs) != 0 {
		t.Fatalf("failing launch left active Runs: %+v", runs)
	}

	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if !menuHas(menu, "issue_evaluation") {
		t.Fatalf("menu lacks the issue_evaluation action: %+v", menu.Actions)
	}
	var conversation *MenuAction
	for i := range menu.Actions {
		if menu.Actions[i].ID == "conversation" {
			conversation = &menu.Actions[i]
		}
	}
	if conversation == nil || !strings.Contains(conversation.Label, "evaluate linked Issue") {
		t.Fatalf("menu did not surface the launch failure: %+v", menu.Actions)
	}
}

// --no-issue-evaluation-start skips the launch and leaves the evaluation
// pending for a later workspace start.
func TestCompleteLinkedWorkspaceNoEvaluationLaunch(t *testing.T) {
	ctx := context.Background()
	s, _ := fixture(t)
	ws, _ := linkedManualWorkspace(t, s, "")

	completed, err := s.CompleteWorkspace(ctx, ws, CompleteOptions{Reason: "later", ExpectedRevision: currentRevision(t, s, ws), NoEvaluationLaunch: true})
	if err != nil {
		t.Fatalf("completion failed: %v", err)
	}
	eval := completed.Workspace.IssueEvaluation
	if eval == nil || eval.State != "pending" || eval.LaunchError != "" {
		t.Fatalf("suppressed launch evaluation: %+v", eval)
	}
	if runs := activeEvaluationRuns(t, s, ws); len(runs) != 0 {
		t.Fatalf("suppressed launch created Runs: %+v", runs)
	}

	if _, err := s.StartOrchestrator(ctx, ws, "later-evaluation"); err != nil {
		t.Fatalf("workspace start did not retry the evaluation: %v", err)
	}
	if runs := activeEvaluationRuns(t, s, ws); len(runs) != 1 {
		t.Fatalf("workspace start did not start the evaluation Run: %+v", runs)
	}
}
