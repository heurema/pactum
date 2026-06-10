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
// registry; an omitted name defaults to the first registry entry.
func resolveExecutorEntry(config configFile, name string) (agentRegistryEntry, error) {
	if strings.TrimSpace(name) == "" {
		return config.Agents[0], nil
	}
	return findRegistryEntry(config, name)
}

// resolveReviewerEntry picks the registry entry for a reviewer-role
// invocation (review, clarify suggest, contract draft): an explicit name
// resolves against the registry; an omitted name keeps cross-model semantics
// against underlying agents — the first entry whose underlying built-in
// differs from the run executor's, falling back to the first entry.
func resolveReviewerEntry(config configFile, context reviewContext, name string) (agentRegistryEntry, error) {
	if strings.TrimSpace(name) != "" {
		return findRegistryEntry(config, name)
	}
	executorAgent, ok := latestExecutionExecutorName(context)
	if !ok {
		// No execution attempt yet (clarify suggest and contract draft run
		// before execution): the executor that would run is the default — the
		// first registry entry.
		executorAgent = config.Agents[0].Agent
	}
	for _, entry := range config.Agents {
		if entry.Agent != executorAgent {
			return entry, nil
		}
	}
	return config.Agents[0], nil
}

// resolveAgentForRole turns a registry entry into the runnable descriptor for
// a role, with the entry's model/effort pins applied.
func (a App) resolveAgentForRole(entry agentRegistryEntry, role agentRole) (resolvedAgent, error) {
	var descriptor agents.AgentDescriptor
	var err error
	// entry.Agent is always non-empty here: readConfig normalizes it (defaulting
	// to the entry name) and validates it against the built-ins. That matters
	// because the agents-package resolvers fall back to their own hardcoded
	// default on an empty name — the exact behavior the registry replaces, which
	// must not re-enter through this seam.
	switch role {
	case agentRoleReviewer:
		descriptor, err = a.agentRegistry().ResolveReviewer(entry.Agent)
	default:
		descriptor, err = a.agentRegistry().ResolveExecutor(entry.Agent)
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
