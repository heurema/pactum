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

// TestPlanReviewHelperProcess is the subprocess used by plan review tests.
// It reads the prompt from stdin, checks for the required heading, and
// conditionally emits a structured findings block.
//
// PACTUM_PLAN_REVIEWER_HELPER_PROCESS=1: activate helper.
// PACTUM_PLAN_REVIEWER_EMIT_FINDINGS=1: emit a structured findings block.
// PACTUM_PLAN_REVIEWER_EMIT_BLOCKING=1: make findings blocking=true.
// PACTUM_PLAN_REVIEWER_PARSE_MISS_DIR=<dir>: emit no block on first call;
//
//	on second call emit a block (tracked via a flag file in <dir>).
func TestPlanReviewHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_PLAN_REVIEWER_HELPER_PROCESS") != "1" {
		return
	}
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("stdin_has_plan_review_heading=%t\n", strings.Contains(string(stdin), "# Plan Review"))
	fmt.Printf("stdin_has_findings_schema=%t\n", strings.Contains(string(stdin), reviewerFindingsSchema))

	// Parse-miss mode: no block on first call, block on second.
	if dir := os.Getenv("PACTUM_PLAN_REVIEWER_PARSE_MISS_DIR"); dir != "" {
		flagFile := filepath.Join(dir, "plan_reviewer_called")
		if _, statErr := os.Stat(flagFile); os.IsNotExist(statErr) {
			// First call: emit prose only.
			if writeErr := os.WriteFile(flagFile, []byte("called"), 0o644); writeErr != nil {
				fmt.Fprintf(os.Stderr, "flag write error: %v\n", writeErr)
				os.Exit(2)
			}
			fmt.Fprintln(os.Stdout, "I reviewed the plan and found no issues worth mentioning.")
			fmt.Fprintln(os.Stderr, "plan-reviewer-stderr-line")
			os.Exit(0)
		}
		// Second call (corrective retry): emit a block.
		fmt.Print(planReviewerStructuredFindingOutput(false))
		fmt.Fprintln(os.Stderr, "plan-reviewer-corrective-stderr-line")
		os.Exit(0)
	}

	if os.Getenv("PACTUM_PLAN_REVIEWER_EMIT_FINDINGS") == "1" {
		blocking := os.Getenv("PACTUM_PLAN_REVIEWER_EMIT_BLOCKING") == "1"
		fmt.Print(planReviewerStructuredFindingOutput(blocking))
	} else {
		fmt.Print(planReviewerEmptyFindingsOutput())
	}
	fmt.Fprintln(os.Stderr, "plan-reviewer-stderr-line")
	os.Exit(0)
}

func planReviewerEmptyFindingsOutput() string {
	var b strings.Builder
	fmt.Fprintln(&b, "After reviewing the plan DAG, I found no issues.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", reviewerFindingsSchema)
	fmt.Fprintln(&b, `  "findings": []`)
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	return b.String()
}

func planReviewerStructuredFindingOutput(blocking bool) string {
	var b strings.Builder
	fmt.Fprintln(&b, "After reviewing the plan DAG, I found one finding.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", reviewerFindingsSchema)
	fmt.Fprintln(&b, `  "findings": [`)
	fmt.Fprintln(&b, "    {")
	fmt.Fprintln(&b, `      "lens": "granularity",`)
	fmt.Fprintln(&b, `      "message": "Task t1 is too coarse.",`)
	fmt.Fprintf(&b, "      \"blocking\": %v,\n", blocking)
	fmt.Fprintln(&b, `      "evidence": "expected_files spans multiple packages"`)
	fmt.Fprintln(&b, "    }")
	fmt.Fprintln(&b, "  ]")
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	return b.String()
}

// configurePlanReviewHelpers registers "helper" in config.pipeline.plan_review.by
// and sets the app registry to use the helper process.
func configurePlanReviewHelpers(t *testing.T, app App, paths artifacts.Paths, names ...string) App {
	t.Helper()
	registerTestAgents(t, paths, names...)
	config := readConfigForTest(t, paths.Config)
	config.Pipeline.PlanReview.By = stageBy(names)
	assertNoError(t, writeYAML(paths.Config, config))
	helpers := testHelperDescriptors(names, "TestPlanReviewHelperProcess")
	app.AgentRegistry = testAgentRegistry(helpers...)
	return app
}

// addPlanToRun revises the contract to include a plan with a single valid task.
func addPlanToRun(t *testing.T, app App, runPaths contractRunPathSet, runID string) {
	t.Helper()
	plan := contractPlan{Tasks: []planTask{{
		ID:            "t1",
		ExpectedFiles: []string{"internal/app/plan.go"},
		Acceptance:    []string{"plan review works"},
		Validation:    []string{"go test ./internal/app -run TestPlan"},
	}}}
	doc := map[string]any{
		"base_version": readVersionForTest(t, runPaths),
		"contract":     map[string]any{"plan": plan},
	}
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", writeTempJSON(t, doc), "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add plan exited %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

// TestPlanReviewNoPlanIsNoOp checks that plan review with no plan exits 0
// and prints a no-op message without invoking any agent.
func TestPlanReviewNoPlanIsNoOp(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "review", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan review exited %d: stderr=%s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "No plan") {
		t.Fatalf("expected no-op message, got:\n%s", got)
	}
}

// TestPlanReviewNoPlanIsNoOpJSON checks the --json output when no plan exists.
func TestPlanReviewNoPlanIsNoOpJSON(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan review exited %d: stderr=%s", code, stderr.String())
	}
	var resp planReviewJSONResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("parse JSON: %v — stdout: %s", err, stdout.String())
	}
	if !resp.NoPlan {
		t.Fatalf("expected no_plan=true, got: %+v", resp)
	}
	if resp.Findings == nil || len(resp.Findings) != 0 {
		t.Fatalf("expected empty findings, got: %+v", resp.Findings)
	}
}

// TestPlanReviewNoReviewersIsNoOp checks that plan review with no plan_review.by
// exits 0 and prints a no-op message.
func TestPlanReviewNoReviewersIsNoOp(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	addPlanToRun(t, app, runPaths, runID)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "review", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan review exited %d: stderr=%s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "No plan_review reviewers") {
		t.Fatalf("expected no-reviewers message, got:\n%s", got)
	}
}

// TestPlanReviewNoReviewersIsNoOpJSON checks --json output when no reviewers configured.
func TestPlanReviewNoReviewersIsNoOpJSON(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	addPlanToRun(t, app, runPaths, runID)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "review", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan review exited %d: stderr=%s", code, stderr.String())
	}
	var resp planReviewJSONResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("parse JSON: %v — stdout: %s", err, stdout.String())
	}
	if !resp.NoReviewers {
		t.Fatalf("expected no_reviewers=true, got: %+v", resp)
	}
}

// TestPlanReviewFindsFindings checks that plan review with a configured reviewer
// and a plan invokes the reviewer, writes artifacts, and exits 1 with findings.
func TestPlanReviewFindsFindings(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configurePlanReviewHelpers(t, app, paths, "helper")
	addPlanToRun(t, app, runPaths, runID)
	t.Setenv("PACTUM_PLAN_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_PLAN_REVIEWER_EMIT_FINDINGS", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "review", runID}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1 (findings), got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	// Live reviewer output goes to stderr; human result goes to stdout.
	if got := stderr.String(); !strings.Contains(got, "plan-reviewer-stderr-line") {
		t.Fatalf("expected live reviewer output in stderr, got:\n%s", got)
	}
	if got := stdout.String(); !strings.Contains(got, "finding") {
		t.Fatalf("expected findings in stdout, got:\n%s", got)
	}
	// No extra error line from the app.go error handler.
	if got := stderr.String(); strings.Contains(got, "pactum plan review:") {
		t.Fatalf("unexpected error line in stderr:\n%s", got)
	}
}

// TestPlanReviewNoFindingsExitsZero checks that a reviewer emitting no
// structured block (clean review) exits 0.
func TestPlanReviewNoFindingsExitsZero(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configurePlanReviewHelpers(t, app, paths, "helper")
	addPlanToRun(t, app, runPaths, runID)
	t.Setenv("PACTUM_PLAN_REVIEWER_HELPER_PROCESS", "1")
	// EMIT_FINDINGS is not set → helper emits an empty findings block

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "review", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0 (no findings), got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "No findings") {
		t.Fatalf("expected no-findings message, got:\n%s", got)
	}
}

// TestPlanReviewArtifactsWritten checks that the required artifacts are
// created: prompt.txt, attempt-1.txt, and findings.json.
func TestPlanReviewArtifactsWritten(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configurePlanReviewHelpers(t, app, paths, "helper")
	addPlanToRun(t, app, runPaths, runID)
	t.Setenv("PACTUM_PLAN_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_PLAN_REVIEWER_EMIT_FINDINGS", "1")

	var stdout, stderr bytes.Buffer
	app.Run([]string{"plan", "review", runID}, &stdout, &stderr) //nolint:errcheck

	reviewerDir := filepath.Join(runPaths.PlanReviewDir, "helper")
	assertFile(t, filepath.Join(reviewerDir, "prompt.txt"))
	assertFile(t, filepath.Join(reviewerDir, "attempt-1.txt"))
	assertFile(t, runPaths.PlanReviewFindingsJSON)

	// findings.json must contain the helper's finding.
	findingsData := mustReadFile(t, runPaths.PlanReviewFindingsJSON)
	var findings []planReviewFinding
	if err := json.Unmarshal([]byte(findingsData), &findings); err != nil {
		t.Fatalf("parse findings.json: %v — content: %s", err, findingsData)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Agent != "helper" || f.Lens != "granularity" || f.Title == "" || f.Severity == "" {
		t.Fatalf("finding missing required fields: %+v", f)
	}
}

// TestPlanReviewJSONOutput checks the --json output shape with findings.
func TestPlanReviewJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configurePlanReviewHelpers(t, app, paths, "helper")
	addPlanToRun(t, app, runPaths, runID)
	t.Setenv("PACTUM_PLAN_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_PLAN_REVIEWER_EMIT_FINDINGS", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "review", runID, "--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d: stderr=%s", code, stderr.String())
	}

	var resp planReviewJSONResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("parse JSON: %v — stdout: %s", err, stdout.String())
	}
	if resp.NoPlan || resp.NoReviewers {
		t.Fatalf("unexpected no_plan/no_reviewers flags: %+v", resp)
	}
	if len(resp.Findings) == 0 {
		t.Fatalf("expected findings in JSON, got: %+v", resp)
	}
	f := resp.Findings[0]
	if f.Agent == "" || f.Lens == "" || f.Title == "" || f.Severity == "" {
		t.Fatalf("finding missing required fields: %+v", f)
	}
}

// TestPlanReviewParseMissTriggersCorrectiveRetry checks that when the reviewer
// emits prose without a valid findings block, a corrective retry is issued and
// both attempt-1.txt and attempt-2.txt are written.
func TestPlanReviewParseMissTriggersCorrectiveRetry(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configurePlanReviewHelpers(t, app, paths, "helper")
	addPlanToRun(t, app, runPaths, runID)
	t.Setenv("PACTUM_PLAN_REVIEWER_HELPER_PROCESS", "1")
	parseMissDir := t.TempDir()
	t.Setenv("PACTUM_PLAN_REVIEWER_PARSE_MISS_DIR", parseMissDir)

	var stdout, stderr bytes.Buffer
	// First attempt produces no block → corrective retry → corrective produces a block → exit 1.
	code := app.Run([]string{"plan", "review", runID}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1 (corrective found findings), got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	reviewerDir := filepath.Join(runPaths.PlanReviewDir, "helper")
	assertFile(t, filepath.Join(reviewerDir, "attempt-1.txt"))
	assertFile(t, filepath.Join(reviewerDir, "attempt-2.txt"))
	assertFile(t, filepath.Join(reviewerDir, "corrective-prompt.txt"))

	// attempt-1.txt must be the prose-only output (no block).
	attempt1 := mustReadFile(t, filepath.Join(reviewerDir, "attempt-1.txt"))
	if strings.Contains(attempt1, reviewerFindingsSchema) {
		t.Fatalf("attempt-1.txt should not contain findings schema (expected prose only)")
	}
	// attempt-2.txt must contain the findings block.
	attempt2 := mustReadFile(t, filepath.Join(reviewerDir, "attempt-2.txt"))
	if !strings.Contains(attempt2, reviewerFindingsSchema) {
		t.Fatalf("attempt-2.txt should contain findings schema, got:\n%s", attempt2)
	}
}

// TestPlanReviewPromptDocumentsLensesAndSchema checks that the rendered plan
// reviewer prompt includes all five lenses and the required schema marker.
func TestPlanReviewPromptDocumentsLensesAndSchema(t *testing.T) {
	contract := draftContract{
		Goal: "test plan review prompt",
		Plan: &contractPlan{Tasks: []planTask{{
			ID:            "t1",
			ExpectedFiles: []string{"internal/app/foo.go"},
			Acceptance:    []string{"tests pass"},
			Validation:    []string{"go test ./internal/app/..."},
		}}},
	}
	prompt := renderPlanReviewerPrompt(contract)

	for _, lens := range planReviewLenses {
		if !strings.Contains(prompt, lens) {
			t.Errorf("prompt missing lens %q", lens)
		}
	}
	if !strings.Contains(prompt, reviewerFindingsSchema) {
		t.Errorf("prompt missing required schema %q", reviewerFindingsSchema)
	}
	if !strings.Contains(prompt, "# Plan Review") {
		t.Errorf("prompt missing heading '# Plan Review'")
	}
}

// TestPlanReviewMultipleReviewers checks that plan review with two configured
// reviewer agents runs both, writes per-reviewer artifacts, and aggregates findings.
func TestPlanReviewMultipleReviewers(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configurePlanReviewHelpers(t, app, paths, "helper1", "helper2")
	addPlanToRun(t, app, runPaths, runID)
	t.Setenv("PACTUM_PLAN_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_PLAN_REVIEWER_EMIT_FINDINGS", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "review", runID}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1 (findings from both reviewers), got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	// Both reviewer artifact directories must exist.
	for _, name := range []string{"helper1", "helper2"} {
		reviewerDir := filepath.Join(runPaths.PlanReviewDir, name)
		assertFile(t, filepath.Join(reviewerDir, "prompt.txt"))
		assertFile(t, filepath.Join(reviewerDir, "attempt-1.txt"))
	}

	// findings.json must contain findings from both reviewers.
	findingsData := mustReadFile(t, runPaths.PlanReviewFindingsJSON)
	var findings []planReviewFinding
	if err := json.Unmarshal([]byte(findingsData), &findings); err != nil {
		t.Fatalf("parse findings.json: %v", err)
	}
	if len(findings) < 2 {
		t.Fatalf("expected at least 2 findings (one per reviewer), got %d: %+v", len(findings), findings)
	}
	seen := map[string]bool{}
	for _, f := range findings {
		seen[f.Agent] = true
	}
	for _, name := range []string{"helper1", "helper2"} {
		if !seen[name] {
			t.Fatalf("no findings recorded from reviewer %q", name)
		}
	}
}

// TestPlanReviewBlockingFindingExitsOne checks that a blocking finding exits 1
// and is surfaced in the JSON response with severity="blocking".
func TestPlanReviewBlockingFindingExitsOne(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configurePlanReviewHelpers(t, app, paths, "helper")
	addPlanToRun(t, app, runPaths, runID)
	t.Setenv("PACTUM_PLAN_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_PLAN_REVIEWER_EMIT_FINDINGS", "1")
	t.Setenv("PACTUM_PLAN_REVIEWER_EMIT_BLOCKING", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "review", runID, "--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1 (blocking finding), got %d", code)
	}

	var resp planReviewJSONResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &resp))
	if len(resp.Findings) == 0 {
		t.Fatal("expected findings in response")
	}
	if resp.Findings[0].Severity != "blocking" {
		t.Fatalf("expected severity=blocking, got %q", resp.Findings[0].Severity)
	}
}
