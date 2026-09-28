package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"slices"
	"strings"
	"time"
)

func workflowProfile(cfg Config, d *Document, role, fallback string) string {
	if !d.State.WorkflowSelected() {
		if role == "orchestrator" && cfg.Defaults.OrchestratorProfile != "" {
			return cfg.Defaults.OrchestratorProfile
		}
		return fallback
	}
	key := map[string]string{"orchestrator": "orchestrator", "planner": "planning", "implementer": "implementation", "integrator": "integration", "tester": "live-testing"}[role]
	profiles := cfg.Workflows[d.State.Workflow.ID].Profiles
	if name := profiles[key]; name != "" {
		return name
	}
	// Existing plan-first configurations map no integration profile; the
	// integrator then uses the implementation profile instead of an unconfigured
	// fallback name.
	if role == "integrator" {
		if name := profiles["implementation"]; name != "" {
			return name
		}
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
	// LimitedUntil is the end of the strongest active hard usage limit that
	// matches this route; Limit is the matching ledger record.
	LimitedUntil *time.Time  `json:"limited_until,omitempty" yaml:"limited_until,omitempty"`
	Limit        *RouteLimit `json:"limit,omitempty" yaml:"limit,omitempty"`
	UsedPercent  *int        `json:"used_percent,omitempty" yaml:"used_percent,omitempty"`
	// SoftLimited routes rank after every eligible route without usage pressure.
	SoftLimited    bool   `json:"soft_limited,omitempty" yaml:"soft_limited,omitempty"`
	UsageLimitMode string `json:"usage_limit_mode,omitempty" yaml:"usage_limit_mode,omitempty"`
}
type RoutingDecision struct {
	Profile         string            `json:"profile" yaml:"profile"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty" yaml:"reasoning_effort,omitempty"`
	Selected        string            `json:"selected,omitempty" yaml:"selected,omitempty"`
	Candidates      []RouteAssessment `json:"candidates" yaml:"candidates"`
	MeasuredAt      time.Time         `json:"measured_at" yaml:"measured_at"`
	Metric          string            `json:"metric" yaml:"metric"`
	Warnings        []string          `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

const usageLimitReasonPrefix = "usage limit: "

func (s *Service) assessRoutes(cfg Config, profile string) (RoutingDecision, error) {
	now := nowUTC()
	out := RoutingDecision{Profile: profile, MeasuredAt: now, Metric: "project launches and active reservations over 24h plus observed route usage limits; not tokens or account usage"}
	p, ok := cfg.Profiles[profile]
	if !ok {
		return out, fail("no_route", "configure model profile %q", profile)
	}
	out.ReasoningEffort = p.ReasoningEffort
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
		for _, run := range d.Registry.Runs {
			key := run.Route.Client + "/" + run.Route.Provider + "/" + run.Route.Model
			if run.Active() {
				active[key]++
			}
			if run.CreatedAt.After(now.Add(-24*time.Hour)) && (run.State != "failed" || run.ExitCode != nil) {
				counts[run.Route.Provider]++
				routeCount[key]++
			} else if run.Active() {
				counts[run.Route.Provider]++
			}
			if run.State == "failed" && run.ExitCode == nil && run.CreatedAt.After(failures[key]) {
				failures[key] = run.CreatedAt
			}
		}
	}
	if cfg, cfgErr := s.Config(); cfgErr == nil {
		if dispatcher, exists, stateErr := loadDispatcherState(s.Root, cfg.ProjectID); stateErr == nil && exists {
			for _, run := range dispatcher.Runs {
				key := run.Route.Client + "/" + run.Route.Provider + "/" + run.Route.Model
				if run.Active() {
					active[key]++
				}
				if run.CreatedAt.After(now.Add(-24*time.Hour)) && (run.State != "failed" || run.ExitCode != nil) {
					counts[run.Route.Provider]++
					routeCount[key]++
				} else if run.Active() {
					counts[run.Route.Provider]++
				}
				if run.State == "failed" && run.ExitCode == nil && run.CreatedAt.After(failures[key]) {
					failures[key] = run.CreatedAt
				}
			}
		}
	}
	// The ledger is advisory: a corrupt file is reported and treated as empty.
	limits, limitsErr := loadRouteLimits(s.Root)
	if limitsErr != nil {
		out.Warnings = append(out.Warnings, limitsErr.Error())
	}
	bestScore := math.Inf(1)
	bestActive, bestCount := math.MaxInt, math.MaxInt
	bestSoft := true
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
		configBlocked := a.Reason != ""
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
		// Configuration and capability failures keep their reason; a usage
		// limit outranks transient load reasons because it outlasts them.
		settings, err := effectiveUsageLimits(p, r)
		if err != nil {
			return out, fail("invalid_config", "usage_limits in route %q of profile %q: %v", r.ID, profile, err)
		}
		if settings.Mode != usageLimitModeAvoid {
			a.UsageLimitMode = settings.Mode
		}
		if settings.Mode != usageLimitModeIgnore {
			match := limits.match(r, now)
			if match.hard != nil {
				until := match.hard.Until
				a.LimitedUntil = &until
				a.Limit = match.hard
				if settings.Mode == usageLimitModeAvoid && !configBlocked {
					a.Reason = usageLimitReasonPrefix + match.hard.Kind + " until " + until.Format(time.RFC3339)
				}
			}
			if match.pressure != nil {
				a.UsedPercent = match.pressure.UsedPercent
				if a.Limit == nil {
					a.Limit = match.pressure
				}
				if settings.Mode == usageLimitModeAvoid && settings.SoftLimitPercent > 0 && usedPercent(*match.pressure) >= settings.SoftLimitPercent {
					a.SoftLimited = true
				}
			}
		}
		a.Eligible = a.Reason == ""
		out.Candidates = append(out.Candidates, a)
		if a.Eligible && routeBetter(a.SoftLimited, a.ProviderScore, a.Active, a.Launches, bestSoft, bestScore, bestActive, bestCount) {
			out.Selected = r.ID
			bestSoft = a.SoftLimited
			bestScore = a.ProviderScore
			bestActive = a.Active
			bestCount = a.Launches
		}
	}
	return out, nil
}

// routeBetter orders eligible routes: no usage pressure first, then the
// weighted provider score, active Runs and route launches.
func routeBetter(soft bool, score float64, active, launches int, bestSoft bool, bestScore float64, bestActive, bestLaunches int) bool {
	if soft != bestSoft {
		return !soft
	}
	if score != bestScore {
		return score < bestScore
	}
	if active != bestActive {
		return active < bestActive
	}
	return launches < bestLaunches
}

// selectedRoute returns the chosen route or the selection error. When nothing
// is eligible and some route is blocked only by a usage limit, the error is
// route_limited with the earliest reset; otherwise it stays no_route.
func selectedRoute(decision RoutingDecision) (Route, error) {
	var earliest *time.Time
	for _, candidate := range decision.Candidates {
		if candidate.Route.ID == decision.Selected && candidate.Eligible {
			return candidate.Route, nil
		}
		if candidate.LimitedUntil != nil && strings.HasPrefix(candidate.Reason, usageLimitReasonPrefix) && (earliest == nil || candidate.LimitedUntil.Before(*earliest)) {
			earliest = candidate.LimitedUntil
		}
	}
	if earliest != nil {
		reset := earliest.UTC().Format(time.RFC3339)
		return Route{}, &Error{Code: "route_limited", Message: fmt.Sprintf("all eligible routes in profile %q are usage-limited; earliest reset %s; use workspace profile limit list", decision.Profile, reset), Options: []string{"retry_after=" + reset}}
	}
	return Route{}, fail("no_route", "no eligible route in profile %q; use workspace profile explain", decision.Profile)
}

// routeLimitedReset extracts the reset time carried by a route_limited error.
func routeLimitedReset(err error) (time.Time, bool) {
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != "route_limited" {
		return time.Time{}, false
	}
	for _, option := range ce.Options {
		value, ok := strings.CutPrefix(option, "retry_after=")
		if !ok {
			continue
		}
		if reset, parseErr := time.Parse(time.RFC3339, value); parseErr == nil {
			return reset, true
		}
	}
	return time.Time{}, false
}

// resumeRouteLimitedError rewrites the generic route_limited selection error
// for a resume, naming the reset time and the wait-or-new-session options.
// The resume fails before a successor Run exists, so a native client thread is
// never silently dropped to switch routes.
func resumeRouteLimitedError(profile string, err error) error {
	reset, ok := routeLimitedReset(err)
	if !ok {
		return err
	}
	formatted := reset.UTC().Format(time.RFC3339)
	return &Error{
		Code:    "route_limited",
		Message: fmt.Sprintf("cannot resume profile %q: its routes are usage-limited until %s; wait until the reset or start a new logical session on another route", profile, formatted),
		Options: []string{"retry_after=" + formatted, "wait_for_reset", "new_session"},
	}
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
