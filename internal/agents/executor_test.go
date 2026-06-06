package agents

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseModelSpec(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		model  string
		effort string
	}{
		{name: "empty", raw: ""},
		{name: "model only", raw: "gpt-5", model: "gpt-5"},
		{name: "effort only", raw: ":high", effort: "high"},
		{name: "model and effort", raw: "gpt-5:high", model: "gpt-5", effort: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseModelSpec(tt.raw)
			if err != nil {
				t.Fatalf("ParseModelSpec returned error: %v", err)
			}
			if spec.Model != tt.model || spec.Effort != tt.effort {
				t.Fatalf("ParseModelSpec(%q) = %#v, want model %q effort %q", tt.raw, spec, tt.model, tt.effort)
			}
		})
	}
}

func TestParseModelSpecRejectsMultipleColons(t *testing.T) {
	if _, err := ParseModelSpec("gpt-5:high:extra"); err == nil {
		t.Fatalf("ParseModelSpec should reject multiple colons")
	}
}

func TestApplyModelSpecEmitsBuiltInAgentArgs(t *testing.T) {
	tests := []struct {
		name  string
		agent AgentDescriptor
		spec  ModelSpec
		args  []string
	}{
		{
			name:  "codex empty",
			agent: AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}, Input: InputPromptFile},
			args:  []string{"exec", "--dangerously-bypass-approvals-and-sandbox"},
		},
		{
			name:  "codex model only",
			agent: AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}, Input: InputPromptFile},
			spec:  ModelSpec{Model: "gpt-5"},
			args:  []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "-c", "model=\"gpt-5\""},
		},
		{
			name:  "codex effort only",
			agent: AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}, Input: InputPromptFile},
			spec:  ModelSpec{Effort: "high"},
			args:  []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "-c", "model_reasoning_effort=high"},
		},
		{
			name:  "codex model and effort",
			agent: AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}, Input: InputPromptFile},
			spec:  ModelSpec{Model: "gpt-5", Effort: "high"},
			args:  []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "-c", "model=\"gpt-5\"", "-c", "model_reasoning_effort=high"},
		},
		{
			name:  "claude model and effort",
			agent: AgentDescriptor{Name: BuiltinClaude, Command: "claude", Args: []string{"-p", "--dangerously-skip-permissions"}, Input: InputPromptFile},
			spec:  ModelSpec{Model: "claude-sonnet-4", Effort: "high"},
			args:  []string{"-p", "--dangerously-skip-permissions", "--model", "claude-sonnet-4", "--effort", "high"},
		},
		{
			name:  "codex reviewer keeps read-only sandbox",
			agent: AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec", "--sandbox", "read-only"}, Input: InputPromptFile},
			spec:  ModelSpec{Model: "gpt-5", Effort: "high"},
			args:  []string{"exec", "--sandbox", "read-only", "-c", "model=\"gpt-5\"", "-c", "model_reasoning_effort=high"},
		},
		{
			name:  "claude reviewer keeps reviewer mode",
			agent: AgentDescriptor{Name: BuiltinClaude, Command: "claude", Args: []string{"-p"}, Input: InputPromptFile},
			spec:  ModelSpec{Model: "claude-sonnet-4", Effort: "high"},
			args:  []string{"-p", "--model", "claude-sonnet-4", "--effort", "high"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := ApplyModelSpec(tt.agent, tt.spec)
			if err != nil {
				t.Fatalf("ApplyModelSpec returned error: %v", err)
			}
			if !sameStringSlice(agent.Args, tt.args) {
				t.Fatalf("args = %#v, want %#v", agent.Args, tt.args)
			}
		})
	}
}

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
				Args:    []string{"exec", "--dangerously-bypass-approvals-and-sandbox"},
				Input:   InputPromptFile,
			},
			command: "codex",
			args:    []string{"exec", "--dangerously-bypass-approvals-and-sandbox"},
		},
		{
			name: "claude",
			agent: AgentDescriptor{
				Name:    BuiltinClaude,
				Command: "claude",
				Args:    []string{"-p", "--dangerously-skip-permissions"},
				Input:   InputPromptFile,
			},
			command: "claude",
			args:    []string{"-p", "--dangerously-skip-permissions"},
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
		Agent:          AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}, Input: InputPromptFile},
		PromptRepoPath: promptRepoPath,
	}, runner)
	if err != nil {
		t.Fatalf("RunSubprocess returned error: %v", err)
	}
	if result.Command != "codex" || !sameStringSlice(result.Args, []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}) || result.ExitCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if runner.spec.Command != "codex" || !sameStringSlice(runner.spec.Args, []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}) {
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
		Agent:          AgentDescriptor{Name: BuiltinClaude, Command: "claude", Args: []string{"-p", "--dangerously-skip-permissions"}, Input: InputPromptFile},
		PromptRepoPath: promptRepoPath,
	}, runner)
	if err != nil {
		t.Fatalf("RunSubprocess returned error: %v", err)
	}
	if runner.spec.Command != "claude" || !sameStringSlice(runner.spec.Args, []string{"-p", "--dangerously-skip-permissions"}) {
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

func TestRunSubprocessTeesLiveOutput(t *testing.T) {
	root := t.TempDir()
	promptRepoPath := ".heurema/pactum/runs/run_123/contract/prompt.md"
	writePrompt(t, root, promptRepoPath, "executor prompt body")

	var live bytes.Buffer
	_, err := runSubprocessWithRunner(RunRequest{
		RepoRoot:       root,
		RunID:          "run_123",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec"}, Input: InputPromptFile},
		PromptRepoPath: promptRepoPath,
		LiveOutput:     &live,
	}, &recordingRunner{})
	if err != nil {
		t.Fatalf("RunSubprocess returned error: %v", err)
	}

	// The live writer receives a copy of both the agent's stdout and stderr.
	if got := live.String(); !strings.Contains(got, "stdout-line") || !strings.Contains(got, "stderr-line") {
		t.Fatalf("live output should tee both streams: %q", got)
	}

	// The per-attempt log files are still captured in full.
	attemptDir := filepath.Join(root, ".heurema", "pactum", "runs", "run_123", "execute", "attempts", "attempt_001")
	if got := readFile(t, filepath.Join(attemptDir, "stdout.log")); !strings.Contains(got, "stdout-line") {
		t.Fatalf("stdout log should still be written: %q", got)
	}
	if got := readFile(t, filepath.Join(attemptDir, "stderr.log")); !strings.Contains(got, "stderr-line") {
		t.Fatalf("stderr log should still be written: %q", got)
	}
}

func TestRunSubprocessWithoutLiveOutputIsCaptureOnly(t *testing.T) {
	root := t.TempDir()
	promptRepoPath := ".heurema/pactum/runs/run_123/contract/prompt.md"
	writePrompt(t, root, promptRepoPath, "executor prompt body")

	// No LiveOutput set: the run must still capture to the log files unchanged.
	_, err := runSubprocessWithRunner(RunRequest{
		RepoRoot:       root,
		RunID:          "run_123",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec"}, Input: InputPromptFile},
		PromptRepoPath: promptRepoPath,
	}, &recordingRunner{})
	if err != nil {
		t.Fatalf("RunSubprocess returned error: %v", err)
	}

	attemptDir := filepath.Join(root, ".heurema", "pactum", "runs", "run_123", "execute", "attempts", "attempt_001")
	if got := readFile(t, filepath.Join(attemptDir, "stdout.log")); !strings.Contains(got, "stdout-line") {
		t.Fatalf("stdout log should still be written: %q", got)
	}
	if got := readFile(t, filepath.Join(attemptDir, "stderr.log")); !strings.Contains(got, "stderr-line") {
		t.Fatalf("stderr log should still be written: %q", got)
	}
}

func TestRunSubprocessTimesOutAfterIdleOutputGap(t *testing.T) {
	root := t.TempDir()
	promptRepoPath := ".heurema/pactum/runs/run_123/contract/prompt.md"
	writePrompt(t, root, promptRepoPath, "executor prompt body")

	result, err := runSubprocessWithRunner(RunRequest{
		RepoRoot:       root,
		RunID:          "run_123",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec"}, Input: InputPromptFile},
		PromptRepoPath: promptRepoPath,
		Timeout:        30 * time.Millisecond,
	}, idleRunner{})
	if err == nil {
		t.Fatalf("RunSubprocess should return an error after idle timeout")
	}
	if !result.TimedOut || result.ExitCode != -1 {
		t.Fatalf("timeout result mismatch: %#v", result)
	}
}

func TestRunSubprocessIdleTimeoutResetsOnOutput(t *testing.T) {
	root := t.TempDir()
	promptRepoPath := ".heurema/pactum/runs/run_123/contract/prompt.md"
	writePrompt(t, root, promptRepoPath, "executor prompt body")

	timeout := 120 * time.Millisecond
	result, err := runSubprocessWithRunner(RunRequest{
		RepoRoot:       root,
		RunID:          "run_123",
		AttemptID:      "attempt_001",
		Agent:          AgentDescriptor{Name: BuiltinCodex, Command: "codex", Args: []string{"exec"}, Input: InputPromptFile},
		PromptRepoPath: promptRepoPath,
		Timeout:        timeout,
	}, periodicOutputRunner{Writes: 6, Interval: 30 * time.Millisecond})
	if err != nil {
		t.Fatalf("RunSubprocess returned error despite periodic output: %v", err)
	}
	if result.TimedOut || result.ExitCode != 0 {
		t.Fatalf("periodic output should prevent timeout: %#v", result)
	}
	if result.DurationMillis < timeout.Milliseconds() {
		t.Fatalf("run should exceed the old wall-clock timeout, got duration %dms", result.DurationMillis)
	}

	stdout := filepath.Join(root, ".heurema", "pactum", "runs", "run_123", "execute", "attempts", "attempt_001", "stdout.log")
	if got := readFile(t, stdout); !strings.Contains(got, "tick-6") {
		t.Fatalf("stdout log should capture periodic output: %q", got)
	}
}

func TestReviewerBuiltinsAreReadOnly(t *testing.T) {
	// Reviewers only read the diff and emit findings — they must never carry the
	// executor's write/edit bypass.
	for _, name := range []string{BuiltinCodex, BuiltinClaude} {
		reviewer, err := BuiltinRegistry{}.ResolveReviewer(name)
		if err != nil {
			t.Fatalf("ResolveReviewer(%q) error: %v", name, err)
		}
		joined := strings.Join(reviewer.Args, " ")
		for _, forbidden := range []string{"--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("reviewer %q must not carry write-bypass flag %q: %v", name, forbidden, reviewer.Args)
			}
		}
	}

	codexReviewer, _ := BuiltinRegistry{}.ResolveReviewer(BuiltinCodex)
	if got := strings.Join(codexReviewer.Args, " "); got != "exec --sandbox read-only" {
		t.Fatalf("codex reviewer should run a read-only sandbox, got %q", got)
	}
	claudeReviewer, _ := BuiltinRegistry{}.ResolveReviewer(BuiltinClaude)
	if got := strings.Join(claudeReviewer.Args, " "); got != "-p" {
		t.Fatalf("claude reviewer should drop write-bypass, got %q", got)
	}

	// Executors must still carry the write bypass: both agents must be able to mutate.
	codexExec, _ := BuiltinRegistry{}.ResolveExecutor(BuiltinCodex)
	if !strings.Contains(strings.Join(codexExec.Args, " "), "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("codex executor should keep full bypass: %v", codexExec.Args)
	}
	claudeExec, _ := BuiltinRegistry{}.ResolveExecutor(BuiltinClaude)
	if !strings.Contains(strings.Join(claudeExec.Args, " "), "--dangerously-skip-permissions") {
		t.Fatalf("claude executor should keep skip-permissions: %v", claudeExec.Args)
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

type idleRunner struct{}

func (idleRunner) Run(ctx context.Context, _ processSpec) error {
	<-ctx.Done()
	return ctx.Err()
}

type periodicOutputRunner struct {
	Writes   int
	Interval time.Duration
}

func (r periodicOutputRunner) Run(ctx context.Context, spec processSpec) error {
	for i := 0; i < r.Writes; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.Interval):
		}
		fmt.Fprintf(spec.Stdout, "tick-%d\n", i+1)
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
