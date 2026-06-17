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
	reviewLoopReviewerName = "loop-reviewer"
	// The panel members are named after the two engines so they infer to
	// distinct engines (engine-keyed resolution cannot tell two same-engine
	// reviewers apart); the test registry overrides each engine's reviewer
	// descriptor with the matching helper process.
	reviewLoopPanelLowName  = "codex"
	reviewLoopPanelHighName = "claude"
	reviewLoopFixerName     = "loop-fixer"
)

func TestReviewLoopFindingsThenCleanUsesConfigMaxRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	setReviewLoopMaxRoundsConfig(t, paths, 2)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "findings_then_clean")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
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
	// The reviewer is recorded by registry name; the fixer agent comes from the
	// fix result document, which records the engine inferred from the entry's
	// model.
	if summary.Reviewer != reviewLoopReviewerName || summary.Agent != testAgentEngine(reviewLoopFixerName) {
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
	assertFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(1)).ResultJSON)
	assertFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(2)).ResultJSON)
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_002").ResultJSON)

	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || findings[0].Source != "reviewer_proposal" || findings[0].Status != "open" {
		t.Fatalf("loop should accept first reviewer proposal into one open finding: %#v", findings)
	}
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Status == "approved" || review.Approval.ApprovedAt != nil || review.Approval.ApprovedBy != nil {
		t.Fatalf("review run must not auto-approve review: %#v", review)
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
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "findings_then_clean")
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "fix_f001")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "2", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	// The only finding is blocking; once the fixer resolves it the loop has no
	// open blocking findings left and converges resolved in the same round.
	if summary.TerminalReason != "resolved" || len(summary.Rounds) != 1 {
		t.Fatalf("fix outcome loop summary mismatch: %#v", summary)
	}
	if got := summary.Rounds[0]; got.OpenFindings != 0 || got.OpenBlockingFindings != 0 || got.FixerAttemptID != "attempt_001" || got.FixOutcomesResolved != 1 || got.FixOutcomesRebutted != 0 || got.FixOutcomesBlocked != 0 {
		t.Fatalf("round 1 should reflect fixed outcome, no open blocking findings, and resolve: %#v", got)
	}
	resolutions := readReviewResolutions(t, runPaths.ReviewResolutionsJSONL)
	if len(resolutions) != 1 || resolutions[0].Outcome != "fixed" || resolutions[0].Source != "review_fix" {
		t.Fatalf("fix outcome resolution mismatch: %#v", resolutions)
	}
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(2)).ResultJSON)
}

func TestReviewLoopResolvesWhenBlockingFindingRebutted(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "rebut_f001")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "5", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	// A single blocking finding the fixer rebuts (false positive) clears the only
	// open blocking finding, so the loop resolves immediately instead of churning
	// to max_rounds.
	if summary.TerminalReason != "resolved" || len(summary.Rounds) != 1 {
		t.Fatalf("rebutted blocking finding should resolve in one round, not max_rounds: %#v", summary)
	}
	if got := summary.Rounds[0]; got.ProposalsAccepted != 1 || got.FixOutcomesRebutted != 1 || got.OpenFindings != 0 || got.OpenBlockingFindings != 0 {
		t.Fatalf("rebut round summary mismatch: %#v", got)
	}
	resolutions := readReviewResolutions(t, runPaths.ReviewResolutionsJSONL)
	if len(resolutions) != 1 || resolutions[0].Outcome != "rebutted" || resolutions[0].Source != "review_fix" {
		t.Fatalf("rebut resolution mismatch: %#v", resolutions)
	}
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_002").ResultJSON)
}

func TestReviewLoopResolvesNonBlockingFindingsWithoutFixer(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "non_blocking_finding")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	// The accepted finding is advisory (non-blocking): it is recorded and stays
	// open, but it leaves no blocking work, so the loop resolves without ever
	// invoking the fixer.
	if summary.TerminalReason != "resolved" || len(summary.Rounds) != 1 {
		t.Fatalf("non-blocking finding should resolve without a fixer round: %#v", summary)
	}
	if got := summary.Rounds[0]; got.ProposalsAccepted != 1 || got.OpenFindings != 1 || got.OpenBlockingFindings != 0 || got.FixerAttemptID != "" {
		t.Fatalf("non-blocking resolved round summary mismatch: %#v", got)
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || findings[0].Blocking || findings[0].Status != "open" {
		t.Fatalf("advisory finding should be recorded and stay open: %#v", findings)
	}
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
}

func TestReviewRunNoFixSinglePanelPassAcceptsFindingsAndStops(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
	assertNoFile(t, runPaths.ReviewJSON)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--no-fix", "--max-rounds", "1", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run --no-fix exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	var response struct {
		reviewLoopSummaryDocument
		Next []string `json:"next"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.TerminalReason != "findings_open" || len(response.Rounds) != 1 {
		t.Fatalf("no-fix run should stop as findings_open after one round: %#v", response.reviewLoopSummaryDocument)
	}
	if got := response.Rounds[0]; got.ProposalsAccepted != 1 || got.OpenBlockingFindings != 1 || got.FixerAttemptID != "" {
		t.Fatalf("no-fix round summary mismatch: %#v", got)
	}
	// The blocking findings await the human in review show.
	if !equalStrings(response.Next, []string{"pactum review show " + runID}) {
		t.Fatalf("no-fix next = %v, want review show", response.Next)
	}
	// The review was scaffolded implicitly, the proposal accepted, and no fixer ran.
	assertFile(t, runPaths.ReviewJSON)
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || !findings[0].Blocking || findings[0].Source != "reviewer_proposal" {
		t.Fatalf("no-fix run should accept the proposal into findings: %#v", findings)
	}
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
}

func TestReviewRunNoFixStopsAfterFirstRoundWithOpenBlockingFindings(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--no-fix", "--max-rounds", "2", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run --no-fix exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	// Nothing can change the tree without the fixer, so reviewer-only rounds
	// must not churn to the round cap: the run stops after round 1.
	summary := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if summary.TerminalReason != "findings_open" || len(summary.Rounds) != 1 {
		t.Fatalf("no-fix run should stop after the first blocking round: %#v", summary)
	}
	assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(2)).ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
}

func TestReviewLoopStopsAtMaxRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "terminal reason: max_rounds") || !strings.Contains(got, "rounds: 1/1") {
		t.Fatalf("human loop output mismatch:\n%s", got)
	}
	summary := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if summary.TerminalReason != "max_rounds" || len(summary.Rounds) != 1 || summary.Rounds[0].OpenFindings != 1 {
		t.Fatalf("max rounds summary mismatch: %#v", summary)
	}
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(2)).ResultJSON)
}

func TestReviewLoopDedupsReproposedOpenFindingAcrossRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "4", "--patience", "2", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
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
	assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(3)).ResultJSON)
}

func TestReviewLoopAcceptsNewFindingInLaterRound(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_new_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "2", "--patience", "3", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
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
	// Loop accepts are automatic decisions: they carry only their source —
	// honest provenance, never the "manual" the CLI verb records — and no
	// decided_by principal (that is reserved for the explicit CLI verbs).
	if decisions[0].Source != "review_loop" || decisions[1].Source != "review_loop" {
		t.Fatalf("loop decisions must record source review_loop: %#v", decisions)
	}
	if decisions[0].DecidedBy != "" || decisions[1].DecidedBy != "" {
		t.Fatalf("loop decisions must not record decided_by: %#v", decisions)
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
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "same_round_duplicates")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
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
	setReviewPanelConfig(t, paths, reviewLoopPanelLowName, reviewLoopPanelHighName)
	app = configureReviewLoopPanelHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "panel_duplicate")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.Reviewer != reviewLoopPanelLowName+","+reviewLoopPanelHighName || !sameStringSlice(summary.Reviewers, []string{reviewLoopPanelLowName, reviewLoopPanelHighName}) {
		t.Fatalf("summary should record full reviewer panel: %#v", summary)
	}
	// Two panel members times five lenses: ten attempts in the round.
	wantAttempts := 2 * len(reviewLenses)
	if len(summary.Rounds) != 1 || len(summary.Rounds[0].ReviewerAttemptIDs) != wantAttempts || summary.Rounds[0].ReviewerAttemptID == "" {
		t.Fatalf("round should record member-times-lens reviewer attempts: %#v", summary.Rounds)
	}
	// Every lens of both members emits the same finding; all ten cross-member,
	// cross-lens duplicates collapse onto one stored finding.
	if got := summary.Rounds[0]; got.ProposalsCreated != wantAttempts || got.ProposalsAccepted != 1 || got.OpenFindings != 1 {
		t.Fatalf("panel duplicate round mismatch: %#v", got)
	}
	if len(summary.Rounds[0].ReviewerAttempts) != wantAttempts {
		t.Fatalf("round should surface the lens per attempt: %#v", summary.Rounds[0].ReviewerAttempts)
	}
	seenMemberLenses := map[string]bool{}
	for _, ref := range summary.Rounds[0].ReviewerAttempts {
		if seenMemberLenses[ref.Reviewer+"/"+ref.Lens] {
			t.Fatalf("duplicate member/lens attempt ref: %#v", summary.Rounds[0].ReviewerAttempts)
		}
		seenMemberLenses[ref.Reviewer+"/"+ref.Lens] = true
		attemptPaths := reviewerAttemptPaths(runPaths, ref.AttemptID)
		assertFile(t, attemptPaths.ResultJSON)
		// Each attempt's request points at its own per-member, per-lens prompt.
		var request reviewerRequestDocument
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.RequestJSON)), &request))
		wantPrompt := "review/reviewer-prompt-" + ref.Reviewer + "-" + ref.Lens + ".md"
		if request.Lens != ref.Lens || request.Artifacts.ReviewerPrompt != wantPrompt {
			t.Fatalf("attempt %s request prompt mismatch (want %s): %#v", ref.AttemptID, wantPrompt, request)
		}
	}
	for _, member := range []string{reviewLoopPanelLowName, reviewLoopPanelHighName} {
		for _, lens := range reviewLenses {
			if !seenMemberLenses[member+"/"+lens.Key] {
				t.Fatalf("missing attempt for member %s lens %s: %#v", member, lens.Key, seenMemberLenses)
			}
			assertFile(t, reviewerLensPromptPath(runPaths, member, lens))
		}
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || findings[0].Severity != "critical" {
		t.Fatalf("duplicate severity should upgrade stored finding to critical: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != wantAttempts || decisions[0].Decision != "accepted" {
		t.Fatalf("panel duplicate decisions mismatch: %#v", decisions)
	}
	for _, decision := range decisions[1:] {
		if decision.Decision != "duplicate" || decision.FindingID != "f_001" {
			t.Fatalf("panel duplicate decisions mismatch: %#v", decisions)
		}
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "reviewer_attempt_started") != wantAttempts || countEvents(eventTypes, "review_finding_severity_upgraded") != 1 {
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
	// The loop helpers (not the panel helpers) back this registry: the panel is
	// configured but must not run, and the explicit reviewer resolves the
	// loop-reviewer helper on its inferred engine.
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewPanelConfig(t, paths, reviewLoopPanelLowName, reviewLoopPanelHighName)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.Reviewer != reviewLoopReviewerName || len(summary.Reviewers) != 0 {
		t.Fatalf("explicit reviewer should disable configured panel: %#v", summary)
	}
	// A single explicit reviewer still fans out into the five lens attempts,
	// but not into the panel's member-times-lens count.
	if len(summary.Rounds) != 1 || summary.Rounds[0].ReviewerAttemptID == "" || len(summary.Rounds[0].ReviewerAttemptIDs) != len(reviewLenses) {
		t.Fatalf("explicit reviewer should run one lens fan-out: %#v", summary.Rounds)
	}
	assertFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_001").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, fmt.Sprintf("reviewer_attempt_%03d", len(reviewLenses)+1)).ResultJSON)
}

func TestReviewLoopCleanPanelRoundTerminates(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	setReviewPanelConfig(t, paths, reviewLoopPanelLowName, reviewLoopPanelHighName)
	app = configureReviewLoopPanelHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "panel_clean")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--agent", reviewLoopFixerName, "--max-rounds", "4", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "clean_round" || len(summary.Rounds) != 1 {
		t.Fatalf("clean panel should terminate after one round: %#v", summary)
	}
	if len(summary.Rounds[0].ReviewerAttemptIDs) != 2*len(reviewLenses) || summary.Rounds[0].ProposalsCreated != 0 || summary.Rounds[0].ProposalsAccepted != 0 {
		t.Fatalf("clean panel round mismatch: %#v", summary.Rounds[0])
	}
	for _, attemptID := range summary.Rounds[0].ReviewerAttemptIDs {
		assertFile(t, reviewerAttemptPaths(runPaths, attemptID).ResultJSON)
	}
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
}

func TestResolveReviewLoopReviewersPanelAllowsSameBuiltInTwice(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths,
		agentRegistryEntry{Name: "fable", Model: "claude-fable-5"},
		agentRegistryEntry{Name: "sonnet", Model: "claude-sonnet-4-6"},
	)
	setReviewPanelConfig(t, paths, "fable", "sonnet")

	var stdout bytes.Buffer
	context, ok, err := app.loadReviewContext(&stdout, runID)
	assertNoError(t, err)
	if !ok {
		t.Fatal("expected review context")
	}
	reviewers, err := app.resolveReviewLoopReviewers(context, "")
	assertNoError(t, err)
	if len(reviewers) != 2 || reviewers[0].Name != "fable" || reviewers[1].Name != "sonnet" {
		t.Fatalf("panel should run both claude-backed entries as separate members: %#v", reviewers)
	}
	if reviewers[0].Agent.Name != "claude" || reviewers[1].Agent.Name != "claude" {
		t.Fatalf("both panel members should run on the claude built-in: %#v", reviewers)
	}
	if reviewers[0].ModelSpec.Model != "claude-fable-5" || reviewers[1].ModelSpec.Model != "claude-sonnet-4-6" {
		t.Fatalf("each panel member should carry its own entry pins: %#v", reviewers)
	}
}

func TestReviewLoopUnknownPanelReviewerFailsClearly(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runReviewCommand(t, app, "gate", "run", runID)
	setReviewPanelConfig(t, paths, "missing-reviewer", reviewLoopPanelHighName)
	app = configureReviewLoopPanelHelpers(t, app, paths)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--agent", reviewLoopFixerName, "--max-rounds", "1"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("unknown panel reviewer exited %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, `"missing-reviewer"`) || !strings.Contains(got, "pipeline.code_review") {
		t.Fatalf("unknown panel reviewer error mismatch:\n%s", got)
	}
}

func TestReviewLoopReacceptsResolvedFindingWhenReproposed(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	appendReviewLoopFindingForTest(t, runPaths, runID, "f_001", reviewLoopFixtureFindingCore("loop reviewer found a fixable issue", 42))
	appendReviewLoopResolutionForTest(t, runPaths, runID, "r_001", "f_001")
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
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
	appendReviewLoopFindingForTest(t, runPaths, runID, "f_001", reviewLoopFixtureFindingCore("loop reviewer found a fixable issue", 42))
	appendReviewLoopResolutionWithOutcomeForTest(t, runPaths, runID, "r_001", "f_001", "rebutted")
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
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
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_new_findings")
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_SEQUENCE_FILE", filepath.Join(stateDir, "fixer-sequence"))
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "append_readme")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--patience", "2", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
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
	assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(4)).ResultJSON)
}

func TestReviewLoopRequiresConsecutiveCleanRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "clean_findings_clean_clean")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "4", "--clean-rounds", "2", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
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
	assertFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(4)).ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(5)).ResultJSON)
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_002").ResultJSON)
}

func TestReviewLoopUnparsedFindingsIsNotCleanRound(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "malformed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "2", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
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
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
	t.Setenv("PACTUM_GATE_HELPER_EXIT", "7")

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{
		Reviewer:   reviewLoopReviewerName,
		Agent:      reviewLoopFixerName,
		MaxRounds:  3,
		JSONOutput: true,
	})
	if err != nil {
		t.Fatalf("review run should stop cleanly on failed gate, got error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
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
	assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(2)).ResultJSON)
}

func TestReviewLoopStopsWithGateFailedWhenFixerViolatesPathScope(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, map[string]any{"goal": "add deterministic gate", "paths_in_scope": []string{"internal/app/**"}}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
	t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "append_readme")

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{
		Reviewer:   reviewLoopReviewerName,
		Agent:      reviewLoopFixerName,
		MaxRounds:  3,
		JSONOutput: true,
	})
	if err != nil {
		t.Fatalf("review run should stop cleanly on blocked scope gate, got error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
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
	assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(2)).ResultJSON)
}

func TestReviewLoopGateInfrastructureErrorStillReturnsError(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runReviewCommand(t, app, "gate", "run", runID)
	mustWriteFile(t, executionAttemptPaths(runPaths, "attempt_001").ResultJSON, "{")
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{
		Reviewer:   reviewLoopReviewerName,
		Agent:      reviewLoopFixerName,
		MaxRounds:  1,
		JSONOutput: true,
	})
	if err == nil {
		t.Fatalf("review run should return gate infrastructure error")
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
		t.Fatalf("errored review run should not emit summary JSON stdout:\n%s", stdout.String())
	}
}

func TestReviewLoopStreamsSubRunOutputToStderrWithCleanStdout(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runReviewCommand(t, app, "gate", "run", runID)
	setReviewLoopMaxRoundsConfig(t, paths, 2)
	app = configureReviewLoopHelpers(t, app, paths)
	setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "findings_then_clean")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
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

// TestReviewLoopPartialLensFailureProceedsWithWarning verifies that a single
// failing reviewer/lens attempt does not abort the round: the round completes
// using the remaining successful lens results, and the skipped attempt is
// recorded with reviewer, lens, and reason in the round summary artifact.
func TestReviewLoopPartialLensFailureProceedsWithWarning(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4-6"})

	// Fail one lens; the remaining four succeed and produce a clean round.
	failingPrompt := runArtifactRepoRel(runID, reviewerLensPromptArtifact("claude", reviewLenses[1]))
	tr := &staggerTransport{}
	tr.failFor = failingPrompt
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	if err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "claude"}); err != nil {
		t.Fatalf("partial lens failure must not abort the round: %v", err)
	}
	summary := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if summary.TerminalReason != "clean_round" || len(summary.Rounds) != 1 {
		t.Fatalf("partial failure should reach clean_round: %#v", summary)
	}
	round := summary.Rounds[0]
	if len(round.SkippedLenses) != 1 {
		t.Fatalf("skipped lenses = %d, want 1: %#v", len(round.SkippedLenses), round)
	}
	s := round.SkippedLenses[0]
	if s.Reviewer != "claude" || s.Lens != reviewLenses[1].Key || s.Reason == "" {
		t.Fatalf("skipped lens mismatch: %#v", s)
	}
	// The 4 succeeding lens attempts are collected; the skipped one is excluded.
	if got := len(round.ReviewerAttemptIDs); got != len(reviewLenses)-1 {
		t.Fatalf("attempt IDs = %d, want %d", got, len(reviewLenses)-1)
	}
	// The human-readable output surfaces the skipped lens.
	humanOut := stdout.String()
	if !strings.Contains(humanOut, "skipped:") || !strings.Contains(humanOut, s.Lens) {
		t.Fatalf("human output should mention skipped lens:\n%s", humanOut)
	}
}

// TestReviewLoopAllLensesFailingReturnsError verifies that when every
// reviewer/lens attempt fails the round returns an error (the fully-unavailable
// panel case) instead of silently treating it as a clean pass.
func TestReviewLoopAllLensesFailingReturnsError(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-fable-5"})

	app.AgentTransport = alwaysErrTransport{err: errors.New("Claude Fable 5 is currently unavailable")}

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "claude"})
	if err == nil {
		t.Fatalf("all lenses failing must return an error")
	}
	if !strings.Contains(err.Error(), "reviewer claude lens") {
		t.Fatalf("error should identify the reviewer and lens: %v", err)
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("error should include the failure cause: %v", err)
	}
	// The summary artifact records the terminal reason as an error.
	summary := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if summary.TerminalReason != "error" {
		t.Fatalf("all-fail summary terminal reason = %q, want error: %#v", summary.TerminalReason, summary)
	}
}

// alwaysErrTransport is a test-only transport that rejects every attempt.
type alwaysErrTransport struct{ err error }

func (tr alwaysErrTransport) Run(req agents.RunRequest) (agents.RunResult, error) {
	return agents.RunResult{}, tr.err
}

// TestReviewLoop is the comprehensive sub-test suite for the loop engine
// integration. Each sub-test targets one terminal-reason path or one
// observable behavioral property of the ported ReviewRun implementation.
func TestReviewLoop(t *testing.T) {
	// (a) A fully-clean reviewer round settles the loop with terminal_reason="clean_round".
	t.Run("settled_becomes_clean_round", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_clean")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--max-rounds", "3", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "clean_round" || len(summary.Rounds) != 1 {
			t.Fatalf("always_clean should settle in one round: %#v", summary)
		}
		if got := summary.Rounds[0]; got.CleanStreak != 1 || got.ProposalsCreated != 0 || len(got.Warnings) != 0 {
			t.Fatalf("clean round summary mismatch: %#v", got)
		}
		assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	})

	// (b) The loop requires Settle consecutive clean rounds before stopping.
	t.Run("settle_requires_consecutive_clean_rounds", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_clean")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--max-rounds", "5", "--clean-rounds", "2", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "clean_round" || summary.CleanRoundsRequired != 2 || len(summary.Rounds) != 2 {
			t.Fatalf("settle=2 should require 2 consecutive clean rounds: %#v", summary)
		}
		if summary.Rounds[0].CleanStreak != 1 || summary.Rounds[1].CleanStreak != 2 {
			t.Fatalf("clean streak should increment consecutively: %#v", summary.Rounds)
		}
	})

	// (c) With --no-fix the loop exits as findings_open after the first blocking round,
	// not at the round cap.
	t.Run("no_fix_exits_as_findings_open_not_max_rounds", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--no-fix", "--max-rounds", "3", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run --no-fix exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "findings_open" || len(summary.Rounds) != 1 {
			t.Fatalf("no-fix should stop as findings_open after round 1: %#v", summary)
		}
		if got := summary.Rounds[0]; got.ProposalsAccepted != 1 || got.OpenBlockingFindings != 1 || got.FixerAttemptID != "" {
			t.Fatalf("no-fix round summary mismatch: %#v", got)
		}
		assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(2)).ResultJSON)
		assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	})

	// (d) Fixer resolves all blocking findings; the loop exits as "resolved" immediately.
	t.Run("resolved_when_fixer_fixes_all_blocking", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
		t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "fix_f001")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "resolved" || len(summary.Rounds) != 1 {
			t.Fatalf("fixer fixing all blocking should resolve in one round: %#v", summary)
		}
		if got := summary.Rounds[0]; got.OpenBlockingFindings != 0 || got.FixOutcomesResolved != 1 || got.FixerAttemptID == "" {
			t.Fatalf("resolved round summary mismatch: %#v", got)
		}
		assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(2)).ResultJSON)
	})

	// (e) Only advisory (non-blocking) findings: loop exits as "resolved" without invoking
	// the fixer.
	t.Run("resolved_when_only_advisory_findings", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "non_blocking_finding")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "resolved" || len(summary.Rounds) != 1 {
			t.Fatalf("advisory-only findings should resolve without fixer: %#v", summary)
		}
		if got := summary.Rounds[0]; got.ProposalsAccepted != 1 || got.OpenFindings != 1 || got.OpenBlockingFindings != 0 || got.FixerAttemptID != "" {
			t.Fatalf("advisory-only resolved round mismatch: %#v", got)
		}
		assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	})

	// (f) Malformed reviewer output (no parseable proposals, only warnings) stops the
	// loop as reviewer_findings_unparsed.
	t.Run("reviewer_unparsed_when_output_is_malformed", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "malformed")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "reviewer_findings_unparsed" || len(summary.Rounds) != 1 {
			t.Fatalf("malformed reviewer output should stop as reviewer_findings_unparsed: %#v", summary)
		}
		if got := summary.Rounds[0]; got.ProposalsCreated != 0 || len(got.Warnings) == 0 || got.FixerAttemptID != "" {
			t.Fatalf("malformed round summary mismatch: %#v", got)
		}
		assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	})

	// (g) A mix of valid blocking proposals and malformed entries does NOT produce
	// reviewer_findings_unparsed: the valid proposals are accepted, the fixer runs,
	// and the warnings appear alongside the round's proposal counts.
	t.Run("mixed_valid_and_malformed_does_not_stop_as_unparsed", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "findings_and_malformed")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "max_rounds" || len(summary.Rounds) != 1 {
			t.Fatalf("valid+malformed should run fixer and reach max_rounds: %#v", summary)
		}
		got := summary.Rounds[0]
		if got.ProposalsAccepted != 1 || len(got.Warnings) == 0 || got.FixerAttemptID == "" {
			t.Fatalf("valid+malformed round should record accepted proposal, warning, and fixer: %#v", got)
		}
		assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
		assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_002").ResultJSON)
	})

	// (h) A gate process failure (non-zero exit) stops the loop cleanly as gate_failed
	// without returning an error to the caller.
	t.Run("gate_failed_stops_loop_cleanly", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		t.Setenv("PACTUM_GATE_HELPER_PROCESS", "1")
		app, paths, runID := setupGatePreparedRun(t, root, []string{gateValidationCommandForTest()}, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
		t.Setenv("PACTUM_GATE_HELPER_EXIT", "7")

		var stdout, stderr bytes.Buffer
		err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{
			Reviewer:   reviewLoopReviewerName,
			Agent:      reviewLoopFixerName,
			MaxRounds:  3,
			JSONOutput: true,
		})
		if err != nil {
			t.Fatalf("gate_failed should not propagate as error: %v", err)
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "gate_failed" || len(summary.Rounds) != 1 {
			t.Fatalf("gate_failed summary mismatch: %#v", summary)
		}
		if got := summary.Rounds[0]; got.GateStatus != "failed" || got.GateReportArtifact != gateReportArtifact || got.FixerAttemptID == "" {
			t.Fatalf("gate_failed round summary mismatch: %#v", got)
		}
		assertNoFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(2)).ResultJSON)
	})

	// (i) When blocking findings persist and max_rounds is reached the loop stops as
	// max_rounds.
	t.Run("max_rounds_reached_when_blocking_persist", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "max_rounds" || len(summary.Rounds) != 1 {
			t.Fatalf("single-round blocking run should reach max_rounds: %#v", summary)
		}
		if got := summary.Rounds[0]; got.OpenBlockingFindings == 0 || got.FixerAttemptID == "" {
			t.Fatalf("max_rounds round should still have open blocking findings: %#v", got)
		}
	})

	// (j) When the fixer never changes the working tree, the stale streak hits patience
	// and the loop exits as stalemate.
	t.Run("stalemate_when_fixer_never_changes_tree", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "5", "--patience", "2", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "stalemate" || len(summary.Rounds) != 2 {
			t.Fatalf("fixer with no tree changes should stalemate after patience=2 rounds: %#v", summary)
		}
		if summary.Rounds[0].UnchangedFingerprintStreak != 1 || summary.Rounds[1].UnchangedFingerprintStreak != 2 {
			t.Fatalf("unchanged fingerprint streak should increment: %#v", summary.Rounds)
		}
	})

	// (k) A finding reproposed in a later round is deduped against the existing open
	// finding; the later round records ProposalsCreated=1 but ProposalsAccepted=0.
	t.Run("open_finding_deduped_in_later_round", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "2", "--patience", "5", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "max_rounds" || len(summary.Rounds) != 2 {
			t.Fatalf("two rounds expected: %#v", summary)
		}
		if got := summary.Rounds[0]; got.ProposalsCreated != 1 || got.ProposalsAccepted != 1 {
			t.Fatalf("round 1 should accept first proposal: %#v", got)
		}
		if got := summary.Rounds[1]; got.ProposalsCreated != 1 || got.ProposalsAccepted != 0 {
			t.Fatalf("round 2 should dedup re-proposed open finding: %#v", got)
		}
		findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
		if len(findings) != 1 {
			t.Fatalf("re-proposed open finding should remain one finding: %#v", findings)
		}
	})

	// (l) A rebutted finding reproposed by the reviewer is treated as a duplicate and
	// does not produce a new open finding; the loop exits resolved (no blocking open).
	t.Run("rebutted_finding_reproposal_is_duplicate", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		appendReviewLoopFindingForTest(t, runPaths, runID, "f_001", reviewLoopFixtureFindingCore("loop reviewer found a fixable issue", 42))
		appendReviewLoopResolutionWithOutcomeForTest(t, runPaths, runID, "r_001", "f_001", "rebutted")
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "1", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "resolved" || len(summary.Rounds) != 1 {
			t.Fatalf("rebutted reproposal should resolve immediately: %#v", summary)
		}
		if got := summary.Rounds[0]; got.ProposalsAccepted != 0 || got.OpenFindings != 0 || got.FixerAttemptID != "" {
			t.Fatalf("rebutted reproposal round mismatch: %#v", got)
		}
		decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
		if len(decisions) != 1 || decisions[0].Decision != "duplicate" || decisions[0].Reason != "matches rebutted finding" {
			t.Fatalf("rebutted reproposal should record duplicate decision: %#v", decisions)
		}
	})

	// (m) The clean streak resets when a dirty round (with findings) interrupts a prior
	// streak; it must restart from zero before the loop can settle.
	t.Run("clean_streak_resets_on_dirty_round", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "clean_findings_clean_clean")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "4", "--clean-rounds", "2", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "clean_round" || summary.CleanRoundsRequired != 2 || len(summary.Rounds) != 4 {
			t.Fatalf("clean_streak_reset summary mismatch: %#v", summary)
		}
		wantStreaks := []int{1, 0, 1, 2}
		for i, want := range wantStreaks {
			if got := summary.Rounds[i].CleanStreak; got != want {
				t.Fatalf("round %d clean streak = %d, want %d: %#v", i+1, got, want, summary.Rounds[i])
			}
		}
	})

	// (n) A fixer that makes progress (changes the working tree) resets the stale streak,
	// preventing a premature stalemate that would otherwise fire at round 2.
	t.Run("progress_prevents_premature_stalemate", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
		t.Setenv("PACTUM_REVIEW_LOOP_FIXER_SEQUENCE_FILE", filepath.Join(stateDir, "fixer-sequence"))
		t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "change_on_second_attempt")

		var stdout, stderr bytes.Buffer
		// patience=2: without the round-2 tree change, stalemate would fire at round 2.
		// With it the stale streak resets to 0 after round 2 and stalemate is delayed to round 4.
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "4", "--patience", "2", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "stalemate" || len(summary.Rounds) != 4 {
			t.Fatalf("progress should delay stalemate to round 4: terminal=%q rounds=%d", summary.TerminalReason, len(summary.Rounds))
		}
		wantStreaks := []int{1, 0, 1, 2}
		for i, want := range wantStreaks {
			if got := summary.Rounds[i].UnchangedFingerprintStreak; got != want {
				t.Fatalf("round %d unchanged fingerprint streak = %d, want %d: %#v", i+1, got, want, summary.Rounds[i])
			}
		}
	})

	// (k) The stdout JSON response and summary artifact contain exactly the
	// always-present fields listed in the schema-preservation criterion. Added or
	// removed JSON keys are caught by parsing into map[string]json.RawMessage
	// rather than a struct (which silently ignores unknown/missing fields).
	t.Run("exact_json_schema_top_level_and_per_round", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_clean")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--max-rounds", "2", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}

		// Always-present top-level fields for the stdout response.
		requiredTopLevel := []string{
			"schema", "run_id", "started_at", "finished_at",
			"max_rounds", "stalemate_patience", "clean_rounds_required",
			"terminal_reason", "rounds", "artifacts", "next",
		}
		// Optional omitempty fields allowed in stdout but not required.
		allowedTopLevel := map[string]bool{
			"schema": true, "run_id": true, "started_at": true, "finished_at": true,
			"max_rounds": true, "stalemate_patience": true, "clean_rounds_required": true,
			"terminal_reason": true, "rounds": true, "artifacts": true, "next": true,
			"reviewer": true, "reviewers": true, "agent": true,
		}
		// Always-present per-round fields.
		requiredRound := []string{
			"round", "reviewer_attempt_id", "proposals_created", "proposals_accepted",
			"open_findings", "open_blocking_findings", "clean_streak", "unchanged_fingerprint_streak",
		}
		// Optional omitempty per-round fields.
		allowedRound := map[string]bool{
			"round": true, "reviewer_attempt_id": true, "proposals_created": true,
			"proposals_accepted": true, "open_findings": true, "open_blocking_findings": true,
			"clean_streak": true, "unchanged_fingerprint_streak": true,
			"reviewer_attempt_ids": true, "reviewer_attempts": true,
			"warnings": true, "skipped_lenses": true, "working_tree_fingerprint": true,
			"fixer_attempt_id": true, "fix_outcomes_resolved": true,
			"fix_outcomes_rebutted": true, "fix_outcomes_blocked": true,
			"gate_status": true, "gate_report_artifact": true,
		}

		var rawResp map[string]json.RawMessage
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &rawResp))
		for _, key := range requiredTopLevel {
			if _, ok := rawResp[key]; !ok {
				t.Errorf("stdout JSON missing required top-level key: %s", key)
			}
		}
		for key := range rawResp {
			if !allowedTopLevel[key] {
				t.Errorf("stdout JSON has unexpected top-level key: %s", key)
			}
		}

		var roundsRaw []map[string]json.RawMessage
		assertNoError(t, json.Unmarshal(rawResp["rounds"], &roundsRaw))
		if len(roundsRaw) == 0 {
			t.Fatal("rounds array is empty, cannot check per-round schema")
		}
		for _, roundEntry := range roundsRaw {
			for _, key := range requiredRound {
				if _, ok := roundEntry[key]; !ok {
					t.Errorf("round entry missing required key: %s", key)
				}
			}
			for key := range roundEntry {
				if !allowedRound[key] {
					t.Errorf("round entry has unexpected key: %s", key)
				}
			}
		}

		// Artifact must have the same top-level keys except no "next".
		var rawArtifact map[string]json.RawMessage
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, runPaths.ReviewLoopSummaryJSON)), &rawArtifact))
		if _, ok := rawArtifact["next"]; ok {
			t.Error("summary artifact must not contain 'next' field")
		}
		for _, key := range requiredTopLevel {
			if key == "next" {
				continue
			}
			if _, ok := rawArtifact[key]; !ok {
				t.Errorf("summary artifact missing required top-level key: %s", key)
			}
		}
		allowedArtifact := map[string]bool{}
		for k, v := range allowedTopLevel {
			allowedArtifact[k] = v
		}
		delete(allowedArtifact, "next")
		for key := range rawArtifact {
			if !allowedArtifact[key] {
				t.Errorf("summary artifact has unexpected top-level key: %s", key)
			}
		}
	})

	// (h) Working-tree fingerprint excludes runner-written artifacts: .heurema/
	// directory contents (reviewer attempt files), ledger events, gate reports,
	// and review summary artifacts must not shift the fingerprint.
	t.Run("fingerprint_excludes_runner_artifacts", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_clean")

		// Verify that the relevant runner-written paths are under .heurema/ so the
		// exclusion claim is meaningful.
		relEvents := filepath.ToSlash(strings.TrimPrefix(paths.EventsJSONL, root+string(filepath.Separator)))
		if !strings.HasPrefix(relEvents, ".heurema/") {
			t.Fatalf("ledger events not under .heurema/: %s", relEvents)
		}
		relSummary := filepath.ToSlash(strings.TrimPrefix(runPaths.ReviewLoopSummaryJSON, root+string(filepath.Separator)))
		if !strings.HasPrefix(relSummary, ".heurema/") {
			t.Fatalf("review summary not under .heurema/: %s", relSummary)
		}
		relGateReport := filepath.ToSlash(strings.TrimPrefix(runPaths.GateReportJSON, root+string(filepath.Separator)))
		if !strings.HasPrefix(relGateReport, ".heurema/") {
			t.Fatalf("gate report not under .heurema/: %s", relGateReport)
		}

		var stdout, stderr bytes.Buffer
		// Two consecutive clean rounds: round 1 runs the reviewer (writing attempt
		// files under .heurema/pactum/) and appends ledger events; round 2 takes
		// a second fingerprint. If runner artifacts were counted, the fingerprints
		// would differ.
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--max-rounds", "5", "--clean-rounds", "2", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "clean_round" || len(summary.Rounds) != 2 {
			t.Fatalf("expected 2 clean rounds: %#v", summary)
		}
		fp1 := summary.Rounds[0].WorkingTreeFingerprint
		fp2 := summary.Rounds[1].WorkingTreeFingerprint
		if fp1 == "" || fp2 == "" {
			t.Fatalf("both rounds must record a working tree fingerprint: round1=%q round2=%q", fp1, fp2)
		}
		if fp1 != fp2 {
			t.Fatalf("fingerprint changed between clean rounds despite no user-space file changes:\nround 1: %s\nround 2: %s\n(.heurema/ runner artifacts must be excluded)", fp1, fp2)
		}
		// Confirm that reviewer attempt artifacts from round 1 exist on disk before
		// round 2's fingerprint was computed, proving they were present but excluded.
		assertFile(t, reviewerAttemptPaths(runPaths, reviewLoopRoundFirstAttemptID(1)).ResultJSON)
	})

	// (j) review_loop_started and review_loop_finished are both emitted to the
	// ledger for any run that completes normally or via a named sentinel.
	t.Run("lifecycle_events_emitted_for_any_terminal", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_clean")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--max-rounds", "2", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
		startIdx := indexOfEvent(eventTypes, "review_loop_started")
		finishIdx := indexOfEvent(eventTypes, "review_loop_finished")
		if startIdx == -1 {
			t.Fatalf("review_loop_started not in ledger: %v", eventTypes)
		}
		if finishIdx == -1 {
			t.Fatalf("review_loop_finished not in ledger: %v", eventTypes)
		}
		if finishIdx <= startIdx {
			t.Fatalf("review_loop_finished (idx=%d) must follow review_loop_started (idx=%d)", finishIdx, startIdx)
		}
	})

	// (i) For each named-terminal scenario the stdout JSON response contains the
	// correct round numbers and the fields terminal_reason, open_findings,
	// open_blocking_findings, and round verifiable from the raw JSON (not just
	// struct unmarshaling, which silently absorbs missing or renamed fields).
	t.Run("named_terminal_stdout_field_round_numbers", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		// stalemate with patience=2: 2 rounds, each with round=1 and round=2
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "5", "--patience", "2", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}

		// Parse the raw JSON to catch missing or renamed field names.
		var rawResp map[string]json.RawMessage
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &rawResp))
		for _, field := range []string{"terminal_reason", "rounds"} {
			if _, ok := rawResp[field]; !ok {
				t.Errorf("stdout JSON missing top-level field: %s", field)
			}
		}
		var terminalReason string
		assertNoError(t, json.Unmarshal(rawResp["terminal_reason"], &terminalReason))
		if terminalReason != "stalemate" {
			t.Fatalf("expected stalemate terminal, got %q", terminalReason)
		}

		var roundsRaw []map[string]json.RawMessage
		assertNoError(t, json.Unmarshal(rawResp["rounds"], &roundsRaw))
		if len(roundsRaw) != 2 {
			t.Fatalf("expected 2 rounds, got %d", len(roundsRaw))
		}
		for i, roundEntry := range roundsRaw {
			for _, field := range []string{"round", "open_findings", "open_blocking_findings"} {
				if _, ok := roundEntry[field]; !ok {
					t.Errorf("round %d JSON missing field: %s", i+1, field)
				}
			}
			var roundNum int
			assertNoError(t, json.Unmarshal(roundEntry["round"], &roundNum))
			if roundNum != i+1 {
				t.Errorf("round %d entry has round=%d, want %d", i+1, roundNum, i+1)
			}
		}
		_ = paths // used for gate setup above
	})

	// (l) For a run involving reviewer execution, fixer execution, gate execution,
	// and proposal acceptance, the ledger contains every event name required by
	// the contract's ledger-events criterion.
	t.Run("ledger_event_baseline_reviewer_fixer_gate", func(t *testing.T) {
		root := t.TempDir()
		stateDir := t.TempDir()
		app, paths, runID := setupGatePreparedRun(t, root, nil, true)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		runReviewCommand(t, app, "gate", "run", runID)
		app = configureReviewLoopHelpers(t, app, paths)
		// always_findings reviewer + fix_f001 fixer: round 1 accepts a blocking
		// finding, fixer resolves it, gate passes, loop exits resolved.
		setReviewLoopHelperEnv(t, root, filepath.Join(stateDir, "reviewer-sequence"), "always_findings")
		t.Setenv("PACTUM_REVIEW_LOOP_FIXER_MODE", "fix_f001")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", reviewLoopReviewerName, "--agent", reviewLoopFixerName, "--max-rounds", "3", "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
		}
		var summary reviewLoopSummaryDocument
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
		if summary.TerminalReason != "resolved" || len(summary.Rounds) != 1 {
			t.Fatalf("expected resolved in one round: %#v", summary)
		}

		eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
		required := []string{
			"review_loop_started",
			"review_loop_finished",
			"reviewer_attempt_started",
			"reviewer_attempt_finished",
			"review_fix_attempt_started",
			"review_fix_attempt_finished",
			"gate_run_started",
			"gate_run_finished",
			"review_findings_proposed",
			"review_proposal_accepted",
			"review_finding_added",
		}
		for _, want := range required {
			if indexOfEvent(eventTypes, want) == -1 {
				t.Errorf("ledger missing required event: %s", want)
			}
		}
		assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	})
}

// registerReviewLoopAgents adds the loop helper agents to the config registry
// so registry-name resolution finds them. The loop reviewer and fixer both
// infer the claude engine; the injected registries keep them apart by role.
func registerReviewLoopAgents(t *testing.T, paths artifacts.Paths) {
	t.Helper()
	setAgentRegistryConfig(t, paths,
		agentRegistryEntry{Name: "codex", Model: "gpt-5"},
		agentRegistryEntry{Name: "claude", Model: "claude-opus-4-8"},
		agentRegistryEntry{Name: reviewLoopReviewerName, Model: testAgentModel(reviewLoopReviewerName)},
		agentRegistryEntry{Name: reviewLoopFixerName, Model: testAgentModel(reviewLoopFixerName)},
	)
}

// reviewLoopHelperDescriptor routes one engine's role resolution to a review
// loop helper process.
func reviewLoopHelperDescriptor(engine string, helperTest string) agents.AgentDescriptor {
	return agents.AgentDescriptor{
		Name:    engine,
		Command: os.Args[0],
		Args:    []string{"-test.run=" + helperTest, "--"},
		Input:   agents.InputPromptFile,
	}
}

func configureReviewLoopHelpers(t *testing.T, app App, paths artifacts.Paths) App {
	t.Helper()
	registerReviewLoopAgents(t, paths)
	app.AgentRegistry = testAgentRegistryRoles(
		[]agents.AgentDescriptor{
			reviewLoopHelperDescriptor(testAgentEngine(reviewLoopFixerName), "TestReviewLoopFixerHelperProcess"),
		},
		[]agents.AgentDescriptor{
			reviewLoopHelperDescriptor(testAgentEngine(reviewLoopReviewerName), "TestReviewLoopReviewerHelperProcess"),
		},
	)
	return app
}

func configureReviewLoopPanelHelpers(t *testing.T, app App, paths artifacts.Paths) App {
	t.Helper()
	registerReviewLoopAgents(t, paths)
	app.AgentRegistry = testAgentRegistryRoles(
		[]agents.AgentDescriptor{
			reviewLoopHelperDescriptor(testAgentEngine(reviewLoopFixerName), "TestReviewLoopFixerHelperProcess"),
		},
		[]agents.AgentDescriptor{
			reviewLoopHelperDescriptor(testAgentEngine(reviewLoopPanelLowName), "TestReviewLoopPanelLowReviewerHelperProcess"),
			reviewLoopHelperDescriptor(testAgentEngine(reviewLoopPanelHighName), "TestReviewLoopPanelHighReviewerHelperProcess"),
		},
	)
	return app
}

func setReviewLoopMaxRoundsConfig(t *testing.T, paths artifacts.Paths, maxRounds int) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	if config.Pipeline.CodeReview.Loop == nil {
		config.Pipeline.CodeReview.Loop = &loopConfig{}
	}
	config.Pipeline.CodeReview.Loop.Max = maxRounds
	assertNoError(t, writeYAML(paths.Config, config))
}

func setReviewPanelConfig(t *testing.T, paths artifacts.Paths, reviewers ...string) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	config.Pipeline.CodeReview.By = stageBy(append([]string{}, reviewers...))
	assertNoError(t, writeYAML(paths.Config, config))
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

// reviewLoopRoundFirstAttemptID is the first lens attempt ID of the given
// round for a single-reviewer loop: each round spawns one attempt per lens.
func reviewLoopRoundFirstAttemptID(round int) string {
	return fmt.Sprintf("reviewer_attempt_%03d", (round-1)*len(reviewLenses)+1)
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

func reviewLoopFixtureNonBlockingFinding(message string, line int) map[string]any {
	finding := reviewLoopFixtureFinding(message, line)
	finding["blocking"] = false
	finding["severity"] = "low"
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
	// Only the correctness-lens attempt of the five-lens fan-out reports and
	// advances the round sequence; the other lenses come back clean. This keeps
	// one reviewer finding per round, so the sequence file counts rounds.
	if !strings.Contains(string(stdin), "You are the correctness reviewer") {
		os.Exit(0)
	}
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
	case mode == "non_blocking_finding":
		fmt.Print(reviewerStructuredOutput([]map[string]any{reviewLoopFixtureNonBlockingFinding("loop reviewer found an advisory issue", 55)}))
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
	case mode == "always_clean":
		// No proposals, no warnings: always a clean round.
	case mode == "findings_and_malformed":
		// One valid blocking finding plus one malformed (empty-message) entry;
		// the malformed entry becomes a warning, so the round has both a
		// created proposal and a warning in its summary.
		fmt.Print(reviewerStructuredOutput([]map[string]any{
			reviewLoopFixtureFinding("loop reviewer found a fixable issue", 42),
			{"message": "", "severity": "medium", "category": "quality"},
		}))
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
	case "change_on_second_attempt":
		attempt := nextReviewLoopHelperAttempt("PACTUM_REVIEW_LOOP_FIXER_SEQUENCE_FILE")
		if attempt == 2 {
			file, err := os.OpenFile("README.md", os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fixer open error: %v\n", err)
				os.Exit(2)
			}
			if _, err := fmt.Fprintf(file, "fixer-change=2\n"); err != nil {
				_ = file.Close()
				fmt.Fprintf(os.Stderr, "fixer write error: %v\n", err)
				os.Exit(2)
			}
			if err := file.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "fixer close error: %v\n", err)
				os.Exit(2)
			}
		}
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
