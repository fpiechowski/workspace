package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func intPtr(v int) *int { return &v }

func emptyRouteLimits() routeLimitLedger {
	return routeLimitLedger{SchemaVersion: routeLimitSchemaVersion, Limits: map[string]RouteLimit{}, Operations: map[string]routeLimitReceipt{}}
}

func defaultUsageSettings() UsageLimitSettings {
	s, _ := effectiveUsageLimits(Profile{}, Route{})
	return s
}

// twoRouteFixture configures frontier with two routes on different providers
// of the same client.
func twoRouteFixture(t *testing.T, edit func(*Profile)) (*Service, string) {
	t.Helper()
	s, id := fixture(t)
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles["frontier"]
	p.Routes = []Route{
		{ID: "frontier-a", Client: "test", Provider: "a", Model: "test-model", MaxConcurrency: 8},
		{ID: "frontier-b", Client: "test", Provider: "b", Model: "test-model", MaxConcurrency: 8},
	}
	if edit != nil {
		edit(&p)
	}
	cfg.Profiles["frontier"] = p
	writeTestConfig(t, s, cfg)
	return s, id
}

func writeTestConfig(t *testing.T, s *Service, cfg Config) {
	t.Helper()
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(s.Root, ".workspace", "config.yaml"), b); err != nil {
		t.Fatal(err)
	}
}

func writeTestLedger(t *testing.T, s *Service, observations map[string]RouteLimitObservation) {
	t.Helper()
	cfg, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	l := emptyRouteLimits()
	for target, obs := range observations {
		p, r, err := resolveRouteTarget(cfg, target)
		if err != nil {
			t.Fatal(err)
		}
		settings, _ := effectiveUsageLimits(p, r)
		if obs.ObservedAt.IsZero() {
			obs.ObservedAt = nowUTC()
		}
		if _, err := applyRouteLimit(&l, r, "command", settings, obs); err != nil {
			t.Fatal(err)
		}
	}
	if err := saveRouteLimits(s.Root, cfg.ProjectID, l, nowUTC()); err != nil {
		t.Fatal(err)
	}
}

func candidate(t *testing.T, d RoutingDecision, id string) RouteAssessment {
	t.Helper()
	for _, c := range d.Candidates {
		if c.Route.ID == id {
			return c
		}
	}
	t.Fatalf("no candidate %s in %+v", id, d)
	return RouteAssessment{}
}

func TestRouteLimitMatchingByScope(t *testing.T) {
	a := Route{Client: "codex", Provider: "openai", Model: "gpt-a"}
	sameProvider := Route{Client: "codex", Provider: "openai", Model: "gpt-b"}
	otherProvider := Route{Client: "codex", Provider: "azure", Model: "gpt-a"}
	otherClient := Route{Client: "claude", Provider: "openai", Model: "gpt-a"}
	now := nowUTC()
	for _, tc := range []struct {
		scope string
		want  []bool
	}{
		{"client", []bool{true, true, true, false}},
		{"provider", []bool{true, true, false, false}},
		{"route", []bool{true, false, false, false}},
	} {
		l := emptyRouteLimits()
		rec, err := applyRouteLimit(&l, a, "codex", defaultUsageSettings(), RouteLimitObservation{Scope: tc.scope, Kind: "rate_limited", Source: "manual", ObservedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		for i, r := range []Route{a, sameProvider, otherProvider, otherClient} {
			if got := rec.matches(r); got != tc.want[i] {
				t.Errorf("scope %s route %d: got %v want %v", tc.scope, i, got, tc.want[i])
			}
		}
	}
	// Default scopes follow account semantics.
	for adapter, want := range map[string]string{"codex": "client", "claude": "client", "opencode": "provider", "command": "provider"} {
		l := emptyRouteLimits()
		rec, err := applyRouteLimit(&l, a, adapter, defaultUsageSettings(), RouteLimitObservation{Kind: "quota_exhausted", ObservedAt: now})
		if err != nil || rec.Scope != want {
			t.Errorf("adapter %s: scope %q err %v, want %s", adapter, rec.Scope, err, want)
		}
	}
	l := emptyRouteLimits()
	if _, err := applyRouteLimit(&l, a, "codex", defaultUsageSettings(), RouteLimitObservation{Kind: "tired", ObservedAt: now}); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if _, err := applyRouteLimit(&l, a, "codex", defaultUsageSettings(), RouteLimitObservation{Kind: "rate_limited", Scope: "planet", ObservedAt: now}); err == nil {
		t.Fatal("unknown scope accepted")
	}
}

func TestRouteLimitBackoffResetClampAndOrdering(t *testing.T) {
	r := Route{Client: "test", Provider: "p", Model: "m"}
	settings := UsageLimitSettings{Mode: "avoid", DefaultBackoffSeconds: 100, MaxBackoffSeconds: 300, SoftLimitPercent: 90}
	l := emptyRouteLimits()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	obs := func(at time.Time) RouteLimitObservation {
		return RouteLimitObservation{Kind: "rate_limited", Source: "report", ObservedAt: at}
	}
	var got []int
	at := now
	for range 4 {
		rec, err := applyRouteLimit(&l, r, "command", settings, obs(at))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, rec.BackoffSeconds)
		if !rec.Until.Equal(at.Add(time.Duration(rec.BackoffSeconds) * time.Second)) {
			t.Fatalf("until does not follow backoff: %+v", rec)
		}
		at = at.Add(10 * time.Second)
	}
	if want := []int{100, 200, 300, 300}; !equalInts(got, want) {
		t.Fatalf("backoff doubling/cap: got %v want %v", got, want)
	}
	// After the window passes the backoff restarts.
	rec, _ := applyRouteLimit(&l, r, "command", settings, obs(at.Add(time.Hour)))
	if rec.BackoffSeconds != 100 {
		t.Fatalf("backoff did not restart: %+v", rec)
	}
	// An out-of-order older observation never replaces the newer record.
	older, _ := applyRouteLimit(&l, r, "command", settings, RouteLimitObservation{Kind: "quota_exhausted", ObservedAt: now})
	if older.Kind != "rate_limited" || !older.ObservedAt.Equal(rec.ObservedAt) {
		t.Fatalf("older observation replaced newer: %+v", older)
	}
	// A later observation wins even with an earlier Until.
	later := at.Add(2 * time.Hour)
	reset := later.Add(time.Minute)
	rec, _ = applyRouteLimit(&l, r, "command", settings, RouteLimitObservation{Kind: "quota_exhausted", ResetAt: &reset, ObservedAt: later})
	if rec.Kind != "quota_exhausted" || !rec.Until.Equal(reset) || len(l.Limits) != 1 {
		t.Fatalf("later observation lost: %+v", l.Limits)
	}
	// Provider reset times are clamped to seven days.
	far := later.Add(30 * 24 * time.Hour)
	rec, _ = applyRouteLimit(&l, r, "command", settings, RouteLimitObservation{Kind: "quota_exhausted", ResetAt: &far, ObservedAt: later.Add(time.Second)})
	if want := later.Add(time.Second).Add(7 * 24 * time.Hour); !rec.Until.Equal(want) {
		t.Fatalf("reset not clamped: %v want %v", rec.Until, want)
	}
	long := strings.Repeat("é", 400)
	rec, _ = applyRouteLimit(&l, r, "command", settings, RouteLimitObservation{Kind: "rate_limited", Message: long, ObservedAt: later.Add(2 * time.Second)})
	if len(rec.Message) > routeLimitMessageBytes || !strings.HasPrefix(long, rec.Message) {
		t.Fatalf("message not truncated on a rune boundary: %d", len(rec.Message))
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRouteLimitLedgerPersistencePruningAndCorruption(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".workspace"), 0700); err != nil {
		t.Fatal(err)
	}
	l, err := loadRouteLimits(root)
	if err != nil || len(l.Limits) != 0 {
		t.Fatalf("missing ledger is not empty: %+v %v", l, err)
	}
	now := nowUTC()
	r := Route{Client: "test", Provider: "p", Model: "m"}
	if _, err := applyRouteLimit(&l, r, "command", defaultUsageSettings(), RouteLimitObservation{Kind: "rate_limited", Scope: "route", ObservedAt: now.Add(-9 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := applyRouteLimit(&l, r, "command", defaultUsageSettings(), RouteLimitObservation{Kind: "rate_limited", Scope: "provider", ObservedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := saveRouteLimits(root, "prj", l, now); err != nil {
		t.Fatal(err)
	}
	l, err = loadRouteLimits(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Limits) != 1 || l.Limits["provider:test/p"].Key != "provider:test/p" {
		t.Fatalf("expired record not pruned: %+v", l.Limits)
	}
	if err := os.WriteFile(routeLimitsPath(root), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	l, err = loadRouteLimits(root)
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != "route_limits_invalid" || len(l.Limits) != 0 {
		t.Fatalf("corrupt ledger: %+v %v", l, err)
	}
	if _, err := loadRouteLimitsForWrite(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(routeLimitsPath(root)); !os.IsNotExist(err) {
		t.Fatal("corrupt ledger was not moved aside before a write")
	}
}

func TestUsageLimitsConfigValidationAndMerge(t *testing.T) {
	s, _ := fixture(t)
	base, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]*UsageLimits{
		"mode":     {Mode: "panic"},
		"negative": {DefaultBackoffSeconds: -1},
		"percent":  {SoftLimitPercent: intPtr(101)},
		"order":    {DefaultBackoffSeconds: 600, MaxBackoffSeconds: 60},
	} {
		for _, onRoute := range []bool{false, true} {
			cfg, _ := s.Config()
			p := cfg.Profiles["frontier"]
			if onRoute {
				routes := append([]Route(nil), p.Routes...)
				routes[0].UsageLimits = u
				p.Routes = routes
			} else {
				p.UsageLimits = u
			}
			cfg.Profiles["frontier"] = p
			if _, err := ValidateConfig(cfg); err == nil {
				t.Errorf("%s (route=%v) accepted", name, onRoute)
			} else {
				expectCode(t, err, "invalid_config")
			}
		}
	}
	p := base.Profiles["frontier"]
	p.UsageLimits = &UsageLimits{Mode: "observe", DefaultBackoffSeconds: 60, SoftLimitPercent: intPtr(0)}
	r := p.Routes[0]
	r.UsageLimits = &UsageLimits{Mode: "ignore", MaxBackoffSeconds: 120}
	got, err := effectiveUsageLimits(p, r)
	if err != nil || got != (UsageLimitSettings{Mode: "ignore", DefaultBackoffSeconds: 60, MaxBackoffSeconds: 120, SoftLimitPercent: 0}) {
		t.Fatalf("merge: %+v %v", got, err)
	}
	if got := defaultUsageSettings(); got != (UsageLimitSettings{Mode: "avoid", DefaultBackoffSeconds: 900, MaxBackoffSeconds: 21600, SoftLimitPercent: 90}) {
		t.Fatalf("defaults: %+v", got)
	}
	// Old configuration without usage_limits keeps loading unchanged.
	if _, err := ValidateConfig(base); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingAvoidsUsageLimitedRouteUntilClearOrReset(t *testing.T) {
	s, _ := twoRouteFixture(t, nil)
	ctx := context.Background()
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "quota_exhausted", For: time.Hour, Scope: "route"}); err != nil {
		t.Fatal(err)
	}
	d, err := s.ExplainProfile(ctx, "frontier")
	if err != nil {
		t.Fatal(err)
	}
	a := candidate(t, d, "frontier-a")
	if a.Eligible || !strings.HasPrefix(a.Reason, "usage limit: quota_exhausted until ") || a.LimitedUntil == nil || a.Limit == nil || d.Selected != "frontier-b" {
		t.Fatalf("limited route not avoided: %+v", d)
	}
	cleared, err := s.ClearRouteLimit(ctx, "", RouteLimitClearOptions{Target: "frontier/frontier-a"})
	if err != nil || len(cleared) != 1 || cleared[0].ClearedAt == nil {
		t.Fatalf("clear: %+v %v", cleared, err)
	}
	d, _ = s.ExplainProfile(ctx, "frontier")
	if a := candidate(t, d, "frontier-a"); !a.Eligible || a.LimitedUntil != nil {
		t.Fatalf("cleared route still limited: %+v", a)
	}
	list, err := s.ListRouteLimits(ctx, false)
	if err != nil || len(list.Limits) != 0 {
		t.Fatalf("cleared record still listed as active: %+v", list)
	}
	list, _ = s.ListRouteLimits(ctx, true)
	if len(list.Limits) != 1 {
		t.Fatalf("--all omitted retained record: %+v", list)
	}
	// A record whose Until has passed no longer affects selection.
	writeTestLedger(t, s, map[string]RouteLimitObservation{"frontier/frontier-a": {Kind: "rate_limited", Scope: "route", RetryAfter: time.Minute, ObservedAt: nowUTC().Add(-time.Hour)}})
	d, _ = s.ExplainProfile(ctx, "frontier")
	if a := candidate(t, d, "frontier-a"); !a.Eligible {
		t.Fatalf("expired limit still applied: %+v", a)
	}
}

func TestRoutingSoftUsagePressureTier(t *testing.T) {
	s, _ := twoRouteFixture(t, nil)
	ctx := context.Background()
	// Without pressure the equal-load tie keeps the first route.
	d, _ := s.ExplainProfile(ctx, "frontier")
	if d.Selected != "frontier-a" {
		t.Fatalf("baseline selection: %+v", d)
	}
	writeTestLedger(t, s, map[string]RouteLimitObservation{"frontier/frontier-a": {Kind: "usage_pressure", Scope: "route", UsedPercent: intPtr(95), RetryAfter: time.Hour}})
	d, _ = s.ExplainProfile(ctx, "frontier")
	a := candidate(t, d, "frontier-a")
	if !a.Eligible || !a.SoftLimited || a.UsedPercent == nil || *a.UsedPercent != 95 || d.Selected != "frontier-b" {
		t.Fatalf("pressured route not ranked last: %+v", d)
	}
	// Below the threshold the route keeps its normal rank.
	writeTestLedger(t, s, map[string]RouteLimitObservation{"frontier/frontier-a": {Kind: "usage_pressure", Scope: "route", UsedPercent: intPtr(50), RetryAfter: time.Hour}})
	if d, _ = s.ExplainProfile(ctx, "frontier"); d.Selected != "frontier-a" || candidate(t, d, "frontier-a").SoftLimited {
		t.Fatalf("pressure below threshold changed ranking: %+v", d)
	}
	// Still selectable when it is the only eligible route.
	writeTestLedger(t, s, map[string]RouteLimitObservation{
		"frontier/frontier-a": {Kind: "usage_pressure", Scope: "route", UsedPercent: intPtr(99), RetryAfter: time.Hour},
		"frontier/frontier-b": {Kind: "quota_exhausted", Scope: "route", RetryAfter: time.Hour},
	})
	if d, _ = s.ExplainProfile(ctx, "frontier"); d.Selected != "frontier-a" {
		t.Fatalf("only pressured route not selected: %+v", d)
	}
}

func TestRoutingAllLimitedFailsWithRouteLimited(t *testing.T) {
	s, id := twoRouteFixture(t, nil)
	ctx := context.Background()
	soon := nowUTC().Add(20 * time.Minute).Truncate(time.Second)
	later := nowUTC().Add(2 * time.Hour).Truncate(time.Second)
	writeTestLedger(t, s, map[string]RouteLimitObservation{
		"frontier/frontier-a": {Kind: "quota_exhausted", Scope: "route", ResetAt: &later},
		"frontier/frontier-b": {Kind: "rate_limited", Scope: "route", ResetAt: &soon},
	})
	_, err := s.StartOrchestrator(ctx, id, "")
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != "route_limited" || len(ce.Options) != 1 || ce.Options[0] != "retry_after="+soon.Format(time.RFC3339) {
		t.Fatalf("expected route_limited with earliest reset, got %v %+v", err, ce)
	}
	cfg, _ := s.Config()
	if _, _, err := s.chooseRoute(cfg, "frontier"); err == nil {
		t.Fatal("chooseRoute selected a limited route")
	}
	// Configuration/capability failures remain no_route.
	p := cfg.Profiles["frontier"]
	p.RequiredCapabilities = []string{"deliver"}
	cfg.Profiles["frontier"] = p
	_, _, err = s.chooseRoute(cfg, "frontier")
	expectCode(t, err, "no_route")
}

func TestRoutingUsageLimitModes(t *testing.T) {
	s, _ := twoRouteFixture(t, func(p *Profile) {
		p.UsageLimits = &UsageLimits{Mode: "observe"}
		p.Routes[1].UsageLimits = &UsageLimits{Mode: "ignore"}
	})
	ctx := context.Background()
	writeTestLedger(t, s, map[string]RouteLimitObservation{
		"frontier/frontier-a": {Kind: "quota_exhausted", Scope: "route", RetryAfter: time.Hour},
		"frontier/frontier-b": {Kind: "quota_exhausted", Scope: "route", RetryAfter: time.Hour},
	})
	d, err := s.ExplainProfile(ctx, "frontier")
	if err != nil {
		t.Fatal(err)
	}
	a, b := candidate(t, d, "frontier-a"), candidate(t, d, "frontier-b")
	if !a.Eligible || a.LimitedUntil == nil || a.UsageLimitMode != "observe" || d.Selected != "frontier-a" {
		t.Fatalf("observe changed eligibility or hid the record: %+v", a)
	}
	if !b.Eligible || b.LimitedUntil != nil || b.Limit != nil || b.UsageLimitMode != "ignore" {
		t.Fatalf("ignore still matched records: %+v", b)
	}
}

func TestRoutingCorruptLedgerNeverBlocksLaunch(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	if err := os.WriteFile(routeLimitsPath(s.Root), []byte("garbage"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := s.ExplainProfile(ctx, "frontier")
	if err != nil {
		t.Fatal(err)
	}
	if d.Selected == "" || len(d.Warnings) != 1 || !strings.HasPrefix(d.Warnings[0], "route_limits_invalid") {
		t.Fatalf("corrupt ledger not surfaced or blocked selection: %+v", d)
	}
	if _, err := s.StartOrchestrator(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingDecisionCompatibility(t *testing.T) {
	old := `{"profile":"p","selected":"r","candidates":[{"route":{"id":"r","client":"c","provider":"x","model":"m","max_concurrency":1},"eligible":true,"active":0,"launches":0,"provider_score":0}],"measured_at":"2026-01-01T00:00:00Z","metric":"m"}`
	var d RoutingDecision
	if err := json.Unmarshal([]byte(old), &d); err != nil || d.Candidates[0].Route.ID != "r" {
		t.Fatalf("old decision: %+v %v", d, err)
	}
	b, _ := json.Marshal(d)
	for _, field := range []string{"limited_until", "limit", "soft_limited", "used_percent", "warnings", "usage_limits", "usage_limit_mode"} {
		if strings.Contains(string(b), `"`+field+`"`) {
			t.Fatalf("empty %s serialized: %s", field, b)
		}
	}
}

func TestRouteLimitSelectionIsPersistedDecision(t *testing.T) {
	s, id := twoRouteFixture(t, nil)
	ctx := context.Background()
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "rate_limited", For: time.Hour}); err != nil {
		t.Fatal(err)
	}
	orch, err := s.StartOrchestrator(ctx, id, "")
	if err != nil {
		t.Fatal(err)
	}
	if orch.Route.ID != "frontier-b" || orch.RoutingDecision == nil || orch.RoutingDecision.Selected != "frontier-b" || candidate(t, *orch.RoutingDecision, "frontier-a").LimitedUntil == nil {
		t.Fatalf("persisted decision does not match selection: %+v", orch)
	}
}

func TestRouteLimitReportAndManualAuthorization(t *testing.T) {
	s, id := twoRouteFixture(t, nil)
	ctx := context.Background()
	if _, err := s.ReportRouteLimit(ctx, id, RouteLimitReportOptions{Kind: "rate_limited"}); err == nil {
		t.Fatal("report outside a Run accepted")
	} else {
		expectCode(t, err, "run_required")
	}
	a, w := worker(t, s, id, "limited")
	session, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	agent := *s
	agent.Actor = Actor{AgentID: session.AgentID, SessionID: session.ID, RunID: session.CurrentRunID}
	rec, err := agent.ReportRouteLimit(ctx, id, RouteLimitReportOptions{Kind: "quota_exhausted", RetryAfter: 30 * time.Minute, Message: "limit reached"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Scope != "provider" || rec.Client != session.Route.Client || rec.Provider != session.Route.Provider || rec.RunID != session.CurrentRunID || rec.WorkspaceID != id || rec.Source != "report" {
		t.Fatalf("report not bound to the calling Run's route: %+v (route %+v)", rec, session.Route)
	}
	// Workers cannot manage limits; the user can.
	_, err = agent.SetRouteLimit(ctx, id, RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "rate_limited", For: time.Minute})
	expectCode(t, err, "forbidden")
	_, err = agent.ClearRouteLimit(ctx, id, RouteLimitClearOptions{Target: "frontier/frontier-a"})
	expectCode(t, err, "forbidden")
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "rate_limited"}); err == nil {
		t.Fatal("set without --until/--for accepted")
	}
	if _, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/missing", Kind: "rate_limited", For: time.Minute}); err == nil {
		t.Fatal("unknown route accepted")
	}
	// Operation keys replay the same result and reject a changed payload.
	first, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "rate_limited", For: time.Minute, OperationKey: "limit-1"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "rate_limited", For: time.Minute, OperationKey: "limit-1"})
	if err != nil || !again.ObservedAt.Equal(first.ObservedAt) {
		t.Fatalf("replay: %+v %v", again, err)
	}
	_, err = s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "quota_exhausted", For: time.Minute, OperationKey: "limit-1"})
	expectCode(t, err, "operation_conflict")
	// A stopped Run can no longer report.
	if _, err := s.StopSession(ctx, id, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.recordRouteLimit(ctx, id, session.CurrentRunID, RouteLimitObservation{Kind: "rate_limited"}); err == nil {
		t.Fatal("stale Run recorded a limit")
	}
}

func TestResumeAndDispatcherFailWithRouteLimited(t *testing.T) {
	s, id := twoRouteFixture(t, nil)
	ctx := context.Background()
	a, w := worker(t, s, id, "resumed")
	session, err := s.StartSession(ctx, id, SessionOptions{Agent: a.ID, Worktree: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopSession(ctx, id, session.ID); err != nil {
		t.Fatal(err)
	}
	// A client-scoped record limits every route of the account.
	rec, err := s.SetRouteLimit(ctx, "", RouteLimitSetOptions{Target: "frontier/frontier-a", Kind: "quota_exhausted", For: time.Hour, Scope: "client"})
	if err != nil || rec.Key != "client:test" {
		t.Fatalf("client scope: %+v %v", rec, err)
	}
	_, err = s.ResumeAgent(ctx, id, a.ID, "")
	expectCode(t, err, "route_limited")
	_, err = s.StartDispatcher(ctx, "", "dispatcher-limited")
	expectCode(t, err, "route_limited")
	if _, err := s.ClearRouteLimit(ctx, "", RouteLimitClearOptions{Target: "frontier/frontier-b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResumeAgent(ctx, id, a.ID, ""); err != nil {
		t.Fatalf("resume after clear: %v", err)
	}
}
