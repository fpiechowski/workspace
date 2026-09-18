package core

import (
	"context"
	"errors"
	"strings"
	"time"
)

// MessageOptions accepts the historical agent selector for compatibility, but
// new messages are always persisted with an exact logical Session target.
type MessageOptions struct {
	To, ToSession, Kind, Body, ReplyTo, OperationKey string
}

func findMessage(d *Document, id string) (*Message, error) {
	for i := range d.Registry.Messages {
		if d.Registry.Messages[i].ID == id {
			return &d.Registry.Messages[i], nil
		}
	}
	return nil, fail("message_not_found", "unknown message %q", id)
}

// addMessage retains the small legacy helper used by old callers. All current
// message-producing paths use addTargetedMessage below.
func addMessage(d *Document, from, session, to, kind, body, handoff, reply string) Message {
	return addTargetedMessage(d, from, session, to, "", kind, body, handoff, reply)
}

func addTargetedMessage(d *Document, from, session, toAgent, toSession, kind, body, handoff, reply string) Message {
	m := Message{ID: ID("msg"), FromAgent: from, FromSession: session, ToAgent: toAgent, ToSession: toSession, Kind: kind, Body: body, HandoffID: handoff, ReplyTo: reply, CreatedAt: time.Now().UTC()}
	if p, err := findSession(d, session); err == nil {
		m.FromSession = p.ID
		m.FromRun = p.CurrentRunID
	}
	d.Registry.Messages = append(d.Registry.Messages, m)
	return m
}

// openSessionCandidates returns logical Sessions which can still receive a
// newly-created message. Closed and deleted Sessions remain in history but can
// never be selected by the compatibility agent resolver.
func openSessionCandidates(d *Document, agentID string) []Session {
	var out []Session
	for _, session := range d.Registry.Sessions {
		if session.AgentID == agentID && session.DeletedAt == nil && session.ClosedAt == nil {
			out = append(out, session)
		}
	}
	return out
}

// resolveTargetSession turns an explicit Session address into the canonical
// delivery address. An agent-only address is accepted only when one open
// logical Session can receive it; in particular, it never means "latest".
func resolveTargetSession(d *Document, toAgent, toSession string) (Session, error) {
	toAgent = strings.TrimSpace(toAgent)
	toSession = strings.TrimSpace(toSession)
	if toSession != "" {
		target, err := findSession(d, toSession)
		if err != nil {
			return Session{}, err
		}
		if target.DeletedAt != nil {
			return Session{}, fail("session_deleted", "target Session %s was deleted", target.ID)
		}
		if target.ClosedAt != nil {
			return Session{}, fail("session_closed", "target Session %s is closed", target.ID)
		}
		if toAgent != "" {
			a, err := findAgent(d, toAgent)
			if err != nil {
				return Session{}, err
			}
			if a.ID != target.AgentID {
				return Session{}, fail("invalid_recipient", "agent %s does not own target Session %s", a.ID, target.ID)
			}
		}
		return *target, nil
	}
	if toAgent == "" {
		return Session{}, fail("session_required", "provide an exact target Session with --to-session")
	}
	a, err := findAgent(d, toAgent)
	if err != nil {
		return Session{}, err
	}
	candidates := openSessionCandidates(d, a.ID)
	switch len(candidates) {
	case 0:
		return Session{}, fail("session_required", "agent %s has no open logical Session; provide --to-session after starting one", a.ID)
	case 1:
		return candidates[0], nil
	default:
		return Session{}, fail("ambiguous_recipient", "agent %s has multiple eligible Sessions; provide --to-session", a.ID)
	}
}

// messageBelongsToSession is the authorization predicate for an agent's
// current inbox. Session-addressed messages are exact. Legacy agent-addressed
// messages are readable by an agent only with strong delivery evidence or when
// the agent still has one unambiguous open logical Session.
func messageBelongsToSession(d *Document, message Message, session Session) bool {
	if message.ToSession != "" {
		return message.ToSession == session.ID && message.ToAgent == session.AgentID
	}
	if message.ToAgent != session.AgentID {
		return false
	}
	if message.DeliveredSessionID != "" || message.NotifiedSessionID != "" {
		return message.DeliveredSessionID == session.ID || message.NotifiedSessionID == session.ID
	}
	candidates := openSessionCandidates(d, session.AgentID)
	return len(candidates) == 1 && candidates[0].ID == session.ID
}

// messageVisibleInSession is slightly more permissive for a user inspecting a
// closed historical Session: an existing delivery/notification reference is
// enough to associate a legacy message with that Session.
func messageVisibleInSession(d *Document, message Message, session Session) bool {
	if message.ToSession != "" {
		return message.ToSession == session.ID && message.ToAgent == session.AgentID
	}
	if message.ToAgent != session.AgentID {
		return false
	}
	if message.DeliveredSessionID != "" || message.NotifiedSessionID != "" {
		return message.DeliveredSessionID == session.ID || message.NotifiedSessionID == session.ID
	}
	candidates := openSessionCandidates(d, session.AgentID)
	return len(candidates) == 1 && candidates[0].ID == session.ID
}

// messageAddressMatchesRun is used by transport receipts. A legacy message
// remains agent-addressed, while every new message must match the exact
// logical Session owning the concrete Run.
func messageAddressMatchesRun(message Message, session Session, run Run) bool {
	if run.SessionID != session.ID {
		return false
	}
	if message.ToSession != "" {
		return message.ToSession == session.ID && message.ToAgent == session.AgentID
	}
	return message.ToAgent == session.AgentID
}

type inboxScope struct {
	agentID   string
	sessionID string
	agentWide bool
}

func recipientMatchesSession(d *Document, recipient string, target Session) (bool, error) {
	if recipient == "" {
		return true, nil
	}
	if session, err := findSession(d, recipient); err == nil {
		return session.ID == target.ID, nil
	}
	agent, err := findAgent(d, recipient)
	if err != nil {
		return false, err
	}
	return agent.ID == target.AgentID, nil
}

// resolveInboxScope implements the user-facing inbox contract. Agent
// processes always get their exact current Session. A user may name a Session,
// or explicitly request the agent-wide historical view with --agent.
func (s *Service) resolveInboxScope(d *Document, recipient, sessionID string) (inboxScope, error) {
	actor, err := s.actor(d)
	if err != nil {
		return inboxScope{}, err
	}
	if actor != nil {
		if sessionID != "" {
			target, targetErr := findSession(d, sessionID)
			if targetErr != nil {
				return inboxScope{}, targetErr
			}
			if target.ID != actor.ID {
				return inboxScope{}, fail("forbidden", "an agent can only inspect its exact current Session")
			}
			matches, matchErr := recipientMatchesSession(d, recipient, *target)
			if matchErr != nil {
				return inboxScope{}, matchErr
			}
			if !matches {
				return inboxScope{}, fail("forbidden", "recipient does not match the exact target Session")
			}
		} else if recipient != "" {
			matches, matchErr := recipientMatchesSession(d, recipient, *actor)
			if matchErr != nil {
				return inboxScope{}, matchErr
			}
			if !matches {
				return inboxScope{}, fail("forbidden", "an agent can only inspect its exact current Session")
			}
		}
		return inboxScope{agentID: actor.AgentID, sessionID: actor.ID}, nil
	}

	if sessionID != "" {
		target, err := findSession(d, sessionID)
		if err != nil {
			return inboxScope{}, err
		}
		matches, matchErr := recipientMatchesSession(d, recipient, *target)
		if matchErr != nil {
			return inboxScope{}, matchErr
		}
		if !matches {
			return inboxScope{}, fail("invalid_recipient", "recipient does not match target Session %s", target.ID)
		}
		return inboxScope{agentID: target.AgentID, sessionID: target.ID}, nil
	}
	if recipient != "" {
		if target, err := findSession(d, recipient); err == nil {
			return inboxScope{agentID: target.AgentID, sessionID: target.ID}, nil
		}
		a, err := findAgent(d, recipient)
		if err != nil {
			return inboxScope{}, err
		}
		return inboxScope{agentID: a.ID, agentWide: true}, nil
	}

	// The user terminal defaults to the exact current orchestrator Session when
	// one exists. With no current orchestrator there is no implicit historical
	// agent-wide view; use --agent explicitly for that view.
	candidates := openSessionCandidates(d, d.State.OrchestratorAgentID)
	if len(candidates) > 1 {
		return inboxScope{}, fail("ambiguous_recipient", "the orchestrator has multiple eligible Sessions; provide --session or --agent")
	}
	if len(candidates) == 1 {
		return inboxScope{agentID: candidates[0].AgentID, sessionID: candidates[0].ID}, nil
	}
	return inboxScope{agentID: d.State.OrchestratorAgentID}, nil
}

func messagesForScope(d *Document, scope inboxScope, all bool) []Message {
	messages := make([]Message, 0)
	var target *Session
	if scope.sessionID != "" {
		target, _ = findSession(d, scope.sessionID)
	}
	for _, message := range d.Registry.Messages {
		if !all && message.AcknowledgedAt != nil {
			continue
		}
		visible := false
		if scope.agentWide {
			visible = message.ToAgent == scope.agentID
		} else if target != nil {
			visible = messageVisibleInSession(d, message, *target)
		}
		if visible {
			messages = append(messages, message)
		}
	}
	return messages
}

func (s *Service) SendMessage(ctx context.Context, selector string, opt MessageOptions) (Message, error) {
	var out Message
	if strings.TrimSpace(opt.Body) == "" || len(opt.Body) > 1024*1024 {
		return out, fail("invalid_message", "body must contain 1–1048576 bytes")
	}
	if opt.Kind == "" {
		opt.Kind = "note"
	}
	if opt.Kind != "note" && opt.Kind != "question" && opt.Kind != "answer" && opt.Kind != "status" {
		return out, fail("invalid_message", "kind must be note, question, answer or status")
	}
	err := s.With(ctx, selector, func(d *Document) error {
		p, err := s.actor(d)
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
			m, err := findMessage(d, id)
			if err != nil {
				return err
			}
			out = *m
			return nil
		}

		from, fromSession := "user", ""
		if p != nil {
			from, fromSession = p.AgentID, p.ID
		}
		toAgent, toSession := strings.TrimSpace(opt.To), strings.TrimSpace(opt.ToSession)
		if opt.ReplyTo != "" {
			previous, err := findMessage(d, opt.ReplyTo)
			if err != nil {
				return err
			}
			if p != nil && !messageBelongsToSession(d, *previous, *p) {
				return fail("forbidden", "cannot reply from a different target Session")
			}
			if previous.FromSession != "" {
				senderAgentID := previous.FromAgent
				if sender, senderErr := findAgent(d, previous.FromAgent); senderErr == nil {
					senderAgentID = sender.ID
				}
				if toSession != "" && toSession != previous.FromSession {
					return fail("invalid_reply", "reply must target the original sender Session")
				}
				if toAgent != "" {
					recipient, recipientErr := findAgent(d, toAgent)
					if recipientErr != nil {
						return recipientErr
					}
					if recipient.ID != senderAgentID {
						return fail("invalid_reply", "reply recipient is not the original sender")
					}
				}
				toSession = previous.FromSession
				toAgent = senderAgentID
			} else if toAgent == "" && toSession == "" {
				return fail("session_required", "the original message has no Session sender; provide --to-session")
			}
		}
		target, err := resolveTargetSession(d, toAgent, toSession)
		legacyTarget := false
		var targetError *Error
		legacyMissingSession := errors.As(err, &targetError) && targetError.Code == "session_required"
		legacyRecipient := ""
		if recipientAgent, agentErr := findAgent(d, toAgent); agentErr == nil {
			legacyRecipient = recipientAgent.ID
		}
		if legacyMissingSession && p != nil && opt.ToSession == "" && p.ParentSessionID == "" &&
			(legacyRecipient == p.ParentAgentID || legacyRecipient == d.State.OrchestratorAgentID) {
			// Sessions created before exact parent lineage was available can still
			// emit an agent-addressed legacy record. Explicit --to resolution above
			// remains strict whenever an eligible Session exists or is ambiguous.
			legacyAgent := p.ParentAgentID
			if legacyAgent == "" {
				legacyAgent = d.State.OrchestratorAgentID
			}
			a, agentErr := findAgent(d, legacyAgent)
			if agentErr != nil {
				return err
			}
			target = Session{AgentID: a.ID}
			legacyTarget = true
			err = nil
		}
		if err != nil {
			return err
		}
		if opt.ReplyTo != "" {
			previous, _ := findMessage(d, opt.ReplyTo)
			if previous.FromAgent != target.AgentID || previous.FromSession != target.ID {
				return fail("invalid_reply", "reply recipient is not the original sender Session")
			}
		}
		if legacyTarget {
			out = addMessage(d, from, fromSession, target.AgentID, opt.Kind, opt.Body, "", opt.ReplyTo)
		} else {
			out = addTargetedMessage(d, from, fromSession, target.AgentID, target.ID, opt.Kind, opt.Body, "", opt.ReplyTo)
		}
		d.remember(opt.OperationKey, opt, out.ID)
		return saveResource(d, opt.OperationKey, out)
	})
	return out, err
}

// Inbox is retained as the compatibility API. A recipient that names an Agent
// from the user terminal requests the explicit agent-wide historical view.
func (s *Service) Inbox(ctx context.Context, selector, recipient string, all bool) ([]Message, error) {
	return s.InboxScoped(ctx, selector, recipient, "", all)
}

func (s *Service) InboxForSession(ctx context.Context, selector, sessionID string, all bool) ([]Message, error) {
	return s.InboxScoped(ctx, selector, "", sessionID, all)
}

func (s *Service) InboxForAgent(ctx context.Context, selector, agentID string, all bool) ([]Message, error) {
	return s.InboxScoped(ctx, selector, agentID, "", all)
}

func (s *Service) InboxScoped(ctx context.Context, selector, recipient, sessionID string, all bool) ([]Message, error) {
	out := []Message{}
	err := s.With(ctx, selector, func(d *Document) error {
		scope, err := s.resolveInboxScope(d, recipient, sessionID)
		if err != nil {
			return err
		}
		out = messagesForScope(d, scope, all)
		return nil
	})
	return out, err
}

func (s *Service) readMessage(ctx context.Context, selector, id, targetSession string, ack bool, keys ...string) (Message, error) {
	var out Message
	if !ack {
		keys = nil
	}
	err := mutate(s, ctx, selector, keys, []any{"inbox.ack", id, targetSession}, &out, func(d *Document) error {
		m, err := findMessage(d, id)
		if err != nil {
			return err
		}
		return s.authorizeMessageSession(d, *m, targetSession)
	}, func(d *Document) error {
		m, err := findMessage(d, id)
		if err != nil {
			return err
		}
		if err := s.authorizeMessageSession(d, *m, targetSession); err != nil {
			return err
		}
		if ack && m.AcknowledgedAt == nil {
			now := time.Now().UTC()
			m.AcknowledgedAt = &now
		}
		out = *m
		if ack {
			return saveDocument(d)
		}
		return nil
	})
	return out, err
}

func (s *Service) authorizeMessageSession(d *Document, message Message, targetSession string) error {
	actor, err := s.actor(d)
	if err != nil {
		return err
	}
	if targetSession != "" {
		target, err := findSession(d, targetSession)
		if err != nil {
			return err
		}
		if actor != nil && actor.ID != target.ID {
			return fail("forbidden", "an agent can only acknowledge its exact current Session")
		}
		if !messageVisibleInSession(d, message, *target) {
			return fail("forbidden", "message does not belong to target Session %s", target.ID)
		}
		return nil
	}
	if actor == nil {
		return nil
	}
	if !messageBelongsToSession(d, message, *actor) {
		return fail("forbidden", "message does not belong to the agent's exact current Session")
	}
	return nil
}

func (s *Service) ReadMessage(ctx context.Context, selector, id string, ack bool, keys ...string) (Message, error) {
	return s.readMessage(ctx, selector, id, "", ack, keys...)
}

func (s *Service) ReadMessageForSession(ctx context.Context, selector, id, sessionID string, ack bool, keys ...string) (Message, error) {
	return s.readMessage(ctx, selector, id, sessionID, ack, keys...)
}

func (s *Service) WaitInbox(ctx context.Context, selector, recipient string, timeout time.Duration) ([]Message, error) {
	return s.WaitInboxScoped(ctx, selector, recipient, "", timeout)
}

func (s *Service) WaitInboxForSession(ctx context.Context, selector, sessionID string, timeout time.Duration) ([]Message, error) {
	return s.WaitInboxScoped(ctx, selector, "", sessionID, timeout)
}

func (s *Service) WaitInboxScoped(ctx context.Context, selector, recipient, sessionID string, timeout time.Duration) ([]Message, error) {
	if timeout <= 0 || timeout > time.Hour {
		return nil, fail("invalid_timeout", "timeout must be between 1 second and 1 hour")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		messages, err := s.InboxScoped(ctx, selector, recipient, sessionID, false)
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return []Message{}, nil
			}
			return nil, err
		}
		if len(messages) > 0 {
			return messages, nil
		}
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return []Message{}, nil
			}
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
