package core

// Actor is an explicit local role contract. An empty actor represents a user at
// the terminal. It is not a sandbox or a cryptographic proof of human identity.
type Actor struct{ AgentID, SessionID string }

func (s *Service) actor(d *Document) (*Session, error) {
	if s.Actor.AgentID == "" && s.Actor.SessionID == "" {
		return nil, nil
	}
	p, err := findSession(d, s.Actor.SessionID)
	if err != nil {
		return nil, fail("stale_actor", "unknown actor session")
	}
	if p.AgentID != s.Actor.AgentID || !p.Active() {
		return nil, fail("stale_actor", "actor is not the active owner of this session")
	}
	return p, nil
}
func (s *Service) requireOrchestrator(d *Document) error {
	p, err := s.actor(d)
	if err != nil {
		return err
	}
	if p != nil && p.AgentID != d.State.OrchestratorAgentID {
		return fail("forbidden", "only the orchestrator or user can perform this operation")
	}
	return nil
}
func (s *Service) requireUser(d *Document) error {
	if s.Actor.AgentID != "" || s.Actor.SessionID != "" {
		return fail("user_decision_required", "this operation records an explicit user decision; run it from the user terminal")
	}
	return nil
}

func (s *Service) requireSessionOwner(d *Document, id string) error {
	p, err := s.actor(d)
	if err != nil {
		return err
	}
	target, err := findSession(d, id)
	if err != nil {
		return err
	}
	if p != nil && p.ID != target.ID && p.AgentID != d.State.OrchestratorAgentID {
		return fail("forbidden", "operation requires the session owner or orchestrator")
	}
	return nil
}
