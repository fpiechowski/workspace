package terminal

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"workspace/internal/core"
)

type fakeCommandRunner struct {
	outputs   map[string]string
	errors    map[string]error
	sequences map[string][]string
	commands  [][]string
	runs      [][]string
}

func (r *fakeCommandRunner) Output(_ context.Context, socket string, args ...string) (string, error) {
	command := append([]string{socket}, args...)
	r.commands = append(r.commands, command)
	if len(args) > 0 {
		key := strings.Join(args, " ")
		if err := r.errors[key]; err != nil {
			return "", err
		}
		if values := r.sequences[key]; len(values) > 0 {
			r.sequences[key] = values[1:]
			return values[0], nil
		}
		return r.outputs[key], nil
	}
	return "", nil
}

func clientFixtureRow(tty, session, sessionID, windowID, paneID, pid, name, term, created string) string {
	return strings.Join([]string{tty, session, sessionID, windowID, paneID, pid, name, term, created}, "\t")
}

func (r *fakeCommandRunner) Run(_ context.Context, _ io.Reader, _, _ io.Writer, socket string, args ...string) error {
	r.runs = append(r.runs, append([]string{socket}, args...))
	return nil
}

func testTarget() core.NavigationTarget {
	return core.NavigationTarget{WorkspaceID: "ws_a", SessionName: "workspace-ws_a", Socket: "private", WindowID: "@1", PaneID: "%1", Kind: "session", SessionID: "sess_a", RunID: "run_a"}
}

func TestNavigatorAttachUsesVerifiedIDsAndCallerStreams(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string]string{
		"has-session -t =workspace-ws_a":                  "",
		"list-windows -t =workspace-ws_a -F #{window_id}": "@1",
		"list-panes -a -t =workspace-ws_a -F #{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_id}\t#{@workspace_kind}\t#{@workspace_session_id}\t#{@workspace_run_id}": "%1\t@1\t0\tws_a\tagent\tsess_a\trun_a",
	}}
	n := &TmuxNavigator{Socket: "private", Runner: runner, Env: func(k string) string { return "" }}
	if err := n.Attach(context.Background(), testTarget()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 1 || strings.Join(runner.runs[0][1:], " ") != "attach-session -t =workspace-ws_a" {
		t.Fatalf("unexpected interactive command: %+v", runner.runs)
	}
	if len(runner.commands) != 5 || strings.Join(runner.commands[3][1:], " ") != "select-window -t @1" || strings.Join(runner.commands[4][1:], " ") != "select-pane -t %1" {
		t.Fatalf("target was not selected by verified IDs: %+v", runner.commands)
	}
}

func TestNavigatorRejectsCrossServerAndSelectsExplicitClient(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string]string{
		"has-session -t =workspace-ws_a":                  "",
		"list-windows -t =workspace-ws_a -F #{window_id}": "@1",
		"list-panes -a -t =workspace-ws_a -F #{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_id}\t#{@workspace_kind}\t#{@workspace_session_id}\t#{@workspace_run_id}": "%1\t@1\t0\tws_a\tagent\tsess_a\trun_a",
		"list-clients -F #{client_tty}":               "/dev/pts/3",
		"display-message -p -c /dev/pts/3 #{pane_id}": "%9",
	}}
	env := map[string]string{"TMUX": "/tmp/tmux-1000/private,11,0", "TMUX_PANE": "%9"}
	n := &TmuxNavigator{Socket: "private", Runner: runner, Env: func(k string) string { return env[k] }}
	if err := n.Select(context.Background(), testTarget()); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) < 6 || strings.Join(runner.commands[5][1:], " ") != "switch-client -c /dev/pts/3 -t =workspace-ws_a" {
		t.Fatalf("did not select the client attached to the current pane: %+v", runner.commands)
	}
	env["TMUX"] = "/tmp/tmux-1000/other,12,0"
	if err := n.Select(context.Background(), testTarget()); err == nil {
		t.Fatal("cross-server navigation unexpectedly succeeded")
	} else {
		var ce *core.Error
		if !errors.As(err, &ce) || ce.Code != "tmux_server_mismatch" {
			t.Fatalf("unexpected cross-server error: %v", err)
		}
	}
}

func TestNavigatorListsClientsAndJumpsExplicitlyChosenClientOutsideTmux(t *testing.T) {
	rows := strings.Join([]string{
		clientFixtureRow("/dev/pts/2", "workspace-one", "$1", "@1", "%1", "100", "terminal-a", "xterm", "10"),
		clientFixtureRow("/dev/pts/7", "workspace-two", "$2", "@2", "%2", "200", "terminal-b", "xterm-256color", "20"),
	}, "\n")
	runner := &fakeCommandRunner{outputs: map[string]string{
		"has-session -t =workspace-ws_a":                  "",
		"list-windows -t =workspace-ws_a -F #{window_id}": "@1",
		"list-panes -a -t =workspace-ws_a -F #{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_id}\t#{@workspace_kind}\t#{@workspace_session_id}\t#{@workspace_run_id}": "%1\t@1\t0\tws_a\tagent\tsess_a\trun_a",
		"list-clients -F " + clientFormat: rows,
	}}
	navigator := &TmuxNavigator{Socket: "private", Runner: runner, Env: func(string) string { return "" }}
	clients, err := navigator.ListClients(context.Background(), testTarget())
	if err != nil || len(clients) != 2 {
		t.Fatalf("ListClients = %+v, %v", clients, err)
	}
	chosen := clients[1]
	if err := navigator.Jump(context.Background(), testTarget(), chosen); err != nil {
		t.Fatal(err)
	}
	var switchCommand, windowCommand, paneCommand []string
	for _, command := range runner.commands {
		joined := strings.Join(command[1:], " ")
		switch {
		case strings.HasPrefix(joined, "switch-client "):
			switchCommand = command[1:]
		case strings.HasPrefix(joined, "select-window "):
			windowCommand = command[1:]
		case strings.HasPrefix(joined, "select-pane "):
			paneCommand = command[1:]
		}
	}
	if strings.Join(switchCommand, " ") != "switch-client -c /dev/pts/7 -t =workspace-ws_a" {
		t.Fatalf("jump did not use the chosen explicit client: %v", switchCommand)
	}
	if strings.Join(windowCommand, " ") != "select-window -t =workspace-ws_a:@1" || strings.Join(paneCommand, " ") != "select-pane -t %1" {
		t.Fatalf("jump did not select exact verified IDs: window=%v pane=%v", windowCommand, paneCommand)
	}
	for _, command := range runner.commands {
		if strings.Contains(strings.Join(command, " "), "attach-session") {
			t.Fatalf("explicit TUI jump unexpectedly attached caller terminal: %v", command)
		}
	}
}

func TestNavigatorRejectsReusedTTYOrChangedClientBeforeSwitch(t *testing.T) {
	base := map[string]string{
		"has-session -t =workspace-ws_a":                  "",
		"list-windows -t =workspace-ws_a -F #{window_id}": "@1",
		"list-panes -a -t =workspace-ws_a -F #{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_id}\t#{@workspace_kind}\t#{@workspace_session_id}\t#{@workspace_run_id}": "%1\t@1\t0\tws_a\tagent\tsess_a\trun_a",
	}
	clientKey := "list-clients -F " + clientFormat
	chosen := Client{TTY: "/dev/pts/4", Session: "workspace", SessionID: "$1", WindowID: "@1", PaneID: "%1", PID: "100", Name: "xterm", Created: "10"}
	for _, tc := range []struct {
		name string
		live string
		code string
	}{
		{name: "TTY reused by a restarted process", live: clientFixtureRow("/dev/pts/4", "workspace", "$1", "@1", "%1", "101", "xterm", "xterm", "11"), code: "client_gone"},
		{name: "same client changed session while picker was open", live: clientFixtureRow("/dev/pts/4", "different", "$2", "@9", "%9", "100", "xterm", "xterm", "10"), code: "client_changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeCommandRunner{outputs: base, sequences: map[string][]string{clientKey: {clientFixtureRow(chosen.TTY, chosen.Session, chosen.SessionID, chosen.WindowID, chosen.PaneID, chosen.PID, chosen.Name, "xterm", chosen.Created), tc.live}}}
			navigator := &TmuxNavigator{Socket: "private", Runner: runner, Env: func(string) string { return "" }}
			err := navigator.Jump(context.Background(), testTarget(), chosen)
			expectNavigationCode(t, err, tc.code)
			for _, command := range runner.commands {
				if strings.HasPrefix(strings.Join(command[1:], " "), "switch-client ") {
					t.Fatal("changed client was switched")
				}
			}
		})
	}
}

func TestNavigatorReportsNoClientsAndDiscoveryFailure(t *testing.T) {
	base := map[string]string{
		"has-session -t =workspace-ws_a":                  "",
		"list-windows -t =workspace-ws_a -F #{window_id}": "@1",
		"list-panes -a -t =workspace-ws_a -F #{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_id}\t#{@workspace_kind}\t#{@workspace_session_id}\t#{@workspace_run_id}": "%1\t@1\t0\tws_a\tagent\tsess_a\trun_a",
	}
	t.Run("zero", func(t *testing.T) {
		navigator := &TmuxNavigator{Socket: "private", Runner: &fakeCommandRunner{outputs: base}, Env: func(string) string { return "" }}
		clients, err := navigator.ListClients(context.Background(), testTarget())
		if err != nil || len(clients) != 0 {
			t.Fatalf("zero-client discovery = %+v, %v", clients, err)
		}
	})
	t.Run("discovery error", func(t *testing.T) {
		errDiscovery := errors.New("permission denied")
		navigator := &TmuxNavigator{Socket: "private", Runner: &fakeCommandRunner{outputs: base, errors: map[string]error{"list-clients -F " + clientFormat: errDiscovery}}, Env: func(string) string { return "" }}
		_, err := navigator.ListClients(context.Background(), testTarget())
		if err == nil || !strings.Contains(err.Error(), "could not discover attached tmux clients") {
			t.Fatalf("discovery failure was not reported clearly: %v", err)
		}
	})
}

func TestNavigatorRejectsWrongAttachedSocketBeforeListingOrSwitching(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string]string{
		"has-session -t =workspace-ws_a":                  "",
		"list-windows -t =workspace-ws_a -F #{window_id}": "@1",
		"list-panes -a -t =workspace-ws_a -F #{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_id}\t#{@workspace_kind}\t#{@workspace_session_id}\t#{@workspace_run_id}": "%1\t@1\t0\tws_a\tagent\tsess_a\trun_a",
	}}
	env := map[string]string{"TMUX": "/tmp/tmux-1000/other,12,0"}
	navigator := &TmuxNavigator{Socket: "private", Runner: runner, Env: func(key string) string { return env[key] }}
	_, err := navigator.ListClients(context.Background(), testTarget())
	var ce *core.Error
	if !errors.As(err, &ce) || ce.Code != "tmux_server_mismatch" {
		t.Fatalf("wrong socket error = %v", err)
	}
	for _, command := range runner.commands {
		if strings.HasPrefix(strings.Join(command[1:], " "), "list-clients ") || strings.HasPrefix(strings.Join(command[1:], " "), "switch-client ") {
			t.Fatalf("wrong socket had a client effect: %v", command)
		}
	}
}
