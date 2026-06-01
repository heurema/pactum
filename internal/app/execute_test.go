package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
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

func readDryRunPlan(t *testing.T, path string) agents.DryRunPlan {
	t.Helper()
	var plan agents.DryRunPlan
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &plan))
	return plan
}
