package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"workspace/internal/core"
)

type Request struct {
	Project, Workspace, Socket string
	CWD, Executable            string
	Env                        map[string]string
	ProjectOnly                bool
}

type Scope struct {
	Service      *core.Service
	ProjectRoot  string
	CWD          string
	WorkspaceID  string
	ProjectFound bool
	ProjectError error
}

// Resolve applies the shared manual CLI precedence: explicit flags, inherited
// WORKSPACE_* values, then the current directory. A missing project or
// workspace is a displayable TUI state; malformed persisted state is an error.
func Resolve(request Request) (*Scope, error) {
	env := request.Env
	if env == nil {
		env = environment()
	}
	cwd := request.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	canonicalCWD, err := canonicalPath(cwd)
	if err != nil {
		return nil, err
	}
	project := request.Project
	if project == "" {
		project = env["WORKSPACE_PROJECT_DIR"]
	}
	if project == "" {
		project = canonicalCWD
	}
	project, err = core.DiscoverProject(project)
	if err != nil {
		var ce *core.Error
		if errors.As(err, &ce) && ce.Code == "project_not_found" {
			return &Scope{CWD: canonicalCWD, ProjectFound: false, ProjectError: err}, nil
		}
		return nil, err
	}
	executable := request.Executable
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	socket := request.Socket
	if socket == "" {
		socket = env["WORKSPACE_TMUX_SOCKET"]
	}
	s := &core.Service{
		Root:       project,
		Runtime:    core.Tmux{Socket: socket},
		Executable: executable,
		Actor: core.Actor{
			AgentID:   env["WORKSPACE_AGENT_ID"],
			SessionID: env["WORKSPACE_SESSION_ID"],
			RunID:     env["WORKSPACE_RUN_ID"],
		},
	}
	out := &Scope{Service: s, ProjectRoot: project, CWD: canonicalCWD, ProjectFound: true}
	if request.ProjectOnly {
		return out, nil
	}
	selector := request.Workspace
	if selector == "" {
		selector = env["WORKSPACE_ID"]
	}
	if selector != "" {
		if _, err := s.WorkspaceSnapshot(context.Background(), selector); err != nil {
			return nil, err
		}
		out.WorkspaceID = selector
		return out, nil
	}
	workspaceID, err := core.InferWorkspace(canonicalCWD)
	if err == nil {
		if _, err := s.WorkspaceSnapshot(context.Background(), workspaceID); err != nil {
			return nil, err
		}
		out.WorkspaceID = workspaceID
		return out, nil
	}
	var ce *core.Error
	if !errors.As(err, &ce) || ce.Code != "workspace_required" {
		return nil, err
	}
	outside, err := matchRegisteredWorktree(context.Background(), s, canonicalCWD)
	if err != nil {
		return nil, err
	}
	out.WorkspaceID = outside
	return out, nil
}

func environment() map[string]string {
	env := make(map[string]string)
	for _, value := range os.Environ() {
		key, item, ok := strings.Cut(value, "=")
		if ok {
			env[key] = item
		}
	}
	return env
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		return filepath.Clean(resolved), nil
	}
	return filepath.Clean(resolved), nil
}

func matchRegisteredWorktree(ctx context.Context, s *core.Service, cwd string) (string, error) {
	overview, err := s.ProjectOverview(ctx)
	if err != nil {
		return "", err
	}
	bestLength := -1
	bestID := ""
	ambiguous := false
	for _, workspace := range overview.Workspaces {
		if workspace.Error != "" {
			continue
		}
		snapshot, err := s.WorkspaceSnapshot(ctx, workspace.ID)
		if err != nil {
			continue
		}
		for _, worktree := range snapshot.Status.Worktrees {
			base, err := canonicalPath(worktree.Path)
			if err != nil || !pathContains(base, cwd) {
				continue
			}
			length := len(base)
			if length > bestLength {
				bestLength, bestID, ambiguous = length, workspace.ID, false
			} else if length == bestLength && workspace.ID != bestID {
				ambiguous = true
			}
		}
	}
	if ambiguous {
		return "", &core.Error{Code: "workspace_conflict", Message: "current directory matches worktrees in multiple workspaces"}
	}
	return bestID, nil
}

func pathContains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}
