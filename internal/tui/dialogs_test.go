package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"
)

func destructiveDialog(t *testing.T) *Model {
	t.Helper()
	m := workFixture()
	m.backend = &actionHarness{}
	if cmd := m.beginAction("delete_workspace", "ws_demo"); cmd == nil {
		t.Fatal("workspace deletion did not open a guarded form")
	}
	if m.form == nil || m.formMode != "destructive" {
		t.Fatalf("expected a destructive form, got mode=%q form=%v", m.formMode, m.form)
	}
	return m
}

func lineIndex(lines []string, needle string) int {
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), needle) {
			return i
		}
	}
	return -1
}

// TestDialogsAreCenteredAndBoundedOnLargeTerminals covers the wide breakpoint:
// the destructive dialog is centered, bounded, and still shows every header
// affordance.
func TestDialogsAreCenteredAndBoundedOnLargeTerminals(t *testing.T) {
	m := destructiveDialog(t)
	m.width, m.height = 120, 32
	m.rebuildViewport()
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 32 {
		t.Fatalf("dialog frame rendered %d rows, want 32", len(lines))
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width > 120 {
			t.Fatalf("dialog line %d rendered %d columns: %q", i, width, line)
		}
	}
	if got := m.dialogWidth(); got != dialogMaxWidth {
		t.Fatalf("wide dialog width = %d, want bounded %d", got, dialogMaxWidth)
	}
	title := lineIndex(lines, "Destructive confirmation")
	if title < 0 {
		t.Fatalf("dialog title missing:\n%s", view)
	}
	if !strings.HasPrefix(ansi.Strip(lines[title]), " ") {
		t.Fatalf("wide dialog is not centered: %q", lines[title])
	}
	for _, needle := range []string{"[!] Destructive", "Target", "ws_demo", "Esc cancels"} {
		if lineIndex(lines, needle) < 0 {
			t.Fatalf("wide dialog missing %q:\n%s", needle, view)
		}
	}
	if title != lineIndex(lines, "[!] Destructive")-1 {
		t.Fatalf("severity is not directly below the title:\n%s", view)
	}
}

// TestDialogsUseFullBodyAtCompactSizes proves compact terminals keep the
// unbounded full-body form rather than a padded dialog box.
func TestDialogsUseFullBodyAtCompactSizes(t *testing.T) {
	m := destructiveDialog(t)
	m.width, m.height = 60, 24
	m.rebuildViewport()
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Fatalf("compact dialog frame rendered %d rows, want 24", len(lines))
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width > 60 {
			t.Fatalf("compact dialog line %d rendered %d columns: %q", i, width, line)
		}
	}
	title := lineIndex(lines, "Destructive confirmation")
	if title < 0 {
		t.Fatalf("compact dialog title missing:\n%s", view)
	}
	if !strings.HasPrefix(ansi.Strip(lines[title]), "Destructive confirmation") {
		t.Fatalf("compact dialog is padded instead of full-body: %q", lines[title])
	}
}

// TestDialogHeaderFitsMinimumViewport checks the title, target, severity and Esc
// cancellation remain visible at the supported minimum size.
func TestDialogHeaderFitsMinimumViewport(t *testing.T) {
	m := destructiveDialog(t)
	m.width, m.height = 40, 12
	m.rebuildViewport()
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 12 {
		t.Fatalf("minimum dialog frame rendered %d rows, want 12", len(lines))
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width > 40 {
			t.Fatalf("minimum dialog line %d rendered %d columns: %q", i, width, line)
		}
	}
	for _, needle := range []string{"Destructive confirmation", "[!] Destructive", "Target", "ws_demo", "Esc cancels"} {
		if !strings.Contains(ansi.Strip(view), needle) {
			t.Fatalf("minimum dialog missing %q:\n%s", needle, view)
		}
	}
}

// TestDestructiveDialogKeepsTypedIDValidation proves the exact-ID guard still
// gates the form after dialog presentation changes.
func TestDestructiveDialogKeepsTypedIDValidation(t *testing.T) {
	m := destructiveDialog(t)
	if m.formAction.TargetID != "ws_demo" {
		t.Fatalf("guard lost its target ID: %+v", m.formAction)
	}
	rendered := m.form.View()
	if !strings.Contains(rendered, "Type the exact ID to confirm permanent removal") || !strings.Contains(rendered, "ws_demo") {
		t.Fatalf("destructive form lost its exact-ID prompt: %q", rendered)
	}

	typeRunes := func(model *Model, value string) {
		for _, r := range value {
			_, _ = model.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}
	submit := func(model *Model) {
		_, cmd := model.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
		for i := 0; model.form != nil && cmd != nil && i < 8; i++ {
			msg := cmd()
			if msg == nil {
				break
			}
			_, cmd = model.updateForm(msg)
		}
	}

	typeRunes(m, "wrong")
	submit(m)
	if m.form == nil || m.form.State == huh.StateCompleted {
		t.Fatal("an inexact ID completed the destructive form")
	}
	if m.actionPending {
		t.Fatal("an inexact ID started the destructive action")
	}

	accepted := destructiveDialog(t)
	typeRunes(accepted, "ws_demo")
	submit(accepted)
	if accepted.form != nil || !accepted.actionPending {
		t.Fatal("the exact ID did not complete the destructive form")
	}
}

// TestFormsRemainModal shows no global shortcut escapes while a form owns input.
func TestFormsRemainModal(t *testing.T) {
	m := workFixture()
	m.backend = &actionHarness{}
	m.navigate(route{Page: "orchestrator"})
	if cmd := m.beginAction("start_orchestrator", ""); cmd == nil {
		t.Fatal("confirmation form did not open")
	}
	if m.form == nil || m.formMode != "confirm" {
		t.Fatalf("expected a confirm form, got mode=%q form=%v", m.formMode, m.form)
	}
	page := m.route.Page
	for _, key := range []rune{'2', '5', 'q', '?', 'g', 'r', 'w'} {
		_, _ = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		if m.route.Page != page {
			t.Fatalf("shortcut %q navigated away from the modal form", key)
		}
		if m.quit {
			t.Fatalf("shortcut %q quit while a form owned input", key)
		}
		if m.showHelp {
			t.Fatalf("shortcut %q opened help while a form owned input", key)
		}
		if m.form == nil {
			t.Fatalf("shortcut %q closed the modal form", key)
		}
	}
}

// TestNoColorDialogMarksSelectedButton proves the primary button keeps an
// explicit text marker without color.
func TestNoColorDialogMarksSelectedButton(t *testing.T) {
	theme := huhTheme(palette{noColor: true})
	if selected := theme.Focused.FocusedButton.Render("Confirm"); !strings.Contains(selected, "> Confirm") {
		t.Fatalf("selected button has no text marker: %q", selected)
	}
	if blurred := theme.Focused.BlurredButton.Render("Cancel"); strings.Contains(blurred, "> ") {
		t.Fatalf("unselected button was marked as selected: %q", blurred)
	}
}

// TestActionSeverityClassifiesGuards keeps destructive and review wording
// distinct.
func TestActionSeverityClassifiesGuards(t *testing.T) {
	if got := actionSeverity("delete_workspace"); got != severityDanger {
		t.Fatalf("delete_workspace severity = %v, want danger", got)
	}
	if got := actionSeverity("pause_interrupt"); got != severityDanger {
		t.Fatalf("pause_interrupt severity = %v, want danger", got)
	}
	if got := actionSeverity("archive_workspace"); got != severityWarning {
		t.Fatalf("archive_workspace severity = %v, want warning", got)
	}
	if got := actionSeverity("select_workflow"); got != severityNeutral {
		t.Fatalf("select_workflow severity = %v, want neutral", got)
	}
}
