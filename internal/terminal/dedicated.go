package terminal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"workspace/internal/core"
)

const dedicatedLaunchLease = 20 * time.Second

// OpenDedicated routes a verified target through a stable tmux session-group
// viewer. A viewer has its own addressable client/session, while the grouped
// windows and panes remain the canonical workspace runtime. The caller's
// Bubble Tea terminal is never used for this operation.
func (n *TmuxNavigator) OpenDedicated(ctx context.Context, target core.NavigationTarget) error {
	if err := n.verify(ctx, target); err != nil {
		return err
	}

	n.dedicatedMu.Lock()
	defer n.dedicatedMu.Unlock()
	if err := n.verify(ctx, target); err != nil {
		return err
	}

	viewer := dedicatedViewerSessionName(target)
	if err := n.ensureViewer(ctx, target, viewer); err != nil {
		return err
	}
	clients, err := n.viewerClients(ctx, n.targetSocket(target), viewer)
	if err != nil {
		return err
	}
	switch len(clients) {
	case 0:
		if n.launchPending(viewer) {
			return &core.Error{Code: "dedicated_launch_pending", Message: "the dedicated terminal is already being opened; wait for it to attach"}
		}
		if n.Launcher == nil {
			return &core.Error{Code: "terminal_launcher_unavailable", Message: "no dedicated terminal launcher is configured; the current terminal was not changed"}
		}
		if err := n.prepareViewer(ctx, target, viewer); err != nil {
			return err
		}
		// A client may have attached while the viewer target was being prepared.
		// Re-observe before launching so an existing client is reused instead of
		// creating another terminal window.
		clients, err = n.viewerClients(ctx, n.targetSocket(target), viewer)
		if err != nil {
			return err
		}
		if len(clients) > 1 {
			return dedicatedAmbiguousClients(len(clients))
		}
		if len(clients) == 1 {
			delete(n.dedicatedLaunches, viewer)
			return n.routeViewer(ctx, target, viewer, clients[0])
		}
		if err := n.verifyDedicatedTarget(ctx, target, viewer); err != nil {
			return err
		}
		n.markLaunchPending(viewer)
		if err := n.Launcher.Launch(ctx, LaunchSpec{Socket: n.launchSocket(target), Session: viewer}); err != nil {
			delete(n.dedicatedLaunches, viewer)
			var coreErr *core.Error
			if errors.As(err, &coreErr) {
				return err
			}
			return &core.Error{Code: "terminal_launch_failed", Message: "dedicated terminal launch failed: " + err.Error()}
		}
		return nil
	case 1:
		delete(n.dedicatedLaunches, viewer)
		return n.routeViewer(ctx, target, viewer, clients[0])
	default:
		return dedicatedAmbiguousClients(len(clients))
	}
}

func dedicatedViewerSessionName(target core.NavigationTarget) string {
	// The digest avoids tmux-name characters and makes collisions between
	// workspaces impractical without copying caller-controlled text into a
	// target expression. The canonical workspace/session pair is the identity.
	digest := sha256.Sum256([]byte(target.WorkspaceID + "\x00" + target.SessionName))
	return "workspace-viewer-" + hex.EncodeToString(digest[:])[:32]
}

func (n *TmuxNavigator) launchSocket(target core.NavigationTarget) string {
	socket := n.targetSocket(target)
	if strings.TrimSpace(socket) == "" {
		return "default"
	}
	return socket
}

func (n *TmuxNavigator) ensureViewer(ctx context.Context, target core.NavigationTarget, viewer string) error {
	socket := n.targetSocket(target)
	canonicalGroup, err := n.sessionGroup(ctx, socket, target.SessionName)
	if err != nil {
		return dedicatedViewerError("the canonical workspace tmux session disappeared", err)
	}
	exists := n.sessionExists(ctx, socket, viewer)
	if !exists {
		if err := n.verify(ctx, target); err != nil {
			return err
		}
		if _, err := n.runner().Output(ctx, socket, "new-session", "-d", "-P", "-F", "#{session_name}", "-t", "="+target.SessionName, "-s", viewer); err != nil {
			// Another caller may have created the same deterministic viewer after
			// the observation above. Reuse it only if the association is now
			// verifiable; never kill or replace an ambiguous session.
			if !n.sessionExists(ctx, socket, viewer) {
				return &core.Error{Code: "dedicated_viewer_create_failed", Message: fmt.Sprintf("could not create the dedicated tmux viewer: %v", err)}
			}
		}
	}
	if err := n.verify(ctx, target); err != nil {
		return err
	}
	canonicalGroup, err = n.sessionGroup(ctx, socket, target.SessionName)
	if err != nil {
		return dedicatedViewerError("the canonical workspace tmux session disappeared", err)
	}
	if err := n.verifyViewerTarget(ctx, target, viewer, canonicalGroup); err != nil {
		return err
	}
	return nil
}

func (n *TmuxNavigator) sessionExists(ctx context.Context, socket, session string) bool {
	_, err := n.runner().Output(ctx, socket, "has-session", "-t", "="+session)
	return err == nil
}

func (n *TmuxNavigator) sessionGroup(ctx context.Context, socket, session string) (string, error) {
	rows, err := n.runner().Output(ctx, socket, "list-sessions", "-F", "#{session_name}\t#{session_group}")
	if err != nil {
		return "", err
	}
	for _, row := range strings.Split(rows, "\n") {
		fields := strings.SplitN(row, "\t", 2)
		if len(fields) == 1 && fields[0] == session {
			return "", nil
		}
		if len(fields) == 2 && fields[0] == session {
			return strings.TrimSpace(fields[1]), nil
		}
	}
	return "", fmt.Errorf("tmux session %s is unavailable", session)
}

func (n *TmuxNavigator) verifyViewerTarget(ctx context.Context, target core.NavigationTarget, viewer, canonicalGroup string) error {
	socket := n.targetSocket(target)
	group, err := n.sessionGroup(ctx, socket, viewer)
	if err != nil {
		return dedicatedViewerError("the dedicated tmux viewer is unavailable", err)
	}
	if canonicalGroup == "" || group == "" || group != canonicalGroup {
		return &core.Error{Code: "dedicated_viewer_mismatch", Message: "the dedicated tmux viewer is not grouped with the canonical workspace session"}
	}
	if target.WindowID != "" {
		windows, err := n.runner().Output(ctx, socket, "list-windows", "-t", "="+viewer, "-F", "#{window_id}")
		if err != nil {
			return dedicatedViewerError("the dedicated tmux viewer could not be inspected", err)
		}
		if !containsLine(windows, target.WindowID) {
			return &core.Error{Code: "dedicated_viewer_stale", Message: "the dedicated tmux viewer no longer shares the verified workspace window; close it and retry"}
		}
	}
	if target.PaneID != "" {
		panes, err := n.runner().Output(ctx, socket, "list-panes", "-a", "-t", "="+viewer, "-F", "#{pane_id}\t#{window_id}\t#{pane_dead}")
		if err != nil {
			return dedicatedViewerError("the dedicated tmux viewer panes could not be inspected", err)
		}
		found := false
		for _, row := range strings.Split(panes, "\n") {
			fields := strings.SplitN(row, "\t", 3)
			if len(fields) == 3 && fields[0] == target.PaneID && fields[1] == target.WindowID && fields[2] == "0" {
				found = true
				break
			}
		}
		if !found {
			return &core.Error{Code: "dedicated_viewer_stale", Message: "the dedicated tmux viewer no longer shares the verified workspace pane; close it and retry"}
		}
	}
	return nil
}

func (n *TmuxNavigator) verifyDedicatedTarget(ctx context.Context, target core.NavigationTarget, viewer string) error {
	if err := n.verify(ctx, target); err != nil {
		return err
	}
	canonicalGroup, err := n.sessionGroup(ctx, n.targetSocket(target), target.SessionName)
	if err != nil {
		return dedicatedViewerError("the canonical workspace tmux session disappeared", err)
	}
	return n.verifyViewerTarget(ctx, target, viewer, canonicalGroup)
}

func (n *TmuxNavigator) viewerClients(ctx context.Context, socket, viewer string) ([]string, error) {
	rows, err := n.runner().Output(ctx, socket, "list-clients", "-t", "="+viewer, "-F", "#{client_tty}")
	if err != nil {
		return nil, dedicatedViewerError("the dedicated tmux viewer clients could not be inspected", err)
	}
	var clients []string
	for _, row := range strings.Split(rows, "\n") {
		if client := strings.TrimSpace(row); client != "" {
			clients = append(clients, client)
		}
	}
	return clients, nil
}

func (n *TmuxNavigator) prepareViewer(ctx context.Context, target core.NavigationTarget, viewer string) error {
	if err := n.verifyDedicatedTarget(ctx, target, viewer); err != nil {
		return err
	}
	if target.WindowID != "" {
		if err := n.verifyDedicatedTarget(ctx, target, viewer); err != nil {
			return err
		}
		if _, err := n.runner().Output(ctx, n.targetSocket(target), "select-window", "-t", "="+viewer+":"+target.WindowID); err != nil {
			return dedicatedViewerError("the dedicated tmux viewer could not select the verified window", err)
		}
	}
	if target.PaneID != "" {
		if err := n.verifyDedicatedTarget(ctx, target, viewer); err != nil {
			return err
		}
		if _, err := n.runner().Output(ctx, n.targetSocket(target), "select-pane", "-t", target.PaneID); err != nil {
			return dedicatedViewerError("the dedicated tmux viewer could not select the verified pane", err)
		}
	}
	return nil
}

func (n *TmuxNavigator) routeViewer(ctx context.Context, target core.NavigationTarget, viewer, client string) error {
	if current, err := n.isCurrentClient(ctx, n.targetSocket(target), client); err != nil {
		return err
	} else if current {
		return &core.Error{Code: "dedicated_client_conflict", Message: "the only dedicated viewer client is the terminal displaying the TUI; the TUI terminal was not changed"}
	}
	if err := n.verifyDedicatedTarget(ctx, target, viewer); err != nil {
		return err
	}
	if _, err := n.runner().Output(ctx, n.targetSocket(target), "switch-client", "-c", client, "-t", "="+viewer); err != nil {
		return dedicatedViewerError("the dedicated tmux client could not be selected", err)
	}
	if target.WindowID != "" {
		if err := n.verifyDedicatedTarget(ctx, target, viewer); err != nil {
			return err
		}
		if _, err := n.runner().Output(ctx, n.targetSocket(target), "select-window", "-t", "="+viewer+":"+target.WindowID); err != nil {
			return dedicatedViewerError("the dedicated tmux client could not select the verified window", err)
		}
	}
	if target.PaneID != "" {
		if err := n.verifyDedicatedTarget(ctx, target, viewer); err != nil {
			return err
		}
		if _, err := n.runner().Output(ctx, n.targetSocket(target), "select-pane", "-t", target.PaneID); err != nil {
			return dedicatedViewerError("the dedicated tmux client could not select the verified pane", err)
		}
	}
	return nil
}

func (n *TmuxNavigator) isCurrentClient(ctx context.Context, socket, client string) (bool, error) {
	tmuxEnv := n.env("TMUX")
	if tmuxEnv == "" {
		return false, nil
	}
	currentPane := n.env("TMUX_PANE")
	if currentPane == "" {
		return false, &core.Error{Code: "dedicated_client_ambiguous", Message: "the current tmux pane could not be identified without risking the TUI terminal"}
	}
	currentSocket := strings.SplitN(tmuxEnv, ",", 2)[0]
	expected := socket
	if expected == "" {
		expected = "default"
	}
	if filepathBase(currentSocket) != expected {
		return false, nil
	}
	pane, err := n.runner().Output(ctx, socket, "display-message", "-p", "-c", client, "#{pane_id}")
	if err != nil {
		return false, &core.Error{Code: "dedicated_client_ambiguous", Message: "the dedicated viewer client could not be identified without risking the TUI terminal"}
	}
	return strings.TrimSpace(pane) == currentPane, nil
}

func (n *TmuxNavigator) launchPending(viewer string) bool {
	if n.dedicatedLaunches == nil {
		n.dedicatedLaunches = make(map[string]time.Time)
	}
	started, ok := n.dedicatedLaunches[viewer]
	if !ok {
		return false
	}
	if time.Since(started) < dedicatedLaunchLease {
		return true
	}
	delete(n.dedicatedLaunches, viewer)
	return false
}

func (n *TmuxNavigator) markLaunchPending(viewer string) {
	if n.dedicatedLaunches == nil {
		n.dedicatedLaunches = make(map[string]time.Time)
	}
	n.dedicatedLaunches[viewer] = time.Now()
}

func dedicatedViewerError(message string, err error) error {
	return &core.Error{Code: "dedicated_viewer_unavailable", Message: fmt.Sprintf("%s: %v", message, err)}
}

func dedicatedAmbiguousClients(count int) error {
	return &core.Error{Code: "navigation_ambiguous", Message: fmt.Sprintf("the dedicated tmux viewer has %d attached clients; refusing to choose one", count)}
}

func filepathBase(path string) string {
	if index := strings.LastIndexAny(path, "/\\"); index >= 0 {
		return path[index+1:]
	}
	return path
}
