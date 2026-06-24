package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
)

func TestGateRunBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"gate", "run", "run_x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("gate run before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("gate run before init stderr mismatch:\n%s", got)
	}
}

func TestGateRunMissingRunReturnsError(t *testing.T) {
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
	code = app.Run([]string{"gate", "run", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("gate run missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestGateRunWithoutExecutionAttemptsFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupGatePreparedRun(t, root, nil, false)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("gate run should fail without completed execution attempts")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot run gate: no completed execution attempts found") {
		t.Fatalf("missing attempts stderr mismatch:\n%s", got)
	}
}

func TestGateRunSucceedsWithNoValidationCommands(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	// commands_allowed stays true even with zero validation commands: the gate
	// always runs whatever the contract declares.
	if report.Status != "passed" || report.Changes.Status != "clean" || len(report.Validation.Commands) != 0 || !report.Validation.CommandsAllowed {
		t.Fatalf("unexpected gate report: %#v", report)
	}
	if got := stdout.String(); !strings.Contains(got, "Gate report created") || !strings.Contains(got, "status: passed") {
		t.Fatalf("gate run output mismatch:\n%s", got)
	}
}

func TestGateRunExecutesValidationCommand(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PACTUM_GATE_HELPER_PROCESS", "1")
	app, paths, runID := setupGatePreparedRun(t, root, []string{gateValidationCommandForTest()}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if report.Status != "passed" || !report.Validation.CommandsAllowed || len(report.Validation.Commands) != 1 || report.Validation.Commands[0].ExitCode != 0 {
		t.Fatalf("unexpected gate report: %#v", report)
	}
	commandDir := filepath.Join(runPaths.GateValidationDir, "command_001")
	assertFile(t, filepath.Join(commandDir, "stdout.log"))
	assertFile(t, filepath.Join(commandDir, "stderr.log"))
	assertFile(t, filepath.Join(commandDir, "result.json"))
	if got := mustReadFile(t, filepath.Join(commandDir, "stdout.log")); !strings.Contains(got, "validation-stdout") {
		t.Fatalf("stdout log mismatch:\n%s", got)
	}
}

func TestGateRunValidationFailureWritesReportAndReturnsNonZero(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PACTUM_GATE_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_GATE_HELPER_EXIT", "7")
	app, paths, runID := setupGatePreparedRun(t, root, []string{gateValidationCommandForTest()}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("gate run should return non-zero for validation failure")
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if report.Status != "failed" || len(report.Validation.Commands) != 1 || report.Validation.Commands[0].ExitCode != 7 {
		t.Fatalf("unexpected failing gate report: %#v", report)
	}
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if indexOfEvent(eventTypes, "gate_run_finished") == -1 {
		t.Fatalf("events missing gate_run_finished:\n%v", eventTypes)
	}
}

func TestGateRunDetectsChangedFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PACTUM_GATE_HELPER_PROCESS", "1")
	app, paths, runID := setupGatePreparedRun(t, root, []string{gateValidationCommandForTest()}, true)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run changed file exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if report.Status != "needs_review" || !containsString(report.Changes.ChangedFiles, "README.md") {
		t.Fatalf("changed file not reported: %#v", report.Changes)
	}
}

func TestGateRunDetectsNewFile(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	mustWriteFile(t, filepath.Join(root, "new.go"), "package newfile\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run new file exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if report.Status != "needs_review" || !containsString(report.Changes.NewFiles, "new.go") {
		t.Fatalf("new file not reported: %#v", report.Changes)
	}
}

func TestGateRunBlocksUndeclaredPathScopeByDefault(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, map[string]any{"goal": "add deterministic gate", "paths_in_scope": []string{"internal/app/**"}}, true)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")
	mustWriteFile(t, filepath.Join(root, "notes.txt"), "outside declared paths\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("gate run with blocked scope should fail")
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if report.Status != "failed" || report.Scope == nil || report.Scope.Status != "blocked" || !report.Summary.ScopeBlocked {
		t.Fatalf("blocked scope not reported: %#v", report)
	}
	if !containsString(report.Scope.Undeclared, "README.md") || !containsString(report.Scope.Undeclared, "notes.txt") {
		t.Fatalf("undeclared files not reported: %#v", report.Scope)
	}
	if len(report.Scope.OutOfScope) != 0 {
		t.Fatalf("unexpected out-of-scope files: %#v", report.Scope.OutOfScope)
	}
	for _, want := range []string{"Scope:", "status: blocked", "Violations:", "undeclared file: README.md", "undeclared file: notes.txt", "Gate:", "status: failed"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("gate run output missing %q:\n%s", want, stdout.String())
		}
	}
	if got := stderr.String(); !strings.Contains(got, "gate status failed") {
		t.Fatalf("blocked scope stderr mismatch:\n%s", got)
	}
}

func TestGateRunBlockedScopeJSONOutputIsGateReport(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupGatePreparedRunWithRevision(t, root, map[string]any{"goal": "add deterministic gate", "paths_in_scope": []string{"internal/app/**"}}, true)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID, "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("gate run with blocked scope should fail")
	}
	if stderr.Len() != 0 {
		t.Fatalf("gate run --json should keep stderr empty, got:\n%s", stderr.String())
	}
	var report gateReportDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &report))
	if report.Schema != gateReportSchema || report.Status != "failed" || report.Scope == nil || report.Scope.Status != "blocked" {
		t.Fatalf("blocked scope JSON report mismatch: %#v", report)
	}
}

func TestGateRunWarnScopeEnforcementKeepsScopeAdvisory(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, map[string]any{"goal": "add deterministic gate", "paths_in_scope": []string{"internal/app/**"}, "paths_out_of_scope": []string{"README.md"}}, true)
	setGateScopeEnforcementConfig(t, paths, gateScopeEnforcementWarn)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run with warning scope enforcement exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if report.Status != "needs_review" || report.Scope == nil || report.Scope.Status != "warnings" || report.Summary.ScopeBlocked {
		t.Fatalf("warning scope should not fail gate: %#v", report)
	}
	if !containsString(report.Scope.Undeclared, "README.md") || !containsString(report.Scope.OutOfScope, "README.md") {
		t.Fatalf("warning scope files not reported: %#v", report.Scope)
	}
	if got := stdout.String(); !strings.Contains(got, "Warnings:") || strings.Contains(got, "Violations:") {
		t.Fatalf("warning scope output mismatch:\n%s", got)
	}
}

func TestGateRunScopeCleanWhenFilesAreDeclared(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, map[string]any{"goal": "add deterministic gate", "paths_in_scope": []string{"internal/app/**"}}, true)
	mustWriteFile(t, filepath.Join(root, "internal", "app", "new.go"), "package app\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run with clean path scope exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if report.Status != "needs_review" || report.Scope == nil || report.Scope.Status != "clean" {
		t.Fatalf("scope clean report mismatch: %#v", report)
	}
	if len(report.Scope.Undeclared) != 0 || len(report.Scope.OutOfScope) != 0 || len(report.Scope.Warnings) != 0 {
		t.Fatalf("clean scope should have no warnings: %#v", report.Scope)
	}
	if got := stdout.String(); !strings.Contains(got, "Scope:") || !strings.Contains(got, "status: clean") || strings.Contains(got, "Warnings:") {
		t.Fatalf("gate run clean scope output mismatch:\n%s", got)
	}
}

func TestGateRunPassesWithPathGlobsAndNoChanges(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, map[string]any{"goal": "add deterministic gate", "paths_in_scope": []string{"internal/app/**"}}, true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run with clean path scope exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if report.Status != "passed" || report.Scope == nil || report.Scope.Status != "clean" || report.Summary.ScopeBlocked {
		t.Fatalf("clean scoped gate should pass: %#v", report)
	}
}

func TestGateRunBlocksOutOfScopePathByDefault(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, map[string]any{"goal": "add deterministic gate", "paths_out_of_scope": []string{"README.md"}}, true)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("gate run with out-of-scope path should fail")
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if report.Status != "failed" || report.Scope == nil || report.Scope.Status != "blocked" || !containsString(report.Scope.OutOfScope, "README.md") {
		t.Fatalf("out-of-scope file not reported: %#v", report.Scope)
	}
	if len(report.Scope.Undeclared) != 0 {
		t.Fatalf("undeclared should not be populated without paths_in_scope: %#v", report.Scope.Undeclared)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"gate", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate show exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "out-of-scope file: README.md") || !strings.Contains(got, "Violations:") || !strings.Contains(got, "status: blocked") {
		t.Fatalf("gate show scope output mismatch:\n%s", got)
	}
}

func TestGateRunOmitsScopeSectionWithoutPathGlobs(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run without path globs exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if report.Status != "needs_review" || report.Scope != nil {
		t.Fatalf("scope report should be omitted without path globs: %#v", report)
	}
	if report.Summary.ScopeBlocked {
		t.Fatalf("scope_blocked should be false without path globs: %#v", report.Summary)
	}
	if strings.Contains(mustReadFile(t, runPaths.GateReportJSON), `"scope"`) {
		t.Fatalf("gate report JSON should omit scope without path globs:\n%s", mustReadFile(t, runPaths.GateReportJSON))
	}
	if strings.Contains(stdout.String(), "Scope:") || strings.Contains(stdout.String(), "Warnings:") {
		t.Fatalf("gate run output should omit scope without path globs:\n%s", stdout.String())
	}
}

func TestGateRunDetectsValidationCommandChanges(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PACTUM_GATE_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_GATE_HELPER_WRITE", "generated.go")
	app, paths, runID := setupGatePreparedRun(t, root, []string{gateValidationCommandForTest()}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run with validation write exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if report.Status != "needs_review" || !containsString(report.Changes.NewFiles, "generated.go") {
		t.Fatalf("validation-created file not reported: %#v", report)
	}
}

func TestGateRunDetectsMissingFile(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	assertNoError(t, os.Remove(filepath.Join(root, "README.md")))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run missing file exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if report.Status != "needs_review" || !containsString(report.Changes.MissingFiles, "README.md") {
		t.Fatalf("missing file not reported: %#v", report.Changes)
	}
}

func TestGateRunRejectsAttemptFromPreviousApprovedContract(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"scope": map[string]any{"in": []string{"Update gate contract"}},
	})
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile, "--allow-approval-reset"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("gate run should reject attempt from previous approved contract")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot run gate: no completed execution attempts found for current approved contract") {
		t.Fatalf("stale attempt stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.GateReportJSON)
}

func TestGateShowBeforeReportPrintsGuidance(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupGatePreparedRun(t, root, nil, true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate show before report exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Gate report has not been created. Run: pactum gate run "+runID) {
		t.Fatalf("gate show guidance mismatch:\n%s", got)
	}
}

func TestGateShowAfterReportPrintsStatusAndSummary(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupGatePreparedRun(t, root, nil, true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"gate", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate show exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"Gate report", "status: passed", "exit code: 0", "commands: 0", "status: clean"} {
		if !strings.Contains(got, want) {
			t.Fatalf("gate show output missing %q:\n%s", want, got)
		}
	}
}

func TestGateShowJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupGatePreparedRun(t, root, nil, true)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"gate", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate show --json exited %d, stderr: %s", code, stderr.String())
	}
	var report gateReportDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &report))
	if report.Schema != gateReportSchema || report.Status != "passed" {
		t.Fatalf("unexpected gate show json: %#v", report)
	}
}

func TestGateShowDoesNotMutateArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	beforeReport := mustReadFile(t, runPaths.GateReportJSON)
	beforeRun := mustReadFile(t, runPaths.RunJSON)
	beforeLedger := mustReadFile(t, paths.EventsJSONL)

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"gate", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate show exited %d, stderr: %s", code, stderr.String())
	}
	if got := mustReadFile(t, runPaths.GateReportJSON); got != beforeReport {
		t.Fatalf("gate show mutated gate report")
	}
	if got := mustReadFile(t, runPaths.RunJSON); got != beforeRun {
		t.Fatalf("gate show mutated run.json")
	}
	if got := mustReadFile(t, paths.EventsJSONL); got != beforeLedger {
		t.Fatalf("gate show mutated ledger")
	}
}

func TestGateReportPathsArePortable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PACTUM_GATE_HELPER_PROCESS", "1")
	app, paths, runID := setupGatePreparedRun(t, root, []string{gateValidationCommandForTest()}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	assertDoesNotContainRoot(t, "gate-report.json", mustReadFile(t, runPaths.GateReportJSON), root)
	assertDoesNotContainRoot(t, "validation result.json", mustReadFile(t, filepath.Join(runPaths.GateValidationDir, "command_001", "result.json")), root)
}

func TestGateValidationCommandParsingPreservesQuotedArguments(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PACTUM_GATE_HELPER_PROCESS", "1")
	command := gateValidationCommandForTest() + ` "two words"`
	app, paths, runID := setupGatePreparedRun(t, root, []string{command}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	got := mustReadFile(t, filepath.Join(runPaths.GateValidationDir, "command_001", "stdout.log"))
	if !strings.Contains(got, "arg=two words") {
		t.Fatalf("quoted argument not preserved as single arg:\n%s", got)
	}
}

func TestGateValidationCommandRunsThroughShell(t *testing.T) {
	root := t.TempDir()
	// Command substitution proves the gate runs commands through sh, not exec directly.
	app, paths, runID := setupGatePreparedRun(t, root, []string{`echo "shell-feature-$(echo ok)"`}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if len(report.Validation.Commands) != 1 || report.Validation.Commands[0].ExitCode != 0 {
		t.Fatalf("unexpected validation result: %#v", report.Validation)
	}
	commandStdout := mustReadFile(t, filepath.Join(runPaths.GateValidationDir, "command_001", "stdout.log"))
	if !strings.Contains(commandStdout, "shell-feature-ok") {
		t.Fatalf("shell command substitution not demonstrated in stdout:\n%s", commandStdout)
	}
}

func TestGateRunUsesLatestCompletedAttempt(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertNoError(t, os.MkdirAll(filepath.Join(runPaths.AttemptsDir, "attempt_002"), 0o755))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if report.Execution.AttemptID != "attempt_001" || report.Execution.Result != "execute/last-result.json" {
		t.Fatalf("gate should use latest completed attempt_001, got %#v", report.Execution)
	}
}

func setupGatePreparedRun(t *testing.T, root string, validationCommands []string, execute bool) (App, artifacts.Paths, string) {
	t.Helper()
	var contractUpdate map[string]any
	if len(validationCommands) > 0 {
		contractUpdate = map[string]any{
			"goal":       "add deterministic gate",
			"validation": map[string]any{"commands": validationCommands},
		}
	}
	return setupGatePreparedRunWithRevision(t, root, contractUpdate, execute)
}

func setupGatePreparedRunWithRevision(t *testing.T, root string, contractUpdate map[string]any, execute bool) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupContractRun(t, root)
	var stdout, stderr bytes.Buffer

	if contractUpdate != nil {
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		fromFile := writeReviseDocForTest(t, runPaths, contractUpdate)
		code := app.Run([]string{"contract", "revise", runID, "--from", fromFile}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
		}
		stdout.Reset()
		stderr.Reset()
	}

	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
	app = configureHelperAgent(app, "helper")
	registerTestAgents(t, paths, "helper")

	for _, args := range [][]string{
		{"prompt", "build", runID},
	} {
		stdout.Reset()
		stderr.Reset()
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}
	if execute {
		t.Setenv("PACTUM_HELPER_PROCESS", "1")
		t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)
		stdout.Reset()
		stderr.Reset()
		code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
		}
	}
	return app, paths, runID
}

func setGateScopeEnforcementConfig(t *testing.T, paths artifacts.Paths, enforcement string) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	config.OutOfScope = enforcement
	assertNoError(t, writeYAML(paths.Config, config))
}

func gateValidationCommandForTest() string {
	return os.Args[0] + " -test.run=TestGateValidationHelperProcess"
}

func readGateReport(t *testing.T, path string) gateReportDocument {
	t.Helper()
	var report gateReportDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &report))
	return report
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestGateValidationHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_GATE_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "validation-stdout")
	fmt.Fprintln(os.Stderr, "validation-stderr")
	for _, arg := range os.Args[1:] {
		fmt.Fprintf(os.Stdout, "arg=%s\n", arg)
	}
	if path := os.Getenv("PACTUM_GATE_HELPER_WRITE"); path != "" {
		if err := os.WriteFile(path, []byte("package generated\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
			os.Exit(2)
		}
	}
	if raw := os.Getenv("PACTUM_GATE_HELPER_EXIT"); raw != "" {
		code, err := strconv.Atoi(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad exit code: %v\n", err)
			os.Exit(2)
		}
		os.Exit(code)
	}
	os.Exit(0)
}

// ---------------------------------------------------------------------------
// Git-guard lifecycle integration tests
// ---------------------------------------------------------------------------

// TestGitGuardLifecycle_PreconditionBlocksTransport verifies that a dirty
// working tree detected before execute prevents the agent transport from
// running and records executor_git_guard_inconclusive in the attempt result.
func TestGitGuardLifecycle_PreconditionBlocksTransport(t *testing.T) {
	skipIfNoGit(t)

	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, false)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Create an untracked file so the git guard clean-tree precondition fails.
	mustWriteFile(t, filepath.Join(root, "untracked.go"), "package main\n")

	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("execute run should fail when git guard precondition fails")
	}

	// Transport must not have been called.
	if len(transport.requests) != 0 {
		t.Errorf("transport should not be called when precondition fails; got %d calls", len(transport.requests))
	}

	// Attempt result must expose the git-guard terminal reason.
	attemptPaths := executionAttemptPaths(runPaths, "attempt_001")
	var result executionResultDocument
	assertNoError(t, readJSON(attemptPaths.ResultJSON, &result))
	if result.GitGuard == nil || result.GitGuard.TerminalReason != gitGuardReasonInconclusive {
		t.Errorf("expected git_guard.terminal_reason=%q in result; got %#v", gitGuardReasonInconclusive, result.GitGuard)
	}
}

// TestGitGuardLifecycle_GateBlockedByGuardedAttempt verifies that the gate
// refuses to proceed when the latest matching execute attempt has a non-empty
// git-guard terminal_reason and does NOT fall back to an older clean attempt.
func TestGitGuardLifecycle_GateBlockedByGuardedAttempt(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Mark the existing (clean) attempt as git-guard-blocked.
	attempt1ResultPath := filepath.Join(runPaths.AttemptsDir, "attempt_001", "result.json")
	markGitGuardBlocked := func(path string) {
		var doc map[string]any
		assertNoError(t, readJSON(path, &doc))
		doc["git_guard"] = map[string]any{"terminal_reason": gitGuardReasonHistoryMutation}
		assertNoError(t, writeJSON(path, doc))
	}
	markGitGuardBlocked(attempt1ResultPath)
	markGitGuardBlocked(runPaths.LastResultJSON)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("gate run should fail when the latest attempt is git-guard-blocked")
	}
	if got := stderr.String(); !strings.Contains(got, "no completed execution attempts found") {
		t.Errorf("expected 'no completed execution attempts found' in gate stderr, got: %s", got)
	}
}

// TestGitGuardLifecycle_ReadOnlyStageSkipsGuard verifies that a read-only
// stage (review run) is not affected by the git-history guard even when
// executed inside a real git repository with a dirty working tree.
func TestGitGuardLifecycle_ReadOnlyStageSkipsGuard(t *testing.T) {
	skipIfNoGit(t)

	// Use a real git repo. setupApprovedPreparedReview writes README.md (and
	// other files) that remain untracked after the initial commit — the same
	// dirty-tree state that would block an execute stage. Review must still
	// call the transport.
	root := initTestRepo(t)
	app, _, runID, _ := setupApprovedPreparedReview(t, root, "passed")

	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run (read-only) exited %d, stderr: %s", code, stderr.String())
	}

	// Transport must be called — the git guard must not interfere with read-only stages.
	if len(transport.requests) == 0 {
		t.Error("read-only review stage should call the transport without git-guard interference")
	}
}

// TestExecuteRunUnbornHeadGitGuard verifies that execute run fails clearly when
// the repository has no commits (unborn HEAD), with the git-guard terminal
// reason and an actionable remedy in the attempt result and human output.
func TestExecuteRunUnbornHeadGitGuard(t *testing.T) {
	skipIfNoGit(t)
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, false)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Transition to an orphan branch (unborn HEAD) while keeping working tree
	// and pactum artifacts intact.
	mustGitG(t, root, "checkout", "--orphan", "no-commits-branch")

	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("execute run should fail in a zero-commit (unborn HEAD) repository")
	}

	// Transport must not be called — the git guard blocks execution.
	if len(transport.requests) != 0 {
		t.Errorf("transport must not be called for unborn HEAD; got %d calls", len(transport.requests))
	}

	// Attempt result must carry the unborn-HEAD git-guard terminal reason with a remedy.
	attemptPaths := executionAttemptPaths(runPaths, "attempt_001")
	var result executionResultDocument
	assertNoError(t, readJSON(attemptPaths.ResultJSON, &result))
	if result.GitGuard == nil {
		t.Fatal("expected git_guard section in attempt result")
	}
	if result.GitGuard.TerminalReason != gitGuardReasonUnbornHead {
		t.Errorf("git_guard.terminal_reason: want %q, got %q", gitGuardReasonUnbornHead, result.GitGuard.TerminalReason)
	}
	if !strings.Contains(result.GitGuard.Detail, "no commits") {
		t.Errorf("git_guard.detail should mention no commits; got: %q", result.GitGuard.Detail)
	}
	if !strings.Contains(result.GitGuard.Detail, "git add") {
		t.Errorf("git_guard.detail should include a remedy; got: %q", result.GitGuard.Detail)
	}

	// Human-readable output must include the remedy text directly.
	if !strings.Contains(stdout.String(), "git add") {
		t.Errorf("human output should include remedy; got: %s", stdout.String())
	}
}

// TestReviewFixRunUnbornHeadGitGuard verifies that review fix run fails clearly
// when the repository has no commits (unborn HEAD), with the git-guard terminal
// reason and remedy in the attempt result.
func TestReviewFixRunUnbornHeadGitGuard(t *testing.T) {
	skipIfNoGit(t)
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	// At least one finding is required for review fix run to proceed.
	runReviewCommand(t, app, "review", "finding", "add", runID, "test finding", "--blocking", "--category", "quality")

	// Transition to an orphan branch (unborn HEAD) while keeping working tree
	// and pactum artifacts intact.
	mustGitG(t, root, "checkout", "--orphan", "no-commits-branch")

	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("review fix run should fail in a zero-commit (unborn HEAD) repository")
	}

	// Transport must not be called — the git guard blocks execution.
	if len(transport.requests) != 0 {
		t.Errorf("transport must not be called for unborn HEAD; got %d calls", len(transport.requests))
	}

	// Attempt result must carry the unborn-HEAD git-guard terminal reason with a remedy.
	fixPaths := reviewFixAttemptPaths(runPaths, "attempt_001")
	var result reviewFixResultDocument
	assertNoError(t, readJSON(fixPaths.ResultJSON, &result))
	if result.GitGuard == nil {
		t.Fatal("expected git_guard section in attempt result")
	}
	if result.GitGuard.TerminalReason != gitGuardReasonUnbornHead {
		t.Errorf("git_guard.terminal_reason: want %q, got %q", gitGuardReasonUnbornHead, result.GitGuard.TerminalReason)
	}
	if !strings.Contains(result.GitGuard.Detail, "no commits") {
		t.Errorf("git_guard.detail should mention no commits; got: %q", result.GitGuard.Detail)
	}
	if !strings.Contains(result.GitGuard.Detail, "git add") {
		t.Errorf("git_guard.detail should include a remedy; got: %q", result.GitGuard.Detail)
	}

	// Human-readable output must include the remedy text directly.
	if !strings.Contains(stdout.String(), "git add") {
		t.Errorf("human output should include remedy; got: %s", stdout.String())
	}
}

// TestGateRunPassesCompletedDespiteTimeoutAttempt pins the end-to-end story of
// completion-aware finalize: an idle-killed attempt whose agent had already
// completed (exit 0, timed_out true, completed_despite_timeout true) must pass
// the gate's execution check — otherwise the success path dies one pipeline
// step later and the feature means nothing.
// TestGateRunBlocksOutOfScopeMissingFile verifies that a file deleted by the
// executor (present in missing_files) is subject to scope enforcement the same
// as modified or new files — deleting an out-of-scope file must block the gate.
func TestGateRunBlocksOutOfScopeMissingFile(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, map[string]any{
		"goal":               "add deterministic gate",
		"paths_out_of_scope": []string{"README.md"},
	}, true)

	// Stage-delete README.md to simulate the executor deleting an out-of-scope file.
	mustGitG(t, root, "rm", "README.md")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("gate run should fail when a missing file is out of scope")
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if report.Status != "failed" {
		t.Fatalf("expected status=failed, got %q", report.Status)
	}
	if report.Scope == nil || report.Scope.Status != "blocked" {
		t.Fatalf("expected scope.status=blocked: %#v", report.Scope)
	}
	if !containsString(report.Changes.MissingFiles, "README.md") {
		t.Fatalf("README.md should be in missing_files: %#v", report.Changes)
	}
	if !containsString(report.Scope.OutOfScope, "README.md") {
		t.Fatalf("README.md should be in scope.out_of_scope: %#v", report.Scope)
	}
}

// TestGateChangeReportExactlyOneBucket verifies the exactly-one-bucket
// invariant: when a staged deletion and an untracked file exist at the same
// path, git may emit two porcelain entries ("D  a.txt" and "?? a.txt"); the
// intermediate-map classifier must place a.txt in exactly one bucket, with
// missing_files winning over new_files (D > ??).
func TestGateChangeReportExactlyOneBucket(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t) // commits a.txt

	// Stage-delete a.txt then recreate it as an untracked file.  git status
	// --porcelain=v1 -z may emit both "D  a.txt" and "?? a.txt".
	mustGitG(t, root, "rm", "a.txt")
	mustWriteFile(t, filepath.Join(root, "a.txt"), "new content\n")

	report := testApp(root).buildGateChangeReport(root)
	if report.Status != "changed" {
		t.Fatalf("expected status=changed, got %q", report.Status)
	}

	inMissing := containsString(report.MissingFiles, "a.txt")
	inChanged := containsString(report.ChangedFiles, "a.txt")
	inNew := containsString(report.NewFiles, "a.txt")

	count := 0
	if inMissing {
		count++
	}
	if inChanged {
		count++
	}
	if inNew {
		count++
	}
	if count != 1 {
		t.Fatalf("a.txt must appear in exactly one bucket; missing=%v changed=%v new=%v", inMissing, inChanged, inNew)
	}
	if !inMissing {
		t.Fatalf("staged-deleted a.txt must be in missing_files (D > ??); missing=%v changed=%v new=%v", inMissing, inChanged, inNew)
	}
}

// TestGateRunDetectsRenamedAndModifiedFile verifies that when a tracked file is
// renamed and the destination is then modified (git status: "RM dest\0orig\0"),
// the classifier puts dest in changed_files (not new_files) and orig in
// missing_files — exercising the R/C destination rule for Y=M.
func TestGateRunDetectsRenamedAndModifiedFile(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)

	// Stage the rename, then modify the destination without staging so Y=M.
	assertNoError(t, os.Rename(filepath.Join(root, "README.md"), filepath.Join(root, "new.go")))
	mustGitG(t, root, "add", "-A")
	mustWriteFile(t, filepath.Join(root, "new.go"), "package main\n// extra\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if !containsString(report.Changes.MissingFiles, "README.md") {
		t.Fatalf("rename origin should be in missing_files: %#v", report.Changes)
	}
	if !containsString(report.Changes.ChangedFiles, "new.go") {
		t.Fatalf("renamed+modified dest should be in changed_files (not new_files): %#v", report.Changes)
	}
	if containsString(report.Changes.NewFiles, "new.go") {
		t.Fatalf("renamed+modified dest must not be in new_files: %#v", report.Changes)
	}
}

// TestGateRunDetectsUnmergedFile verifies that a file in unmerged state
// (git status code UU) is classified as changed_files rather than dropped.
func TestGateRunDetectsUnmergedFile(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)

	// Create a UU (both-modified) conflict: two branches each modify README.md
	// on different lines, then merge.
	mainBranch := mustGitG(t, root, "rev-parse", "--abbrev-ref", "HEAD")
	mustGitG(t, root, "checkout", "-b", "feat")
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Feature\n")
	mustGitG(t, root, "add", "README.md")
	mustGitG(t, root, "commit", "-m", "feat change")
	mustGitG(t, root, "checkout", mainBranch)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Main\n")
	mustGitG(t, root, "add", "README.md")
	mustGitG(t, root, "commit", "-m", "main change")
	tryGitG(root, "merge", "feat") // exits non-zero; conflict state is set up

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run with unmerged file exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, contractRunPaths(filepath.Join(paths.RunsDir, runID)).GateReportJSON)
	if !containsString(report.Changes.ChangedFiles, "README.md") {
		t.Fatalf("unmerged file should be in changed_files: %#v", report.Changes)
	}
}

func TestGateRunPassesCompletedDespiteTimeoutAttempt(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, nil, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Rewrite the recorded attempt as completed-despite-timeout.
	markCompletedDespiteTimeout := func(path string) {
		var doc map[string]any
		assertNoError(t, readJSON(path, &doc))
		doc["timed_out"] = true
		doc["completed_despite_timeout"] = true
		doc["exit_code"] = 0
		assertNoError(t, writeJSON(path, doc))
	}
	markCompletedDespiteTimeout(filepath.Join(runPaths.AttemptsDir, "attempt_001", "result.json"))
	markCompletedDespiteTimeout(runPaths.LastResultJSON)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run should pass a completed-despite-timeout attempt, exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if !report.Summary.ExecutionPassed || report.Status != "passed" {
		t.Fatalf("execution should pass: %#v", report.Summary)
	}
	if !report.Execution.CompletedDespiteTimeout || !report.Execution.TimedOut {
		t.Fatalf("execution report should carry the honest pair: %#v", report.Execution)
	}
}

// TestGateChangeReportDetectsNewFileInSubdir verifies that --untracked-files=all
// causes files inside a newly-created directory to appear individually (e.g.
// "sub/new.go") rather than collapsed to a single directory entry ("sub/").
func TestGateChangeReportDetectsNewFileInSubdir(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	subdir := filepath.Join(root, "sub")
	assertNoError(t, os.MkdirAll(subdir, 0o755))
	mustWriteFile(t, filepath.Join(subdir, "new.go"), "package sub\n")

	report := testApp(root).buildGateChangeReport(root)
	if report.Status != "changed" {
		t.Fatalf("expected status=changed, got %q", report.Status)
	}
	if !containsString(report.NewFiles, "sub/new.go") {
		t.Fatalf("expected sub/new.go in new_files, got new_files=%v", report.NewFiles)
	}
	for _, p := range report.NewFiles {
		if p == "sub/" || p == "sub" {
			t.Fatalf("directory entry %q must not appear in new_files — want individual files", p)
		}
	}
}

// TestGateChangeReportGitFailure verifies that when git status fails (e.g. the
// root is not inside a git repository) the change report sets status=changed and
// populates reasons with a description of the failure.
func TestGateChangeReportGitFailure(t *testing.T) {
	skipIfNoGit(t)
	nonGitRoot := t.TempDir() // not a git repo

	report := testApp(nonGitRoot).buildGateChangeReport(nonGitRoot)
	if report.Status != "changed" {
		t.Fatalf("expected status=changed on git failure, got %q", report.Status)
	}
	if len(report.Reasons) == 0 {
		t.Fatal("expected at least one reason on git failure, got none")
	}
	found := false
	for _, r := range report.Reasons {
		if strings.Contains(r, "git status failed") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected reason containing 'git status failed', got %v", report.Reasons)
	}
}

// TestGateChangeReportRenameIntoHeurema verifies that when a tracked file is
// renamed into .heurema/, the original path is placed in missing_files (the
// executor effectively deleted it from the working tree).
func TestGateChangeReportRenameIntoHeurema(t *testing.T) {
	skipIfNoGit(t)
	root := initTestRepo(t)
	// Create the .heurema/ destination directory.
	assertNoError(t, os.MkdirAll(filepath.Join(root, ".heurema"), 0o755))
	// Stage the rename: a.txt → .heurema/a.txt.
	assertNoError(t, os.Rename(filepath.Join(root, "a.txt"), filepath.Join(root, ".heurema", "a.txt")))
	mustGitG(t, root, "add", "-A")

	report := testApp(root).buildGateChangeReport(root)
	if report.Status != "changed" {
		t.Fatalf("expected status=changed, got %q", report.Status)
	}
	if !containsString(report.MissingFiles, "a.txt") {
		t.Fatalf("expected a.txt in missing_files after rename into .heurema/, got missing=%v", report.MissingFiles)
	}
}
