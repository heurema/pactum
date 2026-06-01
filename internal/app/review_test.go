package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
)

func TestReviewPrepareBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"review", "prepare", "run_x"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review prepare before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("review prepare before init output mismatch:\n%s", got)
	}
}

func TestReviewPrepareMissingRunReturnsError(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"review", "prepare", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review prepare missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestReviewPrepareWithoutGateReportFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "prepare", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review prepare without gate report should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare review: gate report not found") {
		t.Fatalf("missing gate stderr mismatch:\n%s", got)
	}
}

func TestReviewPrepareCreatesArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupRunWithGateReport(t, root, "needs_review")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "prepare", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review prepare exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.ReviewJSON)
	assertFile(t, runPaths.ReviewFindingsJSONL)
	assertFile(t, runPaths.ReviewResolutionsJSONL)
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Status != "pending" || review.Gate.Status != "needs_review" || review.Summary.Findings != 0 {
		t.Fatalf("unexpected review document: %#v", review)
	}
	if got := stdout.String(); !strings.Contains(got, "Review prepared") || !strings.Contains(got, "status: needs_review") {
		t.Fatalf("prepare output mismatch:\n%s", got)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_prepared") == -1 {
		t.Fatalf("events missing review_prepared:\n%v", eventTypes)
	}
}

func TestReviewStatusBeforePreparePrintsGuidance(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "status", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review status before prepare exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Review has not been prepared. Run: pactum review prepare "+runID) {
		t.Fatalf("status guidance mismatch:\n%s", got)
	}
}

func TestReviewShowBeforePreparePrintsGuidance(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review show before prepare exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Review has not been prepared. Run: pactum review prepare "+runID) {
		t.Fatalf("show guidance mismatch:\n%s", got)
	}
}

func TestReviewAddFindingUpdatesSummaryAndLedger(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "add-finding", runID, "Review command should not mutate gate report", "--blocking", "--severity", "medium", "--category", "process"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review add-finding exited %d, stderr: %s", code, stderr.String())
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || findings[0].ID != "f_001" || !findings[0].Blocking || findings[0].Category != "process" {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Status != "changes_requested" || review.Summary.Findings != 1 || review.Summary.Open != 1 || review.Summary.BlockingOpen != 1 {
		t.Fatalf("unexpected review summary: %#v", review)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_finding_added") == -1 {
		t.Fatalf("events missing review_finding_added:\n%v", eventTypes)
	}
}

func TestReviewAddFindingRejectsAbsoluteFilePath(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "add-finding", runID, "absolute path", "--file", filepath.Join(root, "main.go")}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review add-finding with absolute file should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "review finding file must be repo-relative") {
		t.Fatalf("absolute path stderr mismatch:\n%s", got)
	}
}

func TestReviewResolveFindingUpdatesStatusAndLedger(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "blocking process issue", "--blocking", "--severity", "high", "--category", "process")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "resolve", runID, "f_001", "--note", "Verified review commands are read/write only in review artifacts."}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review resolve exited %d, stderr: %s", code, stderr.String())
	}
	resolutions := readReviewResolutions(t, runPaths.ReviewResolutionsJSONL)
	if len(resolutions) != 1 || resolutions[0].ID != "r_001" || resolutions[0].FindingID != "f_001" {
		t.Fatalf("unexpected resolutions: %#v", resolutions)
	}
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Status != "pending" || review.Summary.Open != 0 || review.Summary.Resolved != 1 || review.Summary.BlockingOpen != 0 {
		t.Fatalf("unexpected review summary after resolve: %#v", review)
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"review", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review show exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "status: resolved") {
		t.Fatalf("show should display resolved finding:\n%s", got)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_finding_resolved") == -1 {
		t.Fatalf("events missing review_finding_resolved:\n%v", eventTypes)
	}
}

func TestReviewResolveLatestResolutionWins(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "quality issue")
	runReviewCommand(t, app, "review", "resolve", runID, "f_001", "--note", "first note")
	runReviewCommand(t, app, "review", "resolve", runID, "f_001", "--note", "second note")

	resolutions := readReviewResolutions(t, runPaths.ReviewResolutionsJSONL)
	if len(resolutions) != 2 || resolutions[0].ID != "r_001" || resolutions[1].ID != "r_002" {
		t.Fatalf("resolutions should remain append-only: %#v", resolutions)
	}
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review show exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "second note") || strings.Contains(got, "first note") {
		t.Fatalf("latest resolution did not win for display:\n%s", got)
	}
}

func TestReviewApproveBlocksWithOpenBlockingFinding(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "blocking process issue", "--blocking")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "approve", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review approve should fail with open blocking finding")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot approve review: blocking review findings remain") {
		t.Fatalf("approve blocking stderr mismatch:\n%s", got)
	}
	if review := readReviewDoc(t, runPaths.ReviewJSON); review.Status != "changes_requested" {
		t.Fatalf("review status should remain changes_requested: %#v", review)
	}
}

func TestReviewApproveBlocksIfGateFailed(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "failed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "approve", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review approve should fail with failed gate")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot approve review: gate status is failed") {
		t.Fatalf("failed gate stderr mismatch:\n%s", got)
	}
	if review := readReviewDoc(t, runPaths.ReviewJSON); review.Status != "pending" {
		t.Fatalf("review status should remain pending: %#v", review)
	}
}

func TestReviewApproveSucceeds(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "needs_review")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "approve", runID, "--by", "manual"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review approve exited %d, stderr: %s", code, stderr.String())
	}
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Status != "approved" || review.Approval.ApprovedAt == nil || review.Approval.ApprovedBy == nil || *review.Approval.ApprovedBy != "manual" {
		t.Fatalf("unexpected approved review: %#v", review)
	}
	if got := stdout.String(); !strings.Contains(got, "Review approved") || !strings.Contains(got, "approved by: manual") {
		t.Fatalf("approve output mismatch:\n%s", got)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_approved") == -1 {
		t.Fatalf("events missing review_approved:\n%v", eventTypes)
	}
}

func TestReviewAddFindingAfterApprovedResetsApproval(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "approve", runID, "--by", "manual")

	runReviewCommand(t, app, "review", "add-finding", runID, "new blocking issue", "--blocking")
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Status != "changes_requested" || review.Approval.ApprovedAt != nil || review.Approval.ApprovedBy != nil {
		t.Fatalf("approval should reset after new finding: %#v", review)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_approval_reset") == -1 {
		t.Fatalf("events missing review_approval_reset:\n%v", eventTypes)
	}
}

func TestReviewStatusJSONIncludesSummary(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "process issue", "--category", "process")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "status", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review status --json exited %d, stderr: %s", code, stderr.String())
	}
	var state reviewStateResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &state))
	if state.Review.Summary.Findings != 1 || state.Review.Summary.Open != 1 {
		t.Fatalf("status json missing summary: %#v", state)
	}
}

func TestReviewShowJSONIncludesFindings(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "quality issue", "--category", "quality")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review show --json exited %d, stderr: %s", code, stderr.String())
	}
	var state reviewStateResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &state))
	if len(state.Findings) != 1 || state.Findings[0].ID != "f_001" || state.Findings[0].Category != "quality" {
		t.Fatalf("show json missing findings: %#v", state)
	}
}

func TestReviewStatusAndShowAreReadOnly(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "process issue")

	beforeReview := mustReadFile(t, runPaths.ReviewJSON)
	beforeLedger := mustReadFile(t, paths.EventsJSONL)
	runReviewCommand(t, app, "review", "status", runID)
	runReviewCommand(t, app, "review", "show", runID)
	if got := mustReadFile(t, runPaths.ReviewJSON); got != beforeReview {
		t.Fatalf("status/show mutated review.json")
	}
	if got := mustReadFile(t, paths.EventsJSONL); got != beforeLedger {
		t.Fatalf("status/show mutated ledger")
	}
}

func TestReviewArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "portable path issue", "--file", "internal/app/review.go", "--line", "123")
	runReviewCommand(t, app, "review", "resolve", runID, "f_001", "--note", "relative artifacts only")

	for name, content := range map[string]string{
		"review/review.json":       mustReadFile(t, runPaths.ReviewJSON),
		"review/findings.jsonl":    mustReadFile(t, runPaths.ReviewFindingsJSONL),
		"review/resolutions.jsonl": mustReadFile(t, runPaths.ReviewResolutionsJSONL),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func setupRunWithGateReport(t *testing.T, root string, status string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertNoError(t, os.MkdirAll(runPaths.GateDir, 0o755))
	report := gateReportDocument{
		Schema:    gateReportSchema,
		RunID:     runID,
		CreatedAt: "2026-06-01T22:00:00Z",
		Status:    status,
	}
	assertNoError(t, writeJSON(runPaths.GateReportJSON, report))
	return app, paths, runID, runPaths
}

func setupPreparedReview(t *testing.T, root string, gateStatus string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID, runPaths := setupRunWithGateReport(t, root, gateStatus)
	runReviewCommand(t, app, "review", "prepare", runID)
	return app, paths, runID, runPaths
}

func runReviewCommand(t *testing.T, app App, args ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := app.Run(args, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("%v exited %d, stdout: %s stderr: %s", args, code, stdout.String(), stderr.String())
	}
}

func readReviewDoc(t *testing.T, path string) reviewDocument {
	t.Helper()
	var review reviewDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &review))
	return review
}

func readReviewFindings(t *testing.T, path string) []reviewFindingRecord {
	t.Helper()
	lines := readLines(t, path)
	records := make([]reviewFindingRecord, 0, len(lines))
	for _, line := range lines {
		var record reviewFindingRecord
		assertNoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func readReviewResolutions(t *testing.T, path string) []reviewResolutionRecord {
	t.Helper()
	lines := readLines(t, path)
	records := make([]reviewResolutionRecord, 0, len(lines))
	for _, line := range lines {
		var record reviewResolutionRecord
		assertNoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}
