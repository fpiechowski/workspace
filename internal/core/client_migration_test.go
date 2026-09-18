package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func historicalOpenCodeClient() Client {
	return Client{
		Adapter:     "opencode",
		LaunchArgv:  []string{"opencode", "--model", "{model}", "--prompt", "{prompt}"},
		ResumeArgv:  []string{"opencode", "--session", "{thread_id}", "--model", "{model}", "--prompt", "{prompt}"},
		DeliverArgv: append([]string(nil), legacyOpenCodeDeliverArgv...),
	}
}

func TestOpenCodeClientNormalizationUsesNativeDefaultAndPreservesCustomWrapper(t *testing.T) {
	native, err := normalizeClient(Client{
		Adapter:    "opencode",
		LaunchArgv: []string{"custom-opencode", "--prompt", "{prompt}"},
		ResumeArgv: []string{"custom-opencode", "--session", "{thread_id}", "--prompt", "{prompt}"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !native.NativeDelivery || len(native.DeliverArgv) != 0 || !containsCapability(native.Capabilities, "observe") {
		t.Fatalf("OpenCode without wrapper was not normalized to native delivery: %+v", native)
	}

	custom, err := normalizeClient(Client{
		Adapter:        "opencode",
		LaunchArgv:     []string{"custom-opencode", "--prompt", "{prompt}"},
		DeliverArgv:    []string{"deliver-wrapper", "{message_id}"},
		NativeDelivery: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if custom.NativeDelivery || !reflect.DeepEqual(custom.DeliverArgv, []string{"deliver-wrapper", "{message_id}"}) || !containsCapability(custom.Capabilities, "deliver") {
		t.Fatalf("custom OpenCode wrapper was not kept external: %+v", custom)
	}

	legacy, err := normalizeClient(historicalOpenCodeClient())
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.NativeDelivery || len(legacy.DeliverArgv) != 0 {
		t.Fatalf("historical wrapper was not upgraded: %+v", legacy)
	}

	unchanged := Client{Adapter: "command", DeliverArgv: append([]string(nil), legacyOpenCodeDeliverArgv...)}
	if got, upgraded := upgradeLegacyOpenCodeClient(unchanged); upgraded || !reflect.DeepEqual(got, unchanged) {
		t.Fatalf("non-OpenCode client was changed: got=%+v upgraded=%v", got, upgraded)
	}
	similar := historicalOpenCodeClient()
	similar.DeliverArgv[1] = "{project_dir}/scripts/other-deliver.py"
	if got, upgraded := upgradeLegacyOpenCodeClient(similar); upgraded || !reflect.DeepEqual(got, similar) {
		t.Fatalf("similar wrapper was changed: got=%+v upgraded=%v", got, upgraded)
	}
}

func containsCapability(capabilities []string, wanted string) bool {
	for _, capability := range capabilities {
		if capability == wanted {
			return true
		}
	}
	return false
}

func TestRegistryV3OpenCodeMigrationIsStagedAndIdempotent(t *testing.T) {
	run := Run{ID: "run", SessionID: "session", State: "exited", ClientThreadID: "thread"}
	d := &Document{Registry: Registry{
		SchemaVersion: 3,
		Sessions: []Session{
			{ID: "session", ClientSnapshot: historicalOpenCodeClient()},
			{ID: "custom", ClientSnapshot: Client{Adapter: "opencode", DeliverArgv: []string{"custom-wrapper"}}},
			{ID: "command", ClientSnapshot: Client{Adapter: "command", DeliverArgv: []string{"custom-wrapper"}}},
			{ID: "native", ClientSnapshot: Client{Adapter: "opencode", NativeDelivery: true}},
		},
		Runs: []Run{run},
	}}
	beforeRuns := append([]Run(nil), d.Registry.Runs...)
	if !migrateRegistry(d) {
		t.Fatal("schema-v3 registry was not migrated")
	}
	if d.Registry.SchemaVersion != 5 || len(d.Registry.Sessions[0].ClientSnapshot.DeliverArgv) != 0 || !d.Registry.Sessions[0].ClientSnapshot.NativeDelivery {
		t.Fatalf("legacy snapshot was not upgraded: %+v", d.Registry.Sessions[0].ClientSnapshot)
	}
	if !reflect.DeepEqual(beforeRuns, d.Registry.Runs) {
		t.Fatalf("migration changed historical Runs: before=%+v after=%+v", beforeRuns, d.Registry.Runs)
	}
	if !reflect.DeepEqual(d.Registry.Sessions[1].ClientSnapshot.DeliverArgv, []string{"custom-wrapper"}) || d.Registry.Sessions[1].ClientSnapshot.NativeDelivery {
		t.Fatalf("custom OpenCode wrapper changed: %+v", d.Registry.Sessions[1].ClientSnapshot)
	}
	if migrateRegistry(d) {
		t.Fatal("schema-v5 migration was not idempotent")
	}

	older := &Document{State: Workspace{Tasks: []Task{{ID: "task", SessionID: "legacy-run"}}}, Registry: Registry{
		Sessions: []Session{{ID: "legacy-run", AgentID: "agent", ClientSnapshot: Client{Adapter: "command"}, State: "exited"}},
	}}
	if !migrateRegistry(older) || older.Registry.SchemaVersion != 5 || len(older.Registry.Runs) != 1 || older.Registry.Runs[0].ID != "legacy-run" || older.State.Tasks[0].RunID != "legacy-run" {
		t.Fatalf("older registry did not pass through staged migrations: %+v", older)
	}
}

func configureLegacyOpenCodeProject(t *testing.T, s *Service) {
	t.Helper()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clients["test"] = Client{
		Adapter:     "opencode",
		LaunchArgv:  []string{exe, "-test.run=TestOpenCodeProcess", "--", "{prompt_file}"},
		ResumeArgv:  []string{exe, "-test.run=TestOpenCodeProcess", "--session", "{thread_id}", "--", "{prompt_file}"},
		DeliverArgv: append([]string(nil), legacyOpenCodeDeliverArgv...),
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPersistsLegacyMigrationOnceAndRestartIsRequired(t *testing.T) {
	s, workspace := fixture(t)
	configureLegacyOpenCodeProject(t, s)
	agent, worktree := worker(t, s, workspace, "legacy-opencode")
	started, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	var registry Registry
	index := filepath.Join(status.Directory, ".runtime", "index.json")
	if err := readJSON(index, &registry); err != nil {
		t.Fatal(err)
	}
	registry.SchemaVersion = 3
	for i := range registry.Sessions {
		if registry.Sessions[i].ID == started.ID {
			legacy := registry.Sessions[i].ClientSnapshot
			legacy.DeliverArgv = append([]string(nil), legacyOpenCodeDeliverArgv...)
			legacy.NativeDelivery = false
			registry.Sessions[i].ClientSnapshot = legacy
		}
	}
	for i := range registry.Runs {
		if registry.Runs[i].ID == started.CurrentRunID {
			registry.Runs[i].State = "running"
			registry.Runs[i].OpenCodeEndpoint = ""
			registry.Runs[i].ClientThreadID = ""
		}
	}
	if err := writeJSON(index, registry); err != nil {
		t.Fatal(err)
	}

	migrated, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.Workspace.Revision != status.Workspace.Revision+1 || migrated.SchemaVersion != 2 {
		t.Fatalf("migration was not persisted exactly once: before=%d after=%d", status.Workspace.Revision, migrated.Workspace.Revision)
	}
	if !migrated.Sessions[0].ClientSnapshot.NativeDelivery || len(migrated.Sessions[0].ClientSnapshot.DeliverArgv) != 0 {
		t.Fatalf("migrated snapshot is not native: %+v", migrated.Sessions[0].ClientSnapshot)
	}
	second, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if second.Workspace.Revision != migrated.Workspace.Revision {
		t.Fatalf("second load incremented revision: first=%d second=%d", migrated.Workspace.Revision, second.Workspace.Revision)
	}

	s.openCodeSessionLister = func(context.Context, Session) ([]openCodeSession, error) { return nil, nil }
	if err := s.With(context.Background(), workspace, func(d *Document) error {
		d.Registry.Messages = append(d.Registry.Messages, Message{ID: "pending", ToAgent: agent.ID, Kind: "note", Body: "pending"})
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.With(context.Background(), workspace, func(d *Document) error {
		if len(d.Registry.Deliveries) != 1 || d.Registry.Deliveries[0].Phase != "restart_required" {
			t.Fatalf("missing restart-required delivery phase: %+v", d.Registry.Deliveries)
		}
		message, err := findMessage(d, "pending")
		if err != nil {
			return err
		}
		if message.DeliveredAt != nil || message.AcknowledgedAt != nil {
			t.Fatal("restart-required message was marked delivered or acknowledged")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.StopSession(context.Background(), workspace, started.ID); err != nil {
		t.Fatal(err)
	}
	resumed, err := s.ResumeAgent(context.Background(), workspace, agent.ID, "resume-legacy-opencode")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.CurrentRunID == started.CurrentRunID || resumed.OpenCodeEndpoint == "" || !resumed.ClientSnapshot.NativeDelivery {
		t.Fatalf("resume did not create native Run: %+v", resumed)
	}
	current, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	var currentRun Run
	for _, candidate := range current.Runs {
		if candidate.ID == resumed.CurrentRunID {
			currentRun = candidate
		}
	}
	if currentRun.OpenCodeEndpoint == "" || !hasArg(currentRun.Argv, "--hostname") || !hasArg(currentRun.Argv, "--port") {
		t.Fatalf("resumed Run lacks native server flags: %+v", currentRun)
	}
	message, err := s.Inbox(context.Background(), workspace, agent.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(message) != 1 || message[0].DeliveredRunID != "" {
		t.Fatalf("pending message was not eligible for successor Run: %+v", message)
	}
}

func hasArg(argv []string, wanted string) bool {
	for _, arg := range argv {
		if arg == wanted {
			return true
		}
	}
	return false
}
