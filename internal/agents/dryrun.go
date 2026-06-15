package agents

const (
	DryRunSchema                 = "pactum.execute_dry_run.v1"
	DryRunArtifactPrompt         = "contract/prompt.md"
	DryRunArtifactContext        = "context/executor-context.md"
	DryRunArtifactPromptManifest = "contract/prompt-manifest.json"
	DryRunArtifactDryRun         = "execute/dry-run.json"
)

// BuildACPWouldRun returns the DryRunCommand describing the ACP adapter that
// would be launched for the given agent, model spec, and read-only flag. Callers
// that build partial dry-run documents (reviewer lenses, review-fix plans) use
// this to populate their WouldRun field without duplicating adapter resolution.
func BuildACPWouldRun(agentName string, spec ModelSpec, readOnly bool) (DryRunCommand, error) {
	cmd, args, env, err := acpAdapterCommand(agentName, spec, readOnly)
	if err != nil {
		return DryRunCommand{}, err
	}
	return DryRunCommand{
		Command: cmd,
		Args:    append([]string{}, args...),
		Env:     append([]string{}, env...),
	}, nil
}

func BuildDryRunPlan(runID string, createdAt string, agent AgentDescriptor, spec ModelSpec, readOnly bool, promptRepoPath string) (DryRunPlan, error) {
	adapterCmd, adapterArgs, adapterEnv, err := acpAdapterCommand(agent.Name, spec, readOnly)
	if err != nil {
		return DryRunPlan{}, err
	}

	return DryRunPlan{
		Schema:    DryRunSchema,
		RunID:     runID,
		CreatedAt: createdAt,
		Agent: DryRunAgent{
			Name:  agent.Name,
			Input: agent.Input,
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
			Command: adapterCmd,
			Args:    append([]string{}, adapterArgs...),
			Env:     append([]string{}, adapterEnv...),
		},
	}, nil
}
