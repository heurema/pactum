package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/ledger"
)

const (
	reviewLoopSummarySchema   = "pactum.review_loop.v1"
	reviewLoopSummaryArtifact = "review/loop-summary.json"

	reviewLoopTerminalBudgetExceeded = "budget_exceeded"
)

type reviewLoopOptions struct {
	Reviewer    string
	Agent       string
	MaxRounds   int
	Patience    int
	CleanRounds int
	Timeout     time.Duration
	Yes         bool
	JSONOutput  bool
}

type reviewLoopSettings struct {
	Limits reviewLimits
	Budget reviewLoopBudget
}

type reviewLimits struct {
	MaxRounds   int
	Patience    int
	CleanRounds int
}

type reviewLoopBudget struct {
	Mode      string
	MaxTokens *int64
}

type reviewLoopSummaryDocument struct {
	Schema              string                   `json:"schema"`
	RunID               string                   `json:"run_id"`
	StartedAt           string                   `json:"started_at"`
	FinishedAt          string                   `json:"finished_at"`
	Reviewer            string                   `json:"reviewer,omitempty"`
	Reviewers           []string                 `json:"reviewers,omitempty"`
	Agent               string                   `json:"agent,omitempty"`
	MaxRounds           int                      `json:"max_rounds"`
	StalematePatience   int                      `json:"stalemate_patience"`
	CleanRoundsRequired int                      `json:"clean_rounds_required"`
	TerminalReason      string                   `json:"terminal_reason"`
	Budget              *reviewLoopBudgetSummary `json:"budget,omitempty"`
	Rounds              []reviewLoopRoundSummary `json:"rounds"`
	Artifacts           reviewLoopArtifacts      `json:"artifacts"`
}

type reviewLoopBudgetSummary struct {
	Mode                string   `json:"mode"`
	MaxTokens           int64    `json:"max_tokens"`
	CapturedTotalTokens int64    `json:"captured_total_tokens"`
	Warnings            []string `json:"warnings,omitempty"`
}

type reviewLoopArtifacts struct {
	Summary string `json:"summary"`
}

type reviewLoopRoundSummary struct {
	Round                      int      `json:"round"`
	ReviewerAttemptID          string   `json:"reviewer_attempt_id"`
	ReviewerAttemptIDs         []string `json:"reviewer_attempt_ids,omitempty"`
	ProposalsCreated           int      `json:"proposals_created"`
	ProposalsAccepted          int      `json:"proposals_accepted"`
	OpenFindings               int      `json:"open_findings"`
	OpenBlockingFindings       int      `json:"open_blocking_findings"`
	Warnings                   []string `json:"warnings,omitempty"`
	CleanStreak                int      `json:"clean_streak"`
	UnchangedFingerprintStreak int      `json:"unchanged_fingerprint_streak"`
	WorkingTreeFingerprint     string   `json:"working_tree_fingerprint,omitempty"`
	FixerAttemptID             string   `json:"fixer_attempt_id,omitempty"`
	FixOutcomesResolved        int      `json:"fix_outcomes_resolved,omitempty"`
	FixOutcomesRebutted        int      `json:"fix_outcomes_rebutted,omitempty"`
	FixOutcomesBlocked         int      `json:"fix_outcomes_blocked,omitempty"`
	GateStatus                 string   `json:"gate_status,omitempty"`
	GateReportArtifact         string   `json:"gate_report_artifact,omitempty"`
}

type reviewLoopReviewer struct {
	Agent     agents.AgentDescriptor
	ModelSpec agents.ModelSpec
}

type reviewLoopReviewRoundResult struct {
	Reviewers  []string
	AttemptIDs []string
	Proposals  reviewLoopProposalBatch
}

type reviewLoopProposalBatch struct {
	Created  []reviewProposalRecord
	Warnings []string
}

type synchronizedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *synchronizedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func (a App) ReviewLoop(stdout io.Writer, liveOutput io.Writer, runID string, options reviewLoopOptions) error {
	if !options.Yes {
		return fmt.Errorf("review loop requires --yes because it runs reviewer/fixer agents directly")
	}

	context, ok, err := a.loadReviewContext(io.Discard, runID)
	if err != nil || !ok {
		return err
	}
	if _, err := requireReviewPrepared(context.RunPaths, runID); err != nil {
		return err
	}
	settings, err := a.resolveReviewLoopSettings(context, options)
	if err != nil {
		return err
	}
	reviewers, err := a.resolveReviewLoopReviewers(context, options.Reviewer)
	if err != nil {
		return err
	}
	limits := settings.Limits
	maxRounds := limits.MaxRounds
	reviewerNames := reviewLoopReviewerNames(reviewers)

	startedAt := a.nowUTC()
	summary := reviewLoopSummaryDocument{
		Schema:              reviewLoopSummarySchema,
		RunID:               runID,
		StartedAt:           startedAt.Format(time.RFC3339),
		Reviewer:            strings.Join(reviewerNames, ","),
		MaxRounds:           maxRounds,
		StalematePatience:   limits.Patience,
		CleanRoundsRequired: limits.CleanRounds,
		Rounds:              []reviewLoopRoundSummary{},
		Artifacts: reviewLoopArtifacts{
			Summary: reviewLoopSummaryArtifact,
		},
	}
	if len(reviewerNames) > 1 {
		summary.Reviewers = reviewerNames
	}
	if settings.Budget.MaxTokens != nil {
		summary.Budget = &reviewLoopBudgetSummary{
			Mode:      settings.Budget.Mode,
			MaxTokens: *settings.Budget.MaxTokens,
			Warnings:  []string{},
		}
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_loop_started", Timestamp: startedAt, RunID: runID}); err != nil {
		return err
	}

	var loopErr error
	cleanStreak := 0
	unchangedFingerprintStreak := 0
	for round := 1; round <= maxRounds; round++ {
		if round > 1 {
			stop, err := reviewLoopBudgetExceeded(context.RunPaths, settings.Budget, summary.Budget)
			if err != nil {
				loopErr = err
				break
			}
			if stop {
				summary.TerminalReason = reviewLoopTerminalBudgetExceeded
				break
			}
		}

		reviewerResult, err := a.runReviewLoopReviewRound(context, liveOutput, runID, reviewers, options.Timeout)
		if err != nil {
			loopErr = err
			break
		}
		proposals := reviewerResult.Proposals

		openFindings, rebuttedFindings, err := reviewLoopDedupFindingFingerprints(context.RunPaths)
		if err != nil {
			loopErr = err
			break
		}
		accepted := 0
		duplicates := 0
		for _, proposal := range proposals.Created {
			fingerprint := fingerprintReviewFinding(proposal.findingCore)
			if existingFinding, ok := openFindings[fingerprint]; ok {
				if err := a.recordDuplicateReviewLoopProposal(context, proposal.ID, existingFinding.ID, "matches currently open finding"); err != nil {
					loopErr = err
					break
				}
				upgradedFinding, upgraded, err := a.upgradeDuplicateReviewFindingSeverity(context, existingFinding, proposal)
				if err != nil {
					loopErr = err
					break
				}
				if upgraded {
					openFindings[fingerprint] = upgradedFinding
				}
				duplicates++
				continue
			}
			if existingFinding, ok := rebuttedFindings[fingerprint]; ok {
				if err := a.recordDuplicateReviewLoopProposal(context, proposal.ID, existingFinding.ID, "matches rebutted finding"); err != nil {
					loopErr = err
					break
				}
				duplicates++
				continue
			}
			finding, err := a.acceptReviewLoopProposal(runID, proposal.ID)
			if err != nil {
				loopErr = err
				break
			}
			openFindings[fingerprint] = finding
			accepted++
		}
		if loopErr != nil {
			break
		}
		reviewSummaryAfterAccept, err := reviewLoopReviewSummary(context.RunPaths)
		if err != nil {
			loopErr = err
			break
		}

		roundSummary := reviewLoopRoundSummary{
			Round:                round,
			ReviewerAttemptID:    firstString(reviewerResult.AttemptIDs),
			ProposalsCreated:     len(proposals.Created),
			ProposalsAccepted:    accepted,
			OpenFindings:         reviewSummaryAfterAccept.Open,
			OpenBlockingFindings: reviewSummaryAfterAccept.BlockingOpen,
			Warnings:             append([]string{}, proposals.Warnings...),
		}
		if len(reviewerResult.AttemptIDs) > 1 {
			roundSummary.ReviewerAttemptIDs = append([]string{}, reviewerResult.AttemptIDs...)
		}
		// A round with no accepted or duplicate proposals ends the loop, but only a
		// round that reported NOTHING is a clean pass. If the reviewer reported
		// findings that could not be parsed into proposals (warnings), the code was
		// not actually cleared.
		if accepted == 0 && duplicates == 0 {
			if len(proposals.Created) == 0 && len(proposals.Warnings) == 0 {
				cleanStreak++
			} else {
				cleanStreak = 0
			}
			roundSummary.CleanStreak = cleanStreak
			roundSummary.UnchangedFingerprintStreak = unchangedFingerprintStreak
			fingerprint, err := reviewLoopWorkingTreeFingerprint(context)
			if err != nil {
				summary.Rounds = append(summary.Rounds, roundSummary)
				loopErr = err
				break
			}
			roundSummary.WorkingTreeFingerprint = fingerprint
			summary.Rounds = append(summary.Rounds, roundSummary)
			if len(proposals.Warnings) > 0 {
				summary.TerminalReason = "reviewer_findings_unparsed"
			} else if cleanStreak >= limits.CleanRounds {
				summary.TerminalReason = "clean_round"
			} else if round == maxRounds {
				summary.TerminalReason = "max_rounds"
			} else {
				continue
			}
			break
		}

		cleanStreak = 0
		roundSummary.CleanStreak = cleanStreak

		// Fixer gate: only OPEN BLOCKING findings drive the fixer. A round whose
		// accepted proposals are all advisory (non-blocking) leaves no blocking
		// work, so the loop converges resolved without invoking the fixer.
		if reviewSummaryAfterAccept.BlockingOpen == 0 {
			roundSummary.UnchangedFingerprintStreak = unchangedFingerprintStreak
			fingerprint, err := reviewLoopWorkingTreeFingerprint(context)
			if err != nil {
				summary.Rounds = append(summary.Rounds, roundSummary)
				loopErr = err
				break
			}
			roundSummary.WorkingTreeFingerprint = fingerprint
			summary.Rounds = append(summary.Rounds, roundSummary)
			summary.TerminalReason = "resolved"
			break
		}

		fingerprintBefore, err := reviewLoopWorkingTreeFingerprint(context)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			loopErr = err
			break
		}

		fixResult, err := a.runReviewLoopFixRound(liveOutput, runID, options.Agent, options.Timeout)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			loopErr = err
			break
		}
		if summary.Agent == "" {
			summary.Agent = fixResult.Fixer
		}
		roundSummary.FixerAttemptID = fixResult.AttemptID
		fixOutcomes, err := a.applyReviewLoopFixOutcomes(runID, fixResult.AttemptID)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			loopErr = err
			break
		}
		roundSummary.FixOutcomesResolved = fixOutcomes.Fixed
		roundSummary.FixOutcomesRebutted = fixOutcomes.Rebutted
		roundSummary.FixOutcomesBlocked = fixOutcomes.Blocked
		roundSummary.Warnings = append(roundSummary.Warnings, fixOutcomes.Warnings...)
		reviewSummaryAfterFix, err := reviewLoopReviewSummary(context.RunPaths)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			loopErr = err
			break
		}
		roundSummary.OpenFindings = reviewSummaryAfterFix.Open
		roundSummary.OpenBlockingFindings = reviewSummaryAfterFix.BlockingOpen
		fingerprintAfter, err := reviewLoopWorkingTreeFingerprint(context)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			loopErr = err
			break
		}
		roundSummary.WorkingTreeFingerprint = fingerprintAfter
		if fingerprintAfter == fingerprintBefore {
			unchangedFingerprintStreak++
		} else {
			unchangedFingerprintStreak = 0
		}
		roundSummary.UnchangedFingerprintStreak = unchangedFingerprintStreak

		gateReport, err := a.runReviewLoopGate(runID)
		if err != nil {
			var gateErr gateProcessError
			if errors.As(err, &gateErr) {
				roundSummary.GateStatus = gateReport.Status
				roundSummary.GateReportArtifact = gateReportArtifact
				summary.Rounds = append(summary.Rounds, roundSummary)
				summary.TerminalReason = "gate_failed"
				break
			}
			summary.Rounds = append(summary.Rounds, roundSummary)
			loopErr = err
			break
		}
		roundSummary.GateStatus = gateReport.Status
		roundSummary.GateReportArtifact = gateReportArtifact
		summary.Rounds = append(summary.Rounds, roundSummary)

		// Primary success terminal: no open blocking findings remain after the
		// fixer applied its outcomes — the same condition that makes a review
		// approvable. Non-blocking findings may still be open (advisory).
		if reviewSummaryAfterFix.BlockingOpen == 0 {
			summary.TerminalReason = "resolved"
			break
		}
		if unchangedFingerprintStreak >= limits.Patience {
			summary.TerminalReason = "stalemate"
			break
		}
		if round == maxRounds {
			summary.TerminalReason = "max_rounds"
			break
		}
	}

	// Always finalize: write the summary artifact and emit the finished event even
	// when a round errored, so the run never leaves a dangling started event.
	if summary.TerminalReason == "" {
		if loopErr != nil {
			summary.TerminalReason = "error"
		} else {
			summary.TerminalReason = "max_rounds"
		}
	}
	if summary.Budget != nil {
		total, err := reviewLoopCapturedTokenTotal(context.RunPaths)
		if err != nil && loopErr == nil {
			loopErr = err
		}
		summary.Budget.CapturedTotalTokens = total
	}
	finishedAt := a.nowUTC()
	summary.FinishedAt = finishedAt.Format(time.RFC3339)
	if err := writeJSON(context.RunPaths.ReviewLoopSummaryJSON, summary); err != nil && loopErr == nil {
		loopErr = err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_loop_finished", Timestamp: finishedAt, RunID: runID}); err != nil && loopErr == nil {
		loopErr = err
	}
	if loopErr != nil {
		return loopErr
	}

	if options.JSONOutput {
		return writeJSONResponse(stdout, summary)
	}
	writeReviewLoopSummary(stdout, summary)
	return nil
}

func (a App) resolveReviewLoopSettings(context reviewContext, options reviewLoopOptions) (reviewLoopSettings, error) {
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return reviewLoopSettings{}, err
	}
	defaults := defaultConfigFile().Review
	maxRounds, err := resolveReviewLoopLimit("max rounds", options.MaxRounds, config.Review.MaxRounds, defaults.MaxRounds)
	if err != nil {
		return reviewLoopSettings{}, err
	}
	patience, err := resolveReviewLoopLimit("patience", options.Patience, config.Review.Patience, defaults.Patience)
	if err != nil {
		return reviewLoopSettings{}, err
	}
	cleanRounds, err := resolveReviewLoopLimit("clean rounds", options.CleanRounds, config.Review.CleanRounds, defaults.CleanRounds)
	if err != nil {
		return reviewLoopSettings{}, err
	}
	return reviewLoopSettings{
		Limits: reviewLimits{
			MaxRounds:   maxRounds,
			Patience:    patience,
			CleanRounds: cleanRounds,
		},
		Budget: reviewLoopBudget{
			Mode:      config.Review.Budget.Mode,
			MaxTokens: reviewLoopBudgetMaxTokens(config.Review.Budget.MaxTokens),
		},
	}, nil
}

// reviewLoopBudgetMaxTokens treats a non-positive max_tokens as disabled (no
// budget), consistent with pactum's "0 = off" convention, so a 0/negative value
// can never stop the loop after a single round.
func reviewLoopBudgetMaxTokens(value *int64) *int64 {
	if value == nil || *value <= 0 {
		return nil
	}
	return value
}

func resolveReviewLoopLimit(name string, override int, configured int, fallback int) (int, error) {
	if override < 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	if override > 0 {
		return override, nil
	}
	value := configured
	if value <= 0 {
		value = fallback
	}
	if value <= 0 {
		return 0, fmt.Errorf("review %s must be positive", name)
	}
	return value, nil
}

func reviewLoopBudgetExceeded(runPaths contractRunPathSet, budget reviewLoopBudget, summary *reviewLoopBudgetSummary) (bool, error) {
	if budget.MaxTokens == nil {
		return false, nil
	}
	total, err := reviewLoopCapturedTokenTotal(runPaths)
	if err != nil {
		return false, err
	}
	if summary != nil {
		summary.CapturedTotalTokens = total
	}
	if total < *budget.MaxTokens {
		return false, nil
	}
	if budget.Mode == budgetModeWarn {
		if summary != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("budget max_tokens reached: captured_total_tokens=%d max_tokens=%d mode=warn", total, *budget.MaxTokens))
		}
		return false, nil
	}
	return true, nil
}

func reviewLoopCapturedTokenTotal(runPaths contractRunPathSet) (int64, error) {
	records, err := readUsageRecords(runPaths.UsageJSONL)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, record := range records {
		if record.Captured {
			total += record.TotalTokens
		}
	}
	return total, nil
}

func (a App) resolveReviewLoopReviewers(context reviewContext, reviewerName string) ([]reviewLoopReviewer, error) {
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return nil, err
	}
	var roster []agentModelEntry
	if explicit := strings.TrimSpace(reviewerName); explicit != "" {
		// An explicit --reviewer overrides the roster; it takes pins from its
		// review.panel entry when present and runs unpinned otherwise.
		entry, ok := findAgentModelEntry(config.Review.Panel, explicit)
		if !ok {
			entry = agentModelEntry{Agent: explicit}
		}
		roster = []agentModelEntry{entry}
	} else if len(config.Review.Panel) > 0 {
		roster = config.Review.Panel
	} else {
		// Empty panel: cross-model default — a single reviewer, the agent other
		// than the run's executor.
		roster = []agentModelEntry{{Agent: resolveReviewerNameForReview(context, "")}}
	}

	reviewers := make([]reviewLoopReviewer, 0, len(roster))
	for _, entry := range roster {
		reviewer, err := a.agentRegistry().ResolveReviewer(entry.Agent)
		if err != nil {
			return nil, err
		}
		modelSpec := entry.modelSpec()
		reviewer, err = agents.ApplyModelSpec(reviewer, modelSpec)
		if err != nil {
			return nil, err
		}
		if reviewer.Input != agents.InputPromptFile {
			return nil, fmt.Errorf("unsupported agent input mode: %s", reviewer.Input)
		}
		reviewers = append(reviewers, reviewLoopReviewer{Agent: reviewer, ModelSpec: modelSpec})
	}
	return reviewers, nil
}

func reviewLoopReviewerNames(reviewers []reviewLoopReviewer) []string {
	names := make([]string, 0, len(reviewers))
	for _, reviewer := range reviewers {
		names = append(names, reviewer.Agent.Name)
	}
	return names
}

func (a App) runReviewLoopReviewRound(context reviewContext, liveOutput io.Writer, runID string, reviewers []reviewLoopReviewer, timeout time.Duration) (reviewLoopReviewRoundResult, error) {
	if len(reviewers) == 0 {
		return reviewLoopReviewRoundResult{}, fmt.Errorf("review loop requires at least one reviewer")
	}
	prep, err := a.prepareReviewerForAgent(context, reviewers[0].Agent, reviewers[0].ModelSpec, "run reviewer")
	if err != nil {
		return reviewLoopReviewRoundResult{}, err
	}
	if err := writeReviewerPromptAndContext(prep); err != nil {
		return reviewLoopReviewRoundResult{}, err
	}

	results := make([]reviewerResultDocument, len(reviewers))
	errs := make([]error, len(reviewers))
	var wg sync.WaitGroup
	var sharedLive io.Writer = liveOutput
	if liveOutput != nil && len(reviewers) > 1 {
		sharedLive = &synchronizedWriter{w: liveOutput}
	}
	for index, reviewer := range reviewers {
		wg.Add(1)
		go func(index int, reviewer reviewLoopReviewer) {
			defer wg.Done()
			results[index], errs[index] = a.runReviewLoopReviewerWithAgent(sharedLive, runID, prep, reviewer, timeout)
		}(index, reviewer)
	}
	wg.Wait()
	for index, err := range errs {
		if err != nil {
			return reviewLoopReviewRoundResult{}, fmt.Errorf("reviewer %s: %w", reviewers[index].Agent.Name, err)
		}
	}

	batch := reviewLoopProposalBatch{
		Created:  []reviewProposalRecord{},
		Warnings: []string{},
	}
	attemptIDs := make([]string, 0, len(results))
	for _, result := range results {
		attemptIDs = append(attemptIDs, result.AttemptID)
		proposals, err := a.runReviewLoopProposeFindings(runID, result.AttemptID)
		if err != nil {
			return reviewLoopReviewRoundResult{}, err
		}
		batch.Created = append(batch.Created, proposals.Created...)
		batch.Warnings = append(batch.Warnings, proposals.Warnings...)
	}
	return reviewLoopReviewRoundResult{
		Reviewers:  reviewLoopReviewerNames(reviewers),
		AttemptIDs: attemptIDs,
		Proposals:  batch,
	}, nil
}

func (a App) runReviewLoopReviewerWithAgent(liveOutput io.Writer, runID string, prep reviewerDryRunPreparation, reviewer reviewLoopReviewer, timeout time.Duration) (reviewerResultDocument, error) {
	var stdout bytes.Buffer
	if err := a.runReviewerAttempt(&stdout, liveOutput, runID, prep, reviewer, timeout, true, true, false); err != nil {
		return reviewerResultDocument{}, err
	}
	var result reviewerResultDocument
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return reviewerResultDocument{}, err
	}
	return result, nil
}

func (a App) runReviewerAttempt(stdout io.Writer, liveOutput io.Writer, runID string, prep reviewerDryRunPreparation, reviewer reviewLoopReviewer, timeout time.Duration, confirm bool, jsonOutput bool, writeDryRunArtifacts bool) error {
	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[reviewerDryRunDocument, reviewerRequestDocument, reviewerResultDocument, struct{}]{
		Stdout:          stdout,
		LiveOutput:      liveOutput,
		JSONOutput:      jsonOutput,
		Confirm:         confirm,
		CancelMessage:   "review cancelled",
		Root:            prep.Context.Root,
		EventsJSONL:     prep.Context.Paths.EventsJSONL,
		RunID:           runID,
		Stage:           "review",
		AttemptsDir:     prep.Context.RunPaths.ReviewAttemptsDir,
		AttemptIDPrefix: "reviewer_attempt",
		LastResultJSON:  prep.Context.RunPaths.ReviewLastResultJSON,
		Agent:           reviewer.Agent,
		Model:           reviewer.ModelSpec,
		PromptRepoPath:  reviewerPromptRepoPath(runID),
		ArtifactDir:     reviewerAttemptsArtifact,
		Timeout:         timeout,
		ReadOnly:        true,
		StartedEvent:    "reviewer_attempt_started",
		FinishedEvent:   "reviewer_attempt_finished",
		ExitKind:        "reviewer",
		TimeoutMessage: func(timeout time.Duration) string {
			return fmt.Sprintf("reviewer process produced no output for %s", timeout)
		},
		Prepare: func(createdAt string) (reviewerDryRunDocument, error) {
			if writeDryRunArtifacts {
				return ensureReviewerDryRunArtifacts(prep, createdAt)
			}
			return buildReviewerDryRunDocument(runID, createdAt, reviewer.Agent)
		},
		BuildRequest: func(context agentAttemptContext[reviewerDryRunDocument]) (reviewerRequestDocument, error) {
			return reviewerRequestDocument{
				Schema:    reviewerRequestSchema,
				RunID:     runID,
				AttemptID: context.AttemptID,
				CreatedAt: context.CreatedAt,
				Reviewer:  agentDescriptorDocument(reviewer.Agent),
				Artifacts: context.Prepared.Artifacts,
				WouldRun:  context.Prepared.WouldRun,
			}, nil
		},
		BuildResult: func(context agentAttemptContext[reviewerDryRunDocument], runResult agents.RunResult) reviewerResultDocument {
			return reviewerResultDocument{
				Schema:        reviewerResultSchema,
				RunID:         runID,
				AttemptID:     context.AttemptID,
				Reviewer:      reviewer.Agent.Name,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result reviewerResultDocument) processResult {
			return result.processResult
		},
		RenderRunOnly: func(stdout io.Writer, request reviewerRequestDocument, result reviewerResultDocument) {
			writeReviewRun(stdout, request, result, reviewer.ModelSpec)
		},
	})
}

func (a App) runReviewLoopProposeFindings(runID string, reviewerAttemptID string) (reviewProposeFindingsResponse, error) {
	var stdout bytes.Buffer
	if err := a.ReviewProposeFindings(&stdout, runID, reviewerAttemptID, true); err != nil {
		return reviewProposeFindingsResponse{}, err
	}
	var response reviewProposeFindingsResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return reviewProposeFindingsResponse{}, err
	}
	return response, nil
}

func (a App) acceptReviewLoopProposal(runID string, proposalID string) (reviewFindingRecord, error) {
	var stdout bytes.Buffer
	if err := a.ReviewAcceptProposal(&stdout, runID, proposalID, true); err != nil {
		return reviewFindingRecord{}, err
	}
	var response reviewAcceptProposalResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return reviewFindingRecord{}, err
	}
	return response.Finding, nil
}

func (a App) recordDuplicateReviewLoopProposal(context reviewContext, proposalID string, findingID string, reason string) error {
	_, decisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return err
	}
	if isProposalDecided(proposalID, decisions) {
		return fmt.Errorf("review proposal already decided: %s", proposalID)
	}

	now := a.nowUTC()
	decision := reviewProposalDecisionRecord{
		Schema:     reviewProposalDecisionSchema,
		ID:         nextReviewID("pd", len(decisions)+1),
		RunID:      context.State.RunID,
		ProposalID: proposalID,
		Decision:   "duplicate",
		FindingID:  findingID,
		Reason:     reason,
		CreatedAt:  now.Format(time.RFC3339),
		Source:     "review_loop",
	}
	if err := appendJSONLine(context.RunPaths.ReviewProposalDecisionsJSONL, decision); err != nil {
		return err
	}
	return ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_proposal_duplicate", Timestamp: now, RunID: context.State.RunID})
}

func (a App) upgradeDuplicateReviewFindingSeverity(context reviewContext, existing reviewFindingRecord, proposal reviewProposalRecord) (reviewFindingRecord, bool, error) {
	if reviewSeverityRank(proposal.Severity) <= reviewSeverityRank(existing.Severity) {
		return existing, false, nil
	}
	findings, _, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return reviewFindingRecord{}, false, err
	}
	updated := existing
	found := false
	for index := range findings {
		if findings[index].ID != existing.ID {
			continue
		}
		findings[index].Severity = proposal.Severity
		updated = findings[index]
		found = true
		break
	}
	if !found {
		return reviewFindingRecord{}, false, fmt.Errorf("review finding not found for severity upgrade: %s", existing.ID)
	}
	if err := writeJSONLines(context.RunPaths.ReviewFindingsJSONL, findings); err != nil {
		return reviewFindingRecord{}, false, err
	}
	now := a.nowUTC()
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_finding_severity_upgraded", Timestamp: now, RunID: context.State.RunID}); err != nil {
		return reviewFindingRecord{}, false, err
	}
	return updated, true, nil
}

func reviewSeverityRank(severity string) int {
	switch severity {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

func (a App) runReviewLoopFixRound(liveOutput io.Writer, runID string, agent string, timeout time.Duration) (reviewFixResultDocument, error) {
	var stdout bytes.Buffer
	if err := a.ReviewFix(&stdout, liveOutput, runID, agent, timeout, true, true); err != nil {
		return reviewFixResultDocument{}, err
	}
	var result reviewFixResultDocument
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return reviewFixResultDocument{}, err
	}
	return result, nil
}

func (a App) applyReviewLoopFixOutcomes(runID string, fixerAttemptID string) (reviewApplyFixOutcomesResponse, error) {
	var stdout bytes.Buffer
	if err := a.ReviewApplyFixOutcomes(&stdout, runID, fixerAttemptID, true); err != nil {
		return reviewApplyFixOutcomesResponse{}, err
	}
	var response reviewApplyFixOutcomesResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return reviewApplyFixOutcomesResponse{}, err
	}
	return response, nil
}

func (a App) runReviewLoopGate(runID string) (gateReportDocument, error) {
	var stdout bytes.Buffer
	if err := a.GateRun(&stdout, runID, true, true); err != nil {
		var gateErr gateProcessError
		if errors.As(err, &gateErr) {
			var report gateReportDocument
			if unmarshalErr := json.Unmarshal(stdout.Bytes(), &report); unmarshalErr != nil {
				return gateReportDocument{}, unmarshalErr
			}
			return report, err
		}
		return gateReportDocument{}, err
	}
	var report gateReportDocument
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		return gateReportDocument{}, err
	}
	return report, nil
}

// reviewLoopReviewSummary recomputes the live review summary (open and open
// blocking finding counts) from the durable findings/resolutions. The loop reads
// Open for the round summary and BlockingOpen for both the convergence gate and
// the fixer gate — BlockingOpen is the same condition that makes a review
// approvable.
func reviewLoopReviewSummary(runPaths contractRunPathSet) (reviewSummary, error) {
	findings, resolutions, err := readReviewRecords(runPaths)
	if err != nil {
		return reviewSummary{}, err
	}
	return summarizeReview(findings, resolutions), nil
}

func reviewLoopDedupFindingFingerprints(runPaths contractRunPathSet) (map[reviewFindingFingerprint]reviewFindingRecord, map[reviewFindingFingerprint]reviewFindingRecord, error) {
	findings, resolutions, err := readReviewRecords(runPaths)
	if err != nil {
		return nil, nil, err
	}
	resolved := latestReviewResolutions(resolutions)
	open := make(map[reviewFindingFingerprint]reviewFindingRecord)
	rebutted := make(map[reviewFindingFingerprint]reviewFindingRecord)
	for _, finding := range findings {
		if resolution, ok := resolved[finding.ID]; ok {
			if resolution.Outcome == "rebutted" {
				fingerprint := fingerprintReviewFinding(finding.findingCore)
				if _, exists := rebutted[fingerprint]; !exists {
					rebutted[fingerprint] = finding
				}
			}
			continue
		}
		// Autonomous-loop dedup is deliberately exact: only the stored
		// (file, line, message) tuple matches. Reworded messages and line drift
		// remain separate findings until a later semantic dedup design exists.
		fingerprint := fingerprintReviewFinding(finding.findingCore)
		if _, exists := open[fingerprint]; !exists {
			open[fingerprint] = finding
		}
	}
	return open, rebutted, nil
}

func reviewLoopWorkingTreeFingerprint(context reviewContext) (string, error) {
	changes := buildGateChangeReport(context.Root, context.Paths)
	hasher := sha256.New()
	fmt.Fprintf(hasher, "head\x00%s\x00", reviewLoopGitHead(context.Root))
	fmt.Fprintf(hasher, "status\x00%s\x00", changes.Status)
	for _, reason := range changes.Reasons {
		fmt.Fprintf(hasher, "reason\x00%s\x00", reason)
	}
	for _, path := range changes.ChangedFiles {
		if err := reviewLoopHashFingerprintFile(hasher, context.Root, "changed", path); err != nil {
			return "", err
		}
	}
	for _, path := range changes.NewFiles {
		if err := reviewLoopHashFingerprintFile(hasher, context.Root, "new", path); err != nil {
			return "", err
		}
	}
	for _, path := range changes.MissingFiles {
		fmt.Fprintf(hasher, "missing\x00%s\x00", path)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func reviewLoopHashFingerprintFile(hasher io.Writer, root string, kind string, repoPath string) error {
	fullPath := filepath.Join(root, filepath.FromSlash(repoPath))
	if !filesystemRegularFile(fullPath) {
		fmt.Fprintf(hasher, "%s\x00%s\x00missing\x00", kind, repoPath)
		return nil
	}
	hash, err := fileSHA256(fullPath)
	if err != nil {
		return fmt.Errorf("fingerprint %s file %s: %w", kind, repoPath, err)
	}
	fmt.Fprintf(hasher, "%s\x00%s\x00%s\x00", kind, repoPath, hash)
	return nil
}

func reviewLoopGitHead(root string) string {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--verify", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unavailable"
	}
	head := strings.TrimSpace(string(output))
	if head == "" {
		return "unavailable"
	}
	return head
}

func writeReviewLoopSummary(stdout io.Writer, summary reviewLoopSummaryDocument) {
	fmt.Fprintln(stdout, "Review loop finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", summary.RunID)
	fmt.Fprintf(stdout, "  terminal reason: %s\n", summary.TerminalReason)
	fmt.Fprintf(stdout, "  rounds: %d/%d\n", len(summary.Rounds), summary.MaxRounds)
	fmt.Fprintf(stdout, "  clean rounds: %d\n", summary.CleanRoundsRequired)
	fmt.Fprintf(stdout, "  stalemate patience: %d\n", summary.StalematePatience)
	if summary.Budget != nil {
		fmt.Fprintf(stdout, "  budget mode: %s\n", summary.Budget.Mode)
		fmt.Fprintf(stdout, "  budget max tokens: %d\n", summary.Budget.MaxTokens)
		fmt.Fprintf(stdout, "  budget captured tokens: %d\n", summary.Budget.CapturedTotalTokens)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Round results:")
	for _, round := range summary.Rounds {
		reviewerAttempts := round.ReviewerAttemptID
		if len(round.ReviewerAttemptIDs) > 0 {
			reviewerAttempts = strings.Join(round.ReviewerAttemptIDs, ",")
		}
		fmt.Fprintf(stdout, "  - round %d: open findings %d (blocking %d), proposals accepted %d, reviewer %s", round.Round, round.OpenFindings, round.OpenBlockingFindings, round.ProposalsAccepted, reviewerAttempts)
		if round.FixerAttemptID != "" {
			fmt.Fprintf(stdout, ", fixer %s", round.FixerAttemptID)
		}
		if round.FixOutcomesResolved > 0 || round.FixOutcomesRebutted > 0 || round.FixOutcomesBlocked > 0 {
			fmt.Fprintf(stdout, ", outcomes resolved=%d rebutted=%d blocked=%d", round.FixOutcomesResolved, round.FixOutcomesRebutted, round.FixOutcomesBlocked)
		}
		if round.GateStatus != "" {
			fmt.Fprintf(stdout, ", gate %s", round.GateStatus)
		}
		if round.CleanStreak > 0 {
			fmt.Fprintf(stdout, ", clean streak %d", round.CleanStreak)
		}
		if round.FixerAttemptID != "" {
			fmt.Fprintf(stdout, ", unchanged streak %d", round.UnchangedFingerprintStreak)
		}
		fmt.Fprintln(stdout)
	}
	if summary.Budget != nil && len(summary.Budget.Warnings) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Budget warnings:")
		for _, warning := range summary.Budget.Warnings {
			fmt.Fprintf(stdout, "  - %s\n", warning)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  summary: %s\n", runArtifactRepoRel(summary.RunID, reviewLoopSummaryArtifact))
	if summary.TerminalReason == "gate_failed" {
		for index := len(summary.Rounds) - 1; index >= 0; index-- {
			if summary.Rounds[index].GateReportArtifact != "" {
				fmt.Fprintf(stdout, "  gate report: %s\n", runArtifactRepoRel(summary.RunID, summary.Rounds[index].GateReportArtifact))
				break
			}
		}
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
