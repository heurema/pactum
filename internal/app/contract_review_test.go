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

	"github.com/heurema/pactum/internal/artifacts"
)

// TestContractReviewHelperProcess is the subprocess used by contract review
// tests. It checks that it is running in the repo root, reports whether stdin
// contains a contract review heading, optionally fails for a specific lens
// heading (PACTUM_CONTRACT_REVIEWER_FAIL_LENS), and optionally emits structured
// findings (PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS=1).
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
		fmt.Print(contractReviewerStructuredFindingOutput())
	}
	fmt.Fprintln(os.Stderr, "contract-reviewer-stderr-line")
	os.Exit(0)
}

func contractReviewerStructuredFindingOutput() string {
	block := map[string]any{
		"schema": reviewerFindingsSchema,
		"findings": []any{
			map[string]any{
				"message":    "Acceptance criteria do not include machine-checkable validation commands.",
				"severity":   "high",
				"category":   "correctness",
				"blocking":   true,
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

// configureHelperContractReviewers registers names in the agent registry,
// sets contract.reviewers in config, and wires the test binary as the agent
// runtime for TestContractReviewHelperProcess.
func configureHelperContractReviewers(t *testing.T, app App, paths artifacts.Paths, names ...string) App {
	t.Helper()
	registerTestAgents(t, paths, names...)
	setContractReviewersConfig(t, paths, names...)
	app.AgentRegistry = testAgentRegistry(testHelperDescriptors(names, "TestContractReviewHelperProcess")...)
	return app
}

func setContractReviewersConfig(t *testing.T, paths artifacts.Paths, names ...string) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	config.Contract.Reviewers = names
	assertNoError(t, writeYAML(paths.Config, config))
}

// TestContractReviewNoReviewersIsNoOp checks that contract review with no
// contract.reviewers exits 0, prints a no-op message, and creates no attempt
// artifacts.
func TestContractReviewNoReviewersIsNoOp(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

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

	var response contractReviewResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewSchema || response.RunID != runID {
		t.Fatalf("unexpected response schema/run: %#v", response)
	}
	if len(response.Reviewers) != 1 || response.Reviewers[0] != "helper" {
		t.Fatalf("unexpected reviewers: %#v", response.Reviewers)
	}
	if len(response.Lenses) != len(contractReviewLenses) {
		t.Fatalf("expected %d lenses, got %d: %v", len(contractReviewLenses), len(response.Lenses), response.Lenses)
	}
	if len(response.SkippedLenses) != 0 {
		t.Fatalf("expected no skipped lenses, got: %v", response.SkippedLenses)
	}
	// Helper emits one finding per lens (EMIT_FINDINGS=1); each is parsed.
	if len(response.Findings) != len(contractReviewLenses) {
		t.Fatalf("expected %d findings (one per lens), got %d: %v", len(contractReviewLenses), len(response.Findings), response.Findings)
	}
	for _, f := range response.Findings {
		if f.Reviewer != "helper" || f.Lens == "" || f.Message == "" || f.Severity == "" {
			t.Fatalf("finding missing required fields: %#v", f)
		}
		if !f.Blocking {
			t.Fatalf("finding should be blocking (helper always emits blocking=true): %#v", f)
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
// it is recorded in skipped_lenses and the command still exits 0 (not aborted).
func TestContractReviewFailedLensSkipped(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	// Fail only the "Completeness" lens; all others succeed.
	t.Setenv("PACTUM_CONTRACT_REVIEWER_FAIL_LENS", "Completeness")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review with one failed lens should exit 0, got %d, stderr: %s", code, stderr.String())
	}

	var response contractReviewResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewSchema {
		t.Fatalf("unexpected response schema: %q", response.Schema)
	}

	// Exactly one skipped lens (the completeness attempt that exited 1).
	if len(response.SkippedLenses) != 1 {
		t.Fatalf("expected 1 skipped lens, got %d: %v", len(response.SkippedLenses), response.SkippedLenses)
	}
	if response.SkippedLenses[0].Reviewer != "helper" || response.SkippedLenses[0].Lens != "completeness" {
		t.Fatalf("unexpected skipped lens: %#v", response.SkippedLenses[0])
	}
	// The 4 other lenses ran successfully (helper exits 0 for them).
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

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d, stderr: %s", code, stderr.String())
	}

	var response contractReviewResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewSchema {
		t.Fatalf("unexpected response schema: %q", response.Schema)
	}
	if len(response.Reviewers) != 2 {
		t.Fatalf("expected 2 reviewers, got %d: %v", len(response.Reviewers), response.Reviewers)
	}
	// Two reviewers × N lenses = 2×N findings (helper emits one finding per lens).
	expectedFindings := 2 * len(contractReviewLenses)
	if len(response.Findings) != expectedFindings {
		t.Fatalf("expected %d findings, got %d", expectedFindings, len(response.Findings))
	}
	reviewerCounts := map[string]int{}
	for _, f := range response.Findings {
		if f.Reviewer != "helper-a" && f.Reviewer != "helper-b" {
			t.Fatalf("finding has unexpected reviewer: %q", f.Reviewer)
		}
		reviewerCounts[f.Reviewer]++
	}
	if reviewerCounts["helper-a"] != len(contractReviewLenses) || reviewerCounts["helper-b"] != len(contractReviewLenses) {
		t.Fatalf("reviewer finding counts wrong: %v", reviewerCounts)
	}
	if len(response.SkippedLenses) != 0 {
		t.Fatalf("expected no skipped lenses, got: %v", response.SkippedLenses)
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
	wantFindingsLine := fmt.Sprintf("Findings: %d", len(contractReviewLenses))
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
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}
