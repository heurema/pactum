package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

const (
	reviewLoopReviewerName  = "loop-reviewer"
	reviewLoopPanelLowName  = "loop-reviewer-low"
	reviewLoopPanelHighName = "loop-reviewer-high"
	reviewLoopFixerName     = "loop-fixer"
)

func TestReviewLoopRequiresYes(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("review loop without --yes exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "review loop requires --yes") {
		t.Fatalf("review loop confirmation stderr mismatch:\n%s", got)
	}
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_001").ResultJSON)
	assertNoFile(t, runPaths.ReviewLoopSummaryJSON)
}

func TestReviewLoopFindingsThenCleanUsesConfigMaxRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	setReviewLoopMaxRoundsConfig(t, paths, 2)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "findings_then_clean")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if strings.Contains(stdout.String(), "total_open_findings") {
		t.Fatalf("loop summary JSON should not include total_open_findings:\n%s", stdout.String())
	}
	if summary.Schema != reviewLoopSummarySchema || summary.RunID != runID || summary.MaxRounds != 2 || summary.TerminalReason != "clean_round" {
		t.Fatalf("unexpected loop summary: %#v", summary)
	}
	if summary.CleanRoundsRequired != 1 || summary.StalematePatience != 2 {
		t.Fatalf("default loop limits mismatch: %#v", summary)
	}
	if summary.Reviewer != reviewLoopReviewerName || summary.Agent != reviewLoopFixerName {
		t.Fatalf("summary should record resolved agents: %#v", summary)
	}
	if len(summary.Rounds) != 2 {
		t.Fatalf("rounds = %d, want 2: %#v", len(summary.Rounds), summary.Rounds)
	}
	if got := summary.Rounds[0]; got.OpenFindings != 1 || got.ProposalsCreated != 1 || got.ProposalsAccepted != 1 || got.FixerAttemptID != "attempt_001" || got.GateStatus != "passed" {
		t.Fatalf("round 1 summary mismatch: %#v", got)
	}
	if got := summary.Rounds[1]; got.OpenFindings != 1 || got.ProposalsCreated != 0 || got.ProposalsAccepted != 0 || got.FixerAttemptID != "" || got.GateStatus != "" {
		t.Fatalf("round 2 summary mismatch: %#v", got)
	}

	artifact := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if artifact.TerminalReason != summary.TerminalReason || len(artifact.Rounds) != len(summary.Rounds) {
		t.Fatalf("summary artifact mismatch:\nstdout=%#v\nartifact=%#v", summary, artifact)
	}
	assertFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_001").ResultJSON)
	assertFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_002").ResultJSON)
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_002").ResultJSON)

	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || findings[0].Source != "reviewer_proposal" || findings[0].Status != "open" {
		t.Fatalf("loop should accept first reviewer proposal into one open finding: %#v", findings)
	}
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Status == "approved" || review.Approval.ApprovedAt != nil || review.Approval.ApprovedBy != nil {
		t.Fatalf("review loop must not auto-approve review: %#v", review)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	for _, want := range []string{"review_loop_started", "reviewer_attempt_started", "review_fix_attempt_started", "gate_run_started", "review_loop_finished"} {
		if indexOfEvent(eventTypes, want) == -1 {
			t.Fatalf("events missing %s:\n%v", want, eventTypes)
		}
	}
}

func TestReviewLoopAppliesFixOutcomesAndShrinksOpenFindings(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "findings_then_clean")
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "fix_f001")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "2", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "clean_round" || len(summary.Rounds) != 2 {
		t.Fatalf("fix outcome loop summary mismatch: %#v", summary)
	}
	if got := summary.Rounds[0]; got.OpenFindings != 0 || got.FixOutcomesResolved != 1 || got.FixOutcomesRebutted != 0 || got.FixOutcomesBlocked != 0 {
		t.Fatalf("round 1 should reflect fixed outcome and shrunken open set: %#v", got)
	}
	if got := summary.Rounds[1]; got.OpenFindings != 0 || got.ProposalsAccepted != 0 || got.FixerAttemptID != "" {
		t.Fatalf("round 2 should stay clean after fix outcome: %#v", got)
	}
	resolutions := readReviewResolutions(t, runPaths.ReviewResolutionsJSONL)
	if len(resolutions) != 1 || resolutions[0].Outcome != "fixed" || resolutions[0].Source != "review_fix" {
		t.Fatalf("fix outcome resolution mismatch: %#v", resolutions)
	}
}

func TestReviewLoopStopsAtMaxRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "terminal reason: max_rounds") || !strings.Contains(got, "rounds: 1/1") {
		t.Fatalf("human loop output mismatch:\n%s", got)
	}
	summary := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if summary.TerminalReason != "max_rounds" || len(summary.Rounds) != 1 || summary.Rounds[0].OpenFindings != 1 {
		t.Fatalf("max rounds summary mismatch: %#v", summary)
	}
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_002").ResultJSON)
}

func TestReviewLoopStopsWhenCapturedTokenBudgetExceeded(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	setReviewLoopBudgetConfig(t, paths, budgetModeBlock, reviewLoopInt64Ptr(100))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		SchemaVersion: usageRecordSchemaVersion,
		RecordID:      "usage_budget_001",
		RunID:         runID,
		AttemptID:     "attempt_budget_001",
		Stage:         "review",
		Provider:      "codex",
		Agent:         "codex",
		TotalTokens:   150,
		Captured:      true,
		CreatedAt:     "2026-06-07T18:00:00Z",
	})
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "terminal reason: budget_exceeded") || !strings.Contains(got, "budget max tokens: 100") || !strings.Contains(got, "budget captured tokens: 150") {
		t.Fatalf("human budget stop output mismatch:\n%s", got)
	}

	summary := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if summary.TerminalReason != reviewLoopTerminalBudgetExceeded || len(summary.Rounds) != 1 {
		t.Fatalf("budget stop summary mismatch: %#v", summary)
	}
	if summary.Budget == nil || summary.Budget.Mode != budgetModeBlock || summary.Budget.MaxTokens != 100 || summary.Budget.CapturedTotalTokens != 150 {
		t.Fatalf("budget summary mismatch: %#v", summary.Budget)
	}
	assertFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_001").ResultJSON)
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_002").ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_002").ResultJSON)
}

func TestReviewLoopBudgetWarnContinuesToOtherTerminal(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	setReviewLoopBudgetConfig(t, paths, budgetModeWarn, reviewLoopInt64Ptr(100))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		SchemaVersion: usageRecordSchemaVersion,
		RecordID:      "usage_budget_001",
		RunID:         runID,
		AttemptID:     "attempt_budget_001",
		Stage:         "review",
		Provider:      "codex",
		Agent:         "codex",
		TotalTokens:   150,
		Captured:      true,
		CreatedAt:     "2026-06-07T18:00:00Z",
	})
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "stalemate" || len(summary.Rounds) != 2 {
		t.Fatalf("warn budget should continue to stalemate: %#v", summary)
	}
	if summary.Budget == nil || summary.Budget.Mode != budgetModeWarn || summary.Budget.CapturedTotalTokens != 150 || len(summary.Budget.Warnings) == 0 {
		t.Fatalf("warn budget summary mismatch: %#v", summary.Budget)
	}
	if !strings.Contains(summary.Budget.Warnings[0], "captured_total_tokens=150") {
		t.Fatalf("budget warning should record totals: %#v", summary.Budget.Warnings)
	}
}

func TestReviewLoopNoMaxTokensIgnoresExistingUsage(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		SchemaVersion: usageRecordSchemaVersion,
		RecordID:      "usage_budget_001",
		RunID:         runID,
		AttemptID:     "attempt_budget_001",
		Stage:         "review",
		Provider:      "codex",
		Agent:         "codex",
		TotalTokens:   999999,
		Captured:      true,
		CreatedAt:     "2026-06-07T18:00:00Z",
	})
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "findings_then_clean")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "2", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "clean_round" || len(summary.Rounds) != 2 || summary.Budget != nil {
		t.Fatalf("loop without max_tokens should be unaffected: %#v", summary)
	}
}

func TestReviewLoopBudgetMaxTokensTreatsNonPositiveAsOff(t *testing.T) {
	zero := int64(0)
	negative := int64(-5)
	positive := int64(100)
	if reviewLoopBudgetMaxTokens(nil) != nil {
		t.Fatal("nil max_tokens should stay disabled")
	}
	if reviewLoopBudgetMaxTokens(&zero) != nil {
		t.Fatal("max_tokens 0 should be treated as off")
	}
	if reviewLoopBudgetMaxTokens(&negative) != nil {
		t.Fatal("negative max_tokens should be treated as off")
	}
	if got := reviewLoopBudgetMaxTokens(&positive); got == nil || *got != 100 {
		t.Fatalf("positive max_tokens should pass through, got %v", got)
	}
}

func TestReviewLoopBudgetCountsOnlyCapturedUsage(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	setReviewLoopBudgetConfig(t, paths, budgetModeBlock, reviewLoopInt64Ptr(100))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		SchemaVersion: usageRecordSchemaVersion,
		RecordID:      "usage_budget_uncaptured",
		RunID:         runID,
		AttemptID:     "attempt_budget_uncaptured",
		Stage:         "review",
		Provider:      "codex",
		Agent:         "codex",
		TotalTokens:   10000,
		Captured:      false,
		CreatedAt:     "2026-06-07T18:00:00Z",
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		SchemaVersion: usageRecordSchemaVersion,
		RecordID:      "usage_budget_captured",
		RunID:         runID,
		AttemptID:     "attempt_budget_captured",
		Stage:         "review",
		Provider:      "codex",
		Agent:         "codex",
		TotalTokens:   50,
		Captured:      true,
		CreatedAt:     "2026-06-07T18:01:00Z",
	})
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "stalemate" || len(summary.Rounds) != 2 {
		t.Fatalf("uncaptured usage must not trip budget: %#v", summary)
	}
	if summary.Budget == nil || summary.Budget.CapturedTotalTokens != 50 {
		t.Fatalf("budget should report captured-only tokens: %#v", summary.Budget)
	}
}

func TestReviewLoopDedupsReproposedOpenFindingAcrossRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "4", "--patience", "2", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "stalemate" || summary.MaxRounds != 4 || summary.StalematePatience != 2 {
		t.Fatalf("stalemate summary mismatch: %#v", summary)
	}
	if len(summary.Rounds) != 2 {
		t.Fatalf("rounds = %d, want 2: %#v", len(summary.Rounds), summary.Rounds)
	}
	if got := summary.Rounds[0]; got.ProposalsCreated != 1 || got.ProposalsAccepted != 1 || got.OpenFindings != 1 || got.UnchangedFingerprintStreak != 1 || got.WorkingTreeFingerprint == "" {
		t.Fatalf("round 1 dedup/stalemate signals mismatch: %#v", got)
	}
	if got := summary.Rounds[1]; got.ProposalsCreated != 1 || got.ProposalsAccepted != 0 || got.OpenFindings != 1 || got.UnchangedFingerprintStreak != 2 || got.WorkingTreeFingerprint == "" {
		t.Fatalf("round 2 dedup/stalemate signals mismatch: %#v", got)
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 {
		t.Fatalf("re-proposed open finding should stay a single finding: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 2 || decisions[0].Decision != "accepted" || decisions[0].FindingID != "f_001" || decisions[1].Decision != "duplicate" || decisions[1].FindingID != "f_001" {
		t.Fatalf("duplicate proposal decisions mismatch: %#v", decisions)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "review_proposal_accepted") != 1 || countEvents(eventTypes, "review_finding_added") != 1 || countEvents(eventTypes, "review_proposal_duplicate") != 1 {
		t.Fatalf("duplicate loop ledger event counts mismatch:\n%v", eventTypes)
	}
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_002").ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_003").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_003").ResultJSON)
}

func TestReviewLoopAcceptsNewFindingInLaterRound(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_new_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "2", "--patience", "3", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "max_rounds" || len(summary.Rounds) != 2 {
		t.Fatalf("new later finding summary mismatch: %#v", summary)
	}
	if got := summary.Rounds[0]; got.ProposalsCreated != 1 || got.ProposalsAccepted != 1 || got.OpenFindings != 1 {
		t.Fatalf("round 1 new finding summary mismatch: %#v", got)
	}
	if got := summary.Rounds[1]; got.ProposalsCreated != 1 || got.ProposalsAccepted != 1 || got.OpenFindings != 2 {
		t.Fatalf("round 2 new finding summary mismatch: %#v", got)
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 2 || findings[0].ID != "f_001" || findings[1].ID != "f_002" {
		t.Fatalf("new later finding should be accepted: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 2 || decisions[0].Decision != "accepted" || decisions[1].Decision != "accepted" {
		t.Fatalf("new later finding decisions mismatch: %#v", decisions)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "review_proposal_accepted") != 2 || countEvents(eventTypes, "review_finding_added") != 2 || countEvents(eventTypes, "review_proposal_duplicate") != 0 {
		t.Fatalf("new later finding ledger event counts mismatch:\n%v", eventTypes)
	}
}

func TestReviewLoopDedupsIdenticalProposalsWithinRound(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "same_round_duplicates")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "max_rounds" || len(summary.Rounds) != 1 {
		t.Fatalf("same-round duplicate summary mismatch: %#v", summary)
	}
	if got := summary.Rounds[0]; got.ProposalsCreated != 2 || got.ProposalsAccepted != 1 || got.OpenFindings != 1 {
		t.Fatalf("same-round duplicate round mismatch: %#v", got)
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 {
		t.Fatalf("same-round duplicates should create one finding: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 2 || decisions[0].Decision != "accepted" || decisions[0].FindingID != "f_001" || decisions[1].Decision != "duplicate" || decisions[1].FindingID != "f_001" {
		t.Fatalf("same-round duplicate decisions mismatch: %#v", decisions)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "review_proposal_accepted") != 1 || countEvents(eventTypes, "review_finding_added") != 1 || countEvents(eventTypes, "review_proposal_duplicate") != 1 {
		t.Fatalf("same-round duplicate ledger event counts mismatch:\n%v", eventTypes)
	}
}

func TestReviewLoopPanelRunsReviewersAndUpgradesDuplicateSeverity(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	setReviewPanelConfig(t, paths, reviewLoopPanelLowName, reviewLoopPanelHighName)
	app = configureReviewLoopPanelHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "panel_duplicate")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.Reviewer != reviewLoopPanelLowName+","+reviewLoopPanelHighName || !sameStringSlice(summary.Reviewers, []string{reviewLoopPanelLowName, reviewLoopPanelHighName}) {
		t.Fatalf("summary should record full reviewer panel: %#v", summary)
	}
	if len(summary.Rounds) != 1 || len(summary.Rounds[0].ReviewerAttemptIDs) != 2 || summary.Rounds[0].ReviewerAttemptID == "" {
		t.Fatalf("round should record both reviewer attempts: %#v", summary.Rounds)
	}
	if got := summary.Rounds[0]; got.ProposalsCreated != 2 || got.ProposalsAccepted != 1 || got.OpenFindings != 1 {
		t.Fatalf("panel duplicate round mismatch: %#v", got)
	}
	for _, attemptID := range summary.Rounds[0].ReviewerAttemptIDs {
		assertFile(t, reviewerAttemptPaths(runPaths, attemptID).ResultJSON)
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || findings[0].Severity != "critical" {
		t.Fatalf("duplicate severity should upgrade stored finding to critical: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 2 || decisions[0].Decision != "accepted" || decisions[1].Decision != "duplicate" || decisions[1].FindingID != "f_001" {
		t.Fatalf("panel duplicate decisions mismatch: %#v", decisions)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "reviewer_attempt_started") != 2 || countEvents(eventTypes, "review_finding_severity_upgraded") != 1 {
		t.Fatalf("panel ledger event counts mismatch:\n%v", eventTypes)
	}
	if got := stderr.String(); !strings.Contains(got, "panel_reviewer=low") || !strings.Contains(got, "panel_reviewer=high") {
		t.Fatalf("panel live output missing from stderr:\n%s", got)
	}
}

func TestReviewLoopExplicitReviewerDisablesConfiguredPanel(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	setReviewPanelConfig(t, paths, reviewLoopPanelLowName, reviewLoopPanelHighName)
	app = configureReviewLoopPanelHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.Reviewer != reviewLoopReviewerName || len(summary.Reviewers) != 0 {
		t.Fatalf("explicit reviewer should disable configured panel: %#v", summary)
	}
	if len(summary.Rounds) != 1 || summary.Rounds[0].ReviewerAttemptID == "" || len(summary.Rounds[0].ReviewerAttemptIDs) != 0 {
		t.Fatalf("explicit reviewer should run one reviewer attempt: %#v", summary.Rounds)
	}
	assertFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_001").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_002").ResultJSON)
}

func TestReviewLoopCleanPanelRoundTerminates(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	setReviewPanelConfig(t, paths, reviewLoopPanelLowName, reviewLoopPanelHighName)
	app = configureReviewLoopPanelHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "panel_clean")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--agent", reviewLoopFixerName, "--max-rounds", "4", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "clean_round" || len(summary.Rounds) != 1 {
		t.Fatalf("clean panel should terminate after one round: %#v", summary)
	}
	if len(summary.Rounds[0].ReviewerAttemptIDs) != 2 || summary.Rounds[0].ProposalsCreated != 0 || summary.Rounds[0].ProposalsAccepted != 0 {
		t.Fatalf("clean panel round mismatch: %#v", summary.Rounds[0])
	}
	assertFile(t, reviewerAttemptPaths(runPaths, summary.Rounds[0].ReviewerAttemptIDs[0]).ResultJSON)
	assertFile(t, reviewerAttemptPaths(runPaths, summary.Rounds[0].ReviewerAttemptIDs[1]).ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
}

func TestReviewLoopUnknownPanelReviewerFailsClearly(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	setReviewPanelConfig(t, paths, "missing-reviewer", reviewLoopPanelHighName)
	app = configureReviewLoopPanelHelpers(app)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--yes"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("unknown panel reviewer exited %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, `agents.review_panel reviewer "missing-reviewer"`) || !strings.Contains(got, "unsupported agent") {
		t.Fatalf("unknown panel reviewer error mismatch:\n%s", got)
	}
}

func TestReviewLoopReacceptsResolvedFindingWhenReproposed(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	appendReviewLoopFindingForTest(t, runPaths, runID, "f_001", reviewLoopFixtureFindingCore("loop reviewer found a fixable issue", 42))
	appendReviewLoopResolutionForTest(t, runPaths, runID, "r_001", "f_001")
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if len(summary.Rounds) != 1 || summary.Rounds[0].ProposalsAccepted != 1 || summary.Rounds[0].OpenFindings != 1 {
		t.Fatalf("resolved reproposal summary mismatch: %#v", summary)
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 2 || findings[1].ID != "f_002" {
		t.Fatalf("resolved finding should not suppress reproposal: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 1 || decisions[0].Decision != "accepted" || decisions[0].FindingID != "f_002" {
		t.Fatalf("resolved reproposal decision mismatch: %#v", decisions)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "review_proposal_duplicate") != 0 || countEvents(eventTypes, "review_finding_added") != 1 {
		t.Fatalf("resolved reproposal ledger event counts mismatch:\n%v", eventTypes)
	}
}

func TestReviewLoopSuppressesRebuttedResolvedFindingWhenReproposed(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	appendReviewLoopFindingForTest(t, runPaths, runID, "f_001", reviewLoopFixtureFindingCore("loop reviewer found a fixable issue", 42))
	appendReviewLoopResolutionWithOutcomeForTest(t, runPaths, runID, "r_001", "f_001", "rebutted")
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if len(summary.Rounds) != 1 || summary.Rounds[0].ProposalsAccepted != 0 || summary.Rounds[0].OpenFindings != 0 {
		t.Fatalf("rebutted reproposal summary mismatch: %#v", summary)
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 {
		t.Fatalf("rebutted finding should suppress reproposal: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 1 || decisions[0].Decision != "duplicate" || decisions[0].FindingID != "f_001" || decisions[0].Reason != "matches rebutted finding" {
		t.Fatalf("rebutted reproposal decision mismatch: %#v", decisions)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "review_proposal_duplicate") != 1 || countEvents(eventTypes, "review_finding_added") != 0 {
		t.Fatalf("rebutted reproposal ledger event counts mismatch:\n%v", eventTypes)
	}
}

func TestReviewLoopResetsStalemateStreakWhenFixerChangesWorkingTree(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_new_findings")
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_SEQUENCE_FILE", filepath.Join(stateDir, "fixer-sequence"))
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "append_readme")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--patience", "2", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "max_rounds" {
		t.Fatalf("changing fixer should avoid premature stalemate: %#v", summary)
	}
	if len(summary.Rounds) != 3 {
		t.Fatalf("rounds = %d, want 3: %#v", len(summary.Rounds), summary.Rounds)
	}
	for index, round := range summary.Rounds {
		if round.ProposalsAccepted != 1 || round.FixerAttemptID == "" || round.WorkingTreeFingerprint == "" {
			t.Fatalf("round %d fix signals mismatch: %#v", index+1, round)
		}
		if round.UnchangedFingerprintStreak != 0 {
			t.Fatalf("round %d unchanged streak = %d, want 0 after changed fix: %#v", index+1, round.UnchangedFingerprintStreak, round)
		}
	}
	if got := summary.Rounds[2]; got.FixerAttemptID != "attempt_003" || got.GateStatus != "needs_review" {
		t.Fatalf("loop should run all fixer rounds with changed gate status: %#v", got)
	}
	if got := mustReadFile(t, filepath.Join(root, "README.md")); !strings.Contains(got, "fixer-change=3") {
		t.Fatalf("fixer did not append distinct README changes:\n%s", got)
	}
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_003").ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_004").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_004").ResultJSON)
}

func TestReviewLoopRequiresConsecutiveCleanRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "clean_findings_clean_clean")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "4", "--clean-rounds", "2", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "clean_round" || summary.CleanRoundsRequired != 2 {
		t.Fatalf("clean streak summary mismatch: %#v", summary)
	}
	if len(summary.Rounds) != 4 {
		t.Fatalf("rounds = %d, want 4: %#v", len(summary.Rounds), summary.Rounds)
	}
	wantCleanStreaks := []int{1, 0, 1, 2}
	for index, want := range wantCleanStreaks {
		if got := summary.Rounds[index].CleanStreak; got != want {
			t.Fatalf("round %d clean streak = %d, want %d: %#v", index+1, got, want, summary.Rounds[index])
		}
	}
	wantOpenFindings := []int{0, 1, 1, 1}
	for index, want := range wantOpenFindings {
		if got := summary.Rounds[index].OpenFindings; got != want {
			t.Fatalf("round %d open findings = %d, want %d: %#v", index+1, got, want, summary.Rounds[index])
		}
	}
	if got := summary.Rounds[1]; got.ProposalsAccepted != 1 || got.FixerAttemptID != "attempt_001" || got.UnchangedFingerprintStreak != 1 {
		t.Fatalf("non-clean round should reset clean streak and run one fixer: %#v", got)
	}
	assertFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_004").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_005").ResultJSON)
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_002").ResultJSON)
}

func TestReviewLoopUnparsedFindingsIsNotCleanRound(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "malformed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "2", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stderr: %s", code, stderr.String())
	}
	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "reviewer_findings_unparsed" {
		t.Fatalf("unparseable reviewer findings must not be a clean pass: terminal=%q\n%#v", summary.TerminalReason, summary)
	}
	if len(summary.Rounds) != 1 || summary.Rounds[0].ProposalsCreated != 0 || len(summary.Rounds[0].Warnings) == 0 {
		t.Fatalf("expected one round with 0 created proposals and warnings: %#v", summary.Rounds)
	}
	// Nothing was accepted, so the fixer must not have run.
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
}

func TestReviewLoopStopsWithGateFailedWhenFixerBreaksValidation(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	t.Setenv("PACTUM_GATE_HELPER_PROCESS", "1")
	app, paths, runID := setupGatePreparedRun(t, root, []string{gateValidationCommandForTest()}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID, "--allow-commands")
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
	t.Setenv("PACTUM_GATE_HELPER_EXIT", "7")

	var stdout, stderr bytes.Buffer
	err := app.ReviewLoop(&stdout, &stderr, runID, reviewLoopOptions{
		Reviewer:   reviewLoopReviewerName,
		Agent:      reviewLoopFixerName,
		MaxRounds:  3,
		Yes:        true,
		JSONOutput: true,
	})
	if err != nil {
		t.Fatalf("review loop should stop cleanly on failed gate, got error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "gate_failed" || len(summary.Rounds) != 1 {
		t.Fatalf("gate failure summary mismatch: %#v", summary)
	}
	round := summary.Rounds[0]
	if round.GateStatus != "failed" || round.GateReportArtifact != gateReportArtifact || round.FixerAttemptID != "attempt_001" {
		t.Fatalf("failed gate round summary mismatch: %#v", round)
	}
	artifact := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if artifact.TerminalReason != "gate_failed" || len(artifact.Rounds) != 1 || artifact.Rounds[0].GateReportArtifact != gateReportArtifact {
		t.Fatalf("failed gate summary artifact mismatch: %#v", artifact)
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if report.Status != "failed" || len(report.Validation.Commands) != 1 || report.Validation.Commands[0].ExitCode != 7 {
		t.Fatalf("failed gate report mismatch: %#v", report)
	}
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_002").ResultJSON)
}

func TestReviewLoopStopsWithGateFailedWhenFixerViolatesPathScope(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, []string{"--add-path-in-scope", "internal/app/**"}, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "append_readme")

	var stdout, stderr bytes.Buffer
	err := app.ReviewLoop(&stdout, &stderr, runID, reviewLoopOptions{
		Reviewer:   reviewLoopReviewerName,
		Agent:      reviewLoopFixerName,
		MaxRounds:  3,
		Yes:        true,
		JSONOutput: true,
	})
	if err != nil {
		t.Fatalf("review loop should stop cleanly on blocked scope gate, got error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "gate_failed" || len(summary.Rounds) != 1 {
		t.Fatalf("scope gate failure summary mismatch: %#v", summary)
	}
	round := summary.Rounds[0]
	if round.GateStatus != "failed" || round.GateReportArtifact != gateReportArtifact || round.FixerAttemptID != "attempt_001" {
		t.Fatalf("blocked scope round summary mismatch: %#v", round)
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if report.Status != "failed" || report.Scope == nil || report.Scope.Status != "blocked" || !report.Summary.ScopeBlocked {
		t.Fatalf("blocked scope gate report mismatch: %#v", report)
	}
	if !containsString(report.Scope.Undeclared, "README.md") {
		t.Fatalf("blocked scope should report fixer README change: %#v", report.Scope)
	}
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_002").ResultJSON)
}

func TestReviewLoopGateInfrastructureErrorStillReturnsError(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	mustWriteFile(t, executionAttemptPaths(runPaths, "attempt_001").ResultJSON, "{")
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	err := app.ReviewLoop(&stdout, &stderr, runID, reviewLoopOptions{
		Reviewer:   reviewLoopReviewerName,
		Agent:      reviewLoopFixerName,
		MaxRounds:  1,
		Yes:        true,
		JSONOutput: true,
	})
	if err == nil {
		t.Fatalf("review loop should return gate infrastructure error")
	}
	var gateErr gateProcessError
	if errors.As(err, &gateErr) {
		t.Fatalf("infrastructure error should not be classified as gate process failure: %v", err)
	}
	summary := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if summary.TerminalReason != "error" || len(summary.Rounds) != 1 {
		t.Fatalf("infrastructure error summary mismatch: %#v", summary)
	}
	if got := summary.Rounds[0]; got.GateStatus != "" || got.GateReportArtifact != "" || got.FixerAttemptID != "attempt_001" {
		t.Fatalf("infrastructure error should not record a gate report for the round: %#v", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("errored review loop should not emit summary JSON stdout:\n%s", stdout.String())
	}
}

func TestReviewLoopStreamsSubRunOutputToStderrWithCleanStdout(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runReviewCommand(t, app, "gate", "run", runID)
	runReviewCommand(t, app, "review", "prepare", runID)
	setReviewLoopMaxRoundsConfig(t, paths, 2)
	app = configureReviewLoopHelpers(app)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "findings_then_clean")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "loop", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	// The loop's own stdout stays a clean, parseable summary. The sub-command
	// JSON the loop parses internally must not leak onto it.
	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "clean_round" {
		t.Fatalf("unexpected loop summary: %#v", summary)
	}
	if out := stdout.String(); strings.Contains(out, "stdin_has_reviewer_prompt=") || strings.Contains(out, "stdin_has_review_fix_prompt=") {
		t.Fatalf("sub-run agent output leaked into loop stdout:\n%s", out)
	}

	// The per-round reviewer and fixer agent output streams live to the
	// operator's stderr.
	if got := stderr.String(); !strings.Contains(got, "stdin_has_reviewer_prompt=true") || !strings.Contains(got, "stdin_has_review_fix_prompt=true") {
		t.Fatalf("sub-run live output missing from stderr:\n%s", got)
	}
}

func configureReviewLoopHelpers(app App) App {
	app.AgentRegistry = testAgentRegistry(
		agents.AgentDescriptor{
			Name:    reviewLoopReviewerName,
			Command: os.Args[0],
			Args:    []string{"-test.run=TestReviewLoopReviewerHelperProcess"},
			Input:   agents.InputPromptFile,
		},
		agents.AgentDescriptor{
			Name:    reviewLoopFixerName,
			Command: os.Args[0],
			Args:    []string{"-test.run=TestReviewLoopFixerHelperProcess"},
			Input:   agents.InputPromptFile,
		},
	)
	return app
}

func configureReviewLoopPanelHelpers(app App) App {
	app.AgentRegistry = testAgentRegistry(
		agents.AgentDescriptor{
			Name:    reviewLoopReviewerName,
			Command: os.Args[0],
			Args:    []string{"-test.run=TestReviewLoopReviewerHelperProcess"},
			Input:   agents.InputPromptFile,
		},
		agents.AgentDescriptor{
			Name:    reviewLoopPanelLowName,
			Command: os.Args[0],
			Args:    []string{"-test.run=TestReviewLoopPanelLowReviewerHelperProcess"},
			Input:   agents.InputPromptFile,
		},
		agents.AgentDescriptor{
			Name:    reviewLoopPanelHighName,
			Command: os.Args[0],
			Args:    []string{"-test.run=TestReviewLoopPanelHighReviewerHelperProcess"},
			Input:   agents.InputPromptFile,
		},
		agents.AgentDescriptor{
			Name:    reviewLoopFixerName,
			Command: os.Args[0],
			Args:    []string{"-test.run=TestReviewLoopFixerHelperProcess"},
			Input:   agents.InputPromptFile,
		},
	)
	return app
}

func setReviewLoopMaxRoundsConfig(t *testing.T, paths artifacts.Paths, maxRounds int) {
	t.Helper()
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	config.Limits.Review.MaxRounds = maxRounds
	assertNoError(t, writeYAML(paths.Config, config))
}

func setReviewPanelConfig(t *testing.T, paths artifacts.Paths, reviewers ...string) {
	t.Helper()
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	config.Agents.ReviewPanel = append([]string{}, reviewers...)
	assertNoError(t, writeYAML(paths.Config, config))
}

func setReviewLoopBudgetConfig(t *testing.T, paths artifacts.Paths, mode string, maxTokens *int64) {
	t.Helper()
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	config.Budget.Mode = mode
	config.Budget.MaxTokens = maxTokens
	assertNoError(t, writeYAML(paths.Config, config))
}

func reviewLoopInt64Ptr(value int64) *int64 {
	return &value
}

func setReviewLoopHelperEnv(t *testing.T, root string, sequenceFile string, mode string) {
	t.Helper()
	t.Setenv("PACTUM_REVIEW_LOOP_REVIEWER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEW_LOOP_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_REVIEW_LOOP_REVIEWER_SEQUENCE_FILE", sequenceFile)
	t.Setenv("PACTUM_REVIEW_LOOP_REVIEWER_MODE", mode)
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_EXPECTED_CWD", root)
}

func readReviewLoopSummary(t *testing.T, path string) reviewLoopSummaryDocument {
	t.Helper()
	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &summary))
	return summary
}

func reviewLoopFixtureFindingCore(message string, line int) findingCore {
	return findingCore{
		Message:  message,
		Severity: "medium",
		Category: "quality",
		File:     "internal/app/review_loop.go",
		Line:     line,
		Blocking: true,
	}
}

func reviewLoopFixtureFinding(message string, line int) map[string]any {
	core := reviewLoopFixtureFindingCore(message, line)
	return map[string]any{
		"message":  core.Message,
		"severity": core.Severity,
		"category": core.Category,
		"file":     core.File,
		"line":     core.Line,
		"blocking": core.Blocking,
		"evidence": "fixture reviewer sequence",
	}
}

func reviewLoopFixtureFindingWithSeverity(message string, line int, severity string) map[string]any {
	finding := reviewLoopFixtureFinding(message, line)
	finding["severity"] = severity
	return finding
}

func appendReviewLoopFindingForTest(t *testing.T, runPaths contractRunPathSet, runID string, findingID string, core findingCore) {
	t.Helper()
	record := reviewFindingRecord{
		Schema:      reviewFindingSchema,
		ID:          findingID,
		RunID:       runID,
		findingCore: core,
		Status:      "open",
		CreatedAt:   "2026-06-01T22:00:00Z",
		Source:      "reviewer_proposal",
	}
	assertNoError(t, appendJSONLine(runPaths.ReviewFindingsJSONL, record))
}

func appendReviewLoopResolutionForTest(t *testing.T, runPaths contractRunPathSet, runID string, resolutionID string, findingID string) {
	t.Helper()
	appendReviewLoopResolutionWithOutcomeForTest(t, runPaths, runID, resolutionID, findingID, "")
}

func appendReviewLoopResolutionWithOutcomeForTest(t *testing.T, runPaths contractRunPathSet, runID string, resolutionID string, findingID string, outcome string) {
	t.Helper()
	record := reviewResolutionRecord{
		Schema:    reviewResolutionSchema,
		ID:        resolutionID,
		RunID:     runID,
		FindingID: findingID,
		Outcome:   outcome,
		Note:      "fixture resolved finding",
		CreatedAt: "2026-06-01T22:01:00Z",
		Source:    "manual",
	}
	assertNoError(t, appendJSONLine(runPaths.ReviewResolutionsJSONL, record))
}

func TestReviewLoopReviewerHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_REVIEW_LOOP_REVIEWER_PROCESS") != "1" {
		return
	}
	assertReviewLoopHelperCWD()
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("stdin_has_reviewer_prompt=%t\n", strings.Contains(string(stdin), "# Reviewer Prompt"))
	attempt := nextReviewLoopReviewerAttempt()
	mode := os.Getenv("PACTUM_REVIEW_LOOP_REVIEWER_MODE")
	validFinding := []map[string]any{reviewLoopFixtureFinding("loop reviewer found a fixable issue", 42)}
	switch {
	case mode == "malformed":
		// Empty message -> propose-findings skips it with a warning (0 created),
		// which must NOT be treated as a clean review round.
		fmt.Print(reviewerStructuredOutput([]map[string]any{
			{"message": "", "severity": "medium", "category": "quality"},
		}))
	case mode == "always_findings":
		fmt.Print(reviewerStructuredOutput(validFinding))
	case mode == "always_new_findings":
		fmt.Print(reviewerStructuredOutput([]map[string]any{
			reviewLoopFixtureFinding(fmt.Sprintf("loop reviewer found fixable issue %d", attempt), 42+attempt),
		}))
	case mode == "same_round_duplicates":
		fmt.Print(reviewerStructuredOutput([]map[string]any{
			reviewLoopFixtureFinding("loop reviewer found a fixable issue", 42),
			reviewLoopFixtureFinding("loop reviewer found a fixable issue", 42),
		}))
	case mode == "clean_findings_clean_clean":
		if attempt == 2 {
			fmt.Print(reviewerStructuredOutput(validFinding))
		}
	case attempt == 1:
		fmt.Print(reviewerStructuredOutput(validFinding))
	}
	os.Exit(0)
}

func TestReviewLoopPanelLowReviewerHelperProcess(t *testing.T) {
	runReviewLoopPanelReviewerHelper("low", "low")
}

func TestReviewLoopPanelHighReviewerHelperProcess(t *testing.T) {
	runReviewLoopPanelReviewerHelper("high", "critical")
}

func runReviewLoopPanelReviewerHelper(label string, severity string) {
	if os.Getenv("PACTUM_REVIEW_LOOP_REVIEWER_PROCESS") != "1" {
		return
	}
	assertReviewLoopHelperCWD()
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(2)
	}
	for i := 0; i < 50; i++ {
		fmt.Printf("panel_reviewer=%s line=%d stdin_has_reviewer_prompt=%t\n", label, i, strings.Contains(string(stdin), "# Reviewer Prompt"))
	}
	mode := os.Getenv("PACTUM_REVIEW_LOOP_REVIEWER_MODE")
	switch mode {
	case "panel_clean":
	case "panel_duplicate":
		fmt.Print(reviewerStructuredOutput([]map[string]any{
			reviewLoopFixtureFindingWithSeverity("panel reviewers found a shared issue", 77, severity),
		}))
	default:
		fmt.Print(reviewerStructuredOutput([]map[string]any{
			reviewLoopFixtureFindingWithSeverity("panel reviewers found a shared issue", 77, severity),
		}))
	}
	os.Exit(0)
}

func TestReviewLoopFixerHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_REVIEW_LOOP_FIXER_PROCESS") != "1" {
		return
	}
	assertReviewLoopHelperCWD()
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("stdin_has_review_fix_prompt=%t\n", strings.Contains(string(stdin), "# Review Fix Prompt"))
	switch os.Getenv("PACTUM_REVIEW_LOOP_FIXER_MODE") {
	case "append_readme":
		attempt := nextReviewLoopHelperAttempt("PACTUM_REVIEW_LOOP_FIXER_SEQUENCE_FILE")
		file, err := os.OpenFile("README.md", os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fixer open error: %v\n", err)
			os.Exit(2)
		}
		if _, err := fmt.Fprintf(file, "fixer-change=%d\n", attempt); err != nil {
			_ = file.Close()
			fmt.Fprintf(os.Stderr, "fixer write error: %v\n", err)
			os.Exit(2)
		}
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "fixer close error: %v\n", err)
			os.Exit(2)
		}
	case "fix_f001":
		fmt.Print(reviewFixStructuredOutput([]map[string]any{
			{"finding_id": "f_001", "outcome": "fixed", "note": "fixed by loop fixer"},
		}))
	case "rebut_f001":
		fmt.Print(reviewFixStructuredOutput([]map[string]any{
			{"finding_id": "f_001", "outcome": "rebutted", "note": "fixture false positive"},
		}))
	}
	os.Exit(0)
}

func assertReviewLoopHelperCWD() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cwd error: %v\n", err)
		os.Exit(2)
	}
	expectedCWD := os.Getenv("PACTUM_REVIEW_LOOP_REVIEWER_EXPECTED_CWD")
	if expectedCWD == "" {
		expectedCWD = os.Getenv("PACTUM_REVIEW_LOOP_FIXER_EXPECTED_CWD")
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	if resolved, err := filepath.EvalSymlinks(expectedCWD); err == nil {
		expectedCWD = resolved
	}
	fmt.Printf("cwd_is_repo=%t\n", cwd == expectedCWD)
}

func nextReviewLoopReviewerAttempt() int {
	return nextReviewLoopHelperAttempt("PACTUM_REVIEW_LOOP_REVIEWER_SEQUENCE_FILE")
}

func nextReviewLoopHelperAttempt(sequenceFileEnv string) int {
	path := os.Getenv(sequenceFileEnv)
	if path == "" {
		return 1
	}
	current := 0
	if data, err := os.ReadFile(path); err == nil {
		if parsed, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			current = parsed
		}
	}
	next := current + 1
	if err := os.WriteFile(path, []byte(strconv.Itoa(next)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sequence write error: %v\n", err)
		os.Exit(2)
	}
	return next
}
