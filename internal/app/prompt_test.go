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
	if code != 1 {
		t.Fatalf("prompt build before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("prompt build before init stderr mismatch:\n%s", got)
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
	code := app.Run([]string{"clarify", "add", runID, "Need a decision?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
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

func TestPromptBuildFailsWhenRepoHasNoCommits(t *testing.T) {
	skipIfNoGit(t)
	root := t.TempDir()
	mustGitG(t, root, "init")
	mustGitG(t, root, "config", "user.email", "test@test.com")
	mustGitG(t, root, "config", "user.name", "Test")
	mustGitG(t, root, "config", "commit.gpgsign", "false")
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	app := testApp(root)

	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{
		{"init"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := app.Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}
	setAgentRegistryConfig(t, artifacts.New(root),
		agentRegistryEntry{Name: "codex", Model: "gpt-5"},
	)
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"task", "new", "add feature"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task new exited %d, stderr: %s", code, stderr.String())
	}
	// Approve the contract without ever making a commit — HEAD cannot be resolved.
	paths := artifacts.New(root)
	runID := "run_20260531_184012"
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"goal": "add feature",
		"scope": map[string]any{
			"in":  []string{"internal/app/**"},
			"out": []string{},
		},
		"acceptance_criteria": []string{"Feature works"},
		"validation":          map[string]any{"commands": []string{}},
		"assumptions":         []string{},
	})
	for _, args := range [][]string{
		{"contract", "revise", runID, "--from", fromFile},
		{"contract", "approve", runID},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := app.Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	code := app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("prompt build should fail when repo has no commits")
	}
	remedy := `git add -A && git commit -m "initial commit"`
	if got := stderr.String(); !strings.Contains(got, remedy) {
		t.Fatalf("no-commits prompt build stderr missing remedy:\n%s", got)
	}
	assertNoFile(t, runPaths.PromptManifest)

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "build", runID, "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("prompt build --json should fail when repo has no commits")
	}
	var envelope affordanceEnvelope
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
	if envelope.Error.Code != "repository_has_no_commits" {
		t.Fatalf("code = %q, want repository_has_no_commits", envelope.Error.Code)
	}
	if !strings.Contains(envelope.Error.Message, remedy) {
		t.Fatalf("json message missing remedy:\n%s", envelope.Error.Message)
	}
	if envelope.Error.Fix != nil {
		t.Fatalf("fix = %q, want absent", *envelope.Error.Fix)
	}
}

// TestPromptBuildGitErrorNotMislabeledAsNoCommits confirms the negative path:
// when rev-parse HEAD fails and symbolic-ref also fails (e.g. the working
// directory is not a git repository at all), the error must NOT carry the
// repository_has_no_commits code.
func TestPromptBuildGitErrorNotMislabeledAsNoCommits(t *testing.T) {
	// No git init — the workspace is a plain directory with no .git.
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	app := testApp(root)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	setAgentRegistryConfig(t, artifacts.New(root),
		agentRegistryEntry{Name: "codex", Model: "gpt-5"},
	)
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"task", "new", "add feature"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task new exited %d, stderr: %s", code, stderr.String())
	}
	paths := artifacts.New(root)
	runID := "run_20260531_184012"
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"goal": "add feature",
		"scope": map[string]any{
			"in":  []string{"internal/app/**"},
			"out": []string{},
		},
		"acceptance_criteria": []string{"Feature works"},
		"validation":          map[string]any{"commands": []string{}},
		"assumptions":         []string{},
	})
	for _, args := range [][]string{
		{"contract", "revise", runID, "--from", fromFile},
		{"contract", "approve", runID},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := app.Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}

	// No .git directory: rev-parse HEAD and symbolic-ref --quiet HEAD both fail.
	// This must surface as an ordinary git error, not repository_has_no_commits.
	stdout.Reset()
	stderr.Reset()
	code := app.Run([]string{"prompt", "build", runID, "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("prompt build should fail without a git repository")
	}
	var envelope affordanceEnvelope
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
	if envelope.Error.Code == "repository_has_no_commits" {
		t.Fatalf("non-git-repo HEAD failure must not produce code repository_has_no_commits")
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
		".heurema/pactum/runs/" + runID + "/contract/prompt.md",
		".heurema/pactum/runs/" + runID + "/context/executor-context.md",
		".heurema/pactum/runs/" + runID + "/contract/prompt-manifest.json",
		"Next:",
		"pactum execute plan " + runID,
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
	if !manifest.Checks.ContractApproved || !manifest.Checks.ContractHashMatchesApproval || manifest.Checks.BlockingClarificationsOpen != 0 {
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
		"q_001: Yes. Prompt build must be approved first.",
	} {
		if !strings.Contains(executorContext, want) {
			t.Fatalf("executor-context.md missing %q:\n%s", want, executorContext)
		}
	}
	for _, absent := range []string{
		"repo-map.md",
		"llms.txt",
		"search.sqlite",
		"context/search-results.json",
		"Relevant search results",
	} {
		if strings.Contains(executorContext, absent) {
			t.Fatalf("executor-context.md must not contain %q:\n%s", absent, executorContext)
		}
	}
	for _, absent := range []string{"repo-map.md", "llms.txt", "search.sqlite", "context/search-results.json", "pactum search"} {
		if strings.Contains(promptMD, absent) {
			t.Fatalf("prompt.md must not contain %q:\n%s", absent, promptMD)
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

func TestPromptShowBeforeBuildJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt show --json before build exited %d, stderr: %s", code, stderr.String())
	}
	var response promptShowNotBuiltResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Schema != notReadySchema || response.RunID != runID || response.RunStatus == "" || response.Ready {
		t.Fatalf("unexpected prompt show before build json: %#v", response)
	}
	if !strings.Contains(response.Message, "Executor prompt has not been built") || response.SuggestedCommand != "pactum prompt build "+runID || response.Fix != response.SuggestedCommand {
		t.Fatalf("unexpected prompt show before build guidance: %#v", response)
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

func TestContractReapproveRemovesPromptReadiness(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertFile(t, runPaths.PromptManifest)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
	assertNoFile(t, runPaths.PromptManifest)
	assertFile(t, runPaths.ExecutorContext)
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "approved" || approval.ContractSHA256 == nil {
		t.Fatalf("approval should stay approved after re-approve: %#v", approval)
	}
	if state := readRunState(t, runPaths.RunJSON); state.Status != "contract_approved" {
		t.Fatalf("run status = %q, want contract_approved", state.Status)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt show after re-approve exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Executor prompt has not been built. Run: pactum prompt build "+runID) {
		t.Fatalf("prompt show after re-approve output mismatch:\n%s", got)
	}
}

func TestContractReviseApprovedContractRemovesPromptReadiness(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	assertFile(t, runPaths.PromptManifest)

	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"scope": map[string]any{"in": []string{"Update prompt manifest invalidation"}},
	})
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile, "--allow-approval-reset"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
	}
	assertNoFile(t, runPaths.PromptManifest)
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil {
		t.Fatalf("approval should be pending after revise: %#v", approval)
	}
	if got := mustReadFile(t, runPaths.PromptMD); !strings.Contains(got, "This is a contract-draft placeholder.") {
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
	code := app.Run([]string{"clarify", "add", runID, "Does this reset readiness?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
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

// TestWriteStagePromptsShareHouseStyleSection pins the house-style section in
// both write-stage prompts: the shared text must carry the key rules, and both
// the executor prompt and the fixer prompt must contain that exact section, so
// it cannot be silently dropped or forked in either prompt.
func TestWriteStagePromptsShareHouseStyleSection(t *testing.T) {
	var section strings.Builder
	writeHouseStyleSection(&section)
	for _, want := range []string{
		"## House style",
		"Match the surrounding code: idiom, naming, comment density.",
		"Search for and reuse existing helpers before writing new ones.",
		"Keep the diff small and focused: change only what the contract requires.",
		"Simplicity first: no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization.",
		"Over-engineering DON'Ts: wrappers that add nothing, factories or abstractions for a single case, unused extension points, dual implementations where the old path has no callers, silent fallbacks that hide failures.",
		"No dead code, no commented-out code, no unused parameters.",
		"Tests verify behavior, not implementation details, and cover error paths.",
		"Fake-test DON'Ts: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.",
	} {
		if !strings.Contains(section.String(), want) {
			t.Fatalf("house style section missing %q:\n%s", want, section.String())
		}
	}

	executorPrompt := string(renderApprovedPromptMD(draftContract{}, "run_x", "hash", memorySelectionDocument{}))
	fixerPrompt := renderReviewFixPrompt(reviewFixPreparation{})
	for name, prompt := range map[string]string{
		"executor prompt": executorPrompt,
		"fixer prompt":    fixerPrompt,
	} {
		if !strings.Contains(prompt, section.String()) {
			t.Fatalf("%s missing the shared house style section:\n%s", name, prompt)
		}
	}
	if !strings.Contains(fixerPrompt, "The reviewer will re-check your fixes against the discipline rules above.") {
		t.Fatalf("fixer prompt missing the reviewer re-check note:\n%s", fixerPrompt)
	}
	if strings.Contains(executorPrompt, "The reviewer will re-check your fixes against the discipline rules above.") {
		t.Fatalf("reviewer re-check note is fixer-specific and should not be in the executor prompt:\n%s", executorPrompt)
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
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{
		{"clarify", "add", runID, "Should prompt build require approval?", "--blocking"},
		{"clarify", "answer", runID, "q_001", "Yes. Prompt build must be approved first."},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := app.Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}

	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"goal": "add deterministic prompt boundary",
		"scope": map[string]any{
			"in":  []string{"Add prompt build and prompt show commands", "Require approved contract before prompt build"},
			"out": []string{"Agent execution"},
		},
		"acceptance_criteria": []string{"Prompt build writes deterministic prompt boundary artifacts"},
		"validation":          map[string]any{"commands": []string{"go test ./..."}},
		"assumptions":         []string{"Existing contract approval flow remains the readiness source"},
	})
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"contract", "revise", runID, "--from", fromFile}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
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
