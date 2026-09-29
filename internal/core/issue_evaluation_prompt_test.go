package core

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestCompletedWorkspaceEvaluationPromptNotice is the T2 contract: a
// conversation-only orchestrator Run started for a completed linked Workspace
// with a pending evaluation is told the Issue ID, the frozen snapshot path and
// the exact narrow record command with its ieval operation key.
func TestCompletedWorkspaceEvaluationPromptNotice(t *testing.T) {
	s, _ := fixture(t)
	ws, issue := linkedManualWorkspace(t, s, "")
	completed := completeLinkedWorkspace(t, s, ws, "")
	eval := completed.Workspace.IssueEvaluation
	if eval == nil || eval.State != "pending" {
		t.Fatalf("completion did not leave a pending evaluation: %+v", eval)
	}

	prompt := orchestratorPrompt(t, s, ws, "eval-prompt-start")
	for _, want := range []string{
		"Issue evaluation notice",
		issue.ID,
		filepath.Join(completed.Directory, "inputs", "issue.md"),
		"inputs/issue.md",
		"issue-evaluation record",
		"--outcome delivered|not_delivered",
		"--operation-key issue-evaluation:" + eval.ID,
		"untrusted",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("evaluation prompt missing %q:\n%s", want, prompt)
		}
	}
	// The evaluation notice extends rather than replaces the conversation notice.
	if !strings.Contains(prompt, "This Run is conversation-only") {
		t.Fatalf("evaluation prompt lost the conversation-only notice:\n%s", prompt)
	}
}

// TestCompletedWorkspaceWithoutPendingEvaluationPromptUnchanged verifies the
// notice is gated on the pending state: once the linked Issue has no pending
// evaluation (recorded) or the Workspace is unlinked, the prompt carries no
// evaluation notice and matches the pre-existing conversation-only text.
func TestCompletedWorkspaceWithoutPendingEvaluationPromptUnchanged(t *testing.T) {
	t.Run("linked but recorded", func(t *testing.T) {
		s, _ := fixture(t)
		ws, _ := linkedManualWorkspace(t, s, "")
		completeLinkedWorkspace(t, s, ws, "")
		if err := s.With(context.Background(), ws, func(d *Document) error {
			d.State.IssueEvaluation.State = "recorded"
			return saveDocument(d)
		}); err != nil {
			t.Fatal(err)
		}
		prompt := orchestratorPrompt(t, s, ws, "eval-recorded-start")
		if strings.Contains(prompt, "Issue evaluation notice") {
			t.Fatalf("recorded evaluation leaked the notice:\n%s", prompt)
		}
		if !strings.Contains(prompt, "This Run is conversation-only") {
			t.Fatalf("conversation-only notice missing:\n%s", prompt)
		}
	})

	t.Run("unlinked", func(t *testing.T) {
		s, ws := manualFixture(t)
		if _, err := s.CompleteWorkspace(context.Background(), ws, CompleteOptions{Reason: "finished", ExpectedRevision: currentRevision(t, s, ws)}); err != nil {
			t.Fatal(err)
		}
		prompt := orchestratorPrompt(t, s, ws, "eval-unlinked-start")
		if strings.Contains(prompt, "Issue evaluation notice") {
			t.Fatalf("unlinked workspace leaked the notice:\n%s", prompt)
		}
	})
}
