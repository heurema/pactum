package agents

import "fmt"

const (
	DryRunSchema                 = "pactum.execute_dry_run.v1"
	DryRunArtifactPrompt         = "contract/prompt.md"
	DryRunArtifactContext        = "context/executor-context.md"
	DryRunArtifactPromptManifest = "contract/prompt-manifest.json"
	DryRunArtifactDryRun         = "execute/dry-run.json"
)

func BuildDryRunPlan(runID string, createdAt string, agent AgentDescriptor, promptRepoPath string) (DryRunPlan, error) {
	if agent.Input != InputPromptFile {
		return DryRunPlan{}, fmt.Errorf("unsupported agent input mode: %s", agent.Input)
	}

	agentArgs := append([]string{}, agent.Args...)
	wouldRunArgs := append([]string{}, agent.Args...)

	return DryRunPlan{
		Schema:    DryRunSchema,
		RunID:     runID,
		CreatedAt: createdAt,
		Agent: DryRunAgent{
			Name:    agent.Name,
			Command: agent.Command,
			Args:    agentArgs,
			Input:   agent.Input,
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
			Command: agent.Command,
			Args:    wouldRunArgs,
			Stdin:   promptRepoPath,
		},
	}, nil
}
