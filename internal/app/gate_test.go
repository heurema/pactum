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

func TestGateRunRefusesWithoutAllowCommands(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupGatePreparedRun(t, root, []string{gateValidationCommandForTest()}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("gate run without allow exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "refusing to run validation commands without --allow-commands") {
		t.Fatalf("refusal stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.GateReportJSON)
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
	if report.Status != "passed" || report.Changes.Status != "clean" || len(report.Validation.Commands) != 0 || report.Validation.CommandsAllowed {
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
	code := app.Run([]string{"gate", "run", runID, "--allow-commands"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	report := readGateReport(t, runPaths.GateReportJSON)
	if report.Status != "passed" || len(report.Validation.Commands) != 1 || report.Validation.Commands[0].ExitCode != 0 {
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
	code := app.Run([]string{"gate", "run", runID, "--allow-commands"}, &stdout, &stderr)
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
	code := app.Run([]string{"gate", "run", runID, "--allow-commands"}, &stdout, &stderr)
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
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, []string{"--add-path-in-scope", "internal/app/**"}, nil, true)
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
	app, _, runID := setupGatePreparedRunWithRevision(t, root, []string{"--add-path-in-scope", "internal/app/**"}, nil, true)
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
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, []string{"--add-path-in-scope", "internal/app/**", "--add-path-out-of-scope", "README.md"}, nil, true)
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
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, []string{"--add-path-in-scope", "internal/app/**"}, nil, true)
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
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, []string{"--add-path-in-scope", "internal/app/**"}, nil, true)

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
	app, paths, runID := setupGatePreparedRunWithRevision(t, root, []string{"--add-path-out-of-scope", "README.md"}, nil, true)
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
	code := app.Run([]string{"gate", "run", runID, "--allow-commands"}, &stdout, &stderr)
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

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--add-in-scope", "Update gate contract"}, &stdout, &stderr)
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
	if got := stdout.String(); !strings.Contains(got, "Gate report has not been created. Run: pactum gate run "+runID+" --allow-commands") {
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
	code := app.Run([]string{"gate", "run", runID, "--allow-commands"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	assertDoesNotContainRoot(t, "gate-report.json", mustReadFile(t, runPaths.GateReportJSON), root)
	assertDoesNotContainRoot(t, "validation result.json", mustReadFile(t, filepath.Join(runPaths.GateValidationDir, "command_001", "result.json")), root)
}

func TestGateValidationCommandParsingUsesWhitespaceFields(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PACTUM_GATE_HELPER_PROCESS", "1")
	command := gateValidationCommandForTest() + ` "two words"`
	app, paths, runID := setupGatePreparedRun(t, root, []string{command}, true)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"gate", "run", runID, "--allow-commands"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gate run exited %d, stderr: %s", code, stderr.String())
	}
	got := mustReadFile(t, filepath.Join(runPaths.GateValidationDir, "command_001", "stdout.log"))
	for _, want := range []string{`arg="two`, `arg=words"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("whitespace parsing output missing %q:\n%s", want, got)
		}
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
	return setupGatePreparedRunWithRevision(t, root, nil, validationCommands, execute)
}

func setupGatePreparedRunWithRevision(t *testing.T, root string, reviseFlags []string, validationCommands []string, execute bool) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupContractRun(t, root)
	var stdout, stderr bytes.Buffer

	if len(validationCommands) > 0 || len(reviseFlags) > 0 {
		args := []string{"contract", "revise", runID, "--goal", "add deterministic gate"}
		for _, command := range validationCommands {
			args = append(args, "--add-validation", command)
		}
		args = append(args, reviseFlags...)
		code := app.Run(args, &stdout, &stderr)
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
		{"map", "refresh"},
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
		code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--yes"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
		}
	}
	return app, paths, runID
}

func setGateScopeEnforcementConfig(t *testing.T, paths artifacts.Paths, enforcement string) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	config.Gate.ScopeEnforcement = enforcement
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
