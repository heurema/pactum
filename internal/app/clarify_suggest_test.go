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
	"github.com/heurema/pactum/internal/artifacts"
)

func TestClarifySuggestRunsClarifierAndRecordsOpenQuestions(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	readmeBefore := mustReadFile(t, filepath.Join(root, "README.md"))

	writeExecutionAttemptForTest(t, runPaths, runID, "attempt_001", mustResolveExecutorForTest(t, agents.BuiltinCodex))
	app = configureHelperClarifiers(t, app, paths, agents.BuiltinClaude)

	t.Setenv("PACTUM_CLARIFIER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "suggest", runID}, &stdout, &stderr)
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
	// The run was never approved, so recording suggestions must not warn about an
	// approval reset.
	if strings.Contains(got, "this run was approved") {
		t.Fatalf("non-approved run should not warn about approval reset:\n%s", got)
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
	if questions[0].Kind != "scope" || questions[1].Kind != "acceptance" {
		t.Fatalf("questions did not persist kind: %q, %q", questions[0].Kind, questions[1].Kind)
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
	app, paths, runID := setupContractRun(t, root)
	app = configureHelperClarifiers(t, app, paths, "helper")

	t.Setenv("PACTUM_CLARIFIER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "suggest", runID, "--reviewer", "helper", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify suggest --json exited %d, stderr: %s", code, stderr.String())
	}
	var response clarifySuggestResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	// The response records the engine inferred from the helper entry's model.
	if response.AttemptID != "clarifier_attempt_001" || response.Clarifier != "claude" || len(response.Created) != 2 {
		t.Fatalf("unexpected clarify suggest json: %#v", response)
	}
	// The run was never approved, so approval_reset stays false and is omitted.
	if response.ApprovalReset {
		t.Fatalf("non-approved run should report approval_reset=false: %#v", response)
	}
	if strings.Contains(stdout.String(), "approval_reset") {
		t.Fatalf("non-approved run should omit approval_reset from JSON:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Clarification suggestions recorded") || strings.Contains(stdout.String(), "Resolved:") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestClarifySuggestSurfacesApprovalResetOnApprovedRun(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperClarifiers(t, app, paths, "helper")
	approveRunForTest(t, app, runID)

	t.Setenv("PACTUM_CLARIFIER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "suggest", runID, "--reviewer", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify suggest exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"this run was approved",
		"reset approval to pending",
		// The helper emits a blocking question, so the run lands in clarifying.
		`status now "clarifying"`,
		"pactum contract approve " + runID,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify suggest output missing approval-reset warning %q:\n%s", want, got)
		}
	}
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil {
		t.Fatalf("approval not reset to pending: %#v", approval)
	}
	if eventTypes := ledgerEventTypes(t, paths.EventsJSONL); indexOfEvent(eventTypes, "contract_approval_reset") == -1 {
		t.Fatalf("events missing contract_approval_reset:\n%v", eventTypes)
	}

	// JSON output reports approval_reset=true (a fresh approved run, since the run
	// above is no longer approved after the reset).
	jsonRoot := t.TempDir()
	jsonApp, jsonPaths, jsonRunID := setupContractRun(t, jsonRoot)
	jsonApp = configureHelperClarifiers(t, jsonApp, jsonPaths, "helper")
	approveRunForTest(t, jsonApp, jsonRunID)
	t.Setenv("PACTUM_CLARIFIER_EXPECTED_CWD", jsonRoot)

	var jsonOut, jsonErr bytes.Buffer
	if code := jsonApp.Run([]string{"clarify", "suggest", jsonRunID, "--reviewer", "helper", "--json"}, &jsonOut, &jsonErr); code != 0 {
		t.Fatalf("clarify suggest --json exited %d, stderr: %s", code, jsonErr.String())
	}
	var response clarifySuggestResponse
	assertNoError(t, json.Unmarshal(jsonOut.Bytes(), &response))
	if !response.ApprovalReset {
		t.Fatalf("approved run should report approval_reset=true: %#v", response)
	}
}

func TestClarificationQuestionFromSuggestionRequiresRecommendedAnswerAndConfidence(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	blocking := true
	base := func() clarifierSuggestionInput {
		return clarifierSuggestionInput{
			Text:              "Should the run reset approval?",
			Blocking:          &blocking,
			Kind:              "scope",
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
		if record.Kind != "scope" {
			t.Fatalf("record missing kind: %#v", record)
		}
	})

	t.Run("missing or whitespace kind defaults to other", func(t *testing.T) {
		for _, kind := range []string{"", "  ", "\t"} {
			input := base()
			input.Kind = kind
			record, warning := clarificationQuestionFromSuggestion("/repo", "run", "attempt_001", 1, input, now)
			if warning != "" {
				t.Fatalf("kind %q should default to other, got warning %q", kind, warning)
			}
			if record.Kind != "other" {
				t.Fatalf("kind %q should record kind=other, got %q", kind, record.Kind)
			}
		}
	})

	t.Run("non-empty invalid kind is rejected", func(t *testing.T) {
		for _, kind := range []string{"Terminology", "domain", "unknown"} {
			input := base()
			input.Kind = kind
			_, warning := clarificationQuestionFromSuggestion("/repo", "run", "attempt_001", 1, input, now)
			if warning != "question skipped: kind must be one of terminology, scope, acceptance, edge_case, assumption, other" {
				t.Fatalf("kind %q warning = %q", kind, warning)
			}
		}
	})

	t.Run("every allowed kind is accepted", func(t *testing.T) {
		for _, kind := range []string{"terminology", "scope", "acceptance", "edge_case", "assumption", "other"} {
			input := base()
			input.Kind = kind
			record, warning := clarificationQuestionFromSuggestion("/repo", "run", "attempt_001", 1, input, now)
			if warning != "" || record.Kind != kind {
				t.Fatalf("kind %q should be accepted: warning=%q record.Kind=%q", kind, warning, record.Kind)
			}
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

func TestRenderClarifierPromptProbesEdgeCases(t *testing.T) {
	prompt := renderClarifierPrompt("run_20260101_000000")
	// The dedicated edge-case probing section and its kind=edge_case tagging
	// instruction must survive, so the prompt keeps inventing concrete boundary
	// and failure scenarios rather than abstractly "considering edge cases".
	for _, want := range []string{
		"## Probe edge cases",
		"kind=edge_case",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("clarifier prompt missing edge-case probing guidance %q:\n%s", want, prompt)
		}
	}
}

func TestRenderClarifierContextSurfacesCoverage(t *testing.T) {
	prep := clarifierPreparation{
		Context:  clarifyContext{State: contractRunState{RunID: "run_20260101_000000", Status: "clarifying"}},
		Contract: draftContract{Goal: "add a coverage signal"},
		Status: clarifyStatusResponse{
			Total:        1,
			Open:         1,
			BlockingOpen: 1,
			Converged:    false,
			Coverage: []clarifyKindCoverage{
				{Kind: "terminology", Total: 1, Open: 1, Answered: 0, BlockingOpen: 1},
				{Kind: "scope"},
				{Kind: "acceptance"},
				{Kind: "edge_case"},
				{Kind: "assumption"},
			},
			Questions: []clarifyQuestionStatus{},
		},
	}
	got := renderClarifierContext(prep)
	for _, want := range []string{
		"- Converged: false",
		"- Coverage by dimension",
		"- terminology: total 1, answered 0, open 1, blocking open 1",
		"- acceptance: total 0, answered 0, open 0, blocking open 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarifier context missing coverage detail %q:\n%s", want, got)
		}
	}
}

func TestRenderClarifierPromptSeeksDimensionCoverage(t *testing.T) {
	prompt := renderClarifierPrompt("run_20260101_000000")
	// The coverage guidance must instruct covering the material dimensions
	// before concluding while keeping explore-first discipline (no manufactured
	// questions for dimensions the repo/contract already settles).
	for _, want := range []string{
		"## Cover the material dimensions",
		"scope, acceptance, terminology, edge_case, assumption",
		"Do NOT manufacture questions",
		"explore-first still applies",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("clarifier prompt missing coverage guidance %q:\n%s", want, prompt)
		}
	}
}

func TestResolveClarifierDependsOn(t *testing.T) {
	positionToID := map[int]string{
		1: "q_001",
		2: skippedClarifierPosition, // emitted at position 2 but dropped in validation
		3: "q_003",
	}

	t.Run("valid earlier references resolve to ids", func(t *testing.T) {
		resolved, warnings := resolveClarifierDependsOn(positionToID, 4, "q_004", []int{1, 3})
		if len(warnings) != 0 {
			t.Fatalf("unexpected warnings: %#v", warnings)
		}
		if len(resolved) != 2 || resolved[0] != "q_001" || resolved[1] != "q_003" {
			t.Fatalf("resolved = %#v, want [q_001 q_003]", resolved)
		}
	})

	t.Run("self forward and out-of-range references are dropped with warnings", func(t *testing.T) {
		resolved, warnings := resolveClarifierDependsOn(positionToID, 4, "q_004", []int{4, 5, 0})
		if len(resolved) != 0 {
			t.Fatalf("resolved = %#v, want none", resolved)
		}
		if len(warnings) != 3 {
			t.Fatalf("warnings = %#v, want 3 dropped references", warnings)
		}
	})

	t.Run("reference to a skipped position is dropped", func(t *testing.T) {
		resolved, warnings := resolveClarifierDependsOn(positionToID, 4, "q_004", []int{2})
		if len(resolved) != 0 || len(warnings) != 1 {
			t.Fatalf("skipped-position dependency should be dropped: resolved=%#v warnings=%#v", resolved, warnings)
		}
	})

	t.Run("no dependencies returns nil", func(t *testing.T) {
		resolved, warnings := resolveClarifierDependsOn(positionToID, 4, "q_004", nil)
		if resolved != nil || warnings != nil {
			t.Fatalf("expected nil, got resolved=%#v warnings=%#v", resolved, warnings)
		}
	})
}

func TestRecordClarifierSuggestionsResolvesDependsOnAndBlocks(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	state := readRunState(t, runPaths.RunJSON)
	context := clarifyContext{Root: root, Paths: paths, RunPaths: runPaths, State: state}

	now := time.Unix(0, 0).UTC()
	output := clarifierStructuredOutput([]map[string]any{
		{ // position 1 — foundational, no dependency
			"text":               "Which storage backend should the cache use?",
			"blocking":           true,
			"kind":               "scope",
			"rationale":          "The backend choice constrains the schema.",
			"recommended_answer": "SQLite.",
			"confidence":         "high",
		},
		{ // position 2 — valid dependency on the foundational question
			"text":               "What schema should the cache table use?",
			"blocking":           true,
			"kind":               "terminology",
			"rationale":          "The schema follows from the chosen backend.",
			"recommended_answer": "A single key/value table.",
			"confidence":         "medium",
			"depends_on":         []int{1},
		},
		{ // position 3 — self, forward, and out-of-range refs all dropped
			"text":               "What eviction policy is acceptable?",
			"blocking":           false,
			"kind":               "edge_case",
			"rationale":          "Eviction is independent of the earlier questions.",
			"recommended_answer": "LRU.",
			"confidence":         "low",
			"depends_on":         []int{3, 4, 0},
		},
	})
	stdoutPath := filepath.Join(root, "clarifier-stdout.log")
	mustWriteFile(t, stdoutPath, output)

	created, warnings, status, approvalReset, err := app.recordClarifierSuggestions(context, "clarifier_attempt_001", stdoutPath, now)
	assertNoError(t, err)
	if approvalReset {
		t.Fatalf("approvalReset should be false on a non-approved run")
	}
	if len(created) != 3 {
		t.Fatalf("created = %d, want 3: %#v", len(created), created)
	}
	if len(created[0].DependsOn) != 0 {
		t.Fatalf("foundational q_001 should carry no dependencies: %#v", created[0].DependsOn)
	}
	if len(created[1].DependsOn) != 1 || created[1].DependsOn[0] != "q_001" {
		t.Fatalf("q_002 depends_on = %#v, want [q_001]", created[1].DependsOn)
	}
	if len(created[2].DependsOn) != 0 {
		t.Fatalf("q_003 self/forward/out-of-range deps should all be dropped: %#v", created[2].DependsOn)
	}
	joinedWarnings := strings.Join(warnings, "\n")
	for _, want := range []string{
		"q_003 dependency dropped: depends_on position 3",
		"q_003 dependency dropped: depends_on position 4",
		"q_003 dependency dropped: depends_on position 0",
	} {
		if !strings.Contains(joinedWarnings, want) {
			t.Fatalf("warnings missing %q:\n%s", want, joinedWarnings)
		}
	}

	// The dropped-dependency question is still recorded and persisted.
	persisted, err := readClarificationQuestions(runPaths.QuestionsJSONL)
	assertNoError(t, err)
	if len(persisted) != 3 {
		t.Fatalf("persisted questions = %d, want 3", len(persisted))
	}
	if len(persisted[1].DependsOn) != 1 || persisted[1].DependsOn[0] != "q_001" {
		t.Fatalf("persisted q_002 depends_on = %#v, want [q_001]", persisted[1].DependsOn)
	}
	if persisted[0].Kind != "scope" || persisted[1].Kind != "terminology" || persisted[2].Kind != "edge_case" {
		t.Fatalf("persisted questions did not carry kind: %q, %q, %q", persisted[0].Kind, persisted[1].Kind, persisted[2].Kind)
	}

	// kind is surfaced in the status view for each question.
	if got := clarifyQuestionStatusByID(t, status, "q_002"); got.Kind != "terminology" {
		t.Fatalf("status q_002 kind = %q, want terminology", got.Kind)
	}

	// q_002 is open and its prerequisite q_001 is unanswered → Blocked.
	if got := clarifyQuestionStatusByID(t, status, "q_002"); !got.Blocked {
		t.Fatalf("q_002 should be Blocked while q_001 is unanswered: %#v", got)
	}
	if got := clarifyQuestionStatusByID(t, status, "q_001"); got.Blocked {
		t.Fatalf("foundational q_001 must not be Blocked: %#v", got)
	}
	if got := clarifyQuestionStatusByID(t, status, "q_003"); got.Blocked {
		t.Fatalf("q_003 has no resolved prerequisites and must not be Blocked: %#v", got)
	}

	// Counters treat a blocked blocking question as an ordinary open/blocking one.
	if status.Open != 3 || status.BlockingOpen != 2 || status.Answered != 0 {
		t.Fatalf("unexpected counters: open=%d blocking_open=%d answered=%d", status.Open, status.BlockingOpen, status.Answered)
	}

	// Answering the prerequisite clears the block on q_002.
	var answerStdout bytes.Buffer
	assertNoError(t, app.ClarifyAnswer(&answerStdout, runID, "q_001", "Use SQLite.", "", false))
	afterStatus, err := buildClarificationStatus(runPaths, state)
	assertNoError(t, err)
	if got := clarifyQuestionStatusByID(t, afterStatus, "q_002"); got.Blocked {
		t.Fatalf("q_002 should no longer be Blocked once q_001 is answered: %#v", got)
	}
	if got := clarifyQuestionStatusByID(t, afterStatus, "q_002"); got.Status != "open" {
		t.Fatalf("q_002 should remain open after the prerequisite is answered: %#v", got)
	}
}

func TestRecordClarifierSuggestionsWarnsWhenSchemaMarkerParsesNoBlocks(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	state := readRunState(t, runPaths.RunJSON)
	context := clarifyContext{Root: root, Paths: paths, RunPaths: runPaths, State: state}
	now := time.Unix(0, 0).UTC()

	// A block cut before its closing fence yields zero parsed blocks even
	// though the schema marker is in the output. That must warn — not silently
	// read as "no clarification needed".
	truncated := "Emitting now.\n```json\n{\"schema\": \"" + clarificationSuggestionsSchema + "\", \"questions\": [\n"
	stdoutPath := filepath.Join(root, "clarifier-stdout.log")
	mustWriteFile(t, stdoutPath, truncated)
	created, warnings, _, _, err := app.recordClarifierSuggestions(context, "clarifier_attempt_001", stdoutPath, now)
	assertNoError(t, err)
	if len(created) != 0 {
		t.Fatalf("truncated block must record no questions, got %#v", created)
	}
	if joined := strings.Join(warnings, "\n"); !strings.Contains(joined, "parse miss") {
		t.Fatalf("warnings must flag the parse miss, got:\n%s", joined)
	}

	// A parsed empty-questions block is genuine convergence: no warning.
	mustWriteFile(t, stdoutPath, clarifierStructuredOutput([]map[string]any{}))
	created, warnings, _, _, err = app.recordClarifierSuggestions(context, "clarifier_attempt_002", stdoutPath, now)
	assertNoError(t, err)
	if len(created) != 0 || len(warnings) != 0 {
		t.Fatalf("parsed empty block must record nothing and warn nothing, got created=%#v warnings=%#v", created, warnings)
	}
}

func TestRecordClarifierSuggestionsParsesFenceGluedToProse(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	state := readRunState(t, runPaths.RunJSON)
	context := clarifyContext{Root: root, Paths: paths, RunPaths: runPaths, State: state}
	now := time.Unix(0, 0).UTC()

	// The showcase regression: an id-less ACP stream glues the opening fence
	// to the tail of the preceding prose message, and the transport cannot
	// separate them. The questions must still be recorded.
	glued := "That settles the portability boundary.```json\n" +
		"{\"schema\": \"" + clarificationSuggestionsSchema + "\", \"questions\": [" +
		"{\"text\": \"What does full record mean?\", \"blocking\": true, \"rationale\": \"scope\", " +
		"\"recommended_answer\": \"the whole run tree\", \"confidence\": \"medium\"}" +
		"]}\n```\n"
	stdoutPath := filepath.Join(root, "clarifier-stdout.log")
	mustWriteFile(t, stdoutPath, glued)
	created, warnings, _, _, err := app.recordClarifierSuggestions(context, "clarifier_attempt_001", stdoutPath, now)
	assertNoError(t, err)
	if len(created) != 1 {
		t.Fatalf("glued fence must still yield the question, got created=%#v warnings=%v", created, warnings)
	}
	if len(warnings) != 0 {
		t.Fatalf("a parsed glued fence must not warn, got %v", warnings)
	}
}

func TestRecordClarifierSuggestionsNumbersDependsOnPerBlock(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	state := readRunState(t, runPaths.RunJSON)
	context := clarifyContext{Root: root, Paths: paths, RunPaths: runPaths, State: state}

	now := time.Unix(0, 0).UTC()
	// Two separate fenced suggestion blocks. depends_on positions are 1-based
	// WITHIN each block, so block 2's depends_on:[1] must resolve to block 2's
	// own first question — never block 1's first question.
	block1 := clarifierStructuredOutput([]map[string]any{
		{ // q_001 — block 1, position 1
			"text":               "Which storage backend should the cache use?",
			"blocking":           true,
			"kind":               "scope",
			"rationale":          "The backend choice constrains the schema.",
			"recommended_answer": "SQLite.",
			"confidence":         "high",
		},
	})
	block2 := clarifierStructuredOutput([]map[string]any{
		{ // q_002 — block 2, position 1 (foundational within its block)
			"text":               "What eviction policy is acceptable?",
			"blocking":           true,
			"kind":               "edge_case",
			"rationale":          "Eviction is scoped to this block.",
			"recommended_answer": "LRU.",
			"confidence":         "medium",
		},
		{ // q_003 — block 2, position 2, depends_on:[1] → block 2's first question
			"text":               "What eviction batch size is acceptable?",
			"blocking":           false,
			"kind":               "edge_case",
			"rationale":          "Batch size follows from the eviction policy.",
			"recommended_answer": "100 entries.",
			"confidence":         "low",
			"depends_on":         []int{1},
		},
	})
	stdoutPath := filepath.Join(root, "clarifier-stdout.log")
	mustWriteFile(t, stdoutPath, block1+block2)

	created, warnings, _, _, err := app.recordClarifierSuggestions(context, "clarifier_attempt_001", stdoutPath, now)
	assertNoError(t, err)
	if len(created) != 3 {
		t.Fatalf("created = %d, want 3: %#v", len(created), created)
	}
	if created[0].ID != "q_001" || created[1].ID != "q_002" || created[2].ID != "q_003" {
		t.Fatalf("unexpected ids: %q, %q, %q", created[0].ID, created[1].ID, created[2].ID)
	}
	if len(created[1].DependsOn) != 0 {
		t.Fatalf("block 2's first question must be foundational: %#v", created[1].DependsOn)
	}
	// The crux: block 2's depends_on:[1] resolves to block 2's first question
	// (q_002), not block 1's first question (q_001).
	if len(created[2].DependsOn) != 1 || created[2].DependsOn[0] != "q_002" {
		t.Fatalf("q_003 depends_on = %#v, want [q_002] (block-local, not [q_001])", created[2].DependsOn)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
}

func clarifyQuestionStatusByID(t *testing.T, status clarifyStatusResponse, id string) clarifyQuestionStatus {
	t.Helper()
	for _, question := range status.Questions {
		if question.ID == id {
			return question
		}
	}
	t.Fatalf("question %s not found in status: %#v", id, status.Questions)
	return clarifyQuestionStatus{}
}

func configureHelperClarifiers(t *testing.T, app App, paths artifacts.Paths, names ...string) App {
	t.Helper()
	registerTestAgents(t, paths, names...)
	app.AgentRegistry = testAgentRegistry(testHelperDescriptors(names, "TestClarifierHelperProcess")...)
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
			"kind":               "scope",
			"rationale":          "Approval should be reset if new human input can change scope.",
			"recommended_answer": "Yes — reset approval to clarifying so the human re-approves after answering.",
			"confidence":         "high",
		},
		{
			"text":               "Should non-blocking suggestions remain open for optional human answers?",
			"blocking":           false,
			"kind":               "acceptance",
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
