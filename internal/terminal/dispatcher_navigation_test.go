package terminal

import (
	"context"
	"strings"
	"testing"

	"workspace/internal/core"
)

const dispatcherListPanesFormat = "#{pane_id}\t#{window_id}\t#{pane_dead}\t#{@workspace_scope}\t#{@workspace_project_id}\t#{@workspace_kind}\t#{@workspace_session_id}\t#{@workspace_run_id}"

const dispatcherPaneRow = "%disp\t@disp\t0\tproject\tproj_a\tdispatcher\tsess_disp\trun_disp"

func dispatcherTestTarget() core.NavigationTarget {
	return core.NavigationTarget{
		ProjectID: "proj_a", SessionName: "workspace-dispatcher-proj_a", Socket: "private",
		WindowID: "@disp", PaneID: "%disp", Kind: "dispatcher", SessionID: "sess_disp", RunID: "run_disp",
	}
}

func dispatcherVerifyOutputs(row string) map[string]string {
	return map[string]string{
		"has-session -t =workspace-dispatcher-proj_a":                                   "",
		"list-windows -t =workspace-dispatcher-proj_a -F #{window_id}":                  "@disp",
		"list-panes -a -t =workspace-dispatcher-proj_a -F " + dispatcherListPanesFormat: row,
	}
}

func TestNavigatorVerifiesProjectScopedDispatcherTarget(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
		code string
	}{
		{name: "verified", row: dispatcherPaneRow},
		{name: "wrong scope", row: "%disp\t@disp\t0\tworkspace\tproj_a\tdispatcher\tsess_disp\trun_disp", code: "pane_mismatch"},
		{name: "wrong project", row: "%disp\t@disp\t0\tproject\tproj_b\tdispatcher\tsess_disp\trun_disp", code: "pane_mismatch"},
		{name: "wrong kind", row: "%disp\t@disp\t0\tproject\tproj_a\tagent\tsess_disp\trun_disp", code: "pane_mismatch"},
		{name: "dead pane", row: "%disp\t@disp\t1\tproject\tproj_a\tdispatcher\tsess_disp\trun_disp", code: "pane_mismatch"},
		{name: "mismatched session", row: "%disp\t@disp\t0\tproject\tproj_a\tdispatcher\tsess_other\trun_disp", code: "pane_mismatch"},
		{name: "mismatched run", row: "%disp\t@disp\t0\tproject\tproj_a\tdispatcher\tsess_disp\trun_other", code: "pane_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeCommandRunner{outputs: dispatcherVerifyOutputs(tc.row)}
			navigator := &TmuxNavigator{Socket: "private", Runner: runner, Env: func(string) string { return "" }}
			err := navigator.verify(context.Background(), dispatcherTestTarget())
			if tc.code == "" {
				if err != nil {
					t.Fatalf("verified dispatcher target rejected: %v", err)
				}
				return
			}
			expectNavigationCode(t, err, tc.code)
		})
	}
}

func TestNavigatorRejectsNonCanonicalDispatcherSession(t *testing.T) {
	target := dispatcherTestTarget()
	target.SessionName = "workspace-proj_a"
	runner := &fakeCommandRunner{}
	navigator := &TmuxNavigator{Socket: "private", Runner: runner, Env: func(string) string { return "" }}
	expectNavigationCode(t, navigator.verify(context.Background(), target), "invalid_navigation_target")
	if len(runner.commands) != 0 {
		t.Fatalf("non-canonical dispatcher session reached tmux: %+v", runner.commands)
	}
}

func TestNavigatorListsAndJumpsProjectScopedDispatcher(t *testing.T) {
	chosen := Client{TTY: "/dev/pts/4", Session: "other", SessionID: "$9", WindowID: "@9", PaneID: "%9", PID: "100", Name: "xterm", TermName: "xterm", Created: "10"}
	live := clientFixtureRow(chosen.TTY, chosen.Session, chosen.SessionID, chosen.WindowID, chosen.PaneID, chosen.PID, chosen.Name, chosen.TermName, chosen.Created)
	outputs := dispatcherVerifyOutputs(dispatcherPaneRow)
	outputs["list-clients -F "+clientFormat] = live
	runner := &fakeCommandRunner{outputs: outputs}
	navigator := &TmuxNavigator{Socket: "private", Runner: runner, Env: func(string) string { return "" }}

	clients, err := navigator.ListClients(context.Background(), dispatcherTestTarget())
	if err != nil || len(clients) != 1 || clients[0] != chosen {
		t.Fatalf("dispatcher ListClients = %+v, %v", clients, err)
	}
	if err := navigator.Jump(context.Background(), dispatcherTestTarget(), chosen); err != nil {
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
	if strings.Join(switchCommand, " ") != "switch-client -c /dev/pts/4 -t =workspace-dispatcher-proj_a" {
		t.Fatalf("dispatcher jump did not use the chosen client: %v", switchCommand)
	}
	if strings.Join(windowCommand, " ") != "select-window -t =workspace-dispatcher-proj_a:@disp" || strings.Join(paneCommand, " ") != "select-pane -t %disp" {
		t.Fatalf("dispatcher jump did not select exact verified IDs: window=%v pane=%v", windowCommand, paneCommand)
	}
}

func TestNavigatorReportsMissingDispatcherSessionAsPaneMissing(t *testing.T) {
	runner := &fakeCommandRunner{errors: map[string]error{
		"has-session -t =workspace-dispatcher-proj_a": context.DeadlineExceeded,
	}}
	navigator := &TmuxNavigator{Socket: "private", Runner: runner, Env: func(string) string { return "" }}
	expectNavigationCode(t, navigator.verify(context.Background(), dispatcherTestTarget()), "pane_missing")
}
