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

	reviewLoopTerminalFindingsOpen = "findings_open"
)

type reviewRunOptions struct {
	Reviewer    string
	Agent       string
	MaxRounds   int
	Patience    int
	CleanRounds int
	// NoFix never invokes the fixer: the first round that leaves open blocking
	// findings ends the run as findings_open instead of starting a fix round.
	NoFix      bool
	Timeout    time.Duration
	JSONOutput bool
}

type reviewLimits struct {
	MaxRounds   int
	Patience    int
	CleanRounds int
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
	Rounds              []reviewLoopRoundSummary `json:"rounds"`
	Artifacts           reviewLoopArtifacts      `json:"artifacts"`
}

type reviewLoopArtifacts struct {
	Summary string `json:"summary"`
}

type reviewLoopRoundSummary struct {
	Round                      int                    `json:"round"`
	ReviewerAttemptID          string                 `json:"reviewer_attempt_id"`
	ReviewerAttemptIDs         []string               `json:"reviewer_attempt_ids,omitempty"`
	ReviewerAttempts           []reviewLoopAttemptRef `json:"reviewer_attempts,omitempty"`
	ProposalsCreated           int                    `json:"proposals_created"`
	ProposalsAccepted          int                    `json:"proposals_accepted"`
	OpenFindings               int                    `json:"open_findings"`
	OpenBlockingFindings       int                    `json:"open_blocking_findings"`
	Warnings                   []string               `json:"warnings,omitempty"`
	CleanStreak                int                    `json:"clean_streak"`
	UnchangedFingerprintStreak int                    `json:"unchanged_fingerprint_streak"`
	WorkingTreeFingerprint     string                 `json:"working_tree_fingerprint,omitempty"`
	FixerAttemptID             string                 `json:"fixer_attempt_id,omitempty"`
	FixOutcomesResolved        int                    `json:"fix_outcomes_resolved,omitempty"`
	FixOutcomesRebutted        int                    `json:"fix_outcomes_rebutted,omitempty"`
	FixOutcomesBlocked         int                    `json:"fix_outcomes_blocked,omitempty"`
	GateStatus                 string                 `json:"gate_status,omitempty"`
	GateReportArtifact         string                 `json:"gate_report_artifact,omitempty"`
}

// reviewLoopReviewer is one resolved panel member: the registry name it was
// invoked under, the underlying built-in's read-only descriptor with the
// entry's pins applied, and the pin spec. Two members may share an underlying
// built-in — they run as separate panel members under their own names.
type reviewLoopReviewer struct {
	Name      string
	Agent     agents.AgentDescriptor
	ModelSpec agents.ModelSpec
}

type reviewLoopReviewRoundResult struct {
	Reviewers  []string
	AttemptIDs []string
	Attempts   []reviewLoopAttemptRef
	Proposals  reviewLoopProposalBatch
}

// reviewLoopAttemptRef ties one lens attempt to the panel member (registry
// name) and lens it ran under, so the round summary surfaces the fan-out.
type reviewLoopAttemptRef struct {
	AttemptID string `json:"attempt_id"`
	Reviewer  string `json:"reviewer"`
	Lens      string `json:"lens"`
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

// ReviewRun drives reviewer/fixer rounds until convergence: each round fans the
// reviewer panel out across the fixed lenses, accepts parsed proposals into
// review findings, and — while open blocking findings remain — runs the fixer,
// applies its outcomes, and re-gates. The review record is scaffolded
// implicitly when the gate report exists.
func (a App) ReviewRun(stdout io.Writer, liveOutput io.Writer, runID string, options reviewRunOptions) error {
	context, ok, err := a.loadReviewContext(io.Discard, runID)
	if err != nil || !ok {
		return err
	}
	if _, err := a.ensureReviewRecord(context, "run review"); err != nil {
		return err
	}
	options.Timeout, err = resolveIdleTimeout(context.Paths.Config, options.Timeout)
	if err != nil {
		return err
	}
	limits, err := a.resolveReviewLoopLimits(context, options)
	if err != nil {
		return err
	}
	reviewers, err := a.resolveReviewLoopReviewers(context, options.Reviewer)
	if err != nil {
		return err
	}
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
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_loop_started", Timestamp: startedAt, RunID: runID}); err != nil {
		return err
	}

	var loopErr error
	cleanStreak := 0
	unchangedFingerprintStreak := 0
	for round := 1; round <= maxRounds; round++ {
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
			finding, err := a.acceptReviewLoopProposal(context, proposal.ID)
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
		roundSummary.ReviewerAttempts = append([]reviewLoopAttemptRef{}, reviewerResult.Attempts...)
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
			fingerprint, err := a.reviewLoopWorkingTreeFingerprint(context)
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
			fingerprint, err := a.reviewLoopWorkingTreeFingerprint(context)
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

		// With --no-fix nothing can change the tree, so further reviewer-only
		// rounds would only churn: stop at the first round that leaves open
		// blocking findings and hand the review to the human.
		if options.NoFix {
			roundSummary.UnchangedFingerprintStreak = unchangedFingerprintStreak
			fingerprint, err := a.reviewLoopWorkingTreeFingerprint(context)
			if err != nil {
				summary.Rounds = append(summary.Rounds, roundSummary)
				loopErr = err
				break
			}
			roundSummary.WorkingTreeFingerprint = fingerprint
			summary.Rounds = append(summary.Rounds, roundSummary)
			summary.TerminalReason = reviewLoopTerminalFindingsOpen
			break
		}

		fingerprintBefore, err := a.reviewLoopWorkingTreeFingerprint(context)
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
		fingerprintAfter, err := a.reviewLoopWorkingTreeFingerprint(context)
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
		return writeJSONResponse(stdout, reviewLoopResponse{reviewLoopSummaryDocument: summary, Next: nextCommandsForRun(context.Paths, runID)})
	}
	writeReviewLoopSummary(stdout, summary)
	return nil
}

// reviewLoopResponse is the loop summary plus the next affordance; the
// summary artifact on disk stays unchanged.
type reviewLoopResponse struct {
	reviewLoopSummaryDocument
	Next []string `json:"next"`
}

func (a App) resolveReviewLoopLimits(context reviewContext, options reviewRunOptions) (reviewLimits, error) {
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return reviewLimits{}, err
	}
	defaults := defaultConfigFile().Review
	maxRounds, err := resolveReviewLoopLimit("max rounds", options.MaxRounds, config.Review.MaxRounds, defaults.MaxRounds)
	if err != nil {
		return reviewLimits{}, err
	}
	patience, err := resolveReviewLoopLimit("patience", options.Patience, config.Review.Patience, defaults.Patience)
	if err != nil {
		return reviewLimits{}, err
	}
	cleanRounds, err := resolveReviewLoopLimit("clean rounds", options.CleanRounds, config.Review.CleanRounds, defaults.CleanRounds)
	if err != nil {
		return reviewLimits{}, err
	}
	return reviewLimits{
		MaxRounds:   maxRounds,
		Patience:    patience,
		CleanRounds: cleanRounds,
	}, nil
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

func (a App) resolveReviewLoopReviewers(context reviewContext, reviewerName string) ([]reviewLoopReviewer, error) {
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return nil, err
	}
	var roster []agentRegistryEntry
	if explicit := strings.TrimSpace(reviewerName); explicit != "" {
		// An explicit --reviewer overrides the panel: a single registry name
		// running with its entry's pins.
		entry, err := findRegistryEntry(config, explicit)
		if err != nil {
			return nil, err
		}
		roster = []agentRegistryEntry{entry}
	} else if len(config.Review.Panel) > 0 {
		for _, name := range config.Review.Panel {
			entry, err := findRegistryEntry(config, name)
			if err != nil {
				return nil, err
			}
			roster = append(roster, entry)
		}
	} else {
		// Empty panel: cross-model default — a single reviewer whose underlying
		// agent differs from the run executor's when the registry has one.
		entry, err := resolveReviewerEntry(config, context, "")
		if err != nil {
			return nil, err
		}
		roster = []agentRegistryEntry{entry}
	}

	reviewers := make([]reviewLoopReviewer, 0, len(roster))
	for _, entry := range roster {
		resolved, err := a.resolveAgentForRole(entry, agentRoleReviewer)
		if err != nil {
			return nil, err
		}
		reviewers = append(reviewers, reviewLoopReviewer{Name: resolved.Name, Agent: resolved.Agent, ModelSpec: resolved.ModelSpec})
	}
	return reviewers, nil
}

func reviewLoopReviewerNames(reviewers []reviewLoopReviewer) []string {
	names := make([]string, 0, len(reviewers))
	for _, reviewer := range reviewers {
		names = append(names, reviewer.Name)
	}
	return names
}

func (a App) runReviewLoopReviewRound(context reviewContext, liveOutput io.Writer, runID string, reviewers []reviewLoopReviewer, timeout time.Duration) (reviewLoopReviewRoundResult, error) {
	if len(reviewers) == 0 {
		return reviewLoopReviewRoundResult{}, fmt.Errorf("review run requires at least one reviewer")
	}
	prep, err := a.prepareReviewerForAgent(context, reviewers[0].Agent, reviewers[0].ModelSpec, "run reviewer")
	if err != nil {
		return reviewLoopReviewRoundResult{}, err
	}
	if err := writeReviewerPromptsAndContext(prep, reviewLoopReviewerNames(reviewers)); err != nil {
		return reviewLoopReviewRoundResult{}, err
	}

	memberResults := make([][]reviewerResultDocument, len(reviewers))
	errs := make([]error, len(reviewers))
	var wg sync.WaitGroup
	var sharedLive io.Writer = liveOutput
	if liveOutput != nil {
		sharedLive = &synchronizedWriter{w: liveOutput}
	}
	for index, reviewer := range reviewers {
		wg.Add(1)
		go func(index int, reviewer reviewLoopReviewer) {
			defer wg.Done()
			memberResults[index], errs[index] = a.runReviewerLensAttempts(sharedLive, runID, prep, reviewer, timeout)
		}(index, reviewer)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return reviewLoopReviewRoundResult{}, err
		}
	}

	batch := reviewLoopProposalBatch{
		Created:  []reviewProposalRecord{},
		Warnings: []string{},
	}
	attemptIDs := make([]string, 0, len(reviewers)*len(reviewLenses))
	attempts := make([]reviewLoopAttemptRef, 0, len(reviewers)*len(reviewLenses))
	for index, reviewer := range reviewers {
		for _, result := range memberResults[index] {
			attemptIDs = append(attemptIDs, result.AttemptID)
			attempts = append(attempts, reviewLoopAttemptRef{
				AttemptID: result.AttemptID,
				Reviewer:  reviewer.Name,
				Lens:      result.Lens,
			})
			proposals, err := a.runReviewLoopProposeFindings(runID, result.AttemptID)
			if err != nil {
				return reviewLoopReviewRoundResult{}, err
			}
			batch.Created = append(batch.Created, proposals.Created...)
			batch.Warnings = append(batch.Warnings, proposals.Warnings...)
		}
	}
	return reviewLoopReviewRoundResult{
		Reviewers:  reviewLoopReviewerNames(reviewers),
		AttemptIDs: attemptIDs,
		Attempts:   attempts,
		Proposals:  batch,
	}, nil
}

// runReviewerLensAttempts spawns the fixed lens fan-out for one resolved
// reviewer: five concurrent attempts, one per lens, each reading its own
// per-member, per-lens prompt artifact (written before the launch). Results
// keep the lens order even when an attempt fails, so callers can render
// whatever completed.
func (a App) runReviewerLensAttempts(liveOutput io.Writer, runID string, prep reviewerDryRunPreparation, reviewer reviewLoopReviewer, timeout time.Duration) ([]reviewerResultDocument, error) {
	results := make([]reviewerResultDocument, len(reviewLenses))
	errs := make([]error, len(reviewLenses))
	var wg sync.WaitGroup
	var sharedLive io.Writer = liveOutput
	if liveOutput != nil {
		sharedLive = &synchronizedWriter{w: liveOutput}
	}
	for index, lens := range reviewLenses {
		wg.Add(1)
		go func(index int, lens reviewLens) {
			defer wg.Done()
			results[index], errs[index] = a.runReviewLoopReviewerWithAgent(sharedLive, runID, prep, reviewer, lens, timeout)
		}(index, lens)
	}
	wg.Wait()
	for index, err := range errs {
		if err != nil {
			return results, fmt.Errorf("reviewer %s lens %s: %w", reviewer.Name, reviewLenses[index].Key, err)
		}
	}
	return results, nil
}

func (a App) runReviewLoopReviewerWithAgent(liveOutput io.Writer, runID string, prep reviewerDryRunPreparation, reviewer reviewLoopReviewer, lens reviewLens, timeout time.Duration) (reviewerResultDocument, error) {
	var stdout bytes.Buffer
	runErr := a.runReviewerAttempt(&stdout, liveOutput, runID, prep, reviewer, lens, timeout)
	var result reviewerResultDocument
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			if runErr != nil {
				return reviewerResultDocument{}, runErr
			}
			return reviewerResultDocument{}, err
		}
	}
	return result, runErr
}

func (a App) runReviewerAttempt(stdout io.Writer, liveOutput io.Writer, runID string, prep reviewerDryRunPreparation, reviewer reviewLoopReviewer, lens reviewLens, timeout time.Duration) error {
	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[reviewerLensAttemptPlan, reviewerRequestDocument, reviewerResultDocument, struct{}]{
		Stdout:          stdout,
		LiveOutput:      liveOutput,
		JSONOutput:      true,
		Root:            prep.Context.Root,
		EventsJSONL:     prep.Context.Paths.EventsJSONL,
		RunID:           runID,
		Stage:           "review",
		AttemptsDir:     prep.Context.RunPaths.ReviewAttemptsDir,
		AttemptIDPrefix: "reviewer_attempt",
		LastResultJSON:  prep.Context.RunPaths.ReviewLastResultJSON,
		AgentName:       reviewer.Name,
		Agent:           reviewer.Agent,
		Model:           reviewer.ModelSpec,
		PromptRepoPath:  runArtifactRepoRel(runID, reviewerLensPromptArtifact(reviewer.Name, lens)),
		ArtifactDir:     reviewerAttemptsArtifact,
		Timeout:         timeout,
		ReadOnly:        true,
		StartedEvent:    "reviewer_attempt_started",
		FinishedEvent:   "reviewer_attempt_finished",
		ExitKind:        "reviewer",
		TimeoutMessage: func(timeout time.Duration) string {
			return fmt.Sprintf("reviewer process produced no output for %s", timeout)
		},
		Prepare: func(createdAt string) (reviewerLensAttemptPlan, error) {
			return buildReviewerLensPlan(runID, reviewer.Name, lens, reviewer.Agent)
		},
		BuildRequest: func(context agentAttemptContext[reviewerLensAttemptPlan]) (reviewerRequestDocument, error) {
			return reviewerRequestDocument{
				Schema:    reviewerRequestSchema,
				RunID:     runID,
				AttemptID: context.AttemptID,
				CreatedAt: context.CreatedAt,
				Reviewer:  agentDescriptorDocument(reviewer.Agent),
				Lens:      lens.Key,
				Artifacts: context.Prepared.Artifacts,
				WouldRun:  context.Prepared.WouldRun,
			}, nil
		},
		BuildResult: func(context agentAttemptContext[reviewerLensAttemptPlan], runResult agents.RunResult) reviewerResultDocument {
			return reviewerResultDocument{
				Schema:        reviewerResultSchema,
				RunID:         runID,
				AttemptID:     context.AttemptID,
				Reviewer:      reviewer.Agent.Name,
				Lens:          lens.Key,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result reviewerResultDocument) processResult {
			return result.processResult
		},
		// Unreachable in practice: JSONOutput is hardcoded true above, and the
		// lifecycle renders run-only output only when JSONOutput is false. Kept
		// because the lifecycle field is required; do not add logic here.
		RenderRunOnly: func(stdout io.Writer, request reviewerRequestDocument, result reviewerResultDocument) {
			fmt.Fprintf(stdout, "reviewer attempt %s [%s] exit code %d\n", result.AttemptID, result.Lens, result.ExitCode)
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

func (a App) acceptReviewLoopProposal(context reviewContext, proposalID string) (reviewFindingRecord, error) {
	var stdout bytes.Buffer
	if err := a.acceptReviewProposal(&stdout, context, context.State.RunID, proposalID, "review_loop", "", true); err != nil {
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
	if err := a.ReviewFix(&stdout, liveOutput, runID, agent, timeout, true); err != nil {
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
	if err := a.GateRun(&stdout, runID, true); err != nil {
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

func (a App) reviewLoopWorkingTreeFingerprint(context reviewContext) (string, error) {
	changes := a.buildGateChangeReport(context.Root, context.Paths)
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
	fmt.Fprintln(stdout, "Review run finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", summary.RunID)
	fmt.Fprintf(stdout, "  terminal reason: %s\n", summary.TerminalReason)
	fmt.Fprintf(stdout, "  rounds: %d/%d\n", len(summary.Rounds), summary.MaxRounds)
	fmt.Fprintf(stdout, "  clean rounds: %d\n", summary.CleanRoundsRequired)
	fmt.Fprintf(stdout, "  stalemate patience: %d\n", summary.StalematePatience)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Round results:")
	for _, round := range summary.Rounds {
		reviewerAttempts := round.ReviewerAttemptID
		if len(round.ReviewerAttemptIDs) > 1 {
			reviewerAttempts = fmt.Sprintf("%d lens attempts", len(round.ReviewerAttemptIDs))
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
