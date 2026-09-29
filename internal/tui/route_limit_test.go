package tui

import (
	"strings"
	"testing"
	"time"

	"workspace/internal/core"
)

func intPtr(v int) *int { return &v }

func routeLimitFixture() core.RouteLimit {
	return core.RouteLimit{
		Key:      "route:test/a/test-model",
		Scope:    "route",
		Client:   "test",
		Provider: "a",
		Model:    "test-model",
		Kind:     "quota_exhausted",
		Until:    time.Now().Add(time.Hour),
	}
}

func TestAggregateRouteLimitHealth(t *testing.T) {
	now := time.Now()
	hard := routeLimitFixture()
	hard.Kind = "quota_exhausted"
	pressure := routeLimitFixture()
	pressure.Kind = "usage_pressure"
	pressure.UsedPercent = intPtr(95)
	expired := routeLimitFixture()
	expired.Until = now.Add(-time.Minute)

	cases := []struct {
		name         string
		limits       []core.RouteLimit
		hard         int
		pressure     int
		label        string
		hasIndicator bool
	}{
		{name: "none"},
		{name: "hard", limits: []core.RouteLimit{hard}, hard: 1, label: "routes 1 limited", hasIndicator: true},
		{name: "pressure", limits: []core.RouteLimit{pressure}, pressure: 1, label: "routes 1 pressured", hasIndicator: true},
		{name: "mixed", limits: []core.RouteLimit{hard, pressure}, hard: 1, pressure: 1, label: "routes 1 limited · 1 pressured", hasIndicator: true},
		{name: "expired", limits: []core.RouteLimit{expired}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := aggregateRouteLimitHealth(tc.limits, now)
			if got.hard != tc.hard || got.pressure != tc.pressure {
				t.Fatalf("health = %+v, want hard=%d pressure=%d", got, tc.hard, tc.pressure)
			}
			indicator, ok := got.indicator()
			if ok != tc.hasIndicator {
				t.Fatalf("indicator presence = %t, want %t", ok, tc.hasIndicator)
			}
			if ok && indicator.label != tc.label {
				t.Fatalf("label = %q, want %q", indicator.label, tc.label)
			}
		})
	}
}

func TestHeaderShowsLimitedRoutesAndDetailShowsReset(t *testing.T) {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_limits", NoColor: true})
	m.width, m.height = 120, 24
	m.supervisor = core.SupervisorObservation{State: "running", ObservedAt: time.Now()}
	m.route = route{Page: "tasks"}
	m.snapshot = core.WorkspaceSnapshot{
		ObservedAt: time.Now(),
		Status:     core.Status{Workspace: core.Workspace{ID: "ws_limits", Title: "Limited workspace", Status: "active"}},
		Limits:     []core.RouteLimit{routeLimitFixture()},
	}
	if header := m.header(); !strings.Contains(header, "routes 1 limited") {
		t.Fatalf("header omitted limited route state: %q", header)
	}

	session := core.Session{
		ID:             "sess_limited",
		AgentSnapshot:  core.Agent{Name: "worker"},
		State:          "running",
		ClientSnapshot: core.Client{Adapter: "command"},
		Route:          core.Route{Client: "test", Provider: "a", Model: "test-model"},
		ClientThreadID: "thread-1",
		CurrentRunID:   "run_1",
		LifecycleState: "active",
	}
	m.snapshot.Status.Sessions = []core.Session{session}
	m.route = route{Page: "session", EntityID: session.ID}
	detail := m.detailContent()
	if !strings.Contains(detail, "Usage limit") || !strings.Contains(detail, "quota_exhausted") || !strings.Contains(detail, "Until") {
		t.Fatalf("session detail omitted the usage limit:\n%s", detail)
	}
}

func TestHeaderShowsSoftRoutePressure(t *testing.T) {
	m := New(Config{ProjectFound: true, WorkspaceID: "ws_limits", NoColor: true})
	m.width = 120
	m.supervisor = core.SupervisorObservation{State: "running", ObservedAt: time.Now()}
	m.route = route{Page: "tasks"}
	limit := routeLimitFixture()
	limit.Kind = "usage_pressure"
	limit.UsedPercent = intPtr(96)
	m.snapshot = core.WorkspaceSnapshot{
		ObservedAt: time.Now(),
		Status:     core.Status{Workspace: core.Workspace{ID: "ws_limits", Title: "Limited workspace", Status: "active"}},
		Limits:     []core.RouteLimit{limit},
	}
	if header := m.header(); !strings.Contains(header, "routes 1 pressured") {
		t.Fatalf("header omitted soft pressure state: %q", header)
	}
}
