package core

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed templates
var templates embed.FS

type Service struct {
	IssueFetcher IssueFetcher
	Root         string
	Runtime      Runtime
	Executable   string
	Actor        Actor
	Forge        Forge
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fail("git_error", "%s: %s", strings.Join(args, " "), strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func DiscoverProject(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".workspace", "config.yaml")); err == nil {
			return dir, nil
		}
		if b, err := os.ReadFile(filepath.Join(dir, "WORKSPACE.md")); err == nil {
			d := &Document{}
			if decodeDocument(b, d) == nil && d.State.ProjectRoot != "" {
				s := &Service{Root: d.State.ProjectRoot}
				cfg, err := s.Config()
				if err == nil && cfg.ProjectID == d.State.ProjectID {
					return s.Root, nil
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fail("project_not_found", "run workspace project init inside a Git project")
}
func InitProject(ctx context.Context, dir string, keys ...string) (Config, error) {
	root, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Config{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Config{}, err
	}
	if key := mutationKey(keys); key != "" {
		return projectEffect(ctx, root, key, "project.init", func() (Config, error) { return InitProject(ctx, root) })
	}
	unlock, err := lockProject(ctx, root)
	if err != nil {
		return Config{}, err
	}
	defer unlock()
	s := &Service{Root: root}
	path := filepath.Join(root, ".workspace", "config.yaml")
	existingConfig := false
	if _, err := os.Stat(path); err == nil {
		if _, err := s.Config(); err != nil {
			return Config{}, err
		}
		existingConfig = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	if err := fs.WalkDir(templates, "templates", func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		b, err := templates.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, ".workspace", filepath.FromSlash(path))
		if _, err := os.Stat(target); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return atomicWrite(target, b)
	}); err != nil {
		return Config{}, err
	}
	ignore, err := git(ctx, root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return Config{}, err
	}
	if !filepath.IsAbs(ignore) {
		ignore = filepath.Join(root, ignore)
	}
	b, err := os.ReadFile(ignore)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	for _, rule := range []string{"/.workspace/ws_*/", "/.workspace/.runtime/", "/.workspace/.creating-*/", "/work-products/"} {
		if !strings.Contains("\n"+string(b)+"\n", "\n"+rule+"\n") {
			b = append(b, []byte("\n"+rule+"\n")...)
		}
	}
	if err := atomicWrite(ignore, b); err != nil {
		return Config{}, err
	}
	if existingConfig {
		return s.Config()
	}
	cfg := Config{SchemaVersion: 1, ProjectID: ID("prj"), Runtime: "tmux", Clients: map[string]Client{}, Profiles: map[string]Profile{}}
	b, err = yaml.Marshal(cfg)
	if err != nil {
		return Config{}, err
	}
	return cfg, atomicWrite(path, b)
}
func (s *Service) Config() (Config, error) {
	var cfg Config
	b, err := os.ReadFile(filepath.Join(s.Root, ".workspace", "config.yaml"))
	if err != nil {
		return cfg, err
	}
	if err := strictYAML(b, &cfg); err != nil {
		return cfg, fail("invalid_config", "%v", err)
	}
	if cfg.SchemaVersion != 1 || cfg.ProjectID == "" || cfg.Runtime != "tmux" {
		return cfg, fail("invalid_config", "expected schema_version: 1, project_id and runtime: tmux")
	}
	switch cfg.Tracker.Adapter {
	case "", "github", "gitlab":
	case "command":
		if len(cfg.Tracker.CommandArgv) == 0 || cfg.Tracker.CommandArgv[0] == "" || !strings.Contains(strings.Join(cfg.Tracker.CommandArgv, "\n"), "{url}") {
			return cfg, fail("invalid_config", "tracker command requires executable and {url} argument")
		}
	default:
		return cfg, fail("invalid_config", "unsupported tracker adapter")
	}
	for name, client := range cfg.Clients {
		client, err = normalizeClient(client)
		if err != nil {
			return cfg, fail("invalid_config", "client %q: %v", name, err)
		}
		cfg.Clients[name] = client
	}
	for name, p := range cfg.Profiles {
		if p.Strategy != "" && p.Strategy != "provider-balanced" {
			return cfg, fail("invalid_config", "unsupported strategy in profile %q", name)
		}
		if p.CooldownSeconds < 0 {
			return cfg, fail("invalid_config", "cooldown_seconds must be nonnegative")
		}
		for _, cap := range p.RequiredCapabilities {
			if cap != "launch" && cap != "resume" && cap != "deliver" && cap != "observe" && cap != "interrupt" {
				return cfg, fail("invalid_config", "unknown capability %q", cap)
			}
		}
		if len(p.Routes) == 0 {
			return cfg, fail("invalid_config", "profile %q has no routes", name)
		}
		seen := map[string]bool{}
		for _, r := range p.Routes {
			if r.ID == "" || seen[r.ID] || r.Provider == "" || r.Model == "" || r.MaxConcurrency < 1 || r.MaxLaunches24h < 0 {
				return cfg, fail("invalid_config", "invalid or duplicate route in %q", name)
			}
			seen[r.ID] = true
			if _, ok := cfg.Clients[r.Client]; !ok {
				return cfg, fail("invalid_config", "unknown client %q", r.Client)
			}
		}
		for _, w := range p.ProviderWeights {
			if w <= 0 || math.IsNaN(w) || math.IsInf(w, 0) {
				return cfg, fail("invalid_config", "provider weights must be positive")
			}
		}
	}
	if profile := cfg.Defaults.OrchestratorProfile; profile != "" {
		if _, ok := cfg.Profiles[profile]; !ok {
			return cfg, fail("invalid_config", "defaults.orchestrator_profile references unknown profile %q", profile)
		}
	}
	for name, w := range cfg.Workflows {
		if w.MaxParallelTasks < 0 {
			return cfg, fail("invalid_config", "negative max_parallel_tasks in %s", name)
		}
		if w.ChangeRequests != "" && w.ChangeRequests != "integrated" && w.ChangeRequests != "per-task" {
			return cfg, fail("invalid_config", "change_requests must be integrated or per-task")
		}
		for role, profile := range w.Profiles {
			if role != "orchestrator" && role != "planning" && role != "implementation" && role != "integration" && role != "live-testing" {
				return cfg, fail("invalid_config", "unknown workflow profile role %q", role)
			}
			if _, ok := cfg.Profiles[profile]; !ok {
				return cfg, fail("invalid_config", "workflow %s references unknown profile %s", name, profile)
			}
		}
	}
	return cfg, nil
}
func (s *Service) workspaceDirs() ([]string, error) {
	cfg, err := s.Config()
	if err != nil {
		return nil, err
	}
	root, err := s.storageRoot(cfg)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "ws_") {
			dir := filepath.Join(root, e.Name())
			b, err := os.ReadFile(filepath.Join(dir, "WORKSPACE.md"))
			if err != nil {
				return nil, err
			}
			d := &Document{}
			if err := decodeDocument(b, d); err != nil {
				return nil, err
			}
			if d.State.ProjectID == cfg.ProjectID {
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs, nil
}
func (s *Service) resolve(selector string) (string, error) {
	dirs, err := s.workspaceDirs()
	if err != nil {
		return "", err
	}
	for _, dir := range dirs {
		if filepath.Base(dir) == selector {
			return dir, nil
		}
		b, err := os.ReadFile(filepath.Join(dir, "WORKSPACE.md"))
		if err != nil {
			return "", err
		}
		d := &Document{}
		if err := decodeDocument(b, d); err != nil {
			return "", err
		}
		if d.State.ID == selector {
			return dir, nil
		}
	}
	return "", fail("workspace_not_found", "unknown workspace %q", selector)
}
func (s *Service) With(ctx context.Context, selector string, fn func(*Document) error) error {
	unlock, err := lockProject(ctx, s.Root)
	if err != nil {
		return err
	}
	defer unlock()
	dir, err := s.resolve(selector)
	if err != nil {
		return err
	}
	d, err := loadDocument(dir)
	if err != nil {
		return err
	}
	return fn(d)
}
func (s *Service) List(ctx context.Context) ([]Status, error) {
	unlock, err := lockProject(ctx, s.Root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	dirs, err := s.workspaceDirs()
	if err != nil {
		return nil, err
	}
	out := []Status{}
	for _, dir := range dirs {
		d, err := loadDocument(dir)
		if err != nil {
			return nil, err
		}
		out = append(out, d.Status())
	}
	return out, nil
}
func (s *Service) Status(ctx context.Context, selector string) (Status, error) {
	var out Status
	err := s.With(ctx, selector, func(d *Document) error { out = d.Status(); return nil })
	return out, err
}

type CreateOptions struct{ Title, Input, Source, Workflow, Base, OperationKey string }

const exampleWorkflow = "plan-first"

func workflowTemplateExists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, ".workspace", "templates", "workflows", name, "WORKFLOW.md.tmpl"))
	return err == nil
}

func workflowAvailable(root string, cfg Config, name string) bool {
	if _, ok := cfg.Workflows[name]; ok {
		return true
	}
	// Keep the bundled example selectable before the user has added model
	// profiles. Existing projects may also still contain the legacy template.
	return name == exampleWorkflow && workflowTemplateExists(root, name) || name == "issue-resolution" && workflowTemplateExists(root, name)
}

func WorkflowNames(root string, cfg Config) []string {
	names := make([]string, 0, len(cfg.Workflows)+1)
	for name := range cfg.Workflows {
		names = append(names, name)
	}
	if !slices.Contains(names, exampleWorkflow) && workflowTemplateExists(root, exampleWorkflow) {
		names = append(names, exampleWorkflow)
	}
	slices.Sort(names)
	return names
}

func (s *Service) Create(ctx context.Context, opt CreateOptions) (Status, error) {
	if strings.TrimSpace(opt.Input) == "" && opt.Source == "" {
		return Status{}, fail("input_required", "provide an issue description/snapshot as an argument or with --input-file; a URL alone is insufficient")
	}
	if opt.Title == "" {
		opt.Title = "Untitled issue"
	}
	if opt.Base == "" {
		opt.Base = "HEAD"
	}
	unlock, err := lockProject(ctx, s.Root)
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	cfg, err := s.Config()
	if err != nil {
		return Status{}, err
	}
	if opt.Workflow != "" && !workflowAvailable(s.Root, cfg, opt.Workflow) {
		return Status{}, fail("unknown_workflow", "available workflow: %s", strings.Join(WorkflowNames(s.Root, cfg), ", "))
	}
	dirs, err := s.workspaceDirs()
	if err != nil {
		return Status{}, err
	}
	for _, dir := range dirs {
		d, err := loadDocument(dir)
		if err != nil {
			return Status{}, err
		}
		id, err := d.previous("create:"+opt.OperationKey, opt)
		if opt.OperationKey != "" {
			if err != nil {
				return Status{}, err
			}
			if id != "" {
				var out Status
				if found, err := replayResource(d, "create:"+opt.OperationKey, &out); found || err != nil {
					return out, err
				}
				return d.Status(), nil
			}
		}
	}
	request := opt
	if strings.TrimSpace(opt.Input) == "" {
		fetcher := s.IssueFetcher
		if fetcher == nil {
			fetcher = CLITracker{Root: s.Root, Config: cfg.Tracker}
		}
		issue, err := fetcher.Fetch(ctx, opt.Source)
		if err != nil {
			return Status{}, err
		}
		opt.Input = issueSnapshot(opt.Source, issue)
		if opt.Title == "Untitled issue" && issue.Title != "" {
			opt.Title = issue.Title
		}
	}
	base, err := git(ctx, s.Root, "rev-parse", "--verify", "--end-of-options", opt.Base+"^{commit}")
	if err != nil {
		return Status{}, err
	}
	baseRef := opt.Base
	if baseRef == "HEAD" {
		if branch, err := git(ctx, s.Root, "symbolic-ref", "--short", "HEAD"); err == nil {
			baseRef = branch
		}
	}
	id := ID("ws")
	storage, err := s.storageRoot(cfg)
	if err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(storage, 0700); err != nil {
		return Status{}, err
	}
	dir, err := os.MkdirTemp(storage, ".creating-")
	if err != nil {
		return Status{}, err
	}
	defer os.RemoveAll(dir) // dir is an exclusively-created staging directory, never a worktree.
	d := &Document{Dir: dir, State: Workspace{SchemaVersion: 1, ID: id, ProjectID: cfg.ProjectID, Title: opt.Title, Revision: 1, Status: "needs_workflow", Input: Input{opt.Source, "inputs/issue.md"}, Base: Base{baseRef, base}, CreatedAt: time.Now().UTC()}, Registry: Registry{Agents: []Agent{}, Worktrees: []Worktree{}, Sessions: []Session{}, Operations: map[string]Operation{}}}
	d.State.ProjectRoot = s.Root
	orch := Agent{ID("agent"), "orchestrator", "orchestrator", cfg.Defaults.OrchestratorProfile, "orchestrator", "Coordinate the workflow; delegate all code changes to workers."}
	d.State.OrchestratorAgentID = orch.ID
	d.Registry.Agents = append(d.Registry.Agents, orch)
	for _, sub := range []string{"inputs", "prompts", "tasks", "artifacts", "worktrees", ".runtime"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			return Status{}, err
		}
	}
	if err := atomicWrite(filepath.Join(dir, "inputs", "issue.md"), []byte(opt.Input)); err != nil {
		return Status{}, err
	}
	if err := s.snapshotTemplates(d, opt.Workflow); err != nil {
		return Status{}, err
	}
	if d.State.Workflow != nil {
		orch.Profile = workflowProfile(cfg, d, "orchestrator", orch.Profile)
		d.Registry.Agents[0] = orch
	}
	body, err := s.render("WORKSPACE.md.tmpl", d.State)
	if err != nil {
		return Status{}, err
	}
	d.Body = string(body)
	if opt.OperationKey != "" {
		d.remember("create:"+opt.OperationKey, request, id)
		op := d.Registry.Operations["create:"+opt.OperationKey]
		op.Revision = d.State.Revision
		result := d.Status()
		result.Directory = filepath.Join(storage, id)
		op.Result, err = json.Marshal(result)
		if err != nil {
			return Status{}, err
		}
		d.Registry.Operations["create:"+opt.OperationKey] = op
	}
	b, err := encodeDocument(d)
	if err != nil {
		return Status{}, err
	}
	d.Registry.WorkspaceDigest = digest(b)
	if err := atomicWrite(filepath.Join(dir, "WORKSPACE.md"), b); err != nil {
		return Status{}, err
	}
	if err := writeJSON(filepath.Join(dir, ".runtime", "index.json"), d.Registry); err != nil {
		return Status{}, err
	}
	final := filepath.Join(storage, id)
	if err := os.Rename(dir, final); err != nil {
		return Status{}, err
	}
	if err := syncDirectory(filepath.Dir(final)); err != nil {
		return Status{}, err
	}
	d.Dir = final
	return d.Status(), nil
}
func (s *Service) render(name string, data any) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(s.Root, ".workspace", "templates", filepath.FromSlash(name)))
	if err != nil {
		return nil, err
	}
	t, err := template.New(name).Option("missingkey=error").Parse(string(b))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func (s *Service) snapshotTemplates(d *Document, workflow string) error {
	a, err := s.render("orchestrator.AGENTS.md.tmpl", d.State)
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(d.Dir, "AGENTS.md"), a); err != nil {
		return err
	}
	worker, err := s.render("worker.AGENTS.md.tmpl", d.State)
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(d.Dir, "prompts", "worker.AGENTS.md"), worker); err != nil {
		return err
	}
	w := []byte("# Select a workflow\n\nAsk the user to select one of the available workflows before delegating work.\n")
	if workflow != "" {
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		d.State.ChangeRequestMode = cfg.Workflows[workflow].ChangeRequests
		if d.State.ChangeRequestMode == "" && workflow == "issue-resolution" {
			d.State.ChangeRequestMode = "integrated"
		}
		w, err = s.render("workflows/"+workflow+"/WORKFLOW.md.tmpl", d.State)
		if err != nil {
			return err
		}
		d.State.Workflow = &Workflow{workflow, 1, digest(w), "planning"}
		d.State.Status = "active"
	}
	if err := atomicWrite(filepath.Join(d.Dir, "WORKFLOW.md"), w); err != nil {
		return err
	}
	promptDir := filepath.Join(s.Root, ".workspace", "templates", "workflows", workflow)
	if workflow == "" {
		promptDir = filepath.Join(s.Root, ".workspace", "templates", "workflows", exampleWorkflow)
	}
	entries, err := os.ReadDir(filepath.Join(promptDir, "prompts"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md.tmpl") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md.tmpl")
		b, err := os.ReadFile(filepath.Join(promptDir, "prompts", entry.Name()))
		if err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(d.Dir, "prompts", name+".md.tmpl"), b); err != nil {
			return err
		}
	}
	return nil
}
func InferWorkspace(cwd string) (string, error) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	for {
		if b, err := os.ReadFile(filepath.Join(dir, "WORKSPACE.md")); err == nil {
			d := &Document{}
			if err := decodeDocument(b, d); err != nil {
				return "", err
			}
			return d.State.ID, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fail("workspace_required", "use --workspace <id> or run from a workspace directory")
}
func (s *Service) SelectWorkflow(ctx context.Context, selector, name string, keys ...string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, keys, []any{"workflow.select", name}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		if !workflowAvailable(s.Root, cfg, name) {
			return fail("unknown_workflow", "available workflow: %s", strings.Join(WorkflowNames(s.Root, cfg), ", "))
		}
		if d.State.Workflow != nil {
			if d.State.Workflow.ID != name {
				return fail("workflow_conflict", "workflow already selected")
			}
			out = d.Status()
			return nil
		}
		// Snapshot files first; repeating selection after interruption is safe.
		if err := s.snapshotTemplates(d, name); err != nil {
			return err
		}
		if err := saveDocument(d); err != nil {
			return err
		}
		out = d.Status()
		return nil
	})
	return out, err
}
func (s *Service) SetPaused(ctx context.Context, selector string, paused bool, keys ...string) (Status, error) {
	var out Status
	err := mutate(s, ctx, selector, keys, []any{"set-paused", paused}, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if d.State.Status == "completed" || d.State.Status == "archived" {
			return fail("workspace_closed", "closed workspace cannot be resumed or paused")
		}
		if d.State.Workflow == nil {
			return decisionRequired("select a workflow first", exampleWorkflow)
		}
		next := "active"
		if paused {
			next = "paused"
		}
		if d.State.Status != next {
			d.State.Status = next
			if err := saveDocument(d); err != nil {
				return err
			}
		}
		out = d.Status()
		return nil
	})
	return out, err
}
func (s *Service) String() string { return fmt.Sprintf("workspace project at %s", s.Root) }
