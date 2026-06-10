package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/codeindex"
	"gopkg.in/yaml.v3"
)

type workspaceManifest struct {
	Schema        string    `json:"schema"`
	Tool          string    `json:"tool"`
	ToolVersion   string    `json:"tool_version"`
	RepoRoot      string    `json:"repo_root"`
	InitializedAt time.Time `json:"initialized_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Map           struct {
		CurrentRunID string `json:"current_run_id"`
	} `json:"map"`
	Status string `json:"status"`
}

type configFile struct {
	Schema  string               `yaml:"schema"`
	Agents  []agentRegistryEntry `yaml:"agents"`
	Map     mapConfig            `yaml:"map"`
	Gate    gateConfig           `yaml:"gate"`
	Clarify clarifyConfig        `yaml:"clarify"`
	Review  reviewConfig         `yaml:"review"`
}

type mapConfig struct {
	MaxFileBytes int    `yaml:"max_file_bytes"`
	CodeIndex    string `yaml:"code_index"`
}

type gateConfig struct {
	ScopeEnforcement string `yaml:"scope_enforcement"`
}

// clarifyConfig bounds the autonomous clarify loop. MaxRounds is the Phase 1
// round cap; like the review limits it is resolved at loop time, where a
// non-positive value falls back to the default.
type clarifyConfig struct {
	MaxRounds int `yaml:"max_rounds"`
}

type reviewConfig struct {
	MaxRounds   int                `yaml:"max_rounds"`
	Patience    int                `yaml:"patience"`
	CleanRounds int                `yaml:"clean_rounds"`
	Budget      reviewBudgetConfig `yaml:"budget"`
	Panel       []string           `yaml:"panel"`
}

type reviewBudgetConfig struct {
	Mode      string `yaml:"mode"`
	MaxTokens *int64 `yaml:"max_tokens"`
}

// agentRegistryEntry registers one invocable agent in the top-level agents
// registry, the config's source of truth for agents. Name is how the entry is
// referenced everywhere (review.panel, --agent, --reviewer); Agent is the
// built-in it runs on (defaulting to Name); Model and Effort are optional pins
// that travel with the name wherever it is used. A name is decoupled from the
// built-in, so two entries may run the same built-in with different pins.
type agentRegistryEntry struct {
	Name   string `yaml:"name"`
	Agent  string `yaml:"agent,omitempty"`
	Model  string `yaml:"model,omitempty"`
	Effort string `yaml:"effort,omitempty"`
}

func (e agentRegistryEntry) modelSpec() agents.ModelSpec {
	return agents.ModelSpec{Model: strings.TrimSpace(e.Model), Effort: strings.TrimSpace(e.Effort)}
}

func writeDefaultConfigIfMissing(path string) error {
	if activeStore.Exists(path) {
		return nil
	}
	return writeYAML(path, defaultConfigFile())
}

func defaultConfigFile() configFile {
	return configFile{
		Schema: configSchema,
		Agents: []agentRegistryEntry{
			{Name: agents.BuiltinClaude},
		},
		Map: mapConfig{
			MaxFileBytes: 500000,
			CodeIndex:    codeindex.ModeAuto,
		},
		Gate: gateConfig{
			ScopeEnforcement: gateScopeEnforcementBlock,
		},
		Clarify: clarifyConfig{
			MaxRounds: 3,
		},
		Review: reviewConfig{
			MaxRounds:   10,
			Patience:    2,
			CleanRounds: 1,
			Budget: reviewBudgetConfig{
				Mode:      budgetModeBlock,
				MaxTokens: nil,
			},
			Panel: []string{},
		},
	}
}

func writeYAML(path string, value any) error {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return activeStore.WriteBytes(path, buffer.Bytes(), 0o644)
}

func readWorkspaceManifest(path string) (workspaceManifest, error) {
	var manifest workspaceManifest
	if err := readJSON(path, &manifest); err != nil {
		return workspaceManifest{}, err
	}
	if manifest.Schema == "" || manifest.RepoRoot == "" {
		return workspaceManifest{}, errors.New("workspace manifest is incomplete")
	}
	return manifest, nil
}

func (a App) readConfig(path string) (configFile, error) {
	var config configFile
	data, err := activeStore.ReadBytes(path)
	if err != nil {
		return configFile{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	// Reject unknown keys so removed or mistyped keys fail loudly instead of
	// accumulating silently as dead config.
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil && !errors.Is(err, io.EOF) {
		return configFile{}, fmt.Errorf("config %s: %w", path, err)
	}
	if config.Schema == "" {
		return configFile{}, errors.New("config is incomplete")
	}
	scopeEnforcement, err := normalizeGateScopeEnforcement(config.Gate.ScopeEnforcement)
	if err != nil {
		return configFile{}, err
	}
	config.Gate.ScopeEnforcement = scopeEnforcement
	budgetMode, err := normalizeBudgetMode(config.Review.Budget.Mode)
	if err != nil {
		return configFile{}, err
	}
	config.Review.Budget.Mode = budgetMode
	if err := validateAgentRegistry(config.Agents, a.agentRegistry().ListBuiltins()); err != nil {
		return configFile{}, err
	}
	if err := validateReviewPanel(config.Review.Panel, config.Agents); err != nil {
		return configFile{}, err
	}
	return config, nil
}

// validateAgentRegistry enforces the agents-registry rules: at least one entry
// (the registry is the only way to make an agent invocable), non-empty unique
// names, an underlying agent (after defaulting to the name) that is a built-in,
// and model pins carrying no ':' effort suffix (effort is its own key). Names
// and underlying agents are normalized in place so resolution can rely on a
// trimmed Name and a non-empty Agent.
// agentNameIsPathSafe reports whether a registry name is safe to embed in
// artifact file names (no path separators or other special characters).
func agentNameIsPathSafe(name string) bool {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func validateAgentRegistry(entries []agentRegistryEntry, builtins []agents.AgentDescriptor) error {
	if len(entries) == 0 {
		return errors.New("config agents: at least one agent must be registered")
	}
	known := map[string]bool{}
	knownNames := make([]string, 0, len(builtins))
	for _, descriptor := range builtins {
		known[descriptor.Name] = true
		knownNames = append(knownNames, descriptor.Name)
	}
	seen := map[string]bool{}
	for i := range entries {
		name := strings.TrimSpace(entries[i].Name)
		if name == "" {
			return fmt.Errorf("config agents: entry is missing the name")
		}
		if seen[name] {
			return fmt.Errorf("config agents: duplicate name %q", name)
		}
		seen[name] = true
		// Registry names flow into per-member artifact paths (the review lens
		// prompts), so they must be path-safe.
		if !agentNameIsPathSafe(name) {
			return fmt.Errorf("config agents: name %q must contain only letters, digits, '.', '_', or '-'", name)
		}
		agent := strings.TrimSpace(entries[i].Agent)
		if agent == "" {
			agent = name
		}
		if !known[agent] {
			return fmt.Errorf("config agents: entry %q: unknown agent %q (built-ins: %s)", name, agent, strings.Join(knownNames, ", "))
		}
		if strings.Contains(entries[i].Model, ":") {
			return fmt.Errorf("config agents: entry %q: model %q must not contain ':'; set the effort key instead", name, entries[i].Model)
		}
		entries[i].Name = name
		entries[i].Agent = agent
	}
	return nil
}

// validateReviewPanel checks that every panel member references a registered
// name exactly once. Two names backed by the same built-in are allowed — the
// panel is a list of registry names, not built-ins.
func validateReviewPanel(panel []string, registry []agentRegistryEntry) error {
	registered := map[string]bool{}
	for _, entry := range registry {
		registered[entry.Name] = true
	}
	seen := map[string]bool{}
	for i := range panel {
		name := strings.TrimSpace(panel[i])
		if name == "" {
			return fmt.Errorf("config review.panel: entry is missing the agent name")
		}
		if !registered[name] {
			return fmt.Errorf("config review.panel: unknown agent %q (not registered in config agents)", name)
		}
		if seen[name] {
			return fmt.Errorf("config review.panel: duplicate name %q", name)
		}
		seen[name] = true
		panel[i] = name
	}
	return nil
}

func normalizeGateScopeEnforcement(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return gateScopeEnforcementBlock, nil
	}
	switch value {
	case gateScopeEnforcementBlock, gateScopeEnforcementWarn:
		return value, nil
	default:
		return "", fmt.Errorf("config gate.scope_enforcement must be %q or %q, got %q", gateScopeEnforcementBlock, gateScopeEnforcementWarn, value)
	}
}

func normalizeBudgetMode(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return budgetModeBlock, nil
	}
	switch value {
	case budgetModeBlock, budgetModeWarn:
		return value, nil
	default:
		return "", fmt.Errorf("config review.budget.mode must be %q or %q, got %q", budgetModeBlock, budgetModeWarn, value)
	}
}
