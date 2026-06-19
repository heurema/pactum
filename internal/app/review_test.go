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

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

func TestReviewFindingAddBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"review", "finding", "add", "run_x", "note"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("review finding add before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("review finding add before init stderr mismatch:\n%s", got)
	}
}

func TestReviewFindingAddMissingRunReturnsError(t *testing.T) {
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
	code = app.Run([]string{"review", "finding", "add", "run_missing", "note"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review finding add for a missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestReviewFindingAddWithoutGateReportFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "finding", "add", runID, "note"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review finding add without gate report should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot add review finding: gate report not found") {
		t.Fatalf("missing gate stderr mismatch:\n%s", got)
	}
}

func TestReviewFindingAddScaffoldsReviewArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupRunWithGateReport(t, root, "needs_review")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "finding", "add", runID, "needs a closer look"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review finding add exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.ReviewJSON)
	assertFile(t, runPaths.ReviewFindingsJSONL)
	assertFile(t, runPaths.ReviewResolutionsJSONL)
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Gate.Status != "needs_review" || review.Summary.Findings != 1 {
		t.Fatalf("unexpected review document: %#v", review)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_scaffolded") == -1 {
		t.Fatalf("events missing review_scaffolded:\n%v", eventTypes)
	}
}

func TestReviewApproveFailsClosedWhenFindingsAbsent(t *testing.T) {
	// review approve must not approve when review/findings.jsonl does not exist:
	// ensureReviewRecord would create an empty file and let zero-finding approval
	// look legitimate, so we gate on findings existence before scaffolding.
	root := t.TempDir()
	app, _, runID, runPaths := setupRunWithGateReport(t, root, "passed")
	assertNoFile(t, runPaths.ReviewFindingsJSONL)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "approve", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review approve must refuse when findings.jsonl is absent; got exit 0\nstdout: %s", stdout.String())
	}
	// Review record must not have been created.
	assertNoFile(t, runPaths.ReviewJSON)
}

func TestReviewStatusNotGatedPrintsGuidance(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "status", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review status before gate exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Run has not been gated. Run: pactum gate run "+runID) {
		t.Fatalf("status guidance mismatch:\n%s", got)
	}
}

func TestReviewStatusOnGatedRunShowsDerivedPendingState(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "status", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review status on gated run exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"Review status", "status: pending", "findings: 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("derived status output missing %q:\n%s", want, got)
		}
	}
	assertNoFile(t, runPaths.ReviewJSON)
	assertNoFile(t, runPaths.ReviewFindingsJSONL)
}

func TestReviewShowOnGatedRunShowsDerivedPendingState(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review show on gated run exited %d, stderr: %s", code, stderr.String())
	}
	var state reviewStateResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &state))
	if state.Review.Status != "pending" || state.Review.RunID != runID || len(state.Findings) != 0 {
		t.Fatalf("derived review state mismatch: %#v", state.Review)
	}
	assertNoFile(t, runPaths.ReviewJSON)
}

func TestReviewAddFindingUpdatesSummaryAndLedger(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "finding", "add", runID, "Review command should not mutate gate report", "--blocking", "--severity", "medium", "--category", "process"}, &stdout, &stderr)
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

func TestReviewAddFindingRefreshesCurrentGateStatus(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewGateReportForTest(t, runPaths, runID, "failed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "finding", "add", runID, "gate changed after prepare"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review add-finding exited %d, stderr: %s", code, stderr.String())
	}
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Gate.Status != "failed" {
		t.Fatalf("review gate status should refresh from current gate report: %#v", review.Gate)
	}
}

func TestReviewAddFindingRejectsAbsoluteFilePath(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "finding", "add", runID, "absolute path", "--file", filepath.Join(root, "main.go")}, &stdout, &stderr)
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
	runReviewCommand(t, app, "review", "finding", "add", runID, "blocking process issue", "--blocking", "--severity", "high", "--category", "process")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "finding", "resolve", runID, "f_001", "--note", "Verified review commands are read/write only in review artifacts."}, &stdout, &stderr)
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

func TestReviewResolveRefreshesCurrentGateStatus(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "finding", "add", runID, "blocking process issue", "--blocking")
	writeReviewGateReportForTest(t, runPaths, runID, "failed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "finding", "resolve", runID, "f_001", "--note", "gate changed after prepare"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review resolve exited %d, stderr: %s", code, stderr.String())
	}
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Gate.Status != "failed" {
		t.Fatalf("review gate status should refresh from current gate report: %#v", review.Gate)
	}
}

func TestReviewResolveLatestResolutionWins(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "finding", "add", runID, "quality issue")
	runReviewCommand(t, app, "review", "finding", "resolve", runID, "f_001", "--note", "first note")
	runReviewCommand(t, app, "review", "finding", "resolve", runID, "f_001", "--note", "second note")

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
	runReviewCommand(t, app, "review", "finding", "add", runID, "blocking process issue", "--blocking")

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

// TestReviewApproveGuardFailClosedOnMalformedFindings verifies that malformed
// JSON in the findings JSONL file causes approve to refuse rather than silently
// treating the run as finding-clean.
func TestReviewApproveGuardFailClosedOnMalformedFindings(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	// Write a syntactically-broken findings file.
	if err := activeStore.WriteBytes(runPaths.ReviewFindingsJSONL, []byte("not-valid-json\n"), 0o644); err != nil {
		t.Fatalf("cannot write malformed findings: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "approve", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review approve must refuse when findings file is malformed; got exit 0\nstdout: %s", stdout.String())
	}
}

func TestReviewAddFindingAfterApprovedResetsApproval(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "approve", runID, "--by", "manual")

	runReviewCommand(t, app, "review", "finding", "add", runID, "new blocking issue", "--blocking")
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
	runReviewCommand(t, app, "review", "finding", "add", runID, "process issue", "--category", "process")

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
	runReviewCommand(t, app, "review", "finding", "add", runID, "quality issue", "--category", "quality")

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
	runReviewCommand(t, app, "review", "finding", "add", runID, "process issue")

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
	runReviewCommand(t, app, "review", "finding", "add", runID, "portable path issue", "--file", "internal/app/review.go", "--line", "123")
	runReviewCommand(t, app, "review", "finding", "resolve", runID, "f_001", "--note", "relative artifacts only")

	for name, content := range map[string]string{
		"review/review.json":       mustReadFile(t, runPaths.ReviewJSON),
		"review/findings.jsonl":    mustReadFile(t, runPaths.ReviewFindingsJSONL),
		"review/resolutions.jsonl": mustReadFile(t, runPaths.ReviewResolutionsJSONL),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestReviewPlanBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"review", "plan", "run_x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("review plan before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("review plan before init stderr mismatch:\n%s", got)
	}
}

func TestReviewPlanMissingRunReturnsError(t *testing.T) {
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
	code = app.Run([]string{"review", "plan", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review plan missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestReviewPlanOnGatedRunDerivesReviewState(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	writeReviewGateReportForTest(t, runPaths, runID, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review plan on a gated run exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.ReviewDryRunJSON)
	assertNoFile(t, runPaths.ReviewJSON)
}

func TestReviewPlanMissingGateReportFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedReviewWithoutGateReport(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review plan should fail without gate report")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare reviewer plan: gate report not found") {
		t.Fatalf("missing gate stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.ReviewDryRunJSON)
}

func TestReviewPlanContractNotApprovedFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review plan should fail without approved contract")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare reviewer plan: contract is not approved") {
		t.Fatalf("unapproved contract stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.ReviewDryRunJSON)
}

func TestReviewPlanSucceeds(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review plan exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.ReviewContextMD)
	for _, lens := range reviewLenses {
		assertFile(t, reviewerLensPromptPath(runPaths, "claude", lens))
	}
	assertFile(t, runPaths.ReviewDryRunJSON)
	got := stdout.String()
	// No execution attempt exists, so cross-model selection treats the first
	// registry entry (codex) as the would-be executor and picks claude.
	// Claude runs over ACP; the "Would run" section shows empty commands.
	for _, want := range []string{
		"Reviewer plan prepared",
		"Resolved:",
		"Would run (one attempt per lens):",
		".heurema/pactum/runs/" + runID + "/review/reviewer-context.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("review plan output missing %q:\n%s", want, got)
		}
	}
	assertResolvedBlock(t, got, "claude", "claude-opus-4-8", "inherit", "partial")
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Schema != reviewerDryRunSchema || plan.RunID != runID || plan.Reviewer.Name != "claude" {
		t.Fatalf("unexpected reviewer dry-run plan: %#v", plan)
	}
	if len(plan.Attempts) != len(reviewLenses) {
		t.Fatalf("dry-run should list one attempt per lens: %#v", plan.Attempts)
	}
	for index, lens := range reviewLenses {
		attempt := plan.Attempts[index]
		wantArtifact := reviewerLensPromptArtifact("claude", lens)
		if attempt.Lens != lens.Key || attempt.Artifacts.ReviewerPrompt != wantArtifact {
			t.Fatalf("unexpected lens attempt plan: %#v", attempt)
		}
		// Claude runs over ACP; WouldRun carries the ACP adapter command.
		if attempt.WouldRun.Command != "npx" || strings.Join(attempt.WouldRun.Args, " ") != "-y @agentclientprotocol/claude-agent-acp@latest" {
			t.Fatalf("claude reviewer would_run must carry ACP adapter command: %#v", attempt.WouldRun)
		}
	}
	prompt := mustReadFile(t, reviewerLensPromptPath(runPaths, "claude", reviewLenses[0]))
	for _, want := range []string{
		"Reviewer context: .heurema/pactum/runs/" + runID + "/review/reviewer-context.md",
		"Contract: .heurema/pactum/runs/" + runID + "/contract/contract.json",
		"Gate report: .heurema/pactum/runs/" + runID + "/gate/gate-report.json",
		"Review artifacts: .heurema/pactum/runs/" + runID + "/review/review.json",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("reviewer prompt missing runnable path %q:\n%s", want, prompt)
		}
	}
}

func TestReviewPlanJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review plan --json exited %d, stderr: %s", code, stderr.String())
	}
	var plan reviewerDryRunDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &plan))
	if plan.Reviewer.Name != "claude" || !plan.Checks.ReviewPrepared || !plan.Checks.GateReportReady || !plan.Checks.ContractApproved {
		t.Fatalf("unexpected reviewer dry-run json: %#v", plan)
	}
	if len(plan.Attempts) != len(reviewLenses) {
		t.Fatalf("reviewer dry-run json should list the lens fan-out: %#v", plan)
	}
	first := plan.Attempts[0]
	if first.Lens != "correctness" ||
		first.Artifacts.ReviewerPrompt != reviewerLensPromptArtifact("claude", reviewLenses[0]) ||
		first.Artifacts.ReviewerContext != reviewerContextArtifact {
		t.Fatalf("reviewer dry-run json missing artifacts: %#v", first)
	}
	// Claude runs over ACP; WouldRun carries the ACP adapter command.
	if first.WouldRun.Command != "npx" || strings.Join(first.WouldRun.Args, " ") != "-y @agentclientprotocol/claude-agent-acp@latest" {
		t.Fatalf("claude reviewer would_run must carry ACP adapter command: %#v", first.WouldRun)
	}
	if strings.Contains(stdout.String(), "Reviewer plan prepared") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Resolved:") {
		t.Fatalf("json output should not include resolved human output:\n%s", stdout.String())
	}
}

func TestReviewPlanUsesDefaultReviewer(t *testing.T) {
	// With no execution attempt the would-be executor is the first registry
	// entry (codex), so the default reviewer is the first entry on a different
	// built-in (claude).
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")

	runReviewCommand(t, app, "review", "plan", runID)
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	// Claude runs over ACP; Command is empty (no CLI path).
	if plan.Reviewer.Name != "claude" || plan.Reviewer.Command != "" {
		t.Fatalf("default reviewer mismatch: %#v", plan.Reviewer)
	}
	wouldRun := plan.Attempts[0].WouldRun
	// Claude ACP: WouldRun carries the adapter command, not a CLI invocation.
	if wouldRun.Command != "npx" || strings.Join(wouldRun.Args, " ") != "-y @agentclientprotocol/claude-agent-acp@latest" {
		t.Fatalf("default reviewer would_run must carry ACP adapter command: %#v", wouldRun)
	}
}

func TestReviewPlanCrossModelReviewSelectsOppositeBuiltIn(t *testing.T) {
	for _, tc := range []struct {
		executor  string
		want      string
		wantModel string
	}{
		{executor: agents.BuiltinCodex, want: agents.BuiltinClaude, wantModel: "claude-opus-4-8"},
		{executor: agents.BuiltinClaude, want: agents.BuiltinCodex, wantModel: "gpt-5"},
	} {
		t.Run(tc.executor, func(t *testing.T) {
			root := t.TempDir()
			app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
			writeExecutionAttemptForTest(t, runPaths, runID, "attempt_001", mustResolveExecutorForTest(t, tc.executor))

			var stdout, stderr bytes.Buffer
			code := app.Run([]string{"review", "plan", runID}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("review plan exited %d, stderr: %s", code, stderr.String())
			}
			plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
			if plan.Reviewer.Name != tc.want {
				t.Fatalf("cross-model reviewer = %q, want %q; plan: %#v", plan.Reviewer.Name, tc.want, plan.Reviewer)
			}
			assertResolvedBlock(t, stdout.String(), tc.want, tc.wantModel, "inherit", "partial")
		})
	}
}

func TestReviewPlanCrossModelReviewExplicitReviewerWins(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	writeExecutionAttemptForTest(t, runPaths, runID, "attempt_001", mustResolveExecutorForTest(t, agents.BuiltinCodex))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID, "--reviewer", agents.BuiltinCodex}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review plan exited %d, stderr: %s", code, stderr.String())
	}
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Reviewer.Name != agents.BuiltinCodex {
		t.Fatalf("explicit reviewer should win, got: %#v", plan.Reviewer)
	}
	assertResolvedBlock(t, stdout.String(), agents.BuiltinCodex, "gpt-5", "inherit", "partial")
}

func TestReviewPlanCrossModelSkipsSameUnderlyingEntries(t *testing.T) {
	// The first entry differing by UNDERLYING agent wins: fable runs on claude
	// like the executor, so codex is selected even though fable comes first.
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths,
		agentRegistryEntry{Name: "claude", Model: "claude-opus-4-8"},
		agentRegistryEntry{Name: "fable", Model: "claude-fable-5"},
		agentRegistryEntry{Name: "codex", Model: "gpt-5"},
	)
	writeExecutionAttemptForTest(t, runPaths, runID, "attempt_001", mustResolveExecutorForTest(t, agents.BuiltinClaude))

	runReviewCommand(t, app, "review", "plan", runID)
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Reviewer.Name != agents.BuiltinCodex {
		t.Fatalf("cross-model should skip same-underlying entries, got: %#v", plan.Reviewer)
	}
}

func TestReviewPlanCrossModelFallsBackToFirstEntryWhenNoOtherBuiltIn(t *testing.T) {
	// Every registry entry runs on the executor's built-in, so cross-model
	// selection falls back to the first registry entry.
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	writeExecutionAttemptForTest(t, runPaths, runID, "attempt_001", mustResolveExecutorForTest(t, agents.BuiltinCodex))

	runReviewCommand(t, app, "review", "plan", runID)
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Reviewer.Name != agents.BuiltinCodex {
		t.Fatalf("same-built-in registry should fall back to the first entry, got: %#v", plan.Reviewer)
	}
}

func TestReviewPlanCrossModelReviewForNonBuiltInExecutor(t *testing.T) {
	// The recorded executor is not a built-in, so every registry entry's
	// underlying agent differs and the first entry (codex) is selected.
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	writeExecutionAttemptForTest(t, runPaths, runID, "attempt_001", helperAgentDescriptor("helper"))

	runReviewCommand(t, app, "review", "plan", runID)
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Reviewer.Name != agents.BuiltinCodex {
		t.Fatalf("non-built-in executor should select the first registry entry, got: %#v", plan.Reviewer)
	}
}

func TestReviewPlanExplicitReviewers(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")

	runReviewCommand(t, app, "review", "plan", runID, "--reviewer", "codex")
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Reviewer.Name != "codex" || plan.Reviewer.Command != "" {
		t.Fatalf("codex reviewer mismatch: %#v", plan.Reviewer)
	}
	codexPromptArtifact := reviewerLensPromptArtifact("codex", reviewLenses[0])
	wouldRun := plan.Attempts[0].WouldRun
	// Codex reviewer runs over ACP with read-only sandbox mode; model from config.
	if wouldRun.Command != "npx" || strings.Join(wouldRun.Args, " ") != `-y @heurema/codex-acp@latest -c sandbox_mode="read-only" -c model="gpt-5"` {
		t.Fatalf("codex would_run mismatch: %#v", wouldRun)
	}
	assertCommandArgsDoNotContain(t, wouldRun.Args, codexPromptArtifact, runArtifactRepoRel(runID, codexPromptArtifact))

	runReviewCommand(t, app, "review", "plan", runID, "--reviewer", "claude")
	plan = readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	// Claude runs over ACP; Command is empty (no CLI path).
	if plan.Reviewer.Name != "claude" || plan.Reviewer.Command != "" {
		t.Fatalf("claude reviewer mismatch: %#v", plan.Reviewer)
	}
	wouldRun = plan.Attempts[0].WouldRun
	// Claude ACP: WouldRun carries the adapter command.
	if wouldRun.Command != "npx" || strings.Join(wouldRun.Args, " ") != "-y @agentclientprotocol/claude-agent-acp@latest" {
		t.Fatalf("claude would_run must carry ACP adapter command: %#v", wouldRun)
	}
}

func TestReviewPlanAppliesPanelEntryPinToCodex(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths,
		agentRegistryEntry{Name: "codex", Model: "gpt-5", Effort: "high"},
		agentRegistryEntry{Name: "claude", Model: "claude-opus-4-8"},
	)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID, "--reviewer", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review plan codex exited %d, stderr: %s", code, stderr.String())
	}
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	wantArgs := []string{"-y", "@heurema/codex-acp@latest", "-c", "sandbox_mode=\"read-only\"", "-c", "model=\"gpt-5\"", "-c", "model_reasoning_effort=high"}
	if !sameStringSlice(plan.Attempts[0].WouldRun.Args, wantArgs) {
		t.Fatalf("codex reviewer would_run args = %#v, want %#v", plan.Attempts[0].WouldRun.Args, wantArgs)
	}
	assertResolvedBlock(t, stdout.String(), "codex", "gpt-5", "high", "pinned")

	// The pin is per panel member: the other member carries only its own model
	// pin, not the codex entry's effort.
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"review", "plan", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review plan claude exited %d, stderr: %s", code, stderr.String())
	}
	plan = readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	// Claude runs over ACP; WouldRun carries the adapter command (model pin via env).
	if plan.Attempts[0].WouldRun.Command != "npx" || strings.Join(plan.Attempts[0].WouldRun.Args, " ") != "-y @agentclientprotocol/claude-agent-acp@latest" {
		t.Fatalf("claude reviewer would_run must carry ACP adapter command: %#v", plan.Attempts[0].WouldRun)
	}
	assertResolvedBlock(t, stdout.String(), "claude", "claude-opus-4-8", "inherit", "partial")
}

func TestReviewPlanAppliesPanelEntryPinToClaude(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review plan claude exited %d, stderr: %s", code, stderr.String())
	}
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	// Claude runs over ACP; model/effort pin goes via env vars in the adapter command.
	if plan.Attempts[0].WouldRun.Command != "npx" || strings.Join(plan.Attempts[0].WouldRun.Args, " ") != "-y @agentclientprotocol/claude-agent-acp@latest" {
		t.Fatalf("claude reviewer would_run must carry ACP adapter command: %#v", plan.Attempts[0].WouldRun)
	}
	assertResolvedBlock(t, stdout.String(), "claude", "claude-sonnet-4", "high", "pinned")
}

func TestReviewPlanUnsupportedReviewerFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID, "--reviewer", "missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review plan should fail for missing reviewer")
	}
	if got := stderr.String(); !strings.Contains(got, `unknown agent "missing": not registered in config agents`) {
		t.Fatalf("missing reviewer stderr mismatch:\n%s", got)
	}
}

func TestReviewPlanUnsupportedInputModeFails(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	registerTestAgents(t, paths, "bad-input")
	app.AgentRegistry = testAgentRegistry(agents.AgentDescriptor{
		Name:    testAgentEngine("bad-input"),
		Command: "bad-reviewer",
		Args:    []string{"dry-run"},
		Input:   "stdin",
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "plan", runID, "--reviewer", "bad-input"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review plan should fail for unsupported input mode")
	}
	if got := stderr.String(); !strings.Contains(got, "unsupported agent input mode: stdin") {
		t.Fatalf("unsupported input stderr mismatch:\n%s", got)
	}
}

func TestReviewPlanContextIncludesManualFindings(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "finding", "add", runID, "Manual review artifacts should stay append-only", "--category", "process", "--severity", "medium")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "Pending proposal should be visible to reviewer", true)

	runReviewCommand(t, app, "review", "plan", runID)
	context := mustReadFile(t, runPaths.ReviewContextMD)
	for _, want := range []string{
		"f_001",
		"Manual review artifacts should stay append-only",
		"status=open",
		"Existing proposals:",
		"p_001 severity=medium category=quality blocking=true status=pending",
		"Pending proposal should be visible to reviewer",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("reviewer context missing %q:\n%s", want, context)
		}
	}
}

func TestReviewPlanArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "finding", "add", runID, "portable reviewer context", "--file", "internal/app/review.go")

	runReviewCommand(t, app, "review", "plan", runID)
	contents := map[string]string{
		"review/reviewer-context.md":   mustReadFile(t, runPaths.ReviewContextMD),
		"review/reviewer-dry-run.json": mustReadFile(t, runPaths.ReviewDryRunJSON),
	}
	for _, lens := range reviewLenses {
		contents[reviewerLensPromptArtifact("claude", lens)] = mustReadFile(t, reviewerLensPromptPath(runPaths, "claude", lens))
	}
	for name, content := range contents {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestReviewPlanWritesLedgerEvent(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")

	runReviewCommand(t, app, "review", "plan", runID)
	events := strings.Join(readLines(t, paths.EventsJSONL), "\n")
	if !strings.Contains(events, "review_dry_run_prepared") || !strings.Contains(events, runID) {
		t.Fatalf("events missing review_dry_run_prepared:\n%s", events)
	}
}

func TestReviewPlanDoesNotMutateManualReviewArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "finding", "add", runID, "manual artifact should not change")

	beforeReview := mustReadFile(t, runPaths.ReviewJSON)
	beforeFindings := mustReadFile(t, runPaths.ReviewFindingsJSONL)
	beforeResolutions := mustReadFile(t, runPaths.ReviewResolutionsJSONL)
	beforeEvents := readLines(t, paths.EventsJSONL)

	runReviewCommand(t, app, "review", "plan", runID)

	if got := mustReadFile(t, runPaths.ReviewJSON); got != beforeReview {
		t.Fatalf("review plan mutated review.json")
	}
	if got := mustReadFile(t, runPaths.ReviewFindingsJSONL); got != beforeFindings {
		t.Fatalf("review plan mutated findings.jsonl")
	}
	if got := mustReadFile(t, runPaths.ReviewResolutionsJSONL); got != beforeResolutions {
		t.Fatalf("review plan mutated resolutions.jsonl")
	}
	afterEvents := readLines(t, paths.EventsJSONL)
	if len(afterEvents) != len(beforeEvents)+1 || !strings.Contains(afterEvents[len(afterEvents)-1], "review_dry_run_prepared") {
		t.Fatalf("review plan should only append its ledger event:\nbefore=%v\nafter=%v", beforeEvents, afterEvents)
	}
}

func TestReviewRunBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"review", "run", "run_x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("review run before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("review run before init stderr mismatch:\n%s", got)
	}
}

func TestReviewRunMissingRunReturnsError(t *testing.T) {
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
	code = app.Run([]string{"review", "run", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestReviewRunScaffoldsReviewOnGatedRun(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should still fail on the unapproved contract")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot run reviewer: contract is not approved") {
		t.Fatalf("unapproved contract stderr mismatch:\n%s", got)
	}
	// The review scaffold is created implicitly before the round runs.
	assertFile(t, runPaths.ReviewJSON)
	assertFile(t, runPaths.ReviewFindingsJSONL)
	assertFile(t, runPaths.ReviewResolutionsJSONL)
}

func TestReviewRunMissingGateReportFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedReviewWithoutGateReport(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should fail without gate report")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot run review: gate report not found") {
		t.Fatalf("missing gate stderr mismatch:\n%s", got)
	}
	if _, err := os.Stat(runPaths.ReviewAttemptsDir); err == nil {
		t.Fatalf("review run missing gate should not create attempts dir")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestReviewRunContractNotApprovedFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should fail without approved contract")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot run reviewer: contract is not approved") {
		t.Fatalf("unapproved contract stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.ReviewDryRunJSON)
}

func TestReviewRunUnsupportedReviewerFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should fail for missing reviewer")
	}
	if got := stderr.String(); !strings.Contains(got, `unknown agent "missing": not registered in config agents`) {
		t.Fatalf("missing reviewer stderr mismatch:\n%s", got)
	}
}

func TestReviewRunUnsupportedInputModeFails(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	registerTestAgents(t, paths, "bad-input")
	app.AgentRegistry = testAgentRegistry(agents.AgentDescriptor{
		Name:    testAgentEngine("bad-input"),
		Command: "bad-reviewer",
		Args:    []string{"run"},
		Input:   "stdin",
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "bad-input"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should fail for unsupported input mode")
	}
	if got := stderr.String(); !strings.Contains(got, "unsupported agent input mode: stdin") {
		t.Fatalf("unsupported input stderr mismatch:\n%s", got)
	}
}

func TestReviewRunStreamsLiveOutputToStderr(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
	}

	// The reviewer's stdout and stderr both stream live to the operator's stderr.
	if got := stderr.String(); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stdin_has_reviewer_prompt=true") || !strings.Contains(got, "reviewer-stderr-line") {
		t.Fatalf("live reviewer output missing from stderr:\n%s", got)
	}
	// Stdout stays the clean human summary; agent output never leaks there.
	out := stdout.String()
	if !strings.Contains(out, "Review run finished") {
		t.Fatalf("stdout missing human summary:\n%s", out)
	}
	if strings.Contains(out, "cwd_is_repo=") || strings.Contains(out, "reviewer-stderr-line") {
		t.Fatalf("agent output leaked into stdout:\n%s", out)
	}
}

func TestReviewRunWritesAttemptArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)

	beforeReview := mustReadFile(t, runPaths.ReviewJSON)
	beforeFindings := mustReadFile(t, runPaths.ReviewFindingsJSONL)
	beforeResolutions := mustReadFile(t, runPaths.ReviewResolutionsJSONL)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Review run finished") || !strings.Contains(got, "terminal reason: clean_round") {
		t.Fatalf("review run output mismatch:\n%s", got)
	}

	// Five concurrent lens attempts: the lens-to-attempt-ID mapping depends on
	// scheduling, so collect the lens per attempt and require the full fixed set.
	seenLenses := map[string]bool{}
	for index := 1; index <= len(reviewLenses); index++ {
		attemptID := fmt.Sprintf("reviewer_attempt_%03d", index)
		attemptPaths := reviewerAttemptPaths(runPaths, attemptID)
		assertFile(t, attemptPaths.RequestJSON)
		assertFile(t, attemptPaths.StdoutLog)
		assertFile(t, attemptPaths.StderrLog)
		assertFile(t, attemptPaths.ResultJSON)

		var request reviewerRequestDocument
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.RequestJSON)), &request))
		if request.Schema != reviewerRequestSchema || request.RunID != runID || request.AttemptID != attemptID {
			t.Fatalf("unexpected request: %#v", request)
		}
		// The request records the engine inferred from the helper entry's model.
		if request.Reviewer.Name != "claude" || request.Reviewer.Command != os.Args[0] || request.Reviewer.Input != agents.InputPromptFile {
			t.Fatalf("unexpected request reviewer: %#v", request.Reviewer)
		}
		if request.Lens == "" || seenLenses[request.Lens] {
			t.Fatalf("request lens missing or duplicated: %#v", request)
		}
		seenLenses[request.Lens] = true
		// ACP agents carry the prompt via Artifacts, not WouldRun.Stdin.
		if request.WouldRun.Stdin != "" {
			t.Fatalf("ACP would_run must not use stdin, got %q", request.WouldRun.Stdin)
		}
		wantPrompt := ".heurema/pactum/runs/" + runID + "/review/reviewer-prompt-helper-" + request.Lens + ".md"
		if got := strings.Join(request.WouldRun.Args, " "); strings.Contains(got, wantPrompt) {
			t.Fatalf("would_run args should not pass prompt path positionally: %#v", request.WouldRun.Args)
		}
		if request.Artifacts.ReviewerPrompt != "review/reviewer-prompt-helper-"+request.Lens+".md" || request.Artifacts.GateReport != gateReportArtifact {
			t.Fatalf("unexpected request artifacts: %#v", request.Artifacts)
		}

		var result reviewerResultDocument
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.ResultJSON)), &result))
		if result.Schema != reviewerResultSchema || result.Reviewer != "claude" || result.Lens != request.Lens || result.ExitCode != 0 || result.TimedOut {
			t.Fatalf("unexpected result: %#v", result)
		}
		if result.Stdout != "review/reviewer-attempts/"+attemptID+"/stdout.log" || result.Stderr != "review/reviewer-attempts/"+attemptID+"/stderr.log" {
			t.Fatalf("unexpected output artifact paths: %#v", result)
		}
		if got := mustReadFile(t, attemptPaths.StdoutLog); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stdin_has_reviewer_prompt=true") {
			t.Fatalf("stdout log mismatch:\n%s", got)
		}
		if got := mustReadFile(t, attemptPaths.StderrLog); !strings.Contains(got, "reviewer-stderr-line") {
			t.Fatalf("stderr log mismatch:\n%s", got)
		}
	}
	for _, lens := range reviewLenses {
		if !seenLenses[lens.Key] {
			t.Fatalf("lens %s missing from attempts: %#v", lens.Key, seenLenses)
		}
	}
	assertFile(t, runPaths.ReviewLastResultJSON)
	if got := mustReadFile(t, runPaths.ReviewJSON); got != beforeReview {
		t.Fatalf("review run mutated review.json")
	}
	if got := mustReadFile(t, runPaths.ReviewFindingsJSONL); got != beforeFindings {
		t.Fatalf("review run mutated findings.jsonl")
	}
	if got := mustReadFile(t, runPaths.ReviewResolutionsJSONL); got != beforeResolutions {
		t.Fatalf("review run mutated resolutions.jsonl")
	}

	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	startedIndex := indexOfEvent(eventTypes, "reviewer_attempt_started")
	finishedIndex := indexOfEvent(eventTypes, "reviewer_attempt_finished")
	if startedIndex == -1 || finishedIndex == -1 || startedIndex > finishedIndex {
		t.Fatalf("events missing ordered reviewer attempt lifecycle:\n%v", eventTypes)
	}
	if countEvents(eventTypes, "reviewer_attempt_started") != len(reviewLenses) || countEvents(eventTypes, "reviewer_attempt_finished") != len(reviewLenses) {
		t.Fatalf("review run should write one attempt lifecycle per lens:\n%v", eventTypes)
	}
}

func TestReviewRunNonZeroWritesArtifactsAndReturnsNonZero(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_REVIEWER_EXIT", "7")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "helper"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should return non-zero for reviewer failure")
	}
	if got := stderr.String(); !strings.Contains(got, "reviewer process exited with code 7") {
		t.Fatalf("non-zero stderr mismatch:\n%s", got)
	}

	attemptPaths := reviewerAttemptPaths(runPaths, "reviewer_attempt_001")
	assertFile(t, attemptPaths.ResultJSON)
	var result reviewerResultDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.ResultJSON)), &result))
	if result.ExitCode != 7 || result.TimedOut {
		t.Fatalf("unexpected failing result: %#v", result)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "reviewer_attempt_started") != len(reviewLenses) || countEvents(eventTypes, "reviewer_attempt_finished") != len(reviewLenses) {
		t.Fatalf("non-zero review run should write started and finished events per lens:\n%v", eventTypes)
	}
}

func TestReviewRunCreatesIncrementingAttempts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)

	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", "helper"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run %d exited %d, stderr: %s", i+1, code, stderr.String())
		}
	}
	// Each run spawns one attempt per lens; the second run continues numbering.
	for index := 1; index <= 2*len(reviewLenses); index++ {
		attemptPaths := reviewerAttemptPaths(runPaths, fmt.Sprintf("reviewer_attempt_%03d", index))
		assertFile(t, attemptPaths.RequestJSON)
		assertFile(t, attemptPaths.ResultJSON)
	}
}

func TestReviewRunStoresCrossReviewerAttempts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper-a", "helper-b")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)

	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper-a")
	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper-b")

	// The first run's lens attempts come first, the second run's after them.
	// Attempt artifacts record the engine, and both helper entries infer the
	// claude engine.
	for attemptID, wantReviewer := range map[string]string{
		"reviewer_attempt_001": "claude",
		fmt.Sprintf("reviewer_attempt_%03d", len(reviewLenses)+1): "claude",
	} {
		attemptPaths := reviewerAttemptPaths(runPaths, attemptID)
		assertFile(t, attemptPaths.ResultJSON)
		var result reviewerResultDocument
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.ResultJSON)), &result))
		if result.Reviewer != wantReviewer {
			t.Fatalf("%s reviewer = %q, want %q", attemptID, result.Reviewer, wantReviewer)
		}
	}
}

func TestReviewRunAutoBuildsReviewerPrompts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)
	_ = os.Remove(runPaths.ReviewContextMD)

	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper")
	assertFile(t, runPaths.ReviewContextMD)
	for _, lens := range reviewLenses {
		assertFile(t, reviewerLensPromptPath(runPaths, "helper", lens))
	}
}

func TestReviewRunRequestWouldRunMatchesDryRun(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")
	runReviewCommand(t, app, "review", "plan", runID, "--reviewer", "helper")
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)

	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)
	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper")

	planByLens := map[string]reviewerLensAttemptPlan{}
	for _, attempt := range plan.Attempts {
		planByLens[attempt.Lens] = attempt
	}
	for index := 1; index <= len(reviewLenses); index++ {
		var request reviewerRequestDocument
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, reviewerAttemptPaths(runPaths, fmt.Sprintf("reviewer_attempt_%03d", index)).RequestJSON)), &request))
		planned, ok := planByLens[request.Lens]
		if !ok {
			t.Fatalf("request lens %q missing from dry-run plan: %#v", request.Lens, plan.Attempts)
		}
		if request.WouldRun.Command != planned.WouldRun.Command || request.WouldRun.Stdin != planned.WouldRun.Stdin || !sameStringSlice(request.WouldRun.Args, planned.WouldRun.Args) {
			t.Fatalf("request would_run does not match dry-run:\nrequest=%#v\nplan=%#v", request.WouldRun, planned.WouldRun)
		}
	}
}

func TestReviewRunArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)

	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper")
	attemptPaths := reviewerAttemptPaths(runPaths, "reviewer_attempt_001")
	for name, content := range map[string]string{
		"review/request.json":              mustReadFile(t, attemptPaths.RequestJSON),
		"review/result.json":               mustReadFile(t, attemptPaths.ResultJSON),
		"review/reviewer-last-result.json": mustReadFile(t, runPaths.ReviewLastResultJSON),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestReviewRunDoesNotCreateFindingsFromReviewerOutput(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_REVIEWER_FINDING_TEXT", "1")
	beforeFindings := mustReadFile(t, runPaths.ReviewFindingsJSONL)

	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper")
	if got := mustReadFile(t, runPaths.ReviewFindingsJSONL); got != beforeFindings {
		t.Fatalf("review run should not create findings from stdout")
	}
	if got := mustReadFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_001").StdoutLog); !strings.Contains(got, "FINDING:") {
		t.Fatalf("helper stdout should contain finding-like text:\n%s", got)
	}
}

func TestReviewRunJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "helper", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run --json exited %d, stderr: %s", code, stderr.String())
	}
	var response struct {
		reviewLoopSummaryDocument
		Next []string `json:"next"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != reviewLoopSummarySchema || response.RunID != runID || response.Reviewer != "helper" {
		t.Fatalf("unexpected review run json: %#v", response)
	}
	if response.TerminalReason != "clean_round" || len(response.Rounds) != 1 {
		t.Fatalf("review run with a clean helper should converge in one round: %#v", response)
	}
	if len(response.Rounds[0].ReviewerAttempts) != len(reviewLenses) {
		t.Fatalf("round should record one attempt per lens: %#v", response.Rounds[0])
	}
	// The stage-derived next affordance is present; the full-pipeline value is
	// pinned by the lifecycle affordance tests.
	if response.Next == nil {
		t.Fatalf("review run json missing next affordance:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Review run finished") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestReviewRunPreconditionFailuresDoNotWriteAttemptEvents(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should fail on the unapproved contract")
	}
	afterEvents := readLines(t, paths.EventsJSONL)
	for _, forbidden := range []string{"reviewer_attempt_started", "reviewer_attempt_finished"} {
		if strings.Contains(strings.Join(afterEvents, "\n"), forbidden) {
			t.Fatalf("precondition failure should not write %s:\n%v", forbidden, afterEvents)
		}
	}
}

func TestReviewFixDryRunArtifactsUseWriteEnabledExecutorAndPrompt(t *testing.T) {
	for _, tc := range []struct {
		agent    string
		wantArgs string
		forbid   string
	}{
		{
			agent:    "codex",
			wantArgs: `-y @heurema/codex-acp@latest -c model="gpt-5" -c model_reasoning_effort=high`,
			forbid:   `sandbox_mode="read-only"`,
		},
		{
			agent:    "claude",
			wantArgs: "-y @agentclientprotocol/claude-agent-acp@latest",
			forbid:   `sandbox_mode="read-only"`,
		},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			root := t.TempDir()
			app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
			// The model decides the engine, so each case pins a model of the
			// matching family.
			model := "gpt-5"
			if tc.agent == "claude" {
				model = "claude-sonnet-4"
			}
			setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: tc.agent, Model: model, Effort: "high"})
			runReviewCommand(t, app, "review", "finding", "add", runID, "Fix prompt should include accepted review finding", "--file", "internal/app/review.go", "--line", "42", "--blocking", "--category", "correctness")

			var stdout bytes.Buffer
			context, ok, err := app.loadReviewContext(&stdout, runID)
			assertNoError(t, err)
			if !ok {
				t.Fatalf("expected review context")
			}
			prep, err := app.prepareReviewFixer(context, tc.agent)
			assertNoError(t, err)
			plan, err := ensureReviewFixDryRunArtifacts(prep, "2026-06-01T22:00:00Z")
			assertNoError(t, err)

			assertFile(t, runPaths.ReviewFixContextMD)
			assertFile(t, runPaths.ReviewFixPromptMD)
			assertFile(t, runPaths.ReviewFixDryRunJSON)
			if plan.Schema != reviewFixDryRunSchema || plan.RunID != runID || plan.Fixer.Name != tc.agent {
				t.Fatalf("unexpected review fix dry-run plan: %#v", plan)
			}
			if got := strings.Join(plan.WouldRun.Args, " "); got != tc.wantArgs {
				t.Fatalf("%s fixer args = %q, want %q", tc.agent, got, tc.wantArgs)
			}
			if strings.Contains(strings.Join(plan.WouldRun.Args, " "), tc.forbid) {
				t.Fatalf("fixer args should not contain reviewer-only flag %q: %#v", tc.forbid, plan.WouldRun.Args)
			}
			// ACP agents carry the prompt via Artifacts, not WouldRun.Stdin.
			if plan.WouldRun.Stdin != "" {
				t.Fatalf("review fix ACP would_run must not use stdin, got %q", plan.WouldRun.Stdin)
			}

			prompt := mustReadFile(t, runPaths.ReviewFixPromptMD)
			for _, want := range []string{
				"# Review Fix Prompt",
				"Goal: add deterministic prompt boundary",
				"Fix prompt should include accepted review finding",
				"Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):",
				"f_001 severity=medium category=correctness blocking=true status=open",
				"Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):",
				"Trace each finding to the relevant code before acting.",
				"For false positives, explain a concrete rebuttal instead of changing code.",
				"## Output shape",
				`"schema": "pactum.review_fix_outcomes.v1alpha1"`,
				"Include exactly one outcome entry for every blocking finding listed above with status open.",
			} {
				if !strings.Contains(prompt, want) {
					t.Fatalf("review fix prompt missing %q:\n%s", want, prompt)
				}
			}
			assertDoesNotContainRoot(t, "review/fix/fixer-prompt.md", prompt, root)
		})
	}
}

func TestReviewFixRegeneratesPromptWhenFindingsChange(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "finding", "add", runID, "first finding", "--category", "quality")

	var buf bytes.Buffer
	ctx, ok, err := app.loadReviewContext(&buf, runID)
	assertNoError(t, err)
	if !ok {
		t.Fatalf("expected review context")
	}
	prep1, err := app.prepareReviewFixer(ctx, "codex")
	assertNoError(t, err)
	if _, err := ensureReviewFixDryRunArtifacts(prep1, "2026-06-01T22:00:00Z"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	// Add a second finding and re-prepare. The fixer prompt inlines the findings,
	// so it must be regenerated rather than reusing the first attempt's stale prompt.
	runReviewCommand(t, app, "review", "finding", "add", runID, "second finding added later", "--category", "quality")
	ctx2, ok, err := app.loadReviewContext(&buf, runID)
	assertNoError(t, err)
	if !ok {
		t.Fatalf("expected review context")
	}
	prep2, err := app.prepareReviewFixer(ctx2, "codex")
	assertNoError(t, err)
	if _, err := ensureReviewFixDryRunArtifacts(prep2, "2026-06-01T22:05:00Z"); err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	prompt := mustReadFile(t, runPaths.ReviewFixPromptMD)
	if !strings.Contains(prompt, "second finding added later") {
		t.Fatalf("review fix prompt did not refresh with the new finding (stale reuse):\n%s", prompt)
	}
}

func TestReviewFixRunWritesAttemptArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperFixers(t, app, paths, "helper")
	runReviewCommand(t, app, "review", "finding", "add", runID, "valid fixer finding", "--blocking", "--category", "quality")
	t.Setenv("PACTUM_FIXER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_FIXER_EXPECTED_CWD", root)

	beforeReview := mustReadFile(t, runPaths.ReviewJSON)
	beforeFindings := mustReadFile(t, runPaths.ReviewFindingsJSONL)
	beforeResolutions := mustReadFile(t, runPaths.ReviewResolutionsJSONL)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review fix exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Review fix attempt finished") || !strings.Contains(got, "attempt_001") {
		t.Fatalf("review fix output mismatch:\n%s", got)
	} else {
		assertResolvedBlock(t, got, "helper", "claude-opus-4-8", "inherit", "partial")
	}

	attemptPaths := reviewFixAttemptPaths(runPaths, "attempt_001")
	assertFile(t, runPaths.ReviewFixContextMD)
	assertFile(t, runPaths.ReviewFixPromptMD)
	assertFile(t, runPaths.ReviewFixDryRunJSON)
	assertFile(t, attemptPaths.RequestJSON)
	assertFile(t, attemptPaths.StdoutLog)
	assertFile(t, attemptPaths.StderrLog)
	assertFile(t, attemptPaths.ResultJSON)
	assertFile(t, runPaths.ReviewFixLastResultJSON)

	var request reviewFixRequestDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.RequestJSON)), &request))
	if request.Schema != reviewFixRequestSchema || request.RunID != runID || request.AttemptID != "attempt_001" {
		t.Fatalf("unexpected request: %#v", request)
	}
	// The request records the engine inferred from the helper entry's model.
	if request.Fixer.Name != "claude" || request.Fixer.Command != os.Args[0] || request.Fixer.Input != agents.InputPromptFile {
		t.Fatalf("unexpected request fixer: %#v", request.Fixer)
	}
	// ACP agents carry the prompt via Artifacts, not WouldRun.Stdin.
	if request.WouldRun.Stdin != "" {
		t.Fatalf("ACP would_run must not use stdin, got %q", request.WouldRun.Stdin)
	}
	if request.Artifacts.FixerPrompt != reviewFixPromptArtifact || request.Artifacts.Findings != reviewFindingsArtifact {
		t.Fatalf("unexpected request artifacts: %#v", request.Artifacts)
	}

	var result reviewFixResultDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.ResultJSON)), &result))
	if result.Schema != reviewFixResultSchema || result.Fixer != "claude" || result.ExitCode != 0 || result.TimedOut {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Stdout != "review/fix/attempts/attempt_001/stdout.log" || result.Stderr != "review/fix/attempts/attempt_001/stderr.log" {
		t.Fatalf("unexpected output artifact paths: %#v", result)
	}
	if got := mustReadFile(t, runPaths.ReviewFixLastResultJSON); got != mustReadFile(t, attemptPaths.ResultJSON) {
		t.Fatalf("review fix last-result.json should copy result.json")
	}
	if got := mustReadFile(t, attemptPaths.StdoutLog); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stdin_has_review_fix_prompt=true") {
		t.Fatalf("stdout log mismatch:\n%s", got)
	}
	if got := mustReadFile(t, attemptPaths.StderrLog); !strings.Contains(got, "fixer-stderr-line") {
		t.Fatalf("stderr log mismatch:\n%s", got)
	}
	if got := mustReadFile(t, runPaths.ReviewJSON); got != beforeReview {
		t.Fatalf("review fix mutated review.json")
	}
	if got := mustReadFile(t, runPaths.ReviewFindingsJSONL); got != beforeFindings {
		t.Fatalf("review fix mutated findings.jsonl")
	}
	if got := mustReadFile(t, runPaths.ReviewResolutionsJSONL); got != beforeResolutions {
		t.Fatalf("review fix mutated resolutions.jsonl")
	}

	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	startedIndex := indexOfEvent(eventTypes, "review_fix_attempt_started")
	finishedIndex := indexOfEvent(eventTypes, "review_fix_attempt_finished")
	if startedIndex == -1 || finishedIndex == -1 || startedIndex > finishedIndex {
		t.Fatalf("events missing ordered review fix attempt lifecycle:\n%v", eventTypes)
	}
}

func TestReviewFixStreamsLiveOutputToStderr(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperFixers(t, app, paths, "helper")
	runReviewCommand(t, app, "review", "finding", "add", runID, "valid fixer finding", "--blocking", "--category", "quality")
	t.Setenv("PACTUM_FIXER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_FIXER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review fix exited %d, stderr: %s", code, stderr.String())
	}

	// The fixer's stdout and stderr both stream live to the operator's stderr.
	if got := stderr.String(); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stdin_has_review_fix_prompt=true") || !strings.Contains(got, "fixer-stderr-line") {
		t.Fatalf("live fixer output missing from stderr:\n%s", got)
	}
	// Stdout stays the clean human summary; agent output never leaks there.
	out := stdout.String()
	if !strings.Contains(out, "Review fix attempt finished") {
		t.Fatalf("stdout missing human summary:\n%s", out)
	}
	if strings.Contains(out, "cwd_is_repo=") || strings.Contains(out, "fixer-stderr-line") {
		t.Fatalf("agent output leaked into stdout:\n%s", out)
	}
}

func TestReviewFixResolvedBlockShowsExecutorModel(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "codex", Model: "gpt-5", Effort: "high"})
	runReviewCommand(t, app, "review", "finding", "add", runID, "model pinning should be visible")
	t.Setenv("PACTUM_FIXER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_FIXER_EXPECTED_CWD", root)
	app.AgentRegistry = testAgentRegistry(agents.AgentDescriptor{
		Name:    agents.BuiltinCodex,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestReviewFixerHelperProcess", "--"},
		Input:   agents.InputPromptFile,
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review fix exited %d, stderr: %s", code, stderr.String())
	}
	assertResolvedBlock(t, stdout.String(), "codex", "gpt-5", "high", "pinned")

	var request reviewFixRequestDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").RequestJSON)), &request))
	args := strings.Join(request.WouldRun.Args, " ")
	for _, want := range []string{"@heurema/codex-acp@latest", `model="gpt-5"`, "model_reasoning_effort=high"} {
		if !strings.Contains(args, want) {
			t.Fatalf("review fix would_run args missing %q: %#v", want, request.WouldRun.Args)
		}
	}
	if strings.Contains(args, `sandbox_mode="read-only"`) {
		t.Fatalf("review fix would_run should not use read-only reviewer sandbox mode: %#v", request.WouldRun.Args)
	}
}

func TestReviewFixJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperFixers(t, app, paths, "helper")
	runReviewCommand(t, app, "review", "finding", "add", runID, "json output finding")
	t.Setenv("PACTUM_FIXER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_FIXER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "run", runID, "--agent", "helper", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review fix --json exited %d, stderr: %s", code, stderr.String())
	}
	var result reviewFixResultDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &result))
	// The result records the engine inferred from the helper entry's model.
	if result.AttemptID != "attempt_001" || result.Fixer != "claude" || result.ExitCode != 0 {
		t.Fatalf("unexpected review fix json: %#v", result)
	}
	if strings.Contains(stdout.String(), "Review fix attempt finished") || strings.Contains(stdout.String(), "Resolved:") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestReviewApplyFixOutcomesResolvesFixedAndRebuttedOnly(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "finding", "add", runID, "fixed finding", "--blocking", "--category", "quality")
	runReviewCommand(t, app, "review", "finding", "add", runID, "rebutted finding", "--blocking", "--category", "quality")
	runReviewCommand(t, app, "review", "finding", "add", runID, "blocked finding", "--blocking", "--category", "quality")
	writeReviewFixAttemptForTest(t, runPaths, runID, "attempt_001", reviewFixStructuredOutput([]map[string]any{
		{"finding_id": "f_001", "outcome": "fixed", "note": "changed internal/app/review.go"},
		{"finding_id": "f_002", "outcome": "rebutted", "note": "not applicable in " + root},
		{"finding_id": "f_003", "outcome": "blocked", "note": "needs product decision"},
	}), true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "apply", runID, "attempt_001", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review apply-fix-outcomes exited %d, stderr: %s", code, stderr.String())
	}
	var response reviewApplyFixOutcomesResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Fixed != 1 || response.Rebutted != 1 || response.Blocked != 1 || len(response.Warnings) != 0 {
		t.Fatalf("unexpected apply response: %#v", response)
	}

	resolutions := readReviewResolutions(t, runPaths.ReviewResolutionsJSONL)
	if len(resolutions) != 2 || resolutions[0].Outcome != "fixed" || resolutions[1].Outcome != "rebutted" || resolutions[0].Source != "review_fix" || resolutions[1].Source != "review_fix" {
		t.Fatalf("unexpected resolutions: %#v", resolutions)
	}
	assertDoesNotContainRoot(t, "review/resolutions.jsonl", mustReadFile(t, runPaths.ReviewResolutionsJSONL), root)
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Summary.Open != 1 || review.Summary.Resolved != 2 || review.Summary.BlockingOpen != 1 {
		t.Fatalf("review summary should reflect applied outcomes: %#v", review.Summary)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_fix_outcomes_applied") == -1 {
		t.Fatalf("events missing review_fix_outcomes_applied:\n%v", eventTypes)
	}
}

func TestReviewApplyFixOutcomesMissingOrMalformedBlockWarnsWithoutWriting(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "finding", "add", runID, "open finding", "--blocking", "--category", "quality")
	writeReviewFixAttemptForTest(t, runPaths, runID, "attempt_001", "plain prose only\n", true)
	writeReviewFixAttemptForTest(t, runPaths, runID, "attempt_002", "```json\n{malformed\n```\n", true)

	for _, attemptID := range []string{"attempt_001", "attempt_002"} {
		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "fix", "apply", runID, attemptID, "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review apply-fix-outcomes %s exited %d, stderr: %s", attemptID, code, stderr.String())
		}
		var response reviewApplyFixOutcomesResponse
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
		if response.Fixed != 0 || response.Rebutted != 0 || response.Blocked != 0 || len(response.Warnings) == 0 {
			t.Fatalf("expected warning-only response for %s: %#v", attemptID, response)
		}
	}
	if got := mustReadFile(t, runPaths.ReviewResolutionsJSONL); got != "" {
		t.Fatalf("warning-only apply should not append resolutions:\n%s", got)
	}
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Summary.Open != 1 || review.Summary.Resolved != 0 {
		t.Fatalf("warning-only apply should not mutate review summary: %#v", review.Summary)
	}
}

func TestReviewApplyFixOutcomesSkipsUnknownAndAlreadyResolvedFindings(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "finding", "add", runID, "already fixed", "--blocking", "--category", "quality")
	runReviewCommand(t, app, "review", "finding", "resolve", runID, "f_001", "--note", "manual")
	writeReviewFixAttemptForTest(t, runPaths, runID, "attempt_001", reviewFixStructuredOutput([]map[string]any{
		{"finding_id": "f_001", "outcome": "fixed", "note": "duplicate"},
		{"finding_id": "f_404", "outcome": "fixed", "note": "unknown"},
	}), true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "apply", runID, "attempt_001", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review apply-fix-outcomes exited %d, stderr: %s", code, stderr.String())
	}
	var response reviewApplyFixOutcomesResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	warnings := strings.Join(response.Warnings, "\n")
	for _, want := range []string{"finding already resolved: f_001", "finding not found: f_404"} {
		if !strings.Contains(warnings, want) {
			t.Fatalf("warnings missing %q: %#v", want, response.Warnings)
		}
	}
	resolutions := readReviewResolutions(t, runPaths.ReviewResolutionsJSONL)
	if len(resolutions) != 1 || resolutions[0].Source != "manual" || resolutions[0].Outcome != "" {
		t.Fatalf("skipped outcomes should not append resolutions: %#v", resolutions)
	}
}

func TestReviewProposeFindingsBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"review", "proposal", "collect", "run_x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("review propose-findings before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("review propose-findings before init stderr mismatch:\n%s", got)
	}
}

func TestReviewProposeFindingsMissingRunReturnsError(t *testing.T) {
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
	code = app.Run([]string{"review", "proposal", "collect", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review propose-findings missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestReviewProposeFindingsWithNoReviewerAttemptsFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "proposal", "collect", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review propose-findings should fail without attempts")
	}
	if got := stderr.String(); !strings.Contains(got, "no completed reviewer attempts found") {
		t.Fatalf("no attempts stderr mismatch:\n%s", got)
	}
}

func TestReviewProposeFindingsWithNoStructuredBlockDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", "plain prose only\n", true)
	beforeEvents := readLines(t, paths.EventsJSONL)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "proposal", "collect", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review propose-findings exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "No structured reviewer finding proposals found.") {
		t.Fatalf("no proposal output mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.ReviewProposalsJSONL)
	afterEvents := readLines(t, paths.EventsJSONL)
	if len(afterEvents) != len(beforeEvents) || indexOfEvent(ledgerEventTypes(t, paths.EventsJSONL), "review_findings_proposed") != -1 {
		t.Fatalf("no structured block should not append proposal event:\nbefore=%v\nafter=%v", beforeEvents, afterEvents)
	}
}

func TestReviewProposeFindingsParsesValidStructuredBlock(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", reviewerStructuredOutput([]map[string]any{
		{
			"message":  "review output should stay behind proposal boundary",
			"severity": "medium",
			"category": "quality",
			"file":     "internal/app/review.go",
			"line":     42,
			"blocking": true,
			"evidence": "stdout proposal block",
		},
		{
			"message":  "nonblocking process concern",
			"severity": "low",
			"category": "process",
		},
	}), true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "proposal", "collect", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review propose-findings exited %d, stderr: %s", code, stderr.String())
	}
	proposals := readReviewProposals(t, runPaths.ReviewProposalsJSONL)
	if len(proposals) != 2 || proposals[0].ID != "p_001" || proposals[1].ID != "p_002" || !proposals[0].Blocking {
		t.Fatalf("unexpected proposals: %#v", proposals)
	}
	if got := stdout.String(); !strings.Contains(got, "Review finding proposals created") || !strings.Contains(got, "created: 2") {
		t.Fatalf("proposal output mismatch:\n%s", got)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_findings_proposed") == -1 {
		t.Fatalf("events missing review_findings_proposed:\n%v", eventTypes)
	}
}

func TestReviewProposeFindingsSkipsInvalidProposals(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", "```json\n{malformed\n```\n"+reviewerStructuredOutput([]map[string]any{
		{
			"message":  "valid proposal",
			"severity": "high",
			"category": "correctness",
			"file":     "internal/app/review.go",
		},
		{
			"message":  "invalid severity",
			"severity": "urgent",
			"category": "quality",
		},
		{
			"message":  "absolute path",
			"severity": "medium",
			"category": "quality",
			"file":     filepath.Join(root, "internal/app/review.go"),
		},
	}), true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "proposal", "collect", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review propose-findings --json exited %d, stderr: %s", code, stderr.String())
	}
	var response reviewProposeFindingsResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if len(response.Created) != 1 || response.Created[0].Message != "valid proposal" {
		t.Fatalf("unexpected created proposals: %#v", response.Created)
	}
	warnings := strings.Join(response.Warnings, "\n")
	for _, want := range []string{"invalid JSON", "severity must be", "file must be repo-relative"} {
		if !strings.Contains(warnings, want) {
			t.Fatalf("warnings missing %q:\n%v", want, response.Warnings)
		}
	}
}

// TestReviewProposeFindingsRequiredFieldContract pins the required-field
// rejection contract: message, severity, and category are required; absence
// of any one rejects the finding with a warning. The optional fields (file,
// line, blocking, confidence, evidence) are not required.
func TestReviewProposeFindingsRequiredFieldContract(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")

	// (a) All required fields present → accepted.
	// (b) Missing message → rejected with warning.
	// (c) Missing severity → rejected with warning.
	// (d) Missing category → rejected with warning.
	// (e) Optional fields absent → still accepted (only required fields matter).
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", reviewerStructuredOutput([]map[string]any{
		// (a) valid — all required fields
		{"message": "valid finding", "severity": "medium", "category": "quality"},
		// (b) missing message
		{"severity": "medium", "category": "quality"},
		// (c) missing severity
		{"message": "missing severity", "category": "quality"},
		// (d) missing category
		{"message": "missing category", "severity": "medium"},
		// (e) optional fields absent — must still be accepted
		{"message": "no optional fields", "severity": "low", "category": "correctness"},
		// (e) optional fields present — also accepted
		{
			"message":    "all optional fields",
			"severity":   "high",
			"category":   "correctness",
			"file":       "internal/app/review.go",
			"line":       10,
			"blocking":   true,
			"confidence": "high",
			"evidence":   "some evidence",
		},
	}), true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "proposal", "collect", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review propose-findings --json exited %d, stderr: %s", code, stderr.String())
	}
	var response reviewProposeFindingsResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))

	// Three findings accepted: (a), (e-no-optional), (e-all-optional).
	if len(response.Created) != 3 {
		t.Fatalf("expected 3 accepted proposals (valid + 2 optional-field variants), got %d: %#v", len(response.Created), response.Created)
	}
	messages := make([]string, len(response.Created))
	for i, p := range response.Created {
		messages[i] = p.Message
	}
	for _, want := range []string{"valid finding", "no optional fields", "all optional fields"} {
		found := false
		for _, m := range messages {
			if m == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected accepted proposal %q in %v", want, messages)
		}
	}

	// Three findings rejected (b, c, d) — each must produce a warning.
	warnings := strings.Join(response.Warnings, "\n")
	for _, want := range []string{"message is required", "severity must be", "category must be"} {
		if !strings.Contains(warnings, want) {
			t.Fatalf("warnings missing %q:\n%v", want, response.Warnings)
		}
	}
}

func TestReviewProposeFindingsUsesExplicitAttemptID(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", reviewerStructuredOutput([]map[string]any{
		{"message": "first attempt", "severity": "medium", "category": "quality"},
	}), true)
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_002", reviewerStructuredOutput([]map[string]any{
		{"message": "second attempt", "severity": "medium", "category": "quality"},
	}), true)

	runReviewCommand(t, app, "review", "proposal", "collect", runID, "reviewer_attempt_001")
	proposals := readReviewProposals(t, runPaths.ReviewProposalsJSONL)
	if len(proposals) != 1 || proposals[0].ReviewerAttemptID != "reviewer_attempt_001" || proposals[0].Message != "first attempt" {
		t.Fatalf("explicit attempt not used: %#v", proposals)
	}
}

func TestReviewProposeFindingsUsesLatestCompletedAttempt(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", reviewerStructuredOutput([]map[string]any{
		{"message": "completed attempt", "severity": "medium", "category": "quality"},
	}), true)
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_002", reviewerStructuredOutput([]map[string]any{
		{"message": "incomplete attempt", "severity": "medium", "category": "quality"},
	}), false)

	runReviewCommand(t, app, "review", "proposal", "collect", runID)
	proposals := readReviewProposals(t, runPaths.ReviewProposalsJSONL)
	if len(proposals) != 1 || proposals[0].ReviewerAttemptID != "reviewer_attempt_001" || proposals[0].Message != "completed attempt" {
		t.Fatalf("latest completed attempt not used: %#v", proposals)
	}
}

func TestReviewAcceptProposalCreatesFinding(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "accepted proposal", true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "proposal", "accept", runID, "p_001"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review accept-proposal exited %d, stderr: %s", code, stderr.String())
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || findings[0].ID != "f_001" || findings[0].Source != "reviewer_proposal" || !findings[0].Blocking {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 1 || decisions[0].ID != "pd_001" || decisions[0].Decision != "accepted" || decisions[0].FindingID != "f_001" || decisions[0].DecidedBy != "manual" {
		t.Fatalf("unexpected decisions: %#v", decisions)
	}
	if decisions[0].Source != "manual" {
		t.Fatalf("CLI accept must record source manual: %#v", decisions[0])
	}
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Status != "changes_requested" || review.Summary.Findings != 1 || review.Summary.BlockingOpen != 1 {
		t.Fatalf("unexpected review after accept: %#v", review)
	}
	if got := stdout.String(); !strings.Contains(got, "Review proposal accepted") || !strings.Contains(got, "id: f_001") {
		t.Fatalf("accept output mismatch:\n%s", got)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	for _, want := range []string{"review_proposal_accepted", "review_finding_added"} {
		if indexOfEvent(eventTypes, want) == -1 {
			t.Fatalf("events missing %s:\n%v", want, eventTypes)
		}
	}
}

func TestReviewAcceptProposalDoesNotCopyEvidenceToFinding(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	// appendReviewProposalForTest records the proposal with Evidence "fixture
	// evidence". The message deliberately omits the word "evidence" so the raw
	// findings.jsonl check below only trips on a real evidence field leak.
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "blocking issue", true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "proposal", "accept", runID, "p_001", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review accept-proposal exited %d, stderr: %s", code, stderr.String())
	}

	// Evidence belongs to proposals, not accepted findings: the finding on disk
	// must not carry it.
	if got := mustReadFile(t, runPaths.ReviewFindingsJSONL); strings.Contains(got, `"evidence"`) {
		t.Fatalf("findings.jsonl must not contain an evidence field:\n%s", got)
	}
	if findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL); len(findings) != 1 {
		t.Fatalf("expected exactly one accepted finding, got %#v", findings)
	}

	// The accept-proposal JSON response carries both the proposal (which keeps
	// its evidence) and the new finding (which must not).
	var response struct {
		Proposal map[string]any `json:"proposal"`
		Finding  map[string]any `json:"finding"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if _, ok := response.Finding["evidence"]; ok {
		t.Fatalf("accepted finding must not include evidence: %#v", response.Finding)
	}
	if got, _ := response.Proposal["evidence"].(string); got != "fixture evidence" {
		t.Fatalf("proposal should retain its evidence, got %q in %#v", got, response.Proposal)
	}
}

func TestReviewRejectProposalRecordsDecisionOnly(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "reject proposal", false)
	beforeReview := mustReadFile(t, runPaths.ReviewJSON)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "proposal", "reject", runID, "p_001", "--reason", "not relevant"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review reject-proposal exited %d, stderr: %s", code, stderr.String())
	}
	if findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL); len(findings) != 0 {
		t.Fatalf("reject should not create findings: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 1 || decisions[0].Decision != "rejected" || decisions[0].Reason != "not relevant" || decisions[0].DecidedBy != "manual" {
		t.Fatalf("unexpected decisions: %#v", decisions)
	}
	if got := mustReadFile(t, runPaths.ReviewJSON); got != beforeReview {
		t.Fatalf("reject should not mutate review.json")
	}
	if got := stdout.String(); !strings.Contains(got, "Review proposal rejected") || !strings.Contains(got, "not relevant") {
		t.Fatalf("reject output mismatch:\n%s", got)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_proposal_rejected") == -1 {
		t.Fatalf("events missing review_proposal_rejected:\n%v", eventTypes)
	}
}

// TestReviewProposalDecisionsRecordExplicitBy covers the uniform --by rule on
// proposal decisions: whitespace is trimmed, blank and exact-empty values fall
// back to manual, and repo-root paths are sanitized before decided_by is
// persisted.
func TestReviewProposalDecisionsRecordExplicitBy(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "accept with principal", false)
	appendReviewProposalForTest(t, runPaths, runID, "p_002", "reject with principal", false)
	appendReviewProposalForTest(t, runPaths, runID, "p_003", "accept with blank principal", false)
	appendReviewProposalForTest(t, runPaths, runID, "p_004", "accept with empty principal", false)

	runReviewCommand(t, app, "review", "proposal", "accept", runID, "p_001", "--by", "  alice  ")
	runReviewCommand(t, app, "review", "proposal", "reject", runID, "p_002", "--reason", "duplicate", "--by", root+"/agent")
	runReviewCommand(t, app, "review", "proposal", "accept", runID, "p_003", "--by", "   ")
	runReviewCommand(t, app, "review", "proposal", "accept", runID, "p_004", "--by", "")

	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 4 || decisions[0].DecidedBy != "alice" || decisions[2].DecidedBy != "manual" || decisions[3].DecidedBy != "manual" {
		t.Fatalf("unexpected decision principals: %#v", decisions)
	}
	if decisions[1].DecidedBy == "" || strings.Contains(decisions[1].DecidedBy, root) {
		t.Fatalf("reject decided_by not sanitized: %#v", decisions[1])
	}
	assertDoesNotContainRoot(t, "review/proposal-decisions.jsonl", mustReadFile(t, runPaths.ReviewProposalDecisionsJSONL), root)
}

func TestReviewProposalCannotBeDecidedTwice(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "decide once", false)
	runReviewCommand(t, app, "review", "proposal", "accept", runID, "p_001")

	for _, args := range [][]string{
		{"review", "proposal", "accept", runID, "p_001"},
		{"review", "proposal", "reject", runID, "p_001"},
	} {
		var stdout, stderr bytes.Buffer
		code := app.Run(args, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("%v should fail after proposal is decided", args)
		}
		if got := stderr.String(); !strings.Contains(got, "review proposal already decided: p_001") {
			t.Fatalf("already decided stderr mismatch for %v:\n%s", args, got)
		}
	}
}

func TestReviewProposalLatestDecisionWinsForDisplay(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "defensive display", false)
	appendProposalDecisionForTest(t, runPaths, runID, "pd_001", "p_001", "rejected", "")
	appendProposalDecisionForTest(t, runPaths, runID, "pd_002", "p_001", "accepted", "f_001")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "status", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review status --json exited %d, stderr: %s", code, stderr.String())
	}
	var state reviewStateResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &state))
	if state.ProposalSummary.Accepted != 1 || state.ProposalSummary.Rejected != 0 || state.Proposals[0].Status != "accepted" {
		t.Fatalf("latest decision should win: %#v", state)
	}
}

func TestReviewAcceptProposalAfterApprovedReviewResetsApproval(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "approve", runID, "--by", "manual")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "new blocking proposal", true)

	runReviewCommand(t, app, "review", "proposal", "accept", runID, "p_001")
	review := readReviewDoc(t, runPaths.ReviewJSON)
	if review.Status != "changes_requested" || review.Approval.ApprovedAt != nil || review.Approval.ApprovedBy != nil {
		t.Fatalf("approval should reset after accepted proposal: %#v", review)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "review_approval_reset") == -1 {
		t.Fatalf("events missing review_approval_reset:\n%v", eventTypes)
	}
}

func TestReviewStatusIncludesProposalSummary(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "pending proposal", false)
	appendReviewProposalForTest(t, runPaths, runID, "p_002", "accepted proposal", false)
	appendReviewProposalForTest(t, runPaths, runID, "p_003", "rejected proposal", false)
	appendProposalDecisionForTest(t, runPaths, runID, "pd_001", "p_002", "accepted", "f_001")
	appendProposalDecisionForTest(t, runPaths, runID, "pd_002", "p_003", "rejected", "")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "status", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review status exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"Proposals:", "pending: 1", "accepted: 1", "rejected: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status missing %q:\n%s", want, got)
		}
	}
}

func TestReviewShowIncludesPendingProposals(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "pending proposal", true)
	appendReviewProposalForTest(t, runPaths, runID, "p_002", "accepted proposal", false)
	appendProposalDecisionForTest(t, runPaths, runID, "pd_001", "p_002", "accepted", "f_001")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review show exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "Proposals:") || !strings.Contains(got, "p_001 [medium] [blocking] quality: pending proposal") {
		t.Fatalf("show should include pending proposal:\n%s", got)
	}
	if strings.Contains(got, "accepted proposal") {
		t.Fatalf("show should omit accepted proposal from pending list:\n%s", got)
	}
}

func TestReviewJSONOutputIncludesProposalsAndDecisions(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "json proposal", false)
	appendProposalDecisionForTest(t, runPaths, runID, "pd_001", "p_001", "rejected", "")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review show --json exited %d, stderr: %s", code, stderr.String())
	}
	var state reviewStateResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &state))
	if len(state.Proposals) != 1 || len(state.ProposalDecisions) != 1 || state.ProposalSummary.Rejected != 1 {
		t.Fatalf("json missing proposal records: %#v", state)
	}
}

func TestReviewProposalArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", reviewerStructuredOutput([]map[string]any{
		{
			"message":  "path should be sanitized from " + root,
			"severity": "medium",
			"category": "quality",
			"file":     "internal/app/review.go",
			"evidence": "observed in " + root,
		},
	}), true)

	runReviewCommand(t, app, "review", "proposal", "collect", runID)
	runReviewCommand(t, app, "review", "proposal", "accept", runID, "p_001")
	for name, content := range map[string]string{
		"review/proposals.jsonl":          mustReadFile(t, runPaths.ReviewProposalsJSONL),
		"review/proposal-decisions.jsonl": mustReadFile(t, runPaths.ReviewProposalDecisionsJSONL),
		"review/findings.jsonl":           mustReadFile(t, runPaths.ReviewFindingsJSONL),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestReviewPromptIncludesStructuredProposalContract(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")

	runReviewCommand(t, app, "review", "plan", runID)
	prompt := mustReadFile(t, reviewerLensPromptPath(runPaths, "claude", reviewLenses[0]))
	for _, want := range []string{
		"## Required structured output",
		`"schema": "pactum.reviewer_findings.v1alpha1"`,
		// Mandatory-block language.
		"You MUST emit exactly one fenced JSON block",
		"mandatory",
		// Prose is supplemental only and ignored by the parser.
		"Prose commentary is supplemental only; the parser ignores it.",
		// Worked clean example with findings: [].
		`"findings": []`,
		"Clean example (no findings):",
		// Existing contract assertions.
		"Style or formatting preferences.",
		"Read the actual file and surrounding context before proposing a finding.",
		"Check whether the issue is already mitigated or already represented in existing findings/proposals.",
		"Important: Pactum does not trust this output automatically. A human must accept proposals.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("reviewer prompt missing %q:\n%s", want, prompt)
		}
	}
	// The introductory line must not describe the block as optional.
	for _, gone := range []string{
		"may parse optional structured proposal blocks",
	} {
		if strings.Contains(prompt, gone) {
			t.Fatalf("reviewer prompt still contains optional-block language %q:\n%s", gone, prompt)
		}
	}
}

// TestReviewerPromptIncludesReviewMethodology pins the shared hardened
// sections: every lens prompt carries the identical high-signal contract,
// verify-then-report pass, pre-existing policy, output ordering, and
// confidence schema.
func TestReviewerPromptIncludesReviewMethodology(t *testing.T) {
	for _, lens := range reviewLenses {
		prompt := renderReviewerPrompt("run_x", lens)
		for _, want := range []string{
			"## High-signal contract",
			"If you are not certain an issue is real, do not flag it. False positives erode trust and waste reviewer time.",
			"Report problems only. No positive observations, no praise.",
			"- Do NOT flag:",
			"Style or formatting preferences.",
			"Anything the contract's validation commands already catch",
			"Input-dependent hypotheticals without a concrete failure path.",
			"Subjective redesign suggestions.",
			"## Review lens",
			"## Verify before reporting",
			"Read the actual code at the file and line, plus 20-30 surrounding lines.",
			"Classify the candidate CONFIRMED or FALSE POSITIVE.",
			"Report only CONFIRMED findings. Discard FALSE POSITIVE candidates.",
			"## Pre-existing issues",
			"report them as non-blocking findings",
			"Never mark a pre-existing issue blocking.",
			"## Output ordering",
			"Findings first, ordered by severity, each with file and line.",
			"If there are no findings, say so explicitly and name residual risks or testing gaps.",
			`"confidence": "high",`,
			"- Use confidence: high, medium, low.",
			"- A missing confidence defaults to medium.",
			"blocking=true for findings introduced by this change",
		} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("reviewer prompt (%s) missing %q:\n%s", lens.Key, want, prompt)
			}
		}
		for _, gone := range []string{
			"If uncertain, recommend a blocking manual finding.",
			"If uncertain, set blocking=true and explain uncertainty in evidence.",
			"If uncertain, propose a blocking finding that asks for clarification.",
		} {
			if strings.Contains(prompt, gone) {
				t.Fatalf("reviewer prompt (%s) still contains %q, which contradicts certain-or-silent:\n%s", lens.Key, gone, prompt)
			}
		}
	}
}

// TestReviewerPromptIsLensFocused pins the lens fan-out prompt contract: each
// lens prompt carries only its own checklist heading plus the panel focus
// note; the other four lens headings are absent.
func TestReviewerPromptIsLensFocused(t *testing.T) {
	checklistSamples := map[string]string{
		"correctness":      "Logic errors: off-by-one, wrong operators, inverted conditions.",
		"implementation":   "Does the diff achieve the contract goal?",
		"tests":            "Fake tests: always-pass tests, hardcoded-value checks",
		"over_engineering": "Dual implementations where the old path has no callers.",
		"docs":             "Internal-only changes need no documentation; do not flag them.",
	}
	for _, lens := range reviewLenses {
		prompt := renderReviewerPrompt("run_x", lens)
		focusNote := "You are the " + lens.Focus + " reviewer; other lenses are covered by other reviewers running in parallel — report only findings within your lens; do not silently expand scope."
		if !strings.Contains(prompt, focusNote) {
			t.Fatalf("reviewer prompt (%s) missing focus note:\n%s", lens.Key, prompt)
		}
		if !strings.Contains(prompt, "### "+lens.Heading) {
			t.Fatalf("reviewer prompt (%s) missing its lens heading:\n%s", lens.Key, prompt)
		}
		if sample := checklistSamples[lens.Key]; !strings.Contains(prompt, sample) {
			t.Fatalf("reviewer prompt (%s) missing checklist line %q:\n%s", lens.Key, sample, prompt)
		}
		for _, other := range reviewLenses {
			if other.Key == lens.Key {
				continue
			}
			if strings.Contains(prompt, "### "+other.Heading) {
				t.Fatalf("reviewer prompt (%s) leaks lens heading %q:\n%s", lens.Key, other.Heading, prompt)
			}
		}
	}
}

// TestReviewerContextAlignsWithCertainOrSilent pins that the reviewer context
// artifact (the prompt's first input) carries the same uncertainty rule as the
// prompt — an "if uncertain, escalate" leftover there would contradict the
// high-signal contract on every reviewer invocation.
func TestReviewerContextAlignsWithCertainOrSilent(t *testing.T) {
	prep := reviewerDryRunPreparation{}
	context := renderReviewerContext(prep)
	if strings.Contains(context, "If uncertain, propose a blocking finding") {
		t.Fatalf("reviewer context still escalates uncertainty, contradicting certain-or-silent:\n%s", context)
	}
	if !strings.Contains(context, "If you are not certain an issue is real after verification, do not flag it.") {
		t.Fatalf("reviewer context missing the certain-or-silent rule:\n%s", context)
	}
}

func TestReviewProposeFindingsConfidenceDefaultsAndValidation(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", reviewerStructuredOutput([]map[string]any{
		{
			"message":  "missing confidence defaults to medium",
			"severity": "medium",
			"category": "quality",
		},
		{
			"message":    "valid confidence is recorded",
			"severity":   "high",
			"category":   "correctness",
			"confidence": "low",
		},
		{
			"message":    "invalid confidence is skipped",
			"severity":   "medium",
			"category":   "quality",
			"confidence": "certain",
		},
	}), true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "proposal", "collect", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review propose-findings --json exited %d, stderr: %s", code, stderr.String())
	}
	var response reviewProposeFindingsResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if len(response.Created) != 2 {
		t.Fatalf("unexpected created proposals: %#v", response.Created)
	}
	if response.Created[0].Confidence != "medium" {
		t.Fatalf("missing confidence should default to medium: %#v", response.Created[0])
	}
	if response.Created[1].Confidence != "low" {
		t.Fatalf("valid confidence should be recorded: %#v", response.Created[1])
	}
	warnings := strings.Join(response.Warnings, "\n")
	if !strings.Contains(warnings, "confidence must be one of high, medium, low") {
		t.Fatalf("warnings missing confidence validation:\n%v", response.Warnings)
	}
}

func TestReviewShowDisplaysFindingConfidence(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", reviewerStructuredOutput([]map[string]any{
		{
			"message":    "confidence travels to the finding",
			"severity":   "medium",
			"category":   "quality",
			"file":       "internal/app/review.go",
			"confidence": "high",
		},
	}), true)
	runReviewCommand(t, app, "review", "proposal", "collect", runID)
	runReviewCommand(t, app, "review", "proposal", "accept", runID, "p_001")

	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || findings[0].Confidence != "high" {
		t.Fatalf("accepted finding should carry proposal confidence: %#v", findings)
	}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review show exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "confidence: high") {
		t.Fatalf("review show should display finding confidence:\n%s", got)
	}
}

func setupRunWithGateReport(t *testing.T, root string, status string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	writeReviewGateReportForTest(t, runPaths, runID, status)
	return app, paths, runID, runPaths
}

func writeReviewGateReportForTest(t *testing.T, runPaths contractRunPathSet, runID string, status string) {
	t.Helper()
	assertNoError(t, os.MkdirAll(runPaths.GateDir, 0o755))
	report := gateReportDocument{
		Schema:    gateReportSchema,
		RunID:     runID,
		CreatedAt: "2026-06-01T22:00:00Z",
		Status:    status,
	}
	assertNoError(t, writeJSON(runPaths.GateReportJSON, report))
}

// scaffoldReviewForTest writes the review record the mutating review commands
// would self-scaffold, for tests that start from an existing review.
func scaffoldReviewForTest(t *testing.T, runPaths contractRunPathSet, runID string, gateStatus string) {
	t.Helper()
	assertNoError(t, os.MkdirAll(runPaths.ReviewDir, 0o755))
	assertNoError(t, writeJSON(runPaths.ReviewJSON, newReviewDocument(runID, gateStatus, "2026-05-31T18:40:12Z")))
	assertNoError(t, ensureAppendOnlyFile(runPaths.ReviewFindingsJSONL))
	assertNoError(t, ensureAppendOnlyFile(runPaths.ReviewResolutionsJSONL))
}

func setupPreparedReview(t *testing.T, root string, gateStatus string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID, runPaths := setupRunWithGateReport(t, root, gateStatus)
	scaffoldReviewForTest(t, runPaths, runID, gateStatus)
	return app, paths, runID, runPaths
}

func setupApprovedPreparedReview(t *testing.T, root string, gateStatus string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	writeReviewGateReportForTest(t, runPaths, runID, gateStatus)
	scaffoldReviewForTest(t, runPaths, runID, gateStatus)
	return app, paths, runID, runPaths
}

func setupApprovedReviewWithoutGateReport(t *testing.T, root string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertNoError(t, os.MkdirAll(runPaths.ReviewDir, 0o755))
	assertNoError(t, writeJSON(runPaths.ReviewJSON, newReviewDocument(runID, "passed", "2026-05-31T18:40:12Z")))
	assertNoError(t, ensureAppendOnlyFile(runPaths.ReviewFindingsJSONL))
	assertNoError(t, ensureAppendOnlyFile(runPaths.ReviewResolutionsJSONL))
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

func writeExecutionAttemptForTest(t *testing.T, runPaths contractRunPathSet, runID string, attemptID string, agent agents.AgentDescriptor) {
	t.Helper()
	attemptPaths := executionAttemptPaths(runPaths, attemptID)
	assertNoError(t, os.MkdirAll(attemptPaths.Dir, 0o755))
	mustWriteFile(t, attemptPaths.StdoutLog, "")
	mustWriteFile(t, attemptPaths.StderrLog, "")
	_ = executionPromptRepoPath(runID)
	var wouldRun agents.DryRunCommand
	request := executionRequestDocument{
		Schema:         executionRequestSchema,
		RunID:          runID,
		AttemptID:      attemptID,
		CreatedAt:      "2026-06-01T22:00:00Z",
		ContractSHA256: "fixture",
		Agent: agents.AgentDescriptor{
			Name:    agent.Name,
			Command: agent.Command,
			Args:    append([]string{}, agent.Args...),
			Input:   agent.Input,
		},
		Artifacts: agents.DryRunArtifacts{
			Prompt:          agents.DryRunArtifactPrompt,
			ExecutorContext: agents.DryRunArtifactContext,
			PromptManifest:  agents.DryRunArtifactPromptManifest,
		},
		WouldRun: wouldRun,
	}
	assertNoError(t, writeJSON(attemptPaths.RequestJSON, request))
	result := executionResultDocument{
		Schema:    executionResultSchema,
		RunID:     runID,
		AttemptID: attemptID,
		processResult: processResult{
			StartedAt:      "2026-06-01T22:00:00Z",
			FinishedAt:     "2026-06-01T22:00:01Z",
			DurationMillis: 1000,
			ExitCode:       0,
			TimedOut:       false,
			Stdout:         filepath.ToSlash(filepath.Join("execute", "attempts", attemptID, "stdout.log")),
			Stderr:         filepath.ToSlash(filepath.Join("execute", "attempts", attemptID, "stderr.log")),
		},
	}
	assertNoError(t, writeJSON(attemptPaths.ResultJSON, result))
	assertNoError(t, writeJSON(runPaths.LastResultJSON, result))
}

func mustResolveExecutorForTest(t *testing.T, name string) agents.AgentDescriptor {
	t.Helper()
	agent, err := agents.BuiltinRegistry{}.ResolveExecutor(name)
	assertNoError(t, err)
	return agent
}

func writeReviewerAttemptForTest(t *testing.T, runPaths contractRunPathSet, runID string, attemptID string, stdout string, completed bool) {
	t.Helper()
	attemptPaths := reviewerAttemptPaths(runPaths, attemptID)
	assertNoError(t, os.MkdirAll(attemptPaths.Dir, 0o755))
	mustWriteFile(t, attemptPaths.StdoutLog, stdout)
	mustWriteFile(t, attemptPaths.StderrLog, "")
	if !completed {
		return
	}
	result := reviewerResultDocument{
		Schema:    reviewerResultSchema,
		RunID:     runID,
		AttemptID: attemptID,
		Reviewer:  "fixture",
		processResult: processResult{
			StartedAt:      "2026-06-01T22:00:00Z",
			FinishedAt:     "2026-06-01T22:00:01Z",
			DurationMillis: 1000,
			ExitCode:       0,
			TimedOut:       false,
			Stdout:         filepath.ToSlash(filepath.Join(reviewerAttemptsArtifact, attemptID, "stdout.log")),
			Stderr:         filepath.ToSlash(filepath.Join(reviewerAttemptsArtifact, attemptID, "stderr.log")),
		},
	}
	assertNoError(t, writeJSON(attemptPaths.ResultJSON, result))
}

func writeReviewFixAttemptForTest(t *testing.T, runPaths contractRunPathSet, runID string, attemptID string, stdout string, completed bool) {
	t.Helper()
	attemptPaths := reviewFixAttemptPaths(runPaths, attemptID)
	assertNoError(t, os.MkdirAll(attemptPaths.Dir, 0o755))
	mustWriteFile(t, attemptPaths.StdoutLog, stdout)
	mustWriteFile(t, attemptPaths.StderrLog, "")
	if !completed {
		return
	}
	result := reviewFixResultDocument{
		Schema:    reviewFixResultSchema,
		RunID:     runID,
		AttemptID: attemptID,
		Fixer:     "fixture",
		processResult: processResult{
			StartedAt:      "2026-06-01T22:00:00Z",
			FinishedAt:     "2026-06-01T22:00:01Z",
			DurationMillis: 1000,
			ExitCode:       0,
			TimedOut:       false,
			Stdout:         filepath.ToSlash(filepath.Join(reviewFixAttemptsArtifact, attemptID, "stdout.log")),
			Stderr:         filepath.ToSlash(filepath.Join(reviewFixAttemptsArtifact, attemptID, "stderr.log")),
		},
	}
	assertNoError(t, writeJSON(attemptPaths.ResultJSON, result))
}

func reviewerStructuredOutput(findings []map[string]any) string {
	block := map[string]any{
		"schema":   reviewerFindingsSchema,
		"findings": findings,
	}
	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		panic(err)
	}
	return "reviewer notes\n```json\n" + string(data) + "\n```\n"
}

func reviewFixStructuredOutput(outcomes []map[string]any) string {
	block := map[string]any{
		"schema":   reviewFixOutcomesSchema,
		"outcomes": outcomes,
	}
	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		panic(err)
	}
	return "fixer notes\n```json\n" + string(data) + "\n```\n"
}

func appendReviewProposalForTest(t *testing.T, runPaths contractRunPathSet, runID string, proposalID string, message string, blocking bool) {
	t.Helper()
	record := reviewProposalRecord{
		Schema:            reviewProposalSchema,
		ID:                proposalID,
		RunID:             runID,
		Source:            "reviewer_attempt",
		ReviewerAttemptID: "reviewer_attempt_001",
		findingCore: findingCore{
			Message:  message,
			Severity: "medium",
			Category: "quality",
			File:     "internal/app/review.go",
			Line:     12,
			Blocking: blocking,
		},
		Evidence:  "fixture evidence",
		Status:    "pending",
		CreatedAt: "2026-06-01T22:00:00Z",
	}
	assertNoError(t, appendJSONLine(runPaths.ReviewProposalsJSONL, record))
}

func appendProposalDecisionForTest(t *testing.T, runPaths contractRunPathSet, runID string, decisionID string, proposalID string, decision string, findingID string) {
	t.Helper()
	record := reviewProposalDecisionRecord{
		Schema:     reviewProposalDecisionSchema,
		ID:         decisionID,
		RunID:      runID,
		ProposalID: proposalID,
		Decision:   decision,
		FindingID:  findingID,
		CreatedAt:  "2026-06-01T22:00:01Z",
		Source:     "manual",
	}
	assertNoError(t, appendJSONLine(runPaths.ReviewProposalDecisionsJSONL, record))
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

func readReviewProposals(t *testing.T, path string) []reviewProposalRecord {
	t.Helper()
	lines := readLines(t, path)
	records := make([]reviewProposalRecord, 0, len(lines))
	for _, line := range lines {
		var record reviewProposalRecord
		assertNoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func readReviewProposalDecisions(t *testing.T, path string) []reviewProposalDecisionRecord {
	t.Helper()
	lines := readLines(t, path)
	records := make([]reviewProposalDecisionRecord, 0, len(lines))
	for _, line := range lines {
		var record reviewProposalDecisionRecord
		assertNoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func readReviewerDryRunPlan(t *testing.T, path string) reviewerDryRunDocument {
	t.Helper()
	var plan reviewerDryRunDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &plan))
	return plan
}

func configureHelperReviewers(t *testing.T, app App, paths artifacts.Paths, names ...string) App {
	t.Helper()
	registerTestAgents(t, paths, names...)
	app.AgentRegistry = testAgentRegistry(testHelperDescriptors(names, "TestReviewerHelperProcess")...)
	return app
}

func configureHelperFixers(t *testing.T, app App, paths artifacts.Paths, names ...string) App {
	t.Helper()
	registerTestAgents(t, paths, names...)
	app.AgentRegistry = testAgentRegistry(testHelperDescriptors(names, "TestReviewFixerHelperProcess")...)
	return app
}

func TestReviewerHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_REVIEWER_HELPER_PROCESS") != "1" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cwd error: %v\n", err)
		os.Exit(2)
	}
	expectedCWD := os.Getenv("PACTUM_REVIEWER_EXPECTED_CWD")
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
	fmt.Printf("stdin_has_reviewer_prompt=%t\n", strings.Contains(string(stdin), "# Reviewer Prompt"))
	if os.Getenv("PACTUM_REVIEWER_FINDING_TEXT") == "1" {
		fmt.Println("FINDING: critical issue in generated code")
	}
	fmt.Fprintln(os.Stderr, "reviewer-stderr-line")
	// Always emit a mandatory structured block (even on non-zero exit, so
	// the stdout.log has parseable content for artifact tests).
	fmt.Print(reviewerStructuredOutput([]map[string]any{}))
	if raw := os.Getenv("PACTUM_REVIEWER_EXIT"); raw != "" {
		code, err := strconv.Atoi(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad exit code: %v\n", err)
			os.Exit(2)
		}
		os.Exit(code)
	}
	os.Exit(0)
}

func TestReviewFixerHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_FIXER_HELPER_PROCESS") != "1" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cwd error: %v\n", err)
		os.Exit(2)
	}
	expectedCWD := os.Getenv("PACTUM_FIXER_EXPECTED_CWD")
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
	fmt.Printf("stdin_has_review_fix_prompt=%t\n", strings.Contains(string(stdin), "# Review Fix Prompt"))
	fmt.Fprintln(os.Stderr, "fixer-stderr-line")
	if raw := os.Getenv("PACTUM_FIXER_EXIT"); raw != "" {
		code, err := strconv.Atoi(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad exit code: %v\n", err)
			os.Exit(2)
		}
		os.Exit(code)
	}
	os.Exit(0)
}
