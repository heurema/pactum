package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

// recordingAgentTransport is a fake agents.Transport: it captures each
// RunRequest, writes the configured agent stdout into the same attempt layout
// the real transports use (so post-run parsing keeps working), and returns a
// successful RunResult without launching any subprocess.
type recordingAgentTransport struct {
	requests []agents.RunRequest
	stdout   string
}

func (tr *recordingAgentTransport) Run(request agents.RunRequest) (agents.RunResult, error) {
	tr.requests = append(tr.requests, request)
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
	app, _, runID := setupApprovedAndBuiltPromptWithExecutorModels(t, root, agentModelEntry{Agent: "codex", Model: "gpt-5", Effort: "high"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex", "--yes"}, &stdout, &stderr)
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
	setExecutorModelsConfig(t, paths, agentModelEntry{Agent: "codex", Model: "gpt-5", Effort: "high"})
	runReviewCommand(t, app, "review", "add-finding", runID, "transport request fields")
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "fix", runID, "--agent", "codex", "--yes"}, &stdout, &stderr)
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
	setReviewPanelPinsConfig(t, paths, agentModelEntry{Agent: "codex", Model: "gpt-5", Effort: "high"})
	transport := &recordingAgentTransport{}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "codex", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if !request.ReadOnly {
		t.Fatal("review run is a read-only stage and must set ReadOnly")
	}
	if want := (agents.ModelSpec{Model: "gpt-5", Effort: "high"}); request.Model != want {
		t.Fatalf("review run model spec = %+v, want %+v", request.Model, want)
	}
}

func TestClarifySuggestMarksAttemptReadOnlyAndPassesModelSpec(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setReviewPanelPinsConfig(t, paths, agentModelEntry{Agent: "claude", Model: "claude-sonnet-4", Effort: "high"})
	transport := &recordingAgentTransport{stdout: clarifierStructuredOutput([]map[string]any{
		{
			"text":     "Should the read-only flag reach the transport?",
			"blocking": false,
			"kind":     "scope",
		},
	})}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "suggest", runID, "--reviewer", "claude", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify suggest exited %d, stderr: %s", code, stderr.String())
	}

	request := singleTransportRequest(t, transport)
	if !request.ReadOnly {
		t.Fatal("clarify suggest is a read-only stage and must set ReadOnly")
	}
	if want := (agents.ModelSpec{Model: "claude-sonnet-4", Effort: "high"}); request.Model != want {
		t.Fatalf("clarify suggest model spec = %+v, want %+v", request.Model, want)
	}
}

func TestContractDraftMarksAttemptReadOnlyAndPassesModelSpec(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	setReviewPanelPinsConfig(t, paths, agentModelEntry{Agent: "claude", Model: "claude-sonnet-4", Effort: "high"})
	transport := &recordingAgentTransport{stdout: contractDrafterStructuredOutput()}
	app.AgentTransport = transport

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "draft", runID, "--reviewer", "claude", "--yes"}, &stdout, &stderr)
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
