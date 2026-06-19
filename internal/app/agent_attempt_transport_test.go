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

func TestExecuteRunClaudeACPPathIsWriteStageWithModelSpec(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run (claude) exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.ReadOnly {
		t.Fatal("claude execute run is a write stage and must not set ReadOnly")
	}
	if request.WritePathAllowed == nil {
		t.Fatal("claude execute run must pass the contract write-scope predicate")
	}
	if want := (agents.ModelSpec{Model: "claude-sonnet-4", Effort: "high"}); request.Model != want {
		t.Fatalf("claude execute run model spec = %+v, want %+v", request.Model, want)
	}
	// Claude runs over ACP; the descriptor must carry no CLI flags.
	if request.Agent.Command != "" || len(request.Agent.Args) != 0 {
		t.Fatalf("claude executor descriptor must have no CLI command/args: command=%q args=%v", request.Agent.Command, request.Agent.Args)
	}
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

func TestReviewFixClaudeACPPathIsWriteStageWithModelSpec(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})
	runReviewCommand(t, app, "review", "finding", "add", runID, "transport request fields")
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "run", runID, "--agent", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review fix (claude) exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.ReadOnly {
		t.Fatal("claude review fix is a write stage and must not set ReadOnly")
	}
	if request.WritePathAllowed == nil {
		t.Fatal("claude review fix must pass the contract write-scope predicate")
	}
	if want := (agents.ModelSpec{Model: "claude-sonnet-4", Effort: "high"}); request.Model != want {
		t.Fatalf("claude review fix model spec = %+v, want %+v", request.Model, want)
	}
	// Claude runs over ACP; the descriptor must carry no CLI flags.
	if request.Agent.Command != "" || len(request.Agent.Args) != 0 {
		t.Fatalf("claude fixer descriptor must have no CLI command/args: command=%q args=%v", request.Agent.Command, request.Agent.Args)
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

func TestReviewRunClaudeACPPathIsReadOnlyWithModelSpec(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run (claude) exited %d, stderr: %s", code, stderr.String())
	}

	if len(transport.requests) != len(reviewLenses) {
		t.Fatalf("transport request count = %d, want %d", len(transport.requests), len(reviewLenses))
	}
	for _, request := range transport.requests {
		if !request.ReadOnly {
			t.Fatal("claude review run is a read-only stage and must set ReadOnly")
		}
		if want := (agents.ModelSpec{Model: "claude-sonnet-4", Effort: "high"}); request.Model != want {
			t.Fatalf("claude review run model spec = %+v, want %+v", request.Model, want)
		}
		// Reviewer descriptor must carry no CLI flags — read-only is via RunRequest.ReadOnly.
		if request.Agent.Command != "" || len(request.Agent.Args) != 0 {
			t.Fatalf("claude reviewer descriptor must have no CLI command/args: command=%q args=%v", request.Agent.Command, request.Agent.Args)
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

func TestClarifierRoundOmittedTimeoutUsesBuiltInDefault(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})
	transport := &recordingAgentTransport{stdout: clarifierStructuredOutput([]map[string]any{})}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "run", runID, "--no-auto", "--max-rounds", "1", "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.Timeout != defaultIdleTimeout {
		t.Fatalf("omitted --timeout should use the built-in default: %s, want %s", request.Timeout, defaultIdleTimeout)
	}
}

func TestClarifierRoundExplicitTimeoutOverridesDefault(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})
	transport := &recordingAgentTransport{stdout: clarifierStructuredOutput([]map[string]any{})}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "run", runID, "--no-auto", "--max-rounds", "1", "--reviewer", "claude", "--timeout", "90s"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.Timeout != 90*time.Second {
		t.Fatalf("explicit --timeout should win over the built-in default: %s, want 90s", request.Timeout)
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

func TestExecuteRunPassesDefaultWallClockCap(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.WallClockCap != defaultWallClockCap {
		t.Fatalf("omitted wall_clock_cap should use the built-in default: want %s, got %s", defaultWallClockCap, request.WallClockCap)
	}
}

func TestExecuteRunPassesConfigWallClockCapOverride(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPromptWithAgentRegistry(t, root, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	config := readConfigForTest(t, paths.Config)
	config.WallClockCap = yamlDuration(30 * time.Minute)
	assertNoError(t, writeYAML(paths.Config, config))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.WallClockCap != 30*time.Minute {
		t.Fatalf("wall_clock_cap config override should reach the transport: want 30m, got %s", request.WallClockCap)
	}
}

func TestReviewRunPassesDefaultWallClockCap(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
	}

	for _, request := range transport.requests {
		if request.WallClockCap != defaultWallClockCap {
			t.Fatalf("reviewer transport request should carry the built-in wall-clock default: want %s, got %s", defaultWallClockCap, request.WallClockCap)
		}
	}
}

func TestClarifyRunPassesDefaultWallClockCap(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4"})
	transport := &recordingAgentTransport{stdout: clarifierStructuredOutput([]map[string]any{})}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "run", runID, "--no-auto", "--max-rounds", "1", "--reviewer", "claude"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.WallClockCap != defaultWallClockCap {
		t.Fatalf("clarify run should carry the built-in wall-clock default: want %s, got %s", defaultWallClockCap, request.WallClockCap)
	}
}

func TestReviewFixRunPassesDefaultWallClockCap(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	runReviewCommand(t, app, "review", "finding", "add", runID, "transport cap test")
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review fix run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if request.WallClockCap != defaultWallClockCap {
		t.Fatalf("review fix should carry the built-in wall-clock default: want %s, got %s", defaultWallClockCap, request.WallClockCap)
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
