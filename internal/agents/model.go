package agents

import (
	"fmt"
	"strings"
)

type ModelSpec struct {
	Model  string
	Effort string
}

func ParseModelSpec(raw string) (ModelSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ModelSpec{}, nil
	}

	parts := strings.Split(raw, ":")
	if len(parts) > 2 {
		return ModelSpec{}, fmt.Errorf("invalid model spec %q: expected model[:effort]", raw)
	}

	spec := ModelSpec{Model: strings.TrimSpace(parts[0])}
	if len(parts) == 2 {
		spec.Effort = strings.TrimSpace(parts[1])
	}
	return spec, nil
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
