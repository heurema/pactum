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
	Schema  string        `yaml:"schema"`
	Map     mapConfig     `yaml:"map"`
	Gate    gateConfig    `yaml:"gate"`
	Execute executeConfig `yaml:"execute"`
	Review  reviewConfig  `yaml:"review"`
}

type mapConfig struct {
	MaxFileBytes int    `yaml:"max_file_bytes"`
	CodeIndex    string `yaml:"code_index"`
}

type gateConfig struct {
	ScopeEnforcement string `yaml:"scope_enforcement"`
}

type executeConfig struct {
	Models []agentModelEntry `yaml:"models"`
}

type reviewConfig struct {
	MaxRounds   int                `yaml:"max_rounds"`
	Patience    int                `yaml:"patience"`
	CleanRounds int                `yaml:"clean_rounds"`
	Budget      reviewBudgetConfig `yaml:"budget"`
	Panel       []agentModelEntry  `yaml:"panel"`
}

type reviewBudgetConfig struct {
	Mode      string `yaml:"mode"`
	MaxTokens *int64 `yaml:"max_tokens"`
}

// agentModelEntry pins a model (and optional reasoning effort) to one agent in
// a stage roster (execute.models, review.panel). Agent is required; model and
// effort are optional (empty = inherit the agent CLI's own configuration). The
// agent is chosen at invocation time, so pins are per-agent entries: a pin only
// applies when the invoked agent matches the entry.
type agentModelEntry struct {
	Agent  string `yaml:"agent"`
	Model  string `yaml:"model,omitempty"`
	Effort string `yaml:"effort,omitempty"`
}

func (e agentModelEntry) modelSpec() agents.ModelSpec {
	return agents.ModelSpec{Model: strings.TrimSpace(e.Model), Effort: strings.TrimSpace(e.Effort)}
}

// findAgentModelEntry returns the entry pinned for the named agent, when one
// exists in the roster.
func findAgentModelEntry(entries []agentModelEntry, agentName string) (agentModelEntry, bool) {
	for _, entry := range entries {
		if entry.Agent == agentName {
			return entry, true
		}
	}
	return agentModelEntry{}, false
}

// modelSpecFor returns the model pin for the named agent, or a zero spec
// (unpinned) when the roster has no entry for it.
func modelSpecFor(entries []agentModelEntry, agentName string) agents.ModelSpec {
	entry, ok := findAgentModelEntry(entries, agentName)
	if !ok {
		return agents.ModelSpec{}
	}
	return entry.modelSpec()
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
		Map: mapConfig{
			MaxFileBytes: 500000,
			CodeIndex:    codeindex.ModeAuto,
		},
		Gate: gateConfig{
			ScopeEnforcement: gateScopeEnforcementBlock,
		},
		Execute: executeConfig{
			Models: []agentModelEntry{},
		},
		Review: reviewConfig{
			MaxRounds:   10,
			Patience:    2,
			CleanRounds: 1,
			Budget: reviewBudgetConfig{
				Mode:      budgetModeBlock,
				MaxTokens: nil,
			},
			Panel: []agentModelEntry{},
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

func readConfig(path string) (configFile, error) {
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
	if err := validateAgentModelEntries("execute.models", config.Execute.Models); err != nil {
		return configFile{}, err
	}
	if err := validateAgentModelEntries("review.panel", config.Review.Panel); err != nil {
		return configFile{}, err
	}
	return config, nil
}

// validateAgentModelEntries enforces the shared roster-entry rules for
// execute.models and review.panel: every entry names a resolvable built-in
// agent, an agent appears at most once per roster, and model pins carry no
// ':' effort suffix (effort is its own key).
func validateAgentModelEntries(section string, entries []agentModelEntry) error {
	known := map[string]bool{}
	for _, descriptor := range agents.ListBuiltins() {
		known[descriptor.Name] = true
	}
	seen := map[string]bool{}
	for i := range entries {
		name := strings.TrimSpace(entries[i].Agent)
		if name == "" {
			return fmt.Errorf("config %s: entry is missing the agent name", section)
		}
		if !known[name] {
			return fmt.Errorf("config %s: unknown agent %q", section, entries[i].Agent)
		}
		if seen[name] {
			return fmt.Errorf("config %s: duplicate agent %q", section, name)
		}
		seen[name] = true
		entries[i].Agent = name
		if strings.Contains(entries[i].Model, ":") {
			return fmt.Errorf("config %s: model %q must not contain ':'; set the effort key instead", section, entries[i].Model)
		}
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
