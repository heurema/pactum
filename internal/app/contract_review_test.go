package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

// TestContractReviewHelperProcess is the subprocess used by contract reviewer
// tests. It checks CWD, confirms the prompt contains the review heading, and
// conditionally emits structured findings controlled by env vars.
//
// PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS=1: emit structured findings block.
// PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING=1: make those findings blocking=true.
// PACTUM_CONTRACT_REVIEWER_CLEAN_ON_MARKER=1: emit no findings when stdin
//
//	contains "FIXED_MARKER", regardless of other emit vars.
//
// PACTUM_CONTRACT_REVIEWER_FAIL_LENS=<Heading>: exit 1 for that lens.
func TestContractReviewHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS") != "1" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cwd error: %v\n", err)
		os.Exit(2)
	}
	expectedCWD := os.Getenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD")
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	if resolved, err := filepath.EvalSymlinks(expectedCWD); err == nil {
		expectedCWD = resolved
	}
	fmt.Printf("cwd_is_repo=%t\n", cwd == expectedCWD)
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("stdin_has_contract_review_heading=%t\n", strings.Contains(string(stdin), "# Contract Review:"))
	if failLens := os.Getenv("PACTUM_CONTRACT_REVIEWER_FAIL_LENS"); failLens != "" {
		if strings.Contains(string(stdin), "# Contract Review: "+failLens) {
			fmt.Fprintln(os.Stderr, "contract-reviewer-fail-line")
			os.Exit(1)
		}
	}
	if os.Getenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS") == "1" {
		cleanOnMarker := os.Getenv("PACTUM_CONTRACT_REVIEWER_CLEAN_ON_MARKER") == "1"
		if cleanOnMarker && strings.Contains(string(stdin), "FIXED_MARKER") {
			// Contract already fixed; emit no findings this round.
		} else {
			blocking := os.Getenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING") == "1"
			fmt.Print(contractReviewerStructuredFindingOutput(blocking))
		}
	}
	fmt.Fprintln(os.Stderr, "contract-reviewer-stderr-line")
	os.Exit(0)
}

// TestContractReviewFixerHelperProcess is the subprocess used by contract fixer
// tests. The fixer is always the executor agent (codex / first in registry).
//
// PACTUM_CONTRACT_FIXER_HELPER_PROCESS=1: activates this helper mode.
// PACTUM_CONTRACT_FIXER_EMIT_REVISE=1: emit a valid revise block extracted
//
//	from the prompt (uses base_version from "Current contract version: <v>" line).
//
// PACTUM_CONTRACT_FIXER_EMIT_STALE_VERSION=1: emit a revise block with an
//
//	intentionally wrong base_version (tests stale-version rejection).
//
// PACTUM_CONTRACT_FIXER_CORRUPT_CONTRACT=1: emit a valid revise block AND then
//
//	overwrite the file at PACTUM_CONTRACT_FIXER_CORRUPT_PATH with garbage JSON,
//	triggering a fatal fixer error when the host process calls ContractRevise.
func TestContractReviewFixerHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS") != "1" {
		return
	}
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixer stdin error: %v\n", err)
		os.Exit(2)
	}
	switch {
	case os.Getenv("PACTUM_CONTRACT_FIXER_EMIT_REVISE") == "1":
		version := extractContractVersionFromPrompt(string(stdin))
		fmt.Print(contractReviewFixerReviseOutput(version))
	case os.Getenv("PACTUM_CONTRACT_FIXER_EMIT_STALE_VERSION") == "1":
		fmt.Print(contractReviewFixerReviseOutput("stale-version-that-does-not-match"))
	case os.Getenv("PACTUM_CONTRACT_FIXER_CORRUPT_CONTRACT") == "1":
		version := extractContractVersionFromPrompt(string(stdin))
		fmt.Print(contractReviewFixerReviseOutput(version))
		if p := os.Getenv("PACTUM_CONTRACT_FIXER_CORRUPT_PATH"); p != "" {
			os.WriteFile(p, []byte("{this is garbage json!!!}"), 0o644) //nolint:errcheck
		}
	}
	os.Exit(0)
}

// extractContractVersionFromPrompt extracts the SHA256 version string from the
// fixer prompt line "Current contract version: <v>".
func extractContractVersionFromPrompt(prompt string) string {
	prefix := "Current contract version: "
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return "unknown-version"
}

// contractReviewerStructuredFindingOutput returns a reviewer findings JSON block.
// When blocking is true the finding has blocking=true; otherwise blocking=false.
func contractReviewerStructuredFindingOutput(blocking bool) string {
	block := map[string]any{
		"schema": reviewerFindingsSchema,
		"findings": []any{
			map[string]any{
				"message":    "Acceptance criteria do not include machine-checkable validation commands.",
				"severity":   "high",
				"category":   "correctness",
				"blocking":   blocking,
				"confidence": "high",
				"evidence":   "validation.commands is empty; acceptance_criteria are prose-only.",
			},
		},
	}
	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		panic(err)
	}
	return "reviewer analysis\n```json\n" + string(data) + "\n```\n"
}

// contractReviewFixerReviseOutput returns a fixer revise block that adds
// "FIXED_MARKER" to the contract assumptions so reviewers can detect that
// the contract was revised.
func contractReviewFixerReviseOutput(baseVersion string) string {
	block := map[string]any{
		"schema":       contractReviewFixerReviseSchema,
		"base_version": baseVersion,
		"contract": map[string]any{
			"assumptions": []string{"FIXED_MARKER: this contract has been revised by the fixer"},
		},
	}
	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		panic(err)
	}
	return "fixer analysis\n```json\n" + string(data) + "\n```\n"
}

// configureHelperContractReviewers registers reviewer agents and wires the
// test binary's TestContractReviewHelperProcess as the reviewer runtime.
func configureHelperContractReviewers(t *testing.T, app App, paths artifacts.Paths, names ...string) App {
	t.Helper()
	registerTestAgents(t, paths, names...)
	setContractReviewersConfig(t, paths, names...)
	app.AgentRegistry = testAgentRegistry(testHelperDescriptors(names, "TestContractReviewHelperProcess")...)
	return app
}

// configureHelperContractFixer wires the default reviewer (first entry in
// registry) to TestContractReviewFixerHelperProcess. Call after
// configureHelperContractReviewers — it rebuilds AgentRegistry with the fixer
// and panel reviewers both in the reviewer slot (fixer uses the reviewer role).
func configureHelperContractFixer(t *testing.T, app App, paths artifacts.Paths) App {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	if len(config.Agents) == 0 {
		t.Fatal("no agents registered; call configureHelperContractReviewers first")
	}
	fixerEngine := testAgentEngine(config.Agents[0].Name)
	fixer := agents.AgentDescriptor{
		Name:    fixerEngine,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestContractReviewFixerHelperProcess", "--"},
		Input:   agents.InputPromptFile,
	}
	reviewers := testHelperDescriptors([]string(config.Pipeline.ContractReview.By), "TestContractReviewHelperProcess")
	app.AgentRegistry = testAgentRegistryRoles(reviewers, append([]agents.AgentDescriptor{fixer}, reviewers...))
	return app
}

func setContractReviewersConfig(t *testing.T, paths artifacts.Paths, names ...string) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	config.Pipeline.ContractReview.By = stageBy(names)
	assertNoError(t, writeYAML(paths.Config, config))
}

func setContractReviewPatienceConfig(t *testing.T, paths artifacts.Paths, patience int) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	if config.Pipeline.ContractReview.Loop == nil {
		config.Pipeline.ContractReview.Loop = &loopConfig{}
	}
	config.Pipeline.ContractReview.Loop.Patience = patience
	assertNoError(t, writeYAML(paths.Config, config))
}

func setContractReviewMaxRoundsConfig(t *testing.T, paths artifacts.Paths, maxRounds int) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	if config.Pipeline.ContractReview.Loop == nil {
		config.Pipeline.ContractReview.Loop = &loopConfig{}
	}
	config.Pipeline.ContractReview.Loop.Max = maxRounds
	config.Pipeline.ContractReview.Loop.Patience = maxRounds + 10 // prevent stalemate from firing before max_rounds
	assertNoError(t, writeYAML(paths.Config, config))
}

// TestContractReviewNoReviewersIsNoOp checks that contract review with no
// contract.reviewers exits 0, prints a no-op message, creates no attempt
// artifacts, and still emits the loop started/finished ledger events.
func TestContractReviewNoReviewersIsNoOp(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "No contract reviewers") {
		t.Fatalf("expected no-op message, got:\n%s", got)
	}
	if got := stderr.String(); strings.Contains(got, "contract-reviewer-stderr-line") {
		t.Fatalf("no-op review should not invoke any agent, got stderr:\n%s", got)
	}

	// Loop events are emitted unconditionally, even when no reviewers are configured.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_loop_started") != 1 {
		t.Fatalf("expected 1 contract_review_loop_started event, got:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "contract_review_loop_finished") != 1 {
		t.Fatalf("expected 1 contract_review_loop_finished event, got:\n%v", eventTypes)
	}
}

// TestContractReviewPanelRunsAndEmitsFindings checks that contract review with
// configured reviewers invokes the panel, writes attempt artifacts under
// contract/reviewer/attempts/, and returns structured findings in JSON output.
func TestContractReviewPanelRunsAndEmitsFindings(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	// blocking=false (default) → no fixer invoked → single round → "resolved"

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d, stderr: %s", code, stderr.String())
	}

	// Live reviewer output (both stdout and stderr) streams to operator stderr.
	if got := stderr.String(); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "contract-reviewer-stderr-line") {
		t.Fatalf("live reviewer output missing from stderr:\n%s", got)
	}
	// Clean stdout is the JSON response only.
	if strings.Contains(stdout.String(), "cwd_is_repo=") || strings.Contains(stdout.String(), "contract-reviewer-stderr-line") {
		t.Fatalf("agent output leaked into stdout:\n%s", stdout.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewLoopSchema || response.RunID != runID {
		t.Fatalf("unexpected response schema/run: %#v", response)
	}
	if response.TerminalReason != "resolved" {
		t.Fatalf("expected terminal_reason=resolved, got %q", response.TerminalReason)
	}
	if len(response.Reviewers) != 1 || response.Reviewers[0] != "helper" {
		t.Fatalf("unexpected reviewers: %#v", response.Reviewers)
	}
	if len(response.Lenses) != len(contractReviewLenses) {
		t.Fatalf("expected %d lenses, got %d: %v", len(contractReviewLenses), len(response.Lenses), response.Lenses)
	}
	if len(response.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(response.Rounds))
	}
	round := response.Rounds[0]
	if round.BlockingFindings != 0 {
		t.Fatalf("expected no blocking findings (helper emits blocking=false), got %d", round.BlockingFindings)
	}
	if len(round.SkippedLenses) != 0 {
		t.Fatalf("expected no skipped lenses, got: %v", round.SkippedLenses)
	}
	// Helper emits one finding per lens (EMIT_FINDINGS=1); each is parsed.
	if len(round.Findings) != len(contractReviewLenses) {
		t.Fatalf("expected %d findings (one per lens), got %d: %v", len(contractReviewLenses), len(round.Findings), round.Findings)
	}
	for _, f := range round.Findings {
		if f.Reviewer != "helper" || f.Lens == "" || f.Message == "" || f.Severity == "" {
			t.Fatalf("finding missing required fields: %#v", f)
		}
		if f.Blocking {
			t.Fatalf("finding should be non-blocking (no EMIT_BLOCKING): %#v", f)
		}
	}

	// One attempt per lens under contract/reviewer/attempts/.
	seenLenses := map[string]bool{}
	for index := 1; index <= len(contractReviewLenses); index++ {
		attemptID := fmt.Sprintf("contract_reviewer_attempt_%03d", index)
		ap := contractReviewerAttemptPaths(runPaths, attemptID)
		assertFile(t, ap.RequestJSON)
		assertFile(t, ap.StdoutLog)
		assertFile(t, ap.StderrLog)
		assertFile(t, ap.ResultJSON)

		var request contractReviewerRequestDocument
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, ap.RequestJSON)), &request))
		if request.Schema != contractReviewerRequestSchema || request.RunID != runID || request.AttemptID != attemptID {
			t.Fatalf("unexpected request: %#v", request)
		}
		if request.Lens == "" || seenLenses[request.Lens] {
			t.Fatalf("request lens missing or duplicated: %#v", request)
		}
		seenLenses[request.Lens] = true
		wantPromptArtifact := "contract/reviewer/prompt-helper-" + request.Lens + ".md"
		if request.Artifacts.ReviewerPrompt != wantPromptArtifact {
			t.Fatalf("unexpected prompt artifact: %q, want %q", request.Artifacts.ReviewerPrompt, wantPromptArtifact)
		}
		// ACP agents carry the prompt via Artifacts, not WouldRun.Stdin.
		if request.WouldRun.Stdin != "" {
			t.Fatalf("ACP would_run must not use stdin, got %q", request.WouldRun.Stdin)
		}

		var result contractReviewerResultDocument
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, ap.ResultJSON)), &result))
		if result.Schema != contractReviewerResultSchema || result.Reviewer != "claude" || result.Lens != request.Lens || result.ExitCode != 0 || result.TimedOut {
			t.Fatalf("unexpected result: %#v", result)
		}
		wantStdout := "contract/reviewer/attempts/" + attemptID + "/stdout.log"
		wantStderr := "contract/reviewer/attempts/" + attemptID + "/stderr.log"
		if result.Stdout != wantStdout || result.Stderr != wantStderr {
			t.Fatalf("unexpected result log paths: stdout=%q stderr=%q", result.Stdout, result.Stderr)
		}
	}
	for _, lens := range contractReviewLenses {
		if !seenLenses[lens.Key] {
			t.Fatalf("lens %s missing from attempts: %#v", lens.Key, seenLenses)
		}
	}

	// Prompt files written for each lens.
	for _, lens := range contractReviewLenses {
		promptPath := contractReviewerLensPromptPath(runPaths, "helper", lens)
		assertFile(t, promptPath)
		if got := mustReadFile(t, promptPath); !strings.Contains(got, "# Contract Review: "+lens.Heading) {
			t.Fatalf("prompt for lens %s missing heading:\n%s", lens.Key, got)
		}
	}

	// Events emitted.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_loop_started") != 1 {
		t.Fatalf("expected 1 contract_review_loop_started event, got:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "contract_review_loop_finished") != 1 {
		t.Fatalf("expected 1 contract_review_loop_finished event, got:\n%v", eventTypes)
	}
	startIdx := indexOfEvent(eventTypes, "contract_review_loop_started")
	finishIdx := indexOfEvent(eventTypes, "contract_review_loop_finished")
	if startIdx >= finishIdx {
		t.Fatalf("contract_review_loop_started (%d) must precede contract_review_loop_finished (%d)", startIdx, finishIdx)
	}
	// Loop started must precede per-attempt events.
	firstAttemptIdx := indexOfEvent(eventTypes, "contract_reviewer_attempt_started")
	if firstAttemptIdx != -1 && startIdx >= firstAttemptIdx {
		t.Fatalf("contract_review_loop_started (%d) must precede attempt events (%d)", startIdx, firstAttemptIdx)
	}
	if countEvents(eventTypes, "contract_reviewer_attempt_started") != len(contractReviewLenses) {
		t.Fatalf("expected %d started events, got:\n%v", len(contractReviewLenses), eventTypes)
	}
	if countEvents(eventTypes, "contract_reviewer_attempt_finished") != len(contractReviewLenses) {
		t.Fatalf("expected %d finished events, got:\n%v", len(contractReviewLenses), eventTypes)
	}

	// Next affordance: still contract_draft (review does not change run status).
	if response.Next == nil {
		t.Fatalf("response missing next affordance")
	}
}

// TestContractReviewFailedLensSkipped checks that when one lens attempt fails,
// it is recorded in skipped_lenses in the round, the command still exits 0, and
// the failed lens's findings do not appear in the aggregated results.
func TestContractReviewFailedLensSkipped(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	// Fail only the "Completeness" lens; all others succeed with advisory findings.
	t.Setenv("PACTUM_CONTRACT_REVIEWER_FAIL_LENS", "Completeness")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	// blocking=false (default): round stays clean, no fixer invoked.

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review with one failed lens should exit 0, got %d, stderr: %s", code, stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewLoopSchema {
		t.Fatalf("unexpected response schema: %q", response.Schema)
	}
	if len(response.Rounds) == 0 {
		t.Fatalf("expected at least one round")
	}
	round := response.Rounds[0]

	// (a) The skipped_lenses array reflects the reviewer failure.
	if len(round.SkippedLenses) == 0 {
		t.Fatalf("expected non-empty skipped_lenses reflecting the completeness lens failure")
	}
	if round.SkippedLenses[0].Reviewer != "helper" || round.SkippedLenses[0].Lens != "completeness" {
		t.Fatalf("unexpected skipped lens: %#v", round.SkippedLenses[0])
	}
	if round.SkippedLenses[0].Reason == "" {
		t.Fatalf("skipped lens must carry a non-empty reason")
	}
	// (a) The warnings array also reflects the reviewer failure.
	if len(round.Warnings) == 0 {
		t.Fatalf("expected non-empty warnings array reflecting the completeness lens failure")
	}

	// (b) Findings from the failed lens do not appear; only lenses-1 lenses produced findings.
	expectedFindings := len(contractReviewLenses) - 1
	if len(round.Findings) != expectedFindings {
		t.Fatalf("expected %d findings (skipped lens excluded), got %d: %v", expectedFindings, len(round.Findings), round.Findings)
	}
	for _, f := range round.Findings {
		if f.Lens == "completeness" {
			t.Fatalf("finding from failed lens must not appear: %#v", f)
		}
	}

	// (c) A skipped lens does not affect Clean (blocking_findings count) or
	// Progress (fixer invocation + hash change): round is clean, fixer not invoked.
	if round.BlockingFindings != 0 {
		t.Fatalf("skipped lens must not set blocking_findings; blocking count = %d", round.BlockingFindings)
	}
	if round.FixerAttemptID != "" {
		t.Fatalf("skipped lens must not trigger fixer; fixer_attempt_id = %q", round.FixerAttemptID)
	}

	// The successful lenses ran.
	expectedSuccess := len(contractReviewLenses) - 1
	expectedAttempts := len(contractReviewLenses)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	successCount := 0
	for index := 1; index <= expectedAttempts; index++ {
		ap := contractReviewerAttemptPaths(runPaths, fmt.Sprintf("contract_reviewer_attempt_%03d", index))
		if !fileExists(t, ap.ResultJSON) {
			continue
		}
		var result contractReviewerResultDocument
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, ap.ResultJSON)), &result))
		if result.ExitCode == 0 {
			successCount++
		}
	}
	if successCount != expectedSuccess {
		t.Fatalf("expected %d successful lens attempts, counted %d", expectedSuccess, successCount)
	}

	// Loop events are present.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_loop_started") != 1 {
		t.Fatalf("expected 1 contract_review_loop_started event, got:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "contract_review_loop_finished") != 1 {
		t.Fatalf("expected 1 contract_review_loop_finished event, got:\n%v", eventTypes)
	}
}

// TestContractReviewMultipleReviewers checks that two configured reviewers each
// run once per lens, that every finding records which reviewer produced it, and
// that the response lists both reviewer names.
func TestContractReviewMultipleReviewers(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper-a", "helper-b")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	// blocking=false (default) → no fixer → resolved after round 1

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d, stderr: %s", code, stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewLoopSchema {
		t.Fatalf("unexpected response schema: %q", response.Schema)
	}
	if len(response.Reviewers) != 2 {
		t.Fatalf("expected 2 reviewers, got %d: %v", len(response.Reviewers), response.Reviewers)
	}
	if len(response.Rounds) == 0 {
		t.Fatalf("expected at least one round")
	}
	round := response.Rounds[0]
	// Two reviewers × N lenses = 2×N findings (helper emits one finding per lens).
	expectedFindings := 2 * len(contractReviewLenses)
	if len(round.Findings) != expectedFindings {
		t.Fatalf("expected %d findings, got %d", expectedFindings, len(round.Findings))
	}
	reviewerCounts := map[string]int{}
	for _, f := range round.Findings {
		if f.Reviewer != "helper-a" && f.Reviewer != "helper-b" {
			t.Fatalf("finding has unexpected reviewer: %q", f.Reviewer)
		}
		reviewerCounts[f.Reviewer]++
	}
	if reviewerCounts["helper-a"] != len(contractReviewLenses) || reviewerCounts["helper-b"] != len(contractReviewLenses) {
		t.Fatalf("reviewer finding counts wrong: %v", reviewerCounts)
	}
	if len(round.SkippedLenses) != 0 {
		t.Fatalf("expected no skipped lenses, got: %v", round.SkippedLenses)
	}

	// Loop events are present.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_loop_started") != 1 {
		t.Fatalf("expected 1 contract_review_loop_started event, got:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "contract_review_loop_finished") != 1 {
		t.Fatalf("expected 1 contract_review_loop_finished event, got:\n%v", eventTypes)
	}
}

// TestContractReviewHumanReadableFindings checks that without --json the
// command prints findings in human-readable form.
func TestContractReviewHumanReadableFindings(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d, stderr: %s", code, stderr.String())
	}

	output := stdout.String()
	// Round summary line includes finding count.
	wantFindingsLine := fmt.Sprintf("findings %d (blocking 0)", len(contractReviewLenses))
	if !strings.Contains(output, wantFindingsLine) {
		t.Fatalf("expected %q in output:\n%s", wantFindingsLine, output)
	}
	if !strings.Contains(output, "[helper]") {
		t.Fatalf("expected reviewer name in output:\n%s", output)
	}
	if !strings.Contains(output, "Acceptance criteria do not include machine-checkable validation commands") {
		t.Fatalf("expected finding message in output:\n%s", output)
	}
	if !strings.Contains(output, "Next:") {
		t.Fatalf("expected Next section in output:\n%s", output)
	}

	// Loop events are present.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_loop_started") != 1 {
		t.Fatalf("expected 1 contract_review_loop_started event, got:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "contract_review_loop_finished") != 1 {
		t.Fatalf("expected 1 contract_review_loop_finished event, got:\n%v", eventTypes)
	}
}

// TestContractReviewLoopCleanConvergence checks the full convergence path:
// round 1 produces blocking findings, the fixer applies a revise, and round 2
// returns no findings (contract now has "FIXED_MARKER") → terminal_reason=resolved.
func TestContractReviewLoopCleanConvergence(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	app = configureHelperContractFixer(t, app, paths)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_CLEAN_ON_MARKER", "1")
	t.Setenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_FIXER_EMIT_REVISE", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewLoopSchema {
		t.Fatalf("unexpected schema: %q", response.Schema)
	}
	if response.TerminalReason != "resolved" {
		t.Fatalf("expected terminal_reason=resolved, got %q\nfull response: %s", response.TerminalReason, stdout.String())
	}
	if len(response.Rounds) != 2 {
		t.Fatalf("expected 2 rounds, got %d", len(response.Rounds))
	}

	// Round 1: blocking findings + fixer applies a fix.
	r1 := response.Rounds[0]
	if r1.BlockingFindings == 0 {
		t.Fatalf("round 1 should have blocking findings: %#v", r1)
	}
	if r1.FixerAttemptID == "" {
		t.Fatalf("round 1 should have a fixer attempt ID: %#v", r1)
	}
	if r1.FixesApplied != 1 || r1.FixesSkipped != 0 {
		t.Fatalf("round 1: expected FixesApplied=1, got applied=%d skipped=%d", r1.FixesApplied, r1.FixesSkipped)
	}
	if r1.UnchangedVersionStreak != 0 {
		t.Fatalf("round 1: version should have changed (streak should be 0), got %d", r1.UnchangedVersionStreak)
	}

	// Round 2: clean (no findings, no fixer).
	r2 := response.Rounds[1]
	if len(r2.Findings) != 0 {
		t.Fatalf("round 2 should be clean, got findings: %#v", r2.Findings)
	}
	if r2.FixerAttemptID != "" {
		t.Fatalf("round 2 should not invoke fixer: %#v", r2)
	}
	if r2.CleanStreak < 1 {
		t.Fatalf("round 2 clean streak should be >= 1, got %d", r2.CleanStreak)
	}

	// Loop events are present.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_loop_started") != 1 {
		t.Fatalf("expected 1 contract_review_loop_started event, got:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "contract_review_loop_finished") != 1 {
		t.Fatalf("expected 1 contract_review_loop_finished event, got:\n%v", eventTypes)
	}
}

// TestContractReviewLoopMaxRounds checks that the loop terminates with
// terminal_reason=max_rounds when blocking findings persist through all rounds.
func TestContractReviewLoopMaxRounds(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	app = configureHelperContractFixer(t, app, paths)
	setContractReviewMaxRoundsConfig(t, paths, 2)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING", "1")
	// Fixer emits no revise block → fix skipped → version unchanged.
	// But patience is max_rounds+10 so stalemate does not fire before max_rounds.
	t.Setenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewLoopSchema {
		t.Fatalf("unexpected schema: %q", response.Schema)
	}
	if response.TerminalReason != "max_rounds" {
		t.Fatalf("expected terminal_reason=max_rounds, got %q\nfull response: %s", response.TerminalReason, stdout.String())
	}
	if len(response.Rounds) != 2 {
		t.Fatalf("expected 2 rounds (max_rounds=2), got %d", len(response.Rounds))
	}
	for i, r := range response.Rounds {
		if r.BlockingFindings == 0 {
			t.Fatalf("round %d should have blocking findings", i+1)
		}
	}

	// Loop events are present.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_loop_started") != 1 {
		t.Fatalf("expected 1 contract_review_loop_started event, got:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "contract_review_loop_finished") != 1 {
		t.Fatalf("expected 1 contract_review_loop_finished event, got:\n%v", eventTypes)
	}
}

// TestContractReviewLoopVersionGuard checks that when the fixer outputs a revise
// block with a stale base_version, the fix is skipped (warning surfaced) and the
// loop terminates with stalemate after patience is exhausted.
func TestContractReviewLoopVersionGuard(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	app = configureHelperContractFixer(t, app, paths)
	setContractReviewPatienceConfig(t, paths, 1) // stalemate after 1 unchanged round
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING", "1")
	t.Setenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_FIXER_EMIT_STALE_VERSION", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewLoopSchema {
		t.Fatalf("unexpected schema: %q", response.Schema)
	}
	if response.TerminalReason != "stalemate" {
		t.Fatalf("expected terminal_reason=stalemate, got %q\nfull response: %s", response.TerminalReason, stdout.String())
	}
	if len(response.Rounds) == 0 {
		t.Fatalf("expected at least one round")
	}

	// The fixer ran but the revise was rejected; fix skipped, contract unchanged.
	r1 := response.Rounds[0]
	if r1.FixesApplied != 0 || r1.FixesSkipped == 0 {
		t.Fatalf("expected FixesSkipped>0, got applied=%d skipped=%d", r1.FixesApplied, r1.FixesSkipped)
	}
	if r1.UnchangedVersionStreak == 0 {
		t.Fatalf("expected UnchangedVersionStreak > 0 after stale-version rejection, got %d", r1.UnchangedVersionStreak)
	}
	// At least one warning mentions the stale version.
	hasStaleWarning := false
	for _, w := range r1.Warnings {
		if strings.Contains(w, "STALE_VERSION") || strings.Contains(w, "stale") || strings.Contains(w, "base_version") {
			hasStaleWarning = true
			break
		}
	}
	if !hasStaleWarning {
		t.Fatalf("expected a stale-version warning, got: %v", r1.Warnings)
	}

	// Loop events are present.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_loop_started") != 1 {
		t.Fatalf("expected 1 contract_review_loop_started event, got:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "contract_review_loop_finished") != 1 {
		t.Fatalf("expected 1 contract_review_loop_finished event, got:\n%v", eventTypes)
	}
}

// TestContractReviewLoopFinishedOnError verifies that when the fixer corrupts
// contract.json, the loop returns a fatal error, loop events are still emitted,
// the exit code is non-zero, and the stdout JSON shows terminal_reason="error".
func TestContractReviewLoopFinishedOnError(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	app = configureHelperContractFixer(t, app, paths)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING", "1")
	t.Setenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS", "1")
	// Fixer emits a valid revise block AND then corrupts contract.json on disk.
	t.Setenv("PACTUM_CONTRACT_FIXER_CORRUPT_CONTRACT", "1")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	t.Setenv("PACTUM_CONTRACT_FIXER_CORRUPT_PATH", runPaths.ContractJSON)

	// Run WITHOUT --json to avoid double-writing (the error path always writes
	// JSON to stdout; --json would cause a second write on the non-error path).
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code, got 0\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}

	// Error path always emits JSON on stdout.
	outBytes := stdout.Bytes()
	if len(outBytes) == 0 {
		t.Fatalf("expected JSON on stdout even on error, got empty output (stderr: %s)", stderr.String())
	}
	var response contractReviewLoopResponse
	if err := json.Unmarshal(outBytes, &response); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, string(outBytes))
	}
	if response.TerminalReason != "error" {
		t.Fatalf("expected terminal_reason=error, got %q\nfull response: %s", response.TerminalReason, string(outBytes))
	}

	// Both loop events must be present in the ledger.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_loop_started") != 1 {
		t.Fatalf("expected 1 contract_review_loop_started event, got:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "contract_review_loop_finished") != 1 {
		t.Fatalf("expected 1 contract_review_loop_finished event, got:\n%v", eventTypes)
	}

	// No loop-summary artifact when the loop did not complete normally.
	summaryPath := filepath.Join(filepath.Dir(runPaths.ContractJSON), "loop-summary.json")
	if fileExists(t, summaryPath) {
		t.Fatalf("loop-summary.json must not be written on error, but found at %s", summaryPath)
	}
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}
