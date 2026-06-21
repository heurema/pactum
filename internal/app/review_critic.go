package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
)

const (
	reviewCriticRequestSchema    = "pactum.review_critic_request.v1alpha1"
	reviewCriticResultSchema     = "pactum.review_critic_result.v1alpha1"
	reviewCriticVerdictsSchema   = "pactum.review_critic_verdicts.v1alpha1"
	reviewCriticAttemptsArtifact = "review/critic-attempts"
	reviewCriticAttemptPrefix    = "critic_attempt"

	reviewCriticVerdictConfirmed            = "confirmed"
	reviewCriticVerdictDisputed             = "disputed"
	reviewCriticVerdictInsufficientEvidence = "insufficient_evidence"
	reviewCriticVerdictMissingVerdict       = "missing_verdict"
)

type reviewCriticVerdictEntry struct {
	ProposalID     string `json:"proposal_id"`
	Verdict        string `json:"verdict"`
	Reason         string `json:"reason,omitempty"`
	MissingVerdict bool   `json:"missing_verdict,omitempty"`
}

type reviewCriticOutputBlock struct {
	Schema   string                     `json:"schema"`
	Verdicts []reviewCriticVerdictEntry `json:"verdicts"`
}

type reviewCriticDryRunArtifacts struct {
	CriticPrompt string `json:"critic_prompt"`
}

type reviewCriticAttemptPlan struct {
	Artifacts reviewCriticDryRunArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand        `json:"would_run"`
}

type reviewCriticRequestDocument struct {
	Schema    string                      `json:"schema"`
	RunID     string                      `json:"run_id"`
	AttemptID string                      `json:"attempt_id"`
	CreatedAt string                      `json:"created_at"`
	Reviewer  agents.AgentDescriptor      `json:"reviewer"`
	Artifacts reviewCriticDryRunArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand        `json:"would_run"`
}

type reviewCriticResultDocument struct {
	Schema    string `json:"schema"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Reviewer  string `json:"reviewer"`
	processResult
}

type reviewCriticPassResult struct {
	Verdicts            map[string]reviewCriticVerdictEntry
	InitialAttemptID    string
	CorrectiveAttemptID string
	Candidates          int
	Confirmed           int
	Disputed            int
	Unresolved          int
	ParseFailed         bool
	VerdictArtifactPath string
	Warnings            []string
}

func reviewCriticAttemptPaths(runPaths contractRunPathSet, attemptID string) attemptPathSet {
	return agentAttemptPaths(runPaths.ReviewCriticAttemptsDir, attemptID)
}

func reviewCriticPromptArtifact(round int) string {
	return fmt.Sprintf("review/critic-prompt-round%d.md", round)
}

func reviewCriticPromptPath(runPaths contractRunPathSet, round int) string {
	return filepath.Join(runPaths.ReviewDir, fmt.Sprintf("critic-prompt-round%d.md", round))
}

func reviewCriticCorrectivePromptArtifact(round int) string {
	return fmt.Sprintf("review/critic-prompt-round%d-corrective.md", round)
}

func reviewCriticCorrectivePromptPath(runPaths contractRunPathSet, round int) string {
	return filepath.Join(runPaths.ReviewDir, fmt.Sprintf("critic-prompt-round%d-corrective.md", round))
}

func reviewCriticVerdictsArtifact(round int) string {
	return fmt.Sprintf("review/critic-verdicts-round%d.jsonl", round)
}

func reviewCriticVerdictsPath(runPaths contractRunPathSet, round int) string {
	return filepath.Join(runPaths.ReviewDir, fmt.Sprintf("critic-verdicts-round%d.jsonl", round))
}

// resolveCriticAgent selects the critic agent for the code-review pass.
// Selection order: explicit critic_by config > first different-engine registry entry > first entry (with warning).
func (a App) resolveCriticAgent(context reviewContext, config configFile, reviewers []reviewLoopReviewer) (reviewLoopReviewer, string, error) {
	if name := strings.TrimSpace(config.Pipeline.CodeReview.CriticBy); name != "" {
		entry, err := findRegistryEntry(config, name)
		if err != nil {
			return reviewLoopReviewer{}, "", err
		}
		resolved, err := a.resolveAgentForRole(entry, agentRoleReviewer)
		if err != nil {
			return reviewLoopReviewer{}, "", err
		}
		return reviewLoopReviewer{Name: resolved.Name, Agent: resolved.Agent, ModelSpec: resolved.ModelSpec}, "", nil
	}

	var reviewerEngine string
	if len(reviewers) > 0 {
		reviewerEngine = reviewers[0].Agent.Name
	}

	for _, entry := range config.Agents {
		engine, err := registryEntryEngine(entry)
		if err != nil {
			return reviewLoopReviewer{}, "", err
		}
		if engine != reviewerEngine {
			resolved, err := a.resolveAgentForRole(entry, agentRoleReviewer)
			if err != nil {
				return reviewLoopReviewer{}, "", err
			}
			return reviewLoopReviewer{Name: resolved.Name, Agent: resolved.Agent, ModelSpec: resolved.ModelSpec}, "", nil
		}
	}

	warning := fmt.Sprintf("critic resolved to same engine as reviewer panel (%s); add a different-engine registry entry for stronger precision filtering", reviewers[0].Name)
	return reviewers[0], warning, nil
}

func renderReviewCriticPrompt(runID string, round int, proposals []reviewProposalRecord) string {
	contractPath := runArtifactRepoRel(runID, "contract/contract.json")
	gateReportPath := runArtifactRepoRel(runID, gateReportArtifact)
	findingsPath := runArtifactRepoRel(runID, reviewFindingsArtifact)
	resolutionsPath := runArtifactRepoRel(runID, reviewResolutionsArtifact)

	var b strings.Builder
	fmt.Fprintln(&b, "# Critic Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "You are a precision critic in the Pactum code-review pipeline.")
	fmt.Fprintf(&b, "Round %d reviewer agents have proposed candidate findings. Evaluate each candidate for credibility.\n", round)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Objective")
	fmt.Fprintln(&b, "Filter false positives from the reviewer panel's proposals.")
	fmt.Fprintln(&b, "For each proposal decide: confirmed (the issue is real), disputed (it is a false positive),")
	fmt.Fprintln(&b, "or insufficient_evidence (credibility cannot be determined — name exactly what evidence is missing in reason).")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Inputs")
	fmt.Fprintf(&b, "- Contract: %s\n", contractPath)
	fmt.Fprintf(&b, "- Gate report: %s\n", gateReportPath)
	fmt.Fprintf(&b, "- Review findings: %s\n", findingsPath)
	fmt.Fprintf(&b, "- Review resolutions: %s\n", resolutionsPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Candidate findings to evaluate")
	fmt.Fprintln(&b)
	for _, p := range proposals {
		fmt.Fprintf(&b, "### %s\n", p.ID)
		fmt.Fprintf(&b, "- Message: %s\n", p.Message)
		fmt.Fprintf(&b, "- Severity: %s\n", p.Severity)
		fmt.Fprintf(&b, "- Category: %s\n", p.Category)
		fmt.Fprintf(&b, "- Blocking: %t\n", p.Blocking)
		if p.File != "" {
			if p.Line > 0 {
				fmt.Fprintf(&b, "- Location: %s:%d\n", p.File, p.Line)
			} else {
				fmt.Fprintf(&b, "- Location: %s\n", p.File)
			}
		}
		if p.Evidence != "" {
			fmt.Fprintf(&b, "- Evidence: %s\n", p.Evidence)
		}
		if p.Trigger != "" {
			fmt.Fprintf(&b, "- Trigger: %s\n", p.Trigger)
		}
		if p.FixDirection != "" {
			fmt.Fprintf(&b, "- Fix direction: %s\n", p.FixDirection)
		}
		if p.Uncertainty != "" {
			fmt.Fprintf(&b, "- Uncertainty: %s\n", p.Uncertainty)
		}
		fmt.Fprintf(&b, "- State: %s\n", p.State)
		fmt.Fprintln(&b)
	}
	fmt.Fprintln(&b, "## How to evaluate")
	fmt.Fprintln(&b, "For each candidate:")
	fmt.Fprintln(&b, "- Read the actual code at the cited file and line.")
	fmt.Fprintln(&b, "- Check whether the trigger condition is real and the evidence is concrete.")
	fmt.Fprintln(&b, "- Verify the fix_direction is actionable and addresses the issue.")
	fmt.Fprintln(&b, "- Set verdict=confirmed if you are confident the issue is real after verification.")
	fmt.Fprintln(&b, "- Set verdict=disputed if you are confident the issue is a false positive.")
	fmt.Fprintln(&b, "- Set verdict=insufficient_evidence if you cannot determine credibility; name exactly what is missing in the reason field.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Required structured output")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "You MUST emit exactly one fenced JSON block using the schema below.")
	fmt.Fprintln(&b, "Include only proposals you can reach a verdict on. Omit proposals you are uncertain about.")
	fmt.Fprintln(&b, "Prose commentary is supplemental; the parser uses only the JSON block.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", reviewCriticVerdictsSchema)
	fmt.Fprintln(&b, `  "verdicts": [`)
	fmt.Fprintln(&b, "    {")
	fmt.Fprintln(&b, `      "proposal_id": "p_001",`)
	fmt.Fprintln(&b, `      "verdict": "confirmed",`)
	fmt.Fprintln(&b, `      "reason": "The issue is real: ..."`)
	fmt.Fprintln(&b, "    }")
	fmt.Fprintln(&b, "  ]")
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Rules:")
	fmt.Fprintf(&b, "- Use verdict: %q, %q, or %q.\n", reviewCriticVerdictConfirmed, reviewCriticVerdictDisputed, reviewCriticVerdictInsufficientEvidence)
	fmt.Fprintln(&b, "- verdict=confirmed: you verified the issue is real with concrete evidence.")
	fmt.Fprintln(&b, "- verdict=disputed: you verified the issue is a false positive with counter-evidence.")
	fmt.Fprintln(&b, "- verdict=insufficient_evidence: you cannot determine credibility; name exactly what is missing in reason.")
	fmt.Fprintln(&b, "- Use proposal_id exactly as shown in the candidates above.")
	fmt.Fprintln(&b, "- Do not invent new findings or modify existing ones.")
	return b.String()
}

func renderReviewCriticCorrectivePrompt(runID string, round int) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Corrective Critic Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Your previous response did not include a valid `pactum.review_critic_verdicts.v1alpha1` JSON block.")
	fmt.Fprintln(&b, "Re-evaluate the candidate findings using the critic prompt and emit the required structured output.")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "The critic prompt for round %d is at: %s\n", round, runArtifactRepoRel(runID, reviewCriticPromptArtifact(round)))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Required structured output")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "You MUST emit exactly one fenced JSON block. If you have no verdicts, emit `\"verdicts\": []`.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", reviewCriticVerdictsSchema)
	fmt.Fprintln(&b, `  "verdicts": []`)
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	return b.String()
}

// parseCriticVerdictBlock extracts verdict entries from the critic's stdout.
// Returns (entries, ok, warnings). ok is true only when a valid schema block was found.
// Empty stdout returns (nil, false, nil) — no warnings for genuinely empty output.
func parseCriticVerdictBlock(stdout string) ([]reviewCriticVerdictEntry, bool, []string) {
	blocks := extractFencedJSONBlocks(stdout)
	if len(blocks) == 0 {
		if strings.TrimSpace(stdout) == "" {
			return nil, false, nil
		}
		return nil, false, []string{"parse miss: no fenced JSON block found in critic output"}
	}
	for _, block := range blocks {
		var out reviewCriticOutputBlock
		if err := json.Unmarshal([]byte(block), &out); err != nil {
			continue
		}
		if out.Schema != reviewCriticVerdictsSchema {
			continue
		}
		if out.Verdicts == nil {
			out.Verdicts = []reviewCriticVerdictEntry{}
		}
		return out.Verdicts, true, nil
	}
	return nil, false, []string{fmt.Sprintf("parse miss: no valid %s block found", reviewCriticVerdictsSchema)}
}

func (a App) runCriticAgentAttempt(resultBuf *bytes.Buffer, liveOutput io.Writer, context reviewContext, critic reviewLoopReviewer, timeout, wallClockCap time.Duration, promptRepoPath string) error {
	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[reviewCriticAttemptPlan, reviewCriticRequestDocument, reviewCriticResultDocument, struct{}]{
		Stdout:          resultBuf,
		LiveOutput:      liveOutput,
		JSONOutput:      true,
		Root:            context.Root,
		EventsJSONL:     context.Paths.EventsJSONL,
		RunID:           context.State.RunID,
		Stage:           "review_critic",
		AttemptsDir:     context.RunPaths.ReviewCriticAttemptsDir,
		AttemptIDPrefix: reviewCriticAttemptPrefix,
		LastResultJSON:  context.RunPaths.ReviewCriticLastResultJSON,
		AgentName:       critic.Name,
		Agent:           critic.Agent,
		Model:           critic.ModelSpec,
		PromptRepoPath:  promptRepoPath,
		ArtifactDir:     reviewCriticAttemptsArtifact,
		Timeout:         timeout,
		WallClockCap:    wallClockCap,
		ReadOnly:        true,
		StartedEvent:    "critic_attempt_started",
		FinishedEvent:   "critic_attempt_finished",
		ExitKind:        "critic",
		TimeoutMessage: func(d time.Duration) string {
			return fmt.Sprintf("critic process produced no output for %s", d)
		},
		Prepare: func(createdAt string) (reviewCriticAttemptPlan, error) {
			wouldRun, err := agents.BuildACPWouldRun(critic.Agent.Name, critic.ModelSpec, true)
			if err != nil {
				return reviewCriticAttemptPlan{}, err
			}
			return reviewCriticAttemptPlan{
				Artifacts: reviewCriticDryRunArtifacts{CriticPrompt: promptRepoPath},
				WouldRun:  wouldRun,
			}, nil
		},
		BuildRequest: func(ctx agentAttemptContext[reviewCriticAttemptPlan]) (reviewCriticRequestDocument, error) {
			return reviewCriticRequestDocument{
				Schema:    reviewCriticRequestSchema,
				RunID:     context.State.RunID,
				AttemptID: ctx.AttemptID,
				CreatedAt: ctx.CreatedAt,
				Reviewer:  agentDescriptorDocument(critic.Agent),
				Artifacts: ctx.Prepared.Artifacts,
				WouldRun:  ctx.Prepared.WouldRun,
			}, nil
		},
		BuildResult: func(ctx agentAttemptContext[reviewCriticAttemptPlan], runResult agents.RunResult) reviewCriticResultDocument {
			return reviewCriticResultDocument{
				Schema:        reviewCriticResultSchema,
				RunID:         context.State.RunID,
				AttemptID:     ctx.AttemptID,
				Reviewer:      critic.Agent.Name,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result reviewCriticResultDocument) processResult {
			return result.processResult
		},
		RenderRunOnly: func(w io.Writer, req reviewCriticRequestDocument, res reviewCriticResultDocument) {
			fmt.Fprintf(w, "critic attempt %s exit code %d\n", res.AttemptID, res.ExitCode)
		},
	})
}

func (a App) runCriticAttemptAndGetResult(liveOutput io.Writer, context reviewContext, critic reviewLoopReviewer, timeout, wallClockCap time.Duration, promptRepoPath string) (reviewCriticResultDocument, error) {
	var resultBuf bytes.Buffer
	runErr := a.runCriticAgentAttempt(&resultBuf, liveOutput, context, critic, timeout, wallClockCap, promptRepoPath)
	var result reviewCriticResultDocument
	if resultBuf.Len() > 0 {
		if err := json.Unmarshal(resultBuf.Bytes(), &result); err != nil {
			if runErr != nil {
				return reviewCriticResultDocument{}, runErr
			}
			return reviewCriticResultDocument{}, err
		}
	}
	return result, runErr
}

// runReviewCriticPass runs the critic agent against the given non-duplicate proposals.
// It handles one corrective retry when the initial attempt produces output but no valid block.
// Exec errors (transport never started, or post-start failure) produce ParseFailed with no corrective.
func (a App) runReviewCriticPass(context reviewContext, liveOutput io.Writer, runID string, round int, critic reviewLoopReviewer, proposals []reviewProposalRecord, timeout, wallClockCap time.Duration) (reviewCriticPassResult, error) {
	promptPath := reviewCriticPromptPath(context.RunPaths, round)
	if err := activeStore.WriteBytes(promptPath, []byte(renderReviewCriticPrompt(runID, round, proposals)), 0o644); err != nil {
		return reviewCriticPassResult{}, err
	}
	promptRepoPath := runArtifactRepoRel(runID, reviewCriticPromptArtifact(round))

	initialResult, runErr := a.runCriticAttemptAndGetResult(liveOutput, context, critic, timeout, wallClockCap, promptRepoPath)
	initialAttemptID := initialResult.AttemptID

	var failedErr agentAttemptFailedError
	if runErr != nil && !errors.As(runErr, &failedErr) {
		// Pre-start failure: transport never launched.
		return reviewCriticPassResult{
			ParseFailed:      true,
			InitialAttemptID: initialAttemptID,
			Candidates:       len(proposals),
			Warnings:         []string{fmt.Sprintf("critic attempt failed before starting: %v", runErr)},
		}, nil
	}
	if runErr != nil {
		// Post-start exec error: no corrective retry.
		return reviewCriticPassResult{
			ParseFailed:      true,
			InitialAttemptID: initialAttemptID,
			Candidates:       len(proposals),
			Warnings:         []string{fmt.Sprintf("critic attempt exec error: %v", runErr)},
		}, nil
	}

	stdoutBytes, _ := activeStore.ReadBytes(reviewCriticAttemptPaths(context.RunPaths, initialAttemptID).StdoutLog)
	if len(strings.TrimSpace(string(stdoutBytes))) == 0 {
		return reviewCriticPassResult{
			ParseFailed:      true,
			InitialAttemptID: initialAttemptID,
			Candidates:       len(proposals),
			Warnings:         []string{"critic attempt produced no output"},
		}, nil
	}

	entries, ok, parseWarnings := parseCriticVerdictBlock(string(stdoutBytes))
	var correctiveAttemptID string
	var allWarnings []string
	allWarnings = append(allWarnings, parseWarnings...)

	if !ok {
		// Non-empty output but no valid block — one corrective retry.
		allWarnings = append(allWarnings, "critic output not parseable — corrective retry")

		corrPath := reviewCriticCorrectivePromptPath(context.RunPaths, round)
		if err := activeStore.WriteBytes(corrPath, []byte(renderReviewCriticCorrectivePrompt(runID, round)), 0o644); err != nil {
			return reviewCriticPassResult{}, err
		}
		corrRepoPath := runArtifactRepoRel(runID, reviewCriticCorrectivePromptArtifact(round))

		corrResult, corrErr := a.runCriticAttemptAndGetResult(liveOutput, context, critic, timeout, wallClockCap, corrRepoPath)
		correctiveAttemptID = corrResult.AttemptID

		if corrErr != nil {
			allWarnings = append(allWarnings, fmt.Sprintf("corrective critic attempt failed: %v", corrErr))
			return reviewCriticPassResult{
				ParseFailed:         true,
				InitialAttemptID:    initialAttemptID,
				CorrectiveAttemptID: correctiveAttemptID,
				Candidates:          len(proposals),
				Warnings:            allWarnings,
			}, nil
		}

		corrStdoutBytes, _ := activeStore.ReadBytes(reviewCriticAttemptPaths(context.RunPaths, correctiveAttemptID).StdoutLog)
		corrEntries, corrOK, corrWarnings := parseCriticVerdictBlock(string(corrStdoutBytes))
		allWarnings = append(allWarnings, corrWarnings...)
		if !corrOK {
			allWarnings = append(allWarnings, "corrective critic attempt output not parseable")
			return reviewCriticPassResult{
				ParseFailed:         true,
				InitialAttemptID:    initialAttemptID,
				CorrectiveAttemptID: correctiveAttemptID,
				Candidates:          len(proposals),
				Warnings:            allWarnings,
			}, nil
		}
		entries = corrEntries
	}

	// Build proposal ID set for validation.
	proposalIDSet := make(map[string]bool, len(proposals))
	for _, p := range proposals {
		proposalIDSet[p.ID] = true
	}

	// Build verdicts map; first verdict per proposal_id wins.
	verdicts := make(map[string]reviewCriticVerdictEntry, len(proposals))
	for _, entry := range entries {
		if !proposalIDSet[entry.ProposalID] {
			allWarnings = append(allWarnings, fmt.Sprintf("critic returned verdict for unknown proposal_id %q — dropped", entry.ProposalID))
			continue
		}
		if _, exists := verdicts[entry.ProposalID]; exists {
			allWarnings = append(allWarnings, fmt.Sprintf("critic returned duplicate verdict for proposal_id %q — first wins", entry.ProposalID))
			continue
		}
		verdicts[entry.ProposalID] = entry
	}

	// Synthetic missing_verdict entries for proposals absent from the critic response.
	for _, p := range proposals {
		if _, exists := verdicts[p.ID]; !exists {
			allWarnings = append(allWarnings, fmt.Sprintf("critic emitted no verdict for proposal_id %q — treated as missing_verdict", p.ID))
			verdicts[p.ID] = reviewCriticVerdictEntry{
				ProposalID:     p.ID,
				Verdict:        reviewCriticVerdictMissingVerdict,
				MissingVerdict: true,
			}
		}
	}

	// Count outcomes.
	confirmed := 0
	disputed := 0
	unresolved := 0
	for _, p := range proposals {
		switch verdicts[p.ID].Verdict {
		case reviewCriticVerdictConfirmed:
			confirmed++
		case reviewCriticVerdictDisputed:
			disputed++
		default:
			unresolved++
		}
	}

	// Write verdict artifact sorted by proposal_id.
	sortedVerdicts := make([]reviewCriticVerdictEntry, 0, len(proposals))
	for _, p := range proposals {
		sortedVerdicts = append(sortedVerdicts, verdicts[p.ID])
	}
	sort.Slice(sortedVerdicts, func(i, j int) bool {
		return sortedVerdicts[i].ProposalID < sortedVerdicts[j].ProposalID
	})
	verdictArtifact := reviewCriticVerdictsArtifact(round)
	if err := writeJSONLines(reviewCriticVerdictsPath(context.RunPaths, round), sortedVerdicts); err != nil {
		return reviewCriticPassResult{}, err
	}

	return reviewCriticPassResult{
		Verdicts:            verdicts,
		InitialAttemptID:    initialAttemptID,
		CorrectiveAttemptID: correctiveAttemptID,
		Candidates:          len(proposals),
		Confirmed:           confirmed,
		Disputed:            disputed,
		Unresolved:          unresolved,
		ParseFailed:         false,
		VerdictArtifactPath: verdictArtifact,
		Warnings:            allWarnings,
	}, nil
}
