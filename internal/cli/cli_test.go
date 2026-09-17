package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"workspace/internal/core"
)

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
	if !response.OK || len(response.Data) != 1 || response.Data[0].ID != "plan-first" {
		t.Fatal("invalid workflow list")
	}
	out.Reset()
	errOut.Reset()
	code = Execute([]string{"--json", "--project", t.TempDir(), "status"}, nil, &out, &errOut)
	if code != 1 || !bytes.Contains(out.Bytes(), []byte(`"code":"project_not_found"`)) {
		t.Fatalf("expected structured error: %d %s", code, out.String())
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
		{Workspace: core.Workspace{ID: "ws_flow", Title: "Flow", Status: "active", Workflow: &core.Workflow{ID: "issue-resolution", Phase: "planning"}}},
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
	value := shortOutput([]core.Session{{ID: "sess_1", AgentID: "agent_1", TaskID: "task_1", LifecycleState: "idle", LastRunID: "run_3", RunCount: 3, RunState: "interrupted", ClientThreadID: "thread_1"}})
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"id":"sess_1"`, `"lifecycle_state":"idle"`, `"last_run_id":"run_3"`, `"run_count":3`, `"run_state":"interrupted"`, `"client_thread_id":"thread_1"`} {
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
