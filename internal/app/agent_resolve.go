package app

import (
	"fmt"
	"strings"

	"github.com/heurema/pactum/internal/agents"
)

// agentRole selects which built-in descriptor variant a registry entry
// resolves to: the write-enabled executor or the read-only reviewer.
type agentRole int

const (
	agentRoleExecutor agentRole = iota
	agentRoleReviewer
)

// resolvedAgent is a registry entry resolved for a role: the registry name,
// the role descriptor for the underlying built-in with the entry's pins
// applied, and the pin spec itself (shown in the Resolved block and recorded
// in the usage ledger).
type resolvedAgent struct {
	Name      string
	Agent     agents.AgentDescriptor
	ModelSpec agents.ModelSpec
}

// findRegistryEntry resolves a registry name. References resolve only against
// the registry: a bare built-in name that is not registered is an error.
func findRegistryEntry(config configFile, name string) (agentRegistryEntry, error) {
	name = strings.TrimSpace(name)
	for _, entry := range config.Agents {
		if entry.Name == name {
			return entry, nil
		}
	}
	return agentRegistryEntry{}, fmt.Errorf("unknown agent %q: not registered in config agents", name)
}

// resolveExecutorEntry picks the registry entry for an executor-stage
// invocation (execute, review fix): an explicit name resolves against the
// registry; a non-empty stageBy[0] is the configured stage default; otherwise
// the first registry entry.
func resolveExecutorEntry(config configFile, stageBy stageBy, name string) (agentRegistryEntry, error) {
	if strings.TrimSpace(name) != "" {
		return findRegistryEntry(config, name)
	}
	if len(stageBy) > 0 {
		return findRegistryEntry(config, stageBy[0])
	}
	return config.Agents[0], nil
}

// resolveReviewerEntry picks the registry entry for a reviewer-role
// invocation (review, clarifier round, contract draft): an explicit name
// resolves against the registry; a non-empty stageBy[0] is the configured
// stage default; otherwise cross-model semantics apply against inferred
// engines — the first entry whose engine differs from the run executor's,
// falling back to the first entry.
func resolveReviewerEntry(config configFile, stageBy stageBy, context reviewContext, name string) (agentRegistryEntry, error) {
	if strings.TrimSpace(name) != "" {
		return findRegistryEntry(config, name)
	}
	if len(stageBy) > 0 {
		return findRegistryEntry(config, stageBy[0])
	}
	executorEngine, ok := latestExecutionExecutorName(context)
	if !ok {
		// No execution attempt yet (clarifier rounds and contract draft run
		// before execution): the executor that would run is the default — the
		// first registry entry.
		engine, err := registryEntryEngine(config.Agents[0])
		if err != nil {
			return agentRegistryEntry{}, err
		}
		executorEngine = engine
	}
	for _, entry := range config.Agents {
		engine, err := registryEntryEngine(entry)
		if err != nil {
			return agentRegistryEntry{}, err
		}
		if engine != executorEngine {
			return entry, nil
		}
	}
	return config.Agents[0], nil
}

// registryEntryEngine infers the built-in engine a registry entry runs on from
// its model. readConfig validates inference for every entry, so a failure here
// means the entry bypassed config validation.
func registryEntryEngine(entry agentRegistryEntry) (string, error) {
	engine, ok := agents.InferAgentFromModel(entry.Model)
	if !ok {
		return "", fmt.Errorf("agent %q: cannot infer the engine from model %q", entry.Name, entry.Model)
	}
	return engine, nil
}

// resolveAgentForRole turns a registry entry into the runnable descriptor for
// a role, with the entry's model/effort pins applied.
func (a App) resolveAgentForRole(entry agentRegistryEntry, role agentRole) (resolvedAgent, error) {
	// Inference never yields an empty engine, and an unrecognized model errors
	// out here. That matters because the agents-package resolvers fall back to
	// their own hardcoded default on an empty name — the exact behavior the
	// registry replaces, which must not re-enter through this seam.
	engine, err := registryEntryEngine(entry)
	if err != nil {
		return resolvedAgent{}, err
	}
	var descriptor agents.AgentDescriptor
	switch role {
	case agentRoleReviewer:
		descriptor, err = a.agentRegistry().ResolveReviewer(engine)
	default:
		descriptor, err = a.agentRegistry().ResolveExecutor(engine)
	}
	if err != nil {
		return resolvedAgent{}, err
	}
	spec := entry.modelSpec()
	descriptor, err = agents.ApplyModelSpec(descriptor, spec)
	if err != nil {
		return resolvedAgent{}, err
	}
	if descriptor.Input != agents.InputPromptFile {
		return resolvedAgent{}, fmt.Errorf("unsupported agent input mode: %s", descriptor.Input)
	}
	return resolvedAgent{Name: entry.Name, Agent: descriptor, ModelSpec: spec}, nil
}
