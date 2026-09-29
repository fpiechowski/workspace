package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"workspace/internal/buildinfo"
	"workspace/internal/core"
	"workspace/internal/tui"
)

func TestProfileListIncludesReasoningEffort(t *testing.T) {
	project := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := core.InitProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	s := &core.Service{Root: project}
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clients = map[string]core.Client{"test": {Adapter: "command", LaunchArgv: []string{"wrapper", "{prompt}"}}}
	cfg.Profiles = map[string]core.Profile{"worker": {ReasoningEffort: "high", Routes: []core.Route{{ID: "worker", Client: "test", Provider: "provider", Model: "provider/model", MaxConcurrency: 1}}}}
	config, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".workspace", "config.yaml"), config, 0600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Execute([]string{"--json", "--project", project, "profile", "list"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("profile list failed: %d %s", code, errOut.String())
	}
	var response struct {
		OK   bool                    `json:"ok"`
		Data map[string]core.Profile `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Data["worker"].ReasoningEffort != "high" {
		t.Fatalf("profile list omitted reasoning_effort: %s", out.String())
	}
}

func TestStructuredErrorsAndWorkflowDiscovery(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Execute([]string{"--json", "workflow", "list"}, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("%s", errOut.String())
	}
	var response struct {
		OK   bool `json:"ok"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	planFirst := false
	for _, workflow := range response.Data {
		planFirst = planFirst || workflow.ID == "plan-first"
	}
	if !response.OK || !planFirst {
		t.Fatalf("invalid workflow list: %s", out.String())
	}
	out.Reset()
	errOut.Reset()
	code = Execute([]string{"--json", "--project", t.TempDir(), "status"}, nil, &out, &errOut)
	if code != 1 || !bytes.Contains(out.Bytes(), []byte(`"code":"project_not_found"`)) {
		t.Fatalf("expected structured error: %d %s", code, out.String())
	}
}

func TestVersionCommandWorksOutsideAProject(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Execute([]string{"--json", "version"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("version failed: %d %s", code, errOut.String())
	}
	var response struct {
		OK   bool           `json:"ok"`
		Data buildinfo.Info `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Data.Version == "" || response.Data.GOOS == "" || response.Data.GOARCH == "" {
		t.Fatalf("invalid version response: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("version wrote stderr: %s", errOut.String())
	}
}

func TestPrimeCommandWorksOutsideAProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Execute([]string{"prime"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("prime failed: %d %s", code, errOut.String())
	}
	raw := out.String()
	if raw == "" || !strings.HasSuffix(raw, "\n") {
		t.Fatalf("prime did not return Markdown with a final newline: %q", raw)
	}
	if strings.HasPrefix(strings.TrimSpace(raw), "---") || strings.HasPrefix(strings.TrimSpace(raw), "{") {
		t.Fatalf("prime returned front matter or JSON instead of raw Markdown: %q", raw[:min(len(raw), 80)])
	}
	if !strings.HasPrefix(strings.TrimLeft(raw, "\r\n"), "# Workspace") {
		t.Fatalf("prime returned unexpected Markdown: %q", raw[:min(len(raw), 80)])
	}
	if errOut.Len() != 0 {
		t.Fatalf("prime wrote stderr: %s", errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Execute([]string{"--short", "prime"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("short prime failed: %d %s", code, errOut.String())
	}
	if out.String() != raw {
		t.Fatal("--short truncated or changed prime instructions")
	}

	out.Reset()
	errOut.Reset()
	if code := Execute([]string{"--json", "prime"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("JSON prime failed: %d %s", code, errOut.String())
	}
	var response struct {
		OK   bool `json:"ok"`
		Data struct {
			Instructions string `json:"instructions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Data.Instructions != raw {
		t.Fatalf("JSON prime did not expose the raw instructions: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("JSON prime wrote stderr: %s", errOut.String())
	}

	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("prime created files in an uninitialized directory: before=%v after=%v", before, after)
	}
}

func TestPrimeHelpDescribesBundledGuidance(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Execute([]string{"prime", "--help"}, nil, &out, &errOut); code != 0 || errOut.Len() != 0 {
		t.Fatalf("prime help failed: %d %s", code, errOut.String())
	}
	for _, want := range []string{"bundled", "context compaction", "without a project", "live workspace state"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("prime help lacks %q:\n%s", want, out.String())
		}
	}
}

func TestCreateAcceptsIntentArgument(t *testing.T) {
	project := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := core.InitProject(ctx, project); err != nil {
		t.Fatal(err)
	}

	const intent = "create workspace improvements"
	var out, errOut bytes.Buffer
	code := Execute([]string{"--json", "--project", project, "create", intent}, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("create failed: %d %s", code, errOut.String())
	}
	var response struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Data.Directory == "" {
		t.Fatalf("invalid create response: %s", out.String())
	}
	input, err := os.ReadFile(filepath.Join(response.Data.Directory, "inputs", "issue.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(input) != intent {
		t.Fatalf("saved intent %q, want %q", input, intent)
	}

	out.Reset()
	errOut.Reset()
	code = Execute([]string{"--json", "--project", project, "create", "--", "help"}, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("literal help argument failed: %d %s", code, errOut.String())
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	literalInput, err := os.ReadFile(filepath.Join(response.Data.Directory, "inputs", "issue.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(literalInput) != "help" {
		t.Fatalf("saved literal help %q, want help", literalInput)
	}
}

func TestCreateSupportsExplicitNoWorkflowMode(t *testing.T) {
	project := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := core.InitProject(ctx, project); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := Execute([]string{"--json", "--project", project, "create", "--no-workflow", "manual orchestration"}, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("manual create failed: %d %s", code, errOut.String())
	}
	var response struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Data.Workspace.Status != "active" || response.Data.Workspace.Workflow != nil || !response.Data.Workspace.Manual() {
		t.Fatalf("manual create response: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	code = Execute([]string{"--json", "--project", project, "create", "--workflow", "plan-first", "--no-workflow", "conflict"}, nil, &out, &errOut)
	if code != 1 {
		t.Fatalf("conflicting create succeeded: %d %s", code, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"code":"invalid_option"`)) {
		t.Fatalf("conflicting create did not report invalid_option: %s", out.String())
	}
}

func TestWorkspaceShortNamesAndSelector(t *testing.T) {
	workspaces := []core.Status{
		{Workspace: core.Workspace{ID: "ws_one", Title: "First", Status: "active"}},
		{Workspace: core.Workspace{ID: "ws_two", Title: "Second", Status: "paused"}},
	}
	short := workspaceNameIDs(workspaces)
	if short["First"] != "ws_one" || short["Second"] != "ws_two" {
		t.Fatalf("unexpected short workspace map: %#v", short)
	}

	o := &options{in: strings.NewReader("2\n"), out: &bytes.Buffer{}}
	selected, err := chooseWorkspace(o, workspaces, "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Workspace.ID != "ws_two" {
		t.Fatalf("selected %s, want ws_two", selected.Workspace.ID)
	}
	selected, err = chooseWorkspace(o, workspaces, "First")
	if err != nil || selected.Workspace.ID != "ws_one" {
		t.Fatalf("title lookup: %v %#v", err, selected)
	}
}

func TestWorkspacePickerShowsManualModeLabel(t *testing.T) {
	workspaces := []core.Status{
		{Workspace: core.Workspace{ID: "ws_manual", Title: "Manual", Status: "active"}},
		{Workspace: core.Workspace{ID: "ws_pending", Title: "Pending", Status: "needs_workflow"}},
		{Workspace: core.Workspace{ID: "ws_flow", Title: "Flow", Status: "active", Workflow: &core.Workflow{ID: "extended", Phase: "planning"}}},
	}
	out := &bytes.Buffer{}
	o := &options{in: strings.NewReader("q\n"), out: out}
	if _, err := chooseWorkspace(o, workspaces, ""); err == nil {
		t.Fatal("cancel should stop selection")
	}
	text := out.String()
	for _, want := range []string{"[active / manual]", "[needs_workflow / -]", "[active / planning]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("picker missing %q:\n%s", want, text)
		}
	}
}

func TestSessionCompactOutputIncludesLogicalAndRunSummary(t *testing.T) {
	value := shortOutput([]core.Session{{ID: "sess_1", AgentID: "agent_1", TaskID: "task_1", LifecycleState: "active", State: "idle", LastRunID: "run_3", RunCount: 3, RunState: "running", ClientState: "idle", ClientThreadID: "thread_1"}})
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"id":"sess_1"`, `"lifecycle_state":"active"`, `"state":"idle"`, `"last_run_id":"run_3"`, `"run_count":3`, `"run_state":"running"`, `"client_state":"idle"`, `"client_thread_id":"thread_1"`} {
		if !bytes.Contains(b, []byte(field)) {
			t.Fatalf("compact session output omitted %s: %s", field, b)
		}
	}
	root := newRoot(&options{})
	for _, path := range []string{"session history", "run list", "run inspect"} {
		parts := strings.Split(path, " ")
		if cmd, _, err := root.Find(parts); err != nil || cmd == nil {
			t.Fatalf("missing command %s: %v", path, err)
		}
	}
}

func TestShortIsAvailableForEveryCommand(t *testing.T) {
	root := newRoot(&options{})
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		if cmd.HasSubCommands() {
			for _, child := range cmd.Commands() {
				visit(child)
			}
		}
		if cmd.Runnable() && cmd.Flag("short") == nil {
			t.Errorf("%s does not inherit --short", cmd.CommandPath())
		}
	}
	visit(root)
}

func TestShortOutputCompactsStatusesAndNamedResources(t *testing.T) {
	status := core.Status{Workspace: core.Workspace{ID: "ws_one", Title: "First", Status: "active"}}
	if got := shortOutput(status); !reflect.DeepEqual(got, map[string]string{"First": "ws_one"}) {
		t.Fatalf("unexpected compact status: %#v", got)
	}

	agents := []core.Agent{
		{ID: "agent_one", Name: "Planner", Role: "planner"},
		{ID: "agent_two", Name: "Builder", Role: "implementer"},
	}
	want := map[string]string{"Planner": "agent_one", "Builder": "agent_two"}
	if got := shortOutput(agents); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected compact agents: %#v", got)
	}
}

func TestHelpIsAvailableAtEveryCommandLevel(t *testing.T) {
	root := newRoot(&options{})
	if missing := missingHelpDocumentation(root); len(missing) != 0 {
		t.Fatalf("missing help documentation for: %s", strings.Join(missing, ", "))
	}
	for path := range commandHelpSpecs {
		if commandAtPath(root, path) == nil {
			t.Errorf("help documentation refers to unknown command %s", path)
		}
	}

	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.IsAvailableCommand() || cmd == root {
			if strings.TrimSpace(cmd.Long) == "" {
				t.Errorf("%s has no long help description", cmd.CommandPath())
			}
			if useHasPositionalArguments(cmd.Use) {
				if cmd.Annotations == nil || strings.TrimSpace(cmd.Annotations["workspace.arguments"]) == "" {
					t.Errorf("%s has positional arguments without documentation", cmd.CommandPath())
				}
			}
			cmd.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
				if strings.TrimSpace(flag.Usage) == "" {
					t.Errorf("%s --%s has an empty flag usage", cmd.CommandPath(), flag.Name)
				}
			})
		}
		for _, child := range cmd.Commands() {
			if child.IsAvailableCommand() {
				walk(child)
			}
		}
	}
	walk(root)
}

func TestHelpFlagSpecsReferToRegisteredFlags(t *testing.T) {
	root := newRoot(&options{})
	for path, flags := range flagHelpSpecs {
		cmd := commandAtPath(root, path)
		if cmd == nil {
			t.Errorf("flag help refers to unknown command %s", path)
			continue
		}
		for name := range flags {
			if cmd.Flags().Lookup(name) == nil && cmd.PersistentFlags().Lookup(name) == nil {
				t.Errorf("flag help refers to unknown flag %s on %s", name, path)
			}
		}
	}
}

func useHasPositionalArguments(use string) bool {
	for _, token := range strings.Fields(use)[1:] {
		if token == "[flags]" {
			continue
		}
		if strings.HasPrefix(token, "<") || strings.HasPrefix(token, "[") {
			return true
		}
	}
	return false
}

func TestHelpFormsAreEquivalentAndDoNotRunCommands(t *testing.T) {
	forms := [][]string{
		{"session", "start", "help"},
		{"session", "start", "--help"},
		{"help", "session", "start"},
	}
	outputs := make([]string, len(forms))
	for i, args := range forms {
		var out, errOut bytes.Buffer
		code := Execute(args, nil, &out, &errOut)
		if code != 0 {
			t.Fatalf("%v returned %d: %s", args, code, errOut.String())
		}
		if errOut.Len() != 0 {
			t.Fatalf("%v wrote an error: %s", args, errOut.String())
		}
		outputs[i] = out.String()
	}
	if outputs[0] != outputs[1] || outputs[1] != outputs[2] {
		t.Fatalf("help forms differ:\n suffix:\n%s\n flag:\n%s\n prefix:\n%s", outputs[0], outputs[1], outputs[2])
	}
	for _, section := range []string{"Usage:", "Examples:", "Options:", "Global options:"} {
		if !strings.Contains(outputs[0], section) {
			t.Errorf("session start help lacks %q", section)
		}
	}
	var jsonOut, jsonErr bytes.Buffer
	if code := Execute([]string{"--json", "session", "start", "help"}, nil, &jsonOut, &jsonErr); code != 0 || jsonErr.Len() != 0 {
		t.Fatalf("JSON help returned %d: %s", code, jsonErr.String())
	}
	if strings.HasPrefix(strings.TrimSpace(jsonOut.String()), "{") {
		t.Fatal("help unexpectedly used the command JSON envelope")
	}

	var groupOutputs [2]string
	for i, args := range [][]string{{"session", "help"}, {"help", "session"}} {
		var out, errOut bytes.Buffer
		if code := Execute(args, nil, &out, &errOut); code != 0 || errOut.Len() != 0 {
			t.Fatalf("%v returned %d: %s", args, code, errOut.String())
		}
		groupOutputs[i] = out.String()
	}
	if groupOutputs[0] != groupOutputs[1] {
		t.Fatal("group help forms differ")
	}
}

func TestHelpIncludesPositionalArgumentDocumentation(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Execute([]string{"create", "--help"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("create help returned %d: %s", code, errOut.String())
	}
	for _, want := range []string{
		"Arguments:",
		"[intent]  Issue or task description supplied inline",
		"--input-file string",
		"Global options:",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("create help lacks %q:\n%s", want, out.String())
		}
	}
	var rootOut, rootErr bytes.Buffer
	if code := Execute([]string{"--help"}, nil, &rootOut, &rootErr); code != 0 || rootErr.Len() != 0 {
		t.Fatalf("root help returned %d: %s", code, rootErr.String())
	}
	if strings.Contains(rootOut.String(), "_session-exec") || strings.Contains(rootOut.String(), "_service-exec") {
		t.Fatal("internal runner leaked into user help")
	}
}

func TestHelpRendersForEveryVisibleCommand(t *testing.T) {
	root := newRoot(&options{})
	for _, path := range visibleCommandPaths(root) {
		parts := strings.Fields(path)
		args := append(append([]string(nil), parts[1:]...), "--help")
		var out, errOut bytes.Buffer
		if code := Execute(args, nil, &out, &errOut); code != 0 || errOut.Len() != 0 {
			t.Errorf("%s help returned %d: %s", path, code, errOut.String())
		}
		if strings.TrimSpace(out.String()) == "" {
			t.Errorf("%s help was empty", path)
		}
	}
}

func TestHelpSuffixPrecedenceAndLiteralEscape(t *testing.T) {
	if got := normalizeHelpArgs([]string{"create", "help"}); !reflect.DeepEqual(got, []string{"create", "--help"}) {
		t.Fatalf("suffix help not normalized: %#v", got)
	}
	if got := normalizeHelpArgs([]string{"create", "--", "help"}); !reflect.DeepEqual(got, []string{"create", "--", "help"}) {
		t.Fatalf("literal help was not preserved after --: %#v", got)
	}
	if got := normalizeHelpArgs([]string{"session", "start", "--profile=help"}); !reflect.DeepEqual(got, []string{"session", "start", "--profile=help"}) {
		t.Fatalf("equals-form flag value was changed: %#v", got)
	}
	if got := normalizeHelpArgs([]string{"_session-exec", "help"}); !reflect.DeepEqual(got, []string{"_session-exec", "help"}) {
		t.Fatalf("internal runner arguments were changed: %#v", got)
	}
}

func TestCompleteAndArchiveManualWorkspace(t *testing.T) {
	project := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := core.InitProject(ctx, project); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Execute([]string{"--json", "--project", project, "create", "--no-workflow", "manual completion CLI"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("manual create failed: %d %s", code, errOut.String())
	}
	var created struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	ws := created.Data.Workspace.ID
	revision := created.Data.Workspace.Revision

	out.Reset()
	errOut.Reset()
	args := []string{
		"--json", "--project", project, "complete", "--workspace", ws,
		"--reason", "analysis delivered", "--expected-revision", strconv.Itoa(revision),
		"--operation-key", "complete:1",
	}
	if code := Execute(args, nil, &out, &errOut); code != 0 {
		t.Fatalf("complete failed: %d %s", code, errOut.String())
	}
	var completed struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &completed); err != nil {
		t.Fatal(err)
	}
	if !completed.OK || completed.Data.Workspace.Status != "completed" || !completed.Data.Workspace.Manual() {
		t.Fatalf("complete response: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Execute([]string{"--json", "--project", project, "archive", "--workspace", ws, "--operation-key", "archive:1"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("archive failed: %d %s", code, errOut.String())
	}
	var archived struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &archived); err != nil {
		t.Fatal(err)
	}
	if archived.Data.Workspace.Status != "archived" {
		t.Fatalf("archive response: %s", out.String())
	}
}

func TestReopenJSONAndHelpContract(t *testing.T) {
	project := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := core.InitProject(ctx, project); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Execute([]string{"--json", "--project", project, "create", "--no-workflow", "reopen CLI"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("create failed: %d %s", code, errOut.String())
	}
	var created struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !created.OK {
		t.Fatalf("create response was not successful: %s", out.String())
	}
	ws := created.Data.Workspace.ID
	out.Reset()
	errOut.Reset()
	if code := Execute([]string{
		"--json", "--project", project, "complete", "--workspace", ws,
		"--reason", "initial delivery", "--expected-revision", strconv.Itoa(created.Data.Workspace.Revision),
		"--operation-key", "complete:reopen-cli",
	}, nil, &out, &errOut); code != 0 {
		t.Fatalf("complete failed: %d %s", code, errOut.String())
	}
	var completed struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &completed); err != nil {
		t.Fatal(err)
	}
	if !completed.OK || completed.Data.Workspace.Status != "completed" {
		t.Fatalf("complete response: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	reopenArgs := []string{
		"--json", "--project", project, "reopen", "--workspace", ws,
		"--reason", "User requested a follow-up", "--expected-revision", strconv.Itoa(completed.Data.Workspace.Revision),
		"--operation-key", "reopen:cli",
	}
	if code := Execute(reopenArgs, nil, &out, &errOut); code != 0 {
		t.Fatalf("reopen failed: %d %s", code, errOut.String())
	}
	var reopened struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &reopened); err != nil {
		t.Fatal(err)
	}
	if !reopened.OK || reopened.Data.Workspace.Status != "active" || !reopened.Data.Workspace.Manual() {
		t.Fatalf("reopen response: %s", out.String())
	}
	reopenRevision := reopened.Data.Workspace.Revision

	out.Reset()
	errOut.Reset()
	if code := Execute(reopenArgs, nil, &out, &errOut); code != 0 {
		t.Fatalf("reopen replay failed: %d %s", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"revision":`+strconv.Itoa(reopenRevision))) {
		t.Fatalf("reopen replay did not return the original result: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	changed := append([]string(nil), reopenArgs...)
	for i := range changed {
		if changed[i] == "User requested a follow-up" {
			changed[i] = "different reason"
		}
	}
	if code := Execute(changed, nil, &out, &errOut); code != 1 || !bytes.Contains(out.Bytes(), []byte(`"code":"operation_conflict"`)) {
		t.Fatalf("changed replay did not return operation_conflict: code=%d output=%s", code, out.String())
	}

	var helpOut, helpErr bytes.Buffer
	if code := Execute([]string{"help", "reopen"}, nil, &helpOut, &helpErr); code != 0 || helpErr.Len() != 0 {
		t.Fatalf("reopen help failed: %d %s", code, helpErr.String())
	}
	for _, want := range []string{"workspace reopen", "--reason", "--expected-revision", "history/reopen_ID"} {
		if !strings.Contains(helpOut.String(), want) {
			t.Errorf("reopen help lacks %q:\n%s", want, helpOut.String())
		}
	}
}

type cliFakeRuntime struct {
	launches int
	panes    map[string]core.Pane
}

func (r *cliFakeRuntime) Launch(_ context.Context, l core.Launch) (core.Pane, error) {
	r.launches++
	if r.panes == nil {
		r.panes = map[string]core.Pane{}
	}
	p := core.Pane{ID: "cli%" + strconv.Itoa(r.launches), SessionID: l.SessionID, RunID: l.RunID, WorkspaceID: l.WorkspaceID}
	r.panes[p.ID] = p
	return p, nil
}
func (r *cliFakeRuntime) Inspect(_ context.Context, id string) (core.Pane, error) {
	p, ok := r.panes[id]
	if !ok {
		return core.Pane{}, &core.Error{Code: "pane_missing", Message: "missing pane"}
	}
	return p, nil
}
func (r *cliFakeRuntime) Stop(_ context.Context, id string) error { delete(r.panes, id); return nil }
func (r *cliFakeRuntime) StopWorkspace(_ context.Context, workspaceID string) error {
	for id, p := range r.panes {
		if p.WorkspaceID == workspaceID {
			delete(r.panes, id)
		}
	}
	return nil
}
func (r *cliFakeRuntime) Attach(context.Context, string, string) error { return nil }

func cliGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func cliCommit(t *testing.T, dir, message string) {
	t.Helper()
	cliGit(t, dir, "-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", message)
}

func cliFinishTask(t *testing.T, s *core.Service, ws string, session core.Session, worktree core.Worktree, artifact string) {
	t.Helper()
	ctx := context.Background()
	dir := filepath.Join(worktree.Path, "work-products")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{artifact, "CHECKS.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("Verified CLI fixture result\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h, err := s.SubmitHandoff(ctx, ws, core.HandoffOptions{
		Session:   session.ID,
		Summary:   "Completed",
		Artifacts: []string{"work-products/" + artifact, "work-products/CHECKS.log"},
		Checks:    []core.Check{{Command: "fixture checks", ExitCode: 0, Evidence: "CHECKS.log"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewHandoff(ctx, ws, h.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, ws, session.ID); err != nil {
		t.Fatal(err)
	}
}

func cliCreateTask(t *testing.T, s *core.Service, ws, name, role string, deps []string) core.Task {
	t.Helper()
	task, err := s.CreateTask(context.Background(), ws, core.TaskSpec{Name: name, Title: name, Goal: "Bounded CLI task", Role: role, DependsOn: deps, AcceptanceCriteria: []string{"Produce an inspected result"}}, "cli-task:"+name)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func cliStartTask(t *testing.T, s *core.Service, ws string, task core.Task, worktreeID string) (core.Session, core.Worktree) {
	t.Helper()
	ctx := context.Background()
	prompt := map[string]string{"planner": "planning", "implementer": "implementation", "integrator": "implementation"}[task.Role]
	agent, err := s.CreateAgent(ctx, ws, core.AgentOptions{Name: task.Name, Role: task.Role, PromptTemplate: prompt})
	if err != nil {
		t.Fatal(err)
	}
	var worktree core.Worktree
	if worktreeID == "" {
		worktree, err = s.CreateWorktree(ctx, ws, core.WorktreeOptions{Name: task.Name})
		if err != nil {
			t.Fatal(err)
		}
	} else {
		status, statusErr := s.Status(ctx, ws)
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		for _, candidate := range status.Worktrees {
			if candidate.ID == worktreeID {
				worktree = candidate
			}
		}
		if worktree.ID == "" {
			t.Fatalf("worktree %s missing", worktreeID)
		}
	}
	session, err := s.StartSession(ctx, ws, core.SessionOptions{Agent: agent.ID, Worktree: worktree.ID, Task: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	return session, worktree
}

// TestIntegrationLandJSON drives a plan-first v2 workspace to an accepted
// integration and lands it through the real CLI command, checking the JSON
// contract and the compare-and-swap ref update on an unchecked-out target.
func TestIntegrationLandJSON(t *testing.T) {
	project := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = project
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if _, err := core.InitProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	s := &core.Service{Root: project, Runtime: &cliFakeRuntime{}, Executable: os.Args[0]}
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clients = map[string]core.Client{"test": {Adapter: "command", LaunchArgv: []string{os.Args[0], "{prompt}"}}}
	cfg.Profiles = map[string]core.Profile{}
	cfg.Defaults.OrchestratorProfile = "frontier"
	for _, name := range []string{"frontier", "implementation", "live-testing"} {
		cfg.Profiles[name] = core.Profile{Routes: []core.Route{{ID: name + "-a", Client: "test", Provider: "a", Model: "test-model", MaxConcurrency: 8}}}
	}
	config, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".workspace", "config.yaml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	created, err := s.Create(ctx, core.CreateOptions{Title: "CLI landing", Input: "Land a change", Workflow: "plan-first", OperationKey: "cli-landing"})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	cliGit(t, project, "branch", "release")

	plan := cliCreateTask(t, s, ws, "plan", "planner", nil)
	pp, pw := cliStartTask(t, s, ws, plan, "")
	cliFinishTask(t, s, ws, pp, pw, "PLAN.md")
	if _, err := s.AdvanceWorkflow(ctx, ws, "plan_review", ""); err != nil {
		t.Fatal(err)
	}
	impl := cliCreateTask(t, s, ws, "implementation", "implementer", []string{plan.ID})
	if _, err := s.AdvanceWorkflow(ctx, ws, "implementing", ""); err != nil {
		t.Fatal(err)
	}
	ip, iw := cliStartTask(t, s, ws, impl, "")
	if err := os.WriteFile(filepath.Join(iw.Path, "fix.txt"), []byte("implementation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cliGit(t, iw.Path, "add", "fix.txt")
	cliGit(t, iw.Path, "-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fix issue")
	cliFinishTask(t, s, ws, ip, iw, "IMPLEMENTATION.md")
	if _, err := s.AdvanceWorkflow(ctx, ws, "integration", ""); err != nil {
		t.Fatal(err)
	}
	i, err := s.PrepareIntegration(ctx, ws, core.IntegrationOptions{Target: "release", OperationKey: "cli-integration"})
	if err != nil {
		t.Fatal(err)
	}
	integrator := cliCreateTask(t, s, ws, "integrator", "integrator", []string{impl.ID})
	is, iwt := cliStartTask(t, s, ws, integrator, i.WorktreeID)
	integrationWorktree := ""
	status, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	for _, wt := range status.Worktrees {
		if wt.ID == i.WorktreeID {
			integrationWorktree = wt.Path
		}
	}
	if integrationWorktree == "" {
		t.Fatal("integration worktree missing")
	}
	cliGit(t, integrationWorktree, "-c", "user.name=Workspace Test", "-c", "user.email=test@example.invalid", "merge", "--no-ff", "--no-edit", i.Heads[0])
	cliFinishTask(t, s, ws, is, iwt, "INTEGRATION.md")

	status, err = s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if status.Workspace.Integration == nil || status.Workspace.Integration.HeadCommit == "" {
		t.Fatal("integration was not accepted")
	}
	headCommit := status.Workspace.Integration.HeadCommit
	var out, errOut bytes.Buffer
	if code := Execute([]string{
		"--json", "--project", project, "integration", "land", "--workspace", ws,
		"--target", "release", "--user-confirmed",
		"--expected-revision", strconv.Itoa(status.Workspace.Revision),
		"--operation-key", "cli-land-op",
	}, nil, &out, &errOut); code != 0 {
		t.Fatalf("integration land failed: %d %s", code, errOut.String())
	}
	var landed struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &landed); err != nil {
		t.Fatal(err)
	}
	if !landed.OK || landed.Data.Workspace.Integration == nil || landed.Data.Workspace.Integration.Landing == nil || landed.Data.Workspace.Integration.Landing.State != "landed" {
		t.Fatalf("land response: %s", out.String())
	}
	if got := cliGit(t, project, "rev-parse", "refs/heads/release"); got != headCommit {
		t.Fatalf("release ref: got %s want %s", got, headCommit)
	}

	var helpOut, helpErr bytes.Buffer
	if code := Execute([]string{"help", "integration", "land"}, nil, &helpOut, &helpErr); code != 0 || helpErr.Len() != 0 {
		t.Fatalf("integration land help failed: %d %s", code, helpErr.String())
	}
	for _, want := range []string{"integration land", "--target", "--expected-revision", "--user-confirmed"} {
		if !strings.Contains(helpOut.String(), want) {
			t.Errorf("integration land help lacks %q:\n%s", want, helpOut.String())
		}
	}
}

// initLinkedCLIProject prepares a Git project whose default configuration can
// launch the orchestrator, so the completion/evaluation flow can be exercised.
func initLinkedCLIProject(t *testing.T) (string, *core.Service) {
	t.Helper()
	project := t.TempDir()
	cliGit(t, project, "init")
	cliCommit(t, project, "initial")
	if _, err := core.InitProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	s := &core.Service{Root: project, Runtime: &cliFakeRuntime{}, Executable: os.Args[0]}
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clients = map[string]core.Client{"test": {Adapter: "command", LaunchArgv: []string{os.Args[0], "{prompt}"}}}
	cfg.Profiles = map[string]core.Profile{}
	cfg.Defaults.OrchestratorProfile = "frontier"
	cfg.Profiles["frontier"] = core.Profile{Routes: []core.Route{{ID: "frontier-a", Client: "test", Provider: "a", Model: "test-model", MaxConcurrency: 8}}}
	config, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".workspace", "config.yaml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	return project, s
}

// The CLI JSON of issue-evaluation record equals workspace status --json, and
// the same operation key replays the committed result.
func TestIssueEvaluationRecordJSONMatchesStatusAndReplays(t *testing.T) {
	project := t.TempDir()
	cliGit(t, project, "init")
	cliCommit(t, project, "initial")
	if _, err := core.InitProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Execute([]string{"--json", "--project", project, "issue", "create", "Deliver the linked feature", "--title", "Linked feature"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("issue create failed: %d %s", code, errOut.String())
	}
	var createdIssue struct {
		OK   bool       `json:"ok"`
		Data core.Issue `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &createdIssue); err != nil {
		t.Fatal(err)
	}
	if !createdIssue.OK || createdIssue.Data.ID == "" {
		t.Fatalf("issue create response: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Execute([]string{"--json", "--project", project, "create", "--from-issue", createdIssue.Data.ID, "--no-workflow", "--operation-key", "link:1"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("linked create failed: %d %s", code, errOut.String())
	}
	var created struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !created.OK || created.Data.Workspace.Input.IssueID != createdIssue.Data.ID {
		t.Fatalf("linked create response: %s", out.String())
	}
	ws := created.Data.Workspace.ID
	revision := created.Data.Workspace.Revision

	out.Reset()
	errOut.Reset()
	if code := Execute([]string{
		"--json", "--project", project, "complete", "--workspace", ws,
		"--reason", "delivered", "--expected-revision", strconv.Itoa(revision),
		"--no-issue-evaluation-start", "--operation-key", "complete:eval-cli",
	}, nil, &out, &errOut); code != 0 {
		t.Fatalf("complete failed: %d %s", code, errOut.String())
	}
	var completed struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &completed); err != nil {
		t.Fatal(err)
	}
	if eval := completed.Data.Workspace.IssueEvaluation; eval == nil || eval.State != "pending" {
		t.Fatalf("suppressed launch did not leave a pending evaluation: %s", out.String())
	}
	revision = completed.Data.Workspace.Revision

	recordArgs := []string{
		"--json", "--project", project, "issue-evaluation", "record", "--workspace", ws,
		"--outcome", "delivered", "--reason", "Every criterion has evidence",
		"--expected-revision", strconv.Itoa(revision), "--operation-key", "issue-evaluation:cli",
	}
	out.Reset()
	errOut.Reset()
	if code := Execute(recordArgs, nil, &out, &errOut); code != 0 {
		t.Fatalf("issue-evaluation record failed: %d %s", code, errOut.String())
	}
	var recorded struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	if eval := recorded.Data.Workspace.IssueEvaluation; eval == nil || eval.State != "recorded" || eval.Outcome != "delivered" {
		t.Fatalf("record response: %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Execute([]string{"--json", "--project", project, "status", "--workspace", ws}, nil, &out, &errOut); code != 0 {
		t.Fatalf("status failed: %d %s", code, errOut.String())
	}
	var status struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recorded.Data, status.Data) {
		t.Fatalf("record JSON differs from status JSON:\nrecord=%s\nstatus=%s", out.String(), out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Execute(recordArgs, nil, &out, &errOut); code != 0 {
		t.Fatalf("issue-evaluation record replay failed: %d %s", code, errOut.String())
	}
	var replayed struct {
		OK   bool        `json:"ok"`
		Data core.Status `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &replayed); err != nil {
		t.Fatal(err)
	}
	if !replayed.OK || !reflect.DeepEqual(replayed.Data, recorded.Data) {
		t.Fatalf("issue-evaluation replay did not return the committed result: %s", out.String())
	}
}

// The TUI complete_workspace action goes through CoreBackend and triggers the
// same automatic linked-Issue evaluation launch as the CLI. A replay of the
// action key does not start a second Run.
func TestTUICompleteWorkspaceLaunchesEvaluation(t *testing.T) {
	_, s := initLinkedCLIProject(t)
	ctx := context.Background()
	issue, err := s.IntakeIssue(ctx, core.IssueCreateOptions{Title: "Linked TUI", Body: "Deliver the TUI flow"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateFromIssue(ctx, issue.ID, core.CreateOptions{NoWorkflow: true})
	if err != nil {
		t.Fatal(err)
	}
	ws := created.Workspace.ID
	call := tui.ActionCall{Action: "complete_workspace", Reason: "tui completion", ExpectedRevision: created.Workspace.Revision, Key: "tui-complete-eval"}
	backend := tui.CoreBackend{Service: s}
	if err := backend.PerformAction(ctx, ws, call); err != nil {
		t.Fatalf("tui complete_workspace failed: %v", err)
	}

	countRuns := func() int {
		t.Helper()
		status, err := s.Status(ctx, ws)
		if err != nil {
			t.Fatal(err)
		}
		if status.Workspace.Status != "completed" {
			t.Fatalf("workspace status: %s", status.Workspace.Status)
		}
		if eval := status.Workspace.IssueEvaluation; eval == nil || eval.State != "pending" {
			t.Fatalf("evaluation after tui completion: %+v", eval)
		}
		count := 0
		for _, session := range status.Sessions {
			if !session.Active() || session.AgentSnapshot.Role != "orchestrator" {
				continue
			}
			for _, run := range status.Runs {
				if run.ID == session.CurrentRunID && run.ConversationOnly {
					count++
				}
			}
		}
		return count
	}
	if runs := countRuns(); runs != 1 {
		t.Fatalf("tui completion started %d conversation-only Runs, want one", runs)
	}
	if err := backend.PerformAction(ctx, ws, call); err != nil {
		t.Fatalf("tui complete_workspace replay failed: %v", err)
	}
	if runs := countRuns(); runs != 1 {
		t.Fatalf("tui completion replay started another Run: %d", runs)
	}
}
