package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

// agentMessageText returns the agent's message text from json-wrapped output
// (claude --output-format json / codex --json), falling back to the raw stdout
// for plain-text or unrecognized output. The extracted text is only used when it
// is non-blank: an empty/placeholder result or agent_message must not suppress the
// raw fallback, so fenced findings on other lines are never silently dropped.
func agentMessageText(stdout []byte) string {
	if text, ok := claudeResultText(stdout); ok && strings.TrimSpace(text) != "" {
		return text
	}
	if text, ok := codexAgentMessageText(stdout); ok && strings.TrimSpace(text) != "" {
		return text
	}
	return string(stdout)
}

func claudeResultText(stdout []byte) (string, bool) {
	type resultEnvelope struct {
		Result *string `json:"result"`
	}
	var envelope resultEnvelope
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		return "", false
	}
	if envelope.Result == nil {
		return "", false
	}
	return *envelope.Result, true
}

func codexAgentMessageText(stdout []byte) (string, bool) {
	type eventEnvelope struct {
		Type string          `json:"type"`
		Item json.RawMessage `json:"item"`
	}
	type contentPart struct {
		Text string `json:"text"`
	}
	type itemEnvelope struct {
		Type    string        `json:"type"`
		Text    string        `json:"text"`
		Content []contentPart `json:"content"`
	}

	var b strings.Builder
	found := false
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event eventEnvelope
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type != "item.completed" || len(event.Item) == 0 {
			continue
		}
		var item itemEnvelope
		if err := json.Unmarshal(event.Item, &item); err != nil {
			continue
		}
		if item.Type != "agent_message" {
			continue
		}
		found = true
		if item.Text != "" {
			b.WriteString(item.Text)
			continue
		}
		for _, part := range item.Content {
			b.WriteString(part.Text)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false
	}
	return b.String(), found
}
