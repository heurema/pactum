package agents

import (
	"os"
	"strings"
)

const (
	DryRunSchema                 = "pactum.execute_dry_run.v1alpha1"
	DryRunArtifactPrompt         = "contract/prompt.md"
	DryRunArtifactContext        = "context/executor-context.md"
	DryRunArtifactPromptManifest = "contract/prompt-manifest.json"
	DryRunArtifactDryRun         = "execute/dry-run.json"
)

// sanitizeHomePath replaces an os.UserHomeDir() prefix in s with '~'. If the
// home directory cannot be determined or s does not start with it, s is
// returned unchanged. Only the recorded representation is sanitized; the actual
// command launched by ACPTransport.Run uses the original value.
//
// Also handles KEY=VALUE and KEY="VALUE" composite entries (env vars, flag
// assignments) where VALUE is a home-directory path.
func sanitizeHomePath(s string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return s
	}
	if s == home {
		return "~"
	}
	if strings.HasPrefix(s, home+"/") {
		return "~" + s[len(home):]
	}
	// KEY=VALUE entries (env vars, flag assignments) where VALUE is a home path.
	if idx := strings.Index(s, "="); idx >= 0 {
		val := s[idx+1:]
		if val == home {
			return s[:idx+1] + "~"
		}
		if strings.HasPrefix(val, home+"/") {
			return s[:idx+1] + "~" + val[len(home):]
		}
		// KEY="VALUE" — quoted form produced by fmt.Sprintf("key=%q", path).
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			inner := val[1 : len(val)-1]
			if inner == home {
				return s[:idx+1] + `"~"`
			}
			if strings.HasPrefix(inner, home+"/") {
				return s[:idx+1] + `"~` + inner[len(home):] + `"`
			}
		}
	}
	return s
}

func sanitizeHomePaths(ss []string) []string {
	if len(ss) == 0 {
		return append([]string{}, ss...)
	}
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = sanitizeHomePath(s)
	}
	return out
}

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
		Command: sanitizeHomePath(cmd),
		Args:    sanitizeHomePaths(args),
		Env:     sanitizeHomePaths(env),
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
		},
		Artifacts: DryRunArtifacts{
			Prompt:          DryRunArtifactPrompt,
			ExecutorContext: DryRunArtifactContext,
			PromptManifest:  DryRunArtifactPromptManifest,
		},
		WouldRun: DryRunCommand{
			Command: sanitizeHomePath(adapterCmd),
			Args:    sanitizeHomePaths(adapterArgs),
			Env:     sanitizeHomePaths(adapterEnv),
		},
	}, nil
}
