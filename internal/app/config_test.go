package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/agents"
)

// minimalValidConfig returns a minimal YAML config string with the given agents
// block. Use it as a base and append extra lines.
func minimalValidConfig(agentsBlock string) string {
	return "version: v1alpha1\n" + agentsBlock
}

func TestReadConfigRejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := strings.Join([]string{
		"version: v1alpha1",
		"default_profile: balanced",
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

func TestReadConfigAcceptsLegacyMapBlock(t *testing.T) {
	// Pre-existing configs with map.max_file_bytes and map.code_index must load
	// without error; the map block is silently ignored (no-op).
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\nmap:\n  max_file_bytes: 500000\n  code_index: auto\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := readConfig(path); err != nil {
		t.Fatalf("readConfig should accept legacy map block: %v", err)
	}
}

func TestReadConfigRejectsLegacyTopLevelKeys(t *testing.T) {
	legacyKeys := []struct {
		name    string
		snippet string
	}{
		{"schema", "schema: pactum.config.v1alpha1"},
		{"gate", "gate:\n  scope_enforcement: block"},
		{"review", "review:\n  max_rounds: 10"},
		{"contract", "contract:\n  reviewers: []"},
		{"clarify", "clarify:\n  max_rounds: 3"},
		{"timeouts", "timeouts:\n  idle: 15m"},
	}
	for _, tc := range legacyKeys {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\n" + tc.snippet + "\n"
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			_, err := readConfig(path)
			if err == nil {
				t.Fatalf("readConfig should reject legacy key %q", tc.name)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("error for legacy key %q should mention the key: %v", tc.name, err)
			}
		})
	}
}

func TestReadConfigRejectsInvalidVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
	}{
		{"empty", ""},
		{"old schema value", "pactum.config.v1alpha1"},
		{"wrong prefix", "config.v1alpha1"},
		{"v2", "v2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			var contents string
			if tc.version == "" {
				contents = "agents:\n  - name: claude\n    model: claude-opus-4-8\n"
			} else {
				contents = "version: " + tc.version + "\nagents:\n  - name: claude\n    model: claude-opus-4-8\n"
			}
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			_, err := readConfig(path)
			if err == nil {
				t.Fatalf("readConfig should reject version %q", tc.version)
			}
			if !strings.Contains(err.Error(), "version") {
				t.Fatalf("error should mention version: %v", err)
			}
		})
	}
}

func TestReadConfigParsesOutOfScope(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"absent defaults to block", "", gateScopeEnforcementBlock},
		{"explicit block", "block", gateScopeEnforcementBlock},
		{"explicit warn", "warn", gateScopeEnforcementWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			var contents string
			if tc.value == "" {
				contents = "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\n"
			} else {
				contents = "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\nout_of_scope: " + tc.value + "\n"
			}
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			config, err := readConfig(path)
			if err != nil {
				t.Fatalf("readConfig: %v", err)
			}
			if config.OutOfScope != tc.want {
				t.Fatalf("out_of_scope = %q, want %q", config.OutOfScope, tc.want)
			}
		})
	}
}

func TestReadConfigRejectsUnknownPipelineStage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  foobar:\n    by: claude\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := readConfig(path)
	if err == nil {
		t.Fatal("readConfig should reject an unknown pipeline stage name")
	}
	if !strings.Contains(err.Error(), "foobar") {
		t.Fatalf("error should mention the unknown stage name: %v", err)
	}
}

func TestReadConfigParsesPipelineByScalar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := strings.Join([]string{
		"version: v1alpha1",
		"agents:",
		"  - name: claude",
		"    model: claude-opus-4-8",
		"pipeline:",
		"  execute:",
		"    by: claude",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	config, err := readConfig(path)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if len(config.Pipeline.Execute.By) != 1 || config.Pipeline.Execute.By[0] != "claude" {
		t.Fatalf("execute.by scalar should normalize to [\"claude\"]: %#v", config.Pipeline.Execute.By)
	}
}

func TestReadConfigParsesPipelineByList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := strings.Join([]string{
		"version: v1alpha1",
		"agents:",
		"  - name: claude",
		"    model: claude-opus-4-8",
		"  - name: fable",
		"    model: claude-fable-5",
		"pipeline:",
		"  code_review:",
		"    by:",
		"      - claude",
		"      - fable",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	config, err := readConfig(path)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if len(config.Pipeline.CodeReview.By) != 2 || config.Pipeline.CodeReview.By[0] != "claude" || config.Pipeline.CodeReview.By[1] != "fable" {
		t.Fatalf("code_review.by list: %#v", config.Pipeline.CodeReview.By)
	}
}

func TestReadConfigPipelineLoopValidation(t *testing.T) {
	loopSnippet := "    loop:\n      max: 3\n"
	// loop is accepted on the loop-capable stages.
	for _, stage := range []string{"clarify", "contract_review", "code_review"} {
		t.Run("accepted on "+stage, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  " + stage + ":\n" + loopSnippet
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			if _, err := readConfig(path); err != nil {
				t.Fatalf("loop on %s should be accepted: %v", stage, err)
			}
		})
	}
	// loop is rejected on the non-loop stages.
	for _, stage := range []string{"contract_draft", "execute", "memory"} {
		t.Run("rejected on "+stage, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  " + stage + ":\n" + loopSnippet
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			_, err := readConfig(path)
			if err == nil {
				t.Fatalf("loop on %s should be a load error", stage)
			}
			if !strings.Contains(err.Error(), stage) || !strings.Contains(err.Error(), "loop") {
				t.Fatalf("error should name the stage and 'loop': %v", err)
			}
		})
	}
}

func TestReadConfigPipelineMultiAgentByValidation(t *testing.T) {
	multiBySnippet := "    by:\n      - claude\n      - fable\n"
	// Multi-agent by is accepted on panel stages.
	for _, stage := range []string{"contract_review", "code_review"} {
		t.Run("accepted on "+stage, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\n  - name: fable\n    model: claude-fable-5\npipeline:\n  " + stage + ":\n" + multiBySnippet
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			if _, err := readConfig(path); err != nil {
				t.Fatalf("multi-agent by on %s should be accepted: %v", stage, err)
			}
		})
	}
	// Multi-agent by is rejected on single-agent stages.
	for _, stage := range []string{"clarify", "contract_draft", "execute", "memory"} {
		t.Run("rejected on "+stage, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\n  - name: fable\n    model: claude-fable-5\npipeline:\n  " + stage + ":\n" + multiBySnippet
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			_, err := readConfig(path)
			if err == nil {
				t.Fatalf("multi-agent by on %s should be a load error", stage)
			}
			if !strings.Contains(err.Error(), stage) {
				t.Fatalf("error should name the stage: %v", err)
			}
		})
	}
}

func TestReadConfigPipelineUnresolvedAgentIsRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := strings.Join([]string{
		"version: v1alpha1",
		"agents:",
		"  - name: claude",
		"    model: claude-opus-4-8",
		"pipeline:",
		"  execute:",
		"    by: ghost-agent",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := readConfig(path)
	if err == nil {
		t.Fatal("unresolved agent in by should be a load error")
	}
	if !strings.Contains(err.Error(), "ghost-agent") || !strings.Contains(err.Error(), "pipeline.execute") {
		t.Fatalf("error should name the stage and the agent: %v", err)
	}
}

func TestReadConfigPipelineEmptyByIsValid(t *testing.T) {
	cases := []struct {
		name     string
		contents string
	}{
		{
			"absent by",
			"version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\n",
		},
		{
			"empty list by",
			"version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  code_review:\n    by: []\n",
		},
		{
			"empty string by",
			"version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  execute:\n    by: \"\"\n",
		},
		{
			"absent contract_review by disables contract review",
			"version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\n",
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
			if err != nil {
				t.Fatalf("empty/absent by should be valid: %v", err)
			}
		})
	}
}

func TestReadConfigPipelineOmittedLoopFallsBackToDefault(t *testing.T) {
	// Omitted loop on a loop-capable stage falls back to the built-in defaults.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	config, err := readConfig(path)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if config.Pipeline.CodeReview.Loop != nil {
		t.Fatalf("omitted code_review loop should decode as nil (fallback at resolve time): %#v", config.Pipeline.CodeReview.Loop)
	}
	defaults := defaultConfigFile().Pipeline.CodeReview.Loop
	limits, err := resolveContractReviewLoopLimits(path)
	if err != nil {
		t.Fatalf("resolveContractReviewLoopLimits with absent loop: %v", err)
	}
	if limits.MaxRounds != defaults.Max || limits.Patience != defaults.Patience || limits.CleanRounds != defaults.Settle {
		t.Fatalf("absent loop should fall back to defaults {max:%d patience:%d settle:%d}, got %+v",
			defaults.Max, defaults.Patience, defaults.Settle, limits)
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
	if config.Version != "v1alpha1" {
		t.Fatalf("default config version = %q, want v1alpha1", config.Version)
	}
	if len(config.Agents) != 1 || config.Agents[0].Name != "claude" {
		t.Fatalf("default config should register the single claude entry: %#v", config.Agents)
	}
	if config.Agents[0].Model != "claude-opus-4-8" || config.Agents[0].Effort != "" {
		t.Fatalf("default claude entry should pin claude-opus-4-8 with no effort: %#v", config.Agents[0])
	}
	if engine, ok := agents.InferAgentFromModel(config.Agents[0].Model); !ok || engine != agents.BuiltinClaude {
		t.Fatalf("default entry model should infer the claude engine: %q", config.Agents[0].Model)
	}
	if config.OutOfScope != gateScopeEnforcementBlock {
		t.Fatalf("default out_of_scope = %q, want block", config.OutOfScope)
	}
	// code_review and contract_review panels are empty by default.
	if len(config.Pipeline.CodeReview.By) != 0 {
		t.Fatalf("default code_review by should be empty: %#v", config.Pipeline.CodeReview.By)
	}
	if len(config.Pipeline.ContractReview.By) != 0 {
		t.Fatalf("default contract_review by should be empty: %#v", config.Pipeline.ContractReview.By)
	}
	// Default config must not emit legacy top-level keys. Check for them as
	// line-starting tokens (top-level YAML keys have no leading whitespace).
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	rawStr := string(raw)
	for _, forbidden := range []string{"\nschema:", "\ngate:", "\nreview:", "\ncontract:", "\nclarify:", "\ntimeouts:"} {
		if strings.Contains(rawStr, forbidden) {
			t.Fatalf("generated default config should not emit legacy key %q:\n%s", strings.TrimPrefix(forbidden, "\n"), rawStr)
		}
	}
	for _, required := range []string{"version: v1alpha1", "agents:", "out_of_scope:", "pipeline:"} {
		if !strings.Contains(rawStr, required) {
			t.Fatalf("generated default config missing %q:\n%s", required, rawStr)
		}
	}
	if strings.Contains(rawStr, "\nmap:") {
		t.Fatalf("generated default config must not contain map: block:\n%s", rawStr)
	}
}

// TestResolverEquivalence pins behaviour-preservation: loading a new-shape
// config equivalent to today's defaults resolves to the same per-stage values
// as the old defaults did.
func TestResolverEquivalence(t *testing.T) {
	defaults := defaultConfigFile()

	// code_review loop limits.
	cr := defaults.Pipeline.CodeReview.Loop
	if cr == nil {
		t.Fatal("default pipeline.code_review.loop must not be nil")
	}
	if cr.Max != 10 {
		t.Fatalf("pipeline.code_review.loop.max = %d, want 10 (was review.max_rounds)", cr.Max)
	}
	if cr.Patience != 2 {
		t.Fatalf("pipeline.code_review.loop.patience = %d, want 2 (was review.patience)", cr.Patience)
	}
	if cr.Settle != 1 {
		t.Fatalf("pipeline.code_review.loop.settle = %d, want 1 (was review.clean_rounds)", cr.Settle)
	}

	// contract_review loop limits must equal code_review limits (today's default).
	ctr := defaults.Pipeline.ContractReview.Loop
	if ctr == nil {
		t.Fatal("default pipeline.contract_review.loop must not be nil")
	}
	if ctr.Max != cr.Max || ctr.Patience != cr.Patience || ctr.Settle != cr.Settle {
		t.Fatalf("contract_review.loop {%d,%d,%d} must equal code_review.loop {%d,%d,%d}",
			ctr.Max, ctr.Patience, ctr.Settle, cr.Max, cr.Patience, cr.Settle)
	}

	// clarify loop max.
	cl := defaults.Pipeline.Clarify.Loop
	if cl == nil {
		t.Fatal("default pipeline.clarify.loop must not be nil")
	}
	if cl.Max != 3 {
		t.Fatalf("pipeline.clarify.loop.max = %d, want 3 (was clarify.max_rounds)", cl.Max)
	}

	// out_of_scope default.
	if defaults.OutOfScope != gateScopeEnforcementBlock {
		t.Fatalf("default out_of_scope = %q, want block (was gate.scope_enforcement)", defaults.OutOfScope)
	}

	// Single-agent stages have empty by by default (uses default role assignment).
	for _, stage := range []struct {
		name string
		by   stageBy
	}{
		{"contract_draft", defaults.Pipeline.ContractDraft.By},
		{"execute", defaults.Pipeline.Execute.By},
		{"memory", defaults.Pipeline.Memory.By},
		{"clarify", defaults.Pipeline.Clarify.By},
	} {
		if len(stage.by) != 0 {
			t.Fatalf("default pipeline.%s.by should be empty (uses default role): %#v", stage.name, stage.by)
		}
	}

	// Panel stages have empty by by default (no reviewers = fallback / disabled).
	for _, stage := range []struct {
		name string
		by   stageBy
	}{
		{"code_review", defaults.Pipeline.CodeReview.By},
		{"contract_review", defaults.Pipeline.ContractReview.By},
	} {
		if len(stage.by) != 0 {
			t.Fatalf("default pipeline.%s.by should be empty: %#v", stage.name, stage.by)
		}
	}

	// Observable equivalence: load the default config from disk and assert the
	// runtime resolvers produce the same per-stage assignments as the old config
	// mapping did (first registry entry for single-agent stages, empty panel for
	// code_review and contract_review).
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := writeDefaultConfigIfMissing(configPath); err != nil {
		t.Fatalf("writeDefaultConfigIfMissing: %v", err)
	}
	config, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	wantAgent := config.Agents[0].Name // "claude" — the single default entry

	// execute stage: resolveExecutorEntry with empty stageBy → first registry entry.
	execEntry, err := resolveExecutorEntry(config, config.Pipeline.Execute.By, "")
	if err != nil {
		t.Fatalf("resolveExecutorEntry(execute): %v", err)
	}
	if execEntry.Name != wantAgent {
		t.Fatalf("execute resolved to %q, want %q", execEntry.Name, wantAgent)
	}

	// clarify stage: resolveReviewerEntry with empty stageBy + empty context
	// (no execution yet) → cross-model selection falls back to first registry entry.
	clarifyEntry, err := resolveReviewerEntry(config, config.Pipeline.Clarify.By, reviewContext{}, "")
	if err != nil {
		t.Fatalf("resolveReviewerEntry(clarify): %v", err)
	}
	if clarifyEntry.Name != wantAgent {
		t.Fatalf("clarify resolved to %q, want %q", clarifyEntry.Name, wantAgent)
	}

	// contract_draft stage: same cross-model fallback.
	draftEntry, err := resolveReviewerEntry(config, config.Pipeline.ContractDraft.By, reviewContext{}, "")
	if err != nil {
		t.Fatalf("resolveReviewerEntry(contract_draft): %v", err)
	}
	if draftEntry.Name != wantAgent {
		t.Fatalf("contract_draft resolved to %q, want %q", draftEntry.Name, wantAgent)
	}

	// code_review loop limits from the loaded config match the expected defaults.
	loadedCR := config.Pipeline.CodeReview.Loop
	if loadedCR == nil {
		t.Fatal("loaded pipeline.code_review.loop must not be nil")
	}
	if loadedCR.Max != cr.Max || loadedCR.Patience != cr.Patience || loadedCR.Settle != cr.Settle {
		t.Fatalf("loaded code_review loop {%d,%d,%d} differs from default {%d,%d,%d}",
			loadedCR.Max, loadedCR.Patience, loadedCR.Settle, cr.Max, cr.Patience, cr.Settle)
	}

	// contract_review loop limits via the real resolver path (resolveContractReviewLoopLimits).
	contractLimits, err := resolveContractReviewLoopLimits(configPath)
	if err != nil {
		t.Fatalf("resolveContractReviewLoopLimits: %v", err)
	}
	if contractLimits.MaxRounds != cr.Max || contractLimits.Patience != cr.Patience || contractLimits.CleanRounds != cr.Settle {
		t.Fatalf("resolved contract_review limits {%d,%d,%d} differ from code_review default {%d,%d,%d}",
			contractLimits.MaxRounds, contractLimits.Patience, contractLimits.CleanRounds,
			cr.Max, cr.Patience, cr.Settle)
	}
}

func TestResolveIdleTimeout(t *testing.T) {
	cases := []struct {
		name     string
		override time.Duration
		want     time.Duration
		wantErr  string
	}{
		{name: "explicit flag used", override: 90 * time.Second, want: 90 * time.Second},
		{name: "built-in default when not given", override: 0, want: 25 * time.Minute},
		{name: "negative flag is rejected", override: -time.Second, wantErr: "timeout must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveIdleTimeout(tc.override)
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

func TestResolveWallClockCap(t *testing.T) {
	cases := []struct {
		name     string
		override time.Duration
		want     time.Duration
		wantErr  string
	}{
		{name: "explicit value used", override: 3 * time.Hour, want: 3 * time.Hour},
		{name: "built-in default when zero", override: 0, want: defaultWallClockCap},
		{name: "negative is rejected", override: -time.Minute, wantErr: "must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveWallClockCap(tc.override)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveWallClockCap: %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolveWallClockCap = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestReadConfigRejectsNegativeWallClockCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\nwall_clock_cap: -1h\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := readConfig(path)
	if err == nil {
		t.Fatal("readConfig should reject a negative wall_clock_cap")
	}
	if !strings.Contains(err.Error(), "wall_clock_cap") {
		t.Fatalf("error should mention wall_clock_cap: %v", err)
	}
}

func TestReadConfigRequiresAgentRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := readConfig(path)
	if err == nil || err.Error() != "config agents: at least one agent must be registered" {
		t.Fatalf("readConfig should require a non-empty registry, got: %v", err)
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

func TestDefaultConfigClarifyLoopMax(t *testing.T) {
	l := defaultConfigFile().Pipeline.Clarify.Loop
	if l == nil || l.Max != 3 {
		t.Fatalf("default pipeline.clarify.loop.max = %v, want 3", l)
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
	// Codex runs over ACP; the built-in reviewer descriptor has no CLI command or args.
	if reviewer.Name != "deep" || reviewer.Agent.Name != agents.BuiltinCodex || reviewer.Agent.Command != "" {
		t.Fatalf("o3 entry should resolve the codex reviewer descriptor: %#v", reviewer)
	}
	if len(reviewer.Agent.Args) != 0 {
		t.Fatalf("reviewer role codex descriptor must have no CLI args: %#v", reviewer.Agent.Args)
	}

	executor, err := App{}.resolveAgentForRole(agentRegistryEntry{Name: "writer", Model: "claude-fable-5"}, agentRoleExecutor)
	if err != nil {
		t.Fatalf("resolveAgentForRole executor: %v", err)
	}
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
// rule: a bare built-in name that is not registered does not resolve.
func TestFindRegistryEntryRejectsUnregisteredBuiltIn(t *testing.T) {
	config := configFile{Agents: []agentRegistryEntry{{Name: "claude", Model: "claude-opus-4-8"}}}
	_, err := findRegistryEntry(config, "codex")
	if err == nil || !strings.Contains(err.Error(), `unknown agent "codex": not registered in config agents`) {
		t.Fatalf("unregistered built-in must not resolve: %v", err)
	}
}

// TestReadConfigRejectsOldShapes pins that old config shapes (pre-registry
// execute section, old panel object entries, removed agent key) are now
// rejected as unknown keys rather than with legacy-specific messages.
func TestReadConfigRejectsOldShapes(t *testing.T) {
	cases := []struct {
		name     string
		contents string
	}{
		{
			name: "old execute.models section",
			contents: strings.Join([]string{
				"version: v1alpha1",
				"agents:",
				"  - name: claude",
				"    model: claude-opus-4-8",
				"execute:",
				"  models: []",
			}, "\n"),
		},
		{
			name: "old registry entries with the removed agent key",
			contents: strings.Join([]string{
				"version: v1alpha1",
				"agents:",
				"  - name: fable",
				"    agent: claude",
				"    model: claude-fable-5",
			}, "\n"),
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
			if err == nil {
				t.Fatalf("old shape %q should be rejected", tc.name)
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
