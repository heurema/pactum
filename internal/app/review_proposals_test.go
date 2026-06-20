package app

import (
	"strings"
	"testing"
	"time"
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

// TestParseReviewerFindingBlocksWarnsOnMissingBlock verifies that any
// non-empty reviewer output without a valid findings block is a parse miss:
// a truncated block, a prose-only response, or a schema block missing the
// findings key all produce a warning; only truly empty input stays silent.
func TestParseReviewerFindingBlocksWarnsOnMissingBlock(t *testing.T) {
	truncated := "Findings below.\n```json\n{\"schema\": \"" + reviewerFindingsSchema + "\", \"findings\": [\n"
	blocks, warnings := parseReviewerFindingBlocks(truncated)
	if len(blocks) != 0 {
		t.Fatalf("a block cut before its closing fence should not parse: %#v", blocks)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "parse miss") {
		t.Fatalf("expected parse-miss warning for truncated block, got %v", warnings)
	}

	// Prose-only output (no schema marker) must also warn — it has no valid block.
	blocks, warnings = parseReviewerFindingBlocks("no findings here at all")
	if len(blocks) != 0 {
		t.Fatalf("prose-only output must not parse any blocks: %#v", blocks)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "parse miss") {
		t.Fatalf("prose-only output must warn about parse miss: %v", warnings)
	}

	// Genuinely empty input (zero bytes after agentMessageText) stays silent.
	blocks, warnings = parseReviewerFindingBlocks("")
	if len(blocks) != 0 || len(warnings) != 0 {
		t.Fatalf("empty input must stay warning-free: %#v %v", blocks, warnings)
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

// TestProposalRecordAntiFPFieldsCarryToRecord verifies that the five new
// anti-FP fields (state, trigger, fix_direction, uncertainty, current_code_only)
// are carried from the reviewer input to the returned reviewProposalRecord.
func TestProposalRecordAntiFPFieldsCarryToRecord(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	input := reviewerFindingProposalInput{
		Message:         "the message",
		Severity:        "medium",
		Category:        "quality",
		Evidence:        "concrete evidence",
		State:           "candidate",
		Trigger:         "when X calls Y",
		FixDirection:    "inline the nil check",
		Uncertainty:     "might not trigger in prod",
		CurrentCodeOnly: boolPtr(false),
	}
	rec, warning := proposalRecordFromReviewerInput("/root", "run_001", "reviewer_attempt_001", 1, input, time.Time{})
	if warning != "" {
		t.Fatalf("unexpected rejection: %s", warning)
	}
	if rec.State != "candidate" {
		t.Errorf("State: want %q, got %q", "candidate", rec.State)
	}
	if rec.Trigger != "when X calls Y" {
		t.Errorf("Trigger: want %q, got %q", "when X calls Y", rec.Trigger)
	}
	if rec.FixDirection != "inline the nil check" {
		t.Errorf("FixDirection: want %q, got %q", "inline the nil check", rec.FixDirection)
	}
	if rec.Uncertainty != "might not trigger in prod" {
		t.Errorf("Uncertainty: want %q, got %q", "might not trigger in prod", rec.Uncertainty)
	}
	if rec.CurrentCodeOnly != false {
		t.Errorf("CurrentCodeOnly: want false, got true")
	}
	if rec.Evidence != "concrete evidence" {
		t.Errorf("Evidence: want %q, got %q", "concrete evidence", rec.Evidence)
	}
}

// TestProposalRecordBlockingCurrentCodeOnlyConstraint pins the cross-field
// rule: blocking=true requires current_code_only=true. blocking=false may
// have current_code_only=false (pre-existing advisory finding).
func TestProposalRecordBlockingCurrentCodeOnlyConstraint(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	base := reviewerFindingProposalInput{
		Message:      "blocker",
		Severity:     "high",
		Category:     "correctness",
		Evidence:     "e",
		State:        "confirmed",
		Trigger:      "always",
		FixDirection: "fix it",
	}

	// blocking=true + current_code_only=false → rejected.
	bad := base
	bad.Blocking = boolPtr(true)
	bad.CurrentCodeOnly = boolPtr(false)
	if _, w := proposalRecordFromReviewerInput("/root", "run_001", "att_001", 1, bad, time.Time{}); !strings.Contains(w, "blocking finding must have current_code_only=true") {
		t.Errorf("expected rejection for blocking+!currentCodeOnly, got %q", w)
	}

	// blocking=true + current_code_only=true → accepted.
	good := base
	good.Blocking = boolPtr(true)
	good.CurrentCodeOnly = boolPtr(true)
	if _, w := proposalRecordFromReviewerInput("/root", "run_001", "att_001", 1, good, time.Time{}); w != "" {
		t.Errorf("blocking+currentCodeOnly should be accepted, got %q", w)
	}

	// blocking=false + current_code_only=false → accepted (pre-existing advisory).
	preExisting := base
	preExisting.Blocking = boolPtr(false)
	preExisting.CurrentCodeOnly = boolPtr(false)
	if _, w := proposalRecordFromReviewerInput("/root", "run_001", "att_001", 1, preExisting, time.Time{}); w != "" {
		t.Errorf("non-blocking pre-existing finding should be accepted, got %q", w)
	}
}

// TestContractFindingFromInputToleratesAbsentAntiFPFields verifies that the
// contract-review parsing path (contractFindingFromInput) accepts reviewer
// output that omits the new anti-FP fields — it must not require state,
// trigger, fix_direction, uncertainty, or current_code_only.
func TestContractFindingFromInputToleratesAbsentAntiFPFields(t *testing.T) {
	input := contractReviewerFindingInput{
		Message:  "potential deadlock",
		Severity: "high",
		Category: "correctness",
		Evidence: "seen in loop at line 42",
	}
	finding := contractFindingFromInput("claude", "correctness", input)
	if finding.Message != "potential deadlock" {
		t.Errorf("Message: want %q, got %q", "potential deadlock", finding.Message)
	}
	if finding.Severity != "high" {
		t.Errorf("Severity: want %q, got %q", "high", finding.Severity)
	}
	if finding.Reviewer != "claude" {
		t.Errorf("Reviewer: want %q, got %q", "claude", finding.Reviewer)
	}
}
