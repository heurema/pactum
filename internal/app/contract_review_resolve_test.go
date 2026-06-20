package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// writeBlockingFindingLine writes a single blocking contractReviewFindingLine to
// the run's findings.jsonl, generating the ID and fingerprint the same way the
// production code does.
func writeBlockingFindingLine(t *testing.T, runPaths contractRunPathSet, category, message string) contractReviewFindingLine {
	t.Helper()
	line := contractReviewFindingLine{
		ID:          nextContractReviewFindingID(1),
		Fingerprint: canonicalBlockerKey(category, message),
		Category:    category,
		Message:     message,
		Blocking:    true,
	}
	data, err := json.Marshal(line)
	assertNoError(t, err)
	assertNoError(t, activeStore.WriteBytes(runPaths.ContractReviewFindingsJSONL, append(data, '\n'), 0o644))
	return line
}

// TestContractReviewFindingResolveSuccessAndApprove is the happy-path AC:
// resolve a blocking finding → approve passes with a loud waiver summary.
func TestContractReviewFindingResolveSuccessAndApprove(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	line := writeBlockingFindingLine(t, runPaths, "completeness", "missing acceptance criteria")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, line.ID, "--reason", "operator judgment: already covered in docs", "--by", "alice"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("resolve exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "Contract review finding resolved") {
		t.Fatalf("expected resolved message, got:\n%s", got)
	}
	if !strings.Contains(got, line.ID) {
		t.Fatalf("expected finding id %s in output, got:\n%s", line.ID, got)
	}

	// Resolutions file must have been written.
	resolutions, err := readContractReviewResolutions(runPaths)
	assertNoError(t, err)
	if len(resolutions) != 1 {
		t.Fatalf("expected 1 resolution, got %d", len(resolutions))
	}
	r := resolutions[0]
	if r.FindingID != line.ID {
		t.Fatalf("resolution finding_id %q != %q", r.FindingID, line.ID)
	}
	if r.Fingerprint != line.Fingerprint {
		t.Fatalf("resolution fingerprint %q != %q", r.Fingerprint, line.Fingerprint)
	}
	if r.Reason == "" {
		t.Fatalf("resolution reason must be non-empty")
	}
	if r.Source != "manual" {
		t.Fatalf("resolution source %q != manual", r.Source)
	}
	if r.ContractHash == "" {
		t.Fatalf("resolution contract_hash must be non-empty")
	}

	// Ledger must record the event.
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "contract_review_finding_resolved") != 1 {
		t.Fatalf("expected 1 contract_review_finding_resolved event, got:\n%v", eventTypes)
	}

	// Contract approve must now pass and print a waiver warning.
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve after resolution exited %d, stderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}
	approveOut := stdout.String()
	if !strings.Contains(approveOut, "WARNING") {
		t.Fatalf("approve output must print WARNING for waived findings, got:\n%s", approveOut)
	}
	if !strings.Contains(approveOut, line.ID) {
		t.Fatalf("waiver summary must include finding id %s, got:\n%s", line.ID, approveOut)
	}
}

// TestContractReviewFindingResolveJSONOutput verifies --json output.
func TestContractReviewFindingResolveJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	line := writeBlockingFindingLine(t, runPaths, "correctness", "no gating commands")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, line.ID, "--reason", "covered by CI", "--by", "bob", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("resolve --json exited %d, stderr: %s", code, stderr.String())
	}

	var resp contractReviewFindingResolveResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("cannot parse JSON response: %v\noutput: %s", err, stdout.String())
	}
	if resp.Resolution.FindingID != line.ID {
		t.Fatalf("JSON resolution finding_id %q != %q", resp.Resolution.FindingID, line.ID)
	}
	if resp.Resolution.Fingerprint != line.Fingerprint {
		t.Fatalf("JSON resolution fingerprint mismatch")
	}
	if resp.Resolution.Source != "manual" {
		t.Fatalf("JSON resolution source %q != manual", resp.Resolution.Source)
	}
}

// TestContractReviewApproveRefusesUnresolvedBlocker verifies that approve still
// fails when a blocker has no resolution.
func TestContractReviewApproveRefusesUnresolvedBlocker(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	writeBlockingFindingLine(t, runPaths, "security", "no auth check")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("contract approve should fail when blocking finding is unresolved; got exit 0\nstdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "blocking") {
		t.Fatalf("error must mention blocking, got:\n%s", stderr.String())
	}
}

// TestContractReviewResolutionHashChangeInvalidates verifies that a resolution
// recorded against one contract hash does not satisfy approve after the contract
// changes (hash changes → approve fails again).
func TestContractReviewResolutionHashChangeInvalidates(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	line := writeBlockingFindingLine(t, runPaths, "clarity", "scope is ambiguous")

	// Resolve it (captures the current contract hash).
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, line.ID, "--reason", "scope is fine", "--by", "carol"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("resolve exited %d: %s", code, stderr.String())
	}

	// Mutate the contract so the hash changes.
	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{"assumptions": []string{"assumption added after resolution"}})
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "revise", runID, "--from", fromFile}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d: %s", code, stderr.String())
	}

	// Approve must now fail because the resolution hash no longer matches.
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("contract approve should fail after contract changed (resolution hash stale); got exit 0\nstdout: %s", stdout.String())
	}
}

// TestContractReviewFindingResolveRequiresReason checks that --reason is required.
func TestContractReviewFindingResolveRequiresReason(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	line := writeBlockingFindingLine(t, runPaths, "completeness", "no ACs")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, line.ID, "--by", "alice"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("resolve without --reason should fail; got exit 0")
	}
}

// TestContractReviewFindingResolveRequiresBy checks that --by is required.
func TestContractReviewFindingResolveRequiresBy(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	line := writeBlockingFindingLine(t, runPaths, "completeness", "no ACs")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, line.ID, "--reason", "fine"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("resolve without --by should fail; got exit 0")
	}
}

// TestContractReviewFindingResolveNotFoundError checks that resolving a
// non-existent finding ID returns a non-zero exit code with a clear message.
func TestContractReviewFindingResolveNotFoundError(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	writeBlockingFindingLine(t, runPaths, "correctness", "missing preconditions")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, "crf_999", "--reason", "ok", "--by", "dave"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("resolve with unknown finding ID should fail; got exit 0")
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Fatalf("error must mention not found, got:\n%s", stderr.String())
	}
}

// TestContractReviewFindingResolveMissingFindingsFile checks that resolve fails
// when findings.jsonl has not been written yet (fail-closed).
func TestContractReviewFindingResolveMissingFindingsFile(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, "crf_001", "--reason", "ok", "--by", "eve"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("resolve without findings.jsonl should fail; got exit 0")
	}
}

// TestContractReviewFindingResolveNonBlockingFindingError checks that resolving
// a non-blocking finding is rejected with a useful error.
func TestContractReviewFindingResolveNonBlockingFindingError(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	advisory := contractReviewFindingLine{
		ID:          nextContractReviewFindingID(1),
		Fingerprint: canonicalBlockerKey("style", "verbose prose"),
		Category:    "style",
		Message:     "verbose prose",
		Blocking:    false,
	}
	data, err := json.Marshal(advisory)
	assertNoError(t, err)
	assertNoError(t, activeStore.WriteBytes(runPaths.ContractReviewFindingsJSONL, append(data, '\n'), 0o644))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, advisory.ID, "--reason", "fine", "--by", "frank"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("resolve of non-blocking finding should fail; got exit 0")
	}
	if !strings.Contains(stderr.String(), "not a blocking") {
		t.Fatalf("error must mention 'not a blocking', got:\n%s", stderr.String())
	}
}

// TestContractReviewResolutionReusedByFingerprint checks that a resolution
// recorded for one finding ID covers a re-emitted finding with the same
// fingerprint (different ID, same category+message → same fingerprint).
func TestContractReviewResolutionReusedByFingerprint(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	// First finding with ID crf_001.
	line1 := writeBlockingFindingLine(t, runPaths, "correctness", "validation commands missing")

	// Resolve by the first ID.
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, line1.ID, "--reason", "exempted", "--by", "grace"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("resolve exited %d: %s", code, stderr.String())
	}

	// Overwrite findings.jsonl with the same blocker but a new ID (simulates
	// a reviewer re-emitting the same issue in a later round).
	line2 := contractReviewFindingLine{
		ID:          nextContractReviewFindingID(2),
		Fingerprint: canonicalBlockerKey("correctness", "validation commands missing"),
		Category:    "correctness",
		Message:     "validation commands missing",
		Blocking:    true,
	}
	data, err := json.Marshal(line2)
	assertNoError(t, err)
	assertNoError(t, activeStore.WriteBytes(runPaths.ContractReviewFindingsJSONL, append(data, '\n'), 0o644))

	// Approve must pass because the fingerprint matches an active resolution.
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve should pass when fingerprint-matched resolution exists; got exit %d\nstderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "WARNING") {
		t.Fatalf("approve output must print WARNING for waived finding, got:\n%s", stdout.String())
	}
}

// TestContractReviewApproveMalformedResolutionsFailClosed checks that a
// malformed resolutions.jsonl causes approve to fail (fail-closed).
func TestContractReviewApproveMalformedResolutionsFailClosed(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	writeBlockingFindingLine(t, runPaths, "completeness", "ACs not measurable")
	assertNoError(t, activeStore.WriteBytes(runPaths.ContractReviewResolutionsJSONL, []byte("not-valid-json\n"), 0o644))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("contract approve must fail when resolutions.jsonl is malformed; got exit 0\nstdout: %s", stdout.String())
	}
}

// TestContractReviewApproveWaiverSummaryDedupesByFingerprint checks that two
// findings with the same fingerprint produce only one waiver entry.
func TestContractReviewApproveWaiverSummaryDedupesByFingerprint(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	fp := canonicalBlockerKey("quality", "unclear success metric")
	// Two findings: same fingerprint (same category+message), different IDs.
	line1 := contractReviewFindingLine{
		ID:          nextContractReviewFindingID(1),
		Fingerprint: fp,
		Category:    "quality",
		Message:     "unclear success metric",
		Blocking:    true,
	}
	line2 := contractReviewFindingLine{
		ID:          nextContractReviewFindingID(2),
		Fingerprint: fp,
		Category:    "quality",
		Message:     "unclear success metric",
		Blocking:    true,
	}
	d1, _ := json.Marshal(line1)
	d2, _ := json.Marshal(line2)
	assertNoError(t, activeStore.WriteBytes(runPaths.ContractReviewFindingsJSONL, append(append(d1, '\n'), append(d2, '\n')...), 0o644))

	// Resolve the first finding.
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, line1.ID, "--reason", "accepted", "--by", "heidi"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("resolve exited %d: %s", code, stderr.String())
	}

	// Approve must pass (both findings share the same fingerprint → both waived).
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve should pass; got exit %d\nstderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}

	// Waiver summary must appear with exactly one entry (deduplicated by fingerprint).
	out := stdout.String()
	if !strings.Contains(out, "WARNING: 1 ") {
		t.Fatalf("waiver summary must report exactly 1 deduplicated entry (not 2), got:\n%s", out)
	}
}

// TestContractReviewFindingResolveIDPrefix checks the CLI validation that
// non-crf_ prefixed IDs are rejected at the CLI layer.
func TestContractReviewFindingResolveIDPrefix(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setContractReviewersConfig(t, paths, "helper")
	registerTestAgents(t, paths, "helper")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "finding", "resolve", runID, "f_001", "--reason", "ok", "--by", "ivan"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("resolve with wrong-prefix ID should fail; got exit 0")
	}
	if !strings.Contains(stderr.String(), "crf_") {
		t.Fatalf("error must mention expected crf_ prefix, got:\n%s", stderr.String())
	}
}

// TestContractReviewFindingsJSONLHasIDAndFingerprint verifies that running the
// contract review loop writes findings.jsonl entries with non-empty id and
// fingerprint fields.
func TestContractReviewFindingsJSONLHasIDAndFingerprint(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	app = configureHelperContractReviewers(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_CONTRACT_REVIEWER_EMIT_FINDINGS", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "review", "run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract review run exited %d, stderr: %s", code, stderr.String())
	}

	lines, err := readJSONLines[contractReviewFindingLine](runPaths.ContractReviewFindingsJSONL)
	assertNoError(t, err)
	if len(lines) == 0 {
		t.Fatal("findings.jsonl must have entries when reviewer emits findings")
	}
	for _, l := range lines {
		if l.ID == "" {
			t.Fatalf("findings.jsonl entry missing id: %#v", l)
		}
		if !strings.HasPrefix(l.ID, "crf_") {
			t.Fatalf("findings.jsonl entry id must start with crf_: %s", l.ID)
		}
		if l.Fingerprint == "" {
			t.Fatalf("findings.jsonl entry missing fingerprint: %#v", l)
		}
	}
}
