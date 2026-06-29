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
// PACTUM_CONTRACT_REVIEWER_CLEAN_ON_MARKER=1: emit an empty findings block when stdin
//
//	contains "FIXED_MARKER", regardless of other emit vars.
//
// PACTUM_CONTRACT_REVIEWER_FAIL_LENS=<Heading>: exit 1 for that lens.
// PACTUM_CONTRACT_REVIEWER_PARSE_MISS_LENS=<Heading>: target parse-miss hooks at one lens.
// PACTUM_CONTRACT_REVIEWER_SOFT_PARSE_MISS=1: first attempt exits 0 with unparseable stdout.
// PACTUM_CONTRACT_REVIEWER_EMPTY_STDOUT=1: first attempt exits 0 with empty stdout.
// PACTUM_CONTRACT_REVIEWER_CORRECTIVE_EMIT_FINDINGS=1: corrective attempt emits one finding.
// PACTUM_CONTRACT_REVIEWER_CORRECTIVE_EMIT_EMPTY_BLOCK=1: corrective attempt emits findings=[].
// PACTUM_CONTRACT_REVIEWER_CORRECTIVE_FAIL=1: corrective attempt exits 1.
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
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(2)
	}
	prompt := string(stdin)
	isCorrective := strings.Contains(prompt, "# Corrective Contract Review:")
	if os.Getenv("PACTUM_CONTRACT_REVIEWER_EMPTY_STDOUT") == "1" && !isCorrective && contractReviewerHookMatchesLens(prompt) {
		fmt.Fprintln(os.Stderr, "contract-reviewer-empty-stdout-line")
		os.Exit(0)
	}
	fmt.Printf("cwd_is_repo=%t\n", cwd == expectedCWD)
	fmt.Printf("stdin_has_contract_review_heading=%t\n", strings.Contains(prompt, "# Contract Review:"))
	if failLens := os.Getenv("PACTUM_CONTRACT_REVIEWER_FAIL_LENS"); failLens != "" {
		if strings.Contains(prompt, "# Contract Review: "+failLens) {
			fmt.Fprintln(os.Stderr, "contract-reviewer-fail-line")
			os.Exit(1)
		}
	}

	if os.Getenv("PACTUM_CONTRACT_REVIEWER_SOFT_PARSE_MISS") == "1" && contractReviewerHookMatchesLens(prompt) {
		if isCorrective {
			if os.Getenv("PACTUM_CONTRACT_REVIEWER_CORRECTIVE_FAIL") == "1" {
				fmt.Fprintln(os.Stderr, "contract-reviewer-corrective-fail-line")
				os.Exit(1)
			}
			if os.Getenv("PACTUM_CONTRACT_REVIEWER_CORRECTIVE_EMIT_FINDINGS") == "1" {
				fmt.Print(contractReviewerStructuredFindingOutput(false))
			} else if os.Getenv("PACTUM_CONTRACT_REVIEWER_CORRECTIVE_EMIT_EMPTY_BLOCK") == "1" {
				fmt.Print(contractReviewerEmptyFindingsOutput())
			} else {
				fmt.Println("corrective analysis without structured findings")
			}
		} else {
			fmt.Println("first attempt analysis without structured findings")
		}
		fmt.Fprintln(os.Stderr, "contract-reviewer-stderr-line")
		os.Exit(0)
	}

	if os.Getenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS") == "1" {
		cleanOnMarker := os.Getenv("PACTUM_CONTRACT_REVIEWER_CLEAN_ON_MARKER") == "1"
		if cleanOnMarker && strings.Contains(prompt, "FIXED_MARKER") {
			fmt.Print(contractReviewerEmptyFindingsOutput())
		} else if os.Getenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING_NO_IMPACT") == "1" {
			fmt.Print(contractReviewerBlockingNoImpactOutput())
		} else {
			blocking := os.Getenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING") == "1"
			fmt.Print(contractReviewerStructuredFindingOutput(blocking))
		}
	} else {
		fmt.Print(contractReviewerEmptyFindingsOutput())
	}
	fmt.Fprintln(os.Stderr, "contract-reviewer-stderr-line")
	os.Exit(0)
}

func contractReviewerHookMatchesLens(prompt string) bool {
	lens := os.Getenv("PACTUM_CONTRACT_REVIEWER_PARSE_MISS_LENS")
	if lens == "" {
		return true
	}
	return strings.Contains(prompt, "# Contract Review: "+lens) || strings.Contains(prompt, "# Corrective Contract Review: "+lens)
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

// contractReviewerStructuredFindingOutput returns a reviewer findings JSON block
// under contractReviewerResultSchema. When blocking is true the finding includes
// material_impact (required for a finding to remain blocking after parsing).
func contractReviewerStructuredFindingOutput(blocking bool) string {
	finding := map[string]any{
		"message":  "Acceptance criteria do not include machine-checkable validation commands.",
		"severity": "high",
		"category": "correctness",
		"blocking": blocking,
		"evidence": "validation.commands is empty; acceptance_criteria are prose-only.",
		"state":    "candidate",
	}
	if blocking {
		finding["material_impact"] = "Executor cannot verify completion: there is no machine-checkable acceptance gate."
	}
	block := map[string]any{
		"schema":   contractReviewerResultSchema,
		"findings": []any{finding},
	}
	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		panic(err)
	}
	return "reviewer analysis\n```json\n" + string(data) + "\n```\n"
}

func contractReviewerEmptyFindingsOutput() string {
	block := map[string]any{
		"schema":   contractReviewerResultSchema,
		"findings": []any{},
	}
	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		panic(err)
	}
	return "reviewer analysis\n```json\n" + string(data) + "\n```\n"
}

// contractReviewerBlockingNoImpactOutput returns a findings block with
// blocking=true but no material_impact, exercising the downgrade path.
func contractReviewerBlockingNoImpactOutput() string {
	block := map[string]any{
		"schema": contractReviewerResultSchema,
		"findings": []any{
			map[string]any{
				"message":  "Wording of acceptance criterion could be tighter.",
				"severity": "low",
				"category": "quality",
				"blocking": true,
				"evidence": "acceptance_criteria[0] uses vague language.",
				"state":    "candidate",
				// No material_impact: triggers downgrade to advisory in the parser.
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

func runHelperContractReviewJSON(t *testing.T, app App, runID string) (contractReviewLoopResponse, string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	return response, stdout.String(), stderr.String(), code
}

func countContractReviewerAttemptResults(t *testing.T, runPaths contractRunPathSet) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(runPaths.ContractReviewAttemptsDir, "contract_reviewer_attempt_*", "result.json"))
	if err != nil {
		t.Fatalf("glob reviewer attempt results: %v", err)
	}
	return len(matches)
}

func warningsContain(warnings []string, needle string) bool {
	for _, w := range warnings {
		if strings.Contains(w, needle) {
			return true
		}
	}
	return false
}

// TestContractReviewNoReviewersIsNoOp checks that contract review with no
// contract.reviewers exits 0, prints a no-op message, creates no attempt
// artifacts, and still emits the loop started/finished ledger events.
func TestContractReviewNoReviewersIsNoOp(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "run", runID}, &stdout, &stderr)
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

func TestContractReviewSoftParseMissCorrectiveRetryParses(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_SOFT_PARSE_MISS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_PARSE_MISS_LENS", "Completeness")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_CORRECTIVE_EMIT_FINDINGS", "1")

	response, stdout, stderr, code := runHelperContractReviewJSON(t, app, runID)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if response.TerminalReason != "resolved" {
		t.Fatalf("expected resolved terminal, got %q\n%s", response.TerminalReason, stdout)
	}
	if len(response.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(response.Rounds))
	}
	round := response.Rounds[0]
	if round.ParseMiss {
		t.Fatalf("corrected parse miss must not remain unresolved: %#v", round)
	}
	if len(round.Findings) != 1 || round.Findings[0].Lens != "completeness" {
		t.Fatalf("expected corrective finding for completeness, got: %#v", round.Findings)
	}
	if !warningsContain(round.Warnings, "no valid findings block") || !warningsContain(round.Warnings, "corrective retry") {
		t.Fatalf("expected corrective retry warning, got: %#v", round.Warnings)
	}
	if got, want := countContractReviewerAttemptResults(t, runPaths), len(contractReviewLenses)+1; got != want {
		t.Fatalf("expected %d attempts including one corrective retry, got %d", want, got)
	}
}

func TestContractReviewHardParseMissEmptyStdoutNoRetry(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMPTY_STDOUT", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_PARSE_MISS_LENS", "Completeness")
	writeContractReviewResolutionsForNextTest(t, runPaths)

	response, stdout, stderr, code := runHelperContractReviewJSON(t, app, runID)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if response.TerminalReason != "reviewer_findings_unparsed" {
		t.Fatalf("expected reviewer_findings_unparsed, got %q\n%s", response.TerminalReason, stdout)
	}
	if len(response.Rounds) != 1 || !response.Rounds[0].ParseMiss {
		t.Fatalf("expected one parse-miss round, got: %#v", response.Rounds)
	}
	if !warningsContain(response.Rounds[0].Warnings, "empty stdout") {
		t.Fatalf("expected empty stdout warning, got: %#v", response.Rounds[0].Warnings)
	}
	if got, want := countContractReviewerAttemptResults(t, runPaths), len(contractReviewLenses); got != want {
		t.Fatalf("expected %d initial attempts and no corrective retry, got %d", want, got)
	}
	if isRegularFile(runPaths.ContractReviewResolutionsJSONL) {
		t.Fatalf("parse miss must not create completed resolutions artifact")
	}
	next := nextCommandsForRun(paths, runID)
	assertNextCommands(t, next, "pactum contract show "+runID)
	if slicesContain(next, "pactum contract approve "+runID) {
		t.Fatalf("parse miss must not advertise approve after reload: %v", next)
	}
}

func TestContractReviewUnresolvedSoftParseMissFailsLoud(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_SOFT_PARSE_MISS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_PARSE_MISS_LENS", "Completeness")

	response, stdout, stderr, code := runHelperContractReviewJSON(t, app, runID)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if response.TerminalReason != "reviewer_findings_unparsed" {
		t.Fatalf("expected reviewer_findings_unparsed, got %q\n%s", response.TerminalReason, stdout)
	}
	round := response.Rounds[0]
	if !round.ParseMiss || round.CleanStreak != 0 {
		t.Fatalf("unresolved parse miss must be non-clean, got: %#v", round)
	}
	if round.BlockingFindings != 0 {
		t.Fatalf("test should prove blocking_count=0 is insufficient, got %d", round.BlockingFindings)
	}
	if !warningsContain(round.Warnings, "attempt=2") || !warningsContain(round.Warnings, "no valid contract reviewer findings block parsed") {
		t.Fatalf("expected unresolved corrective parse warning, got: %#v", round.Warnings)
	}
	if got, want := countContractReviewerAttemptResults(t, runPaths), len(contractReviewLenses)+1; got != want {
		t.Fatalf("expected %d attempts including corrective retry, got %d", want, got)
	}
}

func TestContractReviewCleanEmptyFindingsBlockConverges(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)

	response, stdout, stderr, code := runHelperContractReviewJSON(t, app, runID)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if response.TerminalReason != "resolved" {
		t.Fatalf("expected resolved terminal, got %q\n%s", response.TerminalReason, stdout)
	}
	round := response.Rounds[0]
	if round.ParseMiss || len(round.Findings) != 0 || round.BlockingFindings != 0 {
		t.Fatalf("empty findings block should parse as clean, got: %#v", round)
	}
	if got, want := countContractReviewerAttemptResults(t, runPaths), len(contractReviewLenses); got != want {
		t.Fatalf("expected %d attempts, got %d", want, got)
	}
	assertNextCommands(t, response.Next, "pactum contract approve "+runID)
	assertFile(t, runPaths.ContractReviewResolutionsJSONL)
	resolutions, err := readJSONLines[contractReviewResolutionRecord](runPaths.ContractReviewResolutionsJSONL)
	if err != nil {
		t.Fatalf("cannot read resolutions.jsonl: %v", err)
	}
	if len(resolutions) != 0 {
		t.Fatalf("resolved clean review should write empty resolutions.jsonl, got: %#v", resolutions)
	}
}

func TestContractReviewCorrectiveRetryFailsToRunFailsLoud(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_SOFT_PARSE_MISS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_PARSE_MISS_LENS", "Completeness")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_CORRECTIVE_FAIL", "1")

	response, stdout, stderr, code := runHelperContractReviewJSON(t, app, runID)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if response.TerminalReason != "reviewer_findings_unparsed" {
		t.Fatalf("expected reviewer_findings_unparsed, got %q\n%s", response.TerminalReason, stdout)
	}
	round := response.Rounds[0]
	if !round.ParseMiss {
		t.Fatalf("corrective failed-to-run must remain a parse miss: %#v", round)
	}
	if len(round.SkippedLenses) != 0 {
		t.Fatalf("corrective failed-to-run must not revert to skipped_lenses: %#v", round.SkippedLenses)
	}
	if !warningsContain(round.Warnings, "corrective attempt failed") {
		t.Fatalf("expected corrective failure warning, got: %#v", round.Warnings)
	}
	if got, want := countContractReviewerAttemptResults(t, runPaths), len(contractReviewLenses)+1; got != want {
		t.Fatalf("expected %d attempts including corrective retry, got %d", want, got)
	}
}

func TestContractReviewCorrectiveRetryEmptyBlockConverges(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_SOFT_PARSE_MISS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_PARSE_MISS_LENS", "Completeness")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_CORRECTIVE_EMIT_EMPTY_BLOCK", "1")

	response, stdout, stderr, code := runHelperContractReviewJSON(t, app, runID)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if response.TerminalReason != "resolved" {
		t.Fatalf("expected resolved terminal, got %q\n%s", response.TerminalReason, stdout)
	}
	round := response.Rounds[0]
	if round.ParseMiss || len(round.Findings) != 0 || round.BlockingFindings != 0 {
		t.Fatalf("corrective findings=[] should resolve as clean, got: %#v", round)
	}
	if got, want := countContractReviewerAttemptResults(t, runPaths), len(contractReviewLenses)+1; got != want {
		t.Fatalf("expected %d attempts including corrective retry, got %d", want, got)
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
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
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

	assertNextCommands(t, response.Next, "pactum contract approve "+runID)
	assertFile(t, runPaths.ContractReviewResolutionsJSONL)
	resolutions, err := readJSONLines[contractReviewResolutionRecord](runPaths.ContractReviewResolutionsJSONL)
	if err != nil {
		t.Fatalf("cannot read resolutions.jsonl: %v", err)
	}
	if len(resolutions) != 0 {
		t.Fatalf("resolved advisory review should write empty resolutions.jsonl, got: %#v", resolutions)
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
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
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
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
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
	code := app.Run([]string{"contract", "review", "run", runID}, &stdout, &stderr)
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
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
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
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewLoopSchema {
		t.Fatalf("unexpected schema: %q", response.Schema)
	}
	if response.TerminalReason != "blockers_open" {
		t.Fatalf("expected terminal_reason=blockers_open, got %q\nfull response: %s", response.TerminalReason, stdout.String())
	}
	if len(response.Rounds) != 2 {
		t.Fatalf("expected 2 rounds (max_rounds=2), got %d", len(response.Rounds))
	}
	if response.OpenBlockingCount == 0 {
		t.Fatalf("blockers_open must report open_blocking_count > 0: %#v", response)
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
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != contractReviewLoopSchema {
		t.Fatalf("unexpected schema: %q", response.Schema)
	}
	if response.TerminalReason != "blockers_open" {
		t.Fatalf("expected terminal_reason=blockers_open, got %q\nfull response: %s", response.TerminalReason, stdout.String())
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
	code := app.Run([]string{"contract", "review", "run", runID}, &stdout, &stderr)
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

// TestContractReviewBlockersOpen checks that when blocking findings persist
// through stalemate, the loop exits as blockers_open (not stalemate) and the
// response includes a non-zero open_blocking_count.
func TestContractReviewBlockersOpen(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	app = configureHelperContractFixer(t, app, paths)
	setContractReviewPatienceConfig(t, paths, 1)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING", "1")
	// Fixer emits no revise block → version unchanged → stalemate after patience=1 round.
	t.Setenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.TerminalReason != "blockers_open" {
		t.Fatalf("expected terminal_reason=blockers_open, got %q\nfull response: %s", response.TerminalReason, stdout.String())
	}
	if response.OpenBlockingCount == 0 {
		t.Fatalf("blockers_open must report open_blocking_count > 0: %#v", response)
	}
	// Next commands must include an inspection command, not approve.
	hasInspect := false
	for _, cmd := range response.Next {
		if strings.Contains(cmd, "show") || strings.Contains(cmd, "list") || strings.Contains(cmd, "inspect") {
			hasInspect = true
		}
		if strings.Contains(cmd, "approve") {
			t.Fatalf("blockers_open next commands must not include approve: %v", response.Next)
		}
	}
	if !hasInspect {
		t.Fatalf("blockers_open next commands must include an inspection command: %v", response.Next)
	}
}

// TestContractReviewFixerNoProgress checks that when the fixer leaves the
// canonical key set of blocking findings unchanged for K=2 consecutive rounds,
// the loop exits as fixer_no_progress rather than burning all rounds.
func TestContractReviewFixerNoProgress(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	app = configureHelperContractFixer(t, app, paths)
	setContractReviewMaxRoundsConfig(t, paths, 5)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING", "1")
	// Fixer emits no revise block → version unchanged → same blocker key set each round.
	t.Setenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.TerminalReason != "fixer_no_progress" {
		t.Fatalf("expected terminal_reason=fixer_no_progress, got %q\nfull response: %s", response.TerminalReason, stdout.String())
	}
	// K=2: prevKeys set after round 1, streak=1 after round 2, streak=2 after round 3 → exit.
	if len(response.Rounds) != 3 {
		t.Fatalf("fixer_no_progress should exit after 3 rounds (K=2), got %d: %#v", len(response.Rounds), response.Rounds)
	}
	if response.OpenBlockingCount == 0 {
		t.Fatalf("fixer_no_progress must report open_blocking_count > 0: %#v", response)
	}
	if response.NoProgressStreak != 2 {
		t.Fatalf("fixer_no_progress must have no_progress_streak == 2 (K=2), got %d: %#v", response.NoProgressStreak, response)
	}
	if response.NoProgressReason == "" {
		t.Fatalf("fixer_no_progress must have non-empty no_progress_reason: %#v", response)
	}
	// Next commands must include an inspection command, not approve.
	hasInspect := false
	for _, cmd := range response.Next {
		if strings.Contains(cmd, "show") || strings.Contains(cmd, "list") || strings.Contains(cmd, "inspect") {
			hasInspect = true
		}
		if strings.Contains(cmd, "approve") {
			t.Fatalf("fixer_no_progress next commands must not include approve: %v", response.Next)
		}
	}
	if !hasInspect {
		t.Fatalf("fixer_no_progress next commands must include an inspection command: %v", response.Next)
	}
}

// TestContractReviewNonApprovableTerminalHumanOutput verifies that the
// human-readable output for blockers_open and fixer_no_progress terminals
// puts the open blocking count BEFORE the word "blocking" (e.g. "2 open
// blocking findings remain") rather than after.
func TestContractReviewNonApprovableTerminalHumanOutput(t *testing.T) {
	t.Run("blockers_open", func(t *testing.T) {
		root := t.TempDir()
		app, paths, runID := setupContractRun(t, root)
		app = configureHelperContractReviewers(t, app, paths, "helper")
		app = configureHelperContractFixer(t, app, paths)
		setContractReviewPatienceConfig(t, paths, 1)
		t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
		t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
		t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
		t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING", "1")
		t.Setenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS", "1")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "review", "run", runID}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
		}
		got := stdout.String()
		if !strings.Contains(got, "blockers_open") {
			t.Fatalf("human output missing terminal reason:\n%s", got)
		}
		// Count must appear immediately before the word "blocking".
		if !strings.Contains(got, "open blocking findings remain") {
			t.Fatalf("human output must contain '<N> open blocking findings remain':\n%s", got)
		}
	})

	t.Run("fixer_no_progress", func(t *testing.T) {
		root := t.TempDir()
		app, paths, runID := setupContractRun(t, root)
		app = configureHelperContractReviewers(t, app, paths, "helper")
		app = configureHelperContractFixer(t, app, paths)
		setContractReviewMaxRoundsConfig(t, paths, 5)
		t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
		t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
		t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
		t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING", "1")
		t.Setenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS", "1")

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "review", "run", runID}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
		}
		got := stdout.String()
		if !strings.Contains(got, "fixer_no_progress") {
			t.Fatalf("human output missing terminal reason:\n%s", got)
		}
		if !strings.Contains(got, "open blocking findings remain") {
			t.Fatalf("human output must contain '<N> open blocking findings remain':\n%s", got)
		}
	})
}

// TestContractReviewFindingsJSONLWrittenAtLoopEnd checks that a successful
// contract review loop run writes findings.jsonl under contract/reviewer/.
func TestContractReviewFindingsJSONLWrittenAtLoopEnd(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	// blocking=false (default) → loop exits resolved.

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	// findings.jsonl must exist and contain entries from the last round.
	assertFile(t, runPaths.ContractReviewFindingsJSONL)
	lines, err := readJSONLines[contractReviewFindingLine](runPaths.ContractReviewFindingsJSONL)
	if err != nil {
		t.Fatalf("cannot read findings.jsonl: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("findings.jsonl must have at least one entry when reviewer emits findings")
	}
	for _, l := range lines {
		if l.Message == "" {
			t.Fatalf("findings.jsonl entry has empty message: %#v", l)
		}
		if l.Category == "" {
			t.Fatalf("findings.jsonl entry has empty category: %#v", l)
		}
		// blocking=false because EMIT_BLOCKING is not set in this test.
		if l.Blocking {
			t.Fatalf("findings.jsonl entry has unexpected blocking=true (no EMIT_BLOCKING set): %#v", l)
		}
		// state is always set by the helper (to "candidate"); verify it persists.
		if l.State == "" {
			t.Fatalf("findings.jsonl entry has empty state (state must be persisted): %#v", l)
		}
	}
}

// TestContractReviewFindingsJSONLNewFieldsPersisted verifies that material_impact,
// fix_direction, uncertainty, and state are persisted to findings.jsonl for
// blocking findings that carry those fields.
func TestContractReviewFindingsJSONLNewFieldsPersisted(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	app = configureHelperContractFixer(t, app, paths)
	setContractReviewMaxRoundsConfig(t, paths, 1)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING", "1")
	// Fixer emits no revise block → max_rounds reached → blockers_open with blocking findings in JSONL.
	t.Setenv("PACTUM_CONTRACT_FIXER_HELPER_PROCESS", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.TerminalReason != "blockers_open" {
		t.Fatalf("expected terminal_reason=blockers_open, got %q", response.TerminalReason)
	}

	assertFile(t, runPaths.ContractReviewFindingsJSONL)
	lines, err := readJSONLines[contractReviewFindingLine](runPaths.ContractReviewFindingsJSONL)
	if err != nil {
		t.Fatalf("cannot read findings.jsonl: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("findings.jsonl must have entries when blocking findings remain")
	}
	for _, l := range lines {
		// state must be persisted for every finding.
		if l.State == "" {
			t.Fatalf("findings.jsonl entry has empty state: %#v", l)
		}
		// Blocking findings must carry material_impact (the helper always sets it when blocking=true).
		if l.Blocking && l.MaterialImpact == "" {
			t.Fatalf("blocking finding in findings.jsonl has empty material_impact: %#v", l)
		}
	}
	// At least one blocking finding with material_impact must be present.
	var sawBlockingWithImpact bool
	for _, l := range lines {
		if l.Blocking && l.MaterialImpact != "" {
			sawBlockingWithImpact = true
			break
		}
	}
	if !sawBlockingWithImpact {
		t.Fatalf("expected at least one blocking finding with material_impact in findings.jsonl, got: %#v", lines)
	}
}

// TestContractApproveGuardBlockingFindings checks that contract approve refuses
// when findings.jsonl contains blocking=true entries.
func TestContractApproveGuardBlockingFindings(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	// Configure a reviewer so the guard is enforced.
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	// Write a findings.jsonl with one blocking entry.
	blocking := contractReviewFindingLine{Category: "completeness", Message: "missing acceptance criteria", Blocking: true}
	blockingJSON, _ := json.Marshal(blocking)
	assertNoError(t, activeStore.WriteBytes(runPaths.ContractReviewFindingsJSONL, append(blockingJSON, '\n'), 0o644))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("contract approve should fail when blocking contract-review findings remain; got exit 0\nstdout: %s", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "blocking") {
		t.Fatalf("error message should mention blocking findings:\n%s", got)
	}
}

// TestContractApproveGuardFailClosed checks that contract approve refuses when
// the findings.jsonl artifact is absent or malformed (fail-closed).
func TestContractApproveGuardFailClosed(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		root := t.TempDir()
		app, paths, runID := setupContractRun(t, root)
		// Configure a reviewer so the guard is enforced.
		setContractReviewersConfig(t, paths, "helper")
		registerTestAgents(t, paths, "helper")
		// Do NOT write any findings.jsonl → absent → fail-closed.

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("contract approve must refuse when findings.jsonl is absent; got exit 0\nstdout: %s", stdout.String())
		}
	})

	t.Run("malformed", func(t *testing.T) {
		root := t.TempDir()
		app, paths, runID := setupContractRun(t, root)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		// Configure a reviewer so the guard is enforced.
		setContractReviewersConfig(t, paths, "helper")
		registerTestAgents(t, paths, "helper")
		// Write a malformed (non-JSON) findings.jsonl → parse error → fail-closed.
		assertNoError(t, activeStore.WriteBytes(runPaths.ContractReviewFindingsJSONL, []byte("not-json\n"), 0o644))

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("contract approve must refuse when findings.jsonl is malformed; got exit 0\nstdout: %s", stdout.String())
		}
		if got := stderr.String(); !strings.Contains(got, "malformed") {
			t.Fatalf("error message should mention malformed: %s", got)
		}
	})

	// Guard fires via the isRegularFile branch even when reviewers are no longer
	// configured — removing reviewers after a blocking run cannot bypass the guard.
	t.Run("blocking_findings_reviewers_removed", func(t *testing.T) {
		root := t.TempDir()
		app, paths, runID := setupContractRun(t, root)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		// Do NOT configure any reviewers — the guard must still fire because the
		// findings artifact exists with a blocking entry.
		_ = paths
		blocking := contractReviewFindingLine{Category: "correctness", Message: "missing acceptance criteria", Blocking: true}
		blockingJSON, err := json.Marshal(blocking)
		assertNoError(t, err)
		assertNoError(t, activeStore.WriteBytes(runPaths.ContractReviewFindingsJSONL, append(blockingJSON, '\n'), 0o644))

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("contract approve must refuse when findings.jsonl has blocking=true and reviewers are removed; got exit 0\nstdout: %s", stdout.String())
		}
		if got := stderr.String(); !strings.Contains(got, "blocking") {
			t.Fatalf("error message should mention blocking: %s", got)
		}
	})
}

func TestContractReviewerPromptRequiresMandatoryFindingsBlock(t *testing.T) {
	contract := draftContract{Goal: "test goal"}
	for _, lens := range contractReviewLenses {
		prompt := renderContractReviewerPrompt(contract, lens)
		if !strings.Contains(prompt, "You MUST also include exactly one structured findings block") {
			t.Errorf("lens %s: prompt does not mandate exactly one structured findings block", lens.Key)
		}
		if !strings.Contains(prompt, "The block is mandatory") {
			t.Errorf("lens %s: prompt does not mark the block mandatory", lens.Key)
		}
		if !strings.Contains(prompt, `"findings": []`) {
			t.Errorf("lens %s: prompt does not require findings=[] for clean reviews", lens.Key)
		}
		if strings.Contains(prompt, "Do not include an empty findings block") {
			t.Errorf("lens %s: prompt still contains old omit-empty-block instruction", lens.Key)
		}
	}
}

// TestContractReviewerPromptContainsContractSchema verifies that the rendered
// contract reviewer prompt uses contractReviewerResultSchema (not the shared
// code-review schema) in its sample JSON output block, and that the precision
// discipline rule text is present.
func TestContractReviewerPromptContainsContractSchema(t *testing.T) {
	contract := draftContract{Goal: "test goal"}
	ruleChecks := []struct {
		name    string
		wantStr string
	}{
		{"hard-rule blocking", "HARD RULE: blocking=true is allowed ONLY"},
		{"wording advisory rule", "Wording, style, naming, redundancy"},
		{"material_impact required", "Every blocking finding MUST include a concrete material_impact"},
		{"advisory-when-no-impact rule", "If you cannot state a concrete material_impact, mark the finding blocking=false"},
		{"recall-first framing", "recall-first"},
		{"state=candidate framing", "state=candidate with explicit uncertainty"},
	}
	for _, lens := range contractReviewLenses {
		prompt := renderContractReviewerPrompt(contract, lens)
		if !strings.Contains(prompt, contractReviewerResultSchema) {
			t.Errorf("lens %s: prompt does not contain %q", lens.Key, contractReviewerResultSchema)
		}
		if strings.Contains(prompt, "reviewer_findings.v1alpha1") {
			t.Errorf("lens %s: prompt contains code-review schema string, expected contract-local schema only", lens.Key)
		}
		for _, rc := range ruleChecks {
			if !strings.Contains(prompt, rc.wantStr) {
				t.Errorf("lens %s: prompt missing rule text (%s): %q", lens.Key, rc.name, rc.wantStr)
			}
		}
	}
}

func TestContractReviewFixerPromptUsesNestedScopeSchema(t *testing.T) {
	contract := draftContract{
		Goal: "test goal",
		Scope: draftContractScope{
			In:  []string{"existing in-scope item"},
			Out: []string{"existing out-of-scope item"},
		},
	}
	prompt := renderContractReviewFixerPrompt(contract, "version-1", []contractReviewFinding{
		{
			Reviewer: "reviewer",
			Lens:     "scope-boundaries",
			Severity: "high",
			Blocking: true,
			Message:  "Scope needs an explicit in/out update.",
		},
	})

	for _, want := range []string{
		"For scope changes, use contract.scope.in and contract.scope.out as nested arrays.",
		`"scope": {`,
		`      "in": ["...all in-scope items to keep, including unchanged and updated entries..."],`,
		`      "out": ["...all out-of-scope items to keep, including unchanged and updated entries..."]`,
		"Do NOT use top-level contract.scope_in or contract.scope_out fields; they are invalid.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("fixer prompt missing %q:\n%s", want, prompt)
		}
	}

	allowedForbiddenFieldWarning := "Do NOT use top-level contract.scope_in or contract.scope_out fields; they are invalid."
	promptWithoutWarning := strings.Replace(prompt, allowedForbiddenFieldWarning, "", 1)
	for _, invalidField := range []string{"scope_in", "scope_out"} {
		if strings.Contains(promptWithoutWarning, invalidField) {
			t.Fatalf("fixer prompt mentions invalid top-level scope field %q outside the explicit warning:\n%s", invalidField, prompt)
		}
	}
}

func TestContractReviewFixerPromptWarnsListFieldsAreWholeReplacements(t *testing.T) {
	contract := draftContract{
		Goal: "test goal",
		Scope: draftContractScope{
			In:  []string{"existing in-scope item"},
			Out: []string{"existing out-of-scope item"},
		},
		AcceptanceCriteria: []string{"stdout stays clean", "docs updated", "make check passes"},
		Validation: draftValidation{
			Commands: []string{"go test ./internal/app", "make check"},
		},
		Assumptions: []string{"contract revise keeps partial-replace semantics"},
	}
	prompt := renderContractReviewFixerPrompt(contract, "version-1", []contractReviewFinding{
		{
			Reviewer: "reviewer",
			Lens:     "testability",
			Severity: "high",
			Blocking: true,
			Message:  "Acceptance criteria need a focused update.",
		},
	})

	for _, want := range []string{
		"List fields are whole-list replacements in contract revise",
		"include every item that should remain, including unchanged items",
		"For acceptance_criteria, validation.commands, assumptions, scope.in, and scope.out, do NOT output only the changed or newly added entries.",
		`"acceptance_criteria": ["...all criteria to keep, including unchanged and updated entries..."]`,
		`"validation": {"commands": ["...all validation commands to keep, including unchanged and updated entries..."]}`,
		`"assumptions": ["...all assumptions to keep, including unchanged and updated entries..."]`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("fixer prompt missing whole-list guidance %q:\n%s", want, prompt)
		}
	}

	for _, staleExample := range []string{
		`"acceptance_criteria": ["...updated criteria..."]`,
		`"validation": {"commands": ["...updated commands..."]}`,
	} {
		if strings.Contains(prompt, staleExample) {
			t.Fatalf("fixer prompt still suggests partial array replacement %q:\n%s", staleExample, prompt)
		}
	}
}

// TestContractReviewDowngradesBlockingWithoutMaterialImpact verifies that a
// finding parsed with blocking=true and empty material_impact is downgraded to
// advisory (blocking=false), a warning is recorded, no fixer is invoked, and
// the loop resolves cleanly (terminal_reason=resolved).
func TestContractReviewDowngradesBlockingWithoutMaterialImpact(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_BLOCKING_NO_IMPACT", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review exited %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var response contractReviewLoopResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))

	// All blocking findings lack material_impact → downgraded to advisory → resolved.
	if response.TerminalReason != "resolved" {
		t.Fatalf("expected terminal_reason=resolved (blocking downgraded to advisory), got %q\nfull response: %s", response.TerminalReason, stdout.String())
	}
	if len(response.Rounds) == 0 {
		t.Fatalf("expected at least one round")
	}
	round := response.Rounds[0]

	if round.BlockingFindings != 0 {
		t.Fatalf("blocking_findings must be 0 after downgrade, got %d", round.BlockingFindings)
	}
	if round.FixerAttemptID != "" {
		t.Fatalf("fixer must not be invoked when all blocking findings are downgraded, got fixer_attempt_id=%q", round.FixerAttemptID)
	}

	// The downgrade warning must appear in round warnings.
	hasDowngradeWarning := false
	for _, w := range round.Warnings {
		if strings.Contains(w, "downgraded") && strings.Contains(w, "material_impact") {
			hasDowngradeWarning = true
			break
		}
	}
	if !hasDowngradeWarning {
		t.Fatalf("expected downgrade warning in round warnings, got: %v", round.Warnings)
	}

	// All findings are advisory after downgrade.
	for _, f := range round.Findings {
		if f.Blocking {
			t.Fatalf("finding must be non-blocking after downgrade: %#v", f)
		}
	}
}

// TestContractReviewFindingNewFieldPropagation verifies that material_impact,
// fix_direction, uncertainty, and state are propagated from reviewer output to
// the parsed contractReviewFinding, and that a blank/unrecognized state defaults
// to "candidate".
func TestContractReviewFindingNewFieldPropagation(t *testing.T) {
	input := contractReviewerFindingInput{
		Message:        "Missing validation command.",
		Severity:       "high",
		Category:       "validation",
		Blocking:       func() *bool { b := true; return &b }(),
		Evidence:       "validation.commands is empty.",
		MaterialImpact: "Gate cannot verify the implementation.",
		FixDirection:   "Add a runnable validation command.",
		Uncertainty:    "Might be present elsewhere.",
		State:          "confirmed",
	}
	f := contractFindingFromInput("reviewer", "testability", input)
	if f.MaterialImpact != "Gate cannot verify the implementation." {
		t.Errorf("material_impact not propagated: %q", f.MaterialImpact)
	}
	if f.FixDirection != "Add a runnable validation command." {
		t.Errorf("fix_direction not propagated: %q", f.FixDirection)
	}
	if f.Uncertainty != "Might be present elsewhere." {
		t.Errorf("uncertainty not propagated: %q", f.Uncertainty)
	}
	if f.State != "confirmed" {
		t.Errorf("state not propagated: %q", f.State)
	}

	// Blank state defaults to "candidate".
	input.State = ""
	f2 := contractFindingFromInput("reviewer", "testability", input)
	if f2.State != "candidate" {
		t.Errorf("blank state must default to candidate, got %q", f2.State)
	}

	// Unrecognized state defaults to "candidate".
	input.State = "unknown-value"
	f3 := contractFindingFromInput("reviewer", "testability", input)
	if f3.State != "candidate" {
		t.Errorf("unrecognized state must default to candidate, got %q", f3.State)
	}
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}
