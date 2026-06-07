package agents

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func parseAgentUsage(agent AgentDescriptor, stdout []byte, stderr []byte) TokenUsage {
	switch agent.Name {
	case BuiltinCodex:
		if !hasArg(agent.Args, "--json") {
			return TokenUsage{}
		}
		return parseCodexUsage(stdout)
	case BuiltinClaude:
		if !hasOutputFormatJSON(agent.Args) {
			return TokenUsage{}
		}
		return parseClaudeUsage(stdout)
	default:
		return TokenUsage{}
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func hasOutputFormatJSON(args []string) bool {
	for i, arg := range args {
		if arg == "--output-format=json" {
			return true
		}
		if arg == "--output-format" && i+1 < len(args) && args[i+1] == "json" {
			return true
		}
	}
	return false
}

func structuredUsageEnabled(agent AgentDescriptor) bool {
	switch agent.Name {
	case BuiltinCodex:
		return hasArg(agent.Args, "--json")
	case BuiltinClaude:
		return hasOutputFormatJSON(agent.Args)
	default:
		return false
	}
}

func parseCodexUsage(output []byte) TokenUsage {
	type eventUsage struct {
		InputTokens           int64 `json:"input_tokens"`
		CachedInputTokens     int64 `json:"cached_input_tokens"`
		OutputTokens          int64 `json:"output_tokens"`
		ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	}
	type eventEnvelope struct {
		Type  string          `json:"type"`
		Usage json.RawMessage `json:"usage"`
		Turn  struct {
			Usage json.RawMessage `json:"usage"`
		} `json:"turn"`
	}

	var lastRaw json.RawMessage
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope eventEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.Type != "turn.completed" {
			continue
		}
		raw := envelope.Usage
		if len(raw) == 0 {
			raw = envelope.Turn.Usage
		}
		if isEmptyRaw(raw) {
			continue
		}
		lastRaw = append(json.RawMessage(nil), raw...)
	}
	if err := scanner.Err(); err != nil {
		return TokenUsage{CaptureWarning: fmt.Sprintf("codex usage parse failed: %v", err)}
	}
	if len(lastRaw) == 0 {
		return TokenUsage{CaptureWarning: "codex usage parse failed: no turn.completed usage event found"}
	}

	var usage eventUsage
	if err := json.Unmarshal(lastRaw, &usage); err != nil {
		return TokenUsage{CaptureWarning: fmt.Sprintf("codex usage parse failed: %v", err)}
	}
	outputTokens := usage.OutputTokens + usage.ReasoningOutputTokens
	return TokenUsage{
		InputTokens:     usage.InputTokens,
		OutputTokens:    outputTokens,
		TotalTokens:     usage.InputTokens + outputTokens,
		CacheReadTokens: usage.CachedInputTokens,
		ReasoningTokens: usage.ReasoningOutputTokens,
		Captured:        true,
		Raw:             lastRaw,
	}
}

func parseClaudeUsage(output []byte) TokenUsage {
	type resultEnvelope struct {
		Usage json.RawMessage `json:"usage"`
	}
	type resultUsage struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		ReasoningOutputTokens    int64 `json:"reasoning_output_tokens"`
		ThinkingOutputTokens     int64 `json:"thinking_output_tokens"`
		ThinkingTokens           int64 `json:"thinking_tokens"`
	}

	var envelope resultEnvelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		return TokenUsage{CaptureWarning: fmt.Sprintf("claude usage parse failed: %v", err)}
	}
	if isEmptyRaw(envelope.Usage) {
		return TokenUsage{CaptureWarning: "claude usage parse failed: no usage object found"}
	}
	raw := append(json.RawMessage(nil), envelope.Usage...)
	var usage resultUsage
	if err := json.Unmarshal(raw, &usage); err != nil {
		return TokenUsage{CaptureWarning: fmt.Sprintf("claude usage parse failed: %v", err)}
	}
	reasoningTokens := firstNonZero(usage.ReasoningOutputTokens, usage.ThinkingOutputTokens, usage.ThinkingTokens)
	inputTokens := usage.InputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens
	return TokenUsage{
		InputTokens:         inputTokens,
		OutputTokens:        usage.OutputTokens,
		TotalTokens:         inputTokens + usage.OutputTokens,
		CacheReadTokens:     usage.CacheReadInputTokens,
		CacheCreationTokens: usage.CacheCreationInputTokens,
		ReasoningTokens:     reasoningTokens,
		Captured:            true,
		Raw:                 raw,
	}
}

func isEmptyRaw(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
