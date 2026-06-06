package app

import (
	"bytes"
	"encoding/json"
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
	reviewLoopFixerName    = "loop-fixer"
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
	if got := summary.Rounds[0]; got.OpenFindings != 1 || got.TotalOpenFindings != 1 || got.ProposalsCreated != 1 || got.ProposalsAccepted != 1 || got.FixerAttemptID != "attempt_001" || got.GateStatus != "passed" {
		t.Fatalf("round 1 summary mismatch: %#v", got)
	}
	if got := summary.Rounds[1]; got.OpenFindings != 0 || got.TotalOpenFindings != 1 || got.ProposalsCreated != 0 || got.ProposalsAccepted != 0 || got.FixerAttemptID != "" || got.GateStatus != "" {
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

func TestReviewLoopStopsAtStalemateAfterUnchangedFixRounds(t *testing.T) {
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
	if got := summary.Rounds[0]; got.ProposalsAccepted != 1 || got.UnchangedFingerprintStreak != 1 || got.WorkingTreeFingerprint == "" {
		t.Fatalf("round 1 stalemate signals mismatch: %#v", got)
	}
	if got := summary.Rounds[1]; got.ProposalsAccepted != 1 || got.UnchangedFingerprintStreak != 2 || got.WorkingTreeFingerprint == "" {
		t.Fatalf("round 2 stalemate signals mismatch: %#v", got)
	}
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
	assertFile(t, reviewFixAttemptPaths(runPaths, "attempt_002").ResultJSON)
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_003").ResultJSON)
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_003").ResultJSON)
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

func setReviewLoopMaxRoundsConfig(t *testing.T, paths artifacts.Paths, maxRounds int) {
	t.Helper()
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	config.Limits.Review.MaxRounds = maxRounds
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

func readReviewLoopSummary(t *testing.T, path string) reviewLoopSummaryDocument {
	t.Helper()
	var summary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &summary))
	return summary
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
	validFinding := []map[string]any{
		{
			"message":  "loop reviewer found a fixable issue",
			"severity": "medium",
			"category": "quality",
			"file":     "internal/app/review_loop.go",
			"line":     42,
			"blocking": true,
			"evidence": "fixture reviewer sequence",
		},
	}
	switch {
	case mode == "malformed":
		// Empty message -> propose-findings skips it with a warning (0 created),
		// which must NOT be treated as a clean review round.
		fmt.Print(reviewerStructuredOutput([]map[string]any{
			{"message": "", "severity": "medium", "category": "quality"},
		}))
	case mode == "always_findings":
		fmt.Print(reviewerStructuredOutput(validFinding))
	case mode == "clean_findings_clean_clean":
		if attempt == 2 {
			fmt.Print(reviewerStructuredOutput(validFinding))
		}
	case attempt == 1:
		fmt.Print(reviewerStructuredOutput(validFinding))
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
	path := os.Getenv("PACTUM_REVIEW_LOOP_REVIEWER_SEQUENCE_FILE")
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
