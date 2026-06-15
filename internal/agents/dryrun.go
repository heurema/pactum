package agents

const (
	DryRunSchema                 = "pactum.execute_dry_run.v1"
	DryRunArtifactPrompt         = "contract/prompt.md"
	DryRunArtifactContext        = "context/executor-context.md"
	DryRunArtifactPromptManifest = "contract/prompt-manifest.json"
	DryRunArtifactDryRun         = "execute/dry-run.json"
)

func BuildDryRunPlan(runID string, createdAt string, agent AgentDescriptor, promptRepoPath string) (DryRunPlan, error) {
	// ACP agents (e.g. claude) carry no CLI command; skip BuildCommand for them
	// — the adapter is launched at runtime by the ACP transport.
	var wouldRun DryRunCommand
	if agent.Command != "" {
		var err error
		wouldRun, err = BuildCommand(agent, promptRepoPath)
		if err != nil {
			return DryRunPlan{}, err
		}
	}

	agentArgs := append([]string{}, agent.Args...)

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
			Command: wouldRun.Command,
			Args:    append([]string{}, wouldRun.Args...),
			Stdin:   wouldRun.Stdin,
		},
	}, nil
}
