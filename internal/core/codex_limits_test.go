package core

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func int64Ptr(v int64) *int64 { return &v }
func strPtr(v string) *string { return &v }

// The shapes below follow `codex app-server generate-json-schema` for
// codex-cli 0.141.0. No model call, credential or external account is used.

func TestCodexRateLimitObservation(t *testing.T) {
	reset := time.Now().Add(time.Hour).Unix()
	for _, test := range []struct {
		name      string
		snapshot  codexRateLimitSnapshot
		wantOK    bool
		wantKind  string
		wantUsed  *int
		wantReset bool
	}{
		{
			name:      "reached-type-usage-limit",
			snapshot:  codexRateLimitSnapshot{Primary: &codexRateLimitWindow{UsedPercent: 97, ResetsAt: int64Ptr(reset)}, RateLimitReachedType: strPtr("workspace_member_usage_limit_reached")},
			wantOK:    true,
			wantKind:  "quota_exhausted",
			wantUsed:  intPtr(97),
			wantReset: true,
		},
		{
			name:      "reached-type-rate-limit",
			snapshot:  codexRateLimitSnapshot{Primary: &codexRateLimitWindow{UsedPercent: 80, ResetsAt: int64Ptr(reset)}, RateLimitReachedType: strPtr("rate_limit_reached")},
			wantOK:    true,
			wantKind:  "rate_limited",
			wantUsed:  intPtr(80),
			wantReset: true,
		},
		{
			name:      "window-at-100-percent",
			snapshot:  codexRateLimitSnapshot{Primary: &codexRateLimitWindow{UsedPercent: 100, ResetsAt: int64Ptr(reset)}},
			wantOK:    true,
			wantKind:  "quota_exhausted",
			wantUsed:  intPtr(100),
			wantReset: true,
		},
		{
			name:      "soft-usage-pressure",
			snapshot:  codexRateLimitSnapshot{Primary: &codexRateLimitWindow{UsedPercent: 95}, Secondary: &codexRateLimitWindow{UsedPercent: 50, ResetsAt: int64Ptr(reset)}},
			wantOK:    true,
			wantKind:  "usage_pressure",
			wantUsed:  intPtr(95),
			wantReset: false,
		},
		{
			name:      "secondary-window-is-highest",
			snapshot:  codexRateLimitSnapshot{Primary: &codexRateLimitWindow{UsedPercent: 10}, Secondary: &codexRateLimitWindow{UsedPercent: 100, ResetsAt: int64Ptr(reset)}},
			wantOK:    true,
			wantKind:  "quota_exhausted",
			wantUsed:  intPtr(100),
			wantReset: true,
		},
		{
			name:     "no-window",
			snapshot: codexRateLimitSnapshot{PlanType: strPtr("pro")},
			wantOK:   false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			obs, ok := codexRateLimitObservation(test.snapshot)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v (%+v)", ok, test.wantOK, obs)
			}
			if !ok {
				return
			}
			if obs.Kind != test.wantKind || obs.Source != "codex_rate_limits" {
				t.Fatalf("observation = %+v, want kind %s", obs, test.wantKind)
			}
			if test.wantUsed == nil {
				if obs.UsedPercent != nil {
					t.Fatalf("used percent = %v, want none", *obs.UsedPercent)
				}
			} else if obs.UsedPercent == nil || *obs.UsedPercent != *test.wantUsed {
				t.Fatalf("used percent = %v, want %d", obs.UsedPercent, *test.wantUsed)
			}
			if test.wantReset != (obs.ResetAt != nil) {
				t.Fatalf("reset presence = %v, want %v: %+v", obs.ResetAt != nil, test.wantReset, obs)
			}
		})
	}
}

func TestParseCodexRateLimitSnapshot(t *testing.T) {
	seconds := int64(1800000000)
	params := []byte(`{"rateLimits":{"primary":{"usedPercent":100,"resetsAt":1800000000,"windowDurationMins":300},"rateLimitReachedType":"rate_limit_reached","planType":"pro","futureField":true}}`)
	snapshot, ok := parseCodexRateLimitSnapshot(params)
	if !ok || snapshot.Primary == nil || snapshot.Primary.UsedPercent != 100 || snapshot.Primary.ResetsAt == nil || *snapshot.Primary.ResetsAt != seconds ||
		snapshot.RateLimitReachedType == nil || *snapshot.RateLimitReachedType != "rate_limit_reached" {
		t.Fatalf("parsed snapshot = %+v ok=%v", snapshot, ok)
	}
	if _, ok := parseCodexRateLimitSnapshot([]byte(`not json`)); ok {
		t.Fatal("malformed params were accepted")
	}
}

func TestClassifyCodexTurnError(t *testing.T) {
	for _, test := range []struct {
		name      string
		raw       string
		wantKind  string
		wantUsage bool
	}{
		{name: "usage-limit", raw: `{"message":"usage limit","codexErrorInfo":"usageLimitExceeded"}`, wantKind: "quota_exhausted", wantUsage: true},
		{name: "http-429", raw: `{"message":"too many requests","codexErrorInfo":{"httpConnectionFailed":{"httpStatusCode":429}}}`, wantKind: "rate_limited"},
		{name: "http-500", raw: `{"message":"server","codexErrorInfo":{"httpConnectionFailed":{"httpStatusCode":500}}}`},
		{name: "server-overloaded", raw: `{"message":"busy","codexErrorInfo":"serverOverloaded"}`},
		{name: "context-window", raw: `{"message":"too long","codexErrorInfo":"contextWindowExceeded"}`},
		{name: "no-info", raw: `{"message":"plain"}`},
		{name: "unparseable-info", raw: `{"message":"odd","codexErrorInfo":{"unknownVariant":{}}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var turnError codexTurnError
			if err := json.Unmarshal([]byte(test.raw), &turnError); err != nil {
				t.Fatal(err)
			}
			class := classifyCodexTurnError(turnError)
			if class.Kind != test.wantKind || class.UsageLimit != test.wantUsage {
				t.Fatalf("classify(%s) = %+v, want kind %q usage %v", test.raw, class, test.wantKind, test.wantUsage)
			}
		})
	}
}

func codexLimitsFixture(t *testing.T, scenario string) (*Service, string) {
	t.Helper()
	s, workspace := fixture(t)
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Clients["test"] = Client{
		Adapter:      "codex",
		LaunchArgv:   []string{exe, "-test.run=TestCodexLimitsAppServerProcess", "--", scenario},
		ThreadParams: map[string]any{"approvalPolicy": "never"},
	}
	writeTestConfig(t, s, cfg)
	return s, workspace
}

func waitForRouteLimit(t *testing.T, s *Service, kind string, requireReset bool) RouteLimit {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		ledger, _ := loadRouteLimits(s.Root)
		for _, rec := range ledger.Limits {
			if rec.Kind == kind && (!requireReset || rec.ResetAt != nil) {
				return rec
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("no %s route limit was recorded (reset=%v)", kind, requireReset)
	return RouteLimit{}
}

func waitForClientState(t *testing.T, s *Service, ctx context.Context, workspace, sessionID, state string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		status, err := s.Status(ctx, workspace)
		if err != nil {
			t.Fatal(err)
		}
		for _, session := range status.Sessions {
			if session.ID == sessionID && session.ClientState == state {
				return
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("session %s never reached client state %q", sessionID, state)
}

func stopCodexBridge(t *testing.T, w io.Writer, done <-chan error) {
	t.Helper()
	if _, err := io.WriteString(w, "/quit\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("codex bridge did not exit")
	}
}

func TestCodexBridgeRecordsUsageLimits(t *testing.T) {
	for _, test := range []struct {
		scenario  string
		wantKind  string
		wantUsed  *int
		wantReset bool
	}{
		{scenario: "codex-limits:reached-usage", wantKind: "quota_exhausted", wantUsed: intPtr(100), wantReset: true},
		{scenario: "codex-limits:reached-rate", wantKind: "rate_limited", wantUsed: intPtr(80), wantReset: true},
		{scenario: "codex-limits:turn-usage-limit", wantKind: "quota_exhausted", wantUsed: intPtr(100), wantReset: true},
		{scenario: "codex-limits:http-429", wantKind: "rate_limited", wantReset: false},
		{scenario: "codex-limits:usage-pressure", wantKind: "usage_pressure", wantUsed: intPtr(95), wantReset: true},
	} {
		t.Run(strings.TrimPrefix(test.scenario, "codex-limits:"), func(t *testing.T) {
			s, workspace := codexLimitsFixture(t, test.scenario)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			session, err := s.StartOrchestrator(ctx, workspace, "")
			if err != nil {
				t.Fatal(err)
			}
			r, w := io.Pipe()
			defer r.Close()
			defer w.Close()
			done := make(chan error, 1)
			go func() { done <- s.ExecuteSession(ctx, workspace, session.ID, r, io.Discard, io.Discard) }()

			rec := waitForRouteLimit(t, s, test.wantKind, test.wantReset)
			if rec.Scope != "client" || rec.Client != "test" || rec.Source == "" || rec.RunID == "" || rec.WorkspaceID != workspace {
				t.Fatalf("record not bound to the Run's client route: %+v", rec)
			}
			if test.wantUsed == nil {
				if rec.UsedPercent != nil {
					t.Fatalf("used percent = %v, want none", *rec.UsedPercent)
				}
			} else if rec.UsedPercent == nil || *rec.UsedPercent != *test.wantUsed {
				t.Fatalf("used percent = %v, want %d", rec.UsedPercent, *test.wantUsed)
			}
			if test.wantReset && rec.ResetAt == nil {
				t.Fatalf("expected a recorded reset: %+v", rec)
			}
			if !test.wantReset && rec.ResetAt != nil {
				t.Fatalf("unexpected reset: %+v", rec)
			}
			stopCodexBridge(t, w, done)
		})
	}
}

func TestCodexBridgeIgnoresServerOverloaded(t *testing.T) {
	s, workspace := codexLimitsFixture(t, "codex-limits:overloaded")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, err := s.StartOrchestrator(ctx, workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	done := make(chan error, 1)
	go func() { done <- s.ExecuteSession(ctx, workspace, session.ID, r, io.Discard, io.Discard) }()

	waitForClientState(t, s, ctx, workspace, session.ID, "idle")
	ledger, err := loadRouteLimits(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.Limits) != 0 {
		t.Fatalf("serverOverloaded recorded a usage limit: %+v", ledger.Limits)
	}
	stopCodexBridge(t, w, done)
}

// TestCodexLimitsAppServerProcess is a protocol fixture. It is a no-op unless
// the last argument is a codex-limits scenario, so it stays inert in a normal
// test run. No model calls, credentials or external accounts are used.
func TestCodexLimitsAppServerProcess(t *testing.T) {
	if len(os.Args) == 0 {
		return
	}
	scenario := os.Args[len(os.Args)-1]
	if !strings.HasPrefix(scenario, "codex-limits:") {
		return
	}
	encoder := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	for scanner.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		switch req.Method {
		case "initialize":
			_ = encoder.Encode(map[string]any{"id": req.ID, "result": map[string]any{}})
		case "initialized":
		case "thread/start", "thread/resume":
			_ = encoder.Encode(map[string]any{"id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "test-limits-thread"}}})
		case "turn/start":
			_ = encoder.Encode(map[string]any{"id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-1"}}})
			codexEmitLimitFixture(encoder, scenario)
		case "account/rateLimits/read":
			_ = encoder.Encode(map[string]any{"id": req.ID, "result": map[string]any{"rateLimits": codexFixtureSnapshot()}})
		default:
			if strings.TrimSpace(req.Method) != "" {
				_ = encoder.Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -32601, "message": "unsupported"}})
			}
		}
	}
	os.Exit(0)
}

func codexFixtureSnapshot() map[string]any {
	return map[string]any{
		"primary":              map[string]any{"usedPercent": 100, "resetsAt": time.Now().Add(time.Hour).Unix(), "windowDurationMins": 300},
		"rateLimitReachedType": "workspace_member_usage_limit_reached",
		"planType":             "pro",
		"limitId":              "codex",
	}
}

func codexTurnCompleted(status string, turnError any) map[string]any {
	turn := map[string]any{"status": status}
	if turnError != nil {
		turn["error"] = turnError
	}
	return map[string]any{"method": "turn/completed", "params": map[string]any{"turn": turn}}
}

func codexEmitLimitFixture(encoder *json.Encoder, scenario string) {
	reset := time.Now().Add(time.Hour).Unix()
	switch scenario {
	case "codex-limits:reached-usage":
		_ = encoder.Encode(map[string]any{"method": "account/rateLimits/updated", "params": map[string]any{"rateLimits": map[string]any{
			"primary":              map[string]any{"usedPercent": 100, "resetsAt": reset, "windowDurationMins": 300},
			"rateLimitReachedType": "workspace_member_usage_limit_reached",
			"planType":             "pro",
			"limitId":              "codex",
		}}})
		_ = encoder.Encode(codexTurnCompleted("completed", nil))
	case "codex-limits:reached-rate":
		_ = encoder.Encode(map[string]any{"method": "account/rateLimits/updated", "params": map[string]any{"rateLimits": map[string]any{
			"primary":              map[string]any{"usedPercent": 80, "resetsAt": reset, "windowDurationMins": 300},
			"rateLimitReachedType": "rate_limit_reached",
		}}})
		_ = encoder.Encode(codexTurnCompleted("completed", nil))
	case "codex-limits:usage-pressure":
		_ = encoder.Encode(map[string]any{"method": "account/rateLimits/updated", "params": map[string]any{"rateLimits": map[string]any{
			"primary": map[string]any{"usedPercent": 95, "resetsAt": reset, "windowDurationMins": 300},
		}}})
		_ = encoder.Encode(codexTurnCompleted("completed", nil))
	case "codex-limits:turn-usage-limit":
		_ = encoder.Encode(codexTurnCompleted("failed", map[string]any{"message": "You have hit your usage limit", "codexErrorInfo": "usageLimitExceeded"}))
	case "codex-limits:overloaded":
		_ = encoder.Encode(codexTurnCompleted("failed", map[string]any{"message": "Server overloaded", "codexErrorInfo": "serverOverloaded"}))
	case "codex-limits:http-429":
		_ = encoder.Encode(codexTurnCompleted("failed", map[string]any{"message": "rate limited", "codexErrorInfo": map[string]any{"httpConnectionFailed": map[string]any{"httpStatusCode": 429}}}))
	}
}
