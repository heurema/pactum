package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

// recordingAgentTransport is a fake agents.Transport: it captures each
// RunRequest, writes the configured agent stdout into the same attempt layout
// the real transports use (so post-run parsing keeps working), and returns a
// successful RunResult without launching any subprocess. Concurrent lens
// attempts share one transport, so the capture is mutex-guarded.
type recordingAgentTransport struct {
	mu       sync.Mutex
	requests []agents.RunRequest
	stdout   string
}

func (tr *recordingAgentTransport) Run(request agents.RunRequest) (agents.RunResult, error) {
	tr.mu.Lock()
	tr.requests = append(tr.requests, request)
	tr.mu.Unlock()
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
	return agents.RunResult{
		ExitCode:   0,
		StartedAt:  "2026-05-31T18:40:12Z",
		FinishedAt: "2026-05-31T18:40:13Z",
		StdoutPath: artifactDir + "/" + request.AttemptID + "/stdout.log",
		StderrPath: artifactDir + "/" + request.AttemptID + "/stderr.log",
	}, nil
}

func singleTransportRequest(t *testing.T, transport *recordingAgentTransport) agents.RunRequest {
	t.Helper()
	if len(transport.requests) != 1 {
		t.Fatalf("transport request count = %d, want 1", len(transport.requests))
	}
	return transport.requests[0]
}

func TestExecuteRunPassesModelSpecAndStaysWriteStage(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "codex", Model: "gpt-5", Effort: "high"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.ReadOnly {
		t.Fatal("execute run is a write stage and must not set ReadOnly")
	}
	if request.WritePathAllowed == nil {
		t.Fatal("execute run must pass the contract write-scope predicate")
	}
	if want := (agents.ModelSpec{Model: "gpt-5", Effort: "high"}); request.Model != want {
		t.Fatalf("execute run model spec = %+v, want %+v", request.Model, want)
	}
}

func TestReviewFixPassesModelSpecAndStaysWriteStage(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "codex", Model: "gpt-5", Effort: "high"})
	runReviewCommand(t, app, "review", "finding", "add", runID, "transport request fields")
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review fix exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.ReadOnly {
		t.Fatal("review fix is a write stage and must not set ReadOnly")
	}
	if request.WritePathAllowed == nil {
		t.Fatal("review fix must pass the contract write-scope predicate")
	}
	if want := (agents.ModelSpec{Model: "gpt-5", Effort: "high"}); request.Model != want {
		t.Fatalf("review fix model spec = %+v, want %+v", request.Model, want)
	}
}

func TestReviewRunMarksAttemptReadOnlyAndPassesModelSpec(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "codex", Model: "gpt-5", Effort: "high"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
	}

	// One transport request per lens, every one read-only with the entry pin.
	if len(transport.requests) != len(reviewLenses) {
		t.Fatalf("transport request count = %d, want %d", len(transport.requests), len(reviewLenses))
	}
	for _, request := range transport.requests {
		if !request.ReadOnly {
			t.Fatal("review run is a read-only stage and must set ReadOnly")
		}
		if want := (agents.ModelSpec{Model: "gpt-5", Effort: "high"}); request.Model != want {
			t.Fatalf("review run model spec = %+v, want %+v", request.Model, want)
		}
	}
}

func TestClarifierRoundMarksAttemptReadOnlyAndPassesModelSpec(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})
	transport := &recordingAgentTransport{stdout: clarifierStructuredOutput([]map[string]any{
		{
			"text":     "Should the read-only flag reach the transport?",
			"blocking": false,
			"kind":     "scope",
		},
	})}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "run", runID, "--no-auto", "--max-rounds", "1", "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if !request.ReadOnly {
		t.Fatal("the clarifier round is a read-only stage and must set ReadOnly")
	}
	if want := (agents.ModelSpec{Model: "claude-sonnet-4", Effort: "high"}); request.Model != want {
		t.Fatalf("clarifier round model spec = %+v, want %+v", request.Model, want)
	}
}

// setIdleTimeoutConfig sets timeouts.idle in the workspace config.
func setIdleTimeoutConfig(t *testing.T, paths artifacts.Paths, idle string) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	config.Timeouts.Idle = idle
	assertNoError(t, writeYAML(paths.Config, config))
}

func TestClarifierRoundOmittedTimeoutUsesConfigIdleDefault(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})
	setIdleTimeoutConfig(t, paths, "15m")
	transport := &recordingAgentTransport{stdout: clarifierStructuredOutput([]map[string]any{})}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "run", runID, "--no-auto", "--max-rounds", "1", "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.Timeout != 15*time.Minute {
		t.Fatalf("omitted --timeout should resolve timeouts.idle: %s, want 15m", request.Timeout)
	}
}

func TestClarifierRoundExplicitTimeoutOverridesConfigIdleDefault(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})
	setIdleTimeoutConfig(t, paths, "15m")
	transport := &recordingAgentTransport{stdout: clarifierStructuredOutput([]map[string]any{})}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "run", runID, "--no-auto", "--max-rounds", "1", "--reviewer", "claude", "--timeout", "90s"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.Timeout != 90*time.Second {
		t.Fatalf("explicit --timeout should win over timeouts.idle: %s, want 90s", request.Timeout)
	}
}

func TestExecuteRunNegativeTimeoutIsRejected(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex", "--timeout=-5s"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("execute run with a negative --timeout should fail")
	}
	if !strings.Contains(stderr.String(), "timeout must be positive") {
		t.Fatalf("negative --timeout error mismatch:\n%s", stderr.String())
	}
	if len(transport.requests) != 0 {
		t.Fatalf("no agent should launch on a rejected --timeout, got %d requests", len(transport.requests))
	}
}

func TestContractDraftMarksAttemptReadOnlyAndPassesModelSpec(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})
	transport := &recordingAgentTransport{stdout: contractDrafterStructuredOutput()}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "draft", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract draft exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if !request.ReadOnly {
		t.Fatal("contract draft is a read-only stage and must set ReadOnly")
	}
	if want := (agents.ModelSpec{Model: "claude-sonnet-4", Effort: "high"}); request.Model != want {
		t.Fatalf("contract draft model spec = %+v, want %+v", request.Model, want)
	}
}
