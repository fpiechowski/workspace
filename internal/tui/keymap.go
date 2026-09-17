package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// keyMap is the single source of truth for every shortcut the interface
// accepts. Update logic matches against these bindings, and both the
// contextual footer and the full help screen are derived from the same
// definitions plus the current selection.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	PageUp   key.Binding
	PageDown key.Binding

	Open key.Binding
	Back key.Binding

	Filter     key.Binding
	FilterNext key.Binding
	Sort       key.Binding
	Focus      key.Binding
	FocusPrev  key.Binding

	Primary [5]key.Binding
	Related [3]key.Binding

	Terminal        key.Binding
	Jump            key.Binding
	Orchestrator    key.Binding
	WorkspacePicker key.Binding

	Actions key.Binding
	Refresh key.Binding

	Help key.Binding
	Exit key.Binding

	// Mode-specific legends. They describe keys that special input modes own
	// rather than new commands.
	Apply       key.Binding
	FormConfirm key.Binding
	Cancel      key.Binding
	ClearFilter key.Binding
	Retry       key.Binding
}

// defaultKeyMap returns the canonical bindings. Bindings stay enabled here so
// that matching a key never depends on presentation state; the help/footer
// layer disables copies per selection instead.
func defaultKeyMap() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:      key.NewBinding(key.WithKeys("home"), key.WithHelp("Home", "top")),
		Bottom:   key.NewBinding(key.WithKeys("end"), key.WithHelp("End", "bottom")),
		PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("PgUp", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("PgDn", "page down")),
		Open:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("Enter", "open")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("Esc", "back")),

		Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		FilterNext: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "status")),
		Sort:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
		Focus:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("Tab", "focus")),
		FocusPrev:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("Shift+Tab", "focus")),

		Primary: [5]key.Binding{
			key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "Tasks")),
			key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "Sessions")),
			key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "Worktrees")),
			key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "Results")),
			key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "More")),
		},
		Related: [3]key.Binding{
			key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "Sessions")),
			key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "Worktrees")),
			key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "Results")),
		},

		Terminal:        key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "terminal")),
		Jump:            key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "jump")),
		Orchestrator:    key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "orchestrator")),
		WorkspacePicker: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "workspace")),

		Actions: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),

		Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Exit: key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),

		Apply:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("Enter", "apply")),
		FormConfirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("Enter", "confirm")),
		Cancel:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("Esc", "cancel")),
		ClearFilter: key.NewBinding(key.WithKeys("esc"), key.WithHelp("Esc", "clear filter")),
		Retry:       key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "retry")),
	}
}

// exitBinding advertises quit wording for manual instances and hide wording for
// the managed panel without changing key ownership.
func (m *Model) exitBinding() key.Binding {
	b := m.keys.Exit
	if m.managed {
		b.SetHelp(b.Help().Key, "hide")
	}
	return b
}

// enabled returns a copy of binding with its displayed/matching enabled state
// set. Copies keep the canonical keyMap immutable.
func enabled(binding key.Binding, ok bool) key.Binding {
	binding.SetEnabled(ok)
	return binding
}

// terminalCapable reports the selection kinds whose `t` command can start or
// resume a terminal instead of only showing a hint.
func terminalCapable(kind string) bool {
	switch kind {
	case "task", "session", "run", "agent", "orchestrator", "worktree", "service", "workspace":
		return true
	}
	return false
}

// jumpCapable reports the selection kinds that resolve to a verified tmux
// target. Durable records without a live process (artifact, handoff, check,
// decision, change request) are excluded.
func jumpCapable(kind string) bool {
	switch kind {
	case "workspace", "orchestrator", "worktree", "session", "run", "service":
		return true
	}
	return false
}

// selectionKind identifies what the current route or selected row represents.
func (m *Model) selectionKind() string {
	if m.isCollectionPage() {
		item, _, _ := m.selectedItem()
		return item.Kind
	}
	return m.route.Page
}

// contextFlags is the selection-aware capability snapshot shared by the
// footer, the full help screen, and tests.
type contextFlags struct {
	terminal    bool
	jump        bool
	actions     bool
	filter      bool
	status      bool
	sort        bool
	focus       bool
	taskRelated bool
	workspace   bool
}

func (m *Model) contextFlags() contextFlags {
	kind := m.selectionKind()
	collection := m.isCollectionPage()
	options, _ := m.availableActions()
	return contextFlags{
		terminal:    terminalCapable(kind),
		jump:        jumpCapable(kind),
		actions:     len(options) > 0,
		filter:      collection,
		status:      collection && len(m.statusFilterOptions()) > 1,
		sort:        collection,
		focus:       m.route.Page == "results",
		taskRelated: m.route.Page == "task",
		workspace:   m.workspaceID != "",
	}
}

// helpGroup is one named category of the full help screen.
type helpGroup struct {
	Title string
	Keys  []key.Binding
}

// keyGroups returns the categorized, selection-aware bindings. Disabled
// bindings are kept in the slices so that turning a capability on restores them
// without duplicating the grouping.
func (m *Model) keyGroups() []helpGroup {
	flags := m.contextFlags()

	navigation := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Open, m.keys.Back}
	if flags.taskRelated {
		navigation = append(navigation,
			enabled(m.keys.Related[0], true),
			enabled(m.keys.Related[1], true),
			enabled(m.keys.Related[2], true),
		)
	} else {
		for i := range m.keys.Primary {
			navigation = append(navigation, enabled(m.keys.Primary[i], flags.workspace))
		}
	}
	navigation = append(navigation,
		enabled(m.keys.Focus, flags.focus),
		enabled(m.keys.FocusPrev, flags.focus),
		enabled(m.keys.PageUp, true),
		enabled(m.keys.PageDown, true),
	)

	view := []key.Binding{
		enabled(m.keys.Filter, flags.filter),
		enabled(m.keys.FilterNext, flags.status),
		enabled(m.keys.Sort, flags.sort),
		enabled(m.keys.Help, true),
	}

	runtime := []key.Binding{
		enabled(m.keys.Terminal, flags.terminal),
		enabled(m.keys.Jump, flags.jump),
		enabled(m.keys.Orchestrator, flags.workspace),
	}

	actions := []key.Binding{
		enabled(m.keys.Actions, flags.actions),
		enabled(m.keys.Refresh, true),
		enabled(m.keys.WorkspacePicker, true),
	}

	exit := []key.Binding{enabled(m.exitBinding(), true)}

	return []helpGroup{
		{Title: "Navigation", Keys: navigation},
		{Title: "View", Keys: view},
		{Title: "Runtime", Keys: runtime},
		{Title: "Actions", Keys: actions},
		{Title: "Exit", Keys: exit},
	}
}

// shortHelp is the curated one-line legend: the most relevant enabled commands,
// starting with the contextual primary action so narrow terminals keep it.
func (m *Model) shortHelp() []key.Binding {
	flags := m.contextFlags()
	var out []key.Binding
	add := func(binding key.Binding, ok bool) {
		if ok {
			out = append(out, enabled(binding, true))
		}
	}
	add(m.keys.Terminal, flags.terminal)
	add(m.exitBinding(), true)
	add(m.keys.Open, true)
	add(m.keys.Up, true)
	add(m.keys.Focus, flags.focus)
	add(m.keys.Filter, flags.filter)
	add(m.keys.FilterNext, flags.status)
	add(m.keys.Sort, flags.sort)
	add(m.keys.Actions, flags.actions)
	add(m.keys.Jump, flags.jump)
	if flags.taskRelated {
		add(m.keys.Related[0], true)
		add(m.keys.Related[1], true)
		add(m.keys.Related[2], true)
	}
	add(m.keys.Back, true)
	add(m.keys.Help, true)
	return out
}

// legend renders a short help line through bubbles/help so it is width aware.
func (m *Model) legend(bindings []key.Binding) string {
	model := help.New()
	model.Width = max(1, m.width)
	model.Styles = helpStyles(m.palette)
	return model.ShortHelpView(bindings)
}

func helpStyles(p palette) help.Styles {
	if p.noColor {
		plain := lipgloss.NewStyle()
		return help.Styles{
			ShortKey:       plain,
			ShortDesc:      plain,
			ShortSeparator: plain,
			Ellipsis:       plain,
			FullKey:        plain,
			FullDesc:       plain,
			FullSeparator:  plain,
		}
	}
	return help.Styles{
		ShortKey:       p.keycapStyle(),
		ShortDesc:      p.metaStyle(),
		ShortSeparator: p.subtleStyle(),
		Ellipsis:       p.subtleStyle(),
		FullKey:        p.keycapStyle(),
		FullDesc:       p.metaStyle(),
		FullSeparator:  p.subtleStyle(),
	}
}
