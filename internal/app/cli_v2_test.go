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
	if status.Runs.NextCommand != "pactum execute dry-run" {
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

// --- Derived lifecycle respects upstream resets (Blocker 2) -----------------

func TestDeriveStatusRewindsAfterContractRevise(t *testing.T) {
	root := t.TempDir()
	// A run advanced to a built prompt (approval + prompt manifest present).
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	if got := deriveRunStatus(paths, runID); got != "prompt_built" {
		t.Fatalf("pre-revise status = %q, want prompt_built", got)
	}

	// Revising the contract resets approval and removes prompt readiness, so the
	// derived status must rewind to contract_draft even though downstream
	// artifacts could exist.
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"contract", "revise", runID, "--goal", "new goal"}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
	}
	if got := deriveRunStatus(paths, runID); got != "contract_draft" {
		t.Fatalf("post-revise status = %q, want contract_draft", got)
	}
}

func TestDeriveStatusIgnoresStaleMemoryArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Plant stale downstream artifacts (as if the run had been completed once):
	// a gate report, a review doc, a memory candidate, and an accepted memory
	// acceptance.
	mustWriteFile(t, runPaths.GateReportJSON, `{"schema":"x","status":"passed"}`)
	mustWriteFile(t, runPaths.LastResultJSON, `{"schema":"x"}`)
	mustWriteFile(t, runPaths.ReviewJSON, `{"status":"approved"}`)
	mustWriteFile(t, runPaths.MemoryCandidateJSON, `{"schema":"x"}`)
	mustWriteFile(t, runPaths.MemoryAcceptanceJSON, `{"schema":"x","status":"accepted"}`)
	if got := deriveRunStatus(paths, runID); got != "memory_accepted" {
		t.Fatalf("with valid chain status = %q, want memory_accepted", got)
	}

	// Now invalidate the upstream boundary: revise the contract. Even though the
	// memory acceptance still exists, status must rewind to contract_draft.
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"contract", "revise", runID, "--goal", "changed"}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
	}
	if got := deriveRunStatus(paths, runID); got != "contract_draft" {
		t.Fatalf("post-revise status with stale memory = %q, want contract_draft", got)
	}
}

func TestDeriveStatusProposedVsAcceptedMemory(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	_ = app
	// Valid chain through review approval + a memory candidate but a *pending*
	// acceptance (what `memory propose` writes) must read as memory_proposed.
	mustWriteFile(t, runPaths.LastResultJSON, `{"schema":"x"}`)
	mustWriteFile(t, runPaths.GateReportJSON, `{"status":"passed"}`)
	mustWriteFile(t, runPaths.ReviewJSON, `{"status":"approved"}`)
	mustWriteFile(t, runPaths.MemoryCandidateJSON, `{"schema":"x"}`)
	mustWriteFile(t, runPaths.MemoryAcceptanceJSON, `{"schema":"x","status":"pending"}`)
	if got := deriveRunStatus(paths, runID); got != "memory_proposed" {
		t.Fatalf("pending acceptance status = %q, want memory_proposed", got)
	}
}

// --- Status next command is executable (Blocker 3) --------------------------

func TestStatusNextCommandIsResolvable(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	_ = runContractOnlyForTask(t, app, "second task")
	// Remove the current pointer: now there are two active runs and no current,
	// so a bare staged command would not resolve.
	assertNoError(t, os.Remove(currentRunPointerPath(paths)))

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"status", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("status exited %d, stderr: %s", code, stderr.String())
	}
	var status statusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if !strings.Contains(status.Runs.NextCommand, "task use") {
		t.Fatalf("ambiguous next command should point at task use, got %q", status.Runs.NextCommand)
	}
	// The suggested command must itself be runnable.
	parts := strings.Fields(status.Runs.NextCommand)
	code := app.Run(parts[1:], &stdout, &stderr) // drop leading "pactum"
	if code != 0 {
		t.Fatalf("suggested next command %q failed: %d", status.Runs.NextCommand, code)
	}
}

func TestStatusNextCommandBareWhenSoleActiveRun(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	assertNoError(t, os.Remove(currentRunPointerPath(paths)))

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"status", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("status exited %d, stderr: %s", code, stderr.String())
	}
	var status statusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.Runs.NextCommand != "pactum contract revise" {
		t.Fatalf("sole-active next command = %q, want bare 'pactum contract revise'", status.Runs.NextCommand)
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

func TestReviewRunWithoutYesFailsNonInteractive(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperReviewers(t, app, paths, "helper")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "helper"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("review run without --yes exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "without --yes") {
		t.Fatalf("review run confirmation stderr mismatch:\n%s", stderr.String())
	}
	// It must refuse before launching any reviewer attempt.
	assertNoFile(t, reviewerAttemptPaths(runPaths, "reviewer_attempt_001").ResultJSON)
}

func TestReviewFixWithoutYesFailsNonInteractive(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	app = configureHelperFixers(t, app, paths, "helper")
	runReviewCommand(t, app, "review", "add-finding", runID, "fixer requires explicit yes")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("review fix without --yes exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "without --yes") {
		t.Fatalf("review fix confirmation stderr mismatch:\n%s", stderr.String())
	}
	assertNoFile(t, reviewFixAttemptPaths(runPaths, "attempt_001").ResultJSON)
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
