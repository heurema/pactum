package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// wallClockKilledAgentTransport fakes a wall-clock-capped attempt: it writes
// the attempt logs like the real transports do, then returns WallClockTimeout=true
// alongside the kill error.
type wallClockKilledAgentTransport struct {
	stdout string
}

func (tr *wallClockKilledAgentTransport) Run(request agents.RunRequest) (agents.RunResult, error) {
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
	if err := os.WriteFile(filepath.Join(attemptDir, "stderr.log"), []byte("attempt killed: wall-clock cap exceeded\n"), 0o644); err != nil {
		return agents.RunResult{}, err
	}
	return agents.RunResult{
		ExitCode:         -1,
		StartedAt:        "2026-06-10T16:30:51Z",
		FinishedAt:       "2026-06-10T16:32:51Z",
		TimedOut:         false,
		WallClockTimeout: true,
		StdoutPath:       artifactDir + "/" + request.AttemptID + "/stdout.log",
		StderrPath:       artifactDir + "/" + request.AttemptID + "/stderr.log",
	}, errors.New("attempt killed: wall-clock cap exceeded")
}

func TestExecuteRunWallClockTimeoutFailsWithCapMessage(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	app.AgentTransport = &wallClockKilledAgentTransport{}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("wall-clock-killed execute run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "wall-clock cap") {
		t.Fatalf("wall-clock cap message missing in stderr:\n%s", got)
	}
}

// partialWallClockKilledTransport returns wall-clock-killed results for all but
// the first call. The first call succeeds with the provided stdout so that the
// fan-out has at least one successful lens (required for the second-pass loop
// to run and evaluate the ParseMiss / skipped_lenses logic for killed lenses).
type partialWallClockKilledTransport struct {
	mu     sync.Mutex
	count  int
	stdout string
}

func (tr *partialWallClockKilledTransport) Run(request agents.RunRequest) (agents.RunResult, error) {
	tr.mu.Lock()
	n := tr.count
	tr.count++
	tr.mu.Unlock()

	artifactDir := request.ArtifactDir
	if artifactDir == "" {
		artifactDir = "execute/attempts"
	}
	attemptDir := filepath.Join(request.RepoRoot, artifacts.WorkspaceRel, "runs", request.RunID, filepath.FromSlash(artifactDir), request.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return agents.RunResult{}, err
	}
	if n == 0 {
		if err := os.WriteFile(filepath.Join(attemptDir, "stdout.log"), []byte(tr.stdout), 0o644); err != nil {
			return agents.RunResult{}, err
		}
		if err := os.WriteFile(filepath.Join(attemptDir, "stderr.log"), nil, 0o644); err != nil {
			return agents.RunResult{}, err
		}
		return agents.RunResult{
			ExitCode:   0,
			StartedAt:  "2026-06-10T16:30:51Z",
			FinishedAt: "2026-06-10T16:30:52Z",
			StdoutPath: artifactDir + "/" + request.AttemptID + "/stdout.log",
			StderrPath: artifactDir + "/" + request.AttemptID + "/stderr.log",
		}, nil
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stdout.log"), nil, 0o644); err != nil {
		return agents.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stderr.log"), []byte("attempt killed: wall-clock cap exceeded\n"), 0o644); err != nil {
		return agents.RunResult{}, err
	}
	return agents.RunResult{
		ExitCode:         -1,
		StartedAt:        "2026-06-10T16:30:51Z",
		FinishedAt:       "2026-06-10T16:32:51Z",
		WallClockTimeout: true,
		StdoutPath:       artifactDir + "/" + request.AttemptID + "/stdout.log",
		StderrPath:       artifactDir + "/" + request.AttemptID + "/stderr.log",
	}, errors.New("attempt killed: wall-clock cap exceeded")
}

func TestReviewRunWallClockKilledLensIsSkippedNotParseMiss(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	// First lens call succeeds with empty findings; all subsequent lens calls are
	// wall-clock-killed. This ensures successCount > 0 so the second-pass loop
	// runs and exercises the WallClockTimeout → no-ParseMiss code path.
	app.AgentTransport = &partialWallClockKilledTransport{
		stdout: reviewerStructuredOutput([]map[string]any{}),
	}

	var stdout, stderr bytes.Buffer
	// The one successful lens produced empty findings; the wall-clock-killed
	// lenses are in skipped_lenses. The round must converge (exit 0) because
	// no blocking findings were proposed by the surviving lens.
	code := app.Run([]string{"review", "run", runID, "--reviewer", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review round with one successful empty-findings lens should converge (exit 0), got %d\nstderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}

	// The terminal reason must not be reviewer_findings_unparsed (parse miss).
	// Wall-clock-killed lenses must go to skipped_lenses, not ParseMiss.
	out := stdout.String()
	if strings.Contains(out, "reviewer_findings_unparsed") {
		t.Fatalf("wall-clock-killed lens must not produce parse miss; got output:\n%s", out)
	}

	// The round summary records the killed lenses in skipped_lenses.
	var loopSummary reviewLoopSummaryDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, runPaths.ReviewLoopSummaryJSON)), &loopSummary))
	if len(loopSummary.Rounds) == 0 {
		t.Fatal("review loop summary has no rounds")
	}
	round := loopSummary.Rounds[0]
	if len(round.SkippedLenses) == 0 {
		t.Fatalf("wall-clock-killed lenses must appear in skipped_lenses; round: %+v", round)
	}
	for _, skipped := range round.SkippedLenses {
		if !strings.Contains(skipped.Reason, "wall-clock") {
			t.Fatalf("skipped lens reason must mention wall-clock cap, got: %q", skipped.Reason)
		}
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
