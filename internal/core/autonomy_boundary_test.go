package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAutonomyBoundaryExcludedForAgentWhileRunning is the table-driven safety
// boundary check: every operation with external, lifecycle-terminal or
// authorization effects returns autonomy_excluded for an agent actor while the
// run is running, even with an attestation, while the terminal user is not
// excluded.
func TestAutonomyBoundaryExcludedForAgentWhileRunning(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	task := plannedTask(t, s, ws, "boundary-task", "planner", nil)
	p, _ := startTask(t, s, ws, task)
	revision := currentRevision(t, s, ws)

	cases := []struct {
		name string
		call func(svc *Service) error
	}{
		{"integration land", func(svc *Service) error {
			_, err := svc.LandIntegration(ctx, ws, LandingOptions{ExpectedRevision: revision, UserConfirmed: true})
			return err
		}},
		{"complete", func(svc *Service) error {
			_, err := svc.CompleteWorkspace(ctx, ws, CompleteOptions{UserConfirmed: true, ExpectedRevision: revision})
			return err
		}},
		{"decision answer", func(svc *Service) error {
			_, err := svc.AnswerDecision(ctx, ws, DecisionAnswer{ID: "decision_x", Answer: "run", UserConfirmed: true})
			return err
		}},
		{"release confirm", func(svc *Service) error {
			_, err := svc.ConfirmRelease(ctx, ws, "release-ref", true)
			return err
		}},
		{"change-request publish", func(svc *Service) error {
			_, err := svc.PublishChangeRequest(ctx, ws, "cr_x", true)
			return err
		}},
		{"change-request resolve", func(svc *Service) error {
			_, err := svc.ResolveChangeRequest(ctx, ws, "cr_x", "skip", "", "reason", true)
			return err
		}},
		{"reopen", func(svc *Service) error {
			_, err := svc.ReopenWorkspace(ctx, ws, ReopenOptions{Reason: "r", ExpectedRevision: revision, UserConfirmed: true}, "boundary-reopen")
			return err
		}},
		{"archive", func(svc *Service) error {
			_, err := svc.ArchiveGuarded(ctx, ws, "boundary-archive", revision)
			return err
		}},
		{"clean", func(svc *Service) error {
			_, err := svc.Clean(ctx, ws, false, false)
			return err
		}},
		{"state edit", func(svc *Service) error {
			_, err := svc.EditState(ctx, ws, revision, nil)
			return err
		}},
		{"delete task", func(svc *Service) error {
			_, err := svc.DeleteTask(ctx, ws, task.ID, "boundary-del-task", MutationGuard{})
			return err
		}},
		{"delete session", func(svc *Service) error {
			_, err := svc.DeleteSession(ctx, ws, p.ID, "boundary-del-session", MutationGuard{})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(agent); err == nil {
				t.Fatalf("agent %s succeeded under autonomy", tc.name)
			} else {
				expectCode(t, err, "autonomy_excluded")
			}
			err := tc.call(s)
			var e *Error
			if errors.As(err, &e) && e.Code == "autonomy_excluded" {
				t.Fatalf("terminal user %s was excluded: %v", tc.name, err)
			}
		})
	}
}

// TestAutonomyPublishExcludedWhenPublicationAllowed proves the autonomous
// boundary takes precedence over a permissive forge publication policy.
func TestAutonomyPublishExcludedWhenPublicationAllowed(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "extended")
	ctx := context.Background()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Forge.Publication = "allowed"
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.PublishChangeRequest(ctx, ws, "cr-x", false); err == nil {
		t.Fatal("agent published a change request under autonomy with publication: allowed")
	} else {
		expectCode(t, err, "autonomy_excluded")
	}
}

// TestAutonomyBoundaryDeleteWorkspaceExcluded covers workspace deletion, which
// needs its own workspace because a successful deletion removes it.
func TestAutonomyBoundaryDeleteWorkspaceExcluded(t *testing.T) {
	s, agent, ws, _ := autonomousOrchestrator(t, "")
	ctx := context.Background()
	revision := currentRevision(t, s, ws)
	if err := agent.DeleteWorkspace(ctx, ws, "boundary-delete-ws", revision); err == nil {
		t.Fatal("agent deleted an autonomous workspace")
	} else {
		expectCode(t, err, "autonomy_excluded")
	}
	err := s.DeleteWorkspace(ctx, ws, "boundary-delete-ws-user", revision)
	var e *Error
	if errors.As(err, &e) && e.Code == "autonomy_excluded" {
		t.Fatalf("terminal user delete was excluded: %v", err)
	}
}
