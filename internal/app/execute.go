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
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
		return nil
	}

	prep, err := a.prepareExecution(root, runID, agentName)
	if err != nil {
		return err
	}
	now := a.nowUTC()
	plan, err := agents.BuildDryRunPlan(runID, now.Format(time.RFC3339), prep.AgentName, prep.Adapter)
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

func (a App) ExecuteRun(stdout io.Writer, runID string, agentName string, allowExecute bool, timeout time.Duration, jsonOutput bool) error {
	if !allowExecute {
		fmt.Fprintln(stdout, "Refusing to execute external agent without --allow-execute.")
		return nil
	}

	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
		return nil
	}

	prep, err := a.prepareExecution(root, runID, agentName)
	if err != nil {
		return err
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
		Schema:    executionRequestSchema,
		RunID:     runID,
		AttemptID: attemptID,
		CreatedAt: now.Format(time.RFC3339),
		Agent: executionRequestAgent{
			Name:    prep.AgentName,
			Command: prep.Adapter.Command,
			Args:    append([]string{}, prep.Adapter.Args...),
			Input:   prep.Adapter.Input,
		},
		Artifacts: plan.Artifacts,
		WouldRun: agents.DryRunCommand{
			Command: prep.Adapter.Command,
			Args:    append(append([]string{}, prep.Adapter.Args...), "--", promptRepoPath),
		},
	}
	if err := writeJSON(attemptPaths.RequestJSON, request); err != nil {
		return err
	}

	runResult, runErr := agents.RunSubprocess(agents.RunRequest{
		RepoRoot:       root,
		RunID:          runID,
		AttemptID:      attemptID,
		AgentName:      prep.AgentName,
		Adapter:        prep.Adapter,
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
	if err := ledger.Append(prep.Paths.EventsJSONL, ledger.Event{Type: "execution_attempt_completed", Timestamp: executionResultTimestamp(result, now), RunID: runID, RepoRoot: root}); err != nil {
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
		return executionProcessError{ExitCode: result.ExitCode}
	}
	return nil
}

type executionPreparation struct {
	Root      string
	Paths     artifacts.Paths
	RunPaths  contractRunPathSet
	State     contractRunState
	AgentName string
	Adapter   agents.AdapterConfig
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
	if contract.Status != "approved" || approval.Status != "approved" || approval.ContractSHA256 == nil {
		return executionPreparation{}, fmt.Errorf("cannot prepare execution: contract is not approved")
	}
	hash, err := fileSHA256(runPaths.ContractJSON)
	if err != nil {
		return executionPreparation{}, err
	}
	if hash != *approval.ContractSHA256 || manifest.ContractSHA256 != hash {
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

	config, err := readConfig(paths.Config)
	if err != nil {
		return executionPreparation{}, err
	}
	resolvedAgentName, adapter, err := agents.ResolveAdapter(config.Agents, agentName)
	if err != nil {
		return executionPreparation{}, err
	}
	return executionPreparation{
		Root:      root,
		Paths:     paths,
		RunPaths:  runPaths,
		State:     state,
		AgentName: resolvedAgentName,
		Adapter:   adapter,
	}, nil
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
	return strings.Join(parts, " ")
}

const (
	executionRequestSchema = "pactum.execution_request.v1"
	executionResultSchema  = "pactum.execution_result.v1"
)

type executionRequestDocument struct {
	Schema    string                 `json:"schema"`
	RunID     string                 `json:"run_id"`
	AttemptID string                 `json:"attempt_id"`
	CreatedAt string                 `json:"created_at"`
	Agent     executionRequestAgent  `json:"agent"`
	Artifacts agents.DryRunArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand   `json:"would_run"`
}

type executionRequestAgent struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Input   string   `json:"input"`
}

type executionResultDocument struct {
	Schema         string `json:"schema"`
	RunID          string `json:"run_id"`
	AttemptID      string `json:"attempt_id"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	DurationMillis int64  `json:"duration_ms"`
	ExitCode       int    `json:"exit_code"`
	TimedOut       bool   `json:"timed_out"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
}

type executionAttemptPathSet struct {
	Dir         string
	RequestJSON string
	StdoutLog   string
	StderrLog   string
	ResultJSON  string
}

type executionProcessError struct {
	ExitCode int
}

func (e executionProcessError) Error() string {
	return fmt.Sprintf("agent process exited with code %d", e.ExitCode)
}

func ensureDryRunPlan(prep executionPreparation, createdAt string) (agents.DryRunPlan, error) {
	expected, err := agents.BuildDryRunPlan(prep.State.RunID, createdAt, prep.AgentName, prep.Adapter)
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
		sameStringSlice(got.WouldRun.Args, want.WouldRun.Args)
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

func executionAttemptPaths(runPaths contractRunPathSet, attemptID string) executionAttemptPathSet {
	dir := filepath.Join(runPaths.AttemptsDir, attemptID)
	return executionAttemptPathSet{
		Dir:         dir,
		RequestJSON: filepath.Join(dir, "request.json"),
		StdoutLog:   filepath.Join(dir, "stdout.log"),
		StderrLog:   filepath.Join(dir, "stderr.log"),
		ResultJSON:  filepath.Join(dir, "result.json"),
	}
}

func executionPromptRepoPath(runID string) string {
	return filepath.ToSlash(filepath.Join(artifacts.WorkspaceRel, "runs", runID, agents.DryRunArtifactPrompt))
}

func executionResultFromRunResult(runID string, attemptID string, result agents.RunResult) executionResultDocument {
	return executionResultDocument{
		Schema:         executionResultSchema,
		RunID:          runID,
		AttemptID:      attemptID,
		StartedAt:      result.StartedAt,
		FinishedAt:     result.FinishedAt,
		DurationMillis: result.DurationMillis,
		ExitCode:       result.ExitCode,
		TimedOut:       result.TimedOut,
		Stdout:         result.StdoutPath,
		Stderr:         result.StderrPath,
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
