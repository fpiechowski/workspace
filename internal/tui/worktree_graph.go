package tui

import "strings"

// Sorting applies to siblings. Filtered-out parents leave visible children at
// the root; their subtitle retains the actual source, never a fabricated edge.
func (m *Model) orderWorktreeGraph(items []collectionItem) []collectionItem {
	visible := make(map[string]bool, len(items))
	parents := make(map[string]string, len(items))
	for _, item := range items {
		visible[item.ID] = true
	}
	for _, w := range m.snapshot.Status.Worktrees {
		parents[w.ID] = w.ParentWorktreeID
	}
	children := make(map[string][]collectionItem)
	for _, item := range items {
		p := parents[item.ID]
		if !visible[p] || p == item.ID {
			p = ""
		}
		children[p] = append(children[p], item)
	}
	out := make([]collectionItem, 0, len(items))
	seen := make(map[string]bool)
	var walk func([]collectionItem, string)
	walk = func(nodes []collectionItem, prefix string) {
		for i, item := range nodes {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			connector, next := "├─ ", "│  "
			if i == len(nodes)-1 {
				connector, next = "└─ ", "   "
			}
			item.TreePrefix = prefix + connector
			out = append(out, item)
			walk(children[item.ID], prefix+next)
		}
	}
	walk(children[""], "")
	// Malformed legacy/imported cycles must not hide rows or recurse forever.
	for _, item := range items {
		if !seen[item.ID] {
			walk([]collectionItem{item}, "")
		}
	}
	return out
}

func (m *Model) worktreeLineage(id string) string {
	w, ok := findWorktree(m.snapshot.Status.Worktrees, id)
	if !ok {
		return ""
	}
	source := "source unknown"
	if w.ParentWorktreeID != "" {
		source = w.ParentWorktreeID + " (missing)"
		if parent, exists := findWorktree(m.snapshot.Status.Worktrees, w.ParentWorktreeID); exists {
			source = parent.Name
		}
	} else if w.BaseCommit != "" && w.BaseCommit == m.snapshot.Status.Workspace.Base.Commit {
		source = "workspace base"
	} else if w.BaseRef != "" {
		source = w.BaseRef + " (source unknown)"
	}
	return "from " + source + " @ " + shortRevision(w.BaseCommit)
}

func (m *Model) worktreeTasks(id string) string {
	ids := make(map[string]bool)
	for _, session := range m.snapshot.Status.Sessions {
		if session.WorktreeID == id {
			ids[session.TaskID] = true
		}
	}
	var names []string
	for _, task := range m.snapshot.Status.Workspace.Tasks {
		if task.WorktreeID == id || ids[task.ID] {
			names = append(names, firstNonempty(task.Title, task.ID))
		}
	}
	if len(names) == 0 {
		return ""
	}
	return " · Tasks: " + strings.Join(names, ", ")
}
