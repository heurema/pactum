package app

import "testing"

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
