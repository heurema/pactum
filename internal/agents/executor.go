package agents

import (
	"fmt"
	"strings"
)

type promptExecutor interface {
	command(stdin string) DryRunCommand
	env(environ []string) []string
}

// promptFileExecutor runs an agent that reads its prompt from stdin. The only
// behavioral difference between the built-in agents and an arbitrary descriptor
// is which environment variables get stripped, captured by stripEnv.
type promptFileExecutor struct {
	Command  string
	Args     []string
	StripEnv map[string]bool
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

	executor := promptFileExecutor{Command: agent.Command, Args: cloneArgs(agent.Args)}
	switch agent.Name {
	case BuiltinCodex, BuiltinClaude:
		executor.StripEnv = map[string]bool{"CLAUDECODE": true}
	}
	return executor, nil
}

func (e promptFileExecutor) command(stdin string) DryRunCommand {
	return DryRunCommand{
		Command: e.Command,
		Args:    cloneArgs(e.Args),
		Stdin:   stdin,
	}
}

func (e promptFileExecutor) env(environ []string) []string {
	return filteredEnv(environ, e.StripEnv)
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
