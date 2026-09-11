package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type ForgeResult struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	State      string `json:"state"`
	HeadCommit string `json:"head_commit"`
}
type Forge interface {
	Lookup(context.Context, string, ChangeRequest) (*ForgeResult, error)
	Publish(context.Context, string, ChangeRequest, string) (ForgeResult, error)
}
type CLIForge struct{ Config ForgeConfig }

func forgeCommand(ctx context.Context, root string, argv ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GIT_TERMINAL_PROMPT=0", "GLAB_CHECK_UPDATE=false")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, fail("forge_error", "%s: %s", argv[0], stderr.String())
	}
	return b, nil
}
func (f CLIForge) Lookup(ctx context.Context, root string, cr ChangeRequest) (*ForgeResult, error) {
	switch f.Config.Adapter {
	case "github":
		b, err := forgeCommand(ctx, root, "gh", "pr", "list", "--head", cr.Branch, "--base", cr.Target, "--state", "all", "--limit", "100", "--json", "number,url,state,headRefOid,body")
		if err != nil {
			return nil, err
		}
		var items []struct {
			Number                       int
			URL, State, HeadRefOID, Body string
		}
		if err := json.Unmarshal(b, &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			if strings.Contains(item.Body, "workspace-cr:"+cr.ID) {
				return &ForgeResult{strconv.Itoa(item.Number), item.URL, strings.ToLower(item.State), item.HeadRefOID}, nil
			}
		}
		if len(items) > 0 {
			return nil, fail("forge_conflict", "branch has an unrecognized change request; link it explicitly")
		}
		return nil, nil
	case "gitlab":
		b, err := forgeCommand(ctx, root, "glab", "mr", "list", "--source-branch", cr.Branch, "--target-branch", cr.Target, "--all", "--per-page", "100", "--output", "json")
		if err != nil {
			return nil, err
		}
		var items []struct {
			IID         int    `json:"iid"`
			URL         string `json:"web_url"`
			State       string `json:"state"`
			SHA         string `json:"sha"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(b, &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			if strings.Contains(item.Description, "workspace-cr:"+cr.ID) {
				state := item.State
				if state == "opened" {
					state = "open"
				}
				return &ForgeResult{strconv.Itoa(item.IID), item.URL, state, item.SHA}, nil
			}
		}
		if len(items) > 0 {
			return nil, fail("forge_conflict", "branch has an unrecognized merge request; link it explicitly")
		}
		return nil, nil
	case "command":
		return f.command(ctx, root, "lookup", cr, "")
	default:
		return nil, fail("forge_unavailable", "configure forge.adapter (github, gitlab or command), or link/skip the local change request")
	}
}
func (f CLIForge) Publish(ctx context.Context, root string, cr ChangeRequest, bodyFile string) (ForgeResult, error) {
	if f.Config.Adapter == "command" {
		v, err := f.command(ctx, root, "publish", cr, bodyFile)
		if err != nil {
			return ForgeResult{}, err
		}
		if v == nil {
			return ForgeResult{}, fail("forge_error", "publish returned no result")
		}
		return *v, nil
	}
	remote := f.Config.Remote
	if remote == "" {
		remote = "origin"
	}
	if strings.HasPrefix(remote, "-") {
		return ForgeResult{}, fail("invalid_config", "invalid remote")
	}
	if _, err := forgeCommand(ctx, root, "git", "push", remote, cr.HeadCommit+":refs/heads/"+cr.Branch); err != nil {
		return ForgeResult{}, err
	}
	var err error
	switch f.Config.Adapter {
	case "github":
		_, err = forgeCommand(ctx, root, "gh", "pr", "create", "--title", cr.Title, "--body-file", bodyFile, "--base", cr.Target, "--head", cr.Branch)
	case "gitlab":
		_, err = forgeCommand(ctx, root, "glab", "mr", "create", "--title", cr.Title, "--description-file", bodyFile, "--target-branch", cr.Target, "--source-branch", cr.Branch, "--yes")
	default:
		return ForgeResult{}, fail("forge_unavailable", "unknown forge adapter")
	}
	if err != nil {
		return ForgeResult{}, err
	}
	v, err := f.Lookup(ctx, root, cr)
	if err != nil {
		return ForgeResult{}, err
	}
	if v == nil {
		return ForgeResult{}, fail("publication_uncertain", "created request is not yet visible")
	}
	return *v, nil
}
func (f CLIForge) command(ctx context.Context, root, action string, cr ChangeRequest, bodyFile string) (*ForgeResult, error) {
	if len(f.Config.CommandArgv) == 0 {
		return nil, fail("invalid_config", "forge command_argv is required")
	}
	file, err := os.CreateTemp("", "workspace-forge-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	request := struct {
		Version       int           `json:"version"`
		Action        string        `json:"action"`
		ChangeRequest ChangeRequest `json:"change_request"`
		BodyFile      string        `json:"body_file"`
	}{1, action, cr, bodyFile}
	if err := json.NewEncoder(file).Encode(request); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	argv := make([]string, len(f.Config.CommandArgv))
	for i, arg := range f.Config.CommandArgv {
		argv[i] = strings.NewReplacer("{action}", action, "{request_file}", file.Name()).Replace(arg)
	}
	b, err := forgeCommand(ctx, root, argv...)
	if err != nil {
		return nil, err
	}
	var response struct {
		Found  bool         `json:"found"`
		Result *ForgeResult `json:"result"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, err
	}
	if !response.Found {
		return nil, nil
	}
	if response.Result == nil {
		return nil, fmt.Errorf("forge result missing")
	}
	return response.Result, nil
}
func (s *Service) forgeAdapter() (Forge, error) {
	if s.Forge != nil {
		return s.Forge, nil
	}
	cfg, err := s.Config()
	if err != nil {
		return nil, err
	}
	return CLIForge{cfg.Forge}, nil
}
func crBodyPath(d *Document, cr ChangeRequest) string {
	return filepath.Join(d.Dir, "change-requests", cr.ID, "DESCRIPTION.md")
}
