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

func TestExecutePlanBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"execute", "plan", "run_x"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("execute plan before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("execute plan before init stderr mismatch:\n%s", got)
	}
}

func TestExecutePlanMissingRunReturnsError(t *testing.T) {
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
	code = app.Run([]string{"execute", "plan", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestExecutePlanMissingPromptManifestFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedPromptContract(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail without built prompt")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: executor prompt has not been built") {
		t.Fatalf("missing prompt manifest stderr mismatch:\n%s", got)
	}
}

func TestExecutePlanSucceedsAfterPromptBuild(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.DryRunJSON)
	got := stdout.String()
	for _, want := range []string{
		"Execution plan prepared",
		"Resolved:",
		"Would run:",
		"codex exec --json --dangerously-bypass-approvals-and-sandbox -c model=\"gpt-5\" < .heurema/pactum/runs/" + runID + "/contract/prompt.md",
		".heurema/pactum/runs/" + runID + "/execute/dry-run.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("execute plan output missing %q:\n%s", want, got)
		}
	}
	assertResolvedBlock(t, got, "codex", "gpt-5", "inherit", "partial")

	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Schema != agents.DryRunSchema || plan.RunID != runID || plan.Agent.Name != "codex" {
		t.Fatalf("unexpected dry-run plan: %#v", plan)
	}
	wantPrompt := executionPromptRepoPath(runID)
	if plan.WouldRun.Command != "codex" || strings.Join(plan.WouldRun.Args, " ") != `exec --json --dangerously-bypass-approvals-and-sandbox -c model="gpt-5"` || plan.WouldRun.Stdin != wantPrompt {
		t.Fatalf("unexpected would_run command: %#v", plan.WouldRun)
	}
	assertCommandArgsDoNotContain(t, plan.WouldRun.Args, "contract/prompt.md", wantPrompt)
}

func TestExecutePlanJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan --json exited %d, stderr: %s", code, stderr.String())
	}
	var plan agents.DryRunPlan
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &plan))
	if plan.Agent.Name != "codex" || !plan.Checks.PromptManifestReady || plan.Artifacts.Prompt != "contract/prompt.md" {
		t.Fatalf("unexpected dry-run json: %#v", plan)
	}
	if plan.WouldRun.Command != "codex" || strings.Join(plan.WouldRun.Args, " ") != `exec --json --dangerously-bypass-approvals-and-sandbox -c model="gpt-5"` || plan.WouldRun.Stdin != executionPromptRepoPath(runID) {
		t.Fatalf("missing would_run json: %#v", plan.WouldRun)
	}
	if strings.Contains(stdout.String(), "Execution plan prepared") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Resolved:") {
		t.Fatalf("json output should not include resolved human output:\n%s", stdout.String())
	}
}

func TestExecutePlanUnsupportedAgentFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID, "--agent", "missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail for missing agent")
	}
	if got := stderr.String(); !strings.Contains(got, `unknown agent "missing": not registered in config agents`) {
		t.Fatalf("missing agent stderr mismatch:\n%s", got)
	}
}

func TestExecutePlanUsesDefaultExecutor(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan exited %d, stderr: %s", code, stderr.String())
	}
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Agent.Name != "codex" || plan.Agent.Command != "codex" {
		t.Fatalf("default executor mismatch: %#v", plan.Agent)
	}
}

func TestExecutePlanExplicitCodex(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan codex exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Agent.Name != "codex" || plan.Agent.Command != "codex" {
		t.Fatalf("codex agent mismatch: %#v", plan.Agent)
	}
	if strings.Join(plan.WouldRun.Args, " ") != `exec --json --dangerously-bypass-approvals-and-sandbox -c model="gpt-5"` || plan.WouldRun.Stdin != executionPromptRepoPath(runID) {
		t.Fatalf("codex would_run mismatch: %#v", plan.WouldRun)
	}
	assertCommandArgsDoNotContain(t, plan.WouldRun.Args, "contract/prompt.md", executionPromptRepoPath(runID))
}

func TestExecutePlanExplicitClaude(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID, "--agent", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan claude exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	// Claude runs over ACP; no CLI command or args in the dry-run plan.
	if plan.Agent.Name != "claude" || plan.Agent.Command != "" {
		t.Fatalf("claude agent mismatch: %#v", plan.Agent)
	}
	if len(plan.Agent.Args) != 0 || plan.WouldRun.Command != "" || len(plan.WouldRun.Args) != 0 {
		t.Fatalf("claude dry-run plan must carry no CLI args: %#v", plan)
	}
}

func TestExecutePlanAppliesExecutorModelConfigToCodex(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "codex", Model: "gpt-5", Effort: "high"})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan codex exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	wantArgs := []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "-c", "model=\"gpt-5\"", "-c", "model_reasoning_effort=high"}
	if !sameStringSlice(plan.WouldRun.Args, wantArgs) {
		t.Fatalf("codex would_run args = %#v, want %#v", plan.WouldRun.Args, wantArgs)
	}
	assertResolvedBlock(t, stdout.String(), "codex", "gpt-5", "high", "pinned")
}

func TestExecutePlanAppliesExecutorModelConfigToClaude(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"}, agentRegistryEntry{Name: "codex", Model: "gpt-5"})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID, "--agent", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan claude exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	// Claude model/effort are pinned via ACP adapter env vars, not CLI args.
	// The dry-run plan WouldRun must be empty (no CLI subprocess).
	if plan.WouldRun.Command != "" || len(plan.WouldRun.Args) != 0 {
		t.Fatalf("claude ACP dry-run plan must have empty WouldRun: %#v", plan.WouldRun)
	}
	assertResolvedBlock(t, stdout.String(), "claude", "claude-sonnet-4", "high", "pinned")
}

func TestExecutePlanResolvedPartialPin(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "codex", Model: "gpt-5"})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan codex exited %d, stderr: %s", code, stderr.String())
	}
	// Model-only pin: the effort still inherits, so pinning is "partial", not "pinned".
	assertResolvedBlock(t, stdout.String(), "codex", "gpt-5", "inherit", "partial")
}

func TestExecutePlanPinAppliesOnlyToMatchingAgent(t *testing.T) {
	root := t.TempDir()
	// Each entry carries only its own pins: invoking codex must not pick up
	// claude's effort pin.
	app, paths, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"}, agentRegistryEntry{Name: "codex", Model: "gpt-5"})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan codex exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	wantArgs := []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "-c", "model=\"gpt-5\""}
	if !sameStringSlice(plan.WouldRun.Args, wantArgs) {
		t.Fatalf("codex would_run args = %#v, want %#v", plan.WouldRun.Args, wantArgs)
	}
	assertResolvedBlock(t, stdout.String(), "codex", "gpt-5", "inherit", "partial")
}

func TestExecutePlanDefaultsToFirstRegistryEntry(t *testing.T) {
	root := t.TempDir()
	// claude first: an omitted --agent must pick the first registry entry, not
	// a hardcoded built-in default.
	app, paths, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root,
		agentRegistryEntry{Name: "claude", Model: "claude-opus-4-8"},
		agentRegistryEntry{Name: "codex", Model: "gpt-5"},
	)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	// Claude runs over ACP; the plan carries no CLI command or args.
	if plan.Agent.Name != "claude" || plan.Agent.Command != "" {
		t.Fatalf("default executor should be the first registry entry: %#v", plan.Agent)
	}
	assertResolvedBlock(t, stdout.String(), "claude", "claude-opus-4-8", "inherit", "partial")
}

func TestExecutePlanTwoEntriesOnSameBuiltInCarryDistinctPins(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root,
		agentRegistryEntry{Name: "fable", Model: "claude-fable-5"},
		agentRegistryEntry{Name: "opus", Model: "claude-opus-4-8"},
	)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID, "--agent", "fable"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan fable exited %d, stderr: %s", code, stderr.String())
	}
	plan := readDryRunPlan(t, runPaths.DryRunJSON)
	// Claude runs over ACP; no CLI args. The model pin lives in the ACP adapter env.
	if plan.Agent.Name != "claude" || plan.WouldRun.Command != "" || len(plan.WouldRun.Args) != 0 {
		t.Fatalf("fable entry should pin claude-fable-5 on claude (ACP, no CLI args): %#v", plan)
	}
	assertResolvedBlock(t, stdout.String(), "fable", "claude-fable-5", "inherit", "partial")

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "plan", runID, "--agent", "opus"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan opus exited %d, stderr: %s", code, stderr.String())
	}
	plan = readDryRunPlan(t, runPaths.DryRunJSON)
	if plan.Agent.Name != "claude" || plan.WouldRun.Command != "" || len(plan.WouldRun.Args) != 0 {
		t.Fatalf("opus entry should pin claude-opus-4-8 on claude (ACP, no CLI args): %#v", plan)
	}
	assertResolvedBlock(t, stdout.String(), "opus", "claude-opus-4-8", "inherit", "partial")
}

func TestExecutePlanHashMismatchFails(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	contractJSON := strings.Replace(mustReadFile(t, runPaths.ContractJSON), "add deterministic prompt boundary", "tampered prompt boundary", 1)
	mustWriteFile(t, runPaths.ContractJSON, contractJSON)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail with hash mismatch")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: approved contract hash does not match current contract") {
		t.Fatalf("hash mismatch stderr mismatch:\n%s", got)
	}
}

func TestExecutePlanStaleMapFails(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail with stale map")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: project map is stale") {
		t.Fatalf("stale map stderr mismatch:\n%s", got)
	}
}

func TestExecutePlanFailsWhenPromptManifestMapRunIsStale(t *testing.T) {
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
	code = app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail when prompt manifest map run is stale")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: executor prompt was built for a different project map") {
		t.Fatalf("stale prompt map stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.DryRunJSON)
}

func TestExecutePlanSucceedsAfterPromptBuildOnLatestMap(t *testing.T) {
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
	code = app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.DryRunJSON)
}

func TestExecutePlanPathPortability(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan exited %d, stderr: %s", code, stderr.String())
	}
	assertDoesNotContainRoot(t, "execute/dry-run.json", mustReadFile(t, runPaths.DryRunJSON), root)
}

func TestExecutePlanWritesLedgerEvent(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan exited %d, stderr: %s", code, stderr.String())
	}
	events := strings.Join(readLines(t, paths.EventsJSONL), "\n")
	if !strings.Contains(events, "execution_dry_run_prepared") || !strings.Contains(events, runID) {
		t.Fatalf("events missing execution_dry_run_prepared:\n%s", events)
	}
}

func TestExecutePlanUnsupportedInputModeFails(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	registerTestAgents(t, paths, "bad-input")
	app.AgentRegistry = testAgentRegistry(agents.AgentDescriptor{
		Name:    testAgentEngine("bad-input"),
		Command: "bad-agent",
		Args:    []string{"dry-run"},
		Input:   "stdin",
	})
	runReviewCommand(t, app, "map", "refresh")
	runReviewCommand(t, app, "prompt", "build", runID)

	var stdout, stderr bytes.Buffer
	stdout.Reset()
	stderr.Reset()
	code := app.Run([]string{"execute", "plan", runID, "--agent", "bad-input"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail for unsupported input mode")
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
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
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
	code := app.Run([]string{"execute", "plan", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan helper exited %d, stderr: %s", code, stderr.String())
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	dryRunPlan := readDryRunPlan(t, runPaths.DryRunJSON)

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Execution attempt finished") || !strings.Contains(got, "attempt_001") {
		t.Fatalf("execute run output mismatch:\n%s", got)
	} else {
		assertResolvedBlock(t, got, "helper", "claude-opus-4-8", "inherit", "partial")
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
	// The request records the engine inferred from the helper entry's model.
	if request.Agent.Name != "claude" || request.Agent.Command != os.Args[0] || request.Agent.Input != agents.InputPromptFile {
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
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
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
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--json"}, &stdout, &stderr)
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

func TestExecuteRunStreamsLiveOutputToStderr(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}

	// The agent's stdout and stderr both stream live to the operator's stderr.
	if got := stderr.String(); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stdin_has_executor_prompt=true") || !strings.Contains(got, "stderr-line") {
		t.Fatalf("live agent output missing from stderr:\n%s", got)
	}
	// Stdout stays the clean human result channel and never carries agent output.
	out := stdout.String()
	if !strings.Contains(out, "Execution attempt finished") {
		t.Fatalf("stdout missing human summary:\n%s", out)
	}
	if strings.Contains(out, "cwd_is_repo=") || strings.Contains(out, "stderr-line") {
		t.Fatalf("agent output leaked into stdout:\n%s", out)
	}

	// The attempt log files are still captured.
	attemptPaths := executionAttemptPaths(runPaths, "attempt_001")
	if got := mustReadFile(t, attemptPaths.StdoutLog); !strings.Contains(got, "cwd_is_repo=true") {
		t.Fatalf("stdout log not captured:\n%s", got)
	}
	if got := mustReadFile(t, attemptPaths.StderrLog); !strings.Contains(got, "stderr-line") {
		t.Fatalf("stderr log not captured:\n%s", got)
	}
}

func TestExecuteRunJSONKeepsStdoutCleanWithLiveStderr(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run --json exited %d, stderr: %s", code, stderr.String())
	}

	// --json: stdout is a single parseable result document with no agent output.
	var result executionResultDocument
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &result))
	if result.AttemptID != "attempt_001" || result.ExitCode != 0 {
		t.Fatalf("unexpected execute run json: %#v", result)
	}
	if strings.Contains(stdout.String(), "cwd_is_repo=") || strings.Contains(stdout.String(), "stderr-line") {
		t.Fatalf("agent output leaked into --json stdout:\n%s", stdout.String())
	}
	// Live output still reaches stderr in --json mode.
	if got := stderr.String(); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stderr-line") {
		t.Fatalf("live agent output missing from stderr in --json mode:\n%s", got)
	}
}

func TestNextAttemptIDUsesExistingAttemptDirs(t *testing.T) {
	root := t.TempDir()
	attemptsDir := filepath.Join(root, "attempts")
	id, err := nextAgentAttemptID(attemptsDir, "attempt")
	assertNoError(t, err)
	if id != "attempt_001" {
		t.Fatalf("next attempt without dir = %q", id)
	}
	assertNoError(t, os.MkdirAll(filepath.Join(attemptsDir, "attempt_001"), 0o755))
	assertNoError(t, os.MkdirAll(filepath.Join(attemptsDir, "attempt_002"), 0o755))
	mustWriteFile(t, filepath.Join(attemptsDir, "attempt_099.txt"), "not a dir")

	id, err = nextAgentAttemptID(attemptsDir, "attempt")
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
		code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
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

// configureHelperAgent routes the engine a helper name infers to (see
// testAgentEngine) to the execution helper process.
func configureHelperAgent(app App, name string) App {
	app.AgentRegistry = testAgentRegistry(helperAgentDescriptor(testAgentEngine(name)))
	return app
}

func setupApprovedBuiltPromptWithHelperAgent(t *testing.T, root string, name string) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	app = configureHelperAgent(app, name)
	// The helper agent must be registered in the config registry to be
	// referenceable; the injected test agent registry lists it as a built-in.
	registerTestAgents(t, paths, name)

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

func setupApprovedAndBuiltPromptWithAgentRegistry(t *testing.T, root string, entries ...agentRegistryEntry) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	setAgentRegistryConfig(t, paths, entries...)

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
	if os.Getenv("PACTUM_HELPER_CODEX_USAGE") == "1" {
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":12,"cached_input_tokens":3,"output_tokens":4,"reasoning_output_tokens":1}}`)
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":120,"cached_input_tokens":30,"output_tokens":40,"reasoning_output_tokens":10}}`)
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
