package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentMessageTextExtractsCodexAgentMessages(t *testing.T) {
	output := strings.Join([]string{
		`{"type":"session.started"}`,
		codexMessageLineForTest(t, "first\n"),
		`not json`,
		codexMessageContentLineForTest(t, "second"),
		`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":2}}`,
	}, "\n")

	if got := agentMessageText([]byte(output)); got != "first\n\nsecond" {
		t.Fatalf("codex message text = %q", got)
	}
}

func TestAgentMessageTextSeparatesGluedFenceFromProgressMessage(t *testing.T) {
	// Codex emits progress narration as one agent_message (no trailing newline)
	// immediately followed by a final agent_message whose text starts with a
	// fenced ```json block. Without a separator the fence is glued onto the
	// progress text ("...narration```json"), so extractFencedJSONBlocks never
	// sees the fence at the start of a line and recovers zero blocks.
	output := strings.Join([]string{
		`{"type":"session.started"}`,
		codexMessageLineForTest(t, "Analyzing the contract"),
		codexMessageLineForTest(t, "```json\n{\"ok\":true}\n```"),
		`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":2}}`,
	}, "\n")

	blocks := extractFencedJSONBlocks(agentMessageText([]byte(output)))
	if len(blocks) != 1 {
		t.Fatalf("extractFencedJSONBlocks recovered %d blocks, want 1: %#v", len(blocks), blocks)
	}
	if got := strings.TrimSpace(blocks[0]); got != `{"ok":true}` {
		t.Fatalf("recovered JSON block = %q", got)
	}
}

func TestAgentMessageTextExtractsClaudeResult(t *testing.T) {
	output := claudeResultOutputForTest(t, "claude text")
	if got := agentMessageText([]byte(output)); got != "claude text" {
		t.Fatalf("claude message text = %q", got)
	}
}

func TestAgentMessageTextFallsBackToRawOutput(t *testing.T) {
	for _, raw := range []string{
		"plain text\n```json\n{}\n```\n",
		"{malformed",
		"",
	} {
		t.Run(raw, func(t *testing.T) {
			if got := agentMessageText([]byte(raw)); got != raw {
				t.Fatalf("fallback text = %q, want %q", got, raw)
			}
		})
	}
}

func TestReadStageParsersExtractFromJSONWrappedAgentOutput(t *testing.T) {
	reviewerBlocks, reviewerWarnings := parseReviewerFindingBlocks(codexJSONLOutputForTest(t, reviewerStructuredOutput([]map[string]any{
		{"message": "wrapped reviewer finding", "severity": "medium", "category": "quality"},
	})))
	if len(reviewerWarnings) != 0 || len(reviewerBlocks) != 1 || len(reviewerBlocks[0].Findings) != 1 {
		t.Fatalf("wrapped reviewer parse mismatch: blocks=%#v warnings=%#v", reviewerBlocks, reviewerWarnings)
	}
	var reviewerFinding reviewerFindingProposalInput
	assertNoError(t, json.Unmarshal(reviewerBlocks[0].Findings[0], &reviewerFinding))
	if reviewerFinding.Message != "wrapped reviewer finding" {
		t.Fatalf("wrapped reviewer finding = %#v", reviewerFinding)
	}

	clarifierBlocks, clarifierWarnings := parseClarifierSuggestionBlocks(claudeResultOutputForTest(t, clarifierStructuredOutput([]map[string]any{
		{"text": "Wrapped question?", "blocking": true, "rationale": "Wrapped rationale.", "recommended_answer": "Wrapped recommendation.", "confidence": "low", "depends_on": []int{2}},
	})))
	if len(clarifierWarnings) != 0 || len(clarifierBlocks) != 1 || len(clarifierBlocks[0].Questions) != 1 {
		t.Fatalf("wrapped clarifier parse mismatch: blocks=%#v warnings=%#v", clarifierBlocks, clarifierWarnings)
	}
	var clarifierInput clarifierSuggestionInput
	assertNoError(t, json.Unmarshal(clarifierBlocks[0].Questions[0], &clarifierInput))
	if len(clarifierInput.DependsOn) != 1 || clarifierInput.DependsOn[0] != 2 {
		t.Fatalf("wrapped clarifier depends_on = %#v, want [2]", clarifierInput.DependsOn)
	}

	draftBlocks, draftWarnings := parseContractDraftProposalBlocks(claudeResultOutputForTest(t, contractDrafterStructuredOutput()))
	if len(draftWarnings) != 0 || len(draftBlocks) != 1 || len(draftBlocks[0].InScope) != 1 {
		t.Fatalf("wrapped contract draft parse mismatch: blocks=%#v warnings=%#v", draftBlocks, draftWarnings)
	}
}

func TestAgentMessageTextEmptyExtractionFallsBackToRaw(t *testing.T) {
	// An empty/placeholder result or agent_message must not suppress the raw
	// fallback, so fenced findings on other lines are never silently dropped.
	for _, raw := range []string{
		claudeResultOutputForTest(t, ""), // claude result object with empty result
		codexMessageLineForTest(t, ""),   // codex agent_message with empty text
	} {
		if got := agentMessageText([]byte(raw)); got != raw {
			t.Fatalf("empty extraction should fall back to raw: got %q want %q", got, raw)
		}
	}
}

func codexJSONLOutputForTest(t *testing.T, text string) string {
	t.Helper()
	return codexMessageLineForTest(t, text) + "\n" + `{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":2}}` + "\n"
}

func codexMessageLineForTest(t *testing.T, text string) string {
	t.Helper()
	return marshalLineForTest(t, map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type": "agent_message",
			"text": text,
		},
	})
}

func codexMessageContentLineForTest(t *testing.T, text string) string {
	t.Helper()
	return marshalLineForTest(t, map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type":    "agent_message",
			"content": []map[string]any{{"text": text}},
		},
	})
}

func claudeResultOutputForTest(t *testing.T, result string) string {
	t.Helper()
	return marshalLineForTest(t, map[string]any{
		"type":   "result",
		"result": result,
	})
}

func marshalLineForTest(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	assertNoError(t, err)
	return string(data)
}
