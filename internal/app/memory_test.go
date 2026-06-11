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

func TestMemoryProposeBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"memory", "propose", "run_x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("memory propose before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("memory propose before init stderr mismatch:\n%s", got)
	}
}

func TestMemoryProposeMissingRunReturnsError(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	app := testApp(root)
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"memory", "propose", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("memory propose missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestMemoryProposeBlocksIfContractNotApproved(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "propose", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("memory propose should fail before contract approval")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot propose memory: contract is not approved") {
		t.Fatalf("contract approval stderr mismatch:\n%s", got)
	}
}

func TestMemoryProposeBlocksIfGateReportMissing(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedPromptContract(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "propose", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("memory propose should fail without gate report")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot propose memory: gate report not found") {
		t.Fatalf("gate report stderr mismatch:\n%s", got)
	}
}

func TestMemoryProposeBlocksIfNoReviewRecordExists(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	writeReviewGateReportForTest(t, runPaths, runID, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "propose", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("memory propose should fail on a gated run with no review record")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot propose memory: review is not approved") {
		t.Fatalf("derived unapproved review stderr mismatch:\n%s", got)
	}
}

func TestMemoryProposeBlocksIfReviewNotApproved(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPreparedReview(t, root, "passed")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "propose", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("memory propose should fail before review approval")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot propose memory: review is not approved") {
		t.Fatalf("review approved stderr mismatch:\n%s", got)
	}
}

func TestMemoryProposeBlocksIfPendingProposalsRemain(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "pending proposal", false)
	runReviewCommand(t, app, "review", "approve", runID)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "propose", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("memory propose should fail with pending proposals")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot propose memory: pending review proposals remain") {
		t.Fatalf("pending proposals stderr mismatch:\n%s", got)
	}
}

func TestMemoryProposeSucceedsForApprovedReviewedRun(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedReviewedMemoryRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "propose", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory propose exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.MemoryCandidateJSON)
	assertFile(t, runPaths.MemoryCandidateMD)
	assertFile(t, runPaths.MemoryAcceptanceJSON)
	if got := stdout.String(); !strings.Contains(got, "Memory candidate proposed") || !strings.Contains(got, "pactum memory accept "+runID) {
		t.Fatalf("propose output mismatch:\n%s", got)
	}

	candidate := readMemoryCandidateForTest(t, runPaths.MemoryCandidateJSON)
	if candidate.Schema != memoryCandidateSchema || candidate.RunID != runID || candidate.Source != "deterministic" || candidate.Status != "proposed" {
		t.Fatalf("unexpected candidate identity: %#v", candidate)
	}
	if candidate.Contract.Goal == "" || len(candidate.Contract.InScope) == 0 || len(candidate.Contract.AcceptanceCriteria) == 0 || len(candidate.Contract.ValidationCommands) == 0 {
		t.Fatalf("candidate missing contract fields: %#v", candidate.Contract)
	}
	if candidate.Outcome.GateStatus != "passed" || candidate.Outcome.ReviewStatus != "approved" || candidate.Outcome.ExecutionExitCode != 0 || !candidate.Outcome.ValidationPassed {
		t.Fatalf("candidate outcome mismatch: %#v", candidate.Outcome)
	}
	if len(candidate.Clarifications) != 1 || candidate.Clarifications[0].QuestionID != "q_001" {
		t.Fatalf("candidate missing clarifications: %#v", candidate.Clarifications)
	}
	if len(candidate.Review.Findings) != 1 || candidate.Review.Findings[0].ID != "f_001" || candidate.Review.Findings[0].Status != "resolved" {
		t.Fatalf("candidate missing findings: %#v", candidate.Review.Findings)
	}
	if candidate.Review.ProposalSummary.Accepted != 1 || candidate.Review.ProposalSummary.Rejected != 1 || candidate.Review.ProposalSummary.Pending != 0 {
		t.Fatalf("candidate proposal summary mismatch: %#v", candidate.Review.ProposalSummary)
	}
	if strings.Contains(mustReadFile(t, runPaths.MemoryCandidateJSON), "cwd_is_repo") || strings.Contains(mustReadFile(t, runPaths.MemoryCandidateJSON), "stderr-line") {
		t.Fatalf("candidate should not contain raw execution logs:\n%s", mustReadFile(t, runPaths.MemoryCandidateJSON))
	}
	if indexOfEvent(ledgerEventTypes(t, paths.EventsJSONL), "memory_candidate_proposed") == -1 {
		t.Fatalf("events missing memory_candidate_proposed")
	}
}

func TestMemoryShowBeforeCandidatePrintsGuidance(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedReviewedMemoryRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory show before candidate exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Memory candidate has not been created. Run: pactum memory propose "+runID) {
		t.Fatalf("memory show guidance mismatch:\n%s", got)
	}
}

func TestMemoryShowAfterCandidateAndReadOnly(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedReviewedMemoryRun(t, root)
	runMemoryCommand(t, app, "memory", "propose", runID)

	beforeCandidate := mustReadFile(t, runPaths.MemoryCandidateJSON)
	beforeAcceptance := mustReadFile(t, runPaths.MemoryAcceptanceJSON)
	beforeItems := mustReadFile(t, paths.MemoryItems)
	beforeProjectMemory := mustReadFile(t, paths.ProjectMemory)
	beforeLedger := mustReadFile(t, paths.EventsJSONL)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory show exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "# Memory Candidate") || !strings.Contains(got, "## Reusable Project Knowledge") {
		t.Fatalf("memory show output mismatch:\n%s", got)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"memory", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory show --json exited %d, stderr: %s", code, stderr.String())
	}
	var response memoryShowResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Candidate.RunID != runID || response.Acceptance.Status != "pending" {
		t.Fatalf("memory show json mismatch: %#v", response)
	}
	if got := mustReadFile(t, runPaths.MemoryCandidateJSON); got != beforeCandidate {
		t.Fatalf("memory show mutated candidate")
	}
	if got := mustReadFile(t, runPaths.MemoryAcceptanceJSON); got != beforeAcceptance {
		t.Fatalf("memory show mutated acceptance")
	}
	if got := mustReadFile(t, paths.MemoryItems); got != beforeItems {
		t.Fatalf("memory show mutated global memory items")
	}
	if got := mustReadFile(t, paths.ProjectMemory); got != beforeProjectMemory {
		t.Fatalf("memory show mutated project memory")
	}
	if got := mustReadFile(t, paths.EventsJSONL); got != beforeLedger {
		t.Fatalf("memory show mutated ledger")
	}
}

func TestMemoryAcceptBeforeCandidateFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedReviewedMemoryRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "accept", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("memory accept should fail before candidate exists")
	}
	if got := stderr.String(); !strings.Contains(got, "memory candidate has not been created: "+runID) {
		t.Fatalf("memory accept before candidate stderr mismatch:\n%s", got)
	}
}

func TestMemoryAcceptWritesGlobalItemAndStatus(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedReviewedMemoryRun(t, root)
	runMemoryCommand(t, app, "memory", "propose", runID)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "accept", runID, "--by", "reviewer"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory accept exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Memory item accepted") || !strings.Contains(got, "id: mem_001") {
		t.Fatalf("accept output mismatch:\n%s", got)
	}

	items := readMemoryItemsForTest(t, paths.MemoryItems)
	if len(items) != 1 || items[0].ID != "mem_001" || items[0].RunID != runID || items[0].AcceptedBy != "reviewer" {
		t.Fatalf("unexpected memory items: %#v", items)
	}
	projectMemory := mustReadFile(t, paths.ProjectMemory)
	if !strings.Contains(projectMemory, "mem_001") || !strings.Contains(projectMemory, runID) {
		t.Fatalf("project memory missing accepted item:\n%s", projectMemory)
	}
	acceptance := readMemoryAcceptanceForTest(t, runPaths.MemoryAcceptanceJSON)
	if acceptance.Status != "accepted" || acceptance.MemoryItemID == nil || *acceptance.MemoryItemID != "mem_001" {
		t.Fatalf("acceptance mismatch: %#v", acceptance)
	}
	if indexOfEvent(ledgerEventTypes(t, paths.EventsJSONL), "memory_item_accepted") == -1 {
		t.Fatalf("events missing memory_item_accepted")
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Memory:") || !strings.Contains(got, "items: 1") || !strings.Contains(got, "stale: 0") {
		t.Fatalf("status memory count mismatch:\n%s", got)
	}
}

func TestMemoryAcceptTwiceFailsWithoutDuplicateItems(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedReviewedMemoryRun(t, root)
	runMemoryCommand(t, app, "memory", "propose", runID)
	runMemoryCommand(t, app, "memory", "accept", runID)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "accept", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("memory accept twice should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "memory candidate already accepted: "+runID) {
		t.Fatalf("memory accept twice stderr mismatch:\n%s", got)
	}
	if items := readMemoryItemsForTest(t, paths.MemoryItems); len(items) != 1 {
		t.Fatalf("duplicate memory items written: %#v", items)
	}
}

func TestMemoryArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := runSuccessfulHelperAttempt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	writeMemoryGateReportForTest(t, runPaths, runID)
	runMemoryCommand(t, app, "review", "finding", "add", runID, "path should be sanitized from "+root, "--file", "internal/app/memory.go", "--line", "42")
	runMemoryCommand(t, app, "review", "finding", "resolve", runID, "f_001", "--note", "fixed in "+root)
	runMemoryCommand(t, app, "review", "approve", runID)
	runMemoryCommand(t, app, "memory", "propose", runID)
	runMemoryCommand(t, app, "memory", "accept", runID)

	for name, content := range map[string]string{
		"memory-candidate.json":  mustReadFile(t, runPaths.MemoryCandidateJSON),
		"memory-candidate.md":    mustReadFile(t, runPaths.MemoryCandidateMD),
		"memory-acceptance.json": mustReadFile(t, runPaths.MemoryAcceptanceJSON),
		"memory/items.jsonl":     mustReadFile(t, paths.MemoryItems),
		"project-memory.md":      mustReadFile(t, paths.ProjectMemory),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestAcceptedMemoryCandidateCannotBeOverwrittenWithChangedContent(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedReviewedMemoryRun(t, root)
	runMemoryCommand(t, app, "memory", "propose", runID)
	runMemoryCommand(t, app, "memory", "accept", runID)

	report := readGateReport(t, runPaths.GateReportJSON)
	report.Changes.NewFiles = []string{"generated.go"}
	assertNoError(t, writeJSON(runPaths.GateReportJSON, report))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "propose", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("memory propose should fail after accepted candidate source changes")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot update accepted memory candidate") {
		t.Fatalf("accepted candidate overwrite stderr mismatch:\n%s", got)
	}
}

func setupApprovedReviewedMemoryRun(t *testing.T, root string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID := runSuccessfulHelperAttempt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	writeMemoryGateReportForTest(t, runPaths, runID)
	runMemoryCommand(t, app, "review", "finding", "add", runID, "memory candidate should include resolved findings", "--file", "internal/app/memory.go", "--line", "12")
	runMemoryCommand(t, app, "review", "finding", "resolve", runID, "f_001", "--note", "covered by deterministic memory tests")
	appendReviewProposalForTest(t, runPaths, runID, "p_001", "accepted proposal summary", false)
	appendReviewProposalForTest(t, runPaths, runID, "p_002", "rejected proposal summary", false)
	appendProposalDecisionForTest(t, runPaths, runID, "pd_001", "p_001", "accepted", "f_002")
	appendProposalDecisionForTest(t, runPaths, runID, "pd_002", "p_002", "rejected", "")
	runMemoryCommand(t, app, "review", "approve", runID)
	return app, paths, runID, runPaths
}

func writeMemoryGateReportForTest(t *testing.T, runPaths contractRunPathSet, runID string) {
	t.Helper()
	assertNoError(t, os.MkdirAll(runPaths.GateDir, 0o755))
	report := gateReportDocument{
		Schema:    gateReportSchema,
		RunID:     runID,
		CreatedAt: "2026-06-01T22:00:00Z",
		Status:    "passed",
		Execution: gateExecutionReport{
			AttemptID: "attempt_001",
			ExitCode:  0,
			TimedOut:  false,
			Result:    "execute/last-result.json",
		},
		Changes: gateChangeReport{
			Status:       "clean",
			ChangedFiles: []string{},
			NewFiles:     []string{},
			MissingFiles: []string{},
			Reasons:      []string{},
		},
		Validation: gateValidationReport{
			CommandsAllowed: true,
			Commands: []gateValidationCommandReport{
				{
					ID:       "command_001",
					Command:  "go test ./...",
					ExitCode: 0,
					TimedOut: false,
					Result:   "passed",
				},
			},
		},
		Summary: gateSummary{
			ExecutionPassed:   true,
			ValidationPassed:  true,
			ChangesNeedReview: false,
		},
	}
	assertNoError(t, writeJSON(runPaths.GateReportJSON, report))
}

func runMemoryCommand(t *testing.T, app App, args ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := app.Run(args, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("%v exited %d, stdout: %s stderr: %s", args, code, stdout.String(), stderr.String())
	}
}

func readMemoryCandidateForTest(t *testing.T, path string) memoryCandidateDocument {
	t.Helper()
	var candidate memoryCandidateDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &candidate))
	return candidate
}

func readMemoryAcceptanceForTest(t *testing.T, path string) memoryAcceptanceDocument {
	t.Helper()
	var acceptance memoryAcceptanceDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &acceptance))
	return acceptance
}

func readMemoryItemsForTest(t *testing.T, path string) []memoryItemRecord {
	t.Helper()
	lines := readLines(t, path)
	items := make([]memoryItemRecord, 0, len(lines))
	for _, line := range lines {
		var item memoryItemRecord
		assertNoError(t, json.Unmarshal([]byte(line), &item))
		items = append(items, item)
	}
	return items
}
