package core

import (
	"context"
	"math"
	"os/exec"
	"slices"
	"time"
)

func workflowProfile(cfg Config, d *Document, role, fallback string) string {
	if d.State.Workflow == nil {
		if role == "orchestrator" && cfg.Defaults.OrchestratorProfile != "" {
			return cfg.Defaults.OrchestratorProfile
		}
		return fallback
	}
	key := map[string]string{"orchestrator": "orchestrator", "planner": "planning", "implementer": "implementation", "integrator": "integration", "tester": "live-testing"}[role]
	if name := cfg.Workflows[d.State.Workflow.ID].Profiles[key]; name != "" {
		return name
	}
	return fallback
}

type RouteAssessment struct {
	Route         Route      `json:"route" yaml:"route"`
	Eligible      bool       `json:"eligible" yaml:"eligible"`
	Reason        string     `json:"reason,omitempty" yaml:"reason,omitempty"`
	Active        int        `json:"active" yaml:"active"`
	Launches      int        `json:"launches" yaml:"launches"`
	ProviderScore float64    `json:"provider_score" yaml:"provider_score"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty" yaml:"cooldown_until,omitempty"`
}
type RoutingDecision struct {
	Profile    string            `json:"profile" yaml:"profile"`
	Selected   string            `json:"selected,omitempty" yaml:"selected,omitempty"`
	Candidates []RouteAssessment `json:"candidates" yaml:"candidates"`
	MeasuredAt time.Time         `json:"measured_at" yaml:"measured_at"`
	Metric     string            `json:"metric" yaml:"metric"`
}

func (s *Service) assessRoutes(cfg Config, profile string) (RoutingDecision, error) {
	now := nowUTC()
	out := RoutingDecision{Profile: profile, MeasuredAt: now, Metric: "project launches and active reservations over 24h; not tokens or account usage"}
	p, ok := cfg.Profiles[profile]
	if !ok {
		return out, fail("no_route", "configure model profile %q", profile)
	}
	counts, active, routeCount := map[string]int{}, map[string]int{}, map[string]int{}
	failures := map[string]time.Time{}
	dirs, err := s.workspaceDirs()
	if err != nil {
		return out, err
	}
	for _, dir := range dirs {
		d, err := loadDocument(dir)
		if err != nil {
			return out, err
		}
		for _, session := range d.Registry.Sessions {
			key := session.Route.Client + "/" + session.Route.Provider + "/" + session.Route.Model
			if session.Active() {
				active[key]++
			}
			if session.CreatedAt.After(now.Add(-24*time.Hour)) && (session.State != "failed" || session.ExitCode != nil) {
				counts[session.Route.Provider]++
				routeCount[key]++
			} else if session.Active() {
				counts[session.Route.Provider]++
			}
			if session.State == "failed" && session.ExitCode == nil && session.CreatedAt.After(failures[key]) {
				failures[key] = session.CreatedAt
			}
		}
	}
	bestScore := math.Inf(1)
	bestActive, bestCount := math.MaxInt, math.MaxInt
	for _, r := range p.Routes {
		key := r.Client + "/" + r.Provider + "/" + r.Model
		weight := p.ProviderWeights[r.Provider]
		if weight == 0 {
			weight = 1
		}
		a := RouteAssessment{Route: r, Eligible: true, Active: active[key], Launches: routeCount[key], ProviderScore: float64(counts[r.Provider]) / weight}
		client := cfg.Clients[r.Client]
		if len(client.LaunchArgv) == 0 {
			a.Reason = "client unavailable"
		} else if _, err := exec.LookPath(client.LaunchArgv[0]); err != nil {
			a.Reason = "client executable unavailable"
		}
		for _, cap := range p.RequiredCapabilities {
			if !slices.Contains(client.Capabilities, cap) {
				a.Reason = "missing capability: " + cap
				break
			}
		}
		cooldown := p.CooldownSeconds
		if cooldown == 0 {
			cooldown = 30
		}
		if until := failures[key].Add(time.Duration(cooldown) * time.Second); until.After(now) {
			a.CooldownUntil = &until
			a.Reason = "client launch failure cooldown"
		}
		if a.Active >= r.MaxConcurrency {
			a.Reason = "concurrency limit"
		}
		if r.MaxLaunches24h > 0 && a.Launches >= r.MaxLaunches24h {
			a.Reason = "24h launch budget exhausted"
		}
		a.Eligible = a.Reason == ""
		out.Candidates = append(out.Candidates, a)
		if a.Eligible && (a.ProviderScore < bestScore || (a.ProviderScore == bestScore && (a.Active < bestActive || (a.Active == bestActive && a.Launches < bestCount)))) {
			out.Selected = r.ID
			bestScore = a.ProviderScore
			bestActive = a.Active
			bestCount = a.Launches
		}
	}
	return out, nil
}
func (s *Service) ExplainProfile(ctx context.Context, profile string) (RoutingDecision, error) {
	unlock, err := lockProject(ctx, s.Root)
	if err != nil {
		return RoutingDecision{}, err
	}
	defer unlock()
	cfg, err := s.Config()
	if err != nil {
		return RoutingDecision{}, err
	}
	return s.assessRoutes(cfg, profile)
}
