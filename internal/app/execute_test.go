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

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

func TestExecuteDryRunBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"execute", "dry-run", "run_x"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("execute dry-run before init output mismatch:\n%s", got)
	}
}

func TestExecuteDryRunMissingRunReturnsError(t *testing.T) {
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
	code = app.Run([]string{"execute", "dry-run", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute dry-run missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestExecuteDryRunMissingPromptManifestFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedPromptContract(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute dry-run should fail without built prompt")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: executor prompt has not been built") {
		t.Fatalf("missing prompt manifest stderr mismatch:\n%s", got)
	}
}

func TestExecuteDryRunSucceedsAfterPromptBuild(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.DryRunJSON)
	got := stdout.String()
	for _, want := range []string{
		"Execution dry-run prepared",
		"Would run:",
		"codex exec --sandbox read-only -- contract/prompt.md",
		".heurema/pactum/runs/" + runID + "/execute/dry-run.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("execute dry-run output missing %q:\n%s", want, got)
		}
	}

	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Schema != agents.DryRunSchema || plan.RunID != runID || plan.Agent.Name != "codex" {
		t.Fatalf("unexpected dry-run plan: %#v", plan)
	}
	if plan.WouldRun.Command != "codex" || strings.Join(plan.WouldRun.Args, " ") != "exec --sandbox read-only -- contract/prompt.md" {
		t.Fatalf("unexpected would_run command: %#v", plan.WouldRun)
	}
}

func TestExecuteDryRunJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run --json exited %d, stderr: %s", code, stderr.String())
	}
	var plan agents.DryRunPlan
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &plan))
	if plan.Agent.Name != "codex" || !plan.Checks.PromptManifestReady || plan.Artifacts.Prompt != "contract/prompt.md" {
		t.Fatalf("unexpected dry-run json: %#v", plan)
	}
	if plan.WouldRun.Command != "codex" || len(plan.WouldRun.Args) == 0 {
		t.Fatalf("missing would_run json: %#v", plan.WouldRun)
	}
	if strings.Contains(stdout.String(), "Execution dry-run prepared") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestExecuteDryRunMissingAgentAdapterFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID, "--agent", "missing-agent"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute dry-run should fail for missing agent")
	}
	if got := stderr.String(); !strings.Contains(got, "agent adapter not configured: missing-agent") {
		t.Fatalf("missing agent stderr mismatch:\n%s", got)
	}
}

func TestExecuteDryRunUsesDefaultExecutor(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run exited %d, stderr: %s", code, stderr.String())
	}
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Agent.Name != "codex" || plan.Agent.Command != "codex" {
		t.Fatalf("default executor mismatch: %#v", plan.Agent)
	}
}

func TestExecuteDryRunCustomAdapter(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	config := defaultConfigFile()
	config.Agents.Adapters["custom"] = agents.AdapterConfig{
		Command: "custom-agent",
		Args:    []string{"prepare", "--mode", "dry"},
		Input:   agents.InputPromptFile,
	}
	assertNoError(t, writeYAML(paths.Config, config))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "dry-run", runID, "--agent", "custom"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run custom exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Agent.Name != "custom" || plan.Agent.Command != "custom-agent" {
		t.Fatalf("custom agent mismatch: %#v", plan.Agent)
	}
	if strings.Join(plan.WouldRun.Args, " ") != "prepare --mode dry -- contract/prompt.md" {
		t.Fatalf("custom would_run mismatch: %#v", plan.WouldRun)
	}
}

func TestExecuteDryRunHashMismatchFails(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	contractJSON := strings.Replace(mustReadFile(t, runPaths.ContractJSON), "add deterministic prompt boundary", "tampered prompt boundary", 1)
	mustWriteFile(t, runPaths.ContractJSON, contractJSON)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute dry-run should fail with hash mismatch")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: approved contract hash does not match current contract") {
		t.Fatalf("hash mismatch stderr mismatch:\n%s", got)
	}
}

func TestExecuteDryRunStaleMapFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute dry-run should fail with stale map")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: project map is stale") {
		t.Fatalf("stale map stderr mismatch:\n%s", got)
	}
}

func TestExecuteDryRunFailsWhenPromptManifestMapRunIsStale(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertFile(t, runPaths.PromptManifest)
	manifest := readPromptManifestForTest(t, runPaths.PromptManifest)

	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}
	workspace, err := readWorkspaceManifest(paths.Manifest)
	assertNoError(t, err)
	if workspace.Map.CurrentRunID == manifest.MapRunID {
		t.Fatalf("map run did not change after refresh: %s", workspace.Map.CurrentRunID)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute dry-run should fail when prompt manifest map run is stale")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: executor prompt was built for a different project map") {
		t.Fatalf("stale prompt map stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.DryRunJSON)
}

func TestExecuteDryRunSucceedsAfterPromptBuildOnLatestMap(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}
	workspace, err := readWorkspaceManifest(paths.Manifest)
	assertNoError(t, err)

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}
	manifest := readPromptManifestForTest(t, runPaths.PromptManifest)
	if manifest.MapRunID != workspace.Map.CurrentRunID {
		t.Fatalf("prompt manifest map_run_id = %q, want %q", manifest.MapRunID, workspace.Map.CurrentRunID)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.DryRunJSON)
}

func TestExecuteDryRunPathPortability(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run exited %d, stderr: %s", code, stderr.String())
	}
	assertDoesNotContainRoot(t, "execute/dry-run.json", mustReadFile(t, runPaths.DryRunJSON), root)
}

func TestExecuteDryRunWritesLedgerEvent(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run exited %d, stderr: %s", code, stderr.String())
	}
	events := strings.Join(readLines(t, paths.EventsJSONL), "\n")
	if !strings.Contains(events, "execution_dry_run_prepared") || !strings.Contains(events, runID) {
		t.Fatalf("events missing execution_dry_run_prepared:\n%s", events)
	}
}

func TestExecuteDryRunUnsupportedInputModeFails(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	config := defaultConfigFile()
	config.Agents.Adapters["bad-input"] = agents.AdapterConfig{
		Command: "bad-agent",
		Args:    []string{"dry-run"},
		Input:   "stdin",
	}
	assertNoError(t, writeYAML(paths.Config, config))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "dry-run", runID, "--agent", "bad-input"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute dry-run should fail for unsupported input mode")
	}
	if got := stderr.String(); !strings.Contains(got, "unsupported agent input mode: stdin") {
		t.Fatalf("unsupported input stderr mismatch:\n%s", got)
	}
}

func TestExecuteRunRequiresAllowExecute(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run without allow exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); got != "Refusing to execute external agent without --allow-execute.\n" {
		t.Fatalf("refusal output mismatch:\n%s", got)
	}
	if _, err := os.Stat(runPaths.AttemptsDir); err == nil {
		t.Fatalf("execute run without allow should not create attempts dir")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestExecuteRunWritesAttemptArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--allow-execute"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Execution attempt finished") || !strings.Contains(got, "attempt_001") {
		t.Fatalf("execute run output mismatch:\n%s", got)
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	attemptPaths := executionAttemptPaths(runPaths, "attempt_001")
	assertFile(t, attemptPaths.RequestJSON)
	assertFile(t, attemptPaths.StdoutLog)
	assertFile(t, attemptPaths.StderrLog)
	assertFile(t, attemptPaths.ResultJSON)
	assertFile(t, runPaths.LastResultJSON)

	var request executionRequestDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.RequestJSON)), &request))
	if request.Schema != executionRequestSchema || request.RunID != runID || request.AttemptID != "attempt_001" {
		t.Fatalf("unexpected request: %#v", request)
	}
	if request.Agent.Name != "helper" || request.Agent.Command != os.Args[0] || request.Agent.Input != agents.InputPromptFile {
		t.Fatalf("unexpected request agent: %#v", request.Agent)
	}
	wantPrompt := ".heurema/pactum/runs/" + runID + "/contract/prompt.md"
	if got := strings.Join(request.WouldRun.Args, " "); !strings.HasSuffix(got, "-- "+wantPrompt) {
		t.Fatalf("unexpected would_run args: %#v", request.WouldRun.Args)
	}
	if request.Artifacts.Prompt != "contract/prompt.md" || request.Artifacts.ExecutorContext != "context/executor-context.md" {
		t.Fatalf("unexpected request artifacts: %#v", request.Artifacts)
	}

	var result executionResultDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.ResultJSON)), &result))
	if result.Schema != executionResultSchema || result.ExitCode != 0 || result.TimedOut {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Stdout != "execute/attempts/attempt_001/stdout.log" || result.Stderr != "execute/attempts/attempt_001/stderr.log" {
		t.Fatalf("unexpected output artifact paths: %#v", result)
	}
	if got := mustReadFile(t, runPaths.LastResultJSON); got != mustReadFile(t, attemptPaths.ResultJSON) {
		t.Fatalf("last-result.json should copy result.json")
	}
	if got := mustReadFile(t, attemptPaths.StdoutLog); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "prompt="+wantPrompt) {
		t.Fatalf("stdout log mismatch:\n%s", got)
	}
	if got := mustReadFile(t, attemptPaths.StderrLog); !strings.Contains(got, "stderr-line") {
		t.Fatalf("stderr log mismatch:\n%s", got)
	}

	assertDoesNotContainRoot(t, "request.json", mustReadFile(t, attemptPaths.RequestJSON), root)
	assertDoesNotContainRoot(t, "result.json", mustReadFile(t, attemptPaths.ResultJSON), root)
	assertDoesNotContainRoot(t, "last-result.json", mustReadFile(t, runPaths.LastResultJSON), root)
	if state := readRunState(t, runPaths.RunJSON); state.Status != "contract_approved" {
		t.Fatalf("execute run should not change status: %#v", state)
	}
	if events := strings.Join(readLines(t, paths.EventsJSONL), "\n"); !strings.Contains(events, "execution_attempt_completed") {
		t.Fatalf("events missing execution_attempt_completed:\n%s", events)
	}
}

func TestExecuteRunNonZeroWritesArtifactsAndReturnsNonZero(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_HELPER_EXIT", "7")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--allow-execute"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute run should return non-zero for agent failure")
	}
	if got := stderr.String(); !strings.Contains(got, "agent process exited with code 7") {
		t.Fatalf("non-zero stderr mismatch:\n%s", got)
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	attemptPaths := executionAttemptPaths(runPaths, "attempt_001")
	assertFile(t, attemptPaths.ResultJSON)
	var result executionResultDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.ResultJSON)), &result))
	if result.ExitCode != 7 || result.TimedOut {
		t.Fatalf("unexpected failing result: %#v", result)
	}
	if state := readRunState(t, runPaths.RunJSON); state.Status != "contract_approved" {
		t.Fatalf("execute run should not change status: %#v", state)
	}
}

func TestExecuteRunJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--allow-execute", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run --json exited %d, stderr: %s", code, stderr.String())
	}
	var result executionResultDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &result))
	if result.AttemptID != "attempt_001" || result.ExitCode != 0 {
		t.Fatalf("unexpected execute run json: %#v", result)
	}
	if strings.Contains(stdout.String(), "Execution attempt finished") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestNextAttemptIDUsesExistingAttemptDirs(t *testing.T) {
	root := t.TempDir()
	attemptsDir := filepath.Join(root, "attempts")
	id, err := nextAttemptID(attemptsDir)
	assertNoError(t, err)
	if id != "attempt_001" {
		t.Fatalf("next attempt without dir = %q", id)
	}
	assertNoError(t, os.MkdirAll(filepath.Join(attemptsDir, "attempt_001"), 0o755))
	assertNoError(t, os.MkdirAll(filepath.Join(attemptsDir, "attempt_002"), 0o755))
	mustWriteFile(t, filepath.Join(attemptsDir, "attempt_099.txt"), "not a dir")

	id, err = nextAttemptID(attemptsDir)
	assertNoError(t, err)
	if id != "attempt_003" {
		t.Fatalf("next attempt with existing dirs = %q", id)
	}
}

func readDryRunPlan(t *testing.T, path string) agents.DryRunPlan {
	t.Helper()
	var plan agents.DryRunPlan
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &plan))
	return plan
}

func configureHelperAgent(t *testing.T, paths artifacts.Paths, name string) {
	t.Helper()
	config := defaultConfigFile()
	config.Agents.Adapters[name] = agents.AdapterConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestExecutionHelperProcess"},
		Input:   agents.InputPromptFile,
	}
	assertNoError(t, writeYAML(paths.Config, config))
}

func setupApprovedBuiltPromptWithHelperAgent(t *testing.T, root string, name string) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	configureHelperAgent(t, paths, name)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}
	return app, paths, runID
}

func TestExecutionHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_HELPER_PROCESS") != "1" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cwd error: %v\n", err)
		os.Exit(2)
	}
	expectedCWD := os.Getenv("PACTUM_HELPER_EXPECTED_CWD")
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	if resolved, err := filepath.EvalSymlinks(expectedCWD); err == nil {
		expectedCWD = resolved
	}
	fmt.Printf("cwd_is_repo=%t\n", cwd == expectedCWD)
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, ".heurema/") {
			fmt.Printf("prompt=%s\n", arg)
		}
	}
	fmt.Fprintln(os.Stderr, "stderr-line")
	if raw := os.Getenv("PACTUM_HELPER_EXIT"); raw != "" {
		code, err := strconv.Atoi(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad exit code: %v\n", err)
			os.Exit(2)
		}
		os.Exit(code)
	}
	os.Exit(0)
}
