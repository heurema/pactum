package agents

import (
	"fmt"
	"strings"
)

const (
	DefaultExecutor = "codex"
	InputPromptFile = "prompt_file"
)

func DefaultConfig() AgentConfig {
	return AgentConfig{
		DefaultExecutor: DefaultExecutor,
		DefaultReviewer: DefaultExecutor,
		Adapters: map[string]AdapterConfig{
			"codex": {
				Command: "codex",
				Args:    []string{"exec", "--sandbox", "read-only"},
				Input:   InputPromptFile,
			},
			"claude": {
				Command: "claude",
				Args:    []string{"-p"},
				Input:   InputPromptFile,
			},
		},
	}
}

func NormalizeConfig(config AgentConfig) AgentConfig {
	defaults := DefaultConfig()
	if strings.TrimSpace(config.DefaultExecutor) == "" {
		config.DefaultExecutor = defaults.DefaultExecutor
	}
	if strings.TrimSpace(config.DefaultReviewer) == "" {
		config.DefaultReviewer = config.DefaultExecutor
	}
	if config.Adapters == nil {
		config.Adapters = map[string]AdapterConfig{}
	}
	for name, adapter := range defaults.Adapters {
		if _, ok := config.Adapters[name]; !ok {
			config.Adapters[name] = cloneAdapterConfig(adapter)
		}
	}
	for name, adapter := range config.Adapters {
		config.Adapters[name] = cloneAdapterConfig(adapter)
	}
	return config
}

func ResolveAdapter(config AgentConfig, agentName string) (string, AdapterConfig, error) {
	config = NormalizeConfig(config)
	name := strings.TrimSpace(agentName)
	if name == "" {
		name = config.DefaultExecutor
	}
	adapter, ok := config.Adapters[name]
	if !ok || strings.TrimSpace(adapter.Command) == "" {
		return "", AdapterConfig{}, fmt.Errorf("agent adapter not configured: %s", name)
	}
	return name, cloneAdapterConfig(adapter), nil
}

func ResolveReviewerAdapter(config AgentConfig, reviewerName string) (string, AdapterConfig, error) {
	config = NormalizeConfig(config)
	name := strings.TrimSpace(reviewerName)
	if name == "" {
		name = config.DefaultReviewer
	}
	adapter, ok := config.Adapters[name]
	if !ok || strings.TrimSpace(adapter.Command) == "" {
		return "", AdapterConfig{}, fmt.Errorf("agent adapter not configured: %s", name)
	}
	return name, cloneAdapterConfig(adapter), nil
}

func cloneAdapterConfig(adapter AdapterConfig) AdapterConfig {
	adapter.Args = append([]string{}, adapter.Args...)
	return adapter
}
