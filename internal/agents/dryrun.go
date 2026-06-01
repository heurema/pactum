package agents

import "fmt"

const (
	DryRunSchema                 = "pactum.execute_dry_run.v1"
	DryRunArtifactPrompt         = "contract/prompt.md"
	DryRunArtifactContext        = "context/executor-context.md"
	DryRunArtifactPromptManifest = "contract/prompt-manifest.json"
	DryRunArtifactDryRun         = "execute/dry-run.json"
)

func BuildDryRunPlan(runID string, createdAt string, agentName string, adapter AdapterConfig) (DryRunPlan, error) {
	if adapter.Input != InputPromptFile {
		return DryRunPlan{}, fmt.Errorf("unsupported agent input mode: %s", adapter.Input)
	}

	agentArgs := append([]string{}, adapter.Args...)
	wouldRunArgs := append([]string{}, adapter.Args...)
	wouldRunArgs = append(wouldRunArgs, "--", DryRunArtifactPrompt)

	return DryRunPlan{
		Schema:    DryRunSchema,
		RunID:     runID,
		CreatedAt: createdAt,
		Agent: DryRunAgent{
			Name:    agentName,
			Command: adapter.Command,
			Args:    agentArgs,
			Input:   adapter.Input,
		},
		Checks: DryRunChecks{
			PromptManifestReady:         true,
			ContractHashMatchesApproval: true,
			ProjectMapFresh:             true,
		},
		Artifacts: DryRunArtifacts{
			Prompt:          DryRunArtifactPrompt,
			ExecutorContext: DryRunArtifactContext,
			PromptManifest:  DryRunArtifactPromptManifest,
		},
		WouldRun: DryRunCommand{
			Command: adapter.Command,
			Args:    wouldRunArgs,
		},
	}, nil
}
