package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/version"
)

// --- Parser errors (Part C) -------------------------------------------------

func TestParserErrorsAreNotSilent(t *testing.T) {
	root := t.TempDir()

	cases := []struct {
		name string
		args []string
	}{
		{"unknown command", []string{"frobnicate"}},
		{"missing required arg", []string{"task", "new"}},
		{"unknown flag", []string{"status", "--nope"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := testApp(root).Run(tc.args, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("%v exited %d, want 2", tc.args, code)
			}
			if stderr.Len() == 0 {
				t.Fatalf("%v produced empty stderr", tc.args)
			}
		})
	}
}

func TestHelpExitsZeroWithUsage(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"--help"}, {"task", "--help"}, {"execute", "run", "--help"}} {
		var stdout, stderr bytes.Buffer
		code := testApp(root).Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, want 0", args, code)
		}
		if !strings.Contains(stdout.String(), "Usage:") {
			t.Fatalf("%v did not print usage:\n%s", args, stdout.String())
		}
	}
}

// --- Exit-code policy (Part D) ----------------------------------------------

func TestReadOnlyCommandsBeforeInitExitZero(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"status"},
		{"task", "list"},
		{"task", "current"},
		{"version"},
	} {
		var stdout, stderr bytes.Buffer
		code := testApp(root).Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v before init exited %d, want 0, stderr: %s", args, code, stderr.String())
		}
	}
}

func TestMutatingCommandBeforeInitExitsOne(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"task", "new", "x"},
		{"contract", "approve"},
		{"prompt", "build"},
		{"memory", "refresh"},
	} {
		var stdout, stderr bytes.Buffer
		code := testApp(root).Run(args, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("%v before init exited %d, want 1", args, code)
		}
		if !strings.Contains(stderr.String(), "not initialized") {
			t.Fatalf("%v stderr mismatch:\n%s", args, stderr.String())
		}
	}
}

// --- Version (Part G) -------------------------------------------------------

func TestVersionHumanAndJSON(t *testing.T) {
	root := t.TempDir() // not initialized: version must not require init

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "version: "+version.Version) {
		t.Fatalf("version human output mismatch:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := testApp(root).Run([]string{"version", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version --json exited %d, stderr: %s", code, stderr.String())
	}
	var info version.Info
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &info))
	if info.Version != version.Version {
		t.Fatalf("version --json = %#v, want version %q", info, version.Version)
	}
}

func TestVersionDoesNotMutateLedger(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	before := mustReadFile(t, paths.EventsJSONL)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version exited %d, stderr: %s", code, stderr.String())
	}
	if after := mustReadFile(t, paths.EventsJSONL); before != after {
		t.Fatalf("version mutated the ledger")
	}
}

func TestWorkspaceManifestVersionMatchesVersionPackage(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	app := testApp(root)
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	manifest, err := readWorkspaceManifest(artifacts.New(root).Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ToolVersion != version.Version {
		t.Fatalf("manifest tool_version = %q, want %q", manifest.ToolVersion, version.Version)
	}
}

// --- Task commands (Part A/B) -----------------------------------------------

func TestTaskNewCreatesRunAndSetsCurrent(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root) // setupContractRun uses task new
	current, ok := readCurrentRun(paths)
	if !ok || current == "" {
		t.Fatalf("task new did not set current run")
	}
	if !runExists(paths, current) {
		t.Fatalf("current run %q does not exist", current)
	}
	_ = app
}

func TestTaskListAndCurrentShowRun(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"task", "list", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task list exited %d, stderr: %s", code, stderr.String())
	}
	var list taskListResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &list))
	if list.CurrentRunID != runID || len(list.Runs) != 1 || list.Runs[0].RunID != runID {
		t.Fatalf("task list unexpected: %#v", list)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"task", "current"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task current exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), runID) {
		t.Fatalf("task current output mismatch:\n%s", stdout.String())
	}
}

func TestTaskUseChangesCurrent(t *testing.T) {
	root := t.TempDir()
	app, paths, first := setupContractRun(t, root)
	second := runContractOnlyForTask(t, app, "second task")
	if first == second {
		t.Fatalf("expected distinct run ids, got %q twice", first)
	}
	// task new made `second` current; switch back to first.
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"task", "use", first}, &stdout, &stderr); code != 0 {
		t.Fatalf("task use exited %d, stderr: %s", code, stderr.String())
	}
	if current, _ := readCurrentRun(paths); current != first {
		t.Fatalf("current = %q, want %q", current, first)
	}
}

func TestOmittedRunIDUsesCurrentRun(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"contract", "show"}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract show (no run id) exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), runID) {
		t.Fatalf("contract show did not resolve current run:\n%s", stdout.String())
	}
}

func TestOmittedRunIDUsesSoleActiveRunWhenNoCurrent(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	assertNoError(t, os.Remove(currentRunPointerPath(paths)))

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"contract", "show"}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract show (sole active) exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), runID) {
		t.Fatalf("contract show did not resolve sole active run:\n%s", stdout.String())
	}
}

func TestOmittedRunIDWithMultipleRunsAndNoCurrentFails(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	_ = runContractOnlyForTask(t, app, "second task")
	assertNoError(t, os.Remove(currentRunPointerPath(paths)))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "show"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("ambiguous contract show exited %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "run id is required") {
		t.Fatalf("ambiguous run stderr mismatch:\n%s", stderr.String())
	}
}

func TestTaskShowLatest(t *testing.T) {
	root := t.TempDir()
	app := testAppSequence(root)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	first := runContractOnlyForTask(t, app, "first")
	second := runContractOnlyForTask(t, app, "second")
	if first == second {
		t.Fatalf("expected distinct run ids")
	}
	latest := first
	if second > first {
		latest = second
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"task", "show", "--latest", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task show --latest exited %d, stderr: %s", code, stderr.String())
	}
	var show taskShowResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &show))
	if show.RunID != latest {
		t.Fatalf("task show --latest = %q, want %q", show.RunID, latest)
	}
}

// --- Prefix routing (Part B) ------------------------------------------------

func TestSplitLeadingRunID(t *testing.T) {
	cases := []struct {
		args     []string
		wantRun  string
		wantRest []string
	}{
		{[]string{"q_001", "answer"}, "", []string{"q_001", "answer"}},
		{[]string{"run_x", "q_001", "answer"}, "run_x", []string{"q_001", "answer"}},
		{[]string{"f_001"}, "", []string{"f_001"}},
		{[]string{"run_20260101_000000", "f_001"}, "run_20260101_000000", []string{"f_001"}},
		{[]string{}, "", []string{}},
	}
	for _, tc := range cases {
		gotRun, gotRest := splitLeadingRunID(tc.args)
		if gotRun != tc.wantRun || strings.Join(gotRest, ",") != strings.Join(tc.wantRest, ",") {
			t.Fatalf("splitLeadingRunID(%v) = (%q, %v), want (%q, %v)", tc.args, gotRun, gotRest, tc.wantRun, tc.wantRest)
		}
	}
}

func TestClarifyAnswerPrefixRouting(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	// Ask two questions.
	for _, q := range []string{"First?", "Second?"} {
		var stdout, stderr bytes.Buffer
		if code := app.Run([]string{"clarify", "ask", q}, &stdout, &stderr); code != 0 {
			t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
		}
	}

	// Answer without a run id (current run).
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"clarify", "answer", "q_001", "Answer one"}, &stdout, &stderr); code != 0 {
		t.Fatalf("clarify answer (no run id) exited %d, stderr: %s", code, stderr.String())
	}
	// Answer with an explicit run id.
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"clarify", "answer", runID, "q_002", "Answer two"}, &stdout, &stderr); code != 0 {
		t.Fatalf("clarify answer (explicit run id) exited %d, stderr: %s", code, stderr.String())
	}
}

func TestPrefixRoutingRejectsWrongPrefix(t *testing.T) {
	root := t.TempDir()
	app, _, _ := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "answer", "x_001", "ans"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("clarify answer wrong prefix exited %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "expected a question id") {
		t.Fatalf("wrong-prefix stderr mismatch:\n%s", stderr.String())
	}
}

func TestReviewResolvePrefixRouting(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupRunWithGateReport(t, root, "passed")

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"review", "prepare", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("review prepare exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"review", "add-finding", runID, "needs work", "--blocking"}, &stdout, &stderr); code != 0 {
		t.Fatalf("review add-finding exited %d, stderr: %s", code, stderr.String())
	}

	// Resolve with a bare finding id (run resolved from current/sole-active).
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"review", "resolve", "f_001", "--note", "fixed"}, &stdout, &stderr); code != 0 {
		t.Fatalf("review resolve (bare finding id) exited %d, stderr: %s", code, stderr.String())
	}
}

// --- JSON readiness (Part E) ------------------------------------------------

func TestJSONReadinessResponses(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	cases := [][]string{
		{"gate", "show", runID, "--json"},
		{"review", "status", runID, "--json"},
		{"review", "show", runID, "--json"},
		{"memory", "show", runID, "--json"},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, want 0, stderr: %s", args, code, stderr.String())
		}
		var resp notReadyResponse
		if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
			t.Fatalf("%v did not emit valid JSON: %v\n%s", args, err, stdout.String())
		}
		if resp.Ready {
			t.Fatalf("%v ready=true, want false: %#v", args, resp)
		}
		if resp.SuggestedCommand == "" {
			t.Fatalf("%v missing suggested_command: %#v", args, resp)
		}
	}
}

// --- JSON error envelope (Part F) -------------------------------------------

func TestJSONErrorEnvelope(t *testing.T) {
	root := t.TempDir()
	app, _, _ := setupContractRun(t, root)

	cases := []struct {
		args     []string
		wantCode string
	}{
		{[]string{"contract", "show", "run_missing", "--json"}, "run_not_found"},
		{[]string{"execute", "dry-run", "run_missing", "--json"}, "run_not_found"},
		{[]string{"prompt", "build", "--json"}, "contract_not_approved"}, // current run is a draft
	}
	for _, tc := range cases {
		var stdout, stderr bytes.Buffer
		code := app.Run(tc.args, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("%v exited %d, want 1", tc.args, code)
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v wrote stderr in --json mode:\n%s", tc.args, stderr.String())
		}
		var env errorEnvelope
		if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
			t.Fatalf("%v did not emit JSON envelope: %v\n%s", tc.args, err, stdout.String())
		}
		if env.Schema != errorSchema || env.Error.Code != tc.wantCode {
			t.Fatalf("%v envelope = %#v, want code %q", tc.args, env, tc.wantCode)
		}
	}
}

func TestErrorHumanModeUsesStderr(t *testing.T) {
	root := t.TempDir()
	app, _, _ := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "show", "run_missing"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("contract show missing exited %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "run not found: run_missing") {
		t.Fatalf("human error not on stderr:\n%s", stderr.String())
	}
	if strings.Contains(stdout.String(), "{") {
		t.Fatalf("human error leaked JSON to stdout:\n%s", stdout.String())
	}
}

// --- Lifecycle status & next-step (Part H) ----------------------------------

func TestStatusDerivesLifecycleAndNextStep(t *testing.T) {
	root := t.TempDir()
	app, _, _ := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"status", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("status exited %d, stderr: %s", code, stderr.String())
	}
	var status statusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.Runs.LatestStatus != "prompt_built" {
		t.Fatalf("latest status = %q, want prompt_built", status.Runs.LatestStatus)
	}
	if status.Runs.NextCommand != "pactum execute dry-run --agent codex" {
		t.Fatalf("next command = %q", status.Runs.NextCommand)
	}
}

func TestExecuteRunDerivesExecutedStatus(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("execute run --yes exited %d, stderr: %s", code, stderr.String())
	}
	if got := deriveRunStatus(paths, runID); got != "executed" {
		t.Fatalf("derived status = %q, want executed", got)
	}
}

// --- Execute run confirmation (Part I) --------------------------------------

func TestExecuteRunWithoutYesFailsNonInteractive(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("execute run without --yes exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "without --yes") {
		t.Fatalf("confirmation stderr mismatch:\n%s", stderr.String())
	}
	// It must refuse before launching any agent attempt.
	assertNoFile(t, executionAttemptPaths(runPaths, "attempt_001").ResultJSON)
}

func TestExecuteDryRunDoesNotNeedYes(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"execute", "dry-run", runID, "--agent", "codex"}, &stdout, &stderr); code != 0 {
		t.Fatalf("execute dry-run exited %d, stderr: %s", code, stderr.String())
	}
}

// --- Map (Part J) -----------------------------------------------------------

func TestMapRefreshHelpHasNoFullFlag(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"map", "refresh", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh --help exited %d", code)
	}
	if strings.Contains(stdout.String(), "--full") {
		t.Fatalf("map refresh help still mentions --full:\n%s", stdout.String())
	}
}
