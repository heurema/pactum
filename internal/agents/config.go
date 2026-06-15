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
	builtins := []AgentDescriptor{
		{
			Name:    BuiltinCodex,
			Command: "codex",
			Args:    []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox"},
			Input:   InputPromptFile,
		},
		{
			// Claude runs exclusively over ACP; no CLI command or args.
			// Write capability is granted via RunRequest.ReadOnly=false on the ACP client.
			Name:  BuiltinClaude,
			Input: InputPromptFile,
		},
	}
	for i := range builtins {
		builtins[i].Args = append([]string{}, builtins[i].Args...)
	}
	return builtins
}

// reviewerBuiltins returns read-only descriptors for the reviewer role. A reviewer
// only reads the diff and emits findings, so it must NOT carry the executor's
// write/edit bypass. Codex pins its adapter to a read-only sandbox; claude's
// read-only enforcement is applied by the ACP client when RunRequest.ReadOnly is
// true — no adapter flag is needed, so the descriptor carries no CLI args.
func reviewerBuiltins() []AgentDescriptor {
	builtins := []AgentDescriptor{
		{
			Name:    BuiltinCodex,
			Command: "codex",
			Args:    []string{"exec", "--json", "--sandbox", "read-only"},
			Input:   InputPromptFile,
		},
		{
			// Claude reviewer read-only is enforced at the ACP client layer.
			Name:  BuiltinClaude,
			Input: InputPromptFile,
		},
	}
	for i := range builtins {
		builtins[i].Args = append([]string{}, builtins[i].Args...)
	}
	return builtins
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
