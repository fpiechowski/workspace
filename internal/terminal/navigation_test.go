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
	outputs  map[string]string
	commands [][]string
	runs     [][]string
}

func (r *fakeCommandRunner) Output(_ context.Context, socket string, args ...string) (string, error) {
	command := append([]string{socket}, args...)
	r.commands = append(r.commands, command)
	if len(args) > 0 {
		return r.outputs[strings.Join(args, " ")], nil
	}
	return "", nil
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
