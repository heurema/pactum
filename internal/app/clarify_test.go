package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildClarificationStatusCoverageTally(t *testing.T) {
	root := t.TempDir()
	_, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	state := readRunState(t, runPaths.RunJSON)

	// terminology has an open-blocking question and an answered one (the
	// answered-vs-open split); scope and edge_case each have one question;
	// acceptance and assumption stay at zero (an unprobed canonical dimension).
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_001", "terminology", true)
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_002", "terminology", false)
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_003", "scope", false)
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_004", "edge_case", true)
	appendClarificationAnswerForTest(t, runPaths.AnswersJSONL, "a_001", "q_002")
	appendClarificationAnswerForTest(t, runPaths.AnswersJSONL, "a_002", "q_004")

	status, err := buildClarificationStatus(runPaths, state)
	assertNoError(t, err)

	// The five canonical dimensions are always present, in fixed order, and
	// 'other' is absent because every question used a canonical kind.
	if got := coverageKinds(status.Coverage); !equalStrings(got, canonicalClarificationKinds) {
		t.Fatalf("coverage kinds = %v, want the five canonical dimensions in order", got)
	}

	wantByKind := map[string]clarifyKindCoverage{
		"terminology": {Kind: "terminology", Total: 2, Open: 1, Answered: 1, BlockingOpen: 1},
		"scope":       {Kind: "scope", Total: 1, Open: 1, Answered: 0, BlockingOpen: 0},
		"acceptance":  {Kind: "acceptance"},
		"edge_case":   {Kind: "edge_case", Total: 1, Open: 0, Answered: 1, BlockingOpen: 0},
		"assumption":  {Kind: "assumption"},
	}
	for _, coverage := range status.Coverage {
		if coverage != wantByKind[coverage.Kind] {
			t.Fatalf("coverage[%s] = %#v, want %#v", coverage.Kind, coverage, wantByKind[coverage.Kind])
		}
	}

	// The per-kind tallies sum back to the overall counters.
	var sumTotal, sumOpen, sumAnswered, sumBlocking int
	for _, coverage := range status.Coverage {
		sumTotal += coverage.Total
		sumOpen += coverage.Open
		sumAnswered += coverage.Answered
		sumBlocking += coverage.BlockingOpen
	}
	if sumTotal != status.Total || sumOpen != status.Open || sumAnswered != status.Answered || sumBlocking != status.BlockingOpen {
		t.Fatalf("coverage sums (%d/%d/%d/%d) do not match overall (%d/%d/%d/%d)",
			sumTotal, sumOpen, sumAnswered, sumBlocking,
			status.Total, status.Open, status.Answered, status.BlockingOpen)
	}

	// One blocking question is still open, so the run has not converged.
	if status.Converged {
		t.Fatalf("status should not be converged while a blocking question is open: %#v", status)
	}
}

func TestBuildClarificationStatusOtherBucketOnlyWhenUsed(t *testing.T) {
	root := t.TempDir()
	_, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	state := readRunState(t, runPaths.RunJSON)

	// Before any non-canonical question, 'other' must not appear.
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_001", "scope", false)
	beforeStatus, err := buildClarificationStatus(runPaths, state)
	assertNoError(t, err)
	if coverageByKind(beforeStatus.Coverage, "other") != nil {
		t.Fatalf("'other' should be absent until a non-canonical question exists: %#v", beforeStatus.Coverage)
	}
	if len(beforeStatus.Coverage) != len(canonicalClarificationKinds) {
		t.Fatalf("coverage should hold only the canonical dimensions: %#v", beforeStatus.Coverage)
	}

	// An explicit 'other'-kind question and a kind-less manual question both
	// fall into the single 'other' catch-all bucket, appended last.
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_002", "other", false)
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_003", "", true)
	afterStatus, err := buildClarificationStatus(runPaths, state)
	assertNoError(t, err)
	other := coverageByKind(afterStatus.Coverage, "other")
	if other == nil {
		t.Fatalf("'other' should be present once a non-canonical question exists: %#v", afterStatus.Coverage)
	}
	if want := (clarifyKindCoverage{Kind: "other", Total: 2, Open: 2, Answered: 0, BlockingOpen: 1}); *other != want {
		t.Fatalf("'other' coverage = %#v, want %#v", *other, want)
	}
	if last := afterStatus.Coverage[len(afterStatus.Coverage)-1]; last.Kind != "other" {
		t.Fatalf("'other' must be the last coverage entry, got %q", last.Kind)
	}
}

func TestBuildClarificationStatusConvergedReflectsBlockingOpen(t *testing.T) {
	root := t.TempDir()
	_, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	state := readRunState(t, runPaths.RunJSON)

	// An open non-blocking question and an answered blocking question leave no
	// open blocking work, so the run is converged even though one question is
	// still open.
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_001", "scope", false)
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_002", "acceptance", true)
	appendClarificationAnswerForTest(t, runPaths.AnswersJSONL, "a_001", "q_002")

	status, err := buildClarificationStatus(runPaths, state)
	assertNoError(t, err)
	if status.BlockingOpen != 0 {
		t.Fatalf("blocking open = %d, want 0", status.BlockingOpen)
	}
	if !status.Converged {
		t.Fatalf("status should be converged when no blocking question is open: %#v", status)
	}

	// Adding an open blocking question flips Converged back to false.
	appendClarificationQuestionForTest(t, runPaths.QuestionsJSONL, "q_003", "edge_case", true)
	reopened, err := buildClarificationStatus(runPaths, state)
	assertNoError(t, err)
	if reopened.BlockingOpen != 1 || reopened.Converged {
		t.Fatalf("status should not be converged with an open blocking question: %#v", reopened)
	}
}

func TestBuildClarificationStatusCanonicalDimensionsAlwaysPresent(t *testing.T) {
	root := t.TempDir()
	_, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	state := readRunState(t, runPaths.RunJSON)

	// With no questions at all, every canonical dimension is still listed (at
	// zero), in order, no 'other', and an empty run is converged.
	status, err := buildClarificationStatus(runPaths, state)
	assertNoError(t, err)
	if got := coverageKinds(status.Coverage); !equalStrings(got, canonicalClarificationKinds) {
		t.Fatalf("coverage kinds = %v, want the five canonical dimensions in order", got)
	}
	for _, coverage := range status.Coverage {
		if (coverage != clarifyKindCoverage{Kind: coverage.Kind}) {
			t.Fatalf("empty run should leave %q at zero: %#v", coverage.Kind, coverage)
		}
	}
	if status.Total != 0 || !status.Converged {
		t.Fatalf("empty run should be converged with no questions: %#v", status)
	}
}

func TestWriteClarifyStatusShowsCoverageAndConverged(t *testing.T) {
	status := clarifyStatusResponse{
		RunID:        "run_20260101_000000",
		RunStatus:    "clarifying",
		Total:        2,
		Answered:     1,
		Open:         1,
		BlockingOpen: 1,
		Converged:    false,
		Coverage: []clarifyKindCoverage{
			{Kind: "terminology", Total: 2, Open: 1, Answered: 1, BlockingOpen: 1},
			{Kind: "scope"},
			{Kind: "acceptance"},
			{Kind: "edge_case"},
			{Kind: "assumption"},
		},
		Questions: []clarifyQuestionStatus{},
	}

	var buf bytes.Buffer
	writeClarifyStatus(&buf, status)
	got := buf.String()
	for _, want := range []string{
		"converged: no",
		"Coverage by dimension:",
		"- terminology: total 2, answered 1, open 1, blocking open 1",
		"- acceptance: total 0, answered 0, open 0, blocking open 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify status output missing %q:\n%s", want, got)
		}
	}
}

func TestClarifyAskSurfacesApprovalResetOnApprovedRun(t *testing.T) {
	t.Run("approved run warns, regresses, and keeps the ledger event", func(t *testing.T) {
		root := t.TempDir()
		app, paths, runID := setupContractRun(t, root)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		approveRunForTest(t, app, runID)

		var stdout bytes.Buffer
		assertNoError(t, app.ClarifyAsk(&stdout, runID, "Which storage backend should the cache use?", true, false))

		got := stdout.String()
		for _, want := range []string{
			"this run was approved",
			"reset approval to pending",
			`status now "clarifying"`,
			"pactum contract approve " + runID,
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("ask output missing approval-reset warning %q:\n%s", want, got)
			}
		}

		// An open blocking question regresses the run to clarifying and its approval
		// to pending, and the reset is still recorded in the ledger.
		if state := readRunState(t, runPaths.RunJSON); state.Status != "clarifying" {
			t.Fatalf("run status = %q, want clarifying", state.Status)
		}
		if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil {
			t.Fatalf("approval not reset to pending: %#v", approval)
		}
		if eventTypes := ledgerEventTypes(t, paths.EventsJSONL); indexOfEvent(eventTypes, "contract_approval_reset") == -1 {
			t.Fatalf("events missing contract_approval_reset:\n%v", eventTypes)
		}

		// JSON output reports approval_reset=true (checked on a fresh approved run,
		// since the run above is no longer approved).
		jsonRoot := t.TempDir()
		jsonApp, _, jsonRunID := setupContractRun(t, jsonRoot)
		approveRunForTest(t, jsonApp, jsonRunID)
		var jsonOut bytes.Buffer
		assertNoError(t, jsonApp.ClarifyAsk(&jsonOut, jsonRunID, "Which storage backend?", true, true))
		var response clarifyAskResponse
		assertNoError(t, json.Unmarshal(jsonOut.Bytes(), &response))
		if !response.ApprovalReset {
			t.Fatalf("approved run should report approval_reset=true: %#v", response)
		}
	})

	t.Run("non-approved run leaves approval_reset false and prints no warning", func(t *testing.T) {
		root := t.TempDir()
		app, _, runID := setupContractRun(t, root)

		var jsonOut bytes.Buffer
		assertNoError(t, app.ClarifyAsk(&jsonOut, runID, "Which storage backend?", true, true))
		if strings.Contains(jsonOut.String(), "approval_reset") {
			t.Fatalf("non-approved run should omit approval_reset from JSON:\n%s", jsonOut.String())
		}
		var response clarifyAskResponse
		assertNoError(t, json.Unmarshal(jsonOut.Bytes(), &response))
		if response.ApprovalReset {
			t.Fatalf("non-approved run should report approval_reset=false: %#v", response)
		}

		var stdout bytes.Buffer
		assertNoError(t, app.ClarifyAsk(&stdout, runID, "Another question?", false, false))
		if strings.Contains(stdout.String(), "this run was approved") {
			t.Fatalf("non-approved run should not warn about approval reset:\n%s", stdout.String())
		}
	})
}

func TestClarifyAnswerSurfacesApprovalResetOnApprovedRun(t *testing.T) {
	t.Run("approved run warns, regresses, and keeps the ledger event", func(t *testing.T) {
		root := t.TempDir()
		app, paths, runID := setupContractRun(t, root)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

		// A non-blocking question leaves the contract approvable (only open blocking
		// questions block approval), so the run can reach the approved state first.
		var setup bytes.Buffer
		assertNoError(t, app.ClarifyAsk(&setup, runID, "Any preferred eviction policy?", false, false))
		approveRunForTest(t, app, runID)

		var stdout bytes.Buffer
		assertNoError(t, app.ClarifyAnswer(&stdout, runID, "q_001", "LRU is fine.", "", false))

		got := stdout.String()
		for _, want := range []string{
			"this run was approved",
			"reset approval to pending",
			// No blocking question is open, so the run lands in contract_draft; the
			// warning names that precise status rather than contradicting it.
			`status now "contract_draft"`,
			"pactum contract approve " + runID,
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("answer output missing approval-reset warning %q:\n%s", want, got)
			}
		}
		if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil {
			t.Fatalf("approval not reset to pending: %#v", approval)
		}
		if eventTypes := ledgerEventTypes(t, paths.EventsJSONL); indexOfEvent(eventTypes, "contract_approval_reset") == -1 {
			t.Fatalf("events missing contract_approval_reset:\n%v", eventTypes)
		}
	})

	t.Run("non-approved run leaves approval_reset false and prints no warning", func(t *testing.T) {
		root := t.TempDir()
		app, _, runID := setupContractRun(t, root)
		var setup bytes.Buffer
		assertNoError(t, app.ClarifyAsk(&setup, runID, "Any preferred eviction policy?", false, false))

		var jsonOut bytes.Buffer
		assertNoError(t, app.ClarifyAnswer(&jsonOut, runID, "q_001", "LRU is fine.", "", true))
		if strings.Contains(jsonOut.String(), "approval_reset") {
			t.Fatalf("non-approved run should omit approval_reset from JSON:\n%s", jsonOut.String())
		}
		var response clarifyAnswerResponse
		assertNoError(t, json.Unmarshal(jsonOut.Bytes(), &response))
		if response.ApprovalReset {
			t.Fatalf("non-approved run should report approval_reset=false: %#v", response)
		}

		var stdout bytes.Buffer
		assertNoError(t, app.ClarifyAnswer(&stdout, runID, "q_001", "LRU again.", "", false))
		if strings.Contains(stdout.String(), "this run was approved") {
			t.Fatalf("non-approved run should not warn about approval reset:\n%s", stdout.String())
		}
	})
}

// TestClarifyAnswerRecordsExplicitBy covers --by on clarify answer: the trimmed
// principal is persisted as the decision record's decided_by, and repo-root
// paths are sanitized before persistence (the manual default is pinned
// byte-for-byte in TestClarifyAnswerManualRecordsStayByteIdentical).
func TestClarifyAnswerRecordsExplicitBy(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var setup bytes.Buffer
	assertNoError(t, app.ClarifyAsk(&setup, runID, "Which backend?", true, false))
	assertNoError(t, app.ClarifyAsk(&setup, runID, "Which agent answered?", true, false))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "answer", runID, "q_001", "Use SQLite.", "--by", "  bob  "}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify answer --by exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"clarify", "answer", runID, "q_002", "The executor.", "--by", root + "/agent"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify answer --by exited %d, stderr: %s", code, stderr.String())
	}
	decisions, err := readClarificationDecisions(runPaths.DecisionsJSONL)
	assertNoError(t, err)
	if len(decisions) != 2 || decisions[0].DecidedBy != "bob" || decisions[0].Source != "manual_answer" {
		t.Fatalf("decided_by mismatch: %#v", decisions)
	}
	if decisions[1].DecidedBy == "" || strings.Contains(decisions[1].DecidedBy, root) {
		t.Fatalf("decided_by not sanitized: %#v", decisions[1])
	}
	assertDoesNotContainRoot(t, "clarifications/decisions.jsonl", mustReadFile(t, runPaths.DecisionsJSONL), root)
}

func approveRunForTest(t *testing.T, app App, runID string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
}

func appendClarificationQuestionForTest(t *testing.T, path string, id string, kind string, blocking bool) {
	t.Helper()
	assertNoError(t, appendJSONLine(path, clarificationQuestionRecord{
		Schema:   clarificationQuestionSchema,
		ID:       id,
		Question: "Question " + id,
		Blocking: blocking,
		Kind:     kind,
		Status:   "open",
	}))
}

func appendClarificationAnswerForTest(t *testing.T, path string, id string, questionID string) {
	t.Helper()
	assertNoError(t, appendJSONLine(path, clarificationAnswerRecord{
		Schema:     clarificationAnswerSchema,
		ID:         id,
		QuestionID: questionID,
		Answer:     "answer for " + questionID,
	}))
}

func coverageKinds(coverage []clarifyKindCoverage) []string {
	kinds := make([]string, 0, len(coverage))
	for _, c := range coverage {
		kinds = append(kinds, c.Kind)
	}
	return kinds
}

func coverageByKind(coverage []clarifyKindCoverage, kind string) *clarifyKindCoverage {
	for i := range coverage {
		if coverage[i].Kind == kind {
			return &coverage[i]
		}
	}
	return nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
