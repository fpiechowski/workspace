package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"workspace/internal/core"
)

type healthTone int

const (
	healthNeutral healthTone = iota
	healthSuccess
	healthDanger
)

type healthIndicator struct {
	label string
	tone  healthTone
}

// serviceHealth is derived from the effective, latest record for each stable
// service name. The registry is append-only, so counting every record would
// make an old failed run keep a restarted service unhealthy.
type serviceHealth struct {
	active int
	failed int
}

func aggregateServiceHealth(services []core.BackgroundService) serviceHealth {
	latest := make(map[string]core.BackgroundService, len(services))
	for _, service := range services {
		latest[service.Name] = service
	}

	var out serviceHealth
	for _, service := range latest {
		if service.Active() {
			out.active++
		}
		if service.State == "failed" {
			out.failed++
		}
	}
	return out
}

func (h serviceHealth) indicator() healthIndicator {
	switch {
	case h.failed > 0:
		label := fmt.Sprintf("services %d failed", h.failed)
		if h.active > 0 {
			label += fmt.Sprintf(" · %d active", h.active)
		}
		return healthIndicator{label: label, tone: healthDanger}
	case h.active > 0:
		return healthIndicator{label: fmt.Sprintf("services %d active", h.active), tone: healthSuccess}
	default:
		return healthIndicator{label: "services idle", tone: healthNeutral}
	}
}

func supervisorKnown(observation core.SupervisorObservation) bool {
	return observation.State != "" || !observation.ObservedAt.IsZero()
}

func (m *Model) supervisorIndicator() healthIndicator {
	state := strings.ToLower(strings.TrimSpace(m.supervisor.State))
	if !supervisorKnown(m.supervisor) {
		if m.supervisorPending || m.workspaceRoute() && m.runtimePending {
			return healthIndicator{label: "server checking", tone: healthNeutral}
		}
		return healthIndicator{label: "server unknown", tone: healthNeutral}
	}
	switch state {
	case "running":
		return healthIndicator{label: "server running", tone: healthSuccess}
	case "stopped", "conflict", "unavailable":
		return healthIndicator{label: "server " + state, tone: healthDanger}
	default:
		return healthIndicator{label: "server unknown", tone: healthNeutral}
	}
}

func (m *Model) serviceIndicator() healthIndicator {
	if m.snapshot.ObservedAt.IsZero() {
		if m.snapshotPending {
			return healthIndicator{label: "services checking", tone: healthNeutral}
		}
		return healthIndicator{label: "services unknown", tone: healthNeutral}
	}
	return aggregateServiceHealth(m.snapshot.Services).indicator()
}

func (p palette) healthColor(tone healthTone) lipgloss.Color {
	switch tone {
	case healthSuccess:
		return p.success
	case healthDanger:
		return p.danger
	default:
		return p.subtle
	}
}

// renderHealthIndicator keeps the semantic dot and its state text as separate
// styled spans. The marker and label remain present when color is disabled.
func (p palette) renderHealthIndicator(indicator healthIndicator, markerOnly bool) string {
	dot := p.style(p.healthColor(indicator.tone), true).Render("●")
	if indicator.tone == healthNeutral {
		dot = p.style(p.healthColor(indicator.tone), true).Render("○")
	}
	if markerOnly {
		return dot
	}
	label := sanitizeLine(indicator.label)
	return dot + " " + p.style(p.healthColor(indicator.tone), false).Render(label)
}

func (m *Model) workspaceRoute() bool {
	return m.workspaceID != "" && m.route.Page != "project"
}

func (m *Model) fullHeaderHealth() string {
	server := m.palette.renderHealthIndicator(m.supervisorIndicator(), false)
	if !m.workspaceRoute() {
		return server
	}
	services := m.palette.renderHealthIndicator(m.serviceIndicator(), false)
	return server + " " + services
}

func (m *Model) compactHeaderHealth() string {
	server := m.palette.renderHealthIndicator(m.supervisorIndicator(), false)
	if !m.workspaceRoute() {
		return server
	}
	service := m.serviceIndicator()
	if service.tone == healthNeutral {
		return server + " " + m.palette.renderHealthIndicator(service, true)
	}
	return server + " " + m.palette.renderHealthIndicator(service, false)
}

func (m *Model) headerServiceOverflow() string {
	if !m.workspaceRoute() {
		return ""
	}
	if m.snapshot.ObservedAt.IsZero() {
		return ""
	}
	fullIdentity, _ := m.headerIdentityText()
	fullHealth := m.fullHeaderHealth()
	if ansi.StringWidth(fullIdentity)+1+ansi.StringWidth(fullHealth) <= m.width {
		return ""
	}
	return sanitizeLine(m.serviceIndicator().label)
}
