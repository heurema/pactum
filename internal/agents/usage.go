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

func structuredUsageEnabled(agent AgentDescriptor) bool {
	switch agent.Name {
	case BuiltinCodex:
		return hasArg(agent.Args, "--json")
	default:
		return false
	}
}

// codexTurnCompletedEvents returns the parsed turn.completed events from the
// captured codex --json output, skipping blank and non-JSON lines.
func codexTurnCompletedEvents(output []byte) ([]codexEventEnvelope, error) {
	var events []codexEventEnvelope
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope codexEventEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.Type != "turn.completed" {
			continue
		}
		events = append(events, envelope)
	}
	return events, scanner.Err()
}

// codexLastEventType returns the Type of the last parseable JSON event line in
// the captured codex --json output ("" when none parse).
func codexLastEventType(output []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	last := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope codexEventEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		last = envelope.Type
	}
	return last, scanner.Err()
}

type codexEventEnvelope struct {
	Type  string          `json:"type"`
	Usage json.RawMessage `json:"usage"`
	Turn  struct {
		Usage json.RawMessage `json:"usage"`
	} `json:"turn"`
}

// agentRunCompleted reports whether the captured stdout carries the agent's
// successful terminal marker — the signal that the run finished even though the
// idle watchdog killed the lingering process. Codex emits a terminal
// turn.completed event only when the turn finished. Claude runs over ACP where
// the recorded prompt response (client.turnCompleted()) is the completion
// signal; it never passes a non-empty stdoutPath to finalizeTimedOutAttempt.
// Partial, absent, or error output reports false.
func agentRunCompleted(agent AgentDescriptor, stdout []byte) bool {
	switch agent.Name {
	case BuiltinCodex:
		// The LAST parsed event must be turn.completed: an earlier completed
		// turn followed by further (killed) work is not a completed run.
		last, err := codexLastEventType(stdout)
		return err == nil && last == "turn.completed"
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

	events, err := codexTurnCompletedEvents(output)
	if err != nil {
		return TokenUsage{CaptureWarning: fmt.Sprintf("codex usage parse failed: %v", err)}
	}
	var lastRaw json.RawMessage
	for _, envelope := range events {
		raw := envelope.Usage
		if len(raw) == 0 {
			raw = envelope.Turn.Usage
		}
		if isEmptyRaw(raw) {
			continue
		}
		lastRaw = append(json.RawMessage(nil), raw...)
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

func isEmptyRaw(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}
