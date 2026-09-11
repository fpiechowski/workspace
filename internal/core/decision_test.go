package core

import (
	"context"
	"testing"
)

func TestRefreshStaleDecisionPreservesHistory(t *testing.T) {
	s, ws, _ := integratedWorkflow(t)
	ctx := context.Background()
	s.Forge = &fakeForge{}
	cr, err := s.PrepareChangeRequest(ctx, ws, ChangeRequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishChangeRequest(ctx, ws, cr.ID, false); err != nil {
		t.Fatal(err)
	}
	advancePhase(t, s, ws, "live_test_offer")
	menu, err := s.Menu(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	old := *menu.PendingDecision
	if err := s.With(ctx, ws, func(d *Document) error { d.State.ChangeRequests[0].State = "merged"; return saveDocument(d) }); err != nil {
		t.Fatal(err)
	}
	_, err = s.AnswerDecision(ctx, ws, DecisionAnswer{ID: old.ID, ExpectedRevision: old.Revision, Answer: "run", Environment: "local"})
	expectCode(t, err, "decision_stale")
	v, err := s.RefreshDecision(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if v.Workspace.PendingDecision.ID == old.ID || len(v.Workspace.Decisions) != 1 || !v.Workspace.Decisions[0].Superseded {
		t.Fatal("old question history lost")
	}
	current := *v.Workspace.PendingDecision
	if _, err := s.RefreshDecision(ctx, ws); err != nil {
		t.Fatal(err)
	}
	opt := DecisionAnswer{ID: current.ID, ExpectedRevision: current.Revision, Answer: "run", Environment: "local"}
	if _, err := s.AnswerDecision(ctx, ws, opt); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AnswerDecision(ctx, ws, opt); err != nil {
		t.Fatal("same answer retry failed", err)
	}
	opt.Environment = "production"
	_, err = s.AnswerDecision(ctx, ws, opt)
	expectCode(t, err, "decision_not_found")
}
