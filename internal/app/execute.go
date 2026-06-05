package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

func (a App) ExecuteDryRun(stdout io.Writer, runID string, agentName string, jsonOutput bool) error {
	root, _, ok, err := a.requireWorkspace(stdout, false)
	if err != nil || !ok {
		return err
	}

	prep, err := a.prepareExecution(root, runID, agentName)
	if err != nil {
		return err
	}
	now := a.nowUTC()
	plan, err := agents.BuildDryRunPlan(runID, now.Format(time.RFC3339), prep.Agent, executionPromptRepoPath(runID))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(prep.RunPaths.ExecuteDir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(prep.RunPaths.DryRunJSON, plan); err != nil {
		return err
	}
	if err := ledger.Append(prep.Paths.EventsJSONL, ledger.Event{Type: "execution_dry_run_prepared", Timestamp: now, RunID: runID, RepoRoot: root}); err != nil {
		return err
	}

	if jsonOutput {
		return writeJSONResponse(stdout, plan)
	}
	writeExecuteDryRun(stdout, prep.State, plan)
	return nil
}

func (a App) ExecuteRun(stdout io.Writer, runID string, agentName string, timeout time.Duration, confirm bool, jsonOutput bool) error {
	root, _, ok, err := a.requireWorkspace(stdout, false)
	if err != nil || !ok {
		return err
	}

	prep, err := a.prepareExecution(root, runID, agentName)
	if err != nil {
		return err
	}

	// Direct execution runs an external agent in the repository with no sandbox.
	// Require an explicit confirmation (interactive prompt or --yes) first.
	if !confirm {
		proceed, err := confirmDirectExecution(stdout)
		if err != nil {
			return err
		}
		if !proceed {
			return fmt.Errorf("execution cancelled")
		}
	}

	now := a.nowUTC()
	plan, err := ensureDryRunPlan(prep, now.Format(time.RFC3339))
	if err != nil {
		return err
	}

	attemptID, err := nextAttemptID(prep.RunPaths.AttemptsDir)
	if err != nil {
		return err
	}
	attemptPaths := executionAttemptPaths(prep.RunPaths, attemptID)
	if err := os.MkdirAll(attemptPaths.Dir, 0o755); err != nil {
		return err
	}

	promptRepoPath := executionPromptRepoPath(runID)
	request := executionRequestDocument{
		Schema:         executionRequestSchema,
		RunID:          runID,
		AttemptID:      attemptID,
		CreatedAt:      now.Format(time.RFC3339),
		ContractSHA256: prep.ContractSHA256,
		Agent: agents.AgentDescriptor{
			Name:    prep.Agent.Name,
			Command: prep.Agent.Command,
			Args:    append([]string{}, prep.Agent.Args...),
			Input:   prep.Agent.Input,
		},
		Artifacts: plan.Artifacts,
		WouldRun: agents.DryRunCommand{
			Command: plan.WouldRun.Command,
			Args:    append([]string{}, plan.WouldRun.Args...),
			Stdin:   plan.WouldRun.Stdin,
		},
	}
	if err := writeJSON(attemptPaths.RequestJSON, request); err != nil {
		return err
	}
	if err := ledger.Append(prep.Paths.EventsJSONL, ledger.Event{Type: "execution_attempt_started", Timestamp: now, RunID: runID, RepoRoot: root}); err != nil {
		return err
	}

	runResult, runErr := agents.RunSubprocess(agents.RunRequest{
		RepoRoot:       root,
		RunID:          runID,
		AttemptID:      attemptID,
		Agent:          prep.Agent,
		PromptRepoPath: promptRepoPath,
		Timeout:        timeout,
	})
	if runErr != nil && runResult.StartedAt == "" {
		return runErr
	}
	result := executionResultFromRunResult(runID, attemptID, runResult)
	if err := writeJSON(attemptPaths.ResultJSON, result); err != nil {
		return err
	}
	if err := writeJSON(prep.RunPaths.LastResultJSON, result); err != nil {
		return err
	}
	if err := ledger.Append(prep.Paths.EventsJSONL, ledger.Event{Type: "execution_attempt_finished", Timestamp: executionResultTimestamp(result, now), RunID: runID, RepoRoot: root}); err != nil {
		return err
	}

	if jsonOutput {
		if err := writeJSONResponse(stdout, result); err != nil {
			return err
		}
	} else {
		writeExecuteRun(stdout, prep.State, request, result)
	}
	if runErr != nil {
		if result.TimedOut {
			return fmt.Errorf("agent process timed out after %s", timeout)
		}
		return processExitError{Kind: "agent", ExitCode: result.ExitCode}
	}
	return nil
}

type executionPreparation struct {
	Root           string
	Paths          artifacts.Paths
	RunPaths       contractRunPathSet
	State          contractRunState
	ContractSHA256 string
	Agent          agents.AgentDescriptor
}

func (a App) prepareExecution(root string, runID string, agentName string) (executionPreparation, error) {
	paths := artifacts.New(root)
	runDir := filepath.Join(paths.RunsDir, runID)
	info, err := os.Stat(runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return executionPreparation{}, fmt.Errorf("run not found: %s", runID)
		}
		return executionPreparation{}, err
	}
	if !info.IsDir() {
		return executionPreparation{}, fmt.Errorf("run not found: %s", runID)
	}

	runPaths := contractRunPaths(runDir)
	state, err := readContractRunState(runPaths.RunJSON)
	if err != nil {
		return executionPreparation{}, err
	}
	contract, err := readDraftContract(runPaths.ContractJSON)
	if err != nil {
		return executionPreparation{}, err
	}
	approval, err := readApprovalState(runPaths.ApprovalJSON)
	if err != nil {
		return executionPreparation{}, err
	}
	if !isRegularFile(runPaths.PromptManifest) {
		return executionPreparation{}, fmt.Errorf("cannot prepare execution: executor prompt has not been built")
	}

	manifest, err := readPromptManifest(runPaths.PromptManifest)
	if err != nil {
		return executionPreparation{}, err
	}
	if manifest.Status != "ready" {
		return executionPreparation{}, fmt.Errorf("cannot prepare execution: executor prompt has not been built")
	}
	hash, err := verifyApprovedContract(runPaths, contract, approval, "prepare execution")
	if err != nil {
		return executionPreparation{}, err
	}
	if manifest.ContractSHA256 != hash {
		return executionPreparation{}, fmt.Errorf("cannot prepare execution: approved contract hash does not match current contract")
	}

	report, err := a.workspaceStatus(root)
	if err != nil {
		return executionPreparation{}, err
	}
	if report.ProjectMap.Status != "fresh" {
		return executionPreparation{}, fmt.Errorf("cannot prepare execution: project map is stale")
	}
	if manifest.MapRunID != report.ProjectMap.RunID {
		return executionPreparation{}, fmt.Errorf("cannot prepare execution: executor prompt was built for a different project map")
	}

	if _, err := os.ReadFile(runPaths.PromptMD); err != nil {
		return executionPreparation{}, err
	}
	if _, err := os.ReadFile(runPaths.ExecutorContext); err != nil {
		return executionPreparation{}, err
	}
	if err := verifyExecutionMemoryBoundary(paths, runPaths, manifest); err != nil {
		return executionPreparation{}, err
	}

	agent, err := a.agentRegistry().ResolveExecutor(agentName)
	if err != nil {
		return executionPreparation{}, err
	}
	config, err := readConfig(paths.Config)
	if err != nil {
		return executionPreparation{}, err
	}
	modelSpec, err := agents.ParseModelSpec(config.Agents.ExecutorModel)
	if err != nil {
		return executionPreparation{}, err
	}
	agent, err = agents.ApplyExecutorModelSpec(agent, modelSpec)
	if err != nil {
		return executionPreparation{}, err
	}
	return executionPreparation{
		Root:           root,
		Paths:          paths,
		RunPaths:       runPaths,
		State:          state,
		ContractSHA256: hash,
		Agent:          agent,
	}, nil
}

// verifyExecutionMemoryBoundary refuses execution when the accepted-memory
// boundary recorded at prompt build no longer matches the current workspace.
// Run-local memory artifacts and the global accepted-memory source files are
// both validated by hash. Stale-but-unchanged memory is allowed.
func verifyExecutionMemoryBoundary(paths artifacts.Paths, runPaths contractRunPathSet, manifest promptManifest) error {
	if manifest.Memory == nil {
		return fmt.Errorf("cannot prepare execution: executor prompt memory metadata is missing")
	}
	if !isRegularFile(runPaths.MemoryContextMD) || !isRegularFile(runPaths.MemorySelectionJSON) {
		return fmt.Errorf("cannot prepare execution: memory context changed after prompt build")
	}
	contextHash, err := fileSHA256(runPaths.MemoryContextMD)
	if err != nil {
		return err
	}
	selectionHash, err := fileSHA256(runPaths.MemorySelectionJSON)
	if err != nil {
		return err
	}
	if contextHash != manifest.Memory.ContextSHA256 || selectionHash != manifest.Memory.SelectionSHA256 {
		return fmt.Errorf("cannot prepare execution: memory context changed after prompt build")
	}
	sourceHash, err := memorySourceSHA256(paths)
	if err != nil {
		return err
	}
	if sourceHash != manifest.Memory.SourceSHA256 {
		return fmt.Errorf("cannot prepare execution: accepted memory changed after prompt build")
	}
	return nil
}

func writeExecuteDryRun(stdout io.Writer, state contractRunState, plan agents.DryRunPlan) {
	fmt.Fprintln(stdout, "Execution dry-run prepared")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", plan.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", state.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Agent:")
	fmt.Fprintf(stdout, "  name: %s\n", plan.Agent.Name)
	fmt.Fprintf(stdout, "  command: %s\n", plan.Agent.Command)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Checks:")
	fmt.Fprintln(stdout, "  prompt manifest: ready")
	fmt.Fprintln(stdout, "  contract hash: ok")
	fmt.Fprintln(stdout, "  project map: fresh")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Would run:")
	fmt.Fprintf(stdout, "  %s\n", formatAgentCommand(plan.WouldRun))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  dry run: %s\n", runArtifactRepoRel(plan.RunID, agents.DryRunArtifactDryRun))
	fmt.Fprintf(stdout, "  prompt: %s\n", runArtifactRepoRel(plan.RunID, plan.Artifacts.Prompt))
	fmt.Fprintf(stdout, "  executor context: %s\n", runArtifactRepoRel(plan.RunID, plan.Artifacts.ExecutorContext))
}

func formatAgentCommand(command agents.DryRunCommand) string {
	parts := append([]string{command.Command}, command.Args...)
	if command.Stdin != "" {
		parts = append(parts, "<", command.Stdin)
	}
	return strings.Join(parts, " ")
}

const (
	executionRequestSchema = "pactum.execution_request.v1"
	executionResultSchema  = "pactum.execution_result.v1"
)

type executionRequestDocument struct {
	Schema         string                 `json:"schema"`
	RunID          string                 `json:"run_id"`
	AttemptID      string                 `json:"attempt_id"`
	CreatedAt      string                 `json:"created_at"`
	ContractSHA256 string                 `json:"contract_sha256,omitempty"`
	Agent          agents.AgentDescriptor `json:"agent"`
	Artifacts      agents.DryRunArtifacts `json:"artifacts"`
	WouldRun       agents.DryRunCommand   `json:"would_run"`
}

type executionResultDocument struct {
	Schema    string `json:"schema"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	processResult
}

func ensureDryRunPlan(prep executionPreparation, createdAt string) (agents.DryRunPlan, error) {
	expected, err := agents.BuildDryRunPlan(prep.State.RunID, createdAt, prep.Agent, executionPromptRepoPath(prep.State.RunID))
	if err != nil {
		return agents.DryRunPlan{}, err
	}
	if isRegularFile(prep.RunPaths.DryRunJSON) {
		var existing agents.DryRunPlan
		if err := readJSON(prep.RunPaths.DryRunJSON, &existing); err == nil && dryRunPlanMatches(existing, expected) {
			return existing, nil
		}
	}
	if err := os.MkdirAll(prep.RunPaths.ExecuteDir, 0o755); err != nil {
		return agents.DryRunPlan{}, err
	}
	if err := writeJSON(prep.RunPaths.DryRunJSON, expected); err != nil {
		return agents.DryRunPlan{}, err
	}
	return expected, nil
}

func dryRunPlanMatches(got agents.DryRunPlan, want agents.DryRunPlan) bool {
	return got.Schema == want.Schema &&
		got.RunID == want.RunID &&
		got.Agent.Name == want.Agent.Name &&
		got.Agent.Command == want.Agent.Command &&
		got.Agent.Input == want.Agent.Input &&
		sameStringSlice(got.Agent.Args, want.Agent.Args) &&
		got.Checks == want.Checks &&
		got.Artifacts == want.Artifacts &&
		got.WouldRun.Command == want.WouldRun.Command &&
		sameStringSlice(got.WouldRun.Args, want.WouldRun.Args) &&
		got.WouldRun.Stdin == want.WouldRun.Stdin
}

func sameStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func nextAttemptID(attemptsDir string) (string, error) {
	entries, err := os.ReadDir(attemptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "attempt_001", nil
		}
		return "", err
	}
	maxAttempt := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var number int
		if _, err := fmt.Sscanf(entry.Name(), "attempt_%03d", &number); err == nil && number > maxAttempt {
			maxAttempt = number
		}
	}
	return fmt.Sprintf("attempt_%03d", maxAttempt+1), nil
}

func executionAttemptPaths(runPaths contractRunPathSet, attemptID string) attemptPathSet {
	return newAttemptPaths(filepath.Join(runPaths.AttemptsDir, attemptID))
}

func executionPromptRepoPath(runID string) string {
	return filepath.ToSlash(filepath.Join(artifacts.WorkspaceRel, "runs", runID, agents.DryRunArtifactPrompt))
}

func executionResultFromRunResult(runID string, attemptID string, result agents.RunResult) executionResultDocument {
	return executionResultDocument{
		Schema:        executionResultSchema,
		RunID:         runID,
		AttemptID:     attemptID,
		processResult: processResultFromRunResult(result),
	}
}

func executionResultTimestamp(result executionResultDocument, fallback time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, result.FinishedAt); err == nil {
		return parsed
	}
	return fallback
}

func writeExecuteRun(stdout io.Writer, state contractRunState, request executionRequestDocument, result executionResultDocument) {
	fmt.Fprintln(stdout, "Execution attempt finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", result.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", state.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", result.AttemptID)
	fmt.Fprintf(stdout, "  agent: %s\n", request.Agent.Name)
	fmt.Fprintf(stdout, "  command: %s\n", formatAgentCommand(request.WouldRun))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Result:")
	fmt.Fprintf(stdout, "  exit code: %d\n", result.ExitCode)
	fmt.Fprintf(stdout, "  timed out: %t\n", result.TimedOut)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  request: %s\n", runArtifactRepoRel(result.RunID, filepath.ToSlash(filepath.Join("execute", "attempts", result.AttemptID, "request.json"))))
	fmt.Fprintf(stdout, "  result: %s\n", runArtifactRepoRel(result.RunID, filepath.ToSlash(filepath.Join("execute", "attempts", result.AttemptID, "result.json"))))
	fmt.Fprintf(stdout, "  stdout: %s\n", runArtifactRepoRel(result.RunID, result.Stdout))
	fmt.Fprintf(stdout, "  stderr: %s\n", runArtifactRepoRel(result.RunID, result.Stderr))
	fmt.Fprintf(stdout, "  last result: %s\n", runArtifactRepoRel(result.RunID, "execute/last-result.json"))
}
