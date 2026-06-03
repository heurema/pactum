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

func TestReviewPrepareBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"review", "prepare", "run_x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("review prepare before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("review prepare before init stderr mismatch:\n%s", got)
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

func TestReviewAddFindingRefreshesCurrentGateStatus(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewGateReportForTest(t, runPaths, runID, "failed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "add-finding", runID, "gate changed after prepare"}, &stdout, &stderr)
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

func TestReviewResolveRefreshesCurrentGateStatus(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "blocking process issue", "--blocking")
	writeReviewGateReportForTest(t, runPaths, runID, "failed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "resolve", runID, "f_001", "--note", "gate changed after prepare"}, &stdout, &stderr)
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

func TestReviewDryRunBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"review", "dry-run", "run_x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("review dry-run before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("review dry-run before init stderr mismatch:\n%s", got)
	}
}

func TestReviewDryRunMissingRunReturnsError(t *testing.T) {
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
	code = app.Run([]string{"review", "dry-run", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review dry-run missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestReviewDryRunBeforeReviewPreparedFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "dry-run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review dry-run should fail before review prepare")
	}
	if got := stderr.String(); !strings.Contains(got, "review has not been prepared: "+runID) {
		t.Fatalf("review not prepared stderr mismatch:\n%s", got)
	}
}

func TestReviewDryRunMissingGateReportFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedReviewWithoutGateReport(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "dry-run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review dry-run should fail without gate report")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare reviewer dry-run: gate report not found") {
		t.Fatalf("missing gate stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.ReviewDryRunJSON)
}

func TestReviewDryRunContractNotApprovedFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "dry-run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review dry-run should fail without approved contract")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare reviewer dry-run: contract is not approved") {
		t.Fatalf("unapproved contract stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.ReviewDryRunJSON)
}

func TestReviewDryRunSucceeds(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "dry-run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review dry-run exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.ReviewContextMD)
	assertFile(t, runPaths.ReviewPromptMD)
	assertFile(t, runPaths.ReviewDryRunJSON)
	got := stdout.String()
	for _, want := range []string{
		"Reviewer dry-run prepared",
		"Would run:",
		"codex exec --dangerously-bypass-approvals-and-sandbox < .heurema/pactum/runs/" + runID + "/review/reviewer-prompt.md",
		".heurema/pactum/runs/" + runID + "/review/reviewer-context.md",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("review dry-run output missing %q:\n%s", want, got)
		}
	}
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Schema != reviewerDryRunSchema || plan.RunID != runID || plan.Reviewer.Name != "codex" {
		t.Fatalf("unexpected reviewer dry-run plan: %#v", plan)
	}
	wantPrompt := runArtifactRepoRel(runID, reviewerPromptArtifact)
	if plan.WouldRun.Command != "codex" || strings.Join(plan.WouldRun.Args, " ") != "exec --dangerously-bypass-approvals-and-sandbox" || plan.WouldRun.Stdin != wantPrompt {
		t.Fatalf("unexpected would_run command: %#v", plan.WouldRun)
	}
	assertCommandArgsDoNotContain(t, plan.WouldRun.Args, reviewerPromptArtifact, wantPrompt)
	prompt := mustReadFile(t, runPaths.ReviewPromptMD)
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

func TestReviewDryRunJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "dry-run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review dry-run --json exited %d, stderr: %s", code, stderr.String())
	}
	var plan reviewerDryRunDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &plan))
	if plan.Reviewer.Name != "codex" || !plan.Checks.ReviewPrepared || !plan.Checks.GateReportReady || !plan.Checks.ContractApproved {
		t.Fatalf("unexpected reviewer dry-run json: %#v", plan)
	}
	if plan.Artifacts.ReviewerPrompt != reviewerPromptArtifact ||
		plan.Artifacts.ReviewerContext != reviewerContextArtifact ||
		plan.WouldRun.Command != "codex" ||
		strings.Join(plan.WouldRun.Args, " ") != "exec --dangerously-bypass-approvals-and-sandbox" ||
		plan.WouldRun.Stdin != runArtifactRepoRel(runID, reviewerPromptArtifact) {
		t.Fatalf("reviewer dry-run json missing artifacts/would_run: %#v", plan)
	}
	if strings.Contains(stdout.String(), "Reviewer dry-run prepared") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestReviewDryRunUsesDefaultReviewer(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")

	runReviewCommand(t, app, "review", "dry-run", runID)
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Reviewer.Name != "codex" || plan.Reviewer.Command != "codex" {
		t.Fatalf("default reviewer mismatch: %#v", plan.Reviewer)
	}
	if strings.Join(plan.WouldRun.Args, " ") != "exec --dangerously-bypass-approvals-and-sandbox" || plan.WouldRun.Stdin != runArtifactRepoRel(runID, reviewerPromptArtifact) {
		t.Fatalf("default reviewer would_run mismatch: %#v", plan.WouldRun)
	}
}

func TestReviewDryRunExplicitReviewers(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")

	runReviewCommand(t, app, "review", "dry-run", runID, "--reviewer", "codex")
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Reviewer.Name != "codex" || plan.Reviewer.Command != "codex" {
		t.Fatalf("codex reviewer mismatch: %#v", plan.Reviewer)
	}
	if strings.Join(plan.WouldRun.Args, " ") != "exec --dangerously-bypass-approvals-and-sandbox" || plan.WouldRun.Stdin != runArtifactRepoRel(runID, reviewerPromptArtifact) {
		t.Fatalf("codex would_run mismatch: %#v", plan.WouldRun)
	}
	assertCommandArgsDoNotContain(t, plan.WouldRun.Args, reviewerPromptArtifact, runArtifactRepoRel(runID, reviewerPromptArtifact))

	runReviewCommand(t, app, "review", "dry-run", runID, "--reviewer", "claude")
	plan = readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)
	if plan.Reviewer.Name != "claude" || plan.Reviewer.Command != "claude" {
		t.Fatalf("claude reviewer mismatch: %#v", plan.Reviewer)
	}
	if strings.Join(plan.WouldRun.Args, " ") != "-p" || plan.WouldRun.Stdin != runArtifactRepoRel(runID, reviewerPromptArtifact) {
		t.Fatalf("claude would_run mismatch: %#v", plan.WouldRun)
	}
	assertCommandArgsDoNotContain(t, plan.WouldRun.Args, reviewerPromptArtifact, runArtifactRepoRel(runID, reviewerPromptArtifact))
}

func TestReviewDryRunUnsupportedReviewerFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "dry-run", runID, "--reviewer", "missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review dry-run should fail for missing reviewer")
	}
	if got := stderr.String(); !strings.Contains(got, "unsupported agent: missing") {
		t.Fatalf("missing reviewer stderr mismatch:\n%s", got)
	}
}

func TestReviewDryRunUnsupportedInputModeFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	app.AgentRegistry = testAgentRegistry(agents.AgentDescriptor{
		Name:    "bad-input",
		Command: "bad-reviewer",
		Args:    []string{"dry-run"},
		Input:   "stdin",
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "dry-run", runID, "--reviewer", "bad-input"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review dry-run should fail for unsupported input mode")
	}
	if got := stderr.String(); !strings.Contains(got, "unsupported agent input mode: stdin") {
		t.Fatalf("unsupported input stderr mismatch:\n%s", got)
	}
}

func TestReviewDryRunContextIncludesManualFindings(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "Manual review artifacts should stay append-only", "--category", "process", "--severity", "medium")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "Pending proposal should be visible to reviewer", true)

	runReviewCommand(t, app, "review", "dry-run", runID)
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

func TestReviewDryRunArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "portable reviewer context", "--file", "internal/app/review.go")

	runReviewCommand(t, app, "review", "dry-run", runID)
	for name, content := range map[string]string{
		"review/reviewer-context.md":   mustReadFile(t, runPaths.ReviewContextMD),
		"review/reviewer-prompt.md":    mustReadFile(t, runPaths.ReviewPromptMD),
		"review/reviewer-dry-run.json": mustReadFile(t, runPaths.ReviewDryRunJSON),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestReviewDryRunWritesLedgerEvent(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")

	runReviewCommand(t, app, "review", "dry-run", runID)
	events := strings.Join(readLines(t, paths.EventsJSONL), "\n")
	if !strings.Contains(events, "review_dry_run_prepared") || !strings.Contains(events, runID) {
		t.Fatalf("events missing review_dry_run_prepared:\n%s", events)
	}
}

func TestReviewDryRunDoesNotMutateManualReviewArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	runReviewCommand(t, app, "review", "add-finding", runID, "manual artifact should not change")

	beforeReview := mustReadFile(t, runPaths.ReviewJSON)
	beforeFindings := mustReadFile(t, runPaths.ReviewFindingsJSONL)
	beforeResolutions := mustReadFile(t, runPaths.ReviewResolutionsJSONL)
	beforeEvents := readLines(t, paths.EventsJSONL)

	runReviewCommand(t, app, "review", "dry-run", runID)

	if got := mustReadFile(t, runPaths.ReviewJSON); got != beforeReview {
		t.Fatalf("review dry-run mutated review.json")
	}
	if got := mustReadFile(t, runPaths.ReviewFindingsJSONL); got != beforeFindings {
		t.Fatalf("review dry-run mutated findings.jsonl")
	}
	if got := mustReadFile(t, runPaths.ReviewResolutionsJSONL); got != beforeResolutions {
		t.Fatalf("review dry-run mutated resolutions.jsonl")
	}
	afterEvents := readLines(t, paths.EventsJSONL)
	if len(afterEvents) != len(beforeEvents)+1 || !strings.Contains(afterEvents[len(afterEvents)-1], "review_dry_run_prepared") {
		t.Fatalf("review dry-run should only append its ledger event:\nbefore=%v\nafter=%v", beforeEvents, afterEvents)
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

func TestReviewRunBeforeReviewPreparedFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should fail before review prepare")
	}
	if got := stderr.String(); !strings.Contains(got, "review has not been prepared: "+runID) {
		t.Fatalf("review not prepared stderr mismatch:\n%s", got)
	}
}

func TestReviewRunMissingGateReportFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedReviewWithoutGateReport(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should fail without gate report")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot run reviewer: gate report not found") {
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
	if got := stderr.String(); !strings.Contains(got, "unsupported agent: missing") {
		t.Fatalf("missing reviewer stderr mismatch:\n%s", got)
	}
}

func TestReviewRunUnsupportedInputModeFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	app.AgentRegistry = testAgentRegistry(agents.AgentDescriptor{
		Name:    "bad-input",
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

func TestReviewRunWritesAttemptArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper", "helper")
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
	if got := stdout.String(); !strings.Contains(got, "Reviewer attempt finished") || !strings.Contains(got, "reviewer_attempt_001") {
		t.Fatalf("review run output mismatch:\n%s", got)
	}

	attemptPaths := reviewerAttemptPaths(runPaths, "reviewer_attempt_001")
	assertFile(t, attemptPaths.RequestJSON)
	assertFile(t, attemptPaths.StdoutLog)
	assertFile(t, attemptPaths.StderrLog)
	assertFile(t, attemptPaths.ResultJSON)
	assertFile(t, runPaths.ReviewLastResultJSON)

	var request reviewerRequestDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.RequestJSON)), &request))
	if request.Schema != reviewerRequestSchema || request.RunID != runID || request.AttemptID != "reviewer_attempt_001" {
		t.Fatalf("unexpected request: %#v", request)
	}
	if request.Reviewer.Name != "helper" || request.Reviewer.Command != os.Args[0] || request.Reviewer.Input != agents.InputPromptFile {
		t.Fatalf("unexpected request reviewer: %#v", request.Reviewer)
	}
	wantPrompt := ".heurema/pactum/runs/" + runID + "/review/reviewer-prompt.md"
	if request.WouldRun.Stdin != wantPrompt {
		t.Fatalf("unexpected would_run stdin = %q, want %q", request.WouldRun.Stdin, wantPrompt)
	}
	if got := strings.Join(request.WouldRun.Args, " "); strings.Contains(got, wantPrompt) {
		t.Fatalf("would_run args should not pass prompt path positionally: %#v", request.WouldRun.Args)
	}
	if request.Artifacts.ReviewerPrompt != reviewerPromptArtifact || request.Artifacts.GateReport != gateReportArtifact {
		t.Fatalf("unexpected request artifacts: %#v", request.Artifacts)
	}

	var result reviewerResultDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.ResultJSON)), &result))
	if result.Schema != reviewerResultSchema || result.Reviewer != "helper" || result.ExitCode != 0 || result.TimedOut {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Stdout != "review/reviewer-attempts/reviewer_attempt_001/stdout.log" || result.Stderr != "review/reviewer-attempts/reviewer_attempt_001/stderr.log" {
		t.Fatalf("unexpected output artifact paths: %#v", result)
	}
	if got := mustReadFile(t, runPaths.ReviewLastResultJSON); got != mustReadFile(t, attemptPaths.ResultJSON) {
		t.Fatalf("reviewer-last-result.json should copy result.json")
	}
	if got := mustReadFile(t, attemptPaths.StdoutLog); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stdin_has_reviewer_prompt=true") {
		t.Fatalf("stdout log mismatch:\n%s", got)
	}
	if got := mustReadFile(t, attemptPaths.StderrLog); !strings.Contains(got, "reviewer-stderr-line") {
		t.Fatalf("stderr log mismatch:\n%s", got)
	}
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
}

func TestReviewRunNonZeroWritesArtifactsAndReturnsNonZero(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper", "helper")
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
	if countEvents(eventTypes, "reviewer_attempt_started") != 1 || countEvents(eventTypes, "reviewer_attempt_finished") != 1 {
		t.Fatalf("non-zero review run should write started and finished events:\n%v", eventTypes)
	}
}

func TestReviewRunCreatesIncrementingAttempts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper", "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)

	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"review", "run", runID, "--reviewer", "helper"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("review run %d exited %d, stderr: %s", i+1, code, stderr.String())
		}
	}
	for _, attemptID := range []string{"reviewer_attempt_001", "reviewer_attempt_002"} {
		attemptPaths := reviewerAttemptPaths(runPaths, attemptID)
		assertFile(t, attemptPaths.RequestJSON)
		assertFile(t, attemptPaths.ResultJSON)
	}
}

func TestReviewRunStoresCrossReviewerAttempts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper-a", "helper-a", "helper-b")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)

	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper-a")
	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper-b")

	for attemptID, wantReviewer := range map[string]string{
		"reviewer_attempt_001": "helper-a",
		"reviewer_attempt_002": "helper-b",
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

func TestReviewRunAutoBuildsDryRunArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper", "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)
	_ = os.Remove(runPaths.ReviewDryRunJSON)
	_ = os.Remove(runPaths.ReviewPromptMD)
	_ = os.Remove(runPaths.ReviewContextMD)

	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper")
	assertFile(t, runPaths.ReviewDryRunJSON)
	assertFile(t, runPaths.ReviewPromptMD)
	assertFile(t, runPaths.ReviewContextMD)
}

func TestReviewRunRequestWouldRunMatchesDryRun(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper", "helper")
	runReviewCommand(t, app, "review", "dry-run", runID, "--reviewer", "helper")
	plan := readReviewerDryRunPlan(t, runPaths.ReviewDryRunJSON)

	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)
	runReviewCommand(t, app, "review", "run", runID, "--reviewer", "helper")

	var request reviewerRequestDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_001").RequestJSON)), &request))
	if request.WouldRun.Command != plan.WouldRun.Command || request.WouldRun.Stdin != plan.WouldRun.Stdin || !sameStringSlice(request.WouldRun.Args, plan.WouldRun.Args) {
		t.Fatalf("request would_run does not match dry-run:\nrequest=%#v\nplan=%#v", request.WouldRun, plan.WouldRun)
	}
}

func TestReviewRunArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper", "helper")
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
	app = configureHelperReviewers(t, app, paths, "helper", "helper")
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
	app = configureHelperReviewers(t, app, paths, "helper", "helper")
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "helper", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run --json exited %d, stderr: %s", code, stderr.String())
	}
	var result reviewerResultDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &result))
	if result.AttemptID != "reviewer_attempt_001" || result.Reviewer != "helper" || result.ExitCode != 0 {
		t.Fatalf("unexpected review run json: %#v", result)
	}
	if strings.Contains(stdout.String(), "Reviewer attempt finished") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestReviewRunPreconditionFailuresDoNotWriteAttemptEvents(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupRunWithGateReport(t, root, "passed")
	beforeEvents := readLines(t, paths.EventsJSONL)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review run should fail before review prepare")
	}
	afterEvents := readLines(t, paths.EventsJSONL)
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("precondition failure should not append ledger events:\nbefore=%v\nafter=%v", beforeEvents, afterEvents)
	}
	for _, forbidden := range []string{"reviewer_attempt_started", "reviewer_attempt_finished"} {
		if strings.Contains(strings.Join(afterEvents, "\n"), forbidden) {
			t.Fatalf("precondition failure should not write %s:\n%v", forbidden, afterEvents)
		}
	}
}

func TestReviewProposeFindingsBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"review", "propose-findings", "run_x"}, &stdout, &stderr)
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
	code = app.Run([]string{"review", "propose-findings", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review propose-findings missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestReviewProposeFindingsBeforeReviewPreparedFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "propose-findings", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("review propose-findings should fail before review prepare")
	}
	if got := stderr.String(); !strings.Contains(got, "review has not been prepared: "+runID) {
		t.Fatalf("review not prepared stderr mismatch:\n%s", got)
	}
}

func TestReviewProposeFindingsWithNoReviewerAttemptsFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "propose-findings", runID}, &stdout, &stderr)
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
	code := app.Run([]string{"review", "propose-findings", runID}, &stdout, &stderr)
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
	code := app.Run([]string{"review", "propose-findings", runID}, &stdout, &stderr)
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
	code := app.Run([]string{"review", "propose-findings", runID, "--json"}, &stdout, &stderr)
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

func TestReviewProposeFindingsUsesExplicitAttemptID(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_001", reviewerStructuredOutput([]map[string]any{
		{"message": "first attempt", "severity": "medium", "category": "quality"},
	}), true)
	writeReviewerAttemptForTest(t, runPaths, runID, "reviewer_attempt_002", reviewerStructuredOutput([]map[string]any{
		{"message": "second attempt", "severity": "medium", "category": "quality"},
	}), true)

	runReviewCommand(t, app, "review", "propose-findings", runID, "reviewer_attempt_001")
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

	runReviewCommand(t, app, "review", "propose-findings", runID)
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
	code := app.Run([]string{"review", "accept-proposal", runID, "p_001"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review accept-proposal exited %d, stderr: %s", code, stderr.String())
	}
	findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL)
	if len(findings) != 1 || findings[0].ID != "f_001" || findings[0].Source != "reviewer_proposal" || !findings[0].Blocking {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 1 || decisions[0].ID != "pd_001" || decisions[0].Decision != "accepted" || decisions[0].FindingID != "f_001" {
		t.Fatalf("unexpected decisions: %#v", decisions)
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
	code := app.Run([]string{"review", "accept-proposal", runID, "p_001", "--json"}, &stdout, &stderr)
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
	code := app.Run([]string{"review", "reject-proposal", runID, "p_001", "--reason", "not relevant"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review reject-proposal exited %d, stderr: %s", code, stderr.String())
	}
	if findings := readReviewFindings(t, runPaths.ReviewFindingsJSONL); len(findings) != 0 {
		t.Fatalf("reject should not create findings: %#v", findings)
	}
	decisions := readReviewProposalDecisions(t, runPaths.ReviewProposalDecisionsJSONL)
	if len(decisions) != 1 || decisions[0].Decision != "rejected" || decisions[0].Reason != "not relevant" {
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

func TestReviewProposalCannotBeDecidedTwice(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "decide once", false)
	runReviewCommand(t, app, "review", "accept-proposal", runID, "p_001")

	for _, args := range [][]string{
		{"review", "accept-proposal", runID, "p_001"},
		{"review", "reject-proposal", runID, "p_001"},
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

	runReviewCommand(t, app, "review", "accept-proposal", runID, "p_001")
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

	runReviewCommand(t, app, "review", "propose-findings", runID)
	runReviewCommand(t, app, "review", "accept-proposal", runID, "p_001")
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

	runReviewCommand(t, app, "review", "dry-run", runID)
	prompt := mustReadFile(t, runPaths.ReviewPromptMD)
	for _, want := range []string{
		"## Optional structured finding proposals",
		`"schema": "pactum.reviewer_findings.v1"`,
		"Focus on real problems, not style preferences.",
		"Read the actual file and surrounding context before proposing a finding.",
		"Check whether the issue is already mitigated or already represented in existing findings/proposals.",
		"Important: Pactum does not trust this output automatically. A human must accept proposals.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("reviewer prompt missing %q:\n%s", want, prompt)
		}
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

func setupPreparedReview(t *testing.T, root string, gateStatus string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID, runPaths := setupRunWithGateReport(t, root, gateStatus)
	runReviewCommand(t, app, "review", "prepare", runID)
	return app, paths, runID, runPaths
}

func setupApprovedPreparedReview(t *testing.T, root string, gateStatus string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	writeReviewGateReportForTest(t, runPaths, runID, gateStatus)
	runReviewCommand(t, app, "review", "prepare", runID)
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

func configureHelperReviewers(t *testing.T, app App, paths artifacts.Paths, defaultReviewer string, names ...string) App {
	t.Helper()
	_ = paths
	descriptors := make([]agents.AgentDescriptor, 0, len(names))
	for _, name := range names {
		descriptors = append(descriptors, agents.AgentDescriptor{
			Name:    name,
			Command: os.Args[0],
			Args:    []string{"-test.run=TestReviewerHelperProcess"},
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
