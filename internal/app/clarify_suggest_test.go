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
	"time"

	"github.com/heurema/pactum/internal/agents"
)

func TestClarifySuggestRequiresYesNonInteractive(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperClarifiers(app, "helper", "helper")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "suggest", runID, "--reviewer", "helper"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("clarify suggest should fail without --yes in non-interactive use")
	}
	if got := stderr.String(); !strings.Contains(got, "refusing to run agent non-interactively without --yes") {
		t.Fatalf("clarify suggest stderr mismatch:\n%s", got)
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertNoFile(t, runPaths.ClarifierPromptMD)
	assertNoFile(t, runPaths.ClarifierContextMD)
}

func TestClarifySuggestRunsClarifierAndRecordsOpenQuestions(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	readmeBefore := mustReadFile(t, filepath.Join(root, "README.md"))

	setCrossModelReviewConfig(t, paths, true)
	writeExecutionAttemptForTest(t, runPaths, runID, "attempt_001", mustResolveExecutorForTest(t, agents.BuiltinCodex))
	app = configureHelperClarifiers(app, agents.BuiltinCodex, agents.BuiltinClaude)

	t.Setenv("PACTUM_CLARIFIER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "suggest", runID, "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify suggest exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Clarification suggestions recorded",
		"agent: claude",
		"created: 2",
		"q_001 [blocking] Should generated clarification questions reset contract approval?",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify suggest output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "stdin_has_clarifier_prompt=") || strings.Contains(got, "clarifier-stderr-line") {
		t.Fatalf("agent output leaked into stdout:\n%s", got)
	}
	if got := stderr.String(); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stdin_has_clarifier_prompt=true") || !strings.Contains(got, "clarifier-stderr-line") {
		t.Fatalf("live clarifier output missing from stderr:\n%s", got)
	}

	questions, err := readClarificationQuestions(runPaths.QuestionsJSONL)
	assertNoError(t, err)
	if len(questions) != 2 {
		t.Fatalf("questions count = %d, want 2: %#v", len(questions), questions)
	}
	if questions[0].ID != "q_001" || !questions[0].Blocking || questions[0].Status != "open" || questions[0].Source != "clarifier_attempt" || questions[0].ClarifierAttemptID != "clarifier_attempt_001" {
		t.Fatalf("unexpected first question: %#v", questions[0])
	}
	if questions[0].Rationale != "Approval should be reset if new human input can change scope." {
		t.Fatalf("first question rationale = %q", questions[0].Rationale)
	}
	if questions[0].RecommendedAnswer != "Yes — reset approval to clarifying so the human re-approves after answering." || questions[0].Confidence != "high" {
		t.Fatalf("first question missing recommended answer/confidence: %#v", questions[0])
	}
	if questions[1].ID != "q_002" || questions[1].Blocking || questions[1].Status != "open" {
		t.Fatalf("unexpected second question: %#v", questions[1])
	}
	if questions[1].RecommendedAnswer != "Yes — keep them open and non-blocking so progress is not gated." || questions[1].Confidence != "medium" {
		t.Fatalf("second question missing recommended answer/confidence: %#v", questions[1])
	}
	if answers := readLines(t, runPaths.AnswersJSONL); len(answers) != 0 {
		t.Fatalf("clarify suggest should not answer questions: %v", answers)
	}
	if decisions := readLines(t, runPaths.DecisionsJSONL); len(decisions) != 0 {
		t.Fatalf("clarify suggest should not write decisions: %v", decisions)
	}

	state := readRunState(t, runPaths.RunJSON)
	if state.Status != "clarifying" {
		t.Fatalf("run status = %q, want clarifying", state.Status)
	}
	contract := readContractDraft(t, runPaths.ContractJSON)
	if len(contract.OpenQuestions) != 2 || len(contract.Clarifications.Questions) != 2 {
		t.Fatalf("contract missing suggested questions: %#v", contract.Clarifications)
	}
	if contract.Clarifications.Questions[0].Rationale != questions[0].Rationale {
		t.Fatalf("contract did not preserve rationale: %#v", contract.Clarifications.Questions[0])
	}
	if got := mustReadFile(t, runPaths.ContractMD); !strings.Contains(got, "Rationale: Approval should be reset") {
		t.Fatalf("contract.md missing rationale:\n%s", got)
	}

	attemptPaths := clarifierAttemptPaths(runPaths, "clarifier_attempt_001")
	assertFile(t, attemptPaths.RequestJSON)
	assertFile(t, attemptPaths.StdoutLog)
	assertFile(t, attemptPaths.StderrLog)
	assertFile(t, attemptPaths.ResultJSON)
	assertFile(t, runPaths.ClarifierLastResultJSON)

	var request clarifierRequestDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.RequestJSON)), &request))
	if request.Schema != clarifierRequestSchema || request.Clarifier.Name != agents.BuiltinClaude || request.WouldRun.Stdin != clarifierPromptRepoPath(runID) {
		t.Fatalf("unexpected clarifier request: %#v", request)
	}
	var result clarifierResultDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.ResultJSON)), &result))
	if result.Schema != clarifierResultSchema || result.Clarifier != agents.BuiltinClaude || result.ExitCode != 0 || result.TimedOut {
		t.Fatalf("unexpected clarifier result: %#v", result)
	}
	if result.Stdout != "clarify/clarifier-attempts/clarifier_attempt_001/stdout.log" || result.Stderr != "clarify/clarifier-attempts/clarifier_attempt_001/stderr.log" {
		t.Fatalf("unexpected result log paths: %#v", result)
	}
	if got := mustReadFile(t, runPaths.ClarifierLastResultJSON); got != mustReadFile(t, attemptPaths.ResultJSON) {
		t.Fatalf("clarifier-last-result.json should copy result.json")
	}
	if got := mustReadFile(t, attemptPaths.StdoutLog); !strings.Contains(got, clarificationSuggestionsSchema) {
		t.Fatalf("stdout log missing structured suggestions:\n%s", got)
	}
	if got := mustReadFile(t, filepath.Join(root, "README.md")); got != readmeBefore {
		t.Fatalf("clarify suggest should not edit repository files")
	}

	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	for _, want := range []string{"clarifier_attempt_started", "clarifier_attempt_finished", "clarification_questions_suggested"} {
		if indexOfEvent(eventTypes, want) == -1 {
			t.Fatalf("events missing %s:\n%v", want, eventTypes)
		}
	}
}

func TestClarifySuggestJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	app = configureHelperClarifiers(app, "helper", "helper")

	t.Setenv("PACTUM_CLARIFIER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "suggest", runID, "--reviewer", "helper", "--yes", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify suggest --json exited %d, stderr: %s", code, stderr.String())
	}
	var response clarifySuggestResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.AttemptID != "clarifier_attempt_001" || response.Clarifier != "helper" || len(response.Created) != 2 {
		t.Fatalf("unexpected clarify suggest json: %#v", response)
	}
	if strings.Contains(stdout.String(), "Clarification suggestions recorded") || strings.Contains(stdout.String(), "Resolved:") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestClarificationQuestionFromSuggestionRequiresRecommendedAnswerAndConfidence(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	blocking := true
	base := func() clarifierSuggestionInput {
		return clarifierSuggestionInput{
			Text:              "Should the run reset approval?",
			Blocking:          &blocking,
			Rationale:         "Scope may change.",
			RecommendedAnswer: "Yes — reset to clarifying.",
			Confidence:        "high",
		}
	}

	t.Run("valid suggestion populates new fields", func(t *testing.T) {
		record, warning := clarificationQuestionFromSuggestion("/repo", "run", "attempt_001", 1, base(), now)
		if warning != "" {
			t.Fatalf("unexpected warning: %q", warning)
		}
		if record.RecommendedAnswer != "Yes — reset to clarifying." || record.Confidence != "high" {
			t.Fatalf("record missing recommended answer/confidence: %#v", record)
		}
	})

	t.Run("missing recommended answer is rejected", func(t *testing.T) {
		input := base()
		input.RecommendedAnswer = "   "
		_, warning := clarificationQuestionFromSuggestion("/repo", "run", "attempt_001", 1, input, now)
		if warning != "question skipped: recommended answer is required" {
			t.Fatalf("warning = %q", warning)
		}
	})

	t.Run("invalid confidence is rejected", func(t *testing.T) {
		for _, confidence := range []string{"", "certain", "HIGH", "maybe"} {
			input := base()
			input.Confidence = confidence
			_, warning := clarificationQuestionFromSuggestion("/repo", "run", "attempt_001", 1, input, now)
			if warning != "question skipped: confidence must be one of high, medium, low" {
				t.Fatalf("confidence %q warning = %q", confidence, warning)
			}
		}
	})
}

func configureHelperClarifiers(app App, defaultReviewer string, names ...string) App {
	descriptors := make([]agents.AgentDescriptor, 0, len(names))
	for _, name := range names {
		descriptors = append(descriptors, agents.AgentDescriptor{
			Name:    name,
			Command: os.Args[0],
			Args:    []string{"-test.run=TestClarifierHelperProcess"},
			Input:   agents.InputPromptFile,
		})
	}
	registry := testAgentRegistry(descriptors...)
	if fixed, ok := registry.(fixedAgentRegistry); ok {
		fixed.defaultReviewer = defaultReviewer
		registry = fixed
	}
	app.AgentRegistry = registry
	return app
}

func TestClarifierHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_CLARIFIER_HELPER_PROCESS") != "1" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cwd error: %v\n", err)
		os.Exit(2)
	}
	expectedCWD := os.Getenv("PACTUM_CLARIFIER_EXPECTED_CWD")
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
	fmt.Printf("stdin_has_clarifier_prompt=%t\n", strings.Contains(string(stdin), "# Clarifier Prompt"))
	fmt.Print(clarifierStructuredOutput([]map[string]any{
		{
			"text":               "Should generated clarification questions reset contract approval?",
			"blocking":           true,
			"rationale":          "Approval should be reset if new human input can change scope.",
			"recommended_answer": "Yes — reset approval to clarifying so the human re-approves after answering.",
			"confidence":         "high",
		},
		{
			"text":               "Should non-blocking suggestions remain open for optional human answers?",
			"blocking":           false,
			"rationale":          "Optional questions should not block progress but should remain answerable.",
			"recommended_answer": "Yes — keep them open and non-blocking so progress is not gated.",
			"confidence":         "medium",
		},
	}))
	fmt.Fprintln(os.Stderr, "clarifier-stderr-line")
	os.Exit(0)
}

func clarifierStructuredOutput(questions []map[string]any) string {
	block := map[string]any{
		"schema":    clarificationSuggestionsSchema,
		"questions": questions,
	}
	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		panic(err)
	}
	return "clarifier notes\n```json\n" + string(data) + "\n```\n"
}
