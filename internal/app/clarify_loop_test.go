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
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

const clarifyLoopClarifierName = "loop-clarifier"

func TestClarifyLoopRequiresYes(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperClarifiers(app, "helper", "helper")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "loop", runID, "--reviewer", "helper"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("clarify loop without --yes exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "clarify loop requires --yes") {
		t.Fatalf("clarify loop confirmation stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.ClarifyLoopSummaryJSON)
	assertNoFile(t, runPaths.ClarifierPromptMD)
}

// TestClarifyLoopConvergesOnHighConfidenceAnswers drives the loop with the
// standard clarifier helper, which emits one blocking high-confidence question
// (auto-resolvable) and one non-blocking medium question (stays open). Round 1
// auto-resolves the blocking question, leaving no open blocking questions, so
// the loop converges in a single round.
func TestClarifyLoopConvergesOnHighConfidenceAnswers(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperClarifiers(app, "helper", "helper")

	t.Setenv("PACTUM_CLARIFIER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "loop", runID, "--reviewer", "helper", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "Clarify loop finished") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}

	var summary clarifyLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.Schema != clarifyLoopSummarySchema || summary.RunID != runID || summary.TerminalReason != "converged" || !summary.Converged {
		t.Fatalf("unexpected loop summary: %#v", summary)
	}
	// No --max-rounds flag: the cap comes from the clarify.max_rounds default.
	if summary.MaxRounds != 3 {
		t.Fatalf("max rounds = %d, want clarify.max_rounds default 3", summary.MaxRounds)
	}
	if summary.Clarifier != "helper" {
		t.Fatalf("summary should record the resolved clarifier: %#v", summary)
	}
	if summary.RunStatus != "contract_draft" {
		t.Fatalf("run status = %q, want contract_draft (no open blocking questions)", summary.RunStatus)
	}
	if summary.ApprovalReset {
		t.Fatalf("non-approved run should not report approval_reset: %#v", summary)
	}
	if len(summary.Rounds) != 1 {
		t.Fatalf("rounds = %d, want 1: %#v", len(summary.Rounds), summary.Rounds)
	}
	round := summary.Rounds[0]
	if round.Round != 1 || round.ClarifierAttemptID != "clarifier_attempt_001" || round.QuestionsCreated != 2 || round.AutoResolved != 1 || round.OpenBlockingAfter != 0 {
		t.Fatalf("round summary mismatch: %#v", round)
	}
	coverage := coverageByKind(summary.Coverage, "scope")
	if coverage == nil || coverage.Total != 1 || coverage.Answered != 1 || coverage.Open != 0 {
		t.Fatalf("scope coverage should record the auto-resolved question: %#v", coverage)
	}

	// The auto-resolved answer carries the auto sources and the clarifier's own
	// recommendation; the medium-confidence question stays open for the human.
	questions, err := readClarificationQuestions(runPaths.QuestionsJSONL)
	assertNoError(t, err)
	if len(questions) != 2 {
		t.Fatalf("questions = %d, want 2: %#v", len(questions), questions)
	}
	answers, err := readClarificationAnswers(runPaths.AnswersJSONL)
	assertNoError(t, err)
	if len(answers) != 1 {
		t.Fatalf("answers = %d, want 1 (only the high-confidence question): %#v", len(answers), answers)
	}
	if answers[0].ID != "a_001" || answers[0].QuestionID != "q_001" || answers[0].Source != "auto_recommended" {
		t.Fatalf("unexpected auto-resolved answer: %#v", answers[0])
	}
	if answers[0].Answer != questions[0].RecommendedAnswer {
		t.Fatalf("auto-resolved answer should be the recommended answer: %q != %q", answers[0].Answer, questions[0].RecommendedAnswer)
	}
	decisions, err := readClarificationDecisions(runPaths.DecisionsJSONL)
	assertNoError(t, err)
	if len(decisions) != 1 || decisions[0].ID != "d_001" || decisions[0].QuestionID != "q_001" || decisions[0].Source != "clarify_loop_auto" {
		t.Fatalf("unexpected auto-resolve decision: %#v", decisions)
	}
	status, err := buildClarificationStatus(runPaths, readRunState(t, runPaths.RunJSON))
	assertNoError(t, err)
	if got := clarifyQuestionStatusByID(t, status, "q_002"); got.Status != "open" {
		t.Fatalf("medium-confidence q_002 must stay open for the human: %#v", got)
	}

	artifact := readClarifyLoopSummary(t, runPaths.ClarifyLoopSummaryJSON)
	if artifact.TerminalReason != summary.TerminalReason || len(artifact.Rounds) != len(summary.Rounds) || artifact.Converged != summary.Converged {
		t.Fatalf("summary artifact mismatch:\nstdout=%#v\nartifact=%#v", summary, artifact)
	}

	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	for _, want := range []string{"clarification_loop_started", "clarifier_attempt_started", "clarifier_attempt_finished", "clarification_questions_suggested", "clarification_answer_recorded", "clarification_decision_recorded", "clarification_loop_finished"} {
		if indexOfEvent(eventTypes, want) == -1 {
			t.Fatalf("events missing %s:\n%v", want, eventTypes)
		}
	}
	if indexOfEvent(eventTypes, "clarification_loop_started") > indexOfEvent(eventTypes, "clarification_loop_finished") {
		t.Fatalf("loop events out of order:\n%v", eventTypes)
	}
}

// TestClarifyLoopStopsNeedsHuman drives the loop with a clarifier that emits a
// single blocking medium-confidence question in round 1 and nothing afterwards:
// round 2 creates no questions and auto-resolves nothing, so automation is out
// of moves and the loop terminates needs_human with the question still open.
func TestClarifyLoopStopsNeedsHuman(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureClarifyLoopHelpers(app)
	setClarifyLoopHelperEnv(t, filepath.Join(stateDir, "sequence"), "medium_then_empty")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "loop", runID, "--reviewer", clarifyLoopClarifierName, "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Clarify loop finished",
		"terminal reason: needs_human",
		"rounds: 2/3",
		"converged: no",
		"- round 1: questions created 1, auto-resolved 0, open blocking 1, clarifier clarifier_attempt_001",
		"- round 2: questions created 0, auto-resolved 0, open blocking 1, clarifier clarifier_attempt_002",
		"Coverage by dimension:",
		"summary: " + runArtifactRepoRel(runID, clarifyLoopSummaryArtifact),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify loop output missing %q:\n%s", want, got)
		}
	}

	// Medium confidence is never auto-resolved: no answers were recorded and the
	// blocking question awaits the human.
	if answers := readLines(t, runPaths.AnswersJSONL); len(answers) != 0 {
		t.Fatalf("needs_human loop must not auto-resolve medium confidence: %v", answers)
	}
	summary := readClarifyLoopSummary(t, runPaths.ClarifyLoopSummaryJSON)
	if summary.TerminalReason != "needs_human" || summary.Converged || len(summary.Rounds) != 2 {
		t.Fatalf("unexpected loop summary artifact: %#v", summary)
	}
	state := readRunState(t, runPaths.RunJSON)
	if state.Status != "clarifying" {
		t.Fatalf("run status = %q, want clarifying (open blocking question remains)", state.Status)
	}
}

func TestClarifyLoopStopsAtMaxRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureClarifyLoopHelpers(app)
	setClarifyLoopHelperEnv(t, filepath.Join(stateDir, "sequence"), "always_medium")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "loop", runID, "--reviewer", clarifyLoopClarifierName, "--max-rounds", "2", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	var summary clarifyLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.TerminalReason != "max_rounds" || summary.Converged || summary.MaxRounds != 2 {
		t.Fatalf("unexpected loop summary: %#v", summary)
	}
	if len(summary.Rounds) != 2 {
		t.Fatalf("rounds = %d, want 2: %#v", len(summary.Rounds), summary.Rounds)
	}
	// Every round created a new blocking medium question and resolved nothing.
	for index, round := range summary.Rounds {
		if round.QuestionsCreated != 1 || round.AutoResolved != 0 {
			t.Fatalf("round %d mismatch: %#v", index+1, round)
		}
	}
	if summary.Rounds[1].OpenBlockingAfter != 2 {
		t.Fatalf("round 2 open blocking = %d, want 2", summary.Rounds[1].OpenBlockingAfter)
	}
	assertFile(t, runPaths.ClarifyLoopSummaryJSON)
}

func TestClarifyLoopUsesConfigMaxRounds(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureClarifyLoopHelpers(app)
	setClarifyLoopMaxRoundsConfig(t, paths, 1)
	setClarifyLoopHelperEnv(t, filepath.Join(stateDir, "sequence"), "always_medium")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "loop", runID, "--reviewer", clarifyLoopClarifierName, "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify loop exited %d, stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	var summary clarifyLoopSummaryDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.MaxRounds != 1 || summary.TerminalReason != "max_rounds" || len(summary.Rounds) != 1 {
		t.Fatalf("loop should honor clarify.max_rounds=1: %#v", summary)
	}
}

func TestClarifyLoopRejectsNegativeMaxRounds(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	app = configureHelperClarifiers(app, "helper", "helper")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "loop", runID, "--reviewer", "helper", "--max-rounds=-1", "--yes"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("clarify loop with negative max rounds exited %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, "max rounds must be positive") {
		t.Fatalf("negative max rounds stderr mismatch:\n%s", got)
	}
}

func TestClarifyLoopSurfacesApprovalResetOnApprovedRun(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperClarifiers(app, "helper", "helper")
	approveRunForTest(t, app, runID)

	t.Setenv("PACTUM_CLARIFIER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "loop", runID, "--reviewer", "helper", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify loop exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"this run was approved",
		"reset approval to pending",
		"pactum contract approve " + runID,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify loop output missing approval-reset warning %q:\n%s", want, got)
		}
	}
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" {
		t.Fatalf("approval not reset to pending: %#v", approval)
	}
	summary := readClarifyLoopSummary(t, runPaths.ClarifyLoopSummaryJSON)
	if !summary.ApprovalReset {
		t.Fatalf("loop summary should surface approval_reset: %#v", summary)
	}
}

// TestClarifyAnswerManualRecordsStayByteIdentical pins the exact JSONL bytes the
// manual ClarifyAnswer path writes, so extracting the shared
// recordClarificationAnswer helper (used by the clarify loop with different
// sources) can never drift the manual records.
func TestClarifyAnswerManualRecordsStayByteIdentical(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var setup bytes.Buffer
	assertNoError(t, app.ClarifyAsk(&setup, runID, "Which backend?", true, false))
	var stdout bytes.Buffer
	assertNoError(t, app.ClarifyAnswer(&stdout, runID, "q_001", "Use SQLite.", false))

	wantAnswer := `{"schema":"pactum.clarification_answer.v1","id":"a_001","run_id":"` + runID + `","question_id":"q_001","answer":"Use SQLite.","created_at":"2026-05-31T18:40:12Z","source":"manual"}`
	wantDecision := `{"schema":"pactum.clarification_decision.v1","id":"d_001","run_id":"` + runID + `","question_id":"q_001","decision":"Use SQLite.","created_at":"2026-05-31T18:40:12Z","source":"manual_answer"}`
	if answers := readLines(t, runPaths.AnswersJSONL); len(answers) != 1 || answers[0] != wantAnswer {
		t.Fatalf("manual answer record drifted:\ngot  %v\nwant %s", answers, wantAnswer)
	}
	if decisions := readLines(t, runPaths.DecisionsJSONL); len(decisions) != 1 || decisions[0] != wantDecision {
		t.Fatalf("manual decision record drifted:\ngot  %v\nwant %s", decisions, wantDecision)
	}
}

func readClarifyLoopSummary(t *testing.T, path string) clarifyLoopSummaryDocument {
	t.Helper()
	var summary clarifyLoopSummaryDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &summary))
	return summary
}

func setClarifyLoopMaxRoundsConfig(t *testing.T, paths artifacts.Paths, maxRounds int) {
	t.Helper()
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	config.Clarify.MaxRounds = maxRounds
	assertNoError(t, writeYAML(paths.Config, config))
}

func configureClarifyLoopHelpers(app App) App {
	app.AgentRegistry = testAgentRegistry(agents.AgentDescriptor{
		Name:    clarifyLoopClarifierName,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestClarifyLoopHelperProcess"},
		Input:   agents.InputPromptFile,
	})
	return app
}

func setClarifyLoopHelperEnv(t *testing.T, sequenceFile string, mode string) {
	t.Helper()
	t.Setenv("PACTUM_CLARIFY_LOOP_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFY_LOOP_SEQUENCE_FILE", sequenceFile)
	t.Setenv("PACTUM_CLARIFY_LOOP_HELPER_MODE", mode)
}

// TestClarifyLoopHelperProcess is the sequence-aware clarifier helper for loop
// tests: it counts its invocations through the sequence file and emits
// questions according to the selected mode.
func TestClarifyLoopHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_CLARIFY_LOOP_HELPER_PROCESS") != "1" {
		return
	}
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(2)
	}
	sequenceFile := os.Getenv("PACTUM_CLARIFY_LOOP_SEQUENCE_FILE")
	call := 1
	if data, err := os.ReadFile(sequenceFile); err == nil {
		if parsed, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			call = parsed + 1
		}
	}
	if err := os.WriteFile(sequenceFile, []byte(strconv.Itoa(call)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sequence error: %v\n", err)
		os.Exit(2)
	}

	switch os.Getenv("PACTUM_CLARIFY_LOOP_HELPER_MODE") {
	case "medium_then_empty":
		if call == 1 {
			fmt.Print(clarifierStructuredOutput([]map[string]any{clarifyLoopMediumQuestion(call)}))
		} else {
			fmt.Print(clarifierStructuredOutput([]map[string]any{}))
		}
	case "always_medium":
		fmt.Print(clarifierStructuredOutput([]map[string]any{clarifyLoopMediumQuestion(call)}))
	default:
		fmt.Fprintln(os.Stderr, "unknown clarify loop helper mode")
		os.Exit(2)
	}
	os.Exit(0)
}

// clarifyLoopMediumQuestion is a blocking medium-confidence question: blocking
// keeps the loop unconverged, and medium confidence keeps it out of reach of
// auto-resolve, so only a human answer could make progress.
func clarifyLoopMediumQuestion(call int) map[string]any {
	return map[string]any{
		"text":               fmt.Sprintf("What retention policy should round %d assume?", call),
		"blocking":           true,
		"kind":               "scope",
		"rationale":          "Only the human knows the retention policy.",
		"recommended_answer": "Keep 30 days.",
		"confidence":         "medium",
	}
}

// TestAutoResolveClarificationsHonorsDependsOn pins the dependency rule: a
// high-confidence question blocked on an unanswered prerequisite is NOT
// auto-resolved, while a prerequisite auto-resolved earlier in the same round
// unblocks its dependents (questions resolve foundational-first).
func TestAutoResolveClarificationsHonorsDependsOn(t *testing.T) {
	dir := t.TempDir()
	runPaths := contractRunPaths(dir)
	context := clarifyContext{
		Root:     dir,
		Paths:    artifacts.Paths{EventsJSONL: filepath.Join(dir, "events.jsonl")},
		RunPaths: runPaths,
		State:    contractRunState{RunID: "run_dep"},
	}
	now := time.Unix(0, 0).UTC()
	write := func(id string, confidence string, dependsOn []string) {
		assertNoError(t, appendJSONLine(runPaths.QuestionsJSONL, clarificationQuestionRecord{
			Schema:            clarificationQuestionSchema,
			ID:                id,
			RunID:             "run_dep",
			Question:          "q?",
			Blocking:          true,
			Kind:              "scope",
			Rationale:         "r",
			RecommendedAnswer: "answer for " + id,
			Confidence:        confidence,
			DependsOn:         dependsOn,
			Status:            "open",
			CreatedAt:         now,
			Source:            "clarifier_attempt",
		}))
	}
	write("q_001", "medium", nil)             // stays open: not high
	write("q_002", "high", []string{"q_001"}) // blocked: q_001 unanswered
	write("q_003", "high", nil)               // resolves
	write("q_004", "high", []string{"q_003"}) // unblocked same round by q_003

	resolved, err := App{}.autoResolveClarifications(context, now)
	assertNoError(t, err)
	if len(resolved) != 2 || resolved[0].QuestionID != "q_003" || resolved[1].QuestionID != "q_004" {
		t.Fatalf("auto-resolved = %#v, want q_003 then q_004", resolved)
	}
	answers, err := readClarificationAnswers(runPaths.AnswersJSONL)
	assertNoError(t, err)
	for _, answer := range answers {
		if answer.QuestionID == "q_002" {
			t.Fatalf("q_002 is blocked on unanswered q_001 and must not be auto-resolved: %#v", answers)
		}
	}
}
