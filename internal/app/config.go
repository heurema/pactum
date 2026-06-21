package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
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
	Version      string               `yaml:"version"`
	Agents       []agentRegistryEntry `yaml:"agents"`
	Map          mapConfig            `yaml:"map"`
	OutOfScope   string               `yaml:"out_of_scope"`
	Pipeline     pipelineConfig       `yaml:"pipeline"`
	WallClockCap yamlDuration         `yaml:"wall_clock_cap,omitempty"`
	// Warnings collects deprecation messages populated at load time. Not
	// serialized; callers should print these to stderr.
	Warnings []string `yaml:"-"`
}

// yamlDuration is a time.Duration that decodes from a Go duration string (e.g.
// "2h", "25m") in YAML and encodes back to one. yaml.v3 does not auto-handle
// duration strings so the custom unmarshaler is required.
type yamlDuration time.Duration

func (d *yamlDuration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = yamlDuration(dur)
	return nil
}

func (d yamlDuration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

func (d yamlDuration) Duration() time.Duration {
	return time.Duration(d)
}

type mapConfig struct {
	MaxFileBytes int `yaml:"max_file_bytes"`
	// CodeIndex is accepted but ignored for back-compat with existing configs.
	// It has no effect: tree-sitter extraction was removed. Remove this field
	// from your config file when convenient.
	CodeIndex string `yaml:"code_index,omitempty"`
}

// pipelineConfig holds the closed set of pipeline stages. Absent stages decode
// as zero pipelineStage{}, which is valid — empty by uses the stage's default
// role assignment, and absent loop uses the in-code limit defaults.
type pipelineConfig struct {
	Clarify        pipelineStage `yaml:"clarify,omitempty"`
	ContractDraft  pipelineStage `yaml:"contract_draft,omitempty"`
	ContractReview pipelineStage `yaml:"contract_review,omitempty"`
	Execute        pipelineStage `yaml:"execute,omitempty"`
	CodeReview     pipelineStage `yaml:"code_review,omitempty"`
	Memory         pipelineStage `yaml:"memory,omitempty"`
}

// pipelineStage is the performer + optional loop knobs for one pipeline stage.
type pipelineStage struct {
	// By names the agent(s) that perform the stage; normalized to []string on
	// load. Empty/absent means the stage uses its default role assignment.
	By   stageBy     `yaml:"by,omitempty"`
	Loop *loopConfig `yaml:"loop,omitempty"`
}

// stageBy accepts a bare agent name (scalar) or a list of names, normalizing
// both to []string. Empty string and null decode as nil.
type stageBy []string

func (b *stageBy) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Tag == "!!null" || strings.TrimSpace(value.Value) == "" {
			*b = nil
			return nil
		}
		*b = stageBy{strings.TrimSpace(value.Value)}
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		*b = stageBy(items)
		return nil
	default:
		return fmt.Errorf("pipeline by: must be a string or a list")
	}
}

// loopConfig carries the convergence knobs for loop-capable stages: max rounds,
// stalemate patience, and settle (consecutive clean rounds required to stop).
type loopConfig struct {
	Max      int `yaml:"max"`
	Patience int `yaml:"patience"`
	Settle   int `yaml:"settle"`
}

// agentRegistryEntry registers one invocable agent in the top-level agents
// registry, the config's source of truth for agents. Name is how the entry is
// referenced everywhere (pipeline.*.by, --agent, --reviewer); Model is required
// because the underlying engine is inferred solely from it (see
// agents.InferAgentFromModel); Effort is an optional pin. Name and engine are
// decoupled, so two entries may run the same engine with different pins.
type agentRegistryEntry struct {
	Name   string `yaml:"name"`
	Model  string `yaml:"model"`
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
		Version: "v1alpha1",
		Agents: []agentRegistryEntry{
			// The model must be pinned: the engine is inferred from it, so an
			// inherit-the-CLI-default entry cannot exist.
			{Name: agents.BuiltinClaude, Model: "claude-opus-4-8"},
		},
		Map: mapConfig{
			MaxFileBytes: 500000,
		},
		OutOfScope: gateScopeEnforcementBlock,
		Pipeline: pipelineConfig{
			Clarify: pipelineStage{
				Loop: &loopConfig{Max: 3},
			},
			ContractReview: pipelineStage{
				Loop: &loopConfig{Max: 10, Patience: 2, Settle: 1},
			},
			CodeReview: pipelineStage{
				Loop: &loopConfig{Max: 10, Patience: 2, Settle: 1},
			},
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
	if config.Version != "v1alpha1" {
		return configFile{}, fmt.Errorf("config version must be %q, got %q", "v1alpha1", config.Version)
	}
	outOfScope, err := normalizeOutOfScope(config.OutOfScope)
	if err != nil {
		return configFile{}, err
	}
	config.OutOfScope = outOfScope
	if err := validateAgentRegistry(config.Agents); err != nil {
		return configFile{}, err
	}
	if err := validatePipeline(config.Pipeline, config.Agents); err != nil {
		return configFile{}, err
	}
	normalizePipelineBy(&config.Pipeline)
	if config.WallClockCap.Duration() < 0 {
		return configFile{}, errors.New("config wall_clock_cap: must be positive")
	}
	if config.Map.CodeIndex != "" {
		config.Warnings = append(config.Warnings,
			"config map.code_index is deprecated and has no effect; remove it from your config (tree-sitter extraction was removed)")
	}
	return config, nil
}

func normalizeOutOfScope(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return gateScopeEnforcementBlock, nil
	}
	switch value {
	case gateScopeEnforcementBlock, gateScopeEnforcementWarn:
		return value, nil
	default:
		return "", fmt.Errorf("config out_of_scope must be %q or %q, got %q", gateScopeEnforcementBlock, gateScopeEnforcementWarn, value)
	}
}

var (
	// stageLoopAllowed is the closed set of stages that may carry loop knobs.
	stageLoopAllowed = map[string]bool{
		"clarify":         true,
		"contract_review": true,
		"code_review":     true,
	}
	// stageMultiAgentAllowed is the closed set of stages that may name more than
	// one agent in by (the panel stages).
	stageMultiAgentAllowed = map[string]bool{
		"contract_review": true,
		"code_review":     true,
	}
)

func validatePipeline(pipeline pipelineConfig, registry []agentRegistryEntry) error {
	type namedStage struct {
		name  string
		stage pipelineStage
	}
	stages := []namedStage{
		{"clarify", pipeline.Clarify},
		{"contract_draft", pipeline.ContractDraft},
		{"contract_review", pipeline.ContractReview},
		{"execute", pipeline.Execute},
		{"code_review", pipeline.CodeReview},
		{"memory", pipeline.Memory},
	}
	registered := make(map[string]bool, len(registry))
	for _, entry := range registry {
		registered[entry.Name] = true
	}
	for _, ns := range stages {
		if ns.stage.Loop != nil && !stageLoopAllowed[ns.name] {
			return fmt.Errorf("config pipeline.%s: loop is not valid for this stage", ns.name)
		}
		// Count and validate non-empty by entries only; empty strings are treated
		// as absent and normalized out after this validation step.
		var nonEmpty []string
		for _, name := range ns.stage.By {
			if name = strings.TrimSpace(name); name != "" {
				nonEmpty = append(nonEmpty, name)
			}
		}
		if len(nonEmpty) > 1 && !stageMultiAgentAllowed[ns.name] {
			return fmt.Errorf("config pipeline.%s.by: multi-agent list requires a panel stage (contract_review or code_review), got %d agents", ns.name, len(nonEmpty))
		}
		for i, name := range nonEmpty {
			if !registered[name] {
				return fmt.Errorf("config pipeline.%s.by[%d]: unknown agent %q (not registered in config agents)", ns.name, i, name)
			}
		}
	}
	return nil
}

// normalizePipelineBy strips empty and whitespace-only entries from all stage
// by lists so consumers can rely on len(by) == 0 meaning "absent" and all
// entries being valid non-empty agent names.
func normalizePipelineBy(p *pipelineConfig) {
	p.Clarify.By = filterEmptyBy(p.Clarify.By)
	p.ContractDraft.By = filterEmptyBy(p.ContractDraft.By)
	p.ContractReview.By = filterEmptyBy(p.ContractReview.By)
	p.Execute.By = filterEmptyBy(p.Execute.By)
	p.CodeReview.By = filterEmptyBy(p.CodeReview.By)
	p.Memory.By = filterEmptyBy(p.Memory.By)
}

func filterEmptyBy(by stageBy) stageBy {
	var out stageBy
	for _, name := range by {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

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

// validateAgentRegistry enforces the agents-registry rules: at least one entry
// (the registry is the only way to make an agent invocable), non-empty unique
// path-safe names, a required model the engine can be inferred from, and model
// pins carrying no ':' effort suffix (effort is its own key). Names are
// normalized in place so resolution can rely on a trimmed Name; the inferred
// engine is not cached — the resolution layer re-infers it where needed.
func validateAgentRegistry(entries []agentRegistryEntry) error {
	if len(entries) == 0 {
		return errors.New("config agents: at least one agent must be registered")
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
		if strings.TrimSpace(entries[i].Model) == "" {
			return fmt.Errorf("config agents: entry %q: model is required (the engine is inferred from the model)", name)
		}
		if strings.Contains(entries[i].Model, ":") {
			return fmt.Errorf("config agents: entry %q: model %q must not contain ':'; set the effort key instead", name, entries[i].Model)
		}
		if _, ok := agents.InferAgentFromModel(entries[i].Model); !ok {
			return fmt.Errorf("config agents: entry %q: cannot infer the engine from model %q (recognized: claude* or opus/sonnet/haiku/fable run on claude; gpt*, codex*, or o<digit>* run on codex)", name, entries[i].Model)
		}
		entries[i].Name = name
	}
	return nil
}

// defaultIdleTimeout is the built-in idle window for the agent-running
// commands when --timeout is not given.
const defaultIdleTimeout = 25 * time.Minute

// defaultWallClockCap is the built-in absolute ceiling on any single agent
// attempt when wall_clock_cap is absent from config.
const defaultWallClockCap = 2 * time.Hour

// resolveIdleTimeout resolves the idle window for an agent-running command: an
// explicit --timeout wins, otherwise the built-in default. Like the loop
// limits, 0 means the flag was not given; a negative flag value is rejected.
func resolveIdleTimeout(override time.Duration) (time.Duration, error) {
	if override < 0 {
		return 0, errors.New("timeout must be positive")
	}
	if override > 0 {
		return override, nil
	}
	return defaultIdleTimeout, nil
}

// resolveWallClockCap resolves the absolute per-attempt ceiling: a positive
// config value wins, zero (absent) applies the built-in default, and a
// negative value is rejected (already blocked by readConfig validation, but
// callers that pass a duration directly also hit the check here).
func resolveWallClockCap(override time.Duration) (time.Duration, error) {
	if override < 0 {
		return 0, errors.New("config wall_clock_cap: must be positive")
	}
	if override > 0 {
		return override, nil
	}
	return defaultWallClockCap, nil
}
