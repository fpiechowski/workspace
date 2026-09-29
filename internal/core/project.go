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
	IssueFetcher          IssueFetcher
	Root                  string
	Runtime               Runtime
	Executable            string
	Actor                 Actor
	Forge                 Forge
	openCodeSessionLister openCodeSessionLister
	// recoveryLimitSkip defers an orchestrator recovery that failed because
	// every route is usage-limited until the reported reset. It is in-memory
	// per workspace; the durable ledger remains the source of truth.
	recoveryLimitSkip map[string]time.Time
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(b))
		if message == "" {
			message = err.Error()
		}
		return "", fail("git_error", "%s: %s", strings.Join(args, " "), message)
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
func resolveProjectRoot(ctx context.Context, dir string) (string, error) {
	root, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(root)
}

var stockPlanFirstTemplateDigests = map[string]bool{
	"4569beee09d99f7a6c7fec1ad04006f5d41eac8051d32d05e2d69e0b644770bc": true,
	"584360edfa6b9a9e5ecc4e3313a0b233f32198dbbbef71d0b037d399306eda9d": true,
	"ada9a62a3d4c1a725ada2a03f97a63b1110ec0e29d703e2ae97292e43576a8af": true,
	"0f8af62402a8a187ae7fbb022124e7137e1c30920e4526e40694d8e861f3eb1c": true,
}

var stockOrchestratorPromptDigests = map[string]bool{
	"9c2f024ae978aa7ac55210a8b7827842d4d57351eaeda922bcb0238ac8b870e2": true,
	"3c8dba7d97ea130ab30b4b251f33bf481e61e5a1eeb8413ebe7202d12c1ae6dc": true,
	"851d90f98206ce6e1b73243005dda72c1eb643d506e491405e9cf4621d58eb66": true,
	"5c789f44ff118a0bf4bc08a521c7ec5184fec138aa247eeb6c3dbadb517df0f0": true,
}

// ensureProjectScaffold installs missing bundled templates and refreshes only
// known stock plan-first templates. Customized files are preserved and listed.
func ensureProjectScaffold(ctx context.Context, root string) ([]string, error) {
	var stale []string
	for _, item := range []struct {
		rel string
		old map[string]bool
	}{
		{"workflows/plan-first/WORKFLOW.md.tmpl", stockPlanFirstTemplateDigests},
		{"workflows/plan-first/prompts/orchestrator.md.tmpl", stockOrchestratorPromptDigests},
	} {
		target := filepath.Join(root, ".workspace", "templates", filepath.FromSlash(item.rel))
		b, err := os.ReadFile(target)
		if err == nil {
			if item.old[digest(b)] {
				embedded, readErr := templates.ReadFile("templates/" + item.rel)
				if readErr != nil {
					return nil, readErr
				}
				if writeErr := atomicWrite(target, embedded); writeErr != nil {
					return nil, writeErr
				}
			} else {
				stale = append(stale, item.rel)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
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
		return nil, err
	}
	ignore, err := git(ctx, root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(ignore) {
		ignore = filepath.Join(root, ignore)
	}
	b, err := os.ReadFile(ignore)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, rule := range []string{"/.workspace/ws_*/", "/.workspace/.runtime/", "/.workspace/.creating-*/", "/.workspace/issues/", "/.workspace/dispatcher/", "/work-products/"} {
		if !strings.Contains("\n"+string(b)+"\n", "\n"+rule+"\n") {
			b = append(b, []byte("\n"+rule+"\n")...)
		}
	}
	return stale, atomicWrite(ignore, b)
}

func InitProject(ctx context.Context, dir string, keys ...string) (Config, error) {
	root, err := resolveProjectRoot(ctx, dir)
	if err != nil {
		return Config{}, err
	}
	if key := mutationKey(keys); key != "" {
		return projectEffect(ctx, root, key, "project.init", func() (Config, error) { return InitProject(ctx, root) })
	}
	return initProjectLocked(ctx, root, nil)
}

// InitFreshProject initializes a fresh project from a proposed configuration.
// The candidate is validated with the same rules as Service.Config and the
// generated fields (schema_version, runtime and project_id) are filled in when
// omitted. An invalid candidate fails before any file is written. When a config
// already exists it is preserved and returned unchanged, so a concurrent or
// replayed initialization never replaces user configuration.
func InitFreshProject(ctx context.Context, dir string, candidate Config, keys ...string) (Config, error) {
	materialized, err := freshConfig(candidate)
	if err != nil {
		return Config{}, err
	}
	root, err := resolveProjectRoot(ctx, dir)
	if err != nil {
		return Config{}, err
	}
	if key := mutationKey(keys); key != "" {
		return projectEffect(ctx, root, key, []any{"project.init.fresh", candidate}, func() (Config, error) {
			return initProjectLocked(ctx, root, &materialized)
		})
	}
	return initProjectLocked(ctx, root, &materialized)
}

func freshConfig(candidate Config) (Config, error) {
	if candidate.SchemaVersion == 0 {
		candidate.SchemaVersion = 1
	}
	if candidate.Runtime == "" {
		candidate.Runtime = "tmux"
	}
	if candidate.ProjectID == "" {
		candidate.ProjectID = ID("prj")
	}
	if candidate.Clients == nil {
		candidate.Clients = map[string]Client{}
	}
	if candidate.Profiles == nil {
		candidate.Profiles = map[string]Profile{}
	}
	return ValidateConfig(candidate)
}

// initProjectLocked holds the project lock, repairs missing templates and
// exclude rules and installs the configuration. The candidate, when present,
// must already be materialized and validated. An existing configuration is
// always preserved byte-for-byte.
func initProjectLocked(ctx context.Context, root string, candidate *Config) (Config, error) {
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
	staleTemplates, err := ensureProjectScaffold(ctx, root)
	if err != nil {
		return Config{}, err
	}
	if existingConfig {
		cfg, err := s.Config()
		cfg.StaleTemplates = staleTemplates
		return cfg, err
	}
	cfg := Config{SchemaVersion: 1, ProjectID: ID("prj"), Runtime: "tmux", Clients: map[string]Client{}, Profiles: map[string]Profile{}}
	if candidate != nil {
		cfg = *candidate
	}
	cfg.StaleTemplates = staleTemplates
	b, err := yaml.Marshal(cfg)
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
	return ValidateConfig(cfg)
}

// ValidateConfig applies the same rules as Service.Config to an in-memory
// configuration: client normalization, route and profile references, workflow
// mappings and tracker/argv checks. It returns the normalized configuration and
// never writes to disk.
func ValidateConfig(cfg Config) (Config, error) {
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
		normalized, err := normalizeClient(client)
		if err != nil {
			return cfg, fail("invalid_config", "client %q: %v", name, err)
		}
		cfg.Clients[name] = normalized
	}
	for name, p := range cfg.Profiles {
		if p.Strategy != "" && p.Strategy != "provider-balanced" {
			return cfg, fail("invalid_config", "unsupported strategy in profile %q", name)
		}
		if p.ReasoningEffort != "" && strings.TrimSpace(p.ReasoningEffort) == "" {
			return cfg, fail("invalid_config", "reasoning_effort in profile %q must not be whitespace-only", name)
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
		if err := validateUsageLimits(p.UsageLimits); err != nil {
			return cfg, fail("invalid_config", "usage_limits in profile %q: %v", name, err)
		}
		if _, err := effectiveUsageLimits(p, Route{}); err != nil {
			return cfg, fail("invalid_config", "usage_limits in profile %q: %v", name, err)
		}
		for _, r := range p.Routes {
			if err := validateUsageLimits(r.UsageLimits); err != nil {
				return cfg, fail("invalid_config", "usage_limits in route %q of profile %q: %v", r.ID, name, err)
			}
			if _, err := effectiveUsageLimits(p, r); err != nil {
				return cfg, fail("invalid_config", "usage_limits in route %q of profile %q: %v", r.ID, name, err)
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
	if profile := cfg.Defaults.DispatcherProfile; profile != "" {
		if _, ok := cfg.Profiles[profile]; !ok {
			return cfg, fail("invalid_config", "defaults.dispatcher_profile references unknown profile %q", profile)
		}
	}
	for name, w := range cfg.Workflows {
		if w.MaxParallelTasks < 0 {
			return cfg, fail("invalid_config", "negative max_parallel_tasks in %s", name)
		}
		if w.ChangeRequests != "" && w.ChangeRequests != "integrated" && w.ChangeRequests != "per-task" {
			return cfg, fail("invalid_config", "change_requests must be integrated or per-task")
		}
		has := map[string]bool{}
		for _, capability := range w.Capabilities {
			if !knownWorkflowCapability(capability) {
				return cfg, fail("invalid_config", "workflow %s declares unknown capability %q", name, capability)
			}
			has[capability] = true
		}
		if has[capLanding] {
			if !has[capIntegration] {
				return cfg, fail("invalid_config", "workflow %s declares landing without integration", name)
			}
			if !has[capIntegrator] {
				return cfg, fail("invalid_config", "workflow %s declares landing without tasks.role.integrator", name)
			}
			if has[capChangeRequest] || has[capLiveTest] || has[capRelease] {
				return cfg, fail("invalid_config", "workflow %s declares landing together with change_request, live_test or release", name)
			}
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
	// issue-resolution was removed as a bundled workflow. An existing project
	// config may still list it: validate the entry as before (so a bad profile
	// reference is still reported) and then drop it from the normalized
	// configuration so listing, selection, routing and capability lookup can
	// no longer observe it. The config file on disk is never rewritten.
	delete(cfg.Workflows, "issue-resolution")
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

type CreateOptions struct {
	Title, Input, Source, Workflow, Base, OperationKey string
	IssueID, FromIssue                                 string
	IssueRevision                                      int
	IssueDigest                                        string
	OperationRequest                                   any `json:"-" yaml:"-"`
	// NoWorkflow creates an active workspace with a nil workflow for explicit
	// manual orchestration. It cannot be combined with Workflow and is kept in
	// the idempotency payload so retries cannot reinterpret the earlier choice.
	NoWorkflow bool `json:",omitempty"`
	// Autonomous starts the workspace in an autonomous run (state=running,
	// source=create). It is kept in the idempotency payload; omitempty keeps
	// existing receipt digests unchanged for non-autonomous creation.
	Autonomous bool `json:",omitempty"`
	// ID optionally overrides the generated slug for the new workspace. It is
	// omitted when empty so existing idempotency receipt digests do not change.
	ID string `json:",omitempty" yaml:",omitempty"`
}

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
	// profiles.
	return name == exampleWorkflow && workflowTemplateExists(root, name)
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
	if opt.OperationKey != "" {
		if replay, found, err := s.replayWorkspaceCreate(ctx, opt); err != nil {
			return Status{}, err
		} else if found {
			return replay, nil
		}
	}
	if opt.FromIssue != "" {
		return s.CreateFromIssue(ctx, opt.FromIssue, opt)
	}
	if opt.IssueID != "" {
		return s.CreateFromIssue(ctx, opt.IssueID, opt)
	}
	if opt.Source != "" {
		return s.CreateFromIssueURL(ctx, opt)
	}
	if err := s.requireWorkspaceScope(); err != nil {
		return Status{}, err
	}
	return s.createWorkspaceLegacy(ctx, opt, "", 0, "")
}

func (s *Service) createWorkspaceLegacy(ctx context.Context, opt CreateOptions, issueID string, issueRevision int, issueDigest string) (Status, error) {
	if opt.NoWorkflow && opt.Workflow != "" {
		return Status{}, fail("invalid_option", "--workflow and --no-workflow are mutually exclusive")
	}
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
		if opt.Workflow == "issue-resolution" {
			return Status{}, fail("unknown_workflow", "workflow issue-resolution was removed; use plan-first; available workflow: %s", strings.Join(WorkflowNames(s.Root, cfg), ", "))
		}
		return Status{}, fail("unknown_workflow", "available workflow: %s", strings.Join(WorkflowNames(s.Root, cfg), ", "))
	}
	dirs, err := s.workspaceDirs()
	if err != nil {
		return Status{}, err
	}
	takenIDs := map[string]bool{}
	for _, dir := range dirs {
		d, err := loadDocument(dir)
		if err != nil {
			return Status{}, err
		}
		takenIDs[d.State.ID] = true
		operationRequest := any(opt)
		if opt.OperationRequest != nil {
			operationRequest = opt.OperationRequest
		}
		id, err := d.previous("create:"+opt.OperationKey, operationRequest)
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
	if !opt.NoWorkflow && opt.Workflow == "" {
		opt.Workflow = exampleWorkflow
	}
	if opt.Autonomous {
		if err := requireAutonomySupport(cfg, opt.Workflow, opt.NoWorkflow); err != nil {
			return Status{}, err
		}
	}
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
	storage, err := s.storageRoot(cfg)
	if err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(storage, 0700); err != nil {
		return Status{}, err
	}
	takenWorkspace := func(candidate string) bool {
		candidateID := "ws_" + candidate
		if takenIDs[candidateID] {
			return true
		}
		if _, statErr := os.Stat(filepath.Join(storage, candidateID)); statErr == nil {
			return true
		}
		return false
	}
	workspaceIDs := idAllocator{storage: storage, kind: "ws", projectID: cfg.ProjectID, operation: reservationOperation(cfg.ProjectID, "", "ws", opt.OperationKey)}
	var id string
	if opt.ID != "" {
		slug, err := parseExplicitID("ws", opt.ID)
		if err != nil {
			return Status{}, err
		}
		id, err = workspaceIDs.allocateExplicit("ws", slug, takenWorkspace)
		if err != nil {
			return Status{}, err
		}
	} else {
		id, err = workspaceIDs.allocate("ws", baseSlug("ws", opt.Title), takenWorkspace)
		if err != nil {
			return Status{}, err
		}
	}
	dir, err := os.MkdirTemp(storage, ".creating-")
	if err != nil {
		return Status{}, err
	}
	defer os.RemoveAll(dir) // dir is an exclusively-created staging directory, never a worktree.
	status := "needs_workflow"
	if opt.NoWorkflow {
		status = "active"
	}
	d := &Document{Dir: dir, State: Workspace{SchemaVersion: 1, ID: id, ProjectID: cfg.ProjectID, Title: opt.Title, Revision: 1, Status: status, Input: Input{Source: opt.Source, Snapshot: "inputs/issue.md", IssueID: issueID, IssueRevision: issueRevision, IssueDigest: issueDigest}, Base: Base{baseRef, base}, CreatedAt: time.Now().UTC()}, Registry: Registry{SchemaVersion: registrySchemaVersion, Agents: []Agent{}, Worktrees: []Worktree{}, Sessions: []Session{}, Runs: []Run{}, Operations: map[string]Operation{}}}
	d.State.ProjectRoot = s.Root
	agentIDs := idAllocator{storage: storage, kind: "agent", projectID: cfg.ProjectID, workspaceID: id, operation: reservationOperation(cfg.ProjectID, id, "agent", opt.OperationKey)}
	orchID, err := agentIDs.allocate("agent", baseSlug("agent", "orchestrator"), nil)
	if err != nil {
		return Status{}, err
	}
	orch := Agent{ID: orchID, Name: "orchestrator", Role: "orchestrator", Profile: cfg.Defaults.OrchestratorProfile, PromptTemplate: "orchestrator", Instructions: "Coordinate the workflow; delegate all code changes to workers.", Scope: "workspace"}
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
	if opt.Autonomous {
		enabledAt := nowUTC()
		d.State.Autonomy = &Autonomy{Mode: "autonomous", State: "running", EnabledAt: &enabledAt, EnabledRevision: d.State.Revision, Source: "create"}
	}
	if err := s.snapshotTemplates(d, opt.Workflow, d.State.Manual()); err != nil {
		return Status{}, err
	}
	if d.State.WorkflowSelected() {
		orch.Profile = workflowProfile(cfg, d, "orchestrator", orch.Profile)
		d.Registry.Agents[0] = orch
	}
	body, err := s.render("WORKSPACE.md.tmpl", agentPromptData{Workspace: d.State, Manual: d.State.Manual()})
	if err != nil {
		return Status{}, err
	}
	d.Body = string(body)
	if opt.OperationKey != "" {
		rememberRequest := any(request)
		if opt.OperationRequest != nil {
			rememberRequest = opt.OperationRequest
		}
		d.remember("create:"+opt.OperationKey, rememberRequest, id)
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
		if _, statErr := os.Stat(final); statErr == nil {
			return Status{}, fail("id_exists", "workspace %s already exists", id)
		}
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

// agentPromptData carries the workspace state plus the creation mode so shared
// role instructions can avoid referencing a workflow that is intentionally absent.
type agentPromptData struct {
	Workspace
	Manual bool
}

func (s *Service) snapshotTemplates(d *Document, workflow string, manual bool) error {
	data := agentPromptData{Workspace: d.State, Manual: manual}
	a, err := s.render("orchestrator.AGENTS.md.tmpl", data)
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(d.Dir, "AGENTS.md"), a); err != nil {
		return err
	}
	worker, err := s.render("worker.AGENTS.md.tmpl", data)
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(d.Dir, "prompts", "worker.AGENTS.md"), worker); err != nil {
		return err
	}
	w := []byte("# Select a workflow\n\nAsk the user to select one of the available workflows before delegating work.\n")
	promptDir := filepath.Join(s.Root, ".workspace", "templates", "workflows", workflow)
	switch {
	case manual:
		promptDir = filepath.Join(s.Root, ".workspace", "templates", "manual")
		w, err = s.render("manual/WORKFLOW.md.tmpl", d.State)
		if err != nil {
			return err
		}
	case workflow != "":
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		d.State.ChangeRequestMode = cfg.Workflows[workflow].ChangeRequests
		workflowPath := "workflows/" + workflow + "/WORKFLOW.md.tmpl"
		workflowTemplate, err := s.workflowTemplate(workflowPath)
		if err != nil {
			return err
		}
		w, err = renderTemplate(workflowPath, workflowTemplate, d.State)
		if err != nil {
			return err
		}
		d.State.Workflow = &Workflow{ID: workflow, Version: 1, TemplateDigest: digest(w), Phase: "planning", Capabilities: workflowConfigCapabilities(workflow, cfg)}
		d.State.Status = "active"
	default:
		promptDir = filepath.Join(s.Root, ".workspace", "templates", "workflows", exampleWorkflow)
	}
	if err := atomicWrite(filepath.Join(d.Dir, "WORKFLOW.md"), w); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(promptDir, "prompts"))
	if err != nil {
		return err
	}
	if workflow == exampleWorkflow {
		embeddedEntries, readErr := fs.ReadDir(templates, "templates/workflows/plan-first/prompts")
		if readErr != nil {
			return readErr
		}
		present := make(map[string]bool, len(entries))
		for _, entry := range entries {
			present[entry.Name()] = true
		}
		for _, entry := range embeddedEntries {
			if !present[entry.Name()] {
				entries = append(entries, entry)
			}
		}
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md.tmpl") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md.tmpl")
		b, readErr := os.ReadFile(filepath.Join(promptDir, "prompts", entry.Name()))
		if readErr != nil && workflow == exampleWorkflow {
			b, readErr = templates.ReadFile("templates/workflows/plan-first/prompts/" + entry.Name())
		}
		if readErr != nil {
			return readErr
		}
		if err := atomicWrite(filepath.Join(d.Dir, "prompts", name+".md.tmpl"), b); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) workflowTemplate(path string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(s.Root, ".workspace", "templates", filepath.FromSlash(path)))
	if err == nil {
		if path == "workflows/plan-first/WORKFLOW.md.tmpl" && stockPlanFirstTemplateDigests[digest(b)] {
			return templates.ReadFile("templates/" + path)
		}
		return b, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return templates.ReadFile("templates/" + path)
}

func renderTemplate(name string, b []byte, data any) ([]byte, error) {
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
	return s.selectWorkflow(ctx, selector, name, 0, keys...)
}

func (s *Service) SelectWorkflowGuarded(ctx context.Context, selector, name, key string, expectedRevision int) (Status, error) {
	return s.selectWorkflow(ctx, selector, name, expectedRevision, key)
}

func (s *Service) selectWorkflow(ctx context.Context, selector, name string, expectedRevision int, keys ...string) (Status, error) {
	var out Status
	var request any = []any{"workflow.select", name}
	if expectedRevision != 0 {
		request = []any{"workflow.select", name, expectedRevision}
	}
	err := mutate(s, ctx, selector, keys, request, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if expectedRevision != 0 && d.State.Revision != expectedRevision {
			return fail("revision_conflict", "workspace changed while the action was being confirmed")
		}
		if err := rejectNewWorkspaceWork(d, "selecting a workflow"); err != nil {
			return err
		}
		if d.State.Manual() {
			return fail("operation_not_applicable", "workflow selection is not available in a manual workspace")
		}
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		if !workflowAvailable(s.Root, cfg, name) {
			return fail("unknown_workflow", "available workflow: %s", strings.Join(WorkflowNames(s.Root, cfg), ", "))
		}
		if d.State.WorkflowSelected() {
			if d.State.Workflow.ID != name {
				return fail("workflow_conflict", "workflow already selected")
			}
			out = d.Status()
			return nil
		}
		// Snapshot files first; repeating selection after interruption is safe.
		if err := s.snapshotTemplates(d, name, false); err != nil {
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
	return s.setPaused(ctx, selector, paused, MutationGuard{}, keys...)
}

// SetPausedGuarded protects a pause/resume confirmation with the revision the
// user saw. The guard is checked after an idempotent receipt replay and under
// the same lock as the state change.
func (s *Service) SetPausedGuarded(ctx context.Context, selector string, paused bool, key string, guard MutationGuard) (Status, error) {
	return s.setPaused(ctx, selector, paused, guard, key)
}

func (s *Service) setPaused(ctx context.Context, selector string, paused bool, guard MutationGuard, keys ...string) (Status, error) {
	var out Status
	var request any = []any{"set-paused", paused}
	if !guard.empty() {
		request = struct {
			Paused bool
			Guard  MutationGuard
		}{paused, guard}
	}
	err := mutate(s, ctx, selector, keys, request, &out, s.requireOrchestrator, func(d *Document) error {
		if err := s.requireOrchestrator(d); err != nil {
			return err
		}
		if guard.ExpectedRevision != 0 && d.State.Revision != guard.ExpectedRevision {
			return fail("revision_conflict", "workspace changed while the action was being confirmed")
		}
		if d.State.Status == "completed" {
			return fail("workspace_completed", "completed workspace requires workspace reopen; workspace resume applies only to paused workspaces")
		}
		if d.State.Status == "archived" {
			return fail("workspace_archived", "archived workspace cannot be resumed or paused")
		}
		if d.State.NeedsWorkflow() {
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
