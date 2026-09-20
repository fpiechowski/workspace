# Design System: workspace TUI

## Overview

**Operate** mode: a native terminal interface for observing agent work, reviewing
results, and performing explicit operations. The first screen is Tasks: each row
combines the task, its state, and the number of active executions, sessions, and runs
of its current attempt. Process completion does not mean result acceptance: the task,
session, and process states are distinguished, and every result remains attached to
its attempt.

The contract sources are [PRODUCT.md](PRODUCT.md) and [docs/tui.md](docs/tui.md).
The visual implementation is in `internal/tui/theme.go`, `status.go`, `view.go`, and
`work.go`.

## Colors

The palette distinguishes semantic roles, and renderers use them instead of raw colors:
primary, secondary, and subtle text; border and focused border; accent; selection
background and text; success, warning, danger, and an information surface. The `auto`
theme is resolved by detecting the terminal background (Lip Gloss / termenv). When the
terminal does not allow the background to be read reliably (pipe, tmux, screen, `dumb`),
the documented fallback is the dark palette. Explicit `dark` and `light` ignore
detection, while `--no-color` sets no color and remains deterministic. Outside the
selection, the terminal background remains the user's setting; the selection token
fills the entire selected entry, including its description.

| Token | Dark | Light | Role |
|---|---|---|---|
| `primary` | `#E2E8F0` | `#0F172A` | Primary text |
| `secondary` | `#B4C0D3` | `#334155` | Subtitles and labels |
| `subtle` | `#7C8CA3` | `#64748B` | Subtle metadata and IDs |
| `border` | `#475569` | `#CBD5E1` | Unfocused border |
| `focusedBorder` | `#67E8F9` | `#0E7490` | Active-panel border |
| `accent` | `#67E8F9` | `#0E7490` | Headings, focus, active work |
| `selectionFg` / `selectionBg` | `#F8FAFC` / `#1E293B` | `#0F172A` / `#E0F2FE` | Selection text and background |
| `success` | `#86EFAC` | `#166534` | Accepted, completed, ready |
| `warning` | `#FDE68A` | `#92400E` | Blocks, review, and interruption |
| `danger` | `#FDA4AF` | `#BE123C` | Failed, error |
| `infoSurface` | `#164E63` | `#E0F2FE` | Information-message background |

`--no-color` removes colors while preserving text, symbols, bold styling, and animation.
Status must remain recognizable without color.

## Typography

The terminal determines the typeface and font size. Hierarchy is created by the style
factories
(`titleStyle`, `headingStyle`, `sectionStyle`, `labelStyle`, `valueStyle`,
`warningStyle`, `metaStyle`, `keycapStyle`, `noticeStyle`, `selectedStyle`,
`panelStyle`): bold heading, section accent, field label and value, warning, muted
subtitle, selection highlight, and focus border. Widths are measured in terminal
columns with Unicode support; long text is truncated with `…`.

## Layout

The shell has a fixed order: identity and freshness header, primary navigation,
optional breadcrumb/secondary navigation, content, status/message row, and a
contextual key legend. The status row and legend are always reserved, so they remain
visible at every supported size. The breadcrumb appears on detail routes and dependent
collections, while Results uses this line as secondary result-type navigation; the
project picker and the primary Tasks, Sessions, Worktrees, and More collections do not
show it.

- Minimum is **40×12**; a smaller terminal shows a size message.
- One `layoutFor` decision controls every page: **tiny** below 40×12, **compact** for
  medium terminals (one column), and **wide** from 100×24 (list and details side by
  side).
- Collections in wide mode show two named, bordered panels (list and `Preview`) side by
  side, with one clear focus edge; the list width is based on two fifths of the terminal
  width. The project picker in wide mode shows the workspace list next to unbordered
  details for the selection, with a highlighted selection row.
- An entry has two rows: marker, status, and title, followed by context. When fewer than
  four rows remain for the list, the entry collapses to one. At other sizes, adjacent
  entries are separated by a lower-emphasis (`subtle`) line. The selection remains
  visible, and the preview action line advertises only commands supported by the
  selected kind.
- Main tabs and shortcuts use shorter forms below 60 columns; the tabs then read
  `1 Tasks`, `2 Sess`, `3 Trees`, `4 Out`, and `5 More`.

## Elevation & Depth

The layout is flat. Relationships are created by spacing, columns, headings, and
terminal borders. Focus is distinguished by color and bold styling, without shadows.

## Shapes

The panel renderer uses rounded Lip Gloss character borders. Active tabs receive `[ ]`,
and the selected entry receives the `›` marker.

## Components

**Tasks.** A selectable entry combines the task, its state, and the number of active
executions, sessions, and runs of the current attempt. The selection is tied to the ID
even after sorting. Empty collections show a title, one-sentence explanation, and one
valid action; the project picker advertises `a` (Create workspace) instead of claiming
that the TUI never creates a workspace.

**Sessions.** A selectable entry combines the agent, the logical session state, and its
current Run. The list excludes deleted sessions, supports the current/history filter
(`f`), and has no secondary tabs or its own `Tab` cycle. Opening an entry shows the
session details, and `t` opens or resumes its verified terminal.

**Board.** Tasks have two views: the default **List** and **Board**, toggled with `b`
(the footer shows `b board`/`b list`). Board has one column per task state in the fixed
order pending, running, blocked, needs_changes, awaiting_review, accepted, with unknown
states appended at the end and rendered as neutral text. Cards use the same data as the
list and retain the `›` selection marker; columns come from undeleted tasks on the Tasks
page, so filtering removes cards rather than columns. A column header is a state badge
with the number of visible cards, and the active column has a focus border. In wide mode,
columns sit side by side; when space is insufficient, a window scrolls around the active
column with its position in the count line. In compact mode, only the active column is
visible with the `‹ state (n) › k/m` pager, keeping the board readable from 40×12.
The selection remains pinned to `route.SelectedID`, while the List/Board choice is
remembered only for the session (like `route.Sort` and filters) and is not persisted
between launches.

**Runtime.** Tmux topology is a `bubbles/table` (window, pane, kind, owner, run, state)
in wide mode, and the same data in a row layout in compact mode. Runtime errors and
managed-interface state remain above the topology.

**Details and previews.** Every detail and preview is a document made from one set of
blocks: identity and status header, label/value pairs, section headings, bullets,
`[!]` warnings, links to related resources, and subtle provenance (ID, digest,
timestamps). The order is fixed: identity and status, operational facts, narrative,
related resources, and provenance at the end. External values are sanitized before
styling, while long goals, instructions, summaries, reasons, command lines, paths, and
change-request content wrap to the viewport width, with hard breaks for unbreakable
tokens. Explicit newlines in preview text are preserved, and tabs are expanded. Every
document scrolls in the existing viewport (`↑`/`↓`, `PgUp`/`PgDn`); the status row shows
`line x–y of n` only when content exceeds the view, and the scroll position returns when
the route is revisited.

**Status.** A label always accompanies the symbol: `✓` success, `×` error, `!` block,
`◈` review, `○` waiting, `■` stopped or closed, and `◇` interrupted or exited.
`running` and `starting` use a Braille animation every 120 ms. The live-agent summary
animates only for an active current Run; otherwise it shows `○`.

**Navigation and terminal.** Arrows or `j`/`k` select an entry; `Enter` opens details,
and `1`–`5` move between Tasks, Sessions, Worktrees, Results, and More. `Tab` changes
the result type on Results. `t` opens the verified current terminal, and starting or
resuming requires explicit form confirmation. Multiple task sessions require choosing a
specific session. A historical Run retains its exact target and does not automatically
redirect to a newer execution.

**Filter.** `/` edits a case-insensitive search over name, ID, and subtitle. `Enter`
confirms. `Esc` while editing restores the previous filter and selection; outside editing
it first removes the text filter, then the status filter, and only then returns to the
previous page. The footer shows the available action.

**Help and shortcuts.** All shortcuts come from one centralized `bubbles/key` set. The
footer and full help render only bindings enabled for the current selection; unsupported
`t`, `g`, and `a` are not advertised for entries without a process or action. Full help
scrolls through `bubbles/viewport` and is grouped into **Navigation, View, Runtime,
Actions, Exit**, so every group is reachable at 40×12; the status row shows the scroll
position. On task details, `1`–`3` are shortcuts to related resources, not primary
navigation. The breadcrumb and Results type line (`[Artifacts] Handoffs Checks`) name the
location without relying on color.

## Do's and Don'ts

- Preserve the distinction between task, session, and process state and the number of
  accepted results.
- Keep symbols and labels readable without color and keep the selection and footer
  visible when the terminal is resized.
- Show missing data, refresh errors, and stale snapshots as explicit text.
- Do not start processes merely by opening details or refreshing the view.
- Extend the existing terminal character system; web fonts, raster images, and browser
  components do not belong in this interface.
