package core

import (
	"context"
	"strings"
	"time"
)

type MessageOptions struct{ To, Kind, Body, ReplyTo, OperationKey string }

func findMessage(d *Document, id string) (*Message, error) {
	for i := range d.Registry.Messages {
		if d.Registry.Messages[i].ID == id {
			return &d.Registry.Messages[i], nil
		}
	}
	return nil, fail("message_not_found", "unknown message %q", id)
}
func addMessage(d *Document, from, session, to, kind, body, handoff, reply string) Message {
	m := Message{ID: ID("msg"), FromAgent: from, FromSession: session, ToAgent: to, Kind: kind, Body: body, HandoffID: handoff, ReplyTo: reply, CreatedAt: time.Now().UTC()}
	d.Registry.Messages = append(d.Registry.Messages, m)
	return m
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
		to, err := findAgent(d, opt.To)
		if err != nil {
			return err
		}
		from, session := "user", ""
		if p != nil {
			from = p.AgentID
			session = p.ID
		}
		if opt.ReplyTo != "" {
			previous, err := findMessage(d, opt.ReplyTo)
			if err != nil {
				return err
			}
			if previous.FromAgent != to.ID {
				return fail("invalid_reply", "reply recipient is not the original sender")
			}
			if p != nil && previous.ToAgent != from {
				return fail("forbidden", "cannot reply to another agent's inbox")
			}
		}
		out = addMessage(d, from, session, to.ID, opt.Kind, opt.Body, "", opt.ReplyTo)
		d.remember(opt.OperationKey, opt, out.ID)
		return saveResource(d, opt.OperationKey, out)
	})
	return out, err
}
func (s *Service) inboxRecipient(d *Document, recipient string) (string, error) {
	p, err := s.actor(d)
	if err != nil {
		return "", err
	}
	if recipient == "" {
		if p != nil {
			recipient = p.AgentID
		} else {
			recipient = d.State.OrchestratorAgentID
		}
	}
	a, err := findAgent(d, recipient)
	if err != nil {
		return "", err
	}
	if p != nil && p.AgentID != a.ID {
		return "", fail("forbidden", "an agent can only read its own inbox")
	}
	return a.ID, nil
}
func (s *Service) Inbox(ctx context.Context, selector, recipient string, all bool) ([]Message, error) {
	out := []Message{}
	err := s.With(ctx, selector, func(d *Document) error {
		id, err := s.inboxRecipient(d, recipient)
		if err != nil {
			return err
		}
		for _, m := range d.Registry.Messages {
			if m.ToAgent == id && (all || m.AcknowledgedAt == nil) {
				out = append(out, m)
			}
		}
		return nil
	})
	return out, err
}
func (s *Service) ReadMessage(ctx context.Context, selector, id string, ack bool, keys ...string) (Message, error) {
	var out Message
	if !ack {
		keys = nil
	}
	err := mutate(s, ctx, selector, keys, []any{"inbox.ack", id}, &out, func(d *Document) error {
		m, err := findMessage(d, id)
		if err != nil {
			return err
		}
		_, err = s.inboxRecipient(d, m.ToAgent)
		return err
	}, func(d *Document) error {
		m, err := findMessage(d, id)
		if err != nil {
			return err
		}
		if _, err := s.inboxRecipient(d, m.ToAgent); err != nil {
			return err
		}
		if ack && m.AcknowledgedAt == nil {
			now := time.Now().UTC()
			m.AcknowledgedAt = &now
			out = *m
			return saveDocument(d)
		}
		out = *m
		return nil
	})
	return out, err
}
func (s *Service) WaitInbox(ctx context.Context, selector, recipient string, timeout time.Duration) ([]Message, error) {
	if timeout <= 0 || timeout > time.Hour {
		return nil, fail("invalid_timeout", "timeout must be between 1 second and 1 hour")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		messages, err := s.Inbox(ctx, selector, recipient, false)
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
