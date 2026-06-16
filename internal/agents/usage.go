package agents

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

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

func isEmptyRaw(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}
