package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPaletteModesAreDeterministic(t *testing.T) {
	dark, light, plain := darkPalette(), lightPalette(), palette{noColor: true}
	cases := []struct {
		name       string
		theme      string
		noColor    bool
		background bool
		want       palette
	}{
		{name: "explicit dark on dark background", theme: "dark", background: true, want: dark},
		{name: "explicit dark on light background", theme: "dark", background: false, want: dark},
		{name: "explicit light on dark background", theme: "light", background: true, want: light},
		{name: "explicit light on light background", theme: "light", background: false, want: light},
		{name: "auto on dark background", theme: "auto", background: true, want: dark},
		{name: "auto on light background", theme: "auto", background: false, want: light},
		{name: "empty theme falls back to detection", theme: "", background: false, want: light},
		{name: "no-color ignores dark background", theme: "auto", noColor: true, background: true, want: plain},
		{name: "no-color with explicit light", theme: "light", noColor: true, background: false, want: plain},
		{name: "no-color with explicit dark", theme: "dark", noColor: true, background: true, want: plain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := makePaletteFor(tc.theme, tc.noColor, tc.background); got != tc.want {
				t.Fatalf("makePaletteFor(%q, %t, %t) = %+v, want %+v", tc.theme, tc.noColor, tc.background, got, tc.want)
			}
		})
	}
}

// TestAutoThemeUsesDetectionSeam proves auto consults terminal detection and
// stays testable without reading the host terminal.
func TestAutoThemeUsesDetectionSeam(t *testing.T) {
	original := detectDarkBackground
	t.Cleanup(func() { detectDarkBackground = original })

	detectDarkBackground = func() bool { return false }
	if got := makePalette("auto", false); got != lightPalette() {
		t.Fatalf("auto reported a light background but selected %+v", got)
	}
	if got := makePalette("", false); got != lightPalette() {
		t.Fatalf("empty theme reported a light background but selected %+v", got)
	}

	detectDarkBackground = func() bool { return true }
	if got := makePalette("auto", false); got != darkPalette() {
		t.Fatalf("auto reported a dark background but selected %+v", got)
	}

	detectDarkBackground = func() bool { return false }
	if got := makePalette("dark", false); got != darkPalette() {
		t.Fatalf("explicit dark ignored the requested theme: %+v", got)
	}
	if got := makePalette("light", false); got != lightPalette() {
		t.Fatalf("explicit light ignored the requested theme: %+v", got)
	}
	if got := makePalette("auto", true); got != (palette{noColor: true}) {
		t.Fatalf("no-color palette must not carry colors: %+v", got)
	}
}

func TestPaletteExposesSemanticRoles(t *testing.T) {
	for name, p := range map[string]palette{"dark": darkPalette(), "light": lightPalette()} {
		roles := map[string]lipgloss.Color{
			"primary": p.primary, "secondary": p.secondary, "subtle": p.subtle,
			"border": p.border, "focusedBorder": p.focusedBorder, "accent": p.accent,
			"selectionFg": p.selectionFg, "selectionBg": p.selectionBg,
			"success": p.success, "warning": p.warning, "danger": p.danger,
			"infoSurface": p.infoSurface,
		}
		for role, color := range roles {
			if color == "" {
				t.Fatalf("%s palette is missing the %s role", name, role)
			}
		}
		if p.border == p.focusedBorder {
			t.Fatalf("%s palette must distinguish border from focused border", name)
		}
		if p.accent == p.primary {
			t.Fatalf("%s palette must distinguish accent from primary text", name)
		}
		if p.selectionFg == p.selectionBg {
			t.Fatalf("%s palette must distinguish selection foreground from background", name)
		}
	}
}
