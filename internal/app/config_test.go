package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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
		"    model: claude-opus-4-8",
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

func TestReadConfigParsesTimeoutsSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := strings.Join([]string{
		"schema: pactum.config.v1",
		"agents:",
		"  - name: claude",
		"    model: claude-opus-4-8",
		"timeouts:",
		"  idle: 15m",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, err := readConfig(path)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if config.Timeouts.Idle != "15m" {
		t.Fatalf("timeouts.idle = %q, want 15m", config.Timeouts.Idle)
	}
}

func TestReadConfigRejectsInvalidIdleTimeout(t *testing.T) {
	cases := []struct {
		name string
		idle string
	}{
		{name: "garbage value", idle: "soon"},
		{name: "zero duration", idle: "0s"},
		{name: "negative duration", idle: "-5m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			contents := strings.Join([]string{
				"schema: pactum.config.v1",
				"agents:",
				"  - name: claude",
				"    model: claude-opus-4-8",
				"timeouts:",
				"  idle: " + strconv.Quote(tc.idle),
			}, "\n")
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := readConfig(path)
			if err == nil {
				t.Fatalf("readConfig should reject timeouts.idle %q", tc.idle)
			}
			if !strings.Contains(err.Error(), "timeouts.idle") || !strings.Contains(err.Error(), tc.idle) {
				t.Fatalf("error should name timeouts.idle and the value %q: %v", tc.idle, err)
			}
		})
	}
}

func TestResolveIdleTimeout(t *testing.T) {
	writeConfig := func(t *testing.T, lines ...string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.yaml")
		contents := strings.Join(append([]string{
			"schema: pactum.config.v1",
			"agents:",
			"  - name: claude",
			"    model: claude-opus-4-8",
		}, lines...), "\n")
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return path
	}
	withIdle := writeConfig(t, "timeouts:", "  idle: 15m")
	withoutIdle := writeConfig(t)

	cases := []struct {
		name       string
		configPath string
		override   time.Duration
		want       time.Duration
		wantErr    string
	}{
		{name: "explicit flag wins over config", configPath: withIdle, override: 90 * time.Second, want: 90 * time.Second},
		{name: "config beats built-in", configPath: withIdle, override: 0, want: 15 * time.Minute},
		{name: "built-in when both unset", configPath: withoutIdle, override: 0, want: 25 * time.Minute},
		{name: "negative flag is rejected", configPath: withIdle, override: -time.Second, wantErr: "timeout must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveIdleTimeout(tc.configPath, tc.override)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveIdleTimeout: %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolveIdleTimeout = %s, want %s", got, tc.want)
			}
		})
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
				{Name: "claude", Model: "claude-opus-4-8"},
				{Name: "fable", Model: "claude-fable-5"},
				{Name: "codex", Model: "gpt-5"},
				{Name: "deep", Model: "o3"},
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
			entries: []agentRegistryEntry{{Name: "claude", Model: "claude-opus-4-8"}, {Name: "claude", Model: "claude-fable-5"}},
			wantErr: `config agents: duplicate name "claude"`,
		},
		{
			name:    "missing model is rejected naming the entry",
			entries: []agentRegistryEntry{{Name: "writer"}},
			wantErr: `config agents: entry "writer": model is required (the engine is inferred from the model)`,
		},
		{
			name:    "unknown model form is rejected naming the entry and the recognized forms",
			entries: []agentRegistryEntry{{Name: "writer", Model: "gemini-2.5-pro"}},
			wantErr: `config agents: entry "writer": cannot infer the engine from model "gemini-2.5-pro" (recognized: claude* or opus/sonnet/haiku/fable run on claude; gpt*, codex*, or o<digit>* run on codex)`,
		},
		{
			name:    "colon in model points at the effort key",
			entries: []agentRegistryEntry{{Name: "claude", Model: "claude-fable-5:high"}},
			wantErr: `config agents: entry "claude": model "claude-fable-5:high" must not contain ':'; set the effort key instead`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAgentRegistry(tc.entries)
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

func TestValidateAgentRegistryNormalizesNames(t *testing.T) {
	entries := []agentRegistryEntry{{Name: " claude ", Model: "claude-opus-4-8"}}
	if err := validateAgentRegistry(entries); err != nil {
		t.Fatalf("validateAgentRegistry: %v", err)
	}
	if entries[0].Name != "claude" {
		t.Fatalf("entry should normalize the name in place: %#v", entries[0])
	}
}

func TestValidateReviewPanel(t *testing.T) {
	registry := []agentRegistryEntry{
		{Name: "claude", Model: "claude-opus-4-8"},
		{Name: "fable", Model: "claude-fable-5"},
		{Name: "codex", Model: "gpt-5"},
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
	if len(config.Agents) != 1 || config.Agents[0].Name != "claude" {
		t.Fatalf("default config should register the single claude entry: %#v", config.Agents)
	}
	// The model must be pinned (the engine is inferred from it) and infer the
	// claude engine, so the generated default round-trips inference.
	if config.Agents[0].Model != "claude-opus-4-8" || config.Agents[0].Effort != "" {
		t.Fatalf("default claude entry should pin claude-opus-4-8 with no effort: %#v", config.Agents[0])
	}
	if engine, ok := agents.InferAgentFromModel(config.Agents[0].Model); !ok || engine != agents.BuiltinClaude {
		t.Fatalf("default entry model should infer the claude engine: %q", config.Agents[0].Model)
	}
	if len(config.Review.Panel) != 0 {
		t.Fatalf("default review panel should be empty: %#v", config.Review.Panel)
	}
	// The generated default carries only deviations from the built-ins, so it
	// must not emit a timeouts key; the absent section resolves to the
	// built-in idle window.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if strings.Contains(string(raw), "timeouts") {
		t.Fatalf("generated default config should not emit a timeouts key:\n%s", raw)
	}
	if idle, err := resolveIdleTimeout(path, 0); err != nil || idle != 25*time.Minute {
		t.Fatalf("absent timeouts section should resolve the built-in: %s, %v, want 25m", idle, err)
	}
}

func TestAgentRegistryEntryModelSpecTrims(t *testing.T) {
	spec := agentRegistryEntry{Name: "claude", Model: " claude-fable-5 ", Effort: " high "}.modelSpec()
	if spec.Model != "claude-fable-5" || spec.Effort != "high" {
		t.Fatalf("modelSpec should trim values: %#v", spec)
	}
}

// TestResolveAgentForRoleUsesInferredEngine pins that a free-named entry
// resolves the descriptors of the engine inferred from its model — the name
// itself carries no engine.
func TestResolveAgentForRoleUsesInferredEngine(t *testing.T) {
	reviewer, err := App{}.resolveAgentForRole(agentRegistryEntry{Name: "deep", Model: "o3"}, agentRoleReviewer)
	if err != nil {
		t.Fatalf("resolveAgentForRole reviewer: %v", err)
	}
	if reviewer.Name != "deep" || reviewer.Agent.Name != agents.BuiltinCodex || reviewer.Agent.Command != "codex" {
		t.Fatalf("o3 entry should resolve the codex reviewer descriptor: %#v", reviewer)
	}
	if !containsString(reviewer.Agent.Args, "read-only") {
		t.Fatalf("reviewer role should resolve the read-only codex descriptor: %#v", reviewer.Agent.Args)
	}

	executor, err := App{}.resolveAgentForRole(agentRegistryEntry{Name: "writer", Model: "claude-fable-5"}, agentRoleExecutor)
	if err != nil {
		t.Fatalf("resolveAgentForRole executor: %v", err)
	}
	// Claude runs over ACP: no CLI command or args; model/effort are in ModelSpec.
	if executor.Name != "writer" || executor.Agent.Name != agents.BuiltinClaude || executor.Agent.Command != "" {
		t.Fatalf("claude-model entry should resolve the claude executor descriptor: %#v", executor)
	}
	if len(executor.Agent.Args) != 0 {
		t.Fatalf("claude executor must carry no CLI args: %#v", executor.Agent.Args)
	}
	if executor.ModelSpec.Model != "claude-fable-5" {
		t.Fatalf("claude model pin should be preserved in ModelSpec: %#v", executor.ModelSpec)
	}
}

// TestResolveAgentForRoleFailsOnUninferableModel pins the resolution-layer
// guard: an entry whose model infers no engine errors out instead of falling
// back to the agents-package default resolver.
func TestResolveAgentForRoleFailsOnUninferableModel(t *testing.T) {
	_, err := App{}.resolveAgentForRole(agentRegistryEntry{Name: "mystery", Model: "gemini-2.5-pro"}, agentRoleExecutor)
	if err == nil || !strings.Contains(err.Error(), `agent "mystery"`) || !strings.Contains(err.Error(), "gemini-2.5-pro") {
		t.Fatalf("uninferable model should fail naming the entry: %v", err)
	}
}

// TestFindRegistryEntryRejectsUnregisteredBuiltIn pins the source-of-truth
// rule: a bare built-in name that is not registered does not resolve — the
// registry is the only namespace, and built-ins are not implicitly available.
func TestFindRegistryEntryRejectsUnregisteredBuiltIn(t *testing.T) {
	config := configFile{Agents: []agentRegistryEntry{{Name: "claude", Model: "claude-opus-4-8"}}}
	_, err := findRegistryEntry(config, "codex")
	if err == nil || !strings.Contains(err.Error(), `unknown agent "codex": not registered in config agents`) {
		t.Fatalf("unregistered built-in must not resolve: %v", err)
	}
}

// TestReadConfigRejectsPreRegistryShapes pins that the strict parser rejects
// the removed config shapes loudly: the old execute section, the old
// review.panel object entries, and the removed per-entry agent key all fail
// naming the offending key.
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
				"    model: claude-opus-4-8",
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
				"    model: claude-opus-4-8",
				"review:",
				"  panel:",
				"    - agent: claude",
				"      model: claude-fable-5",
			}, "\n"),
			wantErr: "panel",
		},
		{
			name: "old registry entries with the removed agent key",
			contents: strings.Join([]string{
				"schema: pactum.config.v1",
				"agents:",
				"  - name: fable",
				"    agent: claude",
				"    model: claude-fable-5",
			}, "\n"),
			wantErr: "field agent not found",
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

// TestValidateAgentRegistryRejectsPathUnsafeNames pins that registry names are
// path-safe: they flow into per-member review-lens prompt artifact paths.
func TestValidateAgentRegistryRejectsPathUnsafeNames(t *testing.T) {
	for _, name := range []string{"a/b", `a\b`, "a:b", "a b"} {
		err := validateAgentRegistry([]agentRegistryEntry{{Name: name, Model: "claude-opus-4-8"}})
		if err == nil || !strings.Contains(err.Error(), "must contain only letters") {
			t.Fatalf("name %q should be rejected as path-unsafe: %v", name, err)
		}
	}
	if err := validateAgentRegistry([]agentRegistryEntry{{Name: "fable-5.x_ok", Model: "claude-fable-5"}}); err != nil {
		t.Fatalf("path-safe name should pass: %v", err)
	}
}
