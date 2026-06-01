package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
)

func TestPromptBuildBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"prompt", "build", "run_x"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("prompt build before init output mismatch:\n%s", got)
	}
}

func TestPromptBuildMissingRunReturnsError(t *testing.T) {
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
	code = app.Run([]string{"prompt", "build", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("prompt build missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestPromptBuildFailsWhenContractNotApproved(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("prompt build should fail without approval")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot build executor prompt: contract is not approved") {
		t.Fatalf("not approved stderr mismatch:\n%s", got)
	}
}

func TestPromptBuildFailsWhenBlockingClarificationOpen(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "ask", runID, "Need a decision?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("prompt build should fail while blocking questions remain")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot build executor prompt: blocking clarification questions remain") {
		t.Fatalf("blocking clarification stderr mismatch:\n%s", got)
	}
}

func TestPromptBuildFailsWhenProjectMapStale(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedPromptContract(t, root)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("prompt build should fail with stale project map")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot build executor prompt: project map is stale") {
		t.Fatalf("stale map stderr mismatch:\n%s", got)
	}
	if got := stdout.String(); !strings.Contains(got, "pactum map refresh") {
		t.Fatalf("stale map output should suggest refresh:\n%s", got)
	}
}

func TestPromptBuildFailsWhenApprovalHashMismatch(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	contractJSON := strings.Replace(mustReadFile(t, runPaths.ContractJSON), "add deterministic prompt boundary", "tampered prompt boundary", 1)
	mustWriteFile(t, runPaths.ContractJSON, contractJSON)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("prompt build should fail with approval hash mismatch")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot build executor prompt: approved contract hash does not match current contract") {
		t.Fatalf("hash mismatch stderr mismatch:\n%s", got)
	}
}

func TestPromptBuildSucceedsForApprovedContract(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Executor prompt built",
		"status: contract_approved",
		"contract approved: yes",
		"contract hash: ok",
		"project map: fresh",
		".heurema/pactum/runs/" + runID + "/contract/prompt.md",
		".heurema/pactum/runs/" + runID + "/context/executor-context.md",
		".heurema/pactum/runs/" + runID + "/contract/prompt-manifest.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt build output missing %q:\n%s", want, got)
		}
	}
	assertFile(t, runPaths.PromptMD)
	assertFile(t, runPaths.ExecutorContext)
	assertFile(t, runPaths.PromptManifest)

	manifest := readPromptManifestForTest(t, runPaths.PromptManifest)
	approval := readApproval(t, runPaths.ApprovalJSON)
	if manifest.Status != "ready" || manifest.Schema != promptManifestSchema || manifest.RunID != runID {
		t.Fatalf("unexpected manifest identity: %#v", manifest)
	}
	if approval.ContractSHA256 == nil || manifest.ContractSHA256 != *approval.ContractSHA256 {
		t.Fatalf("manifest hash = %q, approval = %#v", manifest.ContractSHA256, approval.ContractSHA256)
	}
	if !manifest.Checks.ContractApproved || !manifest.Checks.ContractHashMatchesApproval || !manifest.Checks.ProjectMapFresh || manifest.Checks.BlockingClarificationsOpen != 0 {
		t.Fatalf("unexpected manifest checks: %#v", manifest.Checks)
	}
	if manifest.Artifacts.Prompt != "contract/prompt.md" || manifest.Artifacts.ExecutorContext != "context/executor-context.md" {
		t.Fatalf("manifest artifact refs mismatch: %#v", manifest.Artifacts)
	}

	promptMD := mustReadFile(t, runPaths.PromptMD)
	for _, want := range []string{
		"This prompt is prepared from an approved Pactum contract.",
		"## Contract status",
		"## Goal\nadd deterministic prompt boundary",
		"## In scope\n- Add prompt build and prompt show commands",
		"## Validation commands\n- go test ./...",
		"## Assumptions\n- Existing contract approval flow remains the readiness source",
		"## Instructions for future executor",
	} {
		if !strings.Contains(promptMD, want) {
			t.Fatalf("prompt.md missing %q:\n%s", want, promptMD)
		}
	}

	executorContext := mustReadFile(t, runPaths.ExecutorContext)
	for _, want := range []string{
		"# Executor Context",
		"Run id: " + runID,
		"Map run id: map_20260531_184012",
		"Repo map: .heurema/pactum/map/repo-map.md",
		"Search index: .heurema/pactum/map/search.sqlite",
		"Use `pactum search \"<term>\"` before adding new code.",
		"q_001: Yes. Prompt build must be approved first.",
	} {
		if !strings.Contains(executorContext, want) {
			t.Fatalf("executor-context.md missing %q:\n%s", want, executorContext)
		}
	}
}

func TestPromptBuildJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedPromptContract(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "build", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build --json exited %d, stderr: %s", code, stderr.String())
	}
	var response promptBuildResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.RunID != runID || response.RunStatus != "contract_approved" || response.Manifest.Status != "ready" {
		t.Fatalf("unexpected prompt build json: %#v", response)
	}
	if response.Manifest.Artifacts.Prompt != "contract/prompt.md" || response.Manifest.Artifacts.ExecutorContext != "context/executor-context.md" {
		t.Fatalf("json manifest artifact refs mismatch: %#v", response.Manifest.Artifacts)
	}
	if strings.Contains(stdout.String(), "Executor prompt built") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestPromptShowBeforeBuildPrintsGuidance(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt show before build exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Executor prompt has not been built. Run: pactum prompt build "+runID) {
		t.Fatalf("prompt show before build output mismatch:\n%s", got)
	}
}

func TestPromptShowAfterBuildPrintsPrompt(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt show exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Executor Prompt",
		"Run: " + runID,
		"Status: contract_approved",
		"# Executor Prompt",
		"add deterministic prompt boundary",
		"## Validation commands\n- go test ./...",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt show output missing %q:\n%s", want, got)
		}
	}
}

func TestPromptShowJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPrompt(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt show --json exited %d, stderr: %s", code, stderr.String())
	}
	var response promptShowResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.RunID != runID || response.RunStatus != "contract_approved" || response.Manifest.Status != "ready" {
		t.Fatalf("unexpected prompt show json: %#v", response)
	}
	if !strings.Contains(response.Prompt, "add deterministic prompt boundary") {
		t.Fatalf("json prompt missing goal:\n%s", response.Prompt)
	}
	if strings.Contains(stdout.String(), "Executor Prompt\nRun:") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestContractReviseApprovedContractRemovesPromptReadiness(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertFile(t, runPaths.PromptManifest)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--add-in-scope", "Update prompt manifest invalidation"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
	}
	assertNoFile(t, runPaths.PromptManifest)
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil {
		t.Fatalf("approval should be pending after revise: %#v", approval)
	}
	if got := mustReadFile(t, runPaths.PromptMD); !strings.Contains(got, "This prompt is not executable yet.") {
		t.Fatalf("prompt.md should return to preview after revise:\n%s", got)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt show after revise exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Executor prompt has not been built. Run: pactum prompt build "+runID) {
		t.Fatalf("prompt show after revise output mismatch:\n%s", got)
	}
}

func TestClarifyAfterApprovedPromptBuildRemovesPromptReadiness(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertFile(t, runPaths.PromptManifest)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "ask", runID, "Does this reset readiness?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
	}
	assertNoFile(t, runPaths.PromptManifest)
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil {
		t.Fatalf("approval should be pending after clarify: %#v", approval)
	}
	if state := readRunState(t, runPaths.RunJSON); state.Status != "clarifying" {
		t.Fatalf("run status = %q, want clarifying", state.Status)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt show after clarify exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Executor prompt has not been built. Run: pactum prompt build "+runID) {
		t.Fatalf("prompt show after clarify output mismatch:\n%s", got)
	}
}

func TestPromptArtifactsUseRepoRelativePaths(t *testing.T) {
	root := t.TempDir()
	_, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	for name, content := range map[string]string{
		"contract/prompt.md":            mustReadFile(t, runPaths.PromptMD),
		"contract/prompt-manifest.json": mustReadFile(t, runPaths.PromptManifest),
		"context/executor-context.md":   mustReadFile(t, runPaths.ExecutorContext),
		"contract/contract.json":        mustReadFile(t, runPaths.ContractJSON),
		"contract/approval.json":        mustReadFile(t, runPaths.ApprovalJSON),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestPromptBuildWritesLedgerEvent(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}
	events := strings.Join(readLines(t, paths.EventsJSONL), "\n")
	if !strings.Contains(events, "executor_prompt_built") || !strings.Contains(events, runID) {
		t.Fatalf("events missing executor_prompt_built for %s:\n%s", runID, events)
	}
}

func setupApprovedPromptContract(t *testing.T, root string) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	commands := [][]string{
		{"clarify", "ask", runID, "Should prompt build require approval?", "--blocking"},
		{"clarify", "answer", runID, "q_001", "Yes. Prompt build must be approved first."},
		{
			"contract", "revise", runID,
			"--goal", "add deterministic prompt boundary",
			"--add-in-scope", "Add prompt build and prompt show commands",
			"--add-in-scope", "Require approved contract before prompt build",
			"--add-out-of-scope", "Agent execution",
			"--add-acceptance", "Prompt build writes deterministic prompt boundary artifacts",
			"--add-validation", "go test ./...",
			"--add-assumption", "Existing contract approval flow remains the readiness source",
		},
		{"contract", "approve", runID},
	}
	for _, args := range commands {
		stdout.Reset()
		stderr.Reset()
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}
	return app, paths, runID
}

func setupApprovedAndBuiltPrompt(t *testing.T, root string) (App, artifacts.Paths, string) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}
	return app, paths, runID
}

func readPromptManifestForTest(t *testing.T, path string) promptManifest {
	t.Helper()
	manifest, err := readPromptManifest(path)
	assertNoError(t, err)
	return manifest
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s should not exist", path)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
