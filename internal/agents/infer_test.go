package agents

import (
	"testing"
)

func TestInferAgentFromModel(t *testing.T) {
	cases := []struct {
		model     string
		wantAgent string
		wantOK    bool
	}{
		// claude: the "claude" prefix, any casing, trimmed.
		{model: "claude-fable-5", wantAgent: BuiltinClaude, wantOK: true},
		{model: "claude-opus-4-8", wantAgent: BuiltinClaude, wantOK: true},
		{model: "claude", wantAgent: BuiltinClaude, wantOK: true},
		{model: " Claude-Sonnet-4-6 ", wantAgent: BuiltinClaude, wantOK: true},
		// claude: each alias, exact match, case-insensitive.
		{model: "opus", wantAgent: BuiltinClaude, wantOK: true},
		{model: "sonnet", wantAgent: BuiltinClaude, wantOK: true},
		{model: "haiku", wantAgent: BuiltinClaude, wantOK: true},
		{model: "fable", wantAgent: BuiltinClaude, wantOK: true},
		{model: "Opus", wantAgent: BuiltinClaude, wantOK: true},
		// codex: the "gpt" and "codex" prefixes.
		{model: "gpt-5.5", wantAgent: BuiltinCodex, wantOK: true},
		{model: "GPT-5-codex", wantAgent: BuiltinCodex, wantOK: true},
		{model: "codex-mini-latest", wantAgent: BuiltinCodex, wantOK: true},
		// codex: o<digit> ids.
		{model: "o3", wantAgent: BuiltinCodex, wantOK: true},
		{model: "o4-mini", wantAgent: BuiltinCodex, wantOK: true},
		// Aliases are exact matches: a prefixed alias is not a claude model.
		{model: "opus-4", wantOK: false},
		// Unknown forms fail instead of guessing.
		{model: "", wantOK: false},
		{model: "   ", wantOK: false},
		{model: "gemini-2.5-pro", wantOK: false},
		{model: "o-mini", wantOK: false},
		{model: "llama-4", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			agent, ok := InferAgentFromModel(tc.model)
			if ok != tc.wantOK || agent != tc.wantAgent {
				t.Fatalf("InferAgentFromModel(%q) = (%q, %t), want (%q, %t)", tc.model, agent, ok, tc.wantAgent, tc.wantOK)
			}
		})
	}
}
