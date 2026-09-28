package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Route usage limits are advisory, project-scoped observations that a client
// account or provider refused work. They live in one ledger beside
// config.yaml, are written only under the project lock and are consumed by
// assessRoutes. A missing or corrupt ledger never blocks a launch.

const (
	routeLimitSchemaVersion = 1
	routeLimitRetention     = 7 * 24 * time.Hour
	routeLimitMaxReset      = 7 * 24 * time.Hour
	routeLimitMessageBytes  = 300

	usageLimitModeAvoid   = "avoid"
	usageLimitModeObserve = "observe"
	usageLimitModeIgnore  = "ignore"

	defaultUsageBackoffSeconds    = 900
	defaultUsageMaxBackoffSeconds = 6 * 60 * 60
	defaultSoftLimitPercent       = 90
)

var (
	routeLimitScopes = []string{"client", "provider", "route"}
	routeLimitKinds  = []string{"rate_limited", "quota_exhausted", "usage_pressure"}
)

// RouteLimit is one ledger record. Scope selects which routes it matches:
// every route of a client (account), every route of a client and provider, or
// one exact client/provider/model route.
type RouteLimit struct {
	Key            string     `json:"key"`
	Scope          string     `json:"scope"`
	Client         string     `json:"client"`
	Provider       string     `json:"provider,omitempty"`
	Model          string     `json:"model,omitempty"`
	Kind           string     `json:"kind"`
	Source         string     `json:"source"`
	ObservedAt     time.Time  `json:"observed_at"`
	ResetAt        *time.Time `json:"reset_at,omitempty"`
	Until          time.Time  `json:"until"`
	BackoffSeconds int        `json:"backoff_seconds,omitempty"`
	UsedPercent    *int       `json:"used_percent,omitempty"`
	RunID          string     `json:"run_id,omitempty"`
	WorkspaceID    string     `json:"workspace_id,omitempty"`
	Message        string     `json:"message,omitempty"`
	ClearedAt      *time.Time `json:"cleared_at,omitempty"`
}

// Active reports whether the record still affects selection at now.
func (l RouteLimit) Active(now time.Time) bool {
	return l.ClearedAt == nil && now.Before(l.Until)
}

// Matches reports whether the record applies to a route: a client-scoped
// record matches every route of the account, a provider-scoped record matches
// the client and provider, and a route-scoped record also needs the model.
func (l RouteLimit) Matches(r Route) bool {
	if l.Client != r.Client {
		return false
	}
	if l.Scope != "client" && l.Provider != r.Provider {
		return false
	}
	return l.Scope != "route" || l.Model == r.Model
}

// hard reports whether the record excludes a route; usage pressure only
// lowers its preference.
func (l RouteLimit) hard() bool { return l.Kind != "usage_pressure" }

type routeLimitReceipt struct {
	ID     string       `json:"id"`
	Digest string       `json:"digest"`
	Result []RouteLimit `json:"result"`
}

type routeLimitLedger struct {
	SchemaVersion int                          `json:"schema_version"`
	ProjectID     string                       `json:"project_id,omitempty"`
	Limits        map[string]RouteLimit        `json:"limits"`
	Operations    map[string]routeLimitReceipt `json:"operations,omitempty"`
	UpdatedAt     time.Time                    `json:"updated_at"`
}

func routeLimitsPath(root string) string {
	return filepath.Join(root, ".workspace", "route-limits.json")
}

func routeLimitKey(scope, client, provider, model string) string {
	switch scope {
	case "client":
		return "client:" + client
	case "provider":
		return "provider:" + client + "/" + provider
	default:
		return "route:" + client + "/" + provider + "/" + model
	}
}

// loadRouteLimits returns an empty ledger for a missing file. A decode error
// is returned together with an empty ledger so callers can report it without
// blocking launches.
func loadRouteLimits(root string) (routeLimitLedger, error) {
	empty := routeLimitLedger{SchemaVersion: routeLimitSchemaVersion, Limits: map[string]RouteLimit{}, Operations: map[string]routeLimitReceipt{}}
	var l routeLimitLedger
	if err := readJSON(routeLimitsPath(root), &l); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return empty, nil
		}
		return empty, fail("route_limits_invalid", "%s: %v", routeLimitsPath(root), err)
	}
	if l.SchemaVersion != routeLimitSchemaVersion {
		return empty, fail("route_limits_invalid", "%s: unsupported schema_version %d", routeLimitsPath(root), l.SchemaVersion)
	}
	if l.Limits == nil {
		l.Limits = map[string]RouteLimit{}
	}
	if l.Operations == nil {
		l.Operations = map[string]routeLimitReceipt{}
	}
	for key, limit := range l.Limits {
		limit.Key = key
		l.Limits[key] = limit
	}
	return l, nil
}

// loadRouteLimitsForWrite replaces a corrupt advisory ledger instead of
// refusing new observations; the corrupt file is kept beside it for review.
func loadRouteLimitsForWrite(root string) (routeLimitLedger, error) {
	l, err := loadRouteLimits(root)
	if err != nil {
		path := routeLimitsPath(root)
		if renameErr := os.Rename(path, path+".invalid-"+nowUTC().Format("20060102T150405Z")); renameErr != nil {
			return l, renameErr
		}
	}
	return l, nil
}

func saveRouteLimits(root, projectID string, l routeLimitLedger, now time.Time) error {
	for key, limit := range l.Limits {
		if limit.Until.Before(now.Add(-routeLimitRetention)) {
			delete(l.Limits, key)
		}
	}
	l.SchemaVersion = routeLimitSchemaVersion
	if projectID != "" {
		l.ProjectID = projectID
	}
	l.UpdatedAt = now
	return writeJSON(routeLimitsPath(root), l)
}

// UsageLimitSettings are the effective usage_limits for one route.
type UsageLimitSettings struct {
	Mode                  string `json:"mode"`
	DefaultBackoffSeconds int    `json:"default_backoff_seconds"`
	MaxBackoffSeconds     int    `json:"max_backoff_seconds"`
	SoftLimitPercent      int    `json:"soft_limit_percent"`
}

func validateUsageLimits(u *UsageLimits) error {
	if u == nil {
		return nil
	}
	switch u.Mode {
	case "", usageLimitModeAvoid, usageLimitModeObserve, usageLimitModeIgnore:
	default:
		return errors.New("mode must be avoid, observe or ignore")
	}
	if u.DefaultBackoffSeconds < 0 || u.MaxBackoffSeconds < 0 {
		return errors.New("backoff seconds must be nonnegative")
	}
	if u.SoftLimitPercent != nil && (*u.SoftLimitPercent < 0 || *u.SoftLimitPercent > 100) {
		return errors.New("soft_limit_percent must be between 0 and 100")
	}
	return nil
}

// effectiveUsageLimits merges route override, profile block and defaults.
func effectiveUsageLimits(p Profile, r Route) (UsageLimitSettings, error) {
	out := UsageLimitSettings{Mode: usageLimitModeAvoid, DefaultBackoffSeconds: defaultUsageBackoffSeconds, MaxBackoffSeconds: defaultUsageMaxBackoffSeconds, SoftLimitPercent: defaultSoftLimitPercent}
	for _, u := range []*UsageLimits{p.UsageLimits, r.UsageLimits} {
		if u == nil {
			continue
		}
		if u.Mode != "" {
			out.Mode = u.Mode
		}
		if u.DefaultBackoffSeconds > 0 {
			out.DefaultBackoffSeconds = u.DefaultBackoffSeconds
		}
		if u.MaxBackoffSeconds > 0 {
			out.MaxBackoffSeconds = u.MaxBackoffSeconds
		}
		if u.SoftLimitPercent != nil {
			out.SoftLimitPercent = *u.SoftLimitPercent
		}
	}
	if out.MaxBackoffSeconds < out.DefaultBackoffSeconds {
		return out, errors.New("max_backoff_seconds must be at least default_backoff_seconds")
	}
	return out, nil
}

// defaultRouteLimitScope follows account semantics: Codex and Claude logins
// are limited as a whole client, other adapters per provider.
func defaultRouteLimitScope(adapter string) string {
	if adapter == "codex" || adapter == "claude" {
		return "client"
	}
	return "provider"
}

// RouteLimitObservation is one limit signal before it is bound to a route.
type RouteLimitObservation struct {
	Scope       string
	Kind        string
	Source      string
	ResetAt     *time.Time
	RetryAfter  time.Duration
	UsedPercent *int
	Message     string
	ObservedAt  time.Time
	RunID       string
	WorkspaceID string
}

func sanitizeRouteLimitMessage(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if len(message) <= routeLimitMessageBytes {
		return message
	}
	cut := routeLimitMessageBytes
	for cut > 0 && (message[cut]&0xC0) == 0x80 {
		cut--
	}
	return message[:cut]
}

// applyRouteLimit binds an observation to the route and merges it into the
// ledger. A later observation replaces an older one for the same key; an
// out-of-order older observation is ignored.
func applyRouteLimit(l *routeLimitLedger, route Route, adapter string, settings UsageLimitSettings, obs RouteLimitObservation) (RouteLimit, error) {
	now := obs.ObservedAt
	if now.IsZero() {
		now = nowUTC()
	}
	if !slices.Contains(routeLimitKinds, obs.Kind) {
		return RouteLimit{}, fail("invalid_argument", "kind must be rate_limited, quota_exhausted or usage_pressure")
	}
	scope := obs.Scope
	if scope == "" {
		scope = defaultRouteLimitScope(adapter)
	}
	if !slices.Contains(routeLimitScopes, scope) {
		return RouteLimit{}, fail("invalid_argument", "scope must be client, provider or route")
	}
	if obs.UsedPercent != nil && (*obs.UsedPercent < 0 || *obs.UsedPercent > 100) {
		return RouteLimit{}, fail("invalid_argument", "used percent must be between 0 and 100")
	}
	if obs.RetryAfter < 0 {
		return RouteLimit{}, fail("invalid_argument", "retry-after must be positive")
	}
	key := routeLimitKey(scope, route.Client, route.Provider, route.Model)
	prior, hasPrior := l.Limits[key]
	if hasPrior && prior.ObservedAt.After(now) {
		return prior, nil
	}
	rec := RouteLimit{Key: key, Scope: scope, Client: route.Client, Kind: obs.Kind, Source: obs.Source, ObservedAt: now, UsedPercent: obs.UsedPercent, RunID: obs.RunID, WorkspaceID: obs.WorkspaceID, Message: sanitizeRouteLimitMessage(obs.Message)}
	if scope != "client" {
		rec.Provider = route.Provider
	}
	if scope == "route" {
		rec.Model = route.Model
	}
	switch {
	case obs.ResetAt != nil:
		reset := obs.ResetAt.UTC()
		if limit := now.Add(routeLimitMaxReset); reset.After(limit) {
			reset = limit
		}
		rec.ResetAt = &reset
		rec.Until = reset
	case obs.RetryAfter > 0:
		rec.Until = now.Add(min(obs.RetryAfter, routeLimitMaxReset))
	default:
		backoff := settings.DefaultBackoffSeconds
		// A repeated hard observation inside the previous backoff window
		// doubles the backoff up to the configured maximum.
		if obs.Kind != "usage_pressure" && hasPrior && prior.hard() && prior.ClearedAt == nil && prior.BackoffSeconds > 0 && now.Before(prior.Until) {
			backoff = min(prior.BackoffSeconds*2, settings.MaxBackoffSeconds)
		}
		rec.BackoffSeconds = backoff
		rec.Until = now.Add(time.Duration(backoff) * time.Second)
	}
	l.Limits[key] = rec
	return rec, nil
}

// routeLimitMatch is the strongest active record for one route.
type routeLimitMatch struct {
	hard     *RouteLimit
	pressure *RouteLimit
}

func (l routeLimitLedger) match(r Route, now time.Time) routeLimitMatch {
	var out routeLimitMatch
	keys := make([]string, 0, len(l.Limits))
	for key := range l.Limits {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		rec := l.Limits[key]
		if !rec.Active(now) || !rec.Matches(r) {
			continue
		}
		if rec.hard() {
			if out.hard == nil || rec.Until.After(out.hard.Until) {
				out.hard = &rec
			}
		} else if out.pressure == nil || usedPercent(rec) > usedPercent(*out.pressure) {
			out.pressure = &rec
		}
	}
	return out
}

func usedPercent(l RouteLimit) int {
	if l.UsedPercent == nil {
		return 0
	}
	return *l.UsedPercent
}

// recordRouteLimit persists an observation for a Run. The route and account
// are derived from the Run record, never from the reporting client, and only
// the current active Run of its Session may record.
func (s *Service) recordRouteLimit(ctx context.Context, selector, runID string, obs RouteLimitObservation) (RouteLimit, error) {
	var out RouteLimit
	err := s.With(ctx, selector, func(d *Document) error {
		r, err := findRun(d, runID)
		if err != nil {
			return err
		}
		p, err := findSession(d, r.SessionID)
		if err != nil {
			return err
		}
		if p.CurrentRunID != r.ID || !r.Active() {
			return fail("stale_run", "run no longer owns the session runtime")
		}
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		adapter := p.ClientSnapshot.Adapter
		if adapter == "" {
			adapter = cfg.Clients[r.Route.Client].Adapter
		}
		settings, err := effectiveUsageLimits(cfg.Profiles[r.Profile], r.Route)
		if err != nil {
			return err
		}
		ledger, err := loadRouteLimitsForWrite(s.Root)
		if err != nil {
			return err
		}
		obs.ObservedAt = nowUTC()
		obs.RunID = r.ID
		obs.WorkspaceID = d.State.ID
		out, err = applyRouteLimit(&ledger, r.Route, adapter, settings, obs)
		if err != nil {
			return err
		}
		return saveRouteLimits(s.Root, cfg.ProjectID, ledger, obs.ObservedAt)
	})
	return out, err
}

// RouteLimitReportOptions is a run-scoped report from a client hook or
// wrapper. The route always comes from the calling Run.
type RouteLimitReportOptions struct {
	Scope       string
	Kind        string
	ResetAt     *time.Time
	RetryAfter  time.Duration
	UsedPercent *int
	Message     string
}

// ReportRouteLimit records a limit for the actor's own current Run.
func (s *Service) ReportRouteLimit(ctx context.Context, selector string, opt RouteLimitReportOptions) (RouteLimit, error) {
	if s.Actor.RunID == "" {
		return RouteLimit{}, fail("run_required", "profile limit report runs only inside a Run (WORKSPACE_RUN_ID)")
	}
	if selector == "" {
		return RouteLimit{}, fail("workspace_required", "profile limit report requires the Run's workspace (WORKSPACE_ID)")
	}
	if err := s.With(ctx, selector, func(d *Document) error {
		p, err := s.actor(d)
		if err != nil {
			return err
		}
		if p == nil {
			return fail("run_required", "profile limit report runs only inside a Run (WORKSPACE_RUN_ID)")
		}
		return nil
	}); err != nil {
		return RouteLimit{}, err
	}
	return s.recordRouteLimit(ctx, selector, s.Actor.RunID, RouteLimitObservation{Scope: opt.Scope, Kind: opt.Kind, Source: "report", ResetAt: opt.ResetAt, RetryAfter: opt.RetryAfter, UsedPercent: opt.UsedPercent, Message: opt.Message})
}

// RouteLimitSetOptions is a manual user or orchestrator override. Target is
// "<profile>/<route-id>".
type RouteLimitSetOptions struct {
	Target       string
	Scope        string
	Kind         string
	Until        *time.Time
	For          time.Duration
	Message      string
	OperationKey string
}

// RouteLimitClearOptions clears matching active records for a route. An empty
// Scope clears every active record that matches the route.
type RouteLimitClearOptions struct {
	Target       string
	Scope        string
	OperationKey string
}

func resolveRouteTarget(cfg Config, target string) (Profile, Route, error) {
	profile, id, ok := strings.Cut(target, "/")
	if !ok || profile == "" || id == "" {
		return Profile{}, Route{}, fail("invalid_argument", "target must be <profile>/<route-id>")
	}
	p, ok := cfg.Profiles[profile]
	if !ok {
		return Profile{}, Route{}, fail("not_found", "unknown profile %q", profile)
	}
	for _, r := range p.Routes {
		if r.ID == id {
			return p, r, nil
		}
	}
	return Profile{}, Route{}, fail("not_found", "unknown route %q in profile %q", id, profile)
}

// requireRouteLimitManager allows the user or a workspace orchestrator.
func (s *Service) requireRouteLimitManager(ctx context.Context, selector string) error {
	if s.Actor.AgentID == "" && s.Actor.SessionID == "" && s.Actor.RunID == "" {
		return nil
	}
	if s.Actor.Scope == "project" || selector == "" {
		return fail("forbidden", "only the user or a workspace orchestrator can set or clear route limits")
	}
	return s.With(ctx, selector, s.requireOrchestrator)
}

// mutateRouteLimits runs fn on the ledger under the project lock with
// operation-key replay stored in the ledger itself.
func (s *Service) mutateRouteLimits(ctx context.Context, key string, request any, fn func(*routeLimitLedger, Config, time.Time) ([]RouteLimit, error)) ([]RouteLimit, error) {
	unlock, err := lockProject(ctx, s.Root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	cfg, err := s.Config()
	if err != nil {
		return nil, err
	}
	ledger, err := loadRouteLimitsForWrite(s.Root)
	if err != nil {
		return nil, err
	}
	if key != "" {
		if op, ok := ledger.Operations[key]; ok {
			if op.Digest != payloadDigest(request) {
				return nil, fail("operation_conflict", "operation key %q was already used with another payload", key)
			}
			return op.Result, nil
		}
	}
	now := nowUTC()
	out, err := fn(&ledger, cfg, now)
	if err != nil {
		return nil, err
	}
	if key != "" {
		ledger.Operations[key] = routeLimitReceipt{ID: ID("op"), Digest: payloadDigest(request), Result: out}
	}
	return out, saveRouteLimits(s.Root, cfg.ProjectID, ledger, now)
}

// SetRouteLimit records a manual limit for a configured route.
func (s *Service) SetRouteLimit(ctx context.Context, selector string, opt RouteLimitSetOptions) (RouteLimit, error) {
	if err := s.requireRouteLimitManager(ctx, selector); err != nil {
		return RouteLimit{}, err
	}
	if (opt.Until == nil) == (opt.For <= 0) {
		return RouteLimit{}, fail("invalid_argument", "use exactly one of --until or --for")
	}
	key := opt.OperationKey
	opt.OperationKey = ""
	out, err := s.mutateRouteLimits(ctx, key, opt, func(l *routeLimitLedger, cfg Config, now time.Time) ([]RouteLimit, error) {
		p, r, err := resolveRouteTarget(cfg, opt.Target)
		if err != nil {
			return nil, err
		}
		if opt.Until != nil && !opt.Until.After(now) {
			return nil, fail("invalid_argument", "--until must be in the future")
		}
		settings, err := effectiveUsageLimits(p, r)
		if err != nil {
			return nil, err
		}
		rec, err := applyRouteLimit(l, r, cfg.Clients[r.Client].Adapter, settings, RouteLimitObservation{Scope: opt.Scope, Kind: opt.Kind, Source: "manual", ResetAt: opt.Until, RetryAfter: opt.For, Message: opt.Message, ObservedAt: now})
		if err != nil {
			return nil, err
		}
		return []RouteLimit{rec}, nil
	})
	if err != nil || len(out) == 0 {
		return RouteLimit{}, err
	}
	return out[0], nil
}

// ClearRouteLimit marks the active records matching a route as cleared.
func (s *Service) ClearRouteLimit(ctx context.Context, selector string, opt RouteLimitClearOptions) ([]RouteLimit, error) {
	if err := s.requireRouteLimitManager(ctx, selector); err != nil {
		return nil, err
	}
	if opt.Scope != "" && !slices.Contains(routeLimitScopes, opt.Scope) {
		return nil, fail("invalid_argument", "scope must be client, provider or route")
	}
	key := opt.OperationKey
	opt.OperationKey = ""
	return s.mutateRouteLimits(ctx, key, opt, func(l *routeLimitLedger, cfg Config, now time.Time) ([]RouteLimit, error) {
		_, r, err := resolveRouteTarget(cfg, opt.Target)
		if err != nil {
			return nil, err
		}
		cleared := []RouteLimit{}
		for _, k := range sortedRouteLimitKeys(*l) {
			rec := l.Limits[k]
			if !rec.Active(now) || !rec.Matches(r) || (opt.Scope != "" && rec.Scope != opt.Scope) {
				continue
			}
			at := now
			rec.ClearedAt = &at
			l.Limits[k] = rec
			cleared = append(cleared, rec)
		}
		return cleared, nil
	})
}

func sortedRouteLimitKeys(l routeLimitLedger) []string {
	keys := make([]string, 0, len(l.Limits))
	for key := range l.Limits {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// RouteLimitList is the read model for workspace profile limit list.
type RouteLimitList struct {
	Limits  []RouteLimit `json:"limits"`
	Invalid string       `json:"invalid,omitempty"`
}

// ListRouteLimits returns active records, or every retained record with all.
func (s *Service) ListRouteLimits(ctx context.Context, all bool) (RouteLimitList, error) {
	unlock, err := lockProject(ctx, s.Root)
	if err != nil {
		return RouteLimitList{}, err
	}
	defer unlock()
	out := RouteLimitList{Limits: []RouteLimit{}}
	ledger, err := loadRouteLimits(s.Root)
	if err != nil {
		out.Invalid = err.Error()
	}
	now := nowUTC()
	for _, key := range sortedRouteLimitKeys(ledger) {
		if rec := ledger.Limits[key]; all || rec.Active(now) {
			out.Limits = append(out.Limits, rec)
		}
	}
	return out, nil
}
