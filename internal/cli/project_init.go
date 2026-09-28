package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"workspace/internal/core"
)

var errInitCancelled = errors.New("project initialization cancelled")

type initPrompt struct {
	in  *bufio.Reader
	out io.Writer
}

func newInitPrompt(in io.Reader, out io.Writer) *initPrompt {
	return &initPrompt{in: bufio.NewReader(in), out: out}
}

func (p *initPrompt) ask(label, suggestion string, required bool) (string, error) {
	if suggestion != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", label, suggestion)
	} else {
		fmt.Fprintf(p.out, "%s: ", label)
	}
	line, err := p.in.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", errInitCancelled
	}
	line = strings.TrimSpace(line)
	if line == "q" || strings.EqualFold(line, "cancel") {
		return "", errInitCancelled
	}
	if line == "" {
		line = suggestion
	}
	if required && line == "" {
		fmt.Fprintln(p.out, "A value is required.")
		return p.ask(label, suggestion, required)
	}
	if required && strings.Contains(strings.ToUpper(line), "YOUR_") {
		fmt.Fprintln(p.out, "README placeholders are examples; enter an account-specific identifier.")
		return p.ask(label, suggestion, required)
	}
	return line, nil
}

func (p *initPrompt) yesNo(label string, suggestion bool) (bool, error) {
	def := "y/N"
	if suggestion {
		def = "Y/n"
	}
	fmt.Fprintf(p.out, "%s [%s]: ", label, def)
	line, err := p.in.ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, errInitCancelled
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "q" || line == "cancel" {
		return false, errInitCancelled
	}
	if line == "" {
		return suggestion, nil
	}
	switch line {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		fmt.Fprintln(p.out, "Please answer y or n.")
		return p.yesNo(label, suggestion)
	}
}

func runProjectInitWizard(ctx context.Context, dir string, in io.Reader, out io.Writer) (core.Config, error) {
	p := newInitPrompt(in, out)
	fmt.Fprintf(out, "Initialize workspace project in %s?\n", dir)
	fmt.Fprintln(out, "This creates templates, Git exclude rules, and .workspace/config.yaml.")
	start, err := p.yesNo("Start setup", true)
	if err != nil || !start {
		return core.Config{}, errInitCancelled
	}

	clients, err := wizardClients(p)
	if err != nil {
		return core.Config{}, err
	}
	var client string
	for {
		client, err = p.ask("Default orchestrator client (configured name)", clients[0], true)
		if err != nil {
			return core.Config{}, err
		}
		if contains(clients, client) {
			break
		}
		fmt.Fprintln(out, "Choose one of the configured client names.")
	}
	provider, err := p.ask("Provider/account identifier", "", true)
	if err != nil {
		return core.Config{}, err
	}
	model, err := p.ask("Orchestrator model identifier", "", true)
	if err != nil {
		return core.Config{}, err
	}
	effort, err := p.ask("Orchestrator reasoning effort (none omits it)", "high", false)
	if err != nil {
		return core.Config{}, err
	}
	concurrencyText, err := p.ask("Orchestrator max concurrency", "3", true)
	if err != nil {
		return core.Config{}, err
	}
	concurrency, err := strconv.Atoi(concurrencyText)
	for err != nil || concurrency < 1 {
		fmt.Fprintln(out, "Concurrency must be a positive integer.")
		concurrencyText, err = p.ask("Orchestrator max concurrency", "3", true)
		if err != nil {
			return core.Config{}, err
		}
		concurrency, err = strconv.Atoi(concurrencyText)
	}

	cfg := core.Config{Clients: make(map[string]core.Client), Profiles: make(map[string]core.Profile), Defaults: core.DefaultsConfig{OrchestratorProfile: "orchestrator", DispatcherProfile: "orchestrator"}}
	for _, name := range clients {
		cfg.Clients[name] = core.Client{Adapter: name}
	}
	orchestrator := core.Profile{Routes: []core.Route{{ID: "orchestrator", Client: client, Provider: provider, Model: model, MaxConcurrency: concurrency}}, ReasoningEffort: effort}
	if strings.EqualFold(effort, "none") {
		orchestrator.ReasoningEffort = ""
	}
	cfg.Profiles["orchestrator"] = orchestrator

	roles, err := p.yesNo("Set up README workflow roles (thinker, worker, supervisor)", true)
	if err != nil {
		return core.Config{}, err
	}
	if roles {
		roleSpecs := []struct {
			name, route, effort, concurrency string
		}{
			{"thinker", "planner", "medium", "1"}, {"worker", "worker", "medium", "3"}, {"supervisor", "supervisor", "low", "1"},
		}
		for _, spec := range roleSpecs {
			roleProvider, e := p.ask(spec.name+" provider/account (Enter reuses "+provider+")", provider, true)
			if e != nil {
				return core.Config{}, e
			}
			roleModel, e := p.ask(spec.name+" model identifier", model, true)
			if e != nil {
				return core.Config{}, e
			}
			count, e := strconv.Atoi(spec.concurrency)
			if e != nil {
				count = 1
			}
			cfg.Profiles[spec.name] = core.Profile{ReasoningEffort: spec.effort, Routes: []core.Route{{ID: spec.route, Client: client, Provider: roleProvider, Model: roleModel, MaxConcurrency: count}}}
		}
		cfg.Workflows = map[string]core.WorkflowConfig{
			"plan-first": {Profiles: map[string]string{"orchestrator": "orchestrator", "planning": "thinker", "implementation": "worker", "integration": "worker"}, MaxParallelTasks: 3, Capabilities: []string{"tasks", "phases", "tasks.role.planner", "tasks.role.implementer", "tasks.role.integrator", "planner_dependency", "integration", "landing"}},
		}
	}
	forge, err := p.yesNo("Configure the optional GitHub forge suggestion", false)
	if err != nil {
		return core.Config{}, err
	}
	if forge {
		cfg.Forge = core.ForgeConfig{Adapter: "github", Remote: "origin", Publication: "ask"}
	}

	fmt.Fprintln(out, "\nProposed configuration:")
	fmt.Fprintf(out, "  clients: %s\n  orchestrator: %s / %s / %s\n", strings.Join(clients, ", "), client, provider, model)
	fmt.Fprintln(out, "  launch and resume argv contain no approval-bypass flags.")
	write, err := p.yesNo("Write configuration?", false)
	if err != nil || !write {
		return core.Config{}, errInitCancelled
	}
	return core.InitFreshProject(ctx, dir, cfg)
}

func wizardClients(p *initPrompt) ([]string, error) {
	available := make([]string, 0, 3)
	for _, name := range []string{"codex", "claude", "opencode"} {
		_, err := exec.LookPath(name)
		status := "not detected"
		if err == nil {
			available = append(available, name)
			status = "detected"
		}
		fmt.Fprintf(p.out, "  %s (%s)\n", name, status)
	}
	defaultName := ""
	if len(available) > 0 {
		defaultName = available[0]
	}
	selection, err := p.ask("Client adapters (comma-separated: codex, claude, opencode)", defaultName, defaultName == "")
	if err != nil {
		return nil, err
	}
	var result []string
	for _, item := range strings.Split(selection, ",") {
		name := strings.TrimSpace(strings.ToLower(item))
		if !contains([]string{"codex", "claude", "opencode"}, name) || contains(result, name) {
			continue
		}
		result = append(result, name)
	}
	if len(result) == 0 {
		fmt.Fprintln(p.out, "Select at least one built-in adapter.")
		return wizardClients(p)
	}
	return result, nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
