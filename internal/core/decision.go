package core

import "context"

func (s *Service) RefreshDecision(ctx context.Context, selector string, keys ...string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, keys, "decision.refresh", &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if d.State.Workflow == nil || d.State.Workflow.Phase != "live_test_offer" || d.State.Status != "active" {
			return fail("workflow_gate", "refresh is available for an active live-test offer")
		}
		if err := validateIntegration(ctx, d); err != nil {
			return err
		}
		if old := d.State.PendingDecision; old != nil {
			if old.BasisDigest == decisionBasis(d) {
				out = d.Status()
				return nil
			}
			old.Superseded = true
			d.State.Decisions = append(d.State.Decisions, *old)
			d.State.PendingDecision = nil
		}
		if _, err := requestDecision(d, "live-testing", "Run live testing against the integrated revision?", []string{"run", "skip"}); err != nil {
			return err
		}
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}
