package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

// idleKilledAgentTransport fakes an idle-killed attempt: it writes the attempt
// logs like the real transports, then returns a timed-out RunResult alongside
// the kill error. With completed=true it reports the completed-with-warning
// finalize the transports produce when the captured output carries the agent's
// successful terminal marker.
type idleKilledAgentTransport struct {
	stdout    string
	completed bool
}

func (tr *idleKilledAgentTransport) Run(request agents.RunRequest) (agents.RunResult, error) {
	artifactDir := request.ArtifactDir
	if artifactDir == "" {
		artifactDir = "execute/attempts"
	}
	attemptDir := filepath.Join(request.RepoRoot, artifacts.WorkspaceRel, "runs", request.RunID, filepath.FromSlash(artifactDir), request.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return agents.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stdout.log"), []byte(tr.stdout), 0o644); err != nil {
		return agents.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stderr.log"), nil, 0o644); err != nil {
		return agents.RunResult{}, err
	}
	result := agents.RunResult{
		ExitCode:   -1,
		StartedAt:  "2026-06-10T16:30:51Z",
		FinishedAt: "2026-06-10T16:30:52Z",
		TimedOut:   true,
		StdoutPath: artifactDir + "/" + request.AttemptID + "/stdout.log",
		StderrPath: artifactDir + "/" + request.AttemptID + "/stderr.log",
	}
	if tr.completed {
		result.ExitCode = 0
		result.CompletedDespiteTimeout = true
	}
	return result, errors.New("agent process killed after idle timeout")
}

func TestExecuteRunCompletedDespiteTimeoutTakesSuccessPath(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	app.AgentTransport = &idleKilledAgentTransport{completed: true}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("completed-despite-timeout execute run should succeed, exited %d, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); strings.Contains(got, "produced no output") {
		t.Fatalf("success path must not report the timeout error:\n%s", got)
	}
	out := stdout.String()
	if !strings.Contains(out, "Execution attempt finished") || !strings.Contains(out, "timed out: true") {
		t.Fatalf("human output mismatch:\n%s", out)
	}
	if !strings.Contains(out, "warning: idle timeout fired after the agent completed") {
		t.Fatalf("human output should carry the completed-with-warning notice:\n%s", out)
	}

	// The attempt result document keeps the honest record: the watchdog fired,
	// and the attempt completed anyway.
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	var result executionResultDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, executionAttemptPaths(runPaths, "attempt_001").ResultJSON)), &result))
	if result.ExitCode != 0 || !result.TimedOut || !result.CompletedDespiteTimeout {
		t.Fatalf("unexpected result document: %#v", result)
	}
}

func TestExecuteRunPlainTimeoutStillFailsWithTimeoutMessage(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	app.AgentTransport = &idleKilledAgentTransport{}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("plain timed-out execute run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "agent process produced no output for") {
		t.Fatalf("timeout message missing:\n%s", got)
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	var result executionResultDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, executionAttemptPaths(runPaths, "attempt_001").ResultJSON)), &result))
	if result.ExitCode != -1 || !result.TimedOut || result.CompletedDespiteTimeout {
		t.Fatalf("unexpected plain timeout result document: %#v", result)
	}
}

func TestClarifierRoundCompletedDespiteTimeoutRunsAfterSuccess(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})
	app.AgentTransport = &idleKilledAgentTransport{
		completed: true,
		stdout: clarifierStructuredOutput([]map[string]any{
			{
				"text":               "Does the timeout finalize keep the record honest?",
				"blocking":           false,
				"kind":               "scope",
				"rationale":          "The run record must distinguish a finished agent from a hung one.",
				"recommended_answer": "Yes.",
				"confidence":         "high",
			},
		}),
	}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "run", runID, "--no-auto", "--max-rounds", "1", "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("completed-despite-timeout clarifier round should succeed, exited %d, stderr: %s", code, stderr.String())
	}
	// AfterSuccess ran: the clarifier stdout was parsed and the question recorded.
	out := stdout.String()
	if !strings.Contains(out, "Clarify loop finished") || !strings.Contains(out, "questions created 1") {
		t.Fatalf("AfterSuccess should record the question:\n%s", out)
	}
}
