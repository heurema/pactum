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

	paths := artifacts.New(root)
	runDir := filepath.Join(paths.RunsDir, runID)
	info, err := os.Stat(runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("run not found: %s", runID)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("run not found: %s", runID)
	}

	runPaths := contractRunPaths(runDir)
	state, err := readContractRunState(runPaths.RunJSON)
	if err != nil {
		return err
	}
	contract, err := readDraftContract(runPaths.ContractJSON)
	if err != nil {
		return err
	}
	approval, err := readApprovalState(runPaths.ApprovalJSON)
	if err != nil {
		return err
	}
	if !isRegularFile(runPaths.PromptManifest) {
		return fmt.Errorf("cannot prepare execution: executor prompt has not been built")
	}

	manifest, err := readPromptManifest(runPaths.PromptManifest)
	if err != nil {
		return err
	}
	if manifest.Status != "ready" {
		return fmt.Errorf("cannot prepare execution: executor prompt has not been built")
	}
	if contract.Status != "approved" || approval.Status != "approved" || approval.ContractSHA256 == nil {
		return fmt.Errorf("cannot prepare execution: contract is not approved")
	}
	hash, err := fileSHA256(runPaths.ContractJSON)
	if err != nil {
		return err
	}
	if hash != *approval.ContractSHA256 || manifest.ContractSHA256 != hash {
		return fmt.Errorf("cannot prepare execution: approved contract hash does not match current contract")
	}

	report, err := a.workspaceStatus(root)
	if err != nil {
		return err
	}
	if report.ProjectMap.Status != "fresh" {
		return fmt.Errorf("cannot prepare execution: project map is stale")
	}

	if _, err := os.ReadFile(runPaths.PromptMD); err != nil {
		return err
	}
	if _, err := os.ReadFile(runPaths.ExecutorContext); err != nil {
		return err
	}

	config, err := readConfig(paths.Config)
	if err != nil {
		return err
	}
	resolvedAgentName, adapter, err := agents.ResolveAdapter(config.Agents, agentName)
	if err != nil {
		return err
	}

	now := a.nowUTC()
	plan, err := agents.BuildDryRunPlan(runID, now.Format(time.RFC3339), resolvedAgentName, adapter)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(runPaths.ExecuteDir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(runPaths.DryRunJSON, plan); err != nil {
		return err
	}
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "execution_dry_run_prepared", Timestamp: now, RunID: runID, RepoRoot: root}); err != nil {
		return err
	}

	if jsonOutput {
		return writeJSONResponse(stdout, plan)
	}
	writeExecuteDryRun(stdout, state, plan)
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
	return strings.Join(parts, " ")
}
