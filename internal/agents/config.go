package agents

import (
	"fmt"
	"strings"
)

const (
	BuiltinCodex    = "codex"
	BuiltinClaude   = "claude"
	InputPromptFile = "prompt_file"
)

type BuiltinRegistry struct{}

func DefaultExecutor() string {
	return BuiltinCodex
}

func DefaultReviewer() string {
	return BuiltinCodex
}

func ResolveExecutor(name string) (AgentDescriptor, error) {
	return BuiltinRegistry{}.ResolveExecutor(name)
}

func ResolveReviewer(name string) (AgentDescriptor, error) {
	return BuiltinRegistry{}.ResolveReviewer(name)
}

func ListBuiltins() []AgentDescriptor {
	return BuiltinRegistry{}.ListBuiltins()
}

func (BuiltinRegistry) DefaultExecutor() string {
	return DefaultExecutor()
}

func (BuiltinRegistry) DefaultReviewer() string {
	return DefaultReviewer()
}

func (BuiltinRegistry) ResolveExecutor(name string) (AgentDescriptor, error) {
	return resolveBuiltin(name, DefaultExecutor())
}

func (BuiltinRegistry) ResolveReviewer(name string) (AgentDescriptor, error) {
	return resolveBuiltin(name, DefaultReviewer())
}

func (BuiltinRegistry) ListBuiltins() []AgentDescriptor {
	builtins := []AgentDescriptor{
		{
			Name:    BuiltinCodex,
			Command: "codex",
			Args:    []string{"exec", "--dangerously-bypass-approvals-and-sandbox"},
			Input:   InputPromptFile,
		},
		{
			Name:    BuiltinClaude,
			Command: "claude",
			Args:    []string{"-p"},
			Input:   InputPromptFile,
		},
	}
	for i := range builtins {
		builtins[i].Args = append([]string{}, builtins[i].Args...)
	}
	return builtins
}

func resolveBuiltin(name string, defaultName string) (AgentDescriptor, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultName
	}
	for _, descriptor := range ListBuiltins() {
		if descriptor.Name == name {
			return cloneDescriptor(descriptor), nil
		}
	}
	return AgentDescriptor{}, fmt.Errorf("unsupported agent: %s", name)
}

func cloneDescriptor(descriptor AgentDescriptor) AgentDescriptor {
	descriptor.Args = append([]string{}, descriptor.Args...)
	return descriptor
}
