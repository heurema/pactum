package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestValidateAgentModelEntries(t *testing.T) {
	cases := []struct {
		name    string
		entries []agentModelEntry
		wantErr string
	}{
		{
			name:    "valid entries pass",
			entries: []agentModelEntry{{Agent: "claude", Model: "claude-fable-5"}, {Agent: "codex"}},
		},
		{
			name:    "unknown agent is rejected",
			entries: []agentModelEntry{{Agent: "gpt6"}},
			wantErr: `config review.panel: unknown agent "gpt6"`,
		},
		{
			name:    "duplicate agent is rejected",
			entries: []agentModelEntry{{Agent: "claude"}, {Agent: "claude", Model: "other"}},
			wantErr: `config review.panel: duplicate agent "claude"`,
		},
		{
			name:    "blank agent is rejected",
			entries: []agentModelEntry{{Agent: "  "}},
			wantErr: "config review.panel: entry is missing the agent name",
		},
		{
			name:    "colon in model points at the effort key",
			entries: []agentModelEntry{{Agent: "claude", Model: "claude-fable-5:high"}},
			wantErr: `config review.panel: model "claude-fable-5:high" must not contain ':'; set the effort key instead`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAgentModelEntries("review.panel", tc.entries)
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

func TestAgentModelEntryModelSpecTrims(t *testing.T) {
	spec := agentModelEntry{Agent: "claude", Model: " claude-fable-5 ", Effort: " high "}.modelSpec()
	if spec.Model != "claude-fable-5" || spec.Effort != "high" {
		t.Fatalf("modelSpec should trim values: %#v", spec)
	}
}
