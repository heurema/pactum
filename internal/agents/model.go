package agents

import (
	"fmt"
)

type ModelSpec struct {
	Model  string
	Effort string
}

func ApplyModelSpec(agent AgentDescriptor, spec ModelSpec) (AgentDescriptor, error) {
	// Clone so appending override flags never mutates the caller's descriptor.
	agent = cloneDescriptor(agent)
	if spec.Model == "" && spec.Effort == "" {
		return agent, nil
	}

	switch agent.Name {
	case BuiltinCodex, BuiltinClaude:
		// Both agents run over ACP; model/effort reach the adapter via RunRequest.Model
		// in acpAdapterCommand (codex: -c overrides, claude: ANTHROPIC_MODEL env vars).
		// No CLI args to append to the descriptor.
	default:
		return AgentDescriptor{}, fmt.Errorf("model override is unsupported for agent: %s", agent.Name)
	}
	return agent, nil
}
