package core

import (
	"context"
	"strings"
	"time"
)

var legacyOpenCodeDeliverArgv = []string{
	"python3",
	"{project_dir}/scripts/opencode-deliver.py",
	"{thread_id}",
	"{message_file}",
	"{message_id}",
}

// upgradeLegacyOpenCodeClient recognizes only the bundled delivery wrapper
// shipped by the historical project configuration. It deliberately compares
// argv values rather than checking whether a path exists: the registry must
// remain migratable after the retired wrapper has been removed.
func upgradeLegacyOpenCodeClient(client Client) (Client, bool) {
	if client.Adapter != "opencode" || len(client.DeliverArgv) != len(legacyOpenCodeDeliverArgv) {
		return client, false
	}
	for i, arg := range client.DeliverArgv {
		if arg != legacyOpenCodeDeliverArgv[i] {
			return client, false
		}
	}
	client.DeliverArgv = nil
	client.NativeDelivery = true
	return client, true
}

func normalizeClient(client Client) (Client, error) {
	client, _ = upgradeLegacyOpenCodeClient(client)
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
		// OpenCode is native by default. A non-empty delivery wrapper is an
		// explicit external transport and remains external, even if an older
		// config also set NativeDelivery.
		client.NativeDelivery = len(client.DeliverArgv) == 0
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
		if client.Adapter == "opencode" && client.NativeDelivery {
			client.Capabilities = append(client.Capabilities, "deliver", "observe")
		} else if len(client.DeliverArgv) > 0 {
			client.Capabilities = append(client.Capabilities, "deliver")
		}
	}
	return client, nil
}

func (s *Service) setDeliveryPhase(ctx context.Context, selector, messageID, runID, phase, deliveryErr string) error {
	return s.With(ctx, selector, func(d *Document) error {
		run, err := findRun(d, runID)
		if err != nil {
			return err
		}
		p, err := findSession(d, run.SessionID)
		if err != nil {
			return err
		}
		if p.CurrentRunID != run.ID || !run.Active() {
			return fail("stale_run", "run no longer owns the session runtime")
		}
		for i := range d.Registry.Deliveries {
			attempt := &d.Registry.Deliveries[i]
			if attempt.MessageID == messageID && attempt.RunID == runID {
				if attempt.Phase == phase && attempt.Error == deliveryErr {
					return nil
				}
				attempt.Phase, attempt.Error, attempt.UpdatedAt = phase, deliveryErr, nowUTC()
				return saveDocument(d)
			}
		}
		d.Registry.Deliveries = append(d.Registry.Deliveries, DeliveryAttempt{MessageID: messageID, RunID: runID, Phase: phase, Error: deliveryErr, UpdatedAt: time.Now().UTC()})
		return saveDocument(d)
	})
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
		for i := range d.Registry.Sessions {
			other := &d.Registry.Sessions[i]
			if other.ID != p.ID && other.Active() && other.ClientSnapshot.Adapter == p.ClientSnapshot.Adapter && other.ClientThreadID == thread {
				return fail("thread_conflict", "client thread is already bound to active session %s", other.ID)
			}
		}
		p.ClientThreadID = thread
		if p.CurrentRunID != "" {
			if r, err := findRun(d, p.CurrentRunID); err == nil {
				r.ClientThreadID = thread
			}
		}
		out = *p
		return saveDocument(d)
	})
	return out, err
}
func (s *Service) clientState(ctx context.Context, selector, id, thread, state string) error {
	return s.With(ctx, selector, func(d *Document) error {
		r, err := findRun(d, id)
		if err != nil {
			return err
		}
		p, err := findSession(d, r.SessionID)
		if err != nil {
			return err
		}
		if p.CurrentRunID != r.ID || !r.Active() {
			return fail("stale_run", "run no longer owns the session runtime")
		}
		if thread != "" {
			for i := range d.Registry.Sessions {
				other := &d.Registry.Sessions[i]
				if other.ID != p.ID && other.Active() && other.ClientSnapshot.Adapter == p.ClientSnapshot.Adapter && other.ClientThreadID == thread {
					return fail("thread_conflict", "client thread is already bound to active session %s", other.ID)
				}
			}
			p.ClientThreadID = thread
			r.ClientThreadID = thread
		}
		if r.ClientState == state && thread == "" {
			return nil
		}
		r.ClientState = state
		return saveDocument(d)
	})
}
func (s *Service) markDelivered(ctx context.Context, selector, session string, ids []string) error {
	return s.With(ctx, selector, func(d *Document) error {
		r, err := findRun(d, session)
		if err != nil {
			return err
		}
		p, err := findSession(d, r.SessionID)
		if err != nil {
			return err
		}
		if p.CurrentRunID != r.ID || !r.Active() {
			return fail("stale_run", "run ended before delivery was recorded")
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
			m.DeliveredSessionID = p.ID
			m.DeliveredRunID = r.ID
			changed = true
		}
		if changed {
			return saveDocument(d)
		}
		return nil
	})
}
