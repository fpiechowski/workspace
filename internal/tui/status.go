package tui

import "strings"

func statusBadge(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	switch state {
	case "running", "starting", "active":
		return "[>] " + state
	case "failed", "error":
		return "[x] " + state
	case "blocked", "needs_changes":
		return "[!] " + state
	case "accepted", "completed", "ready":
		return "[+] " + state
	case "idle", "pending", "paused":
		return "[-] " + state
	case "stopped", "archived", "closed", "removed":
		return "[#] " + state
	case "interrupted", "exited":
		return "[~] " + state
	case "":
		return "[-] unknown"
	default:
		return "[?] " + state
	}
}
