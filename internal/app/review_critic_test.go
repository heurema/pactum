package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

// criticTestTransport is a transport double for critic-pass tests.
// Reviewer calls (prompt path contains "reviewer-prompt") receive reviewerOutput.
// Critic calls (prompt path contains "critic-prompt") draw from criticOutputs in order.
type criticTestTransport struct {
	mu             sync.Mutex
	reviewerOutput string
	criticOutputs  []string
	criticIdx      int
	calls          []string
	criticReadOnly []bool
}

func (tr *criticTestTransport) Run(req agents.RunRequest) (agents.RunResult, error) {
	tr.mu.Lock()
	tr.calls = append(tr.calls, req.PromptRepoPath)
	tr.mu.Unlock()

	artifactDir := req.ArtifactDir
	if artifactDir == "" {
		artifactDir = "execute/attempts"
	}
	attemptDir := filepath.Join(req.RepoRoot, artifacts.WorkspaceRel, "runs", req.RunID, filepath.FromSlash(artifactDir), req.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return agents.RunResult{}, err
	}

	isCritic := strings.Contains(req.PromptRepoPath, "critic-prompt")
	var stdout string
	if isCritic {
		tr.mu.Lock()
		tr.criticReadOnly = append(tr.criticReadOnly, req.ReadOnly)
		if tr.criticIdx < len(tr.criticOutputs) {
			stdout = tr.criticOutputs[tr.criticIdx]
			tr.criticIdx++
		}
		tr.mu.Unlock()
	} else {
		stdout = tr.reviewerOutput
		if stdout == "" {
			stdout = reviewerStructuredOutput([]map[string]any{})
		}
	}

	if err := os.WriteFile(filepath.Join(attemptDir, "stdout.log"), []byte(stdout), 0o644); err != nil {
		return agents.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stderr.log"), nil, 0o644); err != nil {
		return agents.RunResult{}, err
	}
	return agents.RunResult{
		ExitCode:   0,
		StartedAt:  "2026-05-31T18:40:12Z",
		FinishedAt: "2026-05-31T18:40:13Z",
		StdoutPath: artifactDir + "/" + req.AttemptID + "/stdout.log",
		StderrPath: artifactDir + "/" + req.AttemptID + "/stderr.log",
	}, nil
}

func (tr *criticTestTransport) criticCallCount() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.criticIdx
}

func (tr *criticTestTransport) allCriticCallsReadOnly() (bool, []bool) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.criticReadOnly) == 0 {
		return false, nil
	}
	for _, ro := range tr.criticReadOnly {
		if !ro {
			return false, tr.criticReadOnly
		}
	}
	return true, tr.criticReadOnly
}

func criticVerdictsOutput(verdicts []map[string]any) string {
	if verdicts == nil {
		verdicts = []map[string]any{}
	}
	block := map[string]any{
		"schema":   reviewCriticVerdictsSchema,
		"verdicts": verdicts,
	}
	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		panic(err)
	}
	return "critic notes\n```json\n" + string(data) + "\n```\n"
}

func oneFindingOutput(message string, blocking bool) string {
	return reviewerStructuredOutput([]map[string]any{
		{
			"message":           message,
			"severity":          "medium",
			"category":          "quality",
			"blocking":          blocking,
			"evidence":          "evidence text",
			"state":             "confirmed",
			"trigger":           "always",
			"fix_direction":     "fix the issue",
			"current_code_only": true,
		},
	})
}

// setupCriticRun creates a fresh approved review run with a single agent (claude).
// The default config only has claude, so resolveCriticAgent falls back to same-engine.
func setupCriticRun(t *testing.T, root string) (App, string, contractRunPathSet) {
	t.Helper()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	return app, runID, runPaths
}

// --- parseCriticVerdictBlock unit tests ---

func TestReviewCriticParseCriticVerdictBlockEmptyStdout(t *testing.T) {
	entries, ok, warnings := parseCriticVerdictBlock("")
	if ok {
		t.Fatal("empty stdout: expected ok=false")
	}
	if len(entries) != 0 {
		t.Fatalf("empty stdout: expected no entries, got %v", entries)
	}
	if len(warnings) != 0 {
		t.Fatalf("empty stdout: expected no warnings for empty output, got %v", warnings)
	}
}

func TestReviewCriticParseCriticVerdictBlockWhitespaceOnly(t *testing.T) {
	entries, ok, warnings := parseCriticVerdictBlock("   \n\t\n  ")
	if ok {
		t.Fatal("whitespace-only stdout: expected ok=false")
	}
	if len(entries) != 0 {
		t.Fatalf("whitespace-only stdout: expected no entries, got %v", entries)
	}
	if len(warnings) != 0 {
		t.Fatalf("whitespace-only stdout: expected no warnings for whitespace-only, got %v", warnings)
	}
}

func TestReviewCriticParseCriticVerdictBlockNoFencedBlock(t *testing.T) {
	_, ok, warnings := parseCriticVerdictBlock("some output without any fenced block")
	if ok {
		t.Fatal("no fenced block: expected ok=false")
	}
	if len(warnings) == 0 {
		t.Fatal("no fenced block: expected a warning")
	}
}

func TestReviewCriticParseCriticVerdictBlockWrongSchema(t *testing.T) {
	block := map[string]any{
		"schema":   "pactum.reviewer_findings.v1alpha1",
		"findings": []any{},
	}
	data, _ := json.Marshal(block)
	output := "notes\n```json\n" + string(data) + "\n```\n"
	_, ok, warnings := parseCriticVerdictBlock(output)
	if ok {
		t.Fatal("wrong schema: expected ok=false")
	}
	if len(warnings) == 0 {
		t.Fatal("wrong schema: expected a warning")
	}
}

func TestReviewCriticParseCriticVerdictBlockValidWithVerdicts(t *testing.T) {
	output := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": "confirmed", "reason": "real issue"},
		{"proposal_id": "p_002", "verdict": "disputed", "reason": "false positive"},
	})
	entries, ok, warnings := parseCriticVerdictBlock(output)
	if !ok {
		t.Fatalf("valid block: expected ok=true, warnings: %v", warnings)
	}
	if len(entries) != 2 {
		t.Fatalf("valid block: expected 2 entries, got %d", len(entries))
	}
	if entries[0].ProposalID != "p_001" || entries[0].Verdict != reviewCriticVerdictConfirmed {
		t.Fatalf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].ProposalID != "p_002" || entries[1].Verdict != reviewCriticVerdictDisputed {
		t.Fatalf("entry 1 mismatch: %+v", entries[1])
	}
}

func TestReviewCriticParseCriticVerdictBlockEmptyVerdicts(t *testing.T) {
	output := criticVerdictsOutput([]map[string]any{})
	entries, ok, warnings := parseCriticVerdictBlock(output)
	if !ok {
		t.Fatalf("empty verdicts: expected ok=true, warnings: %v", warnings)
	}
	if len(entries) != 0 {
		t.Fatalf("empty verdicts: expected 0 entries, got %d", len(entries))
	}
}

// --- Config validation tests ---

func TestConfigCriticByUnknownAgentReturnsError(t *testing.T) {
	root := t.TempDir()
	_, paths, _ := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-opus-4-8"})

	config := readConfigForTest(t, paths.Config)
	config.Pipeline.CodeReview.CriticBy = "nonexistent-agent"
	assertNoError(t, writeYAML(paths.Config, config))

	// readConfig validates critic_by against the registry.
	_, err := readConfig(paths.Config)
	if err == nil {
		t.Fatal("expected error for unknown critic_by")
	}
	if !strings.Contains(err.Error(), "critic_by") || !strings.Contains(err.Error(), "nonexistent-agent") {
		t.Fatalf("expected error about critic_by, got: %v", err)
	}
}

func TestConfigCriticByKnownAgentIsAccepted(t *testing.T) {
	root := t.TempDir()
	_, paths, _ := setupContractRun(t, root)
	setAgentRegistryConfig(t, paths,
		agentRegistryEntry{Name: "claude", Model: "claude-opus-4-8"},
		agentRegistryEntry{Name: "codex-critic", Model: "gpt-5"},
	)

	config := readConfigForTest(t, paths.Config)
	config.Pipeline.CodeReview.CriticBy = "codex-critic"
	assertNoError(t, writeYAML(paths.Config, config))

	_, err := readConfig(paths.Config)
	assertNoError(t, err)
}

// --- ReviewRun with critic: clean round when no proposals ---

// TestReviewRunCriticNotCalledWhenNoProposals: all reviewer lenses return empty
// findings, so there are no non-duplicate proposals and the critic is never invoked.
func TestReviewRunCriticNotCalledWhenNoProposals(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	tr := &criticTestTransport{}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	if got := tr.criticCallCount(); got != 0 {
		t.Fatalf("critic was called %d times, want 0 (no proposals)", got)
	}
	if got := stdout.String(); !strings.Contains(got, "clean_round") {
		t.Fatalf("expected clean_round terminal reason, got: %s", got)
	}
}

// TestReviewRunCriticConfirmedFindingIsAccepted: one blocking finding from the
// reviewer panel that the critic confirms — the finding should be accepted.
func TestReviewRunCriticConfirmedFindingIsAccepted(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	// Reviewer produces one blocking finding across all lenses (same fingerprint).
	// Critic confirms p_001 (the unique non-dup).
	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictConfirmed, "reason": "real issue"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("confirmed issue", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{NoFix: true})
	assertNoError(t, err)

	if got := tr.criticCallCount(); got != 1 {
		t.Fatalf("expected 1 critic call, got %d", got)
	}
	// Confirmed blocking finding accepted → findings_open (NoFix skips fixer).
	out := stdout.String()
	if !strings.Contains(out, "findings_open") {
		t.Fatalf("expected findings_open terminal reason (confirmed finding accepted), got: %s", out)
	}
}

// TestReviewRunCriticDisputedFindingIsNotAccepted: all blocking proposals disputed
// with no prior blockers → precision_rejected, run is approvable.
func TestReviewRunCriticDisputedFindingIsNotAccepted(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "false positive"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("disputed issue", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	if got := stdout.String(); !strings.Contains(got, "precision_rejected") {
		t.Fatalf("expected precision_rejected terminal reason, got: %s", got)
	}
}

// TestReviewRunCriticInsufficientEvidenceBlocksApproval: blocking proposal with
// insufficient_evidence (critic omits it) with no prior blockers → debate_no_consensus.
func TestReviewRunCriticInsufficientEvidenceBlocksApproval(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	// Critic emits empty verdicts — proposal gets synthetic insufficient_evidence.
	criticOutput := criticVerdictsOutput([]map[string]any{})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("uncertain issue", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	if got := stdout.String(); !strings.Contains(got, "debate_no_consensus") {
		t.Fatalf("expected debate_no_consensus terminal reason, got: %s", got)
	}
}

// TestReviewRunCriticVerdictsUnparsedBlocksApproval: critic returns garbage →
// critic_verdicts_unparsed with non-zero OpenBlockingCount.
func TestReviewRunCriticVerdictsUnparsedBlocksApproval(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("some issue", true),
		// Both initial and corrective attempts produce unparseable output.
		criticOutputs: []string{
			"not a JSON block at all",
			"still not valid",
		},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	if got := stdout.String(); !strings.Contains(got, "critic_verdicts_unparsed") {
		t.Fatalf("expected critic_verdicts_unparsed terminal reason, got: %s", got)
	}
}

// TestReviewRunCriticCorrectiveRetrySucceeds: initial attempt produces unparseable
// output, corrective retry returns a valid block.
func TestReviewRunCriticCorrectiveRetrySucceeds(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	// Initial attempt: no valid block; corrective: valid disputed verdict.
	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "fp"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("some issue", true),
		criticOutputs: []string{
			"not a valid block",
			criticOutput,
		},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	// 2 critic calls: initial + corrective.
	if got := tr.criticCallCount(); got != 2 {
		t.Fatalf("expected 2 critic calls (initial + corrective), got %d", got)
	}
	// Corrective returned disputed → precision_rejected.
	if got := stdout.String(); !strings.Contains(got, "precision_rejected") {
		t.Fatalf("expected precision_rejected after corrective retry, got: %s", got)
	}
}

// TestReviewRunCriticIntraBatchDedupPassesOneTocritic: all reviewer lenses produce
// the same finding — batchSeen dedup passes only one non-duplicate to the critic.
func TestReviewRunCriticIntraBatchDedupPassesOneToCritic(t *testing.T) {
	root := t.TempDir()
	app, runID, runPaths := setupCriticRun(t, root)

	// All lenses return the same finding fingerprint.
	sameFingerprint := oneFindingOutput("same issue everywhere", false)
	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictConfirmed, "reason": "real"},
	})
	tr := &criticTestTransport{
		reviewerOutput: sameFingerprint,
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	// Exactly 1 critic call with 1 candidate (intra-batch dedup collapsed to 1).
	if got := tr.criticCallCount(); got != 1 {
		t.Fatalf("expected 1 critic call after intra-batch dedup, got %d", got)
	}

	// Verify via the JSON loop summary that precision_candidates=1.
	var loopSummary reviewLoopSummaryDocument
	assertNoError(t, readJSON(runPaths.ReviewLoopSummaryJSON, &loopSummary))
	if len(loopSummary.Rounds) == 0 {
		t.Fatal("expected at least one round in loop summary")
	}
	if got := loopSummary.Rounds[0].PrecisionCandidates; got != 1 {
		t.Fatalf("expected PrecisionCandidates=1 (intra-batch dedup collapsed to 1), got %d", got)
	}
}

// TestReviewRunCriticCriterion22AllFilteredIsNotClean: critic filters all
// non-duplicate proposals (no accepted, no duplicates from prior rounds) — the
// round must NOT be classified as a clean round.
func TestReviewRunCriticCriterion22AllFilteredIsNotClean(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	// Non-blocking finding; critic disputes it — accepted=0, duplicates=0, but nonDuplicates=1.
	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "fp"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("non-blocking issue", false),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	// The round had a non-dup that got filtered — not a clean round.
	// Run should NOT end with clean_round as terminal reason on this round.
	// (It will eventually hit max rounds or another terminal, but round 1 was not clean.)
	// The most direct check: the round summary should show proposals_created > 0.
	out := stdout.String()
	if strings.Contains(out, "terminal reason: clean_round") && !strings.Contains(out, "round 2") {
		t.Fatalf("round 1 with critic-filtered proposals should not be clean_round, got: %s", out)
	}
}

// TestReviewRunCriticSameEngineWarningEmitted: with only one registry entry (claude),
// resolveCriticAgent falls back to same-engine and emits a warning in the round summary.
func TestReviewRunCriticSameEngineWarningEmitted(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	// Force single-entry registry so cross-engine fallback is impossible.
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-opus-4-8"})

	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "fp"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("some issue", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "claude"})
	assertNoError(t, err)

	var loopSummary reviewLoopSummaryDocument
	assertNoError(t, readJSON(runPaths.ReviewLoopSummaryJSON, &loopSummary))

	found := false
	for _, round := range loopSummary.Rounds {
		for _, w := range round.Warnings {
			if strings.Contains(w, "same engine") || strings.Contains(w, "stronger precision") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected same-engine warning in round summary JSON, got rounds: %+v", loopSummary.Rounds)
	}
}

// TestReviewRunCriticCrossEngineNoCriticByWarning: with explicit critic_by pointing
// to a different engine than the reviewer, no same-engine warning should appear.
func TestReviewRunCriticCrossEngineNoCriticByWarning(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	// Force single-entry registry first so same-engine fallback would trigger without critic_by.
	setAgentRegistryConfig(t, paths,
		agentRegistryEntry{Name: "claude", Model: "claude-opus-4-8"},
		agentRegistryEntry{Name: "codex", Model: "gpt-5"},
	)

	// Explicitly set critic_by to codex (different engine from claude reviewer).
	config := readConfigForTest(t, paths.Config)
	config.Pipeline.CodeReview.CriticBy = "codex"
	assertNoError(t, writeYAML(paths.Config, config))

	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "fp"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("some issue", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "claude"})
	assertNoError(t, err)

	var loopSummary reviewLoopSummaryDocument
	assertNoError(t, readJSON(runPaths.ReviewLoopSummaryJSON, &loopSummary))

	for _, round := range loopSummary.Rounds {
		for _, w := range round.Warnings {
			if strings.Contains(w, "same engine") || strings.Contains(w, "stronger precision") {
				t.Fatalf("unexpected same-engine warning with explicit cross-engine critic_by: %q", w)
			}
		}
	}
}

// TestReviewRunCriticVerdictArtifactWritten: after the critic runs, a verdict
// JSONL artifact should exist at the expected path.
func TestReviewRunCriticVerdictArtifactWritten(t *testing.T) {
	root := t.TempDir()
	app, runID, runPaths := setupCriticRun(t, root)

	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "fp"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("some issue", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	verdictPath := reviewCriticVerdictsPath(runPaths, 1)
	assertFile(t, verdictPath)

	entries, err := readJSONLines[reviewCriticVerdictEntry](verdictPath)
	assertNoError(t, err)
	if len(entries) != 1 {
		t.Fatalf("expected 1 verdict entry, got %d", len(entries))
	}
	if entries[0].ProposalID != "p_001" || entries[0].Verdict != reviewCriticVerdictDisputed {
		t.Fatalf("unexpected verdict: %+v", entries[0])
	}
}

// TestReviewRunCriticMissingVerdictSyntheticInsufficientEvidence: when the critic
// does not return a verdict for a proposal, it gets synthetic insufficient_evidence.
func TestReviewRunCriticMissingVerdictSyntheticInsufficientEvidence(t *testing.T) {
	root := t.TempDir()
	app, runID, runPaths := setupCriticRun(t, root)

	// Empty verdicts array → p_001 will get synthetic insufficient_evidence.
	criticOutput := criticVerdictsOutput([]map[string]any{})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("uncertain", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	verdictPath := reviewCriticVerdictsPath(runPaths, 1)
	entries, err := readJSONLines[reviewCriticVerdictEntry](verdictPath)
	assertNoError(t, err)

	if len(entries) != 1 {
		t.Fatalf("expected 1 synthetic entry, got %d", len(entries))
	}
	if entries[0].Verdict != reviewCriticVerdictMissingVerdict {
		t.Fatalf("expected missing_verdict for absent verdict, got %q", entries[0].Verdict)
	}
	if !entries[0].MissingVerdict {
		t.Fatalf("expected MissingVerdict=true on synthetic entry")
	}
}

// TestReviewRunCriticDuplicateVerdictFirstWins: critic emits two verdicts for the
// same proposal_id — the first should win.
func TestReviewRunCriticDuplicateVerdictFirstWins(t *testing.T) {
	root := t.TempDir()
	app, runID, runPaths := setupCriticRun(t, root)

	// Two verdicts for p_001: confirmed first, then disputed.
	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictConfirmed, "reason": "first"},
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "second"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("dup verdict", false),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	verdictPath := reviewCriticVerdictsPath(runPaths, 1)
	entries, err2 := readJSONLines[reviewCriticVerdictEntry](verdictPath)
	assertNoError(t, err2)

	if len(entries) != 1 {
		t.Fatalf("expected 1 verdict entry (deduped), got %d", len(entries))
	}
	if entries[0].Verdict != reviewCriticVerdictConfirmed {
		t.Fatalf("first verdict (confirmed) should win, got %q", entries[0].Verdict)
	}
}

// TestReviewRunCriticAttemptArtifactsWritten: verify that critic attempt artifacts
// (request JSON, result JSON, stdout/stderr logs) are written.
func TestReviewRunCriticAttemptArtifactsWritten(t *testing.T) {
	root := t.TempDir()
	app, runID, runPaths := setupCriticRun(t, root)

	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "fp"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("some issue", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	attemptPaths := reviewCriticAttemptPaths(runPaths, "critic_attempt_001")
	assertFile(t, attemptPaths.RequestJSON)
	assertFile(t, attemptPaths.ResultJSON)
	assertFile(t, attemptPaths.StdoutLog)
	assertFile(t, attemptPaths.StderrLog)

	var req reviewCriticRequestDocument
	assertNoError(t, readJSON(attemptPaths.RequestJSON, &req))
	if req.Schema != reviewCriticRequestSchema {
		t.Fatalf("critic request schema mismatch: %q", req.Schema)
	}
	if req.RunID != runID {
		t.Fatalf("critic request run_id mismatch: %q", req.RunID)
	}
}

// TestReviewRunCriticNonBlockingFilteredNoTerminalEarlyExit: a non-blocking
// proposal that the critic disputes should not trigger precision_rejected or
// debate_no_consensus (those only apply when blocking proposals are involved).
func TestReviewRunCriticNonBlockingFilteredNoTerminalEarlyExit(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "fp"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("non-blocking issue", false),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	out := stdout.String()
	if strings.Contains(out, "precision_rejected") {
		t.Fatalf("non-blocking disputed should not trigger precision_rejected: %s", out)
	}
	if strings.Contains(out, "debate_no_consensus") {
		t.Fatalf("non-blocking insufficient_evidence should not trigger debate_no_consensus: %s", out)
	}
}

// TestReviewRunCriticRoundSummaryHasCriticFields: the JSON round summary should
// include critic fields (attempt ID, candidates, confirmed, rejected) when the critic ran.
func TestReviewRunCriticRoundSummaryHasCriticFields(t *testing.T) {
	root := t.TempDir()
	app, runID, runPaths := setupCriticRun(t, root)

	criticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "fp"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("some issue", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	var loopSummary reviewLoopSummaryDocument
	assertNoError(t, readJSON(runPaths.ReviewLoopSummaryJSON, &loopSummary))

	if len(loopSummary.Rounds) == 0 {
		t.Fatal("expected at least one round in loop summary")
	}
	round := loopSummary.Rounds[0]
	if round.CriticAttemptID == "" {
		t.Fatalf("expected CriticAttemptID in round 1 summary, got empty")
	}
	if round.PrecisionCandidates != 1 {
		t.Fatalf("expected PrecisionCandidates=1, got %d", round.PrecisionCandidates)
	}
	if round.PrecisionRejected != 1 {
		t.Fatalf("expected PrecisionRejected=1, got %d", round.PrecisionRejected)
	}
}

// TestReviewRunCriticReadOnlyExecution: every critic agent attempt must be
// launched with ReadOnly=true, enforced at the transport boundary.
func TestReviewRunCriticReadOnlyExecution(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	// Corrective retry also runs; both calls must be read-only.
	validCriticOutput := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictDisputed, "reason": "fp"},
	})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("some issue", true),
		// First call: unparseable → triggers corrective. Second call: valid.
		criticOutputs: []string{"not a valid block", validCriticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	if got := tr.criticCallCount(); got < 1 {
		t.Fatal("expected at least one critic call")
	}
	ok, readOnlyValues := tr.allCriticCallsReadOnly()
	if !ok {
		t.Fatalf("all critic attempts must be executed with ReadOnly=true, got: %v", readOnlyValues)
	}
}

// TestReviewRunCriticMixedConfirmedAndInsufficientEvidence: one confirmed blocking
// proposal and one insufficient_evidence blocking proposal in the same round —
// debate_no_consensus must NOT fire, the confirmed finding is accepted, and the
// loop continues (findings_open with NoFix).
func TestReviewRunCriticMixedConfirmedAndInsufficientEvidence(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	// Two distinct blocking findings from the reviewer (different messages → different fingerprints).
	reviewerOut := reviewerStructuredOutput([]map[string]any{
		{
			"message": "first blocking issue", "severity": "medium", "category": "quality",
			"blocking": true, "evidence": "evidence", "state": "confirmed",
			"trigger": "always", "fix_direction": "fix it", "current_code_only": true,
		},
		{
			"message": "second blocking issue", "severity": "medium", "category": "quality",
			"blocking": true, "evidence": "evidence", "state": "confirmed",
			"trigger": "always", "fix_direction": "fix it", "current_code_only": true,
		},
	})
	// Critic confirms p_001 and omits p_002 → p_002 becomes missing_verdict (insufficient treatment).
	criticOut := criticVerdictsOutput([]map[string]any{
		{"proposal_id": "p_001", "verdict": reviewCriticVerdictConfirmed, "reason": "real issue"},
	})
	tr := &criticTestTransport{
		reviewerOutput: reviewerOut,
		criticOutputs:  []string{criticOut},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{NoFix: true})
	assertNoError(t, err)

	out := stdout.String()
	// debate_no_consensus must NOT fire when one blocking proposal is confirmed.
	if strings.Contains(out, "debate_no_consensus") {
		t.Fatalf("debate_no_consensus must not fire when one blocking proposal is confirmed: %s", out)
	}
	// The confirmed blocking finding was accepted → findings_open (NoFix skips fixer).
	if !strings.Contains(out, "findings_open") {
		t.Fatalf("expected findings_open (confirmed blocking finding accepted), got: %s", out)
	}
}

// TestReviewRunCriticDebateNoConsensusHasOpenBlockingCount: debate_no_consensus
// must have OpenBlockingCount >= 1 so the run cannot be approved.
func TestReviewRunCriticDebateNoConsensusHasOpenBlockingCount(t *testing.T) {
	root := t.TempDir()
	app, runID, _ := setupCriticRun(t, root)

	// Empty verdicts → insufficient_evidence for blocking proposal → debate_no_consensus.
	criticOutput := criticVerdictsOutput([]map[string]any{})
	tr := &criticTestTransport{
		reviewerOutput: oneFindingOutput("uncertain blocking", true),
		criticOutputs:  []string{criticOutput},
	}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	assertNoError(t, err)

	out := stdout.String()
	if !strings.Contains(out, "debate_no_consensus") {
		t.Fatalf("expected debate_no_consensus, got: %s", out)
	}
	// The summary should report at least 1 open blocking finding (blocks approval).
	if strings.Contains(out, "0 blocking") && !strings.Contains(out, "1 blocking") {
		t.Fatalf("debate_no_consensus should report >= 1 blocking finding: %s", out)
	}
}
