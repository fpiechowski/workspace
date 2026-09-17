package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// detailDoc assembles a readable detail document from reusable blocks: an
// identity/status title, labeled facts, section headings, bullets, warning
// callouts, related-resource links and subtle provenance. Every external value
// is sanitized before it is styled and wrapped to the document width, so long
// goals, instructions, summaries, reasons, command lines, paths and change
// request bodies never overflow the viewport.
type detailDoc struct {
	palette palette
	width   int
	lines   []string
}

func newDetailDoc(p palette, width int) *detailDoc {
	if width < 1 {
		width = 1
	}
	return &detailDoc{palette: p, width: width}
}

// title writes the identity block: a section label, the resource name and its
// status badge. It is always the first block of a document.
func (d *detailDoc) title(label, name, state string) {
	label = sanitizeLine(label)
	name = sanitizeLine(name)
	head := d.palette.headingStyle().Render(label)
	labelWidth := ansi.StringWidth(label) + 2
	if name == "" {
		d.lines = append(d.lines, head)
	} else if labelWidth >= d.width {
		d.lines = append(d.lines, head)
		for _, line := range wrapPlain(name, d.width) {
			d.lines = append(d.lines, d.palette.titleStyle().Render(line))
		}
	} else {
		nameLines := wrapPlain(name, max(1, d.width-labelWidth))
		d.lines = append(d.lines, head+"  "+d.palette.titleStyle().Render(nameLines[0]))
		indent := strings.Repeat(" ", labelWidth)
		for _, line := range nameLines[1:] {
			d.lines = append(d.lines, indent+d.palette.titleStyle().Render(line))
		}
	}
	if state = sanitizeLine(state); state != "" {
		d.lines = append(d.lines, d.palette.headingStyle().Render(statusBadge(state)))
	}
}

// action renders an available command hint so the action stays discoverable
// next to the resource it applies to.
func (d *detailDoc) action(value string) {
	value = sanitizeLine(value)
	for _, line := range wrapPlain(value, d.width) {
		d.lines = append(d.lines, d.palette.keycapStyle().Render(line))
	}
}

// link renders a related-resource shortcut line.
func (d *detailDoc) link(value string) {
	value = sanitizeLine(value)
	for _, line := range wrapPlain(value, d.width) {
		d.lines = append(d.lines, d.palette.metaStyle().Render(line))
	}
}

// section starts a new section with a heading, separated by one blank line from
// the previous block.
func (d *detailDoc) section(title string) {
	d.blank()
	d.lines = append(d.lines, d.palette.sectionStyle().Render(sanitizeLine(title)))
}

// field writes an inline "Label: value" fact. The label is never split; the
// value wraps to the remaining width, and unbreakable tokens hard-wrap.
func (d *detailDoc) field(label, value string) {
	d.labeled(label, value, d.palette.labelStyle(), d.palette.valueStyle())
}

// provenance writes a subtle identifier, digest or timestamp. Provenance stays
// readable but visually secondary and is rendered last.
func (d *detailDoc) provenance(label, value string) {
	d.labeled(label, value, d.palette.subtleStyle(), d.palette.subtleStyle())
}

// labeled renders one label/value pair. When the label would consume too much of
// the width it moves to its own line and the value is indented underneath.
func (d *detailDoc) labeled(label, value string, labelStyle, valueStyle lipgloss.Style) {
	label = sanitizeLine(label)
	value = sanitizeLine(value)
	if value == "" {
		value = "—"
	}
	head := label + ":"
	prefix := head + " "
	prefixWidth := ansi.StringWidth(prefix)
	if prefixWidth > d.width || prefixWidth > max(8, d.width/3) {
		for _, line := range wrapPlain(head, d.width) {
			d.lines = append(d.lines, labelStyle.Render(line))
		}
		for _, line := range wrapPlain(value, max(1, d.width-2)) {
			d.lines = append(d.lines, "  "+valueStyle.Render(line))
		}
		return
	}
	valueLines := wrapPlain(value, max(1, d.width-prefixWidth))
	d.lines = append(d.lines, labelStyle.Render(prefix)+valueStyle.Render(valueLines[0]))
	indent := strings.Repeat(" ", prefixWidth)
	for _, line := range valueLines[1:] {
		d.lines = append(d.lines, indent+valueStyle.Render(line))
	}
}

// bullets writes a list. An empty list renders a placeholder instead of
// disappearing.
func (d *detailDoc) bullets(values []string) {
	if len(values) == 0 {
		d.lines = append(d.lines, d.palette.subtleStyle().Render("  —"))
		return
	}
	for _, value := range values {
		d.bullet(value)
	}
}

// bullet writes one list item with a hanging indent.
func (d *detailDoc) bullet(value string) {
	value = sanitizeLine(value)
	const marker = "  · "
	valueLines := wrapPlain(value, max(1, d.width-ansi.StringWidth(marker)))
	d.lines = append(d.lines, d.palette.subtleStyle().Render(marker)+d.palette.valueStyle().Render(valueLines[0]))
	indent := strings.Repeat(" ", ansi.StringWidth(marker))
	for _, line := range valueLines[1:] {
		d.lines = append(d.lines, indent+d.palette.valueStyle().Render(line))
	}
}

// warning writes a callout for binary, truncated, stale or blocked content.
func (d *detailDoc) warning(value string) {
	value = sanitizeLine(value)
	const marker = "[!] "
	valueLines := wrapPlain(value, max(1, d.width-ansi.StringWidth(marker)))
	d.lines = append(d.lines, d.palette.warningStyle().Render(marker+valueLines[0]))
	indent := strings.Repeat(" ", ansi.StringWidth(marker))
	for _, line := range valueLines[1:] {
		d.lines = append(d.lines, indent+d.palette.warningStyle().Render(line))
	}
}

// body writes narrative text, preserving explicit newlines and wrapping each
// physical line to the document width.
func (d *detailDoc) body(value string) {
	value = sanitize(value)
	if value == "" {
		return
	}
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) == "" {
			d.lines = append(d.lines, "")
			continue
		}
		for _, wrapped := range wrapPlain(line, d.width) {
			d.lines = append(d.lines, d.palette.valueStyle().Render(wrapped))
		}
	}
}

// raw appends already-rendered lines such as the runtime table. Callers must
// sanitize their contents.
func (d *detailDoc) raw(lines ...string) {
	d.lines = append(d.lines, lines...)
}

func (d *detailDoc) blank() {
	if len(d.lines) == 0 || d.lines[len(d.lines)-1] == "" {
		return
	}
	d.lines = append(d.lines, "")
}

func (d *detailDoc) render() string {
	return strings.TrimRight(strings.Join(d.lines, "\n"), "\n")
}

// wrapPlain wraps one line of plain text to width, hard-breaking unbreakable
// tokens so no line can overflow. It is ANSI-aware and counts wide glyphs.
func wrapPlain(value string, width int) []string {
	if width < 1 {
		width = 1
	}
	if value == "" {
		return []string{""}
	}
	return strings.Split(ansi.Wrap(value, width, ""), "\n")
}
