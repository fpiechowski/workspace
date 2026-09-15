package tui

import "github.com/charmbracelet/lipgloss"

type palette struct {
	text, muted, border, focus, info, success, warning, danger, selection lipgloss.Color
	noColor                                                               bool
}

func makePalette(theme string, noColor bool) palette {
	if theme == "" || theme == "auto" {
		theme = "dark"
	}
	colors := palette{}
	if noColor {
		colors.noColor = true
		return colors
	}
	if theme == "light" {
		colors.text = lipgloss.Color("#0F172A")
		colors.muted = lipgloss.Color("#475569")
		colors.border = lipgloss.Color("#94A3B8")
		colors.focus = lipgloss.Color("#0E7490")
		colors.info = lipgloss.Color("#0E7490")
		colors.success = lipgloss.Color("#166534")
		colors.warning = lipgloss.Color("#92400E")
		colors.danger = lipgloss.Color("#BE123C")
		colors.selection = lipgloss.Color("#E0F2FE")
		return colors
	}
	colors.text = lipgloss.Color("#E2E8F0")
	colors.muted = lipgloss.Color("#94A3B8")
	colors.border = lipgloss.Color("#475569")
	colors.focus = lipgloss.Color("#67E8F9")
	colors.info = lipgloss.Color("#67E8F9")
	colors.success = lipgloss.Color("#86EFAC")
	colors.warning = lipgloss.Color("#FDE68A")
	colors.danger = lipgloss.Color("#FDA4AF")
	colors.selection = lipgloss.Color("#1E293B")
	return colors
}

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

func (p palette) borderStyle(focused bool) lipgloss.Style {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	if p.noColor {
		return style
	}
	if focused {
		return style.BorderForeground(p.focus)
	}
	return style.BorderForeground(p.border)
}
