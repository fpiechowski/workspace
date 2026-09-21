package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

const reasoningEffortPlaceholder = "{reasoning_effort}"

// expandClientArgv expands the placeholders used by a concrete Run. An empty
// reasoning effort removes its optional argument, including a preceding
// standalone --effort option, so custom wrappers never receive an empty value.
func expandClientArgv(argv []string, route Route, promptFile, prompt, thread, reasoningEffort string) []string {
	replace := strings.NewReplacer(
		"{model}", route.Model,
		"{prompt_file}", promptFile,
		"{prompt}", prompt,
		"{thread_id}", thread,
		reasoningEffortPlaceholder, reasoningEffort,
	)
	out := make([]string, 0, len(argv))
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if reasoningEffort == "" {
			if arg == "--effort" && i+1 < len(argv) && strings.Contains(argv[i+1], reasoningEffortPlaceholder) {
				i++
				continue
			}
			if strings.Contains(arg, reasoningEffortPlaceholder) {
				continue
			}
		}
		out = append(out, replace.Replace(arg))
	}
	return out
}

func hasClaudeEffortOption(argv []string) bool {
	for _, arg := range argv {
		if strings.Contains(arg, reasoningEffortPlaceholder) || arg == "--effort" || strings.HasPrefix(arg, "--effort=") {
			return true
		}
	}
	return false
}

func addClaudeReasoningEffort(argv []string, reasoningEffort string) []string {
	if reasoningEffort == "" || hasClaudeEffortOption(argv) {
		return append([]string(nil), argv...)
	}
	insertAt := len(argv)
	for i, arg := range argv {
		if arg == "--prompt" || arg == "{prompt}" || arg == "{prompt_file}" || strings.Contains(arg, "{prompt}") || strings.Contains(arg, "{prompt_file}") {
			insertAt = i
			break
		}
	}
	out := make([]string, 0, len(argv)+2)
	out = append(out, argv[:insertAt]...)
	out = append(out, "--effort", reasoningEffort)
	out = append(out, argv[insertAt:]...)
	return out
}

func lookupEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix), true
		}
	}
	return "", false
}

func openCodeInitialAgent(argv []string) string {
	for i, arg := range argv {
		switch {
		case arg == "--agent" && i+1 < len(argv) && strings.TrimSpace(argv[i+1]) != "":
			return strings.TrimSpace(argv[i+1])
		case strings.HasPrefix(arg, "--agent=") && strings.TrimSpace(strings.TrimPrefix(arg, "--agent=")) != "":
			return strings.TrimSpace(strings.TrimPrefix(arg, "--agent="))
		}
	}
	return "build"
}

func mergeOpenCodeConfigContent(content string, argv []string, reasoningEffort string) (string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return "", fmt.Errorf("OPENCODE_CONFIG_CONTENT is not valid JSON: %w", err)
	}
	if root == nil {
		return "", fmt.Errorf("OPENCODE_CONFIG_CONTENT must be a JSON object")
	}
	agents := map[string]json.RawMessage{}
	if raw, ok := root["agent"]; ok {
		if err := json.Unmarshal(raw, &agents); err != nil || agents == nil {
			return "", fmt.Errorf("OPENCODE_CONFIG_CONTENT.agent must be a JSON object")
		}
	}
	initialAgent := openCodeInitialAgent(argv)
	settings := map[string]json.RawMessage{}
	if raw, ok := agents[initialAgent]; ok {
		if err := json.Unmarshal(raw, &settings); err != nil || settings == nil {
			return "", fmt.Errorf("OPENCODE_CONFIG_CONTENT.agent.%s must be a JSON object", initialAgent)
		}
	}
	variant, err := json.Marshal(reasoningEffort)
	if err != nil {
		return "", fmt.Errorf("encode OpenCode variant: %w", err)
	}
	settings["variant"] = variant
	agentSettings, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("encode OpenCode agent configuration: %w", err)
	}
	agents[initialAgent] = agentSettings
	agentConfig, err := json.Marshal(agents)
	if err != nil {
		return "", fmt.Errorf("encode OpenCode agent configuration: %w", err)
	}
	root["agent"] = agentConfig
	merged, err := json.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("encode OpenCode configuration: %w", err)
	}
	return string(merged), nil
}

func withOpenCodeReasoningEffortEnv(env, argv []string, reasoningEffort string) ([]string, error) {
	if reasoningEffort == "" {
		return env, nil
	}
	env = replaceEnv(env, "WORKSPACE_REASONING_EFFORT", reasoningEffort)
	content, ok := lookupEnv(env, "OPENCODE_CONFIG_CONTENT")
	if !ok {
		content = "{}"
	}
	merged, err := mergeOpenCodeConfigContent(content, argv, reasoningEffort)
	if err != nil {
		return nil, fail("invalid_config", "OpenCode reasoning_effort cannot be applied: %v", err)
	}
	return replaceEnv(env, "OPENCODE_CONFIG_CONTENT", merged), nil
}
