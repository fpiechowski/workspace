package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

type workDemo struct{ *Model }

func (m workDemo) Init() tea.Cmd { return m.Model.ensureAnimation() }

// The helper runs the real Bubble Tea input decoder and renderer inside a PTY.
func TestWorkspacePTYHelper(t *testing.T) {
	if os.Getenv("WORKSPACE_TUI_HELPER") != "1" {
		t.Skip("PTY helper")
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	lipgloss.SetHasDarkBackground(true)
	m := workFixture()
	if _, err := tea.NewProgram(workDemo{m}, tea.WithAltScreen()).Run(); err != nil {
		t.Fatal(err)
	}
}

type runtimeDemo struct{ *Model }

func (m runtimeDemo) Init() tea.Cmd { return nil }

// TestRuntimePTYHelper drives the real renderer for the Runtime detail route so
// the tmux test can exercise the wide table and compact stack.
func TestRuntimePTYHelper(t *testing.T) {
	if os.Getenv("WORKSPACE_TUI_HELPER") != "1" {
		t.Skip("PTY helper")
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	lipgloss.SetHasDarkBackground(true)
	m := runtimeFixture()
	if _, err := tea.NewProgram(runtimeDemo{m}, tea.WithAltScreen()).Run(); err != nil {
		t.Fatal(err)
	}
}

// TestRuntimeTableInTmux captures the Runtime route at wide and compact sizes
// and asserts the table/stacked equivalence in a real terminal.
func TestRuntimeTableInTmux(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getenv("WORKSPACE_TMUX_TEST") != "1" {
		t.Skip("requires opt-in Linux/tmux")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("workspace-runtime-%d-%d", os.Getpid(), time.Now().UnixNano())
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v %s", args, err, out)
		}
		return string(out)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	command := "env WORKSPACE_TUI_HELPER=1 " + "'" + strings.ReplaceAll(binary, "'", "'\\''") + "' -test.run '^TestRuntimePTYHelper$'"
	pane := strings.TrimSpace(tmux("new-session", "-d", "-P", "-F", "#{pane_id}", "-s", "runtime", "-x", "120", "-y", "32", command))
	waitFor := func(needle string) string {
		t.Helper()
		var capture string
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			capture = tmux("capture-pane", "-p", "-t", pane)
			if strings.Contains(capture, needle) {
				return capture
			}
			time.Sleep(40 * time.Millisecond)
		}
		t.Fatalf("missing %q in PTY:\n%s", needle, capture)
		return ""
	}
	wide := waitFor("Owner")
	if !strings.Contains(wide, "Pane") || !strings.Contains(wide, "%1") {
		t.Fatalf("wide runtime table is incomplete:\n%s", wide)
	}
	tmux("resize-window", "-t", pane, "-x", "60", "-y", "24")
	compact := waitFor("owner sess_a")
	if strings.Contains(compact, "Owner") {
		t.Fatalf("compact runtime still renders the table header:\n%s", compact)
	}
	if !strings.Contains(compact, "Window @1 · pane %1 · agent") {
		t.Fatalf("compact runtime stack is incomplete:\n%s", compact)
	}
}

func TestNavigationFilterEscapeInTmux(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getenv("WORKSPACE_TMUX_TEST") != "1" {
		t.Skip("requires opt-in Linux/tmux")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("workspace-tui-%d-%d", os.Getpid(), time.Now().UnixNano())
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v %s", args, err, out)
		}
		return string(out)
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	command := "env WORKSPACE_TUI_HELPER=1 " + "'" + strings.ReplaceAll(binary, "'", "'\\''") + "' -test.run '^TestWorkspacePTYHelper$'"
	pane := strings.TrimSpace(tmux("new-session", "-d", "-P", "-F", "#{pane_id}", "-s", "ui", "-x", "120", "-y", "32", command))
	waitFor := func(needle string) string {
		t.Helper()
		var capture string
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			capture = tmux("capture-pane", "-p", "-t", pane)
			if strings.Contains(capture, needle) {
				return capture
			}
			time.Sleep(40 * time.Millisecond)
		}
		t.Fatalf("missing %q in PTY:\n%s", needle, capture)
		return ""
	}
	// The initial workspace route is Tasks.
	waitFor("Retry failed payments")
	// Key 2 opens the unparented Sessions collection; no Tab cycling applies.
	tmux("send-keys", "-t", pane, "2")
	waitFor("Payments worker")
	tmux("send-keys", "-t", pane, "Tab")
	if capture := tmux("capture-pane", "-p", "-t", pane); !strings.Contains(capture, "Payments worker") {
		t.Fatalf("Tab changed the Sessions route:\n%s", capture)
	}
	tmux("send-keys", "-t", pane, "1")
	waitFor("Retry failed payments")
	for _, size := range [][2]int{{40, 12}, {60, 24}, {80, 18}, {100, 24}, {120, 32}} {
		tmux("resize-window", "-t", pane, "-x", fmt.Sprint(size[0]), "-y", fmt.Sprint(size[1]))
		waitFor("1 Tasks")
		capture := waitFor("t terminal")
		if dir := os.Getenv("WORKSPACE_TUI_CAPTURES"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("tasks-%dx%d.txt", size[0], size[1])), []byte(capture), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	tmux("resize-window", "-t", pane, "-x", "120", "-y", "32")
	tmux("send-keys", "-t", pane, "2")
	sessions := waitFor("Payments worker")
	if dir := os.Getenv("WORKSPACE_TUI_CAPTURES"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "sessions-120x32.txt"), []byte(sessions), 0600); err != nil {
			t.Fatal(err)
		}
	}
	tmux("send-keys", "-t", pane, "1")
	waitFor("Retry failed payments")
	tmux("send-keys", "-t", pane, "/")
	waitFor("Esc cancel")
	tmux("send-keys", "-t", pane, "-l", "no-match")
	waitFor("No matches")
	tmux("send-keys", "-t", pane, "Escape")
	waitFor("Retry failed payments")
	// Esc was decoded as an application key: it restored the list and released
	// the filter, allowing the next page shortcut rather than typing into it.
	tmux("send-keys", "-t", pane, "2")
	waitFor("Payments worker")
	tmux("send-keys", "-t", pane, "/")
	tmux("send-keys", "-t", pane, "-l", "refund")
	tmux("send-keys", "-t", pane, "Enter")
	waitFor("Esc clear filter")
	tmux("send-keys", "-t", pane, "Escape")
	waitFor("Payments worker")
}

func TestFilterEscapeWithoutTmux(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getenv("WORKSPACE_TMUX_TEST") != "1" {
		t.Skip("requires Linux PTY")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script unavailable")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	command := "stty cols 100 rows 32; exec env -u TMUX -u TMUX_PANE WORKSPACE_TUI_HELPER=1 TERM=xterm-256color '" + strings.ReplaceAll(binary, "'", "'\\''") + "' -test.run '^TestWorkspacePTYHelper$'"
	cmd := exec.CommandContext(ctx, "script", "-qfec", command, "/dev/null")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer input.Close()
		// Answer the capability queries as an emulator would before user input.
		time.Sleep(250 * time.Millisecond)
		_, _ = input.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R"))
		for _, keys := range []string{"/", "no-match", "\x1b", "2", "\x03"} {
			time.Sleep(250 * time.Millisecond)
			if _, err := input.Write([]byte(keys)); err != nil {
				return
			}
		}
	}()
	out, err := cmd.CombinedOutput()
	<-done
	if err != nil {
		t.Fatalf("PTY: %v %s", err, out)
	}
	if !strings.Contains(string(out), "Persist payment receipts") {
		t.Fatalf("Esc failed to release filter outside tmux: %s", out)
	}
}
