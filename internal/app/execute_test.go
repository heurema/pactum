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

func TestExecuteDryRunBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"execute", "dry-run", "run_x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("execute dry-run before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("execute dry-run before init stderr mismatch:\n%s", got)
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
		"Resolved:",
		"Would run:",
		"codex exec --dangerously-bypass-approvals-and-sandbox < .heurema/pactum/runs/" + runID + "/contract/prompt.md",
		".heurema/pactum/runs/" + runID + "/execute/dry-run.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("execute dry-run output missing %q:\n%s", want, got)
		}
	}
	assertResolvedBlock(t, got, "codex", "inherit", "inherit", "inherit")

	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Schema != agents.DryRunSchema || plan.RunID != runID || plan.Agent.Name != "codex" {
		t.Fatalf("unexpected dry-run plan: %#v", plan)
	}
	wantPrompt := executionPromptRepoPath(runID)
	if plan.WouldRun.Command != "codex" || strings.Join(plan.WouldRun.Args, " ") != "exec --dangerously-bypass-approvals-and-sandbox" || plan.WouldRun.Stdin != wantPrompt {
		t.Fatalf("unexpected would_run command: %#v", plan.WouldRun)
	}
	assertCommandArgsDoNotContain(t, plan.WouldRun.Args, "contract/prompt.md", wantPrompt)
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
	if plan.WouldRun.Command != "codex" || strings.Join(plan.WouldRun.Args, " ") != "exec --dangerously-bypass-approvals-and-sandbox" || plan.WouldRun.Stdin != executionPromptRepoPath(runID) {
		t.Fatalf("missing would_run json: %#v", plan.WouldRun)
	}
	if strings.Contains(stdout.String(), "Execution dry-run prepared") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Resolved:") {
		t.Fatalf("json output should not include resolved human output:\n%s", stdout.String())
	}
}

func TestExecuteDryRunUnsupportedAgentFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID, "--agent", "missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute dry-run should fail for missing agent")
	}
	if got := stderr.String(); !strings.Contains(got, "unsupported agent: missing") {
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

func TestExecuteDryRunExplicitCodex(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run codex exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Agent.Name != "codex" || plan.Agent.Command != "codex" {
		t.Fatalf("codex agent mismatch: %#v", plan.Agent)
	}
	if strings.Join(plan.WouldRun.Args, " ") != "exec --dangerously-bypass-approvals-and-sandbox" || plan.WouldRun.Stdin != executionPromptRepoPath(runID) {
		t.Fatalf("codex would_run mismatch: %#v", plan.WouldRun)
	}
	assertCommandArgsDoNotContain(t, plan.WouldRun.Args, "contract/prompt.md", executionPromptRepoPath(runID))
}

func TestExecuteDryRunExplicitClaude(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID, "--agent", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run claude exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Agent.Name != "claude" || plan.Agent.Command != "claude" {
		t.Fatalf("claude agent mismatch: %#v", plan.Agent)
	}
	if strings.Join(plan.WouldRun.Args, " ") != "-p --dangerously-skip-permissions" || plan.WouldRun.Stdin != executionPromptRepoPath(runID) {
		t.Fatalf("claude would_run mismatch: %#v", plan.WouldRun)
	}
	assertCommandArgsDoNotContain(t, plan.WouldRun.Args, "contract/prompt.md", executionPromptRepoPath(runID))
}

func TestExecuteDryRunAppliesExecutorModelConfigToCodex(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPromptWithExecutorModel(t, root, "gpt-5:high")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run codex exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	wantArgs := []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "-c", "model=\"gpt-5\"", "-c", "model_reasoning_effort=high"}
	if !sameStringSlice(plan.WouldRun.Args, wantArgs) {
		t.Fatalf("codex would_run args = %#v, want %#v", plan.WouldRun.Args, wantArgs)
	}
	assertResolvedBlock(t, stdout.String(), "codex", "gpt-5", "high", "pinned")
}

func TestExecuteDryRunAppliesExecutorModelConfigToClaude(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPromptWithExecutorModel(t, root, "claude-sonnet-4:high")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID, "--agent", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run claude exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	wantArgs := []string{"-p", "--dangerously-skip-permissions", "--model", "claude-sonnet-4", "--effort", "high"}
	if !sameStringSlice(plan.WouldRun.Args, wantArgs) {
		t.Fatalf("claude would_run args = %#v, want %#v", plan.WouldRun.Args, wantArgs)
	}
	assertResolvedBlock(t, stdout.String(), "claude", "claude-sonnet-4", "high", "pinned")
}

func TestExecuteDryRunResolvedPartialPin(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPromptWithExecutorModel(t, root, ":high")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run codex exited %d, stderr: %s", code, stderr.String())
	}
	// Effort-only pin: the model still inherits, so pinning is "partial", not "pinned".
	assertResolvedBlock(t, stdout.String(), "codex", "inherit", "high", "partial")
}

func TestLegacyAgentConfigIsToleratedAndIgnored(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))
	config := defaultConfigFile()
	config.Agents = agents.AgentConfig{
		DefaultExecutor: "legacy-helper",
		DefaultReviewer: "legacy-reviewer",
		Adapters: map[string]agents.AdapterConfig{
			"legacy-helper": {
				Command: "legacy-helper",
				Args:    []string{"run"},
				Input:   agents.InputPromptFile,
			},
			"legacy-reviewer": {
				Command: "legacy-reviewer",
				Args:    []string{"review"},
				Input:   agents.InputPromptFile,
			},
		},
	}
	assertNoError(t, writeYAML(paths.Config, config))

	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run with legacy config exited %d, stderr: %s", code, stderr.String())
	}
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Agent.Name != "codex" || plan.Agent.Command != "codex" {
		t.Fatalf("legacy config should be ignored by runtime, got: %#v", plan.Agent)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "dry-run", runID, "--agent", "legacy-helper"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("legacy adapter name should not be supported")
	}
	if got := stderr.String(); !strings.Contains(got, "unsupported agent: legacy-helper") {
		t.Fatalf("legacy adapter stderr mismatch:\n%s", got)
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
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)
	app.AgentRegistry = testAgentRegistry(agents.AgentDescriptor{
		Name:    "bad-input",
		Command: "bad-agent",
		Args:    []string{"dry-run"},
		Input:   "stdin",
	})

	var stdout, stderr bytes.Buffer
	stdout.Reset()
	stderr.Reset()
	code := app.Run([]string{"execute", "dry-run", runID, "--agent", "bad-input"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute dry-run should fail for unsupported input mode")
	}
	if got := stderr.String(); !strings.Contains(got, "unsupported agent input mode: stdin") {
		t.Fatalf("unsupported input stderr mismatch:\n%s", got)
	}
}

func TestExecuteRunDoesNotRequireAllowExecute(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, executionAttemptPaths(runPaths, "attempt_001").ResultJSON)
	events := strings.Join(readLines(t, paths.EventsJSONL), "\n")
	for _, want := range []string{"execution_attempt_started", "execution_attempt_finished"} {
		if !strings.Contains(events, want) {
			t.Fatalf("execute run should write %s:\n%s", want, events)
		}
	}
}

func TestExecuteRunWritesAttemptArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "dry-run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute dry-run helper exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	dryRunPlan := readDryRunPlan(t, runPaths.DryRunJSON)

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "run", runID, "--agent", "helper", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Execution attempt finished") || !strings.Contains(got, "attempt_001") {
		t.Fatalf("execute run output mismatch:\n%s", got)
	} else {
		assertResolvedBlock(t, got, "helper", "inherit", "inherit", "inherit")
	}

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
	wantPrompt := executionPromptRepoPath(runID)
	if request.WouldRun.Stdin != wantPrompt {
		t.Fatalf("unexpected would_run stdin = %q, want %q", request.WouldRun.Stdin, wantPrompt)
	}
	if dryRunPlan.WouldRun.Command != request.WouldRun.Command ||
		!sameStringSlice(dryRunPlan.WouldRun.Args, request.WouldRun.Args) ||
		dryRunPlan.WouldRun.Stdin != request.WouldRun.Stdin {
		t.Fatalf("dry-run would_run should match request would_run\ndry-run: %#v\nrequest: %#v", dryRunPlan.WouldRun, request.WouldRun)
	}
	assertCommandArgsDoNotContain(t, request.WouldRun.Args, "contract/prompt.md", wantPrompt)
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
	if got := mustReadFile(t, attemptPaths.StdoutLog); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stdin_has_executor_prompt=true") {
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
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	startedIndex := indexOfEvent(eventTypes, "execution_attempt_started")
	finishedIndex := indexOfEvent(eventTypes, "execution_attempt_finished")
	if startedIndex == -1 || finishedIndex == -1 {
		t.Fatalf("events missing execution attempt lifecycle events:\n%v", eventTypes)
	}
	if startedIndex > finishedIndex {
		t.Fatalf("execution_attempt_started should appear before execution_attempt_finished:\n%v", eventTypes)
	}
}

func TestExecuteRunNonZeroWritesArtifactsAndReturnsNonZero(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_HELPER_EXIT", "7")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--yes"}, &stdout, &stderr)
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
	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if countEvents(eventTypes, "execution_attempt_started") != 1 || countEvents(eventTypes, "execution_attempt_finished") != 1 {
		t.Fatalf("non-zero execute run should write started and finished events:\n%v", eventTypes)
	}
}

func TestExecuteRunJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--yes", "--json"}, &stdout, &stderr)
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
	if strings.Contains(stdout.String(), "Resolved:") {
		t.Fatalf("json output should not include resolved human output:\n%s", stdout.String())
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

func TestExecuteRunCreatesIncrementingAttemptsAndLedgerEvents(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--yes"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("execute run %d exited %d, stderr: %s", i+1, code, stderr.String())
		}
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	for _, attemptID := range []string{"attempt_001", "attempt_002"} {
		attemptPaths := executionAttemptPaths(runPaths, attemptID)
		assertFile(t, attemptPaths.RequestJSON)
		assertFile(t, attemptPaths.ResultJSON)
	}

	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	if got := countEvents(eventTypes, "execution_attempt_started"); got != 2 {
		t.Fatalf("execution_attempt_started count = %d, want 2:\n%v", got, eventTypes)
	}
	if got := countEvents(eventTypes, "execution_attempt_finished"); got != 2 {
		t.Fatalf("execution_attempt_finished count = %d, want 2:\n%v", got, eventTypes)
	}
}

func readDryRunPlan(t *testing.T, path string) agents.DryRunPlan {
	t.Helper()
	var plan agents.DryRunPlan
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &plan))
	return plan
}

func assertCommandArgsDoNotContain(t *testing.T, args []string, forbidden ...string) {
	t.Helper()
	joined := strings.Join(args, " ")
	for _, value := range forbidden {
		if strings.Contains(joined, value) {
			t.Fatalf("command args should not contain %q: %#v", value, args)
		}
	}
}

func assertResolvedBlock(t *testing.T, got string, agent string, model string, effort string, pinning string) {
	t.Helper()
	want := fmt.Sprintf("Resolved:\n  agent: %s\n  model: %s\n  effort: %s\n  pinning: %s\n", agent, model, effort, pinning)
	if !strings.Contains(got, want) {
		t.Fatalf("resolved output mismatch, missing block:\n%s\noutput:\n%s", want, got)
	}
}

func ledgerEventTypes(t *testing.T, path string) []string {
	t.Helper()
	lines := readLines(t, path)
	types := make([]string, 0, len(lines))
	for _, line := range lines {
		var event struct {
			Type string `json:"type"`
		}
		assertNoError(t, json.Unmarshal([]byte(line), &event))
		types = append(types, event.Type)
	}
	return types
}

func indexOfEvent(events []string, eventType string) int {
	for i, event := range events {
		if event == eventType {
			return i
		}
	}
	return -1
}

func countEvents(events []string, eventType string) int {
	count := 0
	for _, event := range events {
		if event == eventType {
			count++
		}
	}
	return count
}

func configureHelperAgent(app App, name string) App {
	app.AgentRegistry = testAgentRegistry(helperAgentDescriptor(name))
	return app
}

func setupApprovedBuiltPromptWithHelperAgent(t *testing.T, root string, name string) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	app = configureHelperAgent(app, name)

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

func setupApprovedAndBuiltPromptWithExecutorModel(t *testing.T, root string, modelSpec string) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	config.Agents.ExecutorModel = modelSpec
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
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("stdin_has_executor_prompt=%t\n", strings.Contains(string(stdin), "# Executor Prompt"))
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
