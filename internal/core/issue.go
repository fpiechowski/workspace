package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type TrackerConfig struct {
	Adapter     string   `json:"adapter" yaml:"adapter"`
	CommandArgv []string `json:"command_argv,omitempty" yaml:"command_argv,omitempty"`
}
type IssueContent struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}
type IssueFetcher interface {
	Fetch(context.Context, string) (IssueContent, error)
}
type CLITracker struct {
	Root   string
	Config TrackerConfig
}

func (t CLITracker) Fetch(ctx context.Context, source string) (IssueContent, error) {
	u, err := url.Parse(source)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return IssueContent{}, fail("invalid_issue_url", "provide an HTTP(S) issue URL without credentials")
	}
	adapter := t.Config.Adapter
	if adapter == "" {
		switch u.Hostname() {
		case "github.com":
			adapter = "github"
		case "gitlab.com":
			adapter = "gitlab"
		}
	}
	var argv []string
	switch adapter {
	case "github":
		argv = []string{"gh", "issue", "view", source, "--json", "title,body"}
	case "gitlab":
		argv = []string{"glab", "issue", "view", source, "--output", "json"}
	case "command":
		if len(t.Config.CommandArgv) == 0 {
			return IssueContent{}, fail("invalid_config", "tracker command_argv is required")
		}
		for _, arg := range t.Config.CommandArgv {
			argv = append(argv, strings.ReplaceAll(arg, "{url}", source))
		}
	default:
		return IssueContent{}, fail("tracker_unavailable", "configure tracker.adapter or supply --input-file for this issue")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b, err := forgeCommand(ctx, t.Root, argv...)
	if err != nil {
		return IssueContent{}, fail("tracker_unavailable", "cannot retrieve issue; supply --input-file or restore tracker access: %v", err)
	}
	if len(b) > 4*1024*1024 {
		return IssueContent{}, fail("input_too_large", "issue exceeds 4 MiB")
	}
	var result struct{ Title, Body, Description string }
	if err := json.Unmarshal(b, &result); err != nil {
		return IssueContent{}, fail("invalid_issue", "tracker response must be JSON with title and body")
	}
	if adapter == "gitlab" {
		result.Body = result.Description
	}
	if strings.TrimSpace(result.Title) == "" && strings.TrimSpace(result.Body) == "" {
		return IssueContent{}, fail("input_required", "tracker returned no issue description")
	}
	return IssueContent{Title: result.Title, Body: result.Body}, nil
}
func issueSnapshot(source string, content IssueContent) string {
	return fmt.Sprintf("# %s\n\nSource: %s\nRetrieved: %s\n\n%s\n", content.Title, source, nowUTC().Format(time.RFC3339), content.Body)
}
