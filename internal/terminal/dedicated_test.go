package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"workspace/internal/core"
)

type fakeDedicatedRunner struct {
	target       core.NavigationTarget
	viewer       string
	viewerExists bool
	clients      []string
	canonical    bool
	commands     [][]string
	runs         [][]string
}

func newFakeDedicatedRunner(target core.NavigationTarget) *fakeDedicatedRunner {
	return &fakeDedicatedRunner{target: target, viewer: dedicatedViewerSessionName(target), canonical: true}
}

func (r *fakeDedicatedRunner) Output(_ context.Context, socket string, args ...string) (string, error) {
	command := append([]string{socket}, args...)
	r.commands = append(r.commands, command)
	if len(args) == 0 {
		return "", nil
	}
	target := argumentAfter(args, "-t")
	switch args[0] {
	case "has-session":
		if target == "="+r.target.SessionName && !r.canonical {
			return "", errors.New("canonical session missing")
		}
		if target == "="+r.viewer && !r.viewerExists {
			return "", errors.New("viewer missing")
		}
		return "", nil
	case "new-session":
		r.viewerExists = true
		return r.viewer, nil
	case "list-sessions":
		if !r.canonical {
			return "", errors.New("canonical session missing")
		}
		rows := r.target.SessionName + "\t"
		if r.viewerExists {
			rows += r.target.SessionName + "\n" + r.viewer + "\t" + r.target.SessionName
		}
		return rows, nil
	case "display-message":
		if target == "="+r.target.SessionName && !r.canonical {
			return "", errors.New("canonical session missing")
		}
		if target == "="+r.viewer && !r.viewerExists {
			return "", errors.New("viewer missing")
		}
		return r.target.SessionName, nil
	case "list-windows":
		if !r.targetExists(target) {
			return "", errors.New("target missing")
		}
		return r.target.WindowID, nil
	case "list-panes":
		if !r.targetExists(target) {
			return "", errors.New("target missing")
		}
		if strings.Contains(strings.Join(args, " "), "#{pane_dead}") && strings.Contains(strings.Join(args, " "), "#{window_id}") && !strings.Contains(strings.Join(args, " "), "#{@workspace_id}") {
			return r.target.PaneID + "\t" + r.target.WindowID + "\t0", nil
		}
		return r.target.PaneID + "\t" + r.target.WindowID + "\t0\t" + r.target.WorkspaceID + "\tagent\t" + r.target.SessionID + "\t" + r.target.RunID, nil
	case "list-clients":
		return strings.Join(r.clients, "\n"), nil
	case "select-window", "select-pane", "switch-client":
		return "", nil
	default:
		return "", fmt.Errorf("unexpected tmux command %q", args[0])
	}
}

func (r *fakeDedicatedRunner) Run(_ context.Context, _ io.Reader, _, _ io.Writer, socket string, args ...string) error {
	r.runs = append(r.runs, append([]string{socket}, args...))
	return nil
}

func (r *fakeDedicatedRunner) targetExists(target string) bool {
	if !r.canonical && target == "="+r.target.SessionName {
		return false
	}
	return target == "="+r.target.SessionName || target == "="+r.viewer || strings.HasPrefix(target, "="+r.viewer+":")
}

func argumentAfter(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

type fakeDedicatedLauncher struct {
	calls []LaunchSpec
	err   error
}

func (l *fakeDedicatedLauncher) Launch(_ context.Context, spec LaunchSpec) error {
	l.calls = append(l.calls, spec)
	return l.err
}

func dedicatedTestTarget() core.NavigationTarget {
	return core.NavigationTarget{
		WorkspaceID: "ws_a", SessionName: "workspace-ws_a", Socket: "private",
		WindowID: "@1", PaneID: "%1", Kind: "session", SessionID: "sess_a", RunID: "run_a",
	}
}

func dedicatedNavigator(runner *fakeDedicatedRunner, launcher Launcher) *TmuxNavigator {
	return &TmuxNavigator{Socket: "private", Runner: runner, Launcher: launcher, Env: func(string) string { return "" }}
}

func TestOpenDedicatedCreatesViewerSelectsVerifiedTargetAndLaunchesOnce(t *testing.T) {
	target := dedicatedTestTarget()
	runner := newFakeDedicatedRunner(target)
	launcher := &fakeDedicatedLauncher{}
	navigator := dedicatedNavigator(runner, launcher)

	if err := navigator.OpenDedicated(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if !runner.viewerExists {
		t.Fatal("dedicated viewer was not created")
	}
	if len(launcher.calls) != 1 || launcher.calls[0].Session != runner.viewer || launcher.calls[0].Socket != target.Socket {
		t.Fatalf("launcher calls = %#v", launcher.calls)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("dedicated navigation used caller stdio: %#v", runner.runs)
	}
	if !hasCommand(runner.commands, "new-session", "-t", "="+target.SessionName) ||
		!hasCommand(runner.commands, "select-window", "-t", "="+runner.viewer+":"+target.WindowID) ||
		!hasCommand(runner.commands, "select-pane", "-t", target.PaneID) {
		t.Fatalf("viewer setup did not use separate verified argv: %#v", runner.commands)
	}
}

func TestOpenDedicatedReusesOneAttachedViewerClient(t *testing.T) {
	target := dedicatedTestTarget()
	runner := newFakeDedicatedRunner(target)
	launcher := &fakeDedicatedLauncher{}
	navigator := dedicatedNavigator(runner, launcher)
	if err := navigator.OpenDedicated(context.Background(), target); err != nil {
		t.Fatal(err)
	}

	runner.target = target
	runner.clients = []string{"/dev/pts/viewer"}
	if err := navigator.OpenDedicated(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("reused viewer launched %d terminals", len(launcher.calls))
	}
	if !hasCommand(runner.commands, "switch-client", "-c", "/dev/pts/viewer", "-t", "="+runner.viewer) {
		t.Fatalf("attached viewer client was not selected: %#v", runner.commands)
	}
}

func TestOpenDedicatedMovesTheReusedViewerToAnotherVerifiedTarget(t *testing.T) {
	first := dedicatedTestTarget()
	runner := newFakeDedicatedRunner(first)
	launcher := &fakeDedicatedLauncher{}
	navigator := dedicatedNavigator(runner, launcher)
	if err := navigator.OpenDedicated(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.WindowID, second.PaneID, second.RunID = "@2", "%2", "run_b"
	runner.target = second
	runner.clients = []string{"/dev/pts/viewer"}
	if err := navigator.OpenDedicated(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("reused viewer launched %d terminals", len(launcher.calls))
	}
	if !hasCommand(runner.commands, "select-window", "-t", "="+runner.viewer+":"+second.WindowID) || !hasCommand(runner.commands, "select-pane", "-t", second.PaneID) {
		t.Fatalf("viewer did not move to the new target: %#v", runner.commands)
	}
}

func TestOpenDedicatedRejectsAmbiguousViewerClients(t *testing.T) {
	target := dedicatedTestTarget()
	runner := newFakeDedicatedRunner(target)
	runner.viewerExists = true
	runner.clients = []string{"/dev/pts/a", "/dev/pts/b"}
	launcher := &fakeDedicatedLauncher{}
	navigator := dedicatedNavigator(runner, launcher)
	err := navigator.OpenDedicated(context.Background(), target)
	if !hasCoreCode(err, "navigation_ambiguous") {
		t.Fatalf("ambiguous viewer error = %v", err)
	}
	if len(launcher.calls) != 0 || hasCommand(runner.commands, "switch-client") {
		t.Fatalf("ambiguous viewer caused an effect: launches=%#v commands=%#v", launcher.calls, runner.commands)
	}
}

func TestOpenDedicatedRefusesToSwitchWhenCurrentPaneIsUnknown(t *testing.T) {
	target := dedicatedTestTarget()
	runner := newFakeDedicatedRunner(target)
	runner.viewerExists = true
	runner.clients = []string{"/dev/pts/viewer"}
	navigator := dedicatedNavigator(runner, &fakeDedicatedLauncher{})
	navigator.Env = func(key string) string {
		if key == "TMUX" {
			return "/tmp/tmux-1000/private,1,0"
		}
		return ""
	}

	err := navigator.OpenDedicated(context.Background(), target)
	if !hasCoreCode(err, "dedicated_client_ambiguous") {
		t.Fatalf("unknown current pane error = %v", err)
	}
	if hasCommand(runner.commands, "switch-client") {
		t.Fatalf("dedicated viewer was selected despite unknown current pane: %#v", runner.commands)
	}
}

func TestOpenDedicatedMissingOrFailingLauncherNeverUsesCallerStdio(t *testing.T) {
	for _, launcher := range []Launcher{nil, &fakeDedicatedLauncher{err: errors.New("launcher failed")}} {
		t.Run(fmt.Sprintf("%T", launcher), func(t *testing.T) {
			target := dedicatedTestTarget()
			runner := newFakeDedicatedRunner(target)
			navigator := dedicatedNavigator(runner, launcher)
			err := navigator.OpenDedicated(context.Background(), target)
			if err == nil {
				t.Fatal("missing or failing launcher unexpectedly succeeded")
			}
			if len(runner.runs) != 0 {
				t.Fatalf("dedicated failure used caller stdio: %#v", runner.runs)
			}
			if launcher != nil && !hasCoreCode(err, "terminal_launch_failed") {
				t.Fatalf("launch failure = %v", err)
			}
		})
	}
}

func TestOpenDedicatedVerificationFailureHasNoViewerEffect(t *testing.T) {
	target := dedicatedTestTarget()
	runner := newFakeDedicatedRunner(target)
	runner.canonical = false
	launcher := &fakeDedicatedLauncher{}
	navigator := dedicatedNavigator(runner, launcher)
	err := navigator.OpenDedicated(context.Background(), target)
	if !hasCoreCode(err, "pane_missing") {
		t.Fatalf("verification error = %v", err)
	}
	if runner.viewerExists || len(launcher.calls) != 0 || len(runner.runs) != 0 || hasCommand(runner.commands, "new-session") || hasCommand(runner.commands, "select-window") || hasCommand(runner.commands, "select-pane") {
		t.Fatalf("verification failure caused a viewer effect: viewer=%t launches=%#v commands=%#v", runner.viewerExists, launcher.calls, runner.commands)
	}
}

func hasCommand(commands [][]string, name string, pairs ...string) bool {
	for _, command := range commands {
		if len(command) < 2 || command[1] != name {
			continue
		}
		found := true
		for i := 0; i+1 < len(pairs); i += 2 {
			if !containsAdjacent(command[1:], pairs[i], pairs[i+1]) {
				found = false
				break
			}
		}
		if found {
			return true
		}
	}
	return false
}

func containsAdjacent(values []string, first, second string) bool {
	for i := 0; i+1 < len(values); i++ {
		if values[i] == first && values[i+1] == second {
			return true
		}
	}
	return false
}
