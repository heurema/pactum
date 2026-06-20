package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

// sequentialTransportCall configures one call in a sequentialFakeTransport.
type sequentialTransportCall struct {
	stdout   string
	err      error
	timedOut bool
}

// sequentialFakeTransport returns pre-configured responses in order. When all
// configured responses are exhausted it panics (the test has too many calls).
// It writes stdout.log and stderr.log for each call so the lifecycle's
// empty-output check can read the file.
type sequentialFakeTransport struct {
	mu        sync.Mutex
	calls     []sequentialTransportCall
	callCount int
}

func (tr *sequentialFakeTransport) Run(request agents.RunRequest) (agents.RunResult, error) {
	tr.mu.Lock()
	idx := tr.callCount
	tr.callCount++
	call := tr.calls[idx]
	tr.mu.Unlock()

	artifactDir := request.ArtifactDir
	if artifactDir == "" {
		artifactDir = "execute/attempts"
	}
	attemptDir := filepath.Join(request.RepoRoot, artifacts.WorkspaceRel, "runs", request.RunID, filepath.FromSlash(artifactDir), request.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return agents.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stdout.log"), []byte(call.stdout), 0o644); err != nil {
		return agents.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stderr.log"), nil, 0o644); err != nil {
		return agents.RunResult{}, err
	}

	result := agents.RunResult{
		StartedAt:  "2026-06-20T10:00:00Z",
		FinishedAt: "2026-06-20T10:00:01Z",
		StdoutPath: artifactDir + "/" + request.AttemptID + "/stdout.log",
		StderrPath: artifactDir + "/" + request.AttemptID + "/stderr.log",
		TimedOut:   call.timedOut,
	}
	if call.err != nil {
		result.ExitCode = -1
		return result, call.err
	}
	return result, nil
}

func (tr *sequentialFakeTransport) count() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.callCount
}

// findRetryDecisionsFile returns the path of the first retry-decisions.jsonl
// found under root, or "" if none exists.
func findRetryDecisionsFile(root string) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !d.IsDir() && d.Name() == "retry-decisions.jsonl" {
			found = path
		}
		return nil
	})
	return found
}

// readRetryDecisions reads and parses all JSONL lines from path.
func readRetryDecisions(t *testing.T, path string) []retryDecisionRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read retry-decisions.jsonl: %v", err)
	}
	var records []retryDecisionRecord
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec retryDecisionRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal retry-decisions.jsonl line %q: %v", line, err)
		}
		records = append(records, rec)
	}
	return records
}

// transientErr is a synthetic transient error that ClassifyTransportError marks
// as retryable (rate_limit).
var transientErr = errors.New("rate limit exceeded: 429 Too Many Requests")

// permanentErr is a synthetic permanent error that ClassifyTransportError marks
// as non-retryable (auth).
var permanentErr = errors.New("401 Unauthorized: invalid API key")

// TestReadOnlyTransientRetrySucceeds verifies that a read-only lifecycle stage
// retries a transient transport failure and succeeds on the second call.
func TestReadOnlyTransientRetrySucceeds(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &sequentialFakeTransport{
		calls: []sequentialTransportCall{
			{err: transientErr},                         // first call: transient failure
			{stdout: contractDrafterStructuredOutput()}, // second call: success
		},
	}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract draft should succeed on retry, exited %d\nstderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}
	if got := transport.count(); got != 2 {
		t.Fatalf("transport call count = %d, want 2 (initial + 1 retry)", got)
	}

	// Retry-decision artifact: one record for the failed first call.
	decPath := findRetryDecisionsFile(root)
	if decPath == "" {
		t.Fatal("retry-decisions.jsonl not found")
	}
	recs := readRetryDecisions(t, decPath)
	if len(recs) != 1 {
		t.Fatalf("retry-decisions.jsonl record count = %d, want 1", len(recs))
	}
	rec := recs[0]
	if rec.Schema != "pactum.agent_retry_decision.v1alpha1" {
		t.Errorf("Schema = %q, want pactum.agent_retry_decision.v1alpha1", rec.Schema)
	}
	if rec.Stage != "draft" {
		t.Errorf("Stage = %q, want draft", rec.Stage)
	}
	if !rec.Retryable {
		t.Errorf("Retryable = false, want true (transient error)")
	}
	if rec.ReadOnly != true {
		t.Errorf("ReadOnly = false, want true (draft is read-only)")
	}
	if rec.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", rec.AttemptNumber)
	}
	if rec.DelayMS != 1000 {
		t.Errorf("DelayMS = %d, want 1000 (first retry delay)", rec.DelayMS)
	}
	if rec.AttemptID == "" {
		t.Error("AttemptID must not be empty")
	}
	if filepath.IsAbs(rec.AttemptID) {
		t.Errorf("AttemptID must not be an absolute path: %q", rec.AttemptID)
	}
}

// TestReadOnlyPermanentErrorNoRetry verifies that a permanent transport failure
// on a read-only stage is not retried.
func TestReadOnlyPermanentErrorNoRetry(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &sequentialFakeTransport{
		calls: []sequentialTransportCall{
			{err: permanentErr},
		},
	}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for permanent transport failure")
	}
	if got := transport.count(); got != 1 {
		t.Fatalf("transport call count = %d, want 1 (permanent error, no retry)", got)
	}

	decPath := findRetryDecisionsFile(root)
	if decPath == "" {
		t.Fatal("retry-decisions.jsonl not found")
	}
	recs := readRetryDecisions(t, decPath)
	if len(recs) != 1 {
		t.Fatalf("retry-decisions.jsonl record count = %d, want 1", len(recs))
	}
	rec := recs[0]
	if rec.Retryable {
		t.Errorf("Retryable = true, want false (permanent error)")
	}
	if rec.DelayMS != 0 {
		t.Errorf("DelayMS = %d, want 0 (no retry scheduled)", rec.DelayMS)
	}
	if rec.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", rec.AttemptNumber)
	}
}

// TestWriteEnabledTransientNoRetry verifies that a write-enabled (execute) stage
// is never retried, even for a transient transport failure.
func TestWriteEnabledTransientNoRetry(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &sequentialFakeTransport{
		calls: []sequentialTransportCall{
			{err: transientErr},
		},
	}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "claude"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for transport failure on write stage")
	}
	if got := transport.count(); got != 1 {
		t.Fatalf("transport call count = %d, want 1 (write stage must not retry)", got)
	}

	// Write-enabled stages must not produce a retry-decisions artifact.
	if decPath := findRetryDecisionsFile(root); decPath != "" {
		t.Errorf("retry-decisions.jsonl must not be written for write-enabled stage: %s", decPath)
	}
}

// TestEmptyOutputRetryCapThreeCalls verifies that an all-empty-output read-only
// stage retries up to the max-attempts cap (3 total calls) and then returns the
// final empty result.
func TestEmptyOutputRetryCapThreeCalls(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &sequentialFakeTransport{
		calls: []sequentialTransportCall{
			{stdout: ""}, // call 1: empty output
			{stdout: ""}, // call 2: empty output
			{stdout: ""}, // call 3: empty output (final, no more retries)
		},
	}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	// The command will fail (empty output can't be parsed), but we only check
	// the transport call count and artifact.
	app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)

	if got := transport.count(); got != 3 {
		t.Fatalf("transport call count = %d, want 3 (initial + 2 retries at cap)", got)
	}

	decPath := findRetryDecisionsFile(root)
	if decPath == "" {
		t.Fatal("retry-decisions.jsonl not found")
	}
	recs := readRetryDecisions(t, decPath)
	if len(recs) != 3 {
		t.Fatalf("retry-decisions.jsonl record count = %d, want 3", len(recs))
	}
	// First two records: retryable, positive delay.
	for i, rec := range recs[:2] {
		if rec.Kind != "empty_output" {
			t.Errorf("record[%d] Kind = %q, want empty_output", i, rec.Kind)
		}
		if !rec.Retryable {
			t.Errorf("record[%d] Retryable = false, want true (empty output is retryable)", i)
		}
		if rec.DelayMS <= 0 {
			t.Errorf("record[%d] DelayMS = %d, want > 0 (retry is scheduled)", i, rec.DelayMS)
		}
		if rec.AttemptNumber != i+1 {
			t.Errorf("record[%d] AttemptNumber = %d, want %d", i, rec.AttemptNumber, i+1)
		}
	}
	// Third record: retryable error/empty but no delay (max attempts exhausted).
	last := recs[2]
	if last.AttemptNumber != 3 {
		t.Errorf("last record AttemptNumber = %d, want 3", last.AttemptNumber)
	}
	if last.DelayMS != 0 {
		t.Errorf("last record DelayMS = %d, want 0 (no retry after cap)", last.DelayMS)
	}
}

// TestReadOnlyIdleTimeoutRetryable verifies that an idle timeout on a read-only
// stage (RunResult.TimedOut=true with context.Canceled error) is treated as
// retryable even though context.Canceled alone is not.
func TestReadOnlyIdleTimeoutRetryable(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &sequentialFakeTransport{
		calls: []sequentialTransportCall{
			{err: context.Canceled, timedOut: true},
			{stdout: contractDrafterStructuredOutput()},
		},
	}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success after idle-timeout retry, exited %d\nstderr: %s", code, stderr.String())
	}
	if got := transport.count(); got != 2 {
		t.Fatalf("transport call count = %d, want 2 (idle-timeout retry)", got)
	}

	decPath := findRetryDecisionsFile(root)
	if decPath == "" {
		t.Fatal("retry-decisions.jsonl not found")
	}
	recs := readRetryDecisions(t, decPath)
	if len(recs) != 1 {
		t.Fatalf("retry-decisions.jsonl record count = %d, want 1", len(recs))
	}
	if recs[0].Kind != "idle_timeout" {
		t.Errorf("record Kind = %q, want idle_timeout", recs[0].Kind)
	}
	if !recs[0].Retryable {
		t.Errorf("idle timeout record must be Retryable=true")
	}
}

// TestReadOnlyContextCanceledNotTimedOutNoRetry verifies that a plain
// context.Canceled without TimedOut=true is not retried.
func TestReadOnlyContextCanceledNotTimedOutNoRetry(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &sequentialFakeTransport{
		calls: []sequentialTransportCall{
			{err: context.Canceled, timedOut: false},
		},
	}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)

	if got := transport.count(); got != 1 {
		t.Fatalf("transport call count = %d, want 1 (plain canceled must not retry)", got)
	}
}

// TestWallClockTimeoutSuppressesRetry verifies that a wall-clock-cap result
// suppresses any retry even on a read-only stage.
func TestWallClockTimeoutSuppressesRetry(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &wallClockKilledAgentTransport{}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for wall-clock-killed attempt")
	}

	// wallClockKilledAgentTransport does not expose a call count but we can
	// verify no retry-decisions artifact was written.
	if decPath := findRetryDecisionsFile(root); decPath != "" {
		t.Errorf("retry-decisions.jsonl must not be written when WallClockTimeout suppresses retry: %s", decPath)
	}
}

// TestRetryDecisionArtifactNoAbsolutePaths verifies that retry-decision records
// do not contain absolute local paths in any field.
func TestRetryDecisionArtifactNoAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &sequentialFakeTransport{
		calls: []sequentialTransportCall{
			{err: transientErr},
			{stdout: contractDrafterStructuredOutput()},
		},
	}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)

	decPath := findRetryDecisionsFile(root)
	if decPath == "" {
		t.Fatal("retry-decisions.jsonl not found")
	}
	data, err := os.ReadFile(decPath)
	if err != nil {
		t.Fatalf("read retry-decisions.jsonl: %v", err)
	}
	// No field value in the JSONL should be an absolute path containing root.
	if strings.Contains(string(data), root) {
		t.Errorf("retry-decisions.jsonl must not contain absolute paths:\n%s", string(data))
	}
}

// completedDespiteTimeoutReadOnlyTransport fakes a read-only stage that
// completes with output despite the idle timeout firing. CompletedDespiteTimeout
// must suppress any retry on read-only stages.
type completedDespiteTimeoutReadOnlyTransport struct {
	callCount int
}

func (tr *completedDespiteTimeoutReadOnlyTransport) Run(request agents.RunRequest) (agents.RunResult, error) {
	tr.callCount++
	artifactDir := request.ArtifactDir
	if artifactDir == "" {
		artifactDir = "execute/attempts"
	}
	attemptDir := filepath.Join(request.RepoRoot, artifacts.WorkspaceRel, "runs", request.RunID, filepath.FromSlash(artifactDir), request.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return agents.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stdout.log"), []byte(contractDrafterStructuredOutput()), 0o644); err != nil {
		return agents.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stderr.log"), nil, 0o644); err != nil {
		return agents.RunResult{}, err
	}
	return agents.RunResult{
		ExitCode:                0,
		StartedAt:               "2026-06-20T10:00:00Z",
		FinishedAt:              "2026-06-20T10:00:01Z",
		TimedOut:                true,
		CompletedDespiteTimeout: true,
		StdoutPath:              artifactDir + "/" + request.AttemptID + "/stdout.log",
		StderrPath:              artifactDir + "/" + request.AttemptID + "/stderr.log",
	}, errors.New("agent process killed after idle timeout")
}

// TestCompletedDespiteTimeoutSuppressesRetry verifies that CompletedDespiteTimeout
// suppresses retries on a read-only stage even though the attempt carries an error.
func TestCompletedDespiteTimeoutSuppressesRetry(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &completedDespiteTimeoutReadOnlyTransport{}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("completed-despite-timeout draft should succeed, exited %d\nstderr: %s", code, stderr.String())
	}
	if got := transport.callCount; got != 1 {
		t.Fatalf("transport call count = %d, want 1 (CompletedDespiteTimeout suppresses retry)", got)
	}
	if decPath := findRetryDecisionsFile(root); decPath != "" {
		t.Errorf("retry-decisions.jsonl must not be written when CompletedDespiteTimeout suppresses retry: %s", decPath)
	}
}

// TestSecondRetryUsesLongerDelay verifies that the second retry uses a longer
// delay than the first (exponential backoff).
func TestSecondRetryUsesLongerDelay(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})

	transport := &sequentialFakeTransport{
		calls: []sequentialTransportCall{
			{err: transientErr},
			{err: transientErr},
			{stdout: contractDrafterStructuredOutput()},
		},
	}
	app.AgentTransport = transport
	app.Sleep = func(_ time.Duration) {}
	app.Jitter = func() float64 { return 1.0 }

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected success after two retries, exited %d\nstderr: %s", code, stderr.String())
	}
	if got := transport.count(); got != 3 {
		t.Fatalf("transport call count = %d, want 3", got)
	}

	decPath := findRetryDecisionsFile(root)
	if decPath == "" {
		t.Fatal("retry-decisions.jsonl not found")
	}
	recs := readRetryDecisions(t, decPath)
	if len(recs) != 2 {
		t.Fatalf("retry-decisions.jsonl record count = %d, want 2", len(recs))
	}
	if recs[0].DelayMS != 1000 {
		t.Errorf("first retry DelayMS = %d, want 1000", recs[0].DelayMS)
	}
	if recs[1].DelayMS != 2000 {
		t.Errorf("second retry DelayMS = %d, want 2000", recs[1].DelayMS)
	}
}
