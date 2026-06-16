package app

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

func (a App) ExecutePlan(stdout io.Writer, runID string, agentName string, jsonOutput bool) error {
	root, _, ok, err := a.requireWorkspace(stdout, false)
	if err != nil || !ok {
		return err
	}

	prep, err := a.prepareExecution(root, runID, agentName)
	if err != nil {
		return err
	}
	now := a.nowUTC()
	plan, err := agents.BuildDryRunPlan(runID, now.Format(time.RFC3339), prep.Agent, prep.ModelSpec, false, executionPromptRepoPath(runID))
	if err != nil {
		return err
	}
	if err := activeStore.MkdirAll(prep.RunPaths.ExecuteDir); err != nil {
		return err
	}
	if err := writeJSON(prep.RunPaths.DryRunJSON, plan); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, prep.Paths.EventsJSONL, ledger.Event{Type: "execution_dry_run_prepared", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	if jsonOutput {
		// Running the executor is human-approved, so the plan has no safe next.
		return writeJSONResponse(stdout, executePlanResponse{DryRunPlan: plan, Next: []string{}})
	}
	writeExecutePlan(stdout, prep.State, plan, prep.AgentName, prep.ModelSpec)
	return nil
}

// executePlanResponse is the dry-run plan plus the next affordance; the plan
// artifact on disk stays unchanged.
type executePlanResponse struct {
	agents.DryRunPlan
	Next []string `json:"next"`
}

// executeRunResponse is the attempt result plus the next affordance; the
// attempt artifacts on disk stay unchanged.
type executeRunResponse struct {
	executionResultDocument
	Next []string `json:"next"`
}

func (a App) ExecuteRun(stdout io.Writer, liveOutput io.Writer, runID string, agentName string, timeout time.Duration, jsonOutput bool) error {
	root, _, ok, err := a.requireWorkspace(stdout, false)
	if err != nil || !ok {
		return err
	}

	prep, err := a.prepareExecution(root, runID, agentName)
	if err != nil {
		return err
	}
	timeout, err = resolveIdleTimeout(prep.Paths.Config, timeout)
	if err != nil {
		return err
	}

	promptRepoPath := executionPromptRepoPath(runID)
	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[agents.DryRunPlan, executionRequestDocument, executionResultDocument, struct{}]{
		Stdout:           stdout,
		LiveOutput:       liveOutput,
		JSONOutput:       jsonOutput,
		Root:             root,
		EventsJSONL:      prep.Paths.EventsJSONL,
		RunID:            runID,
		Stage:            "execute",
		AttemptsDir:      prep.RunPaths.AttemptsDir,
		AttemptIDPrefix:  "attempt",
		LastResultJSON:   prep.RunPaths.LastResultJSON,
		AgentName:        prep.AgentName,
		Agent:            prep.Agent,
		Model:            prep.ModelSpec,
		PromptRepoPath:   promptRepoPath,
		Timeout:          timeout,
		WritePathAllowed: contractWritePathAllowed(prep.Contract),
		StartedEvent:     "execution_attempt_started",
		FinishedEvent:    "execution_attempt_finished",
		ExitKind:         "agent",
		TimeoutMessage: func(timeout time.Duration) string {
			return fmt.Sprintf("agent process produced no output for %s", timeout)
		},
		Prepare: func(createdAt string) (agents.DryRunPlan, error) {
			return ensureDryRunPlan(prep, createdAt)
		},
		BuildRequest: func(context agentAttemptContext[agents.DryRunPlan]) (executionRequestDocument, error) {
			return executionRequestDocument{
				Schema:         executionRequestSchema,
				RunID:          runID,
				AttemptID:      context.AttemptID,
				CreatedAt:      context.CreatedAt,
				ContractSHA256: prep.ContractSHA256,
				Agent:          agentDescriptorDocument(prep.Agent),
				Artifacts:      context.Prepared.Artifacts,
				WouldRun: agents.DryRunCommand{
					Command: context.Prepared.WouldRun.Command,
					Args:    append([]string{}, context.Prepared.WouldRun.Args...),
					Stdin:   context.Prepared.WouldRun.Stdin,
					Env:     append([]string{}, context.Prepared.WouldRun.Env...),
				},
			}, nil
		},
		BuildResult: func(context agentAttemptContext[agents.DryRunPlan], runResult agents.RunResult) executionResultDocument {
			return executionResultDocument{
				Schema:        executionResultSchema,
				RunID:         runID,
				AttemptID:     context.AttemptID,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result executionResultDocument) processResult {
			return result.processResult
		},
		RenderRunOnly: func(stdout io.Writer, request executionRequestDocument, result executionResultDocument) {
			writeExecuteRun(stdout, prep.State, request, result, prep.AgentName, prep.ModelSpec)
		},
		WrapRunOnly: func(result executionResultDocument) any {
			next := []string{}
			if result.ExitCode == 0 && (!result.TimedOut || result.CompletedDespiteTimeout) {
				next = nextCommandsForRun(prep.Paths, runID)
			}
			return executeRunResponse{executionResultDocument: result, Next: next}
		},
	})
}

type executionPreparation struct {
	Root           string
	Paths          artifacts.Paths
	RunPaths       contractRunPathSet
	State          contractRunState
	Contract       draftContract
	ContractSHA256 string
	// AgentName is the registry name the run was invoked under; Agent is the
	// underlying built-in's descriptor with the entry's pins applied.
	AgentName string
	Agent     agents.AgentDescriptor
	ModelSpec agents.ModelSpec
}

func (a App) prepareExecution(root string, runID string, agentName string) (executionPreparation, error) {
	paths := artifacts.New(root)
	runDir := filepath.Join(paths.RunsDir, runID)
	runDirExists, err := storeDirExists(runDir)
	if err != nil {
		return executionPreparation{}, err
	}
	if !runDirExists {
		return executionPreparation{}, runNotFoundError(runID)
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
		return executionPreparation{}, promptNotBuiltError("prepare execution", runID)
	}

	manifest, err := readPromptManifest(runPaths.PromptManifest)
	if err != nil {
		return executionPreparation{}, err
	}
	if manifest.Status != "ready" {
		return executionPreparation{}, promptNotBuiltError("prepare execution", runID)
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
		return executionPreparation{}, projectMapStaleError("prepare execution")
	}
	if manifest.MapRunID != report.ProjectMap.RunID {
		return executionPreparation{}, fmt.Errorf("cannot prepare execution: executor prompt was built for a different project map")
	}

	if _, err := activeStore.ReadBytes(runPaths.PromptMD); err != nil {
		return executionPreparation{}, err
	}
	if _, err := activeStore.ReadBytes(runPaths.ExecutorContext); err != nil {
		return executionPreparation{}, err
	}
	if err := verifyExecutionMemoryBoundary(paths, runPaths, manifest); err != nil {
		return executionPreparation{}, err
	}

	config, err := readConfig(paths.Config)
	if err != nil {
		return executionPreparation{}, err
	}
	entry, err := resolveExecutorEntry(config, agentName)
	if err != nil {
		return executionPreparation{}, err
	}
	resolved, err := a.resolveAgentForRole(entry, agentRoleExecutor)
	if err != nil {
		return executionPreparation{}, err
	}
	return executionPreparation{
		Root:           root,
		Paths:          paths,
		RunPaths:       runPaths,
		State:          state,
		Contract:       contract,
		ContractSHA256: hash,
		AgentName:      resolved.Name,
		Agent:          resolved.Agent,
		ModelSpec:      resolved.ModelSpec,
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
	contextHash, err := storeFileSHA256(runPaths.MemoryContextMD)
	if err != nil {
		return err
	}
	selectionHash, err := storeFileSHA256(runPaths.MemorySelectionJSON)
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

func writeExecutePlan(stdout io.Writer, state contractRunState, plan agents.DryRunPlan, agentName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Execution plan prepared")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", plan.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", state.Status)
	fmt.Fprintln(stdout)
	writeResolved(stdout, agentName, modelSpec)
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
	fmt.Fprintf(stdout, "  plan: %s\n", runArtifactRepoRel(plan.RunID, agents.DryRunArtifactDryRun))
	fmt.Fprintf(stdout, "  prompt: %s\n", runArtifactRepoRel(plan.RunID, plan.Artifacts.Prompt))
	fmt.Fprintf(stdout, "  executor context: %s\n", runArtifactRepoRel(plan.RunID, plan.Artifacts.ExecutorContext))
}

func formatAgentCommand(command agents.DryRunCommand) string {
	var parts []string
	parts = append(parts, command.Env...)
	parts = append(parts, command.Command)
	parts = append(parts, command.Args...)
	if command.Stdin != "" {
		parts = append(parts, "<", command.Stdin)
	}
	return strings.Join(parts, " ")
}

func writeResolved(stdout io.Writer, agentName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Resolved:")
	fmt.Fprintf(stdout, "  agent: %s\n", agentName)
	fmt.Fprintf(stdout, "  model: %s\n", inheritedValue(modelSpec.Model))
	fmt.Fprintf(stdout, "  effort: %s\n", inheritedValue(modelSpec.Effort))
	fmt.Fprintf(stdout, "  pinning: %s\n", pinningMode(modelSpec))
}

func inheritedValue(value string) string {
	if value == "" {
		return "inherit"
	}
	return value
}

func pinningMode(modelSpec agents.ModelSpec) string {
	switch {
	case modelSpec.Model == "" && modelSpec.Effort == "":
		return "inherit"
	case modelSpec.Model != "" && modelSpec.Effort != "":
		return "pinned"
	default:
		// Exactly one of model/effort is set; the other still inherits, so
		// reporting "pinned" would be misleading.
		return "partial"
	}
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
	expected, err := agents.BuildDryRunPlan(prep.State.RunID, createdAt, prep.Agent, prep.ModelSpec, false, executionPromptRepoPath(prep.State.RunID))
	if err != nil {
		return agents.DryRunPlan{}, err
	}
	if isRegularFile(prep.RunPaths.DryRunJSON) {
		var existing agents.DryRunPlan
		if err := readJSON(prep.RunPaths.DryRunJSON, &existing); err == nil && dryRunPlanMatches(existing, expected) {
			return existing, nil
		}
	}
	if err := activeStore.MkdirAll(prep.RunPaths.ExecuteDir); err != nil {
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

func executionAttemptPaths(runPaths contractRunPathSet, attemptID string) attemptPathSet {
	return agentAttemptPaths(runPaths.AttemptsDir, attemptID)
}

func executionPromptRepoPath(runID string) string {
	return filepath.ToSlash(filepath.Join(artifacts.WorkspaceRel, "runs", runID, agents.DryRunArtifactPrompt))
}

func writeExecuteRun(stdout io.Writer, state contractRunState, request executionRequestDocument, result executionResultDocument, agentName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Execution attempt finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", result.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", state.Status)
	fmt.Fprintln(stdout)
	writeResolved(stdout, agentName, modelSpec)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", result.AttemptID)
	fmt.Fprintf(stdout, "  agent: %s\n", request.Agent.Name)
	fmt.Fprintf(stdout, "  command: %s\n", formatAgentCommand(request.WouldRun))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Result:")
	fmt.Fprintf(stdout, "  exit code: %d\n", result.ExitCode)
	fmt.Fprintf(stdout, "  timed out: %t\n", result.TimedOut)
	writeCompletedDespiteTimeoutWarning(stdout, result.processResult)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  request: %s\n", runArtifactRepoRel(result.RunID, filepath.ToSlash(filepath.Join("execute", "attempts", result.AttemptID, "request.json"))))
	fmt.Fprintf(stdout, "  result: %s\n", runArtifactRepoRel(result.RunID, filepath.ToSlash(filepath.Join("execute", "attempts", result.AttemptID, "result.json"))))
	fmt.Fprintf(stdout, "  stdout: %s\n", runArtifactRepoRel(result.RunID, result.Stdout))
	fmt.Fprintf(stdout, "  stderr: %s\n", runArtifactRepoRel(result.RunID, result.Stderr))
	fmt.Fprintf(stdout, "  last result: %s\n", runArtifactRepoRel(result.RunID, "execute/last-result.json"))
}
