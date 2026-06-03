package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
)

func TestExecuteStatusBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"execute", "status", "run_x"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute status before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("execute status before init output mismatch:\n%s", got)
	}
}

func TestExecuteStatusWithNoAttempts(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "status", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute status exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Execution status",
		"ready: yes",
		"exists: no",
		"count: 0",
		".heurema/pactum/runs/" + runID + "/execute/last-result.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("execute status output missing %q:\n%s", want, got)
		}
	}
}

func TestExecuteStatusAfterHelperRunAndJSON(t *testing.T) {
	root := t.TempDir()
	app, _, runID := runSuccessfulHelperAttempt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "status", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute status exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"count: 1",
		"last: attempt_001",
		"last exit code: 0",
		"last timed out: false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("execute status output missing %q:\n%s", want, got)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "status", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute status --json exited %d, stderr: %s", code, stderr.String())
	}
	var response executeStatusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.RunID != runID || response.RunStatus != "contract_approved" || !response.PromptReady {
		t.Fatalf("unexpected status json: %#v", response)
	}
	if response.Attempts.Count != 1 || response.Attempts.LastAttemptID != "attempt_001" {
		t.Fatalf("unexpected attempts json: %#v", response.Attempts)
	}
	if !response.LastResult.Exists || response.LastResult.ExitCode != 0 || response.LastResult.TimedOut {
		t.Fatalf("unexpected last result json: %#v", response.LastResult)
	}
	assertDoesNotContainRoot(t, "execute status json", stdout.String(), root)
}

func TestExecuteStatusDoesNotPairPreviousResultWithNewerIncompleteAttempt(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := runSuccessfulHelperAttempt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertNoError(t, os.MkdirAll(filepath.Join(runPaths.AttemptsDir, "attempt_002"), 0o755))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "status", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute status exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"count: 2",
		"last: attempt_002",
		"last completed: attempt_001",
		"last completed exit code: 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("execute status output missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"last exit code: 0",
		"last timed out: false",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("execute status output should not pair previous result with attempt_002 via %q:\n%s", forbidden, got)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "status", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute status --json exited %d, stderr: %s", code, stderr.String())
	}
	var response executeStatusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Attempts.Count != 2 || response.Attempts.LastAttemptID != "attempt_002" {
		t.Fatalf("unexpected attempts json: %#v", response.Attempts)
	}
	if !response.LastResult.Exists || response.LastResult.AttemptID != "attempt_001" || response.LastResult.ExitCode != 0 {
		t.Fatalf("unexpected last result json: %#v", response.LastResult)
	}
}

func TestExecuteShowWithNoAttempts(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute show exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "No execution attempts found. Run: pactum execute run "+runID) {
		t.Fatalf("execute show no-attempt output mismatch:\n%s", got)
	}
}

func TestExecuteShowAfterHelperRun(t *testing.T) {
	root := t.TempDir()
	app, _, runID := runSuccessfulHelperAttempt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute show exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Execution attempt",
		"id: attempt_001",
		"exit code: 0",
		"timed out: false",
		".heurema/pactum/runs/" + runID + "/execute/attempts/attempt_001/request.json",
		".heurema/pactum/runs/" + runID + "/execute/attempts/attempt_001/result.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("execute show output missing %q:\n%s", want, got)
		}
	}
}

func TestExecuteShowExplicitAttemptAndMissingAttempt(t *testing.T) {
	root := t.TempDir()
	app, _, runID := runSuccessfulHelperAttempt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "show", runID, "attempt_001"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute show attempt_001 exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "id: attempt_001") {
		t.Fatalf("explicit attempt output mismatch:\n%s", got)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "show", runID, "attempt_999"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute show missing attempt should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "execution attempt not found: attempt_999") {
		t.Fatalf("missing attempt stderr mismatch:\n%s", got)
	}
}

func TestExecuteShowLogsAreBounded(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := runSuccessfulHelperAttempt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	attemptPaths := executionAttemptPaths(runPaths, "attempt_001")
	mustWriteFile(t, attemptPaths.StdoutLog, numberedLines("stdout-line", 120))
	mustWriteFile(t, attemptPaths.StderrLog, numberedLines("stderr-line", 120))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "show", runID, "attempt_001", "--logs"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute show --logs exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Logs:",
		"stdout-line-120",
		"stderr-line-120",
		"stdout truncated",
		"stderr truncated",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("execute show --logs output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "stdout-line-001") || strings.Contains(got, "stderr-line-001") {
		t.Fatalf("logs should be bounded to a trailing excerpt:\n%s", got)
	}
}

func TestExecuteShowJSONIncludesLogsOnlyWhenRequested(t *testing.T) {
	root := t.TempDir()
	app, _, runID := runSuccessfulHelperAttempt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute show --json exited %d, stderr: %s", code, stderr.String())
	}
	var response executeShowResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.RunID != runID || response.AttemptID != "attempt_001" || response.Request["schema"] == nil || response.Result["schema"] == nil {
		t.Fatalf("unexpected show json: %#v", response)
	}
	if response.StdoutExcerpt != nil || response.StderrExcerpt != nil {
		t.Fatalf("log excerpts should be omitted without --logs: %#v", response)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "show", runID, "--json", "--logs"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute show --json --logs exited %d, stderr: %s", code, stderr.String())
	}
	response = executeShowResponse{}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.StdoutExcerpt == nil || !strings.Contains(*response.StdoutExcerpt, "cwd_is_repo=true") {
		t.Fatalf("stdout excerpt missing from json: %#v", response.StdoutExcerpt)
	}
	if response.StderrExcerpt == nil || !strings.Contains(*response.StderrExcerpt, "stderr-line") {
		t.Fatalf("stderr excerpt missing from json: %#v", response.StderrExcerpt)
	}
	assertDoesNotContainRoot(t, "execute show json", stdout.String(), root)
}

func TestExecuteStatusShowReadOnlyRunState(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := runSuccessfulHelperAttempt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runBefore := mustReadFile(t, runPaths.RunJSON)
	eventsBefore := mustReadFile(t, paths.EventsJSONL)

	for _, args := range [][]string{
		{"execute", "status", runID},
		{"execute", "status", runID, "--json"},
		{"execute", "show", runID},
		{"execute", "show", runID, "--json", "--logs"},
	} {
		var stdout, stderr bytes.Buffer
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}

	if runAfter := mustReadFile(t, runPaths.RunJSON); runAfter != runBefore {
		t.Fatalf("reporting commands changed run.json\nbefore:\n%s\nafter:\n%s", runBefore, runAfter)
	}
	if eventsAfter := mustReadFile(t, paths.EventsJSONL); eventsAfter != eventsBefore {
		t.Fatalf("reporting commands changed events.jsonl\nbefore:\n%s\nafter:\n%s", eventsBefore, eventsAfter)
	}
}

func runSuccessfulHelperAttempt(t *testing.T, root string) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}
	return app, paths, runID
}

func numberedLines(prefix string, count int) string {
	var builder strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&builder, "%s-%03d\n", prefix, i)
	}
	return builder.String()
}
