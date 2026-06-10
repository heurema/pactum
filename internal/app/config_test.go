package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
)

func TestReadConfigRejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := strings.Join([]string{
		"schema: pactum.config.v1",
		"default_profile: balanced",
		"review:",
		"  max_rounds: 10",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := readConfig(path)
	if err == nil {
		t.Fatal("readConfig should reject an unknown key")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "default_profile") {
		t.Fatalf("error should name the file and the unknown key: %v", err)
	}
}

func TestReadConfigParsesClarifySection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := strings.Join([]string{
		"schema: pactum.config.v1",
		"agents:",
		"  - name: claude",
		"clarify:",
		"  max_rounds: 5",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, err := readConfig(path)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if config.Clarify.MaxRounds != 5 {
		t.Fatalf("clarify.max_rounds = %d, want 5", config.Clarify.MaxRounds)
	}
}

func TestDefaultConfigClarifyMaxRounds(t *testing.T) {
	if got := defaultConfigFile().Clarify.MaxRounds; got != 3 {
		t.Fatalf("default clarify.max_rounds = %d, want 3", got)
	}
}

func TestValidateAgentRegistry(t *testing.T) {
	cases := []struct {
		name    string
		entries []agentRegistryEntry
		wantErr string
	}{
		{
			name: "valid entries pass",
			entries: []agentRegistryEntry{
				{Name: "claude"},
				{Name: "fable", Agent: "claude", Model: "claude-fable-5"},
				{Name: "codex"},
			},
		},
		{
			name:    "empty registry is rejected",
			entries: []agentRegistryEntry{},
			wantErr: "config agents: at least one agent must be registered",
		},
		{
			name:    "blank name is rejected",
			entries: []agentRegistryEntry{{Name: "  "}},
			wantErr: "config agents: entry is missing the name",
		},
		{
			name:    "duplicate name is rejected",
			entries: []agentRegistryEntry{{Name: "claude"}, {Name: "claude", Model: "other"}},
			wantErr: `config agents: duplicate name "claude"`,
		},
		{
			name:    "unknown built-in is rejected",
			entries: []agentRegistryEntry{{Name: "writer", Agent: "gpt6"}},
			wantErr: `config agents: entry "writer": unknown agent "gpt6" (built-ins: codex, claude)`,
		},
		{
			name:    "name defaulting to the underlying agent must be a built-in",
			entries: []agentRegistryEntry{{Name: "gpt6"}},
			wantErr: `config agents: entry "gpt6": unknown agent "gpt6" (built-ins: codex, claude)`,
		},
		{
			name:    "colon in model points at the effort key",
			entries: []agentRegistryEntry{{Name: "claude", Model: "claude-fable-5:high"}},
			wantErr: `config agents: entry "claude": model "claude-fable-5:high" must not contain ':'; set the effort key instead`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAgentRegistry(tc.entries, agents.ListBuiltins())
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateAgentRegistryDefaultsUnderlyingAgentToName(t *testing.T) {
	entries := []agentRegistryEntry{{Name: " claude "}}
	if err := validateAgentRegistry(entries, agents.ListBuiltins()); err != nil {
		t.Fatalf("validateAgentRegistry: %v", err)
	}
	if entries[0].Name != "claude" || entries[0].Agent != "claude" {
		t.Fatalf("entry should normalize name and default the underlying agent: %#v", entries[0])
	}
}

func TestValidateReviewPanel(t *testing.T) {
	registry := []agentRegistryEntry{
		{Name: "claude", Agent: "claude"},
		{Name: "fable", Agent: "claude"},
		{Name: "codex", Agent: "codex"},
	}
	cases := []struct {
		name    string
		panel   []string
		wantErr string
	}{
		{
			name:  "registered names pass, including two names on one built-in",
			panel: []string{"fable", "claude"},
		},
		{
			name:    "unregistered name is rejected",
			panel:   []string{"gpt6"},
			wantErr: `config review.panel: unknown agent "gpt6" (not registered in config agents)`,
		},
		{
			name:    "duplicate name is rejected",
			panel:   []string{"fable", "fable"},
			wantErr: `config review.panel: duplicate name "fable"`,
		},
		{
			name:    "blank name is rejected",
			panel:   []string{"  "},
			wantErr: "config review.panel: entry is missing the agent name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateReviewPanel(tc.panel, registry)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestReadConfigRequiresAgentRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := strings.Join([]string{
		"schema: pactum.config.v1",
		"review:",
		"  max_rounds: 10",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := readConfig(path)
	if err == nil || err.Error() != "config agents: at least one agent must be registered" {
		t.Fatalf("readConfig should require a non-empty registry, got: %v", err)
	}
}

func TestDefaultConfigRoundTripsStrictReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := writeDefaultConfigIfMissing(path); err != nil {
		t.Fatalf("writeDefaultConfigIfMissing: %v", err)
	}

	config, err := readConfig(path)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if len(config.Agents) != 1 || config.Agents[0].Name != "claude" || config.Agents[0].Agent != "claude" {
		t.Fatalf("default config should register the single claude entry: %#v", config.Agents)
	}
	if config.Agents[0].Model != "" || config.Agents[0].Effort != "" {
		t.Fatalf("default claude entry should be unpinned: %#v", config.Agents[0])
	}
	if len(config.Review.Panel) != 0 {
		t.Fatalf("default review panel should be empty: %#v", config.Review.Panel)
	}
}

func TestAgentRegistryEntryModelSpecTrims(t *testing.T) {
	spec := agentRegistryEntry{Name: "claude", Model: " claude-fable-5 ", Effort: " high "}.modelSpec()
	if spec.Model != "claude-fable-5" || spec.Effort != "high" {
		t.Fatalf("modelSpec should trim values: %#v", spec)
	}
}

// TestFindRegistryEntryRejectsUnregisteredBuiltIn pins the source-of-truth
// rule: a bare built-in name that is not registered does not resolve — the
// registry is the only namespace, and built-ins are not implicitly available.
func TestFindRegistryEntryRejectsUnregisteredBuiltIn(t *testing.T) {
	config := configFile{Agents: []agentRegistryEntry{{Name: "claude", Agent: "claude"}}}
	_, err := findRegistryEntry(config, "codex")
	if err == nil || !strings.Contains(err.Error(), `unknown agent "codex": not registered in config agents`) {
		t.Fatalf("unregistered built-in must not resolve: %v", err)
	}
}

// TestReadConfigRejectsPreRegistryShapes pins that the strict parser rejects
// the pre-M18.0 config shapes loudly: the removed execute section and the old
// review.panel object entries both fail naming the offending key.
func TestReadConfigRejectsPreRegistryShapes(t *testing.T) {
	cases := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name: "old execute.models section",
			contents: strings.Join([]string{
				"schema: pactum.config.v1",
				"agents:",
				"  - name: claude",
				"execute:",
				"  models: []",
			}, "\n"),
			wantErr: "execute",
		},
		{
			name: "old panel object entries",
			contents: strings.Join([]string{
				"schema: pactum.config.v1",
				"agents:",
				"  - name: claude",
				"review:",
				"  panel:",
				"    - agent: claude",
				"      model: claude-fable-5",
			}, "\n"),
			wantErr: "panel",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tc.contents), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			_, err := readConfig(path)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("pre-registry shape %q should fail mentioning %q: %v", tc.name, tc.wantErr, err)
			}
		})
	}
}
