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
	case BuiltinCodex:
		if spec.Model != "" {
			// codex parses `-c key=value` as TOML, so the model string must be
			// quoted (matches the documented `-c model="o3"` form); a bare value
			// like gpt-5.5 would otherwise fail to parse.
			agent.Args = append(agent.Args, "-c", fmt.Sprintf("model=%q", spec.Model))
		}
		if spec.Effort != "" {
			agent.Args = append(agent.Args, "-c", "model_reasoning_effort="+spec.Effort)
		}
	case BuiltinClaude:
		if spec.Model != "" {
			agent.Args = append(agent.Args, "--model", spec.Model)
		}
		if spec.Effort != "" {
			agent.Args = append(agent.Args, "--effort", spec.Effort)
		}
	default:
		return AgentDescriptor{}, fmt.Errorf("model override is unsupported for agent: %s", agent.Name)
	}
	return agent, nil
}
