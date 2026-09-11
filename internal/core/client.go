package core

import (
	"context"
	"strings"
)

func normalizeClient(client Client) (Client, error) {
	switch client.Adapter {
	case "codex":
		if len(client.LaunchArgv) == 0 {
			client.LaunchArgv = []string{"codex", "app-server", "--stdio"}
		}
		client.Capabilities = []string{"launch", "resume", "deliver", "observe", "interrupt"}
	case "claude":
		if len(client.LaunchArgv) == 0 {
			client.LaunchArgv = []string{"claude", "--model", "{model}", "{prompt}"}
		}
		if len(client.ResumeArgv) == 0 {
			client.ResumeArgv = []string{"claude", "--resume", "{thread_id}", "--model", "{model}", "{prompt}"}
		}
	case "opencode":
		if len(client.LaunchArgv) == 0 {
			client.LaunchArgv = []string{"opencode", "--model", "{model}", "--prompt", "{prompt}"}
		}
		if len(client.ResumeArgv) == 0 {
			client.ResumeArgv = []string{"opencode", "--session", "{thread_id}", "--model", "{model}", "--prompt", "{prompt}"}
		}
	case "command":
	default:
		return client, fail("invalid_config", "unknown client adapter %q", client.Adapter)
	}
	if len(client.LaunchArgv) == 0 || client.LaunchArgv[0] == "" {
		return client, fail("invalid_config", "client needs launch_argv")
	}
	if client.Adapter != "codex" {
		args := strings.Join(client.LaunchArgv, "\n")
		if !strings.Contains(args, "{prompt_file}") && !strings.Contains(args, "{prompt}") {
			return client, fail("invalid_config", "client must receive {prompt_file} or {prompt}")
		}
		client.Capabilities = []string{"launch"}
		if len(client.ResumeArgv) > 0 {
			client.Capabilities = append(client.Capabilities, "resume")
		}
		if len(client.DeliverArgv) > 0 {
			client.Capabilities = append(client.Capabilities, "deliver")
		}
	}
	return client, nil
}
func (s *Service) BindThread(ctx context.Context, selector, id, thread string, keys ...string) (Session, error) {
	var out Session
	if strings.TrimSpace(thread) == "" {
		return out, fail("thread_required", "provide the client's conversation ID")
	}
	err := mutate(s, ctx, selector, keys, []any{"session.bind-thread", id, thread}, &out, func(d *Document) error { return s.requireSessionOwner(d, id) }, func(d *Document) error {
		actor, err := s.actor(d)
		if err != nil {
			return err
		}
		p, err := findSession(d, id)
		if err != nil {
			return err
		}
		if actor != nil && actor.ID != p.ID && actor.AgentID != d.State.OrchestratorAgentID {
			return fail("forbidden", "can only bind your own client thread")
		}
		if p.ClientThreadID != "" && p.ClientThreadID != thread {
			return fail("thread_conflict", "session is already bound to a different client thread")
		}
		p.ClientThreadID = thread
		out = *p
		return saveDocument(d)
	})
	return out, err
}
func (s *Service) clientState(ctx context.Context, selector, id, thread, state string) error {
	return s.With(ctx, selector, func(d *Document) error {
		p, err := findSession(d, id)
		if err != nil {
			return err
		}
		if !p.Active() {
			return fail("stale_session", "session no longer owns the runtime")
		}
		if thread != "" {
			p.ClientThreadID = thread
		}
		if p.ClientState == state && thread == "" {
			return nil
		}
		p.ClientState = state
		return saveDocument(d)
	})
}
func (s *Service) markDelivered(ctx context.Context, selector, session string, ids []string) error {
	return s.With(ctx, selector, func(d *Document) error {
		p, err := findSession(d, session)
		if err != nil {
			return err
		}
		if !p.Active() {
			return fail("stale_session", "session ended before delivery was recorded")
		}
		changed := false
		for _, id := range ids {
			m, err := findMessage(d, id)
			if err != nil {
				return err
			}
			if m.ToAgent != p.AgentID {
				return fail("forbidden", "delivery recipient mismatch")
			}
			now := nowUTC()
			m.DeliveredAt = &now
			m.DeliveredSessionID = session
			changed = true
		}
		if changed {
			return saveDocument(d)
		}
		return nil
	})
}
