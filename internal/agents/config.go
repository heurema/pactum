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
	return resolveFrom(ListBuiltins(), name, DefaultExecutor())
}

func (BuiltinRegistry) ResolveReviewer(name string) (AgentDescriptor, error) {
	return resolveFrom(reviewerBuiltins(), name, DefaultReviewer())
}

func (BuiltinRegistry) ListBuiltins() []AgentDescriptor {
	return []AgentDescriptor{
		{
			// Codex and Claude both run exclusively over ACP; no CLI command or args.
			// Write capability is granted via RunRequest.ReadOnly=false on the ACP client.
			Name:  BuiltinCodex,
			Input: InputPromptFile,
		},
		{
			Name:  BuiltinClaude,
			Input: InputPromptFile,
		},
	}
}

// reviewerBuiltins returns read-only descriptors for the reviewer role. A reviewer
// only reads the diff and emits findings — the read-only constraint is enforced
// per agent at the ACP layer: codex via a sandbox_mode="read-only" adapter flag,
// claude via the ACP client denying writes and refusing permission requests when
// RunRequest.ReadOnly is true. No CLI args are needed or present.
func reviewerBuiltins() []AgentDescriptor {
	return []AgentDescriptor{
		{Name: BuiltinCodex, Input: InputPromptFile},
		{Name: BuiltinClaude, Input: InputPromptFile},
	}
}

func resolveFrom(descriptors []AgentDescriptor, name string, defaultName string) (AgentDescriptor, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultName
	}
	for _, descriptor := range descriptors {
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
