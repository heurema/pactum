package agents

import "testing"

func TestParseCodexUsageTakesLastCompletedEvent(t *testing.T) {
	output := []byte(`
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":3,"reasoning_output_tokens":1}}
not-json
{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":30,"reasoning_output_tokens":7}}
`)
	usage := parseAgentUsage(AgentDescriptor{
		Name: BuiltinCodex,
		Args: []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox"},
	}, output, nil)

	if !usage.Captured {
		t.Fatalf("usage should be captured: %#v", usage)
	}
	if usage.InputTokens != 100 || usage.OutputTokens != 37 || usage.TotalTokens != 137 {
		t.Fatalf("normalized codex usage mismatch: %#v", usage)
	}
	if usage.CacheReadTokens != 25 || usage.ReasoningTokens != 7 || len(usage.Raw) == 0 {
		t.Fatalf("codex token classes/raw mismatch: %#v", usage)
	}
}

func TestParseClaudeUsageNormalizesCacheAdditiveInput(t *testing.T) {
	output := []byte(`{
  "type": "result",
  "subtype": "success",
  "usage": {
    "input_tokens": 50,
    "output_tokens": 20,
    "cache_creation_input_tokens": 7,
    "cache_read_input_tokens": 13
  }
}`)
	usage := parseAgentUsage(AgentDescriptor{
		Name: BuiltinClaude,
		Args: []string{"-p", "--output-format", "json", "--dangerously-skip-permissions"},
	}, output, nil)

	if !usage.Captured {
		t.Fatalf("usage should be captured: %#v", usage)
	}
	if usage.InputTokens != 70 || usage.OutputTokens != 20 || usage.TotalTokens != 90 {
		t.Fatalf("normalized claude usage mismatch: %#v", usage)
	}
	if usage.CacheReadTokens != 13 || usage.CacheCreationTokens != 7 || len(usage.Raw) == 0 {
		t.Fatalf("claude token classes/raw mismatch: %#v", usage)
	}
}

func TestParseUsageMalformedOrEmptyOutputIsUncaptured(t *testing.T) {
	tests := []struct {
		name   string
		agent  AgentDescriptor
		output []byte
	}{
		{
			name:   "codex empty",
			agent:  AgentDescriptor{Name: BuiltinCodex, Args: []string{"exec", "--json"}},
			output: nil,
		},
		{
			name:   "claude malformed",
			agent:  AgentDescriptor{Name: BuiltinClaude, Args: []string{"-p", "--output-format", "json"}},
			output: []byte(`not json`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := parseAgentUsage(tt.agent, tt.output, nil)
			if usage.Captured {
				t.Fatalf("usage should not be captured: %#v", usage)
			}
			if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.TotalTokens != 0 {
				t.Fatalf("uncaptured usage should not carry counts: %#v", usage)
			}
			if usage.CaptureWarning == "" {
				t.Fatalf("parse miss should carry a warning")
			}
		})
	}
}

func TestParseUsageSkippedWhenStructuredOutputIsNotEnabled(t *testing.T) {
	usage := parseAgentUsage(AgentDescriptor{Name: BuiltinCodex, Args: []string{"exec", "--sandbox", "read-only"}}, []byte(`not json`), nil)
	if usage.Captured || usage.CaptureWarning != "" {
		t.Fatalf("unstructured read-stage usage should stay quietly uncaptured: %#v", usage)
	}
}
