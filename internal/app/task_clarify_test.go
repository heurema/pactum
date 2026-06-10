package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// taskNewSecondRunID is the id task new assigns to the second run created under
// the fixed test clock (reserveContractRunDir suffixes same-second runs).
const taskNewSecondRunID = "run_20260531_184012_02"

func TestTaskNewClarifyRequiresYes(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	app = configureHelperClarifiers(t, app, paths, "helper")

	runsBefore, err := listRunIDs(paths)
	assertNoError(t, err)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"task", "new", "integrate clarify", "--clarify", "--reviewer", "helper"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("task new --clarify without --yes exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "task new --clarify requires --yes") {
		t.Fatalf("task new --clarify confirmation stderr mismatch:\n%s", got)
	}
	// The refusal fires before run creation: nothing to clean up or re-use.
	runsAfter, err := listRunIDs(paths)
	assertNoError(t, err)
	if len(runsAfter) != len(runsBefore) {
		t.Fatalf("refused task new --clarify must not create a run: before %v, after %v", runsBefore, runsAfter)
	}
}

// TestTaskNewClarifyConvergesAndRendersSummary drives task new --clarify with
// the standard clarifier helper (one blocking high-confidence question, one
// non-blocking medium question): the loop converges in round 1 by auto-resolving
// the blocking question, and the command renders the created run, the loop
// summary, and an empty questions-awaiting section.
func TestTaskNewClarifyConvergesAndRendersSummary(t *testing.T) {
	root := t.TempDir()
	app, paths, firstRunID := setupContractRun(t, root)
	app = configureHelperClarifiers(t, app, paths, "helper")

	t.Setenv("PACTUM_CLARIFIER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"task", "new", "integrate clarify", "--clarify", "--reviewer", "helper", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("task new --clarify exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Run created",
		"id: " + taskNewSecondRunID,
		"status: contract_draft",
		"task: integrate clarify",
		"current: yes",
		"Clarify loop finished",
		"terminal reason: converged",
		"converged: yes",
		"- round 1: questions created 2, auto-resolved 1, open blocking 0, clarifier clarifier_attempt_001",
		"Questions awaiting you:",
		"(none — no open blocking questions remain)",
		"Review the contract draft, then: pactum contract approve",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("task new --clarify output missing %q:\n%s", want, got)
		}
	}

	// The loop ran against the new run, not the pre-existing one, and recorded
	// the auto-resolved answer there.
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, taskNewSecondRunID))
	answers, err := readClarificationAnswers(runPaths.AnswersJSONL)
	assertNoError(t, err)
	if len(answers) != 1 || answers[0].QuestionID != "q_001" || answers[0].Source != "auto_recommended" {
		t.Fatalf("auto-resolved answer mismatch: %#v", answers)
	}
	assertFile(t, runPaths.ClarifyLoopSummaryJSON)
	assertNoFile(t, contractRunPaths(filepath.Join(paths.RunsDir, firstRunID)).ClarifyLoopSummaryJSON)
	if current, _ := readCurrentRun(paths); current != taskNewSecondRunID {
		t.Fatalf("current run = %q, want %q", current, taskNewSecondRunID)
	}
}

// TestTaskNewClarifyListsRemainingBlockingQuestions drives task new --clarify
// with the sequence helper that emits one blocking medium-confidence question:
// the loop ends needs_human and the question lands in the questions-awaiting
// section with its kind, confidence, and recommended answer.
func TestTaskNewClarifyListsRemainingBlockingQuestions(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	app = configureClarifyLoopHelpers(t, app, paths)
	setClarifyLoopHelperEnv(t, filepath.Join(stateDir, "sequence"), "medium_then_empty")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"task", "new", "integrate clarify", "--clarify", "--reviewer", clarifyLoopClarifierName, "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("task new --clarify exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Run created",
		"id: " + taskNewSecondRunID,
		"Clarify loop finished",
		"terminal reason: needs_human",
		"Questions awaiting you:",
		"- q_001 What retention policy should round 1 assume?",
		"kind: scope",
		"recommended answer (confidence medium): Keep 30 days.",
		"Answer each question with: pactum clarify answer <question_id> \"<answer>\", then: pactum contract approve",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("task new --clarify output missing %q:\n%s", want, got)
		}
	}
	state := readRunState(t, contractRunPaths(filepath.Join(paths.RunsDir, taskNewSecondRunID)).RunJSON)
	if state.Status != "clarifying" {
		t.Fatalf("run status = %q, want clarifying (open blocking question remains)", state.Status)
	}
}

func TestTaskNewClarifyJSONEmbedsRunAndLoopSummary(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	app = configureHelperClarifiers(t, app, paths, "helper")

	t.Setenv("PACTUM_CLARIFIER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"task", "new", "integrate clarify", "--clarify", "--reviewer", "helper", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("task new --clarify --json exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "Run created") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}

	var response taskNewClarifyResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != taskNewClarifySchema {
		t.Fatalf("schema = %q, want %q", response.Schema, taskNewClarifySchema)
	}
	if response.Run.RunID != taskNewSecondRunID || response.Run.Status != "contract_draft" || response.Run.Task != "integrate clarify" {
		t.Fatalf("embedded run mismatch: %#v", response.Run)
	}
	loop := response.ClarifyLoop
	if loop.Schema != clarifyLoopSummarySchema || loop.RunID != taskNewSecondRunID || loop.TerminalReason != "converged" || !loop.Converged {
		t.Fatalf("embedded loop summary mismatch: %#v", loop)
	}
	if len(loop.Rounds) != 1 || loop.Rounds[0].AutoResolved != 1 {
		t.Fatalf("embedded loop rounds mismatch: %#v", loop.Rounds)
	}
}

// TestTaskNewClarifyLoopFailureLeavesRunCreated breaks the clarifier (unknown
// helper mode, exit 2): the loop fails after the run was created, the error
// names the run and the re-run command, and the run stays created, current, and
// usable.
func TestTaskNewClarifyLoopFailureLeavesRunCreated(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	app = configureClarifyLoopHelpers(t, app, paths)
	setClarifyLoopHelperEnv(t, filepath.Join(stateDir, "sequence"), "broken")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"task", "new", "integrate clarify", "--clarify", "--reviewer", clarifyLoopClarifierName, "--yes"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("task new --clarify with broken clarifier exited %d, want 1, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{
		"run " + taskNewSecondRunID + " was created, but its clarify loop failed",
		"pactum clarify loop " + taskNewSecondRunID + " --yes",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("loop failure stderr missing %q:\n%s", want, stderr.String())
		}
	}
	// The created-run section was already printed, so the operator sees the run
	// id even though the command failed.
	if !strings.Contains(stdout.String(), "id: "+taskNewSecondRunID) {
		t.Fatalf("loop failure stdout missing created-run section:\n%s", stdout.String())
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, taskNewSecondRunID))
	state := readRunState(t, runPaths.RunJSON)
	if state.RunID != taskNewSecondRunID || state.Task != "integrate clarify" {
		t.Fatalf("run state not intact after loop failure: %#v", state)
	}
	if current, _ := readCurrentRun(paths); current != taskNewSecondRunID {
		t.Fatalf("current run = %q, want %q (failed loop must not orphan the run)", current, taskNewSecondRunID)
	}
	// The loop finalized its own summary with the error terminal, so a re-run
	// starts from a consistent record.
	summary := readClarifyLoopSummary(t, runPaths.ClarifyLoopSummaryJSON)
	if summary.TerminalReason != "error" {
		t.Fatalf("loop summary terminal = %q, want error: %#v", summary.TerminalReason, summary)
	}
}

// TestTaskNewWithoutClarifyOutputUnchanged pins the full task new output without
// --clarify byte-for-byte, so the --clarify integration can never drift the
// default path.
func TestTaskNewWithoutClarifyOutputUnchanged(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"task", "new", "another task"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("task new exited %d, stderr: %s", code, stderr.String())
	}
	want := "Run created\n" +
		"\n" +
		"Run:\n" +
		"  id: " + taskNewSecondRunID + "\n" +
		"  status: contract_draft\n" +
		"  task: another task\n" +
		"  current: yes\n" +
		"\n" +
		"Next:\n" +
		"  Review the contract draft, then: pactum contract approve\n"
	if got := stdout.String(); got != want {
		t.Fatalf("task new output drifted:\ngot:\n%s\nwant:\n%s", got, want)
	}
	assertNoFile(t, contractRunPaths(filepath.Join(paths.RunsDir, taskNewSecondRunID)).ClarifyLoopSummaryJSON)
}
