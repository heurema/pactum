package app

import (
	"strings"
	"testing"
)

// TestResolveReviewerAttemptsForProposalsDefaultsToAllCompleted pins the
// fan-out rule: with no explicit attempt, propose-findings parses EVERY
// completed reviewer attempt (a review spawns five lens attempts per reviewer,
// so one attempt is a fraction of the review), in ascending order; an explicit
// attempt narrows to that one.
func TestResolveReviewerAttemptsForProposalsDefaultsToAllCompleted(t *testing.T) {
	dir := t.TempDir()
	runPaths := contractRunPaths(dir)
	complete := func(id string) {
		paths := reviewerAttemptPaths(runPaths, id)
		assertNoError(t, activeStore.MkdirAll(paths.Dir))
		assertNoError(t, activeStore.WriteBytes(paths.ResultJSON, []byte("{}"), 0o644))
	}
	complete("reviewer_attempt_001")
	complete("reviewer_attempt_003")
	// attempt_002 exists but has no result.json: incomplete, must be skipped.
	assertNoError(t, activeStore.MkdirAll(reviewerAttemptPaths(runPaths, "reviewer_attempt_002").Dir))

	attempts, err := resolveReviewerAttemptsForProposals(runPaths, "")
	assertNoError(t, err)
	if len(attempts) != 2 || attempts[0] != "reviewer_attempt_001" || attempts[1] != "reviewer_attempt_003" {
		t.Fatalf("default should list all completed attempts ascending: %v", attempts)
	}

	attempts, err = resolveReviewerAttemptsForProposals(runPaths, "reviewer_attempt_003")
	assertNoError(t, err)
	if len(attempts) != 1 || attempts[0] != "reviewer_attempt_003" {
		t.Fatalf("explicit attempt should narrow to one: %v", attempts)
	}

	if _, err := resolveReviewerAttemptsForProposals(runPaths, "reviewer_attempt_002"); err == nil {
		t.Fatal("incomplete explicit attempt should error")
	}
}

// TestParseReviewerFindingBlocksWarnsOnUnparsedMarker mirrors the clarify-side
// silent-zero guard: a findings schema marker that yields no parsed block is a
// parse miss (e.g. a stream cut before the closing fence) and must warn
// instead of reading as a clean review.
func TestParseReviewerFindingBlocksWarnsOnUnparsedMarker(t *testing.T) {
	truncated := "Findings below.\n```json\n{\"schema\": \"" + reviewerFindingsSchema + "\", \"findings\": [\n"
	blocks, warnings := parseReviewerFindingBlocks(truncated)
	if len(blocks) != 0 {
		t.Fatalf("a block cut before its closing fence should not parse: %#v", blocks)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "parse miss") {
		t.Fatalf("expected the parse-miss warning, got %v", warnings)
	}

	blocks, warnings = parseReviewerFindingBlocks("no findings here at all")
	if len(blocks) != 0 || len(warnings) != 0 {
		t.Fatalf("genuinely empty output must stay warning-free: %#v %v", blocks, warnings)
	}
}

// TestExtractFencedJSONBlocksHandlesGluedOpener pins the ACP glue fix: an
// opening fence stuck to the tail of a prose line (id-less token deltas give
// the transport no boundary to separate on) still starts a block, and a
// line-anchored ```json reached mid-block re-opens instead of closing — an
// in-prose fence mention must not swallow the real block.
func TestExtractFencedJSONBlocksHandlesGluedOpener(t *testing.T) {
	glued := "All settled — emitting now.```json\n{\"k\": 1}\n```\n"
	blocks := extractFencedJSONBlocks(glued)
	if len(blocks) != 1 || strings.TrimSpace(blocks[0]) != "{\"k\": 1}" {
		t.Fatalf("glued opener must yield the block, got %#v", blocks)
	}

	strayThenReal := "Wrap the output in ```json\n```json\n{\"k\": 2}\n```\n"
	blocks = extractFencedJSONBlocks(strayThenReal)
	if len(blocks) != 1 || strings.TrimSpace(blocks[0]) != "{\"k\": 2}" {
		t.Fatalf("a stray glued opener must not swallow the real block, got %#v", blocks)
	}

	prose := "mentioning a ```json fence mid-sentence opens nothing"
	if blocks = extractFencedJSONBlocks(prose); len(blocks) != 0 {
		t.Fatalf("a mid-sentence fence mention must not open a block, got %#v", blocks)
	}
}
