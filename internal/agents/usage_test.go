package agents

import (
	"strings"
	"testing"
)

func TestAgentRunCompleted(t *testing.T) {
	codex := AgentDescriptor{Name: BuiltinCodex}
	tests := []struct {
		name   string
		agent  AgentDescriptor
		stdout string
		want   bool
	}{
		{"codex terminal turn.completed", codex, "{\"type\":\"turn.started\"}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1}}\n", true},
		{"codex turn.completed without usage", codex, `{"type":"turn.completed"}`, true},
		{"codex no terminal event", codex, "{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\"}\n", false},
		{"codex completed turn followed by killed work", codex, "{\"type\":\"turn.completed\"}\n{\"type\":\"turn.started\"}\n", false},
		{"codex empty output", codex, "", false},
		// Claude runs over ACP; its completion signal is the recorded prompt
		// response, not a CLI stdout marker. The CLI path always returns false.
		{"claude ACP stdout is not a completion signal", AgentDescriptor{Name: BuiltinClaude}, `{"type":"result","subtype":"success","is_error":false}`, false},
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
