package agents

import (
	"strings"
	"testing"
)

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

func TestAgentRunCompleted(t *testing.T) {
	claude := AgentDescriptor{Name: BuiltinClaude}
	codex := AgentDescriptor{Name: BuiltinCodex}
	tests := []struct {
		name   string
		agent  AgentDescriptor
		stdout string
		want   bool
	}{
		{"claude success envelope", claude, `{"type":"result","subtype":"success","is_error":false,"usage":{"input_tokens":1}}`, true},
		{"claude error envelope", claude, `{"type":"result","subtype":"error_during_execution","is_error":true}`, false},
		{"claude partial envelope", claude, `{"type":"result","is_er`, false},
		{"claude non-result envelope", claude, `{"type":"message","is_error":false}`, false},
		{"claude missing subtype is not success", claude, `{"type":"result","is_error":false}`, false},
		{"claude empty output", claude, "", false},
		{"codex terminal turn.completed", codex, "{\"type\":\"turn.started\"}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1}}\n", true},
		{"codex turn.completed without usage", codex, `{"type":"turn.completed"}`, true},
		{"codex no terminal event", codex, "{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\"}\n", false},
		{"codex completed turn followed by killed work", codex, "{\"type\":\"turn.completed\"}\n{\"type\":\"turn.started\"}\n", false},
		{"codex empty output", codex, "", false},
		{"unknown agent", AgentDescriptor{Name: "custom"}, `{"type":"result","is_error":false}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agentRunCompleted(tt.agent, []byte(tt.stdout)); got != tt.want {
				t.Fatalf("agentRunCompleted = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestFinalizeTimedOutAttemptEmptyPathSkipsDetection pins the ACP guard: over
// ACP the attempt log is free-streamed agent text where CLI terminal markers
// cannot legitimately appear — an agent merely quoting a turn.completed event
// must not convert a stalled, idle-killed turn into success. The ACP transport
// passes an empty stdoutPath, which must skip the captured-output detection
// entirely and rely on the protocol's recorded prompt response alone.
func TestFinalizeTimedOutAttemptEmptyPathSkipsDetection(t *testing.T) {
	var stderr strings.Builder
	exitCode, completed := finalizeTimedOutAttempt(AgentDescriptor{Name: BuiltinCodex}, "", false, &stderr, nil)
	if exitCode != -1 || completed {
		t.Fatalf("empty stdoutPath must keep the timed-out failure: exit=%d completed=%t", exitCode, completed)
	}
	if stderr.Len() != 0 {
		t.Fatalf("no completion notice expected: %q", stderr.String())
	}

	exitCode, completed = finalizeTimedOutAttempt(AgentDescriptor{Name: BuiltinClaude}, "", true, &stderr, nil)
	if exitCode != 0 || !completed {
		t.Fatalf("alreadyCompleted must finalize as completed: exit=%d completed=%t", exitCode, completed)
	}
}
