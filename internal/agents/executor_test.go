package agents

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCommandUsesStdinForBuiltInAgents(t *testing.T) {
	promptPath := ".heurema/pactum/runs/run_123/contract/prompt.md"

	tests := []struct {
		name    string
		agent   AgentDescriptor
		command string
		args    []string
	}{
		{
			name: "codex",
			agent: AgentDescriptor{
				Name:    BuiltinCodex,
				Command: "codex",
				Args:    []string{"exec"},
				Input:   InputPromptFile,
			},
			command: "codex",
			args:    []string{"exec"},
		},
		{
			name: "claude",
			agent: AgentDescriptor{
				Name:    BuiltinClaude,
				Command: "claude",
				Args:    []string{"-p"},
				Input:   InputPromptFile,
			},
			command: "claude",
			args:    []string{"-p"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := BuildCommand(tt.agent, promptPath)
			if err != nil {
				t.Fatalf("BuildCommand returned error: %v", err)
			}
			if command.Command != tt.command || !sameStringSlice(command.Args, tt.args) || command.Stdin != promptPath {
				t.Fatalf("unexpected command: %#v", command)
			}
			assertArgsDoNotContain(t, command.Args, "contract/prompt.md", promptPath)
		})
	}
}

func TestRunSubprocessCodexUsesTypedRunnerStdinAndEnv(t *testing.T) {
	root := t.TempDir()
	promptRepoPath := ".heurema/pactum/runs/run_123/contract/prompt.md"
	writePrompt(t, root, promptRepoPath, "executor prompt body")
	t.Setenv("CLAUDECODE", "nested")
	t.Setenv("ANTHROPIC_API_KEY", "kept-for-codex")

	runner := &recordingRunner{}
	result, err := runSubprocessWithRunner(RunRequest{
		RepoRoot:       root,
		RunID:          "run_123",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec"}, Input: InputPromptFile},
		PromptRepoPath: promptRepoPath,
	}, runner)
	if err != nil {
		t.Fatalf("RunSubprocess returned error: %v", err)
	}
	if result.Command != "codex" || !sameStringSlice(result.Args, []string{"exec"}) || result.ExitCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if runner.spec.Command != "codex" || !sameStringSlice(runner.spec.Args, []string{"exec"}) {
		t.Fatalf("unexpected process spec: %#v", runner.spec)
	}
	if runner.spec.Dir != root {
		t.Fatalf("process dir = %q, want %q", runner.spec.Dir, root)
	}
	if runner.stdin != "executor prompt body" {
		t.Fatalf("stdin mismatch: %q", runner.stdin)
	}
	assertArgsDoNotContain(t, runner.spec.Args, "contract/prompt.md", promptRepoPath)
	if envContainsName(runner.spec.Env, "CLAUDECODE") {
		t.Fatalf("codex env should strip CLAUDECODE: %#v", runner.spec.Env)
	}
	if !envContainsName(runner.spec.Env, "ANTHROPIC_API_KEY") {
		t.Fatalf("codex env should preserve ANTHROPIC_API_KEY")
	}

	stdout := filepath.Join(root, ".heurema", "pactum", "runs", "run_123", "execute", "attempts", "attempt_001", "stdout.log")
	stderr := filepath.Join(root, ".heurema", "pactum", "runs", "run_123", "execute", "attempts", "attempt_001", "stderr.log")
	if got := readFile(t, stdout); !strings.Contains(got, "stdout-line") {
		t.Fatalf("stdout log mismatch: %q", got)
	}
	if got := readFile(t, stderr); !strings.Contains(got, "stderr-line") {
		t.Fatalf("stderr log mismatch: %q", got)
	}
}

func TestRunSubprocessClaudeFiltersNestedAgentMarker(t *testing.T) {
	root := t.TempDir()
	promptRepoPath := ".heurema/pactum/runs/run_123/contract/prompt.md"
	writePrompt(t, root, promptRepoPath, "claude prompt")
	t.Setenv("CLAUDECODE", "nested")
	t.Setenv("ANTHROPIC_API_KEY", "kept-for-claude")

	runner := &recordingRunner{}
	_, err := runSubprocessWithRunner(RunRequest{
		RepoRoot:       root,
		RunID:          "run_123",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinClaude, Command: "claude", Args: []string{"-p"}, Input: InputPromptFile},
		PromptRepoPath: promptRepoPath,
	}, runner)
	if err != nil {
		t.Fatalf("RunSubprocess returned error: %v", err)
	}
	if runner.spec.Command != "claude" || !sameStringSlice(runner.spec.Args, []string{"-p"}) {
		t.Fatalf("unexpected process spec: %#v", runner.spec)
	}
	if runner.stdin != "claude prompt" {
		t.Fatalf("stdin mismatch: %q", runner.stdin)
	}
	if envContainsName(runner.spec.Env, "CLAUDECODE") {
		t.Fatalf("claude env should strip nested agent marker: %#v", runner.spec.Env)
	}
	if !envContainsName(runner.spec.Env, "ANTHROPIC_API_KEY") {
		t.Fatalf("claude env should preserve ANTHROPIC_API_KEY")
	}
}

type recordingRunner struct {
	spec  processSpec
	stdin string
	err   error
}

func (r *recordingRunner) Run(_ context.Context, spec processSpec) error {
	r.spec = processSpec{
		Command: spec.Command,
		Args:    append([]string{}, spec.Args...),
		Dir:     spec.Dir,
		Env:     append([]string{}, spec.Env...),
		Stdout:  spec.Stdout,
		Stderr:  spec.Stderr,
	}
	stdin, err := io.ReadAll(spec.Stdin)
	if err != nil {
		return err
	}
	r.stdin = string(stdin)
	fmt.Fprintln(spec.Stdout, "stdout-line")
	fmt.Fprintln(spec.Stderr, "stderr-line")
	if r.err != nil {
		return r.err
	}
	return nil
}

func writePrompt(t *testing.T, root string, promptRepoPath string, prompt string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(promptRepoPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir prompt dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(prompt), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
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

func envContainsName(environ []string, name string) bool {
	prefix := name + "="
	for _, entry := range environ {
		if entry == name || strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
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
