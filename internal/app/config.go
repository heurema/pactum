package app

import (
	"bytes"
	"errors"
	"fmt"
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
	Schema         string             `yaml:"schema"`
	DefaultProfile string             `yaml:"default_profile"`
	ProjectMap     projectMapConfig   `yaml:"project_map"`
	Gate           gateConfig         `yaml:"gate"`
	Agents         agents.AgentConfig `yaml:"agents,omitempty"`
	Limits         limitsConfig       `yaml:"limits"`
	Budget         budgetConfig       `yaml:"budget"`
	Memory         memoryConfig       `yaml:"memory"`
}

type projectMapConfig struct {
	Refresh      string `yaml:"refresh"`
	MaxFileBytes int    `yaml:"max_file_bytes"`
	CodeIndex    string `yaml:"code_index"`
}

type gateConfig struct {
	ScopeEnforcement string `yaml:"scope_enforcement"`
}

type limitsConfig struct {
	Clarify iterationLimits `yaml:"clarify"`
	Execute executeLimits   `yaml:"execute"`
	Review  reviewLimits    `yaml:"review"`
}

type iterationLimits struct {
	MaxIterations        int `yaml:"max_iterations"`
	MaxQuestionsPerRound int `yaml:"max_questions_per_round"`
}

type executeLimits struct {
	MaxIterations int `yaml:"max_iterations"`
}

type reviewLimits struct {
	MaxRounds   int `yaml:"max_rounds"`
	Patience    int `yaml:"patience"`
	CleanRounds int `yaml:"clean_rounds"`
}

type budgetConfig struct {
	Mode      string   `yaml:"mode"`
	MaxTokens *int64   `yaml:"max_tokens"`
	MaxUSD    *float64 `yaml:"max_usd"`
}

type memoryConfig struct {
	Enabled      bool   `yaml:"enabled"`
	IncludeStale string `yaml:"include_stale"`
}

func writeDefaultConfigIfMissing(path string) error {
	if activeStore.Exists(path) {
		return nil
	}
	return writeYAML(path, defaultConfigFile())
}

func defaultConfigFile() configFile {
	return configFile{
		Schema:         configSchema,
		DefaultProfile: "balanced",
		ProjectMap: projectMapConfig{
			Refresh:      "auto",
			MaxFileBytes: 500000,
			CodeIndex:    codeindex.ModeAuto,
		},
		Gate: gateConfig{
			ScopeEnforcement: gateScopeEnforcementBlock,
		},
		Limits: limitsConfig{
			Clarify: iterationLimits{
				MaxIterations:        5,
				MaxQuestionsPerRound: 5,
			},
			Execute: executeLimits{
				MaxIterations: 10,
			},
			Review: reviewLimits{
				MaxRounds:   4,
				Patience:    2,
				CleanRounds: 1,
			},
		},
		Budget: budgetConfig{
			Mode:      budgetModeBlock,
			MaxTokens: nil,
			MaxUSD:    nil,
		},
		Memory: memoryConfig{
			Enabled:      true,
			IncludeStale: "warn",
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
	if err := yaml.Unmarshal(data, &config); err != nil {
		return configFile{}, err
	}
	if config.Schema == "" {
		return configFile{}, errors.New("config is incomplete")
	}
	scopeEnforcement, err := normalizeGateScopeEnforcement(config.Gate.ScopeEnforcement)
	if err != nil {
		return configFile{}, err
	}
	config.Gate.ScopeEnforcement = scopeEnforcement
	budgetMode, err := normalizeBudgetMode(config.Budget.Mode)
	if err != nil {
		return configFile{}, err
	}
	config.Budget.Mode = budgetMode
	return config, nil
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
		return "", fmt.Errorf("config budget.mode must be %q or %q, got %q", budgetModeBlock, budgetModeWarn, value)
	}
}
