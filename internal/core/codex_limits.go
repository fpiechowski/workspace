package core

import (
	"encoding/json"
	"strings"
	"time"
)

// This file classifies the usage-limit signals the Codex app-server exposes
// over the JSON-RPC bridge. The classifiers are pure functions so the bridge
// and its tests share exactly one interpretation of the protocol. Shapes were
// taken from `codex app-server generate-json-schema` for codex-cli 0.141.0
// (RateLimitSnapshot, RateLimitWindow, TurnError and the CodexErrorInfo
// union). No model call, credential or external account is involved.

// codexRateLimitWindow mirrors the RateLimitWindow schema.
type codexRateLimitWindow struct {
	UsedPercent        int    `json:"usedPercent"`
	ResetsAt           *int64 `json:"resetsAt"`
	WindowDurationMins *int64 `json:"windowDurationMins"`
}

// codexRateLimitSnapshot mirrors the RateLimitSnapshot schema.
type codexRateLimitSnapshot struct {
	Primary              *codexRateLimitWindow `json:"primary"`
	Secondary            *codexRateLimitWindow `json:"secondary"`
	RateLimitReachedType *string               `json:"rateLimitReachedType"`
	LimitID              *string               `json:"limitId"`
	LimitName            *string               `json:"limitName"`
	PlanType             *string               `json:"planType"`
}

// codexTurnError mirrors the TurnError schema.
type codexTurnError struct {
	Message        string          `json:"message"`
	CodexErrorInfo json.RawMessage `json:"codexErrorInfo"`
}

// codexErrorInfo is the decoded CodexErrorInfo union: either a plain variant
// name or a variant object that carries an upstream HTTP status.
type codexErrorInfo struct {
	Name           string
	HTTPStatusCode *int
}

func parseCodexErrorInfo(raw json.RawMessage) codexErrorInfo {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return codexErrorInfo{}
	}
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		return codexErrorInfo{Name: name}
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return codexErrorInfo{}
	}
	for key, value := range object {
		var detail struct {
			HTTPStatusCode *int `json:"httpStatusCode"`
		}
		_ = json.Unmarshal(value, &detail)
		return codexErrorInfo{Name: key, HTTPStatusCode: detail.HTTPStatusCode}
	}
	return codexErrorInfo{}
}

// codexTurnErrorClass is the usage-limit classification of a TurnError. An
// empty Kind means the error is not a limit signal (for example
// serverOverloaded or a non-429 connection failure).
type codexTurnErrorClass struct {
	Kind       string // "", "rate_limited" or "quota_exhausted"
	UsageLimit bool   // usageLimitExceeded: a reset may be fetched once
	Message    string
}

func classifyCodexTurnError(err codexTurnError) codexTurnErrorClass {
	info := parseCodexErrorInfo(err.CodexErrorInfo)
	switch info.Name {
	case "usageLimitExceeded":
		return codexTurnErrorClass{Kind: "quota_exhausted", UsageLimit: true, Message: err.Message}
	case "httpConnectionFailed":
		if info.HTTPStatusCode != nil && *info.HTTPStatusCode == 429 {
			return codexTurnErrorClass{Kind: "rate_limited", Message: err.Message}
		}
	}
	return codexTurnErrorClass{}
}

// codexWindowReset converts the epoch-seconds resetsAt to a UTC time. A
// missing or nonpositive value means the snapshot supplied no reset.
func codexWindowReset(window *codexRateLimitWindow) *time.Time {
	if window == nil || window.ResetsAt == nil || *window.ResetsAt <= 0 {
		return nil
	}
	reset := time.Unix(*window.ResetsAt, 0).UTC()
	return &reset
}

// codexHighestWindow is the window with the greatest usedPercent.
func codexHighestWindow(snapshot codexRateLimitSnapshot) *codexRateLimitWindow {
	var best *codexRateLimitWindow
	for _, window := range []*codexRateLimitWindow{snapshot.Primary, snapshot.Secondary} {
		if window == nil {
			continue
		}
		if best == nil || window.UsedPercent > best.UsedPercent {
			best = window
		}
	}
	return best
}

func parseCodexRateLimitSnapshot(params json.RawMessage) (codexRateLimitSnapshot, bool) {
	var wrapper struct {
		RateLimits codexRateLimitSnapshot `json:"rateLimits"`
	}
	if err := json.Unmarshal(params, &wrapper); err != nil {
		return codexRateLimitSnapshot{}, false
	}
	return wrapper.RateLimits, true
}

// codexRateLimitObservation maps a rateLimits snapshot to a ledger
// observation. An exhausted window or a rateLimitReachedType is a hard limit;
// any other window becomes the soft usage_pressure signal. A snapshot without
// windows and without a reached type is not an observation.
func codexRateLimitObservation(snapshot codexRateLimitSnapshot) (RouteLimitObservation, bool) {
	reached := snapshot.RateLimitReachedType != nil && *snapshot.RateLimitReachedType != ""
	best := codexHighestWindow(snapshot)
	if !reached && (best == nil || best.UsedPercent < 100) {
		if best == nil {
			return RouteLimitObservation{}, false
		}
		percent := best.UsedPercent
		return RouteLimitObservation{Kind: "usage_pressure", Source: "codex_rate_limits", UsedPercent: &percent, ResetAt: codexWindowReset(best)}, true
	}
	kind := "quota_exhausted"
	if reached && *snapshot.RateLimitReachedType == "rate_limit_reached" {
		kind = "rate_limited"
	}
	obs := RouteLimitObservation{Kind: kind, Source: "codex_rate_limits"}
	if best != nil {
		percent := best.UsedPercent
		obs.UsedPercent = &percent
		obs.ResetAt = codexWindowReset(best)
	}
	return obs, true
}

// codexBridgeLimitState remembers the reset the bridge already knows so a
// usageLimitExceeded turn error triggers at most one account/rateLimits/read.
type codexBridgeLimitState struct {
	ResetAt  *time.Time
	ReadSent bool
}
