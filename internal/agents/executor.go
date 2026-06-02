package agents

import (
	"fmt"
	"strings"
)

type promptExecutor interface {
	command(stdin string) DryRunCommand
	env(environ []string) []string
}

type codexExecutor struct {
	Command string
	Args    []string
}

type claudeExecutor struct {
	Command string
	Args    []string
}

type descriptorExecutor struct {
	Command string
	Args    []string
}

func BuildCommand(agent AgentDescriptor, stdin string) (DryRunCommand, error) {
	executor, err := newPromptExecutor(agent)
	if err != nil {
		return DryRunCommand{}, err
	}
	return executor.command(stdin), nil
}

func newPromptExecutor(agent AgentDescriptor) (promptExecutor, error) {
	if strings.TrimSpace(agent.Command) == "" {
		return nil, fmt.Errorf("agent command is required")
	}
	if agent.Input != InputPromptFile {
		return nil, fmt.Errorf("unsupported agent input mode: %s", agent.Input)
	}

	switch agent.Name {
	case BuiltinCodex:
		return codexExecutor{Command: agent.Command, Args: cloneArgs(agent.Args)}, nil
	case BuiltinClaude:
		return claudeExecutor{Command: agent.Command, Args: cloneArgs(agent.Args)}, nil
	default:
		return descriptorExecutor{Command: agent.Command, Args: cloneArgs(agent.Args)}, nil
	}
}

func (e codexExecutor) command(stdin string) DryRunCommand {
	return DryRunCommand{
		Command: e.Command,
		Args:    cloneArgs(e.Args),
		Stdin:   stdin,
	}
}

func (e codexExecutor) env(environ []string) []string {
	return filteredEnv(environ, map[string]bool{
		"CLAUDECODE": true,
	})
}

func (e claudeExecutor) command(stdin string) DryRunCommand {
	return DryRunCommand{
		Command: e.Command,
		Args:    cloneArgs(e.Args),
		Stdin:   stdin,
	}
}

func (e claudeExecutor) env(environ []string) []string {
	return filteredEnv(environ, map[string]bool{
		"CLAUDECODE": true,
	})
}

func (e descriptorExecutor) command(stdin string) DryRunCommand {
	return DryRunCommand{
		Command: e.Command,
		Args:    cloneArgs(e.Args),
		Stdin:   stdin,
	}
}

func (e descriptorExecutor) env(environ []string) []string {
	return append([]string{}, environ...)
}

func filteredEnv(environ []string, strip map[string]bool) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		name := entry
		if i := strings.IndexByte(entry, '='); i >= 0 {
			name = entry[:i]
		}
		if strip[name] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func cloneArgs(args []string) []string {
	return append([]string{}, args...)
}
