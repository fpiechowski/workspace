package tui

import "github.com/charmbracelet/lipgloss"

// palette holds the semantic visual roles used by the shell renderers. Call
// sites must use these roles (or the style factories below) instead of raw
// colors so that dark, light, and no-color modes stay coherent.
type palette struct {
	noColor bool

	// Text hierarchy.
	primary   lipgloss.Color
	secondary lipgloss.Color
	subtle    lipgloss.Color
	// Chrome.
	border        lipgloss.Color
	focusedBorder lipgloss.Color
	accent        lipgloss.Color
	// Selection.
	selectionFg lipgloss.Color
	selectionBg lipgloss.Color
	// State meaning.
	success lipgloss.Color
	warning lipgloss.Color
	danger  lipgloss.Color
	// Informational surface used by notices and callouts.
	infoSurface lipgloss.Color
}

// detectDarkBackground reports whether the attached terminal uses a dark
// background. It is a variable so tests can pin both answers without reading
// the host terminal. Lip Gloss reports dark whenever it cannot query the
// terminal (pipes, tmux, screen, dumb terminals), and that is the documented
// fallback for --theme auto.
var detectDarkBackground = lipgloss.HasDarkBackground

func darkPalette() palette {
	return palette{
		primary:       lipgloss.Color("#E2E8F0"),
		secondary:     lipgloss.Color("#B4C0D3"),
		subtle:        lipgloss.Color("#7C8CA3"),
		border:        lipgloss.Color("#475569"),
		focusedBorder: lipgloss.Color("#67E8F9"),
		accent:        lipgloss.Color("#67E8F9"),
		selectionFg:   lipgloss.Color("#F8FAFC"),
		selectionBg:   lipgloss.Color("#1E293B"),
		success:       lipgloss.Color("#86EFAC"),
		warning:       lipgloss.Color("#FDE68A"),
		danger:        lipgloss.Color("#FDA4AF"),
		infoSurface:   lipgloss.Color("#164E63"),
	}
}

func lightPalette() palette {
	return palette{
		primary:       lipgloss.Color("#0F172A"),
		secondary:     lipgloss.Color("#334155"),
		subtle:        lipgloss.Color("#64748B"),
		border:        lipgloss.Color("#CBD5E1"),
		focusedBorder: lipgloss.Color("#0E7490"),
		accent:        lipgloss.Color("#0E7490"),
		selectionFg:   lipgloss.Color("#0F172A"),
		selectionBg:   lipgloss.Color("#E0F2FE"),
		success:       lipgloss.Color("#166534"),
		warning:       lipgloss.Color("#92400E"),
		danger:        lipgloss.Color("#BE123C"),
		infoSurface:   lipgloss.Color("#E0F2FE"),
	}
}

// makePalette resolves the requested theme for the current terminal.
func makePalette(theme string, noColor bool) palette {
	return makePaletteFor(theme, noColor, detectDarkBackground())
}

// makePaletteFor resolves a theme against an explicit background answer so that
// tests do not depend on the host terminal. no-color mode carries no colors and
// is fully deterministic; explicit dark and light ignore the detected
// background. Only auto consults the background answer (dark is the fallback).
func makePaletteFor(theme string, noColor bool, darkBackground bool) palette {
	if noColor {
		return palette{noColor: true}
	}
	switch theme {
	case "dark":
		return darkPalette()
	case "light":
		return lightPalette()
	default:
		if darkBackground {
			return darkPalette()
		}
		return lightPalette()
	}
}

// style returns a foreground style that disappears in no-color mode.
func (p palette) style(color lipgloss.Color, bold bool) lipgloss.Style {
	s := lipgloss.NewStyle()
	if !p.noColor {
		s = s.Foreground(color)
	}
	if bold {
		s = s.Bold(true)
	}
	return s
}

// Semantic style factories used by the shell renderers.

// titleStyle renders the identity/status header.
func (p palette) titleStyle() lipgloss.Style { return p.style(p.primary, true) }

// headingStyle renders section, tab, and panel headings.
func (p palette) headingStyle() lipgloss.Style { return p.style(p.accent, true) }

// labelStyle renders field labels.
func (p palette) labelStyle() lipgloss.Style { return p.style(p.secondary, true) }

// metaStyle renders secondary metadata such as subtitles.
func (p palette) metaStyle() lipgloss.Style { return p.style(p.secondary, false) }

// subtleStyle renders provenance IDs and timestamps.
func (p palette) subtleStyle() lipgloss.Style { return p.style(p.subtle, false) }

// keycapStyle renders keyboard shortcuts in the contextual legend.
func (p palette) keycapStyle() lipgloss.Style { return p.style(p.accent, true) }

// noticeStyle renders the status/notice row.
func (p palette) noticeStyle() lipgloss.Style {
	s := lipgloss.NewStyle()
	if !p.noColor {
		s = s.Foreground(p.primary).Background(p.infoSurface)
	}
	return s
}

// selectedStyle renders a focused collection row. Width pads the background so
// the selection spans the row.
func (p palette) selectedStyle(width int) lipgloss.Style {
	s := lipgloss.NewStyle()
	if p.noColor {
		return s
	}
	return s.Foreground(p.selectionFg).Background(p.selectionBg).Width(max(1, width))
}

// panelStyle renders a bordered panel; the focused panel uses the accent border.
func (p palette) panelStyle(focused bool) lipgloss.Style {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	if p.noColor {
		return style
	}
	if focused {
		return style.BorderForeground(p.focusedBorder)
	}
	return style.BorderForeground(p.border)
}

// borderStyle is retained for call sites that only need a rounded border.
func (p palette) borderStyle(focused bool) lipgloss.Style { return p.panelStyle(focused) }
