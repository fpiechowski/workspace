package cli

import (
	"encoding/json"
	"strings"

	"workspace/internal/core"
)

// shortOutput reduces command results to identifiers and the small amount of
// context needed to recognize them. JSON output deliberately bypasses this
// transformation: --json is the stable, lossless machine-readable contract.
func shortOutput(v any) any {
	switch value := v.(type) {
	case core.Status:
		return map[string]string{workspaceTitle(value): value.Workspace.ID}
	case []core.Status:
		return workspaceNameIDs(value)
	}

	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var value any
	if err := json.Unmarshal(b, &value); err != nil {
		return v
	}
	return compactValue(value, true)
}

func compactValue(value any, single bool) any {
	switch value := value.(type) {
	case []any:
		if named, ok := compactNamedRecords(value); ok {
			return named
		}
		out := make([]any, len(value))
		for i := range value {
			out[i] = compactValue(value[i], false)
		}
		return out
	case map[string]any:
		if _, ok := value["id"].(string); ok {
			return compactRecord(value, single)
		}
		out := make(map[string]any, len(value))
		for key, child := range value {
			out[key] = compactValue(child, true)
		}
		return out
	default:
		return value
	}
}

func compactNamedRecords(values []any) (map[string]string, bool) {
	if len(values) == 0 {
		return nil, false
	}
	out := make(map[string]string, len(values))
	for _, value := range values {
		record, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		id, _ := record["id"].(string)
		name := recordLabel(record)
		if id == "" || name == "" || name == id {
			return nil, false
		}
		if _, exists := out[name]; exists {
			name += " (" + id + ")"
		}
		out[name] = id
	}
	return out, true
}

func recordLabel(record map[string]any) string {
	for _, key := range []string{"title", "name", "question", "id"} {
		if value, ok := record[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compactRecord(record map[string]any, single bool) map[string]any {
	keys := []string{
		"id", "title", "name", "status", "state", "phase", "kind", "role",
		"profile", "task_id", "agent_id", "from_agent", "to_agent", "session_id",
		"run_id", "current_run_id", "last_run_id", "run_count", "generation", "lifecycle_state", "run_state",
		"client_thread_id", "pane_id", "last_active_at",
		"worktree_id", "attempt", "exit_code", "path", "branch", "url", "reference",
		"available", "stopping", "revision",
	}
	if single {
		// Reading one message or handoff must remain useful in short mode.
		keys = append(keys, "body", "summary", "outcome", "question", "answer", "reason")
	}
	out := make(map[string]any)
	for _, key := range keys {
		if value, ok := record[key]; ok && !emptyJSONValue(value) {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return record
	}
	return out
}

func emptyJSONValue(value any) bool {
	switch value := value.(type) {
	case nil:
		return true
	case string:
		return value == ""
	case []any:
		return len(value) == 0
	case map[string]any:
		return len(value) == 0
	default:
		return false
	}
}
