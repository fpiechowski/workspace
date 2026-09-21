package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

const (
	logicalRegistrySchemaVersion  = 3
	opencodeRegistrySchemaVersion = 4
	registrySchemaVersion         = 5
)

func migratedSessionID(key string) string {
	sum := sha256.Sum256([]byte("workspace/logical-session/v2\x00" + key))
	return "sess_" + hex.EncodeToString(sum[:12])
}

// migrateRegistryV2 converts the former process-shaped Session records into
// logical Sessions plus concrete Runs. Old sess_* IDs are retained as Run IDs,
// so every persisted provenance reference and active legacy process remains
// resolvable. Grouping is deliberately conservative.
func migrateRegistryV2(d *Document) bool {
	if d.Registry.SchemaVersion >= logicalRegistrySchemaVersion {
		d.syncSessions()
		return false
	}
	changed := true
	if len(d.Registry.Runs) == 0 && len(d.Registry.Sessions) > 0 {
		legacy := append([]Session(nil), d.Registry.Sessions...)
		d.Registry.Sessions = nil
		groups := map[string]string{}
		legacyToLogical := map[string]string{}
		for _, old := range legacy {
			key := "legacy:" + old.ID
			if old.ClientThreadID != "" {
				key = fmt.Sprintf("thread:%s\x00agent:%s\x00adapter:%s\x00task:%s\x00attempt:%d\x00input:%s\x00worktree:%s",
					old.ClientThreadID, old.AgentID, old.ClientSnapshot.Adapter, old.TaskID, old.TaskAttempt, old.InputDigest, old.WorktreeID)
			}
			sid, ok := groups[key]
			if !ok {
				sid = migratedSessionID(key)
				groups[key] = sid
				d.Registry.Sessions = append(d.Registry.Sessions, Session{
					ID: sid, AgentID: old.AgentID, AgentSnapshot: old.AgentSnapshot,
					ParentAgentID: old.ParentAgentID, ParentSessionID: old.ParentSessionID, WorktreeID: old.WorktreeID,
					TaskID: old.TaskID, TaskAttempt: old.TaskAttempt, InputDigest: old.InputDigest,
					ReasoningEffort: old.ReasoningEffort,
					ClientSnapshot:  old.ClientSnapshot, ClientThreadID: old.ClientThreadID,
					ReadOnly: old.ReadOnly, CreatedAt: old.CreatedAt, LifecycleState: "idle",
				})
			} else {
				for i := range d.Registry.Sessions {
					p := &d.Registry.Sessions[i]
					if p.ID == sid && (p.CreatedAt.IsZero() || !old.CreatedAt.IsZero() && old.CreatedAt.Before(p.CreatedAt)) {
						p.CreatedAt = old.CreatedAt
					}
				}
			}
			legacyToLogical[old.ID] = sid
			r := Run{ID: old.ID, SessionID: sid, Profile: old.Profile, ReasoningEffort: old.ReasoningEffort, Route: old.Route,
				RoutingDecision: old.RoutingDecision, Argv: append([]string(nil), old.Argv...), CWD: old.CWD,
				PromptFile: old.PromptFile, State: old.State, PaneID: old.PaneID, WindowID: old.WindowID,
				ClientState: old.ClientState, ClientThreadID: old.ClientThreadID, OpenCodeEndpoint: old.OpenCodeEndpoint, CreatedAt: old.CreatedAt,
				FinishedAt: old.FinishedAt, ExitCode: old.ExitCode, Error: old.Error}
			d.Registry.Runs = append(d.Registry.Runs, r)
		}
		for i := range d.Registry.Sessions {
			p := &d.Registry.Sessions[i]
			if sid := legacyToLogical[p.ParentSessionID]; sid != "" {
				p.ParentSessionID = sid
			}
			indices := make([]int, 0)
			for j := range d.Registry.Runs {
				if d.Registry.Runs[j].SessionID == p.ID {
					indices = append(indices, j)
				}
			}
			sort.SliceStable(indices, func(a, b int) bool {
				left, right := d.Registry.Runs[indices[a]], d.Registry.Runs[indices[b]]
				return left.CreatedAt.Before(right.CreatedAt) || left.CreatedAt.Equal(right.CreatedAt) && left.ID < right.ID
			})
			var active *Run
			for generation, j := range indices {
				r := &d.Registry.Runs[j]
				r.Generation = generation + 1
				p.LastRunID = r.ID
				if r.Active() && (active == nil || r.CreatedAt.After(active.CreatedAt) || r.CreatedAt.Equal(active.CreatedAt) && r.ID > active.ID) {
					active = r
				}
			}
			if active != nil {
				p.CurrentRunID = active.ID
				for j := range d.Registry.Runs {
					r := &d.Registry.Runs[j]
					if r.SessionID == p.ID && r.Active() && r.ID != active.ID {
						r.State = "interrupted"
						r.Error = "superseded during session schema migration"
					}
				}
			}
		}
		// A legacy registry may already contain conflicting active records. Keep
		// only the newest owner for an adapter/thread binding and fence the rest.
		owners := map[string]*Run{}
		for i := range d.Registry.Sessions {
			p := &d.Registry.Sessions[i]
			if p.ClientThreadID == "" || p.CurrentRunID == "" {
				continue
			}
			r, err := findRun(d, p.CurrentRunID)
			if err != nil || !r.Active() {
				continue
			}
			key := p.ClientSnapshot.Adapter + "\x00" + p.ClientThreadID
			if prior := owners[key]; prior != nil {
				loser := r
				if r.CreatedAt.After(prior.CreatedAt) || r.CreatedAt.Equal(prior.CreatedAt) && r.ID > prior.ID {
					loser, owners[key] = prior, r
				}
				loser.State = "interrupted"
				loser.Error = "duplicate native thread ownership fenced during migration"
				for j := range d.Registry.Sessions {
					if d.Registry.Sessions[j].CurrentRunID == loser.ID {
						d.Registry.Sessions[j].CurrentRunID = ""
					}
				}
			} else {
				owners[key] = r
			}
		}
		for i := range d.State.Tasks {
			if sid := legacyToLogical[d.State.Tasks[i].SessionID]; sid != "" {
				d.State.Tasks[i].RunID = d.State.Tasks[i].SessionID
				d.State.Tasks[i].SessionID = sid
			}
		}
		for i := range d.Registry.Checks {
			if sid := legacyToLogical[d.Registry.Checks[i].SessionID]; sid != "" {
				d.Registry.Checks[i].RunID = d.Registry.Checks[i].SessionID
				d.Registry.Checks[i].SessionID = sid
			}
		}
		for i := range d.Registry.Handoffs {
			if sid := legacyToLogical[d.Registry.Handoffs[i].ToSession]; sid != "" {
				d.Registry.Handoffs[i].ToSession = sid
			}
			if sid := legacyToLogical[d.Registry.Handoffs[i].FromSession]; sid != "" {
				d.Registry.Handoffs[i].FromRun = d.Registry.Handoffs[i].FromSession
				d.Registry.Handoffs[i].FromSession = sid
			}
		}
		for i := range d.State.Artifacts {
			if sid := legacyToLogical[d.State.Artifacts[i].SessionID]; sid != "" {
				d.State.Artifacts[i].RunID = d.State.Artifacts[i].SessionID
				d.State.Artifacts[i].SessionID = sid
			}
		}
		for i := range d.Registry.Messages {
			m := &d.Registry.Messages[i]
			if sid := legacyToLogical[m.ToSession]; sid != "" {
				m.ToSession = sid
			}
			if sid := legacyToLogical[m.FromSession]; sid != "" {
				m.FromRun, m.FromSession = m.FromSession, sid
			}
			if sid := legacyToLogical[m.DeliveredSessionID]; sid != "" {
				m.DeliveredRunID, m.DeliveredSessionID = m.DeliveredSessionID, sid
			}
			if sid := legacyToLogical[m.NotifiedSessionID]; sid != "" {
				m.NotifiedRunID, m.NotifiedSessionID = m.NotifiedSessionID, sid
			}
		}
		for key, op := range d.Registry.Operations {
			if sid := legacyToLogical[op.ResourceID]; sid != "" {
				op.ResourceID = sid
				// Session start resources can be reconstructed from ResourceID. An
				// old serialized process-shaped Session would violate the v2 response.
				op.Result = nil
				d.Registry.Operations[key] = op
			}
		}
	}
	d.Registry.SchemaVersion = logicalRegistrySchemaVersion
	d.syncSessions()
	return changed
}

// migrateRegistryV3 upgrades only persisted Session client snapshots. Runs
// and all references to concrete executions are intentionally untouched.
func migrateRegistryV3(d *Document) bool {
	if d.Registry.SchemaVersion >= opencodeRegistrySchemaVersion {
		d.syncSessions()
		return false
	}
	if d.Registry.SchemaVersion != logicalRegistrySchemaVersion {
		return false
	}
	for i := range d.Registry.Sessions {
		client, upgraded := upgradeLegacyOpenCodeClient(d.Registry.Sessions[i].ClientSnapshot)
		if !upgraded {
			continue
		}
		// The historical snapshot is known to be a project client shape. Run
		// the same normalizer used by Config so capabilities are regenerated.
		if normalized, err := normalizeClient(client); err == nil {
			client = normalized
		}
		d.Registry.Sessions[i].ClientSnapshot = client
	}
	d.Registry.SchemaVersion = opencodeRegistrySchemaVersion
	d.syncSessions()
	return true
}

func validSessionTarget(d *Document, sessionID, agentID string) bool {
	if agentID == "" {
		return false
	}
	_, ok := canonicalSessionTarget(d, sessionID, agentID)
	return ok
}

func canonicalSessionTarget(d *Document, sessionID, agentID string) (string, bool) {
	if sessionID == "" {
		return "", false
	}
	session, err := findSession(d, sessionID)
	if err != nil {
		return "", false
	}
	if agentID != "" {
		if agent, agentErr := findAgent(d, agentID); agentErr == nil {
			agentID = agent.ID
		}
		if session.AgentID != agentID {
			return "", false
		}
	}
	return session.ID, true
}

// inferParentSession uses only a single unambiguous open Session belonging to
// the recorded parent Agent. It intentionally leaves ambiguous ancestry
// legacy rather than selecting a latest Session.
func inferParentSession(d *Document, child Session) string {
	if child.ParentAgentID == "" {
		return ""
	}
	candidates := openSessionCandidates(d, child.ParentAgentID)
	history := sessionHistoryCandidates(d, child.ParentAgentID)
	filtered := make([]Session, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == child.ID {
			continue
		}
		if !child.CreatedAt.IsZero() && !candidate.CreatedAt.IsZero() && candidate.CreatedAt.After(child.CreatedAt) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	if len(filtered) == 1 && len(history) == 1 {
		return filtered[0].ID
	}
	return ""
}

func sessionHistoryCandidates(d *Document, agentID string) []Session {
	var out []Session
	for _, session := range d.Registry.Sessions {
		if session.AgentID == agentID {
			out = append(out, session)
		}
	}
	return out
}

func uniqueHistoricalOpenSession(d *Document, agentID string) string {
	if agent, err := findAgent(d, agentID); err == nil {
		agentID = agent.ID
	}
	open := openSessionCandidates(d, agentID)
	if len(open) == 1 && len(sessionHistoryCandidates(d, agentID)) == 1 {
		return open[0].ID
	}
	return ""
}

func historicalMessageTarget(d *Document, message Message) string {
	if target, ok := canonicalSessionTarget(d, message.DeliveredSessionID, ""); ok {
		return target
	}
	if target, ok := canonicalSessionTarget(d, message.NotifiedSessionID, ""); ok {
		return target
	}
	if message.HandoffID != "" {
		if handoff, err := findHandoff(d, message.HandoffID); err == nil {
			if target, ok := canonicalSessionTarget(d, handoff.ToSession, ""); ok {
				return target
			}
		}
	}
	return uniqueHistoricalOpenSession(d, message.ToAgent)
}

// migrateRegistryV4 adds session-targeted communication and repairs native
// OpenCode transport metadata. The stage preserves ambiguous historical
// agent-addressed records as legacy records with an empty ToSession.
func migrateRegistryV4(d *Document) bool {
	if d.Registry.SchemaVersion >= registrySchemaVersion {
		d.syncSessions()
		return false
	}
	if d.Registry.SchemaVersion != opencodeRegistrySchemaVersion {
		return false
	}
	changed := true

	for i := range d.Registry.Sessions {
		session := &d.Registry.Sessions[i]
		if agent, err := findAgent(d, session.ParentAgentID); err == nil {
			session.ParentAgentID = agent.ID
		}
		if session.ParentSessionID != "" {
			if canonical, ok := canonicalSessionTarget(d, session.ParentSessionID, session.ParentAgentID); ok {
				session.ParentSessionID = canonical
				if parent, err := findSession(d, canonical); err == nil && session.ParentAgentID == "" {
					session.ParentAgentID = parent.AgentID
				}
			} else {
				// Keep ParentAgentID as immutable historical ownership, but make an
				// invalid session pointer explicitly legacy instead of trusting it.
				session.ParentSessionID = ""
			}
		}
		if session.ParentSessionID == "" {
			session.ParentSessionID = inferParentSession(d, *session)
		}
		if session.ClientSnapshot.Adapter == "opencode" {
			if normalized, err := normalizeClient(session.ClientSnapshot); err == nil {
				if payloadDigest(normalized) != payloadDigest(session.ClientSnapshot) {
					session.ClientSnapshot = normalized
				}
			}
		}
	}

	for i := range d.Registry.Messages {
		message := &d.Registry.Messages[i]
		if agent, err := findAgent(d, message.ToAgent); err == nil {
			message.ToAgent = agent.ID
		}
		if message.ToSession != "" {
			if canonical, ok := canonicalSessionTarget(d, message.ToSession, ""); ok {
				message.ToSession = canonical
				if target, err := findSession(d, canonical); err == nil {
					message.ToAgent = target.AgentID
				}
			} else {
				message.ToSession = ""
			}
		}
		if message.ToSession == "" {
			message.ToSession = historicalMessageTarget(d, *message)
			if target, err := findSession(d, message.ToSession); err == nil {
				message.ToAgent = target.AgentID
			}
		} else if target, err := findSession(d, message.ToSession); err == nil {
			message.ToAgent = target.AgentID
		}
	}
	for i := range d.Registry.Handoffs {
		handoff := &d.Registry.Handoffs[i]
		if agent, err := findAgent(d, handoff.ToAgent); err == nil {
			handoff.ToAgent = agent.ID
		}
		if handoff.ToSession != "" {
			if canonical, ok := canonicalSessionTarget(d, handoff.ToSession, ""); ok {
				handoff.ToSession = canonical
				if target, err := findSession(d, canonical); err == nil {
					handoff.ToAgent = target.AgentID
				}
			} else {
				handoff.ToSession = ""
			}
		}
		if handoff.ToSession == "" {
			candidate := ""
			ambiguous := false
			for _, message := range d.Registry.Messages {
				if message.HandoffID != handoff.ID || message.ToSession == "" || message.ToAgent != handoff.ToAgent {
					continue
				}
				if candidate != "" && candidate != message.ToSession {
					ambiguous = true
					break
				}
				candidate = message.ToSession
			}
			if !ambiguous && candidate != "" {
				handoff.ToSession = candidate
			} else if candidate := uniqueHistoricalOpenSession(d, handoff.ToAgent); candidate != "" {
				handoff.ToSession = candidate
			}
		}
		if handoff.ToSession != "" {
			if target, err := findSession(d, handoff.ToSession); err == nil {
				handoff.ToAgent = target.AgentID
			}
		}
	}

	for i := range d.Registry.Runs {
		run := &d.Registry.Runs[i]
		if run.OpenCodeEndpoint != "" {
			continue
		}
		session, err := findSession(d, run.SessionID)
		if err != nil || !usesNativeOpenCodeDelivery(session.ClientSnapshot) {
			continue
		}
		if endpoint, err := openCodeEndpointFromArgv(run.Argv); err == nil && endpoint != "" {
			run.OpenCodeEndpoint = endpoint
		}
	}

	d.Registry.SchemaVersion = registrySchemaVersion
	d.syncSessions()
	return changed
}

// repairPersistedRuntime keeps the repair invariant active for a registry that
// already reached the current schema. This is intentionally limited to
// deterministic OpenCode metadata; it does not infer a new conversation or
// rewrite immutable provenance.
func repairPersistedRuntime(d *Document) bool {
	changed := false
	for i := range d.Registry.Sessions {
		session := &d.Registry.Sessions[i]
		if session.ClientSnapshot.Adapter != "opencode" {
			continue
		}
		normalized, err := normalizeClient(session.ClientSnapshot)
		if err == nil && payloadDigest(normalized) != payloadDigest(session.ClientSnapshot) {
			session.ClientSnapshot = normalized
			changed = true
		}
	}
	for i := range d.Registry.Runs {
		run := &d.Registry.Runs[i]
		if run.OpenCodeEndpoint != "" {
			continue
		}
		session, err := findSession(d, run.SessionID)
		if err != nil || !usesNativeOpenCodeDelivery(session.ClientSnapshot) {
			continue
		}
		endpoint, err := openCodeEndpointFromArgv(run.Argv)
		if err == nil && endpoint != "" {
			run.OpenCodeEndpoint = endpoint
			changed = true
		}
	}
	if changed {
		d.syncSessions()
	}
	return changed
}

// migrateRegistry applies private registry migrations in source-version
// order. Each stage is idempotent; loadDocument persists the resulting
// document once through the normal atomic write-ahead path.
func migrateRegistry(d *Document) bool {
	changed := false
	for d.Registry.SchemaVersion < registrySchemaVersion {
		var stepChanged bool
		switch d.Registry.SchemaVersion {
		case 0, 1, 2:
			stepChanged = migrateRegistryV2(d)
		case logicalRegistrySchemaVersion:
			stepChanged = migrateRegistryV3(d)
		case opencodeRegistrySchemaVersion:
			stepChanged = migrateRegistryV4(d)
		default:
			return changed
		}
		if !stepChanged {
			return changed
		}
		changed = true
	}
	return changed
}
