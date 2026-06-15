package agents

import (
	"strings"
	"testing"
)

func TestApplyModelSpecCodexIsNoOp(t *testing.T) {
	// Codex model/effort go via -c overrides in acpAdapterCommand; ApplyModelSpec
	// must not append CLI flags to the descriptor.
	codexExec := AgentDescriptor{Name: BuiltinCodex, Input: InputPromptFile}
	for _, spec := range []ModelSpec{
		{Model: "gpt-5", Effort: "high"},
		{Model: "gpt-5"},
		{Effort: "high"},
		{},
	} {
		got, err := ApplyModelSpec(codexExec, spec)
		if err != nil {
			t.Fatalf("ApplyModelSpec(codex, %+v) returned error: %v", spec, err)
		}
		if len(got.Args) != 0 {
			t.Fatalf("ApplyModelSpec(codex, %+v) must not add CLI args, got %v", spec, got.Args)
		}
	}
}

func TestApplyModelSpecClaudeIsNoOp(t *testing.T) {
	// Claude model/effort go via env vars in acpAdapterCommand; ApplyModelSpec
	// must not append --model/--effort CLI flags to the descriptor.
	claudeExec := AgentDescriptor{Name: BuiltinClaude, Input: InputPromptFile}
	for _, spec := range []ModelSpec{
		{Model: "claude-sonnet-4", Effort: "high"},
		{Model: "claude-sonnet-4"},
		{Effort: "high"},
		{},
	} {
		got, err := ApplyModelSpec(claudeExec, spec)
		if err != nil {
			t.Fatalf("ApplyModelSpec(claude, %+v) returned error: %v", spec, err)
		}
		if len(got.Args) != 0 {
			t.Fatalf("ApplyModelSpec(claude, %+v) must not add CLI args, got %v", spec, got.Args)
		}
	}
}

func TestReviewerBuiltinsHaveNoWriteBypassArgs(t *testing.T) {
	// Both built-in reviewers run over ACP and carry no CLI args at all.
	// Write-stage bypass is never in scope for reviewers; read-only enforcement
	// is applied at the ACP layer via RunRequest.ReadOnly=true (codex: sandbox_mode
	// adapter flag; claude: ACP client write denial).
	for _, name := range []string{BuiltinCodex, BuiltinClaude} {
		reviewer, err := BuiltinRegistry{}.ResolveReviewer(name)
		if err != nil {
			t.Fatalf("ResolveReviewer(%q) error: %v", name, err)
		}
		if len(reviewer.Args) != 0 {
			t.Fatalf("reviewer %q must have no CLI args, got %v", name, reviewer.Args)
		}
	}
}

func TestClaudeDescriptorHasNoCLIArgs(t *testing.T) {
	// Claude runs over ACP; its built-in descriptors must carry no CLI command
	// or args. Write-stage capability is granted by RunRequest.ReadOnly=false on
	// the ACP client; read-only is enforced by RunRequest.ReadOnly=true.
	claudeExec, err := BuiltinRegistry{}.ResolveExecutor(BuiltinClaude)
	if err != nil {
		t.Fatalf("ResolveExecutor(claude) error: %v", err)
	}
	if claudeExec.Command != "" {
		t.Fatalf("claude executor must have no CLI command, got %q", claudeExec.Command)
	}
	if len(claudeExec.Args) != 0 {
		t.Fatalf("claude executor must have no CLI args, got %v", claudeExec.Args)
	}

	claudeReviewer, err := BuiltinRegistry{}.ResolveReviewer(BuiltinClaude)
	if err != nil {
		t.Fatalf("ResolveReviewer(claude) error: %v", err)
	}
	if claudeReviewer.Command != "" {
		t.Fatalf("claude reviewer must have no CLI command, got %q", claudeReviewer.Command)
	}
	if len(claudeReviewer.Args) != 0 {
		t.Fatalf("claude reviewer must have no CLI args, got %v", claudeReviewer.Args)
	}
}

func assertArgsDoNotContain(t *testing.T, args []string, forbidden ...string) {
	t.Helper()
	joined := strings.Join(args, " ")
	for _, value := range forbidden {
		if strings.Contains(joined, value) {
			t.Fatalf("args should not contain %q: %#v", value, args)
		}
	}
}

func sameStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
