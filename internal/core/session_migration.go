package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

const registrySchemaVersion = 2

func migratedSessionID(key string) string {
	sum := sha256.Sum256([]byte("workspace/logical-session/v2\x00" + key))
	return "sess_" + hex.EncodeToString(sum[:12])
}

// migrateRegistryV2 converts the former process-shaped Session records into
// logical Sessions plus concrete Runs. Old sess_* IDs are retained as Run IDs,
// so every persisted provenance reference and active legacy process remains
// resolvable. Grouping is deliberately conservative.
func migrateRegistryV2(d *Document) bool {
	if d.Registry.SchemaVersion >= registrySchemaVersion {
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
					ParentAgentID: old.ParentAgentID, WorktreeID: old.WorktreeID,
					TaskID: old.TaskID, TaskAttempt: old.TaskAttempt, InputDigest: old.InputDigest,
					ClientSnapshot: old.ClientSnapshot, ClientThreadID: old.ClientThreadID,
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
			r := Run{ID: old.ID, SessionID: sid, Profile: old.Profile, Route: old.Route,
				RoutingDecision: old.RoutingDecision, Argv: append([]string(nil), old.Argv...), CWD: old.CWD,
				PromptFile: old.PromptFile, State: old.State, PaneID: old.PaneID, WindowID: old.WindowID,
				ClientState: old.ClientState, ClientThreadID: old.ClientThreadID, CreatedAt: old.CreatedAt,
				FinishedAt: old.FinishedAt, ExitCode: old.ExitCode, Error: old.Error}
			d.Registry.Runs = append(d.Registry.Runs, r)
		}
		for i := range d.Registry.Sessions {
			p := &d.Registry.Sessions[i]
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
	d.Registry.SchemaVersion = registrySchemaVersion
	d.syncSessions()
	return changed
}
