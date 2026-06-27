package app

import (
	"bufio"
	"bytes"
	stdctx "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/ledger"
	"github.com/heurema/pactum/internal/loop"
)

const (
	contractReviewSchema           = "pactum.contract_review.v1alpha1"
	contractReviewerRequestSchema  = "pactum.contract_reviewer_request.v1alpha1"
	contractReviewerResultSchema   = "pactum.contract_reviewer_result.v1alpha1"
	contractReviewResolutionSchema = "pactum.contract_review_resolution.v1alpha1"

	contractReviewLoopSchema         = "pactum.contract_review_loop.v1alpha1"
	contractReviewFixerReviseSchema  = "pactum.contract_revise.v1alpha1"
	contractReviewFixerRequestSchema = "pactum.contract_review_fixer_request.v1alpha1"
	contractReviewFixerResultSchema  = "pactum.contract_review_fixer_result.v1alpha1"

	// contractReviewerAttemptsArtifact is the repo-relative artifact path prefix
	// for contract review attempts; mirrors reviewerAttemptsArtifact for code review.
	contractReviewerAttemptsArtifact    = "contract/reviewer/attempts"
	contractReviewFixerAttemptsArtifact = "contract/fixer/attempts"
	contractReviewFixerPromptArtifact   = "contract/fixer/fixer-prompt.md"
)

// contractReviewLoopFatalError wraps a loop error after the loop response has
// already been written to stdout. App.Run checks for this type to skip the
// generic error envelope and avoid writing two JSON documents.
type contractReviewLoopFatalError struct{ cause error }

func (e contractReviewLoopFatalError) Error() string { return e.cause.Error() }
func (e contractReviewLoopFatalError) Unwrap() error { return e.cause }

// contractReviewLens is one specialist lens for reviewing the contract document.
// The set is fixed in code — every review spawns one attempt per lens per reviewer.
type contractReviewLens struct {
	Key       string
	Focus     string
	Heading   string
	Checklist []string
}

var contractReviewLenses = []contractReviewLens{
	{
		Key:     "completeness",
		Focus:   "contract-completeness",
		Heading: "Completeness",
		Checklist: []string{
			"Does the contract fully cover its goal? Are there gaps in scope or acceptance_criteria?",
			"Is every acceptance criterion specific and observable enough to verify?",
		},
	},
	{
		Key:     "testability",
		Focus:   "acceptance-testability",
		Heading: "Testability",
		Checklist: []string{
			"Is each acceptance criterion backed by or expressible as a runnable validation command (not just prose)?",
			"Are any criteria purely prose with no machine-checkable outcome?",
		},
	},
	{
		Key:     "validation-soundness",
		Focus:   "validation-soundness",
		Heading: "Validation soundness",
		Checklist: []string{
			"Are validation.commands gate-runnable (no shell forms the gate cannot execute)?",
			"Are they non-vacuous: would they fail on wrong output?",
			"Are they self-consistent and not contradictory with the tests?",
		},
	},
	{
		Key:     "scope-fidelity",
		Focus:   "scope-fidelity",
		Heading: "Scope fidelity",
		Checklist: []string{
			"Is scope.in coherent with and proportionate to the goal?",
			"Is scope.out coherent and not contradictory with scope.in?",
			"Is the scope neither over-broad nor under-broad for the stated goal?",
		},
	},
	{
		Key:     "assumptions-surfaced",
		Focus:   "assumptions-surfaced",
		Heading: "Assumptions surfaced",
		Checklist: []string{
			"Are risky assumptions explicitly called out rather than buried in scope or acceptance criteria?",
			"Are there implicit assumptions that affect executor behaviour and should be made explicit?",
		},
	},
}

// contractReviewFinding is one structured finding from a contract review attempt.
type contractReviewFinding struct {
	Reviewer       string `json:"reviewer"`
	Lens           string `json:"lens"`
	Category       string `json:"category,omitempty"`
	Severity       string `json:"severity"`
	Blocking       bool   `json:"blocking"`
	Message        string `json:"message"`
	Evidence       string `json:"evidence,omitempty"`
	MaterialImpact string `json:"material_impact,omitempty"`
	FixDirection   string `json:"fix_direction,omitempty"`
	Uncertainty    string `json:"uncertainty,omitempty"`
	State          string `json:"state,omitempty"`
}

// contractReviewerFindingInput is the contract-local parse-input struct for
// reviewer findings emitted under contractReviewerResultSchema. It carries the
// new precision fields in addition to the shared core fields.
type contractReviewerFindingInput struct {
	Message        string `json:"message"`
	Severity       string `json:"severity"`
	Category       string `json:"category"`
	Blocking       *bool  `json:"blocking"`
	Evidence       string `json:"evidence"`
	MaterialImpact string `json:"material_impact"`
	FixDirection   string `json:"fix_direction"`
	Uncertainty    string `json:"uncertainty"`
	State          string `json:"state"`
}

type contractReviewerAttemptArtifacts struct {
	ReviewerPrompt string `json:"reviewer_prompt"`
}

type contractReviewerAttemptPlan struct {
	Lens      string                           `json:"lens"`
	Artifacts contractReviewerAttemptArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand             `json:"would_run"`
}

type contractReviewerRequestDocument struct {
	Schema    string                           `json:"schema"`
	RunID     string                           `json:"run_id"`
	AttemptID string                           `json:"attempt_id"`
	CreatedAt string                           `json:"created_at"`
	Reviewer  agents.AgentDescriptor           `json:"reviewer"`
	Lens      string                           `json:"lens"`
	Artifacts contractReviewerAttemptArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand             `json:"would_run"`
}

type contractReviewerResultDocument struct {
	Schema    string `json:"schema"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Reviewer  string `json:"reviewer"`
	Lens      string `json:"lens"`
	processResult
}

// contractReviewRoundResult holds the collected output of one reviewer-panel round.
type contractReviewRoundResult struct {
	Findings      []contractReviewFinding
	SkippedLenses []reviewLoopSkippedLens
	Warnings      []string
	ParseMiss     bool
}

// contractReviewLoopRoundSummary records the outcome of one loop round, including
// findings from the reviewer panel and the fixer's result (when invoked).
type contractReviewLoopRoundSummary struct {
	Round                  int                     `json:"round"`
	Findings               []contractReviewFinding `json:"findings"`
	BlockingFindings       int                     `json:"blocking_findings"`
	ParseMiss              bool                    `json:"parse_miss,omitempty"`
	SkippedLenses          []reviewLoopSkippedLens `json:"skipped_lenses"`
	Warnings               []string                `json:"warnings,omitempty"`
	FixerAttemptID         string                  `json:"fixer_attempt_id,omitempty"`
	FixesApplied           int                     `json:"fixes_applied,omitempty"`
	FixesSkipped           int                     `json:"fixes_skipped,omitempty"`
	CleanStreak            int                     `json:"clean_streak"`
	UnchangedVersionStreak int                     `json:"unchanged_version_streak"`
}

// contractReviewResponse is the JSON response for contract review when
// contract.reviewers is absent or empty (the slice-1 no-op path).
type contractReviewResponse struct {
	Schema        string                  `json:"schema"`
	RunID         string                  `json:"run_id"`
	Reviewers     []string                `json:"reviewers"`
	Lenses        []string                `json:"lenses"`
	Findings      []contractReviewFinding `json:"findings"`
	SkippedLenses []reviewLoopSkippedLens `json:"skipped_lenses"`
	Warnings      []string                `json:"warnings,omitempty"`
	Next          []string                `json:"next"`
}

// contractReviewLoopResponse is the JSON response for contract review when
// reviewers are configured and the convergence loop runs.
type contractReviewLoopResponse struct {
	Schema              string                           `json:"schema"`
	RunID               string                           `json:"run_id"`
	Reviewers           []string                         `json:"reviewers"`
	Lenses              []string                         `json:"lenses"`
	MaxRounds           int                              `json:"max_rounds"`
	StalematePatience   int                              `json:"stalemate_patience"`
	CleanRoundsRequired int                              `json:"clean_rounds_required"`
	TerminalReason      string                           `json:"terminal_reason"`
	OpenBlockingCount   int                              `json:"open_blocking_count,omitempty"`
	NoProgressStreak    int                              `json:"no_progress_streak,omitempty"`
	NoProgressReason    string                           `json:"no_progress_reason,omitempty"`
	Rounds              []contractReviewLoopRoundSummary `json:"rounds"`
	Next                []string                         `json:"next"`
}

// contractReviewFindingLine is one entry in the contract/reviewer/findings.jsonl
// artifact written at the end of a contract review loop run.
type contractReviewFindingLine struct {
	ID             string `json:"id"`
	Fingerprint    string `json:"fingerprint"`
	Category       string `json:"category"`
	Message        string `json:"message"`
	Blocking       bool   `json:"blocking"`
	MaterialImpact string `json:"material_impact,omitempty"`
	FixDirection   string `json:"fix_direction,omitempty"`
	Uncertainty    string `json:"uncertainty,omitempty"`
	State          string `json:"state,omitempty"`
}

// contractReviewResolutionRecord is one entry in contract/reviewer/resolutions.jsonl.
type contractReviewResolutionRecord struct {
	Schema       string `json:"schema"`
	ID           string `json:"id"`
	FindingID    string `json:"finding_id"`
	Fingerprint  string `json:"fingerprint"`
	ContractHash string `json:"contract_hash"`
	Reason       string `json:"reason"`
	By           string `json:"by"`
	Timestamp    string `json:"timestamp"`
	Source       string `json:"source"`
}

// contractReviewWaivedFinding is one entry in the waiver summary printed when
// contract approve succeeds despite operator-resolved blocking findings.
type contractReviewWaivedFinding struct {
	FindingID   string `json:"finding_id"`
	Fingerprint string `json:"fingerprint"`
	Reason      string `json:"reason"`
	By          string `json:"by"`
}

// contractReviewFindingResolveResponse is the JSON response for
// pactum contract review finding resolve.
type contractReviewFindingResolveResponse struct {
	Resolution contractReviewResolutionRecord `json:"resolution"`
	Next       []string                       `json:"next"`
}

type contractReviewFixerArtifacts struct {
	FixerPrompt string `json:"fixer_prompt"`
}

type contractReviewFixerAttemptPlan struct {
	Artifacts contractReviewFixerArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand         `json:"would_run"`
}

type contractReviewFixerRequestDocument struct {
	Schema    string                       `json:"schema"`
	RunID     string                       `json:"run_id"`
	AttemptID string                       `json:"attempt_id"`
	CreatedAt string                       `json:"created_at"`
	Fixer     agents.AgentDescriptor       `json:"fixer"`
	Artifacts contractReviewFixerArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand         `json:"would_run"`
}

type contractReviewFixerResultDocument struct {
	Schema    string `json:"schema"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Fixer     string `json:"fixer"`
	processResult
}

// contractReviewerLensTask is one (reviewer, lens) attempt of the fan-out.
type contractReviewerLensTask struct {
	reviewerIndex int
	lensIndex     int
	reviewer      reviewLoopReviewer
	lens          contractReviewLens
}

// contractReviewerLensGroup groups lens tasks by stagger key (same as code review).
type contractReviewerLensGroup struct {
	key    reviewerStaggerKey
	claude bool
	tasks  []contractReviewerLensTask
}

func (g contractReviewerLensGroup) staggered() bool {
	return g.claude && len(g.tasks) > 1
}

// ContractReview runs the configured contract.reviewers panel against the
// contract document for the given run. When contract.reviewers is absent or
// empty, it exits successfully with a no-op message and no reviewer attempts.
// When reviewers are configured, it runs the fixer convergence loop.
func (a App) ContractReview(stdout io.Writer, liveOutput io.Writer, runID string, timeout time.Duration, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return err
	}

	lensKeys := contractReviewLensKeys()

	if len(config.Pipeline.ContractReview.By) == 0 {
		// Emit loop events unconditionally even for the no-reviewer no-op path.
		now := a.nowUTC()
		if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "contract_review_loop_started", Timestamp: now, RunID: runID}); err != nil {
			return err
		}
		if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "contract_review_loop_finished", Timestamp: now, RunID: runID}); err != nil {
			return err
		}
		if jsonOutput {
			return writeJSONResponse(stdout, contractReviewResponse{
				Schema:        contractReviewSchema,
				RunID:         runID,
				Reviewers:     []string{},
				Lenses:        lensKeys,
				Findings:      []contractReviewFinding{},
				SkippedLenses: []reviewLoopSkippedLens{},
				Next:          nextCommandsForRun(context.Paths, runID),
			})
		}
		fmt.Fprintln(stdout, "No contract reviewers configured (contract.reviewers is empty or absent); nothing to review.")
		return nil
	}

	timeout, err = resolveReviewIdleTimeout(timeout)
	if err != nil {
		return err
	}
	wallClockCap, err := resolveWallClockCap(config.WallClockCap.Duration())
	if err != nil {
		return err
	}

	reviewers, err := a.resolveContractReviewers(config)
	if err != nil {
		return err
	}

	limits, err := resolveContractReviewLoopLimits(context.Paths.Config)
	if err != nil {
		return err
	}

	return a.runContractReviewLoop(stdout, liveOutput, runID, context, reviewers, limits, lensKeys, timeout, wallClockCap, jsonOutput)
}

// runContractReviewLoop drives the reviewer/fixer convergence loop for contract
// review. Each round runs the configured reviewer panel; blocking findings drive
// a fixer attempt via contract revise --from -; the loop converges when a clean
// round is reached, patience is exhausted (stalemate), or max rounds is hit.
func (a App) runContractReviewLoop(
	stdout, liveOutput io.Writer,
	runID string,
	runCtx runContext,
	reviewers []reviewLoopReviewer,
	limits reviewLimits,
	lensKeys []string,
	timeout time.Duration,
	wallClockCap time.Duration,
	jsonOutput bool,
) error {
	startedAt := a.nowUTC()
	if err := ledger.Append(activeStore, runCtx.Paths.EventsJSONL, ledger.Event{Type: "contract_review_loop_started", Timestamp: startedAt, RunID: runID}); err != nil {
		return err
	}

	rounds := make([]contractReviewLoopRoundSummary, 0)
	cleanStreak := 0
	unchangedVersionStreak := 0
	noProgressStreak := 0
	var prevBlockerKeys map[string]bool

	step := func(_ stdctx.Context, rc loop.RoundContext) (loop.RoundResult, error) {
		round := rc.Round

		// Reload context: contract may have changed due to fixer in a prior round.
		roundCtx, ok, err := a.loadContractContext(io.Discard, runID, true)
		if err != nil || !ok {
			return loop.RoundResult{}, err
		}

		if err := activeStore.MkdirAll(roundCtx.RunPaths.ContractReviewAttemptsDir); err != nil {
			return loop.RoundResult{}, err
		}
		if err := writeContractReviewerPrompts(roundCtx.RunPaths, roundCtx.Contract, reviewers); err != nil {
			return loop.RoundResult{}, err
		}

		roundResult, err := a.runContractReviewRound(liveOutput, runID, roundCtx, reviewers, timeout, wallClockCap)
		if err != nil {
			return loop.RoundResult{}, err
		}

		blockingCount := countContractReviewBlockingFindings(roundResult.Findings)
		roundSummary := contractReviewLoopRoundSummary{
			Round:            round,
			Findings:         roundResult.Findings,
			BlockingFindings: blockingCount,
			ParseMiss:        roundResult.ParseMiss,
			SkippedLenses:    roundResult.SkippedLenses,
			Warnings:         roundResult.Warnings,
		}

		if roundResult.ParseMiss {
			cleanStreak = 0
			roundSummary.CleanStreak = cleanStreak
			roundSummary.UnchangedVersionStreak = unchangedVersionStreak
			rounds = append(rounds, roundSummary)
			return loop.RoundResult{}, errReviewerUnparsed
		}

		if blockingCount == 0 {
			// Non-blocking (advisory) findings do not block convergence; only
			// blocking findings reset the clean streak.
			cleanStreak++
			roundSummary.CleanStreak = cleanStreak
			roundSummary.UnchangedVersionStreak = unchangedVersionStreak
			rounds = append(rounds, roundSummary)
			return loop.RoundResult{Clean: true}, nil
		}

		cleanStreak = 0
		roundSummary.CleanStreak = 0

		versionBefore, err := storeFileSHA256(roundCtx.RunPaths.ContractJSON)
		if err != nil {
			return loop.RoundResult{}, err
		}

		fixerAttemptID, fixesApplied, fixesSkipped, fixerWarnings, err := a.runContractReviewFixRound(liveOutput, runID, roundCtx, roundResult.Findings, versionBefore, timeout, wallClockCap)
		if err != nil {
			return loop.RoundResult{}, err
		}
		roundSummary.FixerAttemptID = fixerAttemptID
		roundSummary.FixesApplied = fixesApplied
		roundSummary.FixesSkipped = fixesSkipped
		roundSummary.Warnings = append(roundSummary.Warnings, fixerWarnings...)

		versionAfter, err := storeFileSHA256(roundCtx.RunPaths.ContractJSON)
		if err != nil {
			return loop.RoundResult{}, err
		}

		progress := versionAfter != versionBefore
		if progress {
			unchangedVersionStreak = 0
		} else {
			unchangedVersionStreak++
		}
		roundSummary.UnchangedVersionStreak = unchangedVersionStreak

		// Fixer no-progress escalation: if the canonical key set of blocking
		// findings from the reviewer is unchanged for K=2 consecutive rounds,
		// exit early so the fixer cannot burn all rounds on the same blockers.
		currBlockerKeys := contractBlockingFindingKeySet(roundResult.Findings)
		if prevBlockerKeys != nil && sameStringSet(currBlockerKeys, prevBlockerKeys) {
			noProgressStreak++
		} else {
			noProgressStreak = 0
		}
		prevBlockerKeys = currBlockerKeys
		rounds = append(rounds, roundSummary)
		if noProgressStreak >= 2 {
			return loop.RoundResult{}, errFixerNoProgress
		}

		return loop.RoundResult{Clean: false, Progress: progress}, nil
	}

	loopLimits := loop.Limits{
		Max:      limits.MaxRounds,
		Patience: limits.Patience,
		Settle:   limits.CleanRounds,
	}

	outcome, loopErr := loop.Run(stdctx.Background(), loopLimits, step)

	terminalReason := ""
	openBlockingCount := 0
	loopNoProgressStreak := 0
	loopNoProgressReason := ""
	switch {
	case errors.Is(loopErr, errFixerNoProgress):
		terminalReason = "fixer_no_progress"
		openBlockingCount = lastRoundBlockingCount(rounds)
		loopNoProgressStreak = noProgressStreak
		loopNoProgressReason = "open blocking finding key set unchanged for 2 consecutive fixer rounds"
		loopErr = nil
	case errors.Is(loopErr, errReviewerUnparsed):
		terminalReason = "reviewer_findings_unparsed"
		loopErr = nil
	case loopErr != nil:
		terminalReason = "error"
	default:
		switch outcome.Reason {
		case "settled":
			terminalReason = "resolved"
		case "stalemate", "max":
			// Blocking findings open at loop end → override to blockers_open so
			// callers are never left with a silently non-approvable terminal reason.
			if n := lastRoundBlockingCount(rounds); n > 0 {
				terminalReason = "blockers_open"
				openBlockingCount = n
			} else if outcome.Reason == "stalemate" {
				terminalReason = "stalemate"
			} else {
				terminalReason = "max_rounds"
			}
		case "human":
			terminalReason = "human"
		}
	}

	// Write the durable findings artifact so contract approve can enforce the guard.
	if loopErr == nil && terminalReason != "error" {
		if writeErr := removeContractReviewResolutionsArtifact(runCtx.RunPaths); writeErr != nil {
			loopErr = writeErr
		}
		if loopErr == nil {
			if writeErr := writeContractReviewFindingsJSONL(runCtx.RunPaths, rounds); writeErr != nil {
				loopErr = writeErr
			}
		}
		if loopErr == nil && terminalReason == "resolved" {
			if writeErr := ensureContractReviewResolutionsJSONL(runCtx.RunPaths); writeErr != nil && loopErr == nil {
				loopErr = writeErr
			}
		}
	}

	// Always emit the finished event so no run leaves a dangling started event.
	finishedAt := a.nowUTC()
	if appendErr := ledger.Append(activeStore, runCtx.Paths.EventsJSONL, ledger.Event{Type: "contract_review_loop_finished", Timestamp: finishedAt, RunID: runID}); appendErr != nil && loopErr == nil {
		loopErr = appendErr
	}

	// For non-approvable terminals, point the operator at inspection rather than approve.
	nextCmds := nextCommandsForRun(runCtx.Paths, runID)
	if terminalReason == "blockers_open" || terminalReason == "fixer_no_progress" || terminalReason == "reviewer_findings_unparsed" {
		nextCmds = []string{"pactum contract show " + runID}
	}

	response := contractReviewLoopResponse{
		Schema:              contractReviewLoopSchema,
		RunID:               runID,
		Reviewers:           reviewLoopReviewerNames(reviewers),
		Lenses:              lensKeys,
		MaxRounds:           limits.MaxRounds,
		StalematePatience:   limits.Patience,
		CleanRoundsRequired: limits.CleanRounds,
		TerminalReason:      terminalReason,
		OpenBlockingCount:   openBlockingCount,
		NoProgressStreak:    loopNoProgressStreak,
		NoProgressReason:    loopNoProgressReason,
		Rounds:              rounds,
		Next:                nextCmds,
	}

	if loopErr != nil {
		// Write the structured response to stdout even on error so callers can
		// parse terminal_reason "error". Wrap the error so App.Run skips its
		// generic error envelope — one JSON document on stdout is the contract.
		_ = writeJSONResponse(stdout, response)
		return contractReviewLoopFatalError{cause: loopErr}
	}

	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeContractReviewLoopOutput(stdout, response)
	return nil
}

// runContractReviewRound runs one round of the reviewer panel and collects findings.
func (a App) runContractReviewRound(
	liveOutput io.Writer,
	runID string,
	context runContext,
	reviewers []reviewLoopReviewer,
	timeout time.Duration,
	wallClockCap time.Duration,
) (contractReviewRoundResult, error) {
	memberResults, errs := a.runContractReviewFanOut(liveOutput, runID, context, reviewers, timeout, wallClockCap)

	var skipped []reviewLoopSkippedLens
	var skipWarnings []string
	var firstErr error
	var firstErrReviewer, firstErrLens string
	successCount := 0
	for reviewerIndex, reviewer := range reviewers {
		for lensIndex, lensErr := range errs[reviewerIndex] {
			if lensErr != nil {
				if firstErr == nil {
					firstErr = lensErr
					firstErrReviewer = reviewer.Name
					firstErrLens = contractReviewLenses[lensIndex].Key
				}
				skipped = append(skipped, reviewLoopSkippedLens{
					Reviewer: reviewer.Name,
					Lens:     contractReviewLenses[lensIndex].Key,
					Reason:   lensErr.Error(),
				})
				skipWarnings = append(skipWarnings, fmt.Sprintf("%s/%s: %s", reviewer.Name, contractReviewLenses[lensIndex].Key, lensErr.Error()))
			} else {
				successCount++
			}
		}
	}
	if successCount == 0 && firstErr != nil {
		return contractReviewRoundResult{}, fmt.Errorf("contract reviewer %s lens %s: %w", firstErrReviewer, firstErrLens, firstErr)
	}
	if skipped == nil {
		skipped = []reviewLoopSkippedLens{}
	}

	findings := []contractReviewFinding{}
	var parseWarnings []string
	parseMiss := false
	for reviewerIndex, reviewer := range reviewers {
		for lensIndex, result := range memberResults[reviewerIndex] {
			if errs[reviewerIndex][lensIndex] != nil || result.AttemptID == "" {
				continue
			}
			attemptPaths := agentAttemptPaths(context.RunPaths.ContractReviewAttemptsDir, result.AttemptID)
			stdoutBytes, readErr := activeStore.ReadBytes(attemptPaths.StdoutLog)
			if readErr != nil {
				return contractReviewRoundResult{}, readErr
			}
			lensKey := contractReviewLenses[lensIndex].Key
			if len(stdoutBytes) == 0 {
				parseMiss = true
				parseWarnings = append(parseWarnings, fmt.Sprintf("%s/%s: attempt=1: empty stdout — hard failure (parse miss)", reviewer.Name, lensKey))
				continue
			}
			blocks, blockWarnings := parseContractReviewerFindingBlocks(string(stdoutBytes))
			if len(blocks) == 0 {
				parseWarnings = append(parseWarnings, fmt.Sprintf("%s/%s: attempt=1: no valid findings block — corrective retry", reviewer.Name, lensKey))
				for _, w := range blockWarnings {
					parseWarnings = append(parseWarnings, fmt.Sprintf("%s/%s: attempt=1: %s", reviewer.Name, lensKey, w))
				}
				corrResult, corrOK, corrWarnings := a.runContractReviewerCorrectiveAttempt(liveOutput, runID, context, reviewer, contractReviewLenses[lensIndex], timeout, wallClockCap)
				for _, w := range corrWarnings {
					parseWarnings = append(parseWarnings, fmt.Sprintf("%s/%s: attempt=2: %s", reviewer.Name, lensKey, w))
				}
				if !corrOK {
					parseMiss = true
					continue
				}
				attemptPaths = agentAttemptPaths(context.RunPaths.ContractReviewAttemptsDir, corrResult.AttemptID)
				stdoutBytes, readErr = activeStore.ReadBytes(attemptPaths.StdoutLog)
				if readErr != nil {
					return contractReviewRoundResult{}, readErr
				}
				blocks, blockWarnings = parseContractReviewerFindingBlocks(string(stdoutBytes))
				for _, w := range blockWarnings {
					parseWarnings = append(parseWarnings, fmt.Sprintf("%s/%s: attempt=2: %s", reviewer.Name, lensKey, w))
				}
			} else {
				for _, w := range blockWarnings {
					parseWarnings = append(parseWarnings, fmt.Sprintf("%s/%s: attempt=1: %s", reviewer.Name, lensKey, w))
				}
			}
			parsedFindings, findingWarnings := contractReviewFindingsFromBlocks(reviewer.Name, lensKey, blocks)
			findings = append(findings, parsedFindings...)
			for _, w := range findingWarnings {
				parseWarnings = append(parseWarnings, fmt.Sprintf("%s/%s: %s", reviewer.Name, lensKey, w))
			}
		}
	}

	return contractReviewRoundResult{
		Findings:      findings,
		SkippedLenses: skipped,
		Warnings:      append(skipWarnings, parseWarnings...),
		ParseMiss:     parseMiss,
	}, nil
}

// runContractReviewFixRound runs one fixer round: invokes the fixer agent,
// parses its revise output, and applies it via ContractRevise. Returns the
// fixer attempt ID, the number of fixes applied and skipped, and any warnings.
// A fixer failure (stale version, agent error, unparseable output) is recorded
// as a skipped fix; the loop refreshes the version and continues.
func (a App) runContractReviewFixRound(
	liveOutput io.Writer,
	runID string,
	context runContext,
	findings []contractReviewFinding,
	currentVersion string,
	timeout time.Duration,
	wallClockCap time.Duration,
) (attemptID string, applied int, skipped int, warnings []string, err error) {
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return "", 0, 0, nil, err
	}

	entry, err := resolveExecutorEntry(config, config.Pipeline.Execute.By, "")
	if err != nil {
		return "", 0, 0, nil, err
	}
	resolved, err := a.resolveAgentForRole(entry, agentRoleReviewer)
	if err != nil {
		return "", 0, 0, nil, err
	}

	if err := activeStore.MkdirAll(context.RunPaths.ContractFixerDir); err != nil {
		return "", 0, 0, nil, err
	}

	blockingFindings := contractReviewBlockingFindings(findings)
	prompt := renderContractReviewFixerPrompt(context.Contract, currentVersion, blockingFindings)
	if err := activeStore.WriteBytes(context.RunPaths.ContractFixerPromptMD, []byte(prompt), 0o644); err != nil {
		return "", 0, 0, nil, err
	}

	var fixerOut bytes.Buffer
	runErr := a.runContractReviewFixerAttempt(&fixerOut, liveOutput, runID, context, resolved, timeout, wallClockCap)

	var result contractReviewFixerResultDocument
	if fixerOut.Len() > 0 {
		_ = json.Unmarshal(fixerOut.Bytes(), &result)
	}
	resultAttemptID := result.AttemptID

	if runErr != nil {
		msg := fmt.Sprintf("fixer attempt failed: %v", runErr)
		if resultAttemptID != "" {
			msg = fmt.Sprintf("fixer attempt %s failed: %v", resultAttemptID, runErr)
		}
		return resultAttemptID, 0, 1, []string{msg}, nil
	}
	if resultAttemptID == "" {
		return "", 0, 1, []string{"fixer produced no parseable result"}, nil
	}

	attemptPaths := agentAttemptPaths(context.RunPaths.ContractFixerAttemptsDir, resultAttemptID)
	fixerStdout, err := activeStore.ReadBytes(attemptPaths.StdoutLog)
	if err != nil {
		return resultAttemptID, 0, 1, []string{fmt.Sprintf("cannot read fixer stdout: %v", err)}, nil
	}

	reviseInput, ok := parseContractReviewFixerOutput(string(fixerStdout))
	if !ok {
		return resultAttemptID, 0, 1, []string{"fixer produced no revise block; fix skipped"}, nil
	}

	// The fixer block includes "schema" for identification; strip it before
	// parseContractReviseInput (which rejects unknown top-level fields).
	strippedInput, stripErr := stripJSONField(reviseInput, "schema")
	if stripErr != nil {
		return resultAttemptID, 0, 1, []string{"fixer revise invalid JSON: " + stripErr.Error()}, nil
	}

	// Parse and validate; check goal protection before applying.
	_, update, parseIssues := parseContractReviseInput(strippedInput)
	if len(parseIssues) > 0 {
		msgs := make([]string, 0, len(parseIssues))
		for _, issue := range parseIssues {
			msgs = append(msgs, "fixer revise invalid: "+issue.Message)
		}
		return resultAttemptID, 0, 1, msgs, nil
	}
	if update.Goal != nil && *update.Goal != context.Contract.Goal {
		return resultAttemptID, 0, 1, []string{"fixer revise rejected: would change contract goal (out of scope)"}, nil
	}

	var reviseOut bytes.Buffer
	reviseErr := a.ContractRevise(&reviseOut, runID, strippedInput, false, true)
	if reviseErr != nil {
		var reviseFailure contractReviseFailureError
		if errors.As(reviseErr, &reviseFailure) {
			var failure contractReviseFailure
			msg := "fixer revise skipped: " + reviseErr.Error()
			if json.Unmarshal(reviseOut.Bytes(), &failure) == nil && len(failure.Issues) > 0 {
				msg = "fixer revise skipped: " + failure.Issues[0].Message
			}
			return resultAttemptID, 0, 1, []string{msg}, nil
		}
		return "", 0, 0, nil, reviseErr
	}

	versionAfterRevise, err := storeFileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return "", 0, 0, nil, err
	}
	if versionAfterRevise == currentVersion {
		return resultAttemptID, 0, 1, []string{"fixer revise was a no-op; contract unchanged"}, nil
	}
	return resultAttemptID, 1, 0, nil, nil
}

func (a App) runContractReviewFixerAttempt(stdout io.Writer, liveOutput io.Writer, runID string, context runContext, resolved resolvedAgent, timeout time.Duration, wallClockCap time.Duration) error {
	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[contractReviewFixerAttemptPlan, contractReviewFixerRequestDocument, contractReviewFixerResultDocument, struct{}]{
		Stdout:          stdout,
		LiveOutput:      liveOutput,
		JSONOutput:      true,
		Root:            context.Root,
		EventsJSONL:     context.Paths.EventsJSONL,
		RunID:           runID,
		Stage:           "contract_fixer",
		AttemptsDir:     context.RunPaths.ContractFixerAttemptsDir,
		AttemptIDPrefix: "contract_fixer_attempt",
		LastResultJSON:  context.RunPaths.ContractFixerLastResultJSON,
		AgentName:       resolved.Name,
		Agent:           resolved.Agent,
		Model:           resolved.ModelSpec,
		PromptRepoPath:  runArtifactRepoRel(runID, contractReviewFixerPromptArtifact),
		ArtifactDir:     contractReviewFixerAttemptsArtifact,
		Timeout:         timeout,
		WallClockCap:    wallClockCap,
		ReadOnly:        true,
		StartedEvent:    "contract_fixer_attempt_started",
		FinishedEvent:   "contract_fixer_attempt_finished",
		ExitKind:        "contract fixer",
		TimeoutMessage: func(d time.Duration) string {
			return fmt.Sprintf("contract fixer process produced no output for %s", d)
		},
		Prepare: func(createdAt string) (contractReviewFixerAttemptPlan, error) {
			wouldRun, err := agents.BuildACPWouldRun(resolved.Agent.Name, resolved.ModelSpec, false)
			if err != nil {
				return contractReviewFixerAttemptPlan{}, err
			}
			return contractReviewFixerAttemptPlan{
				Artifacts: contractReviewFixerArtifacts{FixerPrompt: contractReviewFixerPromptArtifact},
				WouldRun:  wouldRun,
			}, nil
		},
		BuildRequest: func(ctx agentAttemptContext[contractReviewFixerAttemptPlan]) (contractReviewFixerRequestDocument, error) {
			return contractReviewFixerRequestDocument{
				Schema:    contractReviewFixerRequestSchema,
				RunID:     runID,
				AttemptID: ctx.AttemptID,
				CreatedAt: ctx.CreatedAt,
				Fixer:     agentDescriptorDocument(resolved.Agent),
				Artifacts: ctx.Prepared.Artifacts,
				WouldRun:  ctx.Prepared.WouldRun,
			}, nil
		},
		BuildResult: func(ctx agentAttemptContext[contractReviewFixerAttemptPlan], runResult agents.RunResult) contractReviewFixerResultDocument {
			return contractReviewFixerResultDocument{
				Schema:        contractReviewFixerResultSchema,
				RunID:         runID,
				AttemptID:     ctx.AttemptID,
				Fixer:         resolved.Agent.Name,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result contractReviewFixerResultDocument) processResult {
			return result.processResult
		},
		RenderRunOnly: func(out io.Writer, request contractReviewFixerRequestDocument, result contractReviewFixerResultDocument) {
			fmt.Fprintf(out, "contract fixer attempt %s exit code %d\n", result.AttemptID, result.ExitCode)
		},
	})
}

// stripJSONField returns a copy of the JSON object input with the named field
// removed. Used to strip the "schema" identifier before passing a fixer revise
// block to parseContractReviseInput, which rejects unknown top-level fields.
func stripJSONField(input []byte, field string) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return nil, err
	}
	delete(m, field)
	return json.Marshal(m)
}

// parseContractReviewFixerOutput extracts the revise payload from the fixer's
// stdout. It takes the last JSON block whose "schema" field matches
// contractReviewFixerReviseSchema, so that the fixer can include reasoning
// before or after the block without it being misinterpreted.
func parseContractReviewFixerOutput(output string) ([]byte, bool) {
	text := agentMessageText([]byte(output))
	blocks := extractFencedJSONBlocks(text)
	for i := len(blocks) - 1; i >= 0; i-- {
		var probe struct {
			Schema string `json:"schema"`
		}
		if json.Unmarshal([]byte(blocks[i]), &probe) != nil {
			continue
		}
		if probe.Schema == contractReviewFixerReviseSchema {
			return []byte(blocks[i]), true
		}
	}
	return nil, false
}

// resolveContractReviewLoopLimits reads loop limits from the contract_review
// pipeline stage, falling back to the same in-code defaults used by code review.
func resolveContractReviewLoopLimits(configPath string) (reviewLimits, error) {
	config, err := readConfig(configPath)
	if err != nil {
		return reviewLimits{}, err
	}
	defaults := defaultConfigFile().Pipeline.ContractReview.Loop
	var configMax, configPatience, configSettle int
	if l := config.Pipeline.ContractReview.Loop; l != nil {
		configMax, configPatience, configSettle = l.Max, l.Patience, l.Settle
	}
	maxRounds, err := resolveReviewLoopLimit("max rounds", 0, configMax, defaults.Max)
	if err != nil {
		return reviewLimits{}, err
	}
	patience, err := resolveReviewLoopLimit("patience", 0, configPatience, defaults.Patience)
	if err != nil {
		return reviewLimits{}, err
	}
	cleanRounds, err := resolveReviewLoopLimit("clean rounds", 0, configSettle, defaults.Settle)
	if err != nil {
		return reviewLimits{}, err
	}
	return reviewLimits{MaxRounds: maxRounds, Patience: patience, CleanRounds: cleanRounds}, nil
}

func (a App) resolveContractReviewers(config configFile) ([]reviewLoopReviewer, error) {
	reviewers := make([]reviewLoopReviewer, 0, len(config.Pipeline.ContractReview.By))
	for _, name := range config.Pipeline.ContractReview.By {
		entry, err := findRegistryEntry(config, name)
		if err != nil {
			return nil, err
		}
		resolved, err := a.resolveAgentForRole(entry, agentRoleReviewer)
		if err != nil {
			return nil, err
		}
		reviewers = append(reviewers, reviewLoopReviewer{
			Name:      resolved.Name,
			Agent:     resolved.Agent,
			ModelSpec: resolved.ModelSpec,
		})
	}
	return reviewers, nil
}

func contractReviewLensKeys() []string {
	keys := make([]string, len(contractReviewLenses))
	for i, l := range contractReviewLenses {
		keys[i] = l.Key
	}
	return keys
}

func countContractReviewBlockingFindings(findings []contractReviewFinding) int {
	count := 0
	for _, f := range findings {
		if f.Blocking {
			count++
		}
	}
	return count
}

// contractBlockingFindingKeySet returns canonical keys for all blocking findings
// in a round, used ONLY for fixer-no-progress streak detection.
func contractBlockingFindingKeySet(findings []contractReviewFinding) map[string]bool {
	keys := map[string]bool{}
	for _, f := range findings {
		if !f.Blocking {
			continue
		}
		category := f.Category
		if category == "" {
			category = f.Lens
		}
		keys[canonicalBlockerKey(category, f.Message)] = true
	}
	return keys
}

// lastRoundBlockingCount returns the BlockingFindings count from the final
// round summary, or 0 if rounds is empty.
func lastRoundBlockingCount(rounds []contractReviewLoopRoundSummary) int {
	if len(rounds) == 0 {
		return 0
	}
	return rounds[len(rounds)-1].BlockingFindings
}

// writeContractReviewFindingsJSONL persists the last round's findings to the
// durable findings artifact so contract approve can enforce the blocking guard.
func writeContractReviewFindingsJSONL(runPaths contractRunPathSet, rounds []contractReviewLoopRoundSummary) error {
	var findings []contractReviewFinding
	if len(rounds) > 0 {
		findings = rounds[len(rounds)-1].Findings
	}
	if findings == nil {
		findings = []contractReviewFinding{}
	}
	lines := make([]contractReviewFindingLine, len(findings))
	for i, f := range findings {
		category := f.Category
		if category == "" {
			category = f.Lens
		}
		lines[i] = contractReviewFindingLine{
			ID:             nextContractReviewFindingID(i + 1),
			Fingerprint:    canonicalBlockerKey(category, f.Message),
			Category:       category,
			Message:        f.Message,
			Blocking:       f.Blocking,
			MaterialImpact: f.MaterialImpact,
			FixDirection:   f.FixDirection,
			Uncertainty:    f.Uncertainty,
			State:          f.State,
		}
	}
	var buf []byte
	for _, line := range lines {
		b, err := json.Marshal(line)
		if err != nil {
			return err
		}
		buf = append(buf, b...)
		buf = append(buf, '\n')
	}
	if buf == nil {
		buf = []byte{}
	}
	return activeStore.WriteBytes(runPaths.ContractReviewFindingsJSONL, buf, 0o644)
}

func ensureContractReviewResolutionsJSONL(runPaths contractRunPathSet) error {
	if isRegularFile(runPaths.ContractReviewResolutionsJSONL) {
		return nil
	}
	return activeStore.WriteBytes(runPaths.ContractReviewResolutionsJSONL, []byte{}, 0o644)
}

func removeContractReviewResolutionsArtifact(runPaths contractRunPathSet) error {
	if err := activeStore.Remove(runPaths.ContractReviewResolutionsJSONL); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func removeContractReviewAggregateArtifacts(runPaths contractRunPathSet) error {
	if err := activeStore.Remove(runPaths.ContractReviewFindingsJSONL); err != nil && !os.IsNotExist(err) {
		return err
	}
	return removeContractReviewResolutionsArtifact(runPaths)
}

func nextContractReviewFindingID(index int) string {
	return fmt.Sprintf("crf_%03d", index)
}

func nextContractReviewResolutionID(index int) string {
	return fmt.Sprintf("cres_%03d", index)
}

// readContractReviewFindingLines opens findings.jsonl via the activeStore.
// Fail-closed: absent or unreadable file → error (use readJSONLines for lenient reads).
func readContractReviewFindingLines(runPaths contractRunPathSet) ([]contractReviewFindingLine, error) {
	file, err := activeStore.Open(runPaths.ContractReviewFindingsJSONL)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var findings []contractReviewFindingLine
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry contractReviewFindingLine
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("contract review findings artifact is malformed: %w", err)
		}
		findings = append(findings, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("contract review findings unreadable: %w", err)
	}
	return findings, nil
}

// readContractReviewResolutions reads resolutions.jsonl. Returns empty slice if
// the file is absent (no resolutions recorded yet); errors on malformed content.
func readContractReviewResolutions(runPaths contractRunPathSet) ([]contractReviewResolutionRecord, error) {
	return readJSONLines[contractReviewResolutionRecord](runPaths.ContractReviewResolutionsJSONL)
}

// checkContractReviewFindingsApprovalGuard reads the durable findings artifact
// and refuses approval if any blocking finding lacks an active manual resolution.
// Fail-closed: findings absent, unreadable, or malformed → refuse. Resolutions
// absent → OK (treated as empty). A resolution is active when its ContractHash
// matches the current contract's SHA-256 and its Fingerprint matches the finding.
func checkContractReviewFindingsApprovalGuard(runPaths contractRunPathSet) ([]contractReviewWaivedFinding, error) {
	findings, err := readContractReviewFindingLines(runPaths)
	if err != nil {
		return nil, fmt.Errorf("cannot approve contract: contract review findings unavailable (run contract review first): %w", err)
	}

	resolutions, err := readContractReviewResolutions(runPaths)
	if err != nil {
		return nil, fmt.Errorf("cannot approve contract: contract review resolutions artifact is malformed: %w", err)
	}

	contractHash, err := storeFileSHA256(runPaths.ContractJSON)
	if err != nil {
		return nil, fmt.Errorf("cannot approve contract: cannot read contract hash: %w", err)
	}

	activeByFingerprint := make(map[string]contractReviewResolutionRecord)
	for _, r := range resolutions {
		if r.ContractHash == contractHash && r.Fingerprint != "" {
			activeByFingerprint[r.Fingerprint] = r
		}
	}

	var waivers []contractReviewWaivedFinding
	var blockingCount int
	seenWaiver := map[string]bool{}
	for _, f := range findings {
		if !f.Blocking {
			continue
		}
		r, ok := activeByFingerprint[f.Fingerprint]
		if !ok || f.Fingerprint == "" {
			blockingCount++
			continue
		}
		if !seenWaiver[f.Fingerprint] {
			seenWaiver[f.Fingerprint] = true
			waivers = append(waivers, contractReviewWaivedFinding{
				FindingID:   f.ID,
				Fingerprint: f.Fingerprint,
				Reason:      r.Reason,
				By:          r.By,
			})
		}
	}
	if blockingCount > 0 {
		return nil, fmt.Errorf("cannot approve contract: %d open blocking contract-review finding(s) remain", blockingCount)
	}
	return waivers, nil
}

// ContractReviewFindingResolve records a manual operator resolution for a
// blocking contract-review finding, enabling contract approve to proceed.
func (a App) ContractReviewFindingResolve(stdout io.Writer, runID, findingID, reason, by string, jsonOutput bool) error {
	root, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	findings, err := readContractReviewFindingLines(runPaths)
	if err != nil {
		return fmt.Errorf("contract review findings unavailable (run 'pactum contract review run %s' first): %w", runID, err)
	}

	var target *contractReviewFindingLine
	for i := range findings {
		if findings[i].ID == findingID {
			target = &findings[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("contract review finding not found: %s", findingID)
	}
	if !target.Blocking {
		return fmt.Errorf("finding %s is not a blocking finding; only blocking findings can be resolved", findingID)
	}

	existingResolutions, err := readContractReviewResolutions(runPaths)
	if err != nil {
		return fmt.Errorf("contract review resolutions artifact is malformed: %w", err)
	}

	contractHash, err := storeFileSHA256(runPaths.ContractJSON)
	if err != nil {
		return fmt.Errorf("cannot read contract hash: %w", err)
	}

	now := a.nowUTC()
	resolution := contractReviewResolutionRecord{
		Schema:       contractReviewResolutionSchema,
		ID:           nextContractReviewResolutionID(len(existingResolutions) + 1),
		FindingID:    findingID,
		Fingerprint:  target.Fingerprint,
		ContractHash: contractHash,
		Reason:       reason,
		By:           normalizePrincipal(root, by),
		Timestamp:    now.Format(time.RFC3339),
		Source:       "manual",
	}
	if err := appendJSONLine(runPaths.ContractReviewResolutionsJSONL, resolution); err != nil {
		return err
	}
	if ledgerErr := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "contract_review_finding_resolved", Timestamp: now, RunID: runID}); ledgerErr != nil {
		return fmt.Errorf("resolution recorded but ledger append failed: %w", ledgerErr)
	}

	response := contractReviewFindingResolveResponse{
		Resolution: resolution,
		Next:       nextCommandsForRun(paths, runID),
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeContractReviewFindingResolved(stdout, response)
	return nil
}

func writeContractReviewFindingResolved(stdout io.Writer, response contractReviewFindingResolveResponse) {
	fmt.Fprintln(stdout, "Contract review finding resolved")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Resolution:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Resolution.ID)
	fmt.Fprintf(stdout, "  finding: %s\n", response.Resolution.FindingID)
	fmt.Fprintf(stdout, "  by: %s\n", response.Resolution.By)
	fmt.Fprintf(stdout, "  reason: %s\n", response.Resolution.Reason)
	fmt.Fprintf(stdout, "  contract sha256: %s\n", response.Resolution.ContractHash)
	if len(response.Next) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Next:")
		for _, cmd := range response.Next {
			fmt.Fprintf(stdout, "  %s\n", cmd)
		}
	}
}

func contractReviewBlockingFindings(findings []contractReviewFinding) []contractReviewFinding {
	var blocking []contractReviewFinding
	for _, f := range findings {
		if f.Blocking {
			blocking = append(blocking, f)
		}
	}
	return blocking
}

func contractFindingFromInput(reviewerName string, lensKey string, input contractReviewerFindingInput) contractReviewFinding {
	blocking := false
	if input.Blocking != nil {
		blocking = *input.Blocking
	}
	state := strings.TrimSpace(input.State)
	if state != "candidate" && state != "confirmed" {
		state = "candidate"
	}
	return contractReviewFinding{
		Reviewer:       reviewerName,
		Lens:           lensKey,
		Category:       strings.TrimSpace(input.Category),
		Severity:       strings.TrimSpace(input.Severity),
		Blocking:       blocking,
		Message:        strings.TrimSpace(input.Message),
		Evidence:       strings.TrimSpace(input.Evidence),
		MaterialImpact: strings.TrimSpace(input.MaterialImpact),
		FixDirection:   strings.TrimSpace(input.FixDirection),
		Uncertainty:    strings.TrimSpace(input.Uncertainty),
		State:          state,
	}
}

func contractReviewFindingsFromBlocks(reviewerName string, lensKey string, blocks []reviewerFindingBlock) ([]contractReviewFinding, []string) {
	var findings []contractReviewFinding
	var warnings []string
	for _, block := range blocks {
		for _, rawFinding := range *block.Findings {
			var input contractReviewerFindingInput
			if err := json.Unmarshal(rawFinding, &input); err != nil {
				warnings = append(warnings, "finding skipped: invalid JSON")
				continue
			}
			if strings.TrimSpace(input.Message) == "" {
				continue
			}
			f := contractFindingFromInput(reviewerName, lensKey, input)
			if f.Blocking && f.MaterialImpact == "" {
				f.Blocking = false
				warnings = append(warnings, "finding downgraded to advisory: blocking=true requires non-empty material_impact")
			}
			findings = append(findings, f)
		}
	}
	return findings, warnings
}

// parseContractReviewerFindingBlocks extracts structured finding blocks from
// contract reviewer output. Accepts only blocks whose schema matches
// contractReviewerResultSchema; mirrors parseReviewerFindingBlocks but is
// contract-local so the contract and code-review schemas remain independent.
func parseContractReviewerFindingBlocks(output string) ([]reviewerFindingBlock, []string) {
	text := agentMessageText([]byte(output))
	jsonBlocks := extractFencedJSONBlocks(text)
	blocks := make([]reviewerFindingBlock, 0, len(jsonBlocks))
	warnings := []string{}
	schemaBlockCount := 0
	for _, raw := range jsonBlocks {
		var block reviewerFindingBlock
		if err := json.Unmarshal([]byte(raw), &block); err != nil {
			if strings.Contains(raw, contractReviewerResultSchema) {
				schemaBlockCount++
				warnings = append(warnings, "contract reviewer findings block has invalid JSON — parse miss (inspect attempt stdout)")
			} else {
				warnings = append(warnings, "structured block skipped: invalid JSON")
			}
			continue
		}
		if block.Schema != contractReviewerResultSchema {
			continue
		}
		schemaBlockCount++
		if block.Findings == nil {
			warnings = append(warnings, "contract reviewer findings block has null or absent findings key — parse miss (inspect attempt stdout)")
			continue
		}
		blocks = append(blocks, block)
	}
	if schemaBlockCount > 1 {
		warnings = append(warnings, fmt.Sprintf("multiple contract reviewer findings blocks found (%d) — parse miss (inspect attempt stdout)", schemaBlockCount))
		return nil, warnings
	}
	if len(blocks) == 0 && strings.TrimSpace(text) != "" {
		warnings = append(warnings, "no valid contract reviewer findings block parsed — parse miss (inspect attempt stdout)")
	}
	return blocks, warnings
}

func writeContractReviewerPrompts(runPaths contractRunPathSet, contract draftContract, reviewers []reviewLoopReviewer) error {
	for _, reviewer := range reviewers {
		for _, lens := range contractReviewLenses {
			path := contractReviewerLensPromptPath(runPaths, reviewer.Name, lens)
			if err := activeStore.WriteBytes(path, []byte(renderContractReviewerPrompt(contract, lens)), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func contractReviewerLensPromptArtifact(member string, lens contractReviewLens) string {
	return fmt.Sprintf("contract/reviewer/prompt-%s-%s.md", member, lens.Key)
}

func contractReviewerLensCorrectivePromptArtifact(member string, lens contractReviewLens) string {
	return fmt.Sprintf("contract/reviewer/prompt-%s-%s-corrective.md", member, lens.Key)
}

func contractReviewerLensPromptPath(runPaths contractRunPathSet, member string, lens contractReviewLens) string {
	return filepath.Join(runPaths.ContractReviewDir, fmt.Sprintf("prompt-%s-%s.md", member, lens.Key))
}

func contractReviewerLensCorrectivePromptPath(runPaths contractRunPathSet, member string, lens contractReviewLens) string {
	return filepath.Join(runPaths.ContractReviewDir, fmt.Sprintf("prompt-%s-%s-corrective.md", member, lens.Key))
}

func renderContractReviewerPrompt(contract draftContract, lens contractReviewLens) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Contract Review: %s\n\n", lens.Heading)
	fmt.Fprintf(&b, "You are reviewing a software change contract through the **%s** lens.\n\n", lens.Focus)
	fmt.Fprintln(&b, "Review the contract fields below using only your assigned lens checklist.")
	fmt.Fprintln(&b, "Do not flag issues that belong to other lenses.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Contract")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "**Goal**: %s\n\n", valueOrNone(contract.Goal))
	writeMarkdownStringList(&b, "**Scope in**:", contract.Scope.In)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Scope out**:", contract.Scope.Out)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Acceptance criteria**:", contract.AcceptanceCriteria)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Validation commands**:", contract.Validation.Commands)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Assumptions**:", contract.Assumptions)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "## Lens: %s\n\n", lens.Heading)
	fmt.Fprintln(&b, "Checklist:")
	for _, item := range lens.Checklist {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	fmt.Fprintln(&b)
	writeContractReviewerOutputFormat(&b)
	return b.String()
}

func writeContractReviewerOutputFormat(b *strings.Builder) {
	fmt.Fprintln(b, "## Output")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "Report likely-real defects (recall-first), then gate on precision before marking blocking.")
	fmt.Fprintln(b, "Use state=candidate with explicit uncertainty when you believe a finding is real but have not fully confirmed it.")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "State your analysis in prose. You MUST also include exactly one structured findings block:")
	fmt.Fprintln(b, "The block is mandatory — even when you have no findings, emit `\"findings\": []`.")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "```json")
	fmt.Fprintln(b, "{")
	fmt.Fprintf(b, "  \"schema\": %q,\n", contractReviewerResultSchema)
	fmt.Fprintln(b, `  "findings": [`)
	fmt.Fprintln(b, "    {")
	fmt.Fprintln(b, `      "message": "Describe the contract issue clearly.",`)
	fmt.Fprintln(b, `      "severity": "medium",`)
	fmt.Fprintln(b, `      "category": "quality",`)
	fmt.Fprintln(b, `      "blocking": true,`)
	fmt.Fprintln(b, `      "evidence": "Quote or cite the contract field that shows the issue.",`)
	fmt.Fprintln(b, `      "material_impact": "Concrete way this spec defect would make the implementation wrong, ambiguous, or stuck.",`)
	fmt.Fprintln(b, `      "fix_direction": "What the contract author should change to resolve this.",`)
	fmt.Fprintln(b, `      "uncertainty": "Any doubt about this finding — omit if confident.",`)
	fmt.Fprintln(b, `      "state": "candidate"`)
	fmt.Fprintln(b, "    }")
	fmt.Fprintln(b, "  ]")
	fmt.Fprintln(b, "}")
	fmt.Fprintln(b, "```")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "Rules:")
	fmt.Fprintf(b, "- Use severity: %s.\n", strings.Join(reviewSeverities, ", "))
	fmt.Fprintf(b, "- Use category: %s.\n", strings.Join(reviewCategories, ", "))
	fmt.Fprintln(b, "- Omit file and line (not applicable for contract review).")
	fmt.Fprintln(b, "- Set state=candidate when likely real but not fully confirmed; set state=confirmed when certain.")
	fmt.Fprintln(b, "- HARD RULE: blocking=true is allowed ONLY for a material spec defect that would make the implementation wrong, ambiguous, or stuck.")
	fmt.Fprintln(b, "- Wording, style, naming, redundancy, and completeness/thoroughness preferences MUST be blocking=false (advisory).")
	fmt.Fprintln(b, "- Every blocking finding MUST include a concrete material_impact explaining the implementation consequence.")
	fmt.Fprintln(b, "- If you cannot state a concrete material_impact, mark the finding blocking=false (advisory).")
	fmt.Fprintln(b, "- Set blocking=false for advisory issues.")
	fmt.Fprintln(b, "- If no issues, say so clearly and emit the mandatory empty findings block.")
}

func renderContractReviewerCorrectivePrompt(contract draftContract, lens contractReviewLens) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Corrective Contract Review: %s\n\n", lens.Heading)
	fmt.Fprintln(&b, "This prompt is prepared for a corrective contract reviewer attempt.")
	fmt.Fprintf(&b, "Your previous response did not include a valid `%s` JSON block.\n", contractReviewerResultSchema)
	fmt.Fprintln(&b, "Findings expressed only in prose in the previous attempt are not recoverable; re-review the contract using only your assigned lens checklist.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Contract")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "**Goal**: %s\n\n", valueOrNone(contract.Goal))
	writeMarkdownStringList(&b, "**Scope in**:", contract.Scope.In)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Scope out**:", contract.Scope.Out)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Acceptance criteria**:", contract.AcceptanceCriteria)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Validation commands**:", contract.Validation.Commands)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Assumptions**:", contract.Assumptions)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "## Lens: %s\n\n", lens.Heading)
	for _, item := range lens.Checklist {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Required structured output")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "You MUST emit exactly one fenced JSON block. If you have no findings, emit `\"findings\": []`.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", contractReviewerResultSchema)
	fmt.Fprintln(&b, `  "findings": []`)
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	return b.String()
}

func renderContractReviewFixerPrompt(contract draftContract, currentVersion string, findings []contractReviewFinding) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Contract Review Fixer Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "You are fixing a software change contract to address blocking review findings.")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Current contract version: %s\n", currentVersion)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Current Contract")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "**Goal**: %s\n\n", valueOrNone(contract.Goal))
	writeMarkdownStringList(&b, "**Scope in**:", contract.Scope.In)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Scope out**:", contract.Scope.Out)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Acceptance criteria**:", contract.AcceptanceCriteria)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Validation commands**:", contract.Validation.Commands)
	fmt.Fprintln(&b)
	writeMarkdownStringList(&b, "**Assumptions**:", contract.Assumptions)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Blocking Findings to Address")
	fmt.Fprintln(&b)
	for i, f := range findings {
		fmt.Fprintf(&b, "%d. [%s/%s] %s\n", i+1, f.Reviewer, f.Lens, f.Message)
		if f.Evidence != "" {
			fmt.Fprintf(&b, "   Evidence: %s\n", f.Evidence)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Fixer Instructions")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "- Address each blocking finding by updating the relevant contract field.")
	fmt.Fprintln(&b, "- Do NOT change the goal field — it is out of scope for the fixer.")
	fmt.Fprintln(&b, "- Only include the contract fields you are changing in the output.")
	fmt.Fprintln(&b, "- base_version must exactly match the version shown above.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Output")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Output your reasoning, then a single JSON block with the revise payload:")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", contractReviewFixerReviseSchema)
	fmt.Fprintf(&b, "  \"base_version\": %q,\n", currentVersion)
	fmt.Fprintln(&b, `  "contract": {`)
	fmt.Fprintln(&b, `    "acceptance_criteria": ["...updated criteria..."],`)
	fmt.Fprintln(&b, `    "validation": {"commands": ["...updated commands..."]}`)
	fmt.Fprintln(&b, "  }")
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Omit any contract field you are not changing. Do not include the goal field.")
	return b.String()
}

func buildContractReviewerLensPlan(runID string, member string, lens contractReviewLens, reviewer agents.AgentDescriptor, spec agents.ModelSpec) (contractReviewerAttemptPlan, error) {
	wouldRun, err := agents.BuildACPWouldRun(reviewer.Name, spec, true)
	if err != nil {
		return contractReviewerAttemptPlan{}, err
	}
	return contractReviewerAttemptPlan{
		Lens: lens.Key,
		Artifacts: contractReviewerAttemptArtifacts{
			ReviewerPrompt: contractReviewerLensPromptArtifact(member, lens),
		},
		WouldRun: wouldRun,
	}, nil
}

func (a App) runContractReviewerAttempt(stdout io.Writer, liveOutput io.Writer, runID string, context runContext, reviewer reviewLoopReviewer, lens contractReviewLens, timeout time.Duration, wallClockCap time.Duration, onFirstOutput func(), promptRepoPath string) error {
	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[contractReviewerAttemptPlan, contractReviewerRequestDocument, contractReviewerResultDocument, struct{}]{
		Stdout:          stdout,
		LiveOutput:      liveOutput,
		OnFirstOutput:   onFirstOutput,
		JSONOutput:      true,
		Root:            context.Root,
		EventsJSONL:     context.Paths.EventsJSONL,
		RunID:           runID,
		Stage:           "contract_review",
		AttemptsDir:     context.RunPaths.ContractReviewAttemptsDir,
		AttemptIDPrefix: "contract_reviewer_attempt",
		LastResultJSON:  context.RunPaths.ContractReviewLastResultJSON,
		AgentName:       reviewer.Name,
		Agent:           reviewer.Agent,
		Model:           reviewer.ModelSpec,
		PromptRepoPath:  promptRepoPath,
		ArtifactDir:     contractReviewerAttemptsArtifact,
		Timeout:         timeout,
		WallClockCap:    wallClockCap,
		ReadOnly:        true,
		StartedEvent:    "contract_reviewer_attempt_started",
		FinishedEvent:   "contract_reviewer_attempt_finished",
		ExitKind:        "contract reviewer",
		TimeoutMessage: func(d time.Duration) string {
			return fmt.Sprintf("contract reviewer process produced no output for %s", d)
		},
		Prepare: func(createdAt string) (contractReviewerAttemptPlan, error) {
			return buildContractReviewerLensPlan(runID, reviewer.Name, lens, reviewer.Agent, reviewer.ModelSpec)
		},
		BuildRequest: func(ctx agentAttemptContext[contractReviewerAttemptPlan]) (contractReviewerRequestDocument, error) {
			return contractReviewerRequestDocument{
				Schema:    contractReviewerRequestSchema,
				RunID:     runID,
				AttemptID: ctx.AttemptID,
				CreatedAt: ctx.CreatedAt,
				Reviewer:  agentDescriptorDocument(reviewer.Agent),
				Lens:      lens.Key,
				Artifacts: ctx.Prepared.Artifacts,
				WouldRun:  ctx.Prepared.WouldRun,
			}, nil
		},
		BuildResult: func(ctx agentAttemptContext[contractReviewerAttemptPlan], runResult agents.RunResult) contractReviewerResultDocument {
			return contractReviewerResultDocument{
				Schema:        contractReviewerResultSchema,
				RunID:         runID,
				AttemptID:     ctx.AttemptID,
				Reviewer:      reviewer.Agent.Name,
				Lens:          lens.Key,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result contractReviewerResultDocument) processResult {
			return result.processResult
		},
		RenderRunOnly: func(out io.Writer, request contractReviewerRequestDocument, result contractReviewerResultDocument) {
			fmt.Fprintf(out, "contract reviewer attempt %s [%s] exit code %d\n", result.AttemptID, result.Lens, result.ExitCode)
		},
	})
}

func (a App) runContractReviewerWithAgent(liveOutput io.Writer, runID string, context runContext, reviewer reviewLoopReviewer, lens contractReviewLens, timeout time.Duration, wallClockCap time.Duration, onFirstOutput func()) (contractReviewerResultDocument, error) {
	promptRepoPath := runArtifactRepoRel(runID, contractReviewerLensPromptArtifact(reviewer.Name, lens))
	return a.runContractReviewerWithPrompt(liveOutput, runID, context, reviewer, lens, timeout, wallClockCap, onFirstOutput, promptRepoPath)
}

func (a App) runContractReviewerWithPrompt(liveOutput io.Writer, runID string, context runContext, reviewer reviewLoopReviewer, lens contractReviewLens, timeout time.Duration, wallClockCap time.Duration, onFirstOutput func(), promptRepoPath string) (contractReviewerResultDocument, error) {
	var stdout bytes.Buffer
	runErr := a.runContractReviewerAttempt(&stdout, liveOutput, runID, context, reviewer, lens, timeout, wallClockCap, onFirstOutput, promptRepoPath)
	var result contractReviewerResultDocument
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			if runErr != nil {
				return contractReviewerResultDocument{}, runErr
			}
			return contractReviewerResultDocument{}, err
		}
	}
	return result, runErr
}

func (a App) runContractReviewerCorrectiveAttempt(liveOutput io.Writer, runID string, context runContext, reviewer reviewLoopReviewer, lens contractReviewLens, timeout time.Duration, wallClockCap time.Duration) (contractReviewerResultDocument, bool, []string) {
	corrPath := contractReviewerLensCorrectivePromptPath(context.RunPaths, reviewer.Name, lens)
	if err := activeStore.WriteBytes(corrPath, []byte(renderContractReviewerCorrectivePrompt(context.Contract, lens)), 0o644); err != nil {
		return contractReviewerResultDocument{}, false, []string{fmt.Sprintf("corrective prompt write failed: %v", err)}
	}
	corrRepoPath := runArtifactRepoRel(runID, contractReviewerLensCorrectivePromptArtifact(reviewer.Name, lens))

	result, err := a.runContractReviewerWithPrompt(liveOutput, runID, context, reviewer, lens, timeout, wallClockCap, nil, corrRepoPath)
	if err != nil {
		return contractReviewerResultDocument{}, false, []string{fmt.Sprintf("corrective attempt failed: %v", err)}
	}

	stdoutBytes, readErr := activeStore.ReadBytes(agentAttemptPaths(context.RunPaths.ContractReviewAttemptsDir, result.AttemptID).StdoutLog)
	if readErr != nil {
		return result, false, []string{fmt.Sprintf("corrective attempt stdout unreadable: %v", readErr)}
	}
	if len(stdoutBytes) == 0 {
		return result, false, []string{"corrective attempt produced no output — parse miss"}
	}

	blocks, warnings := parseContractReviewerFindingBlocks(string(stdoutBytes))
	if len(blocks) == 0 {
		return result, false, warnings
	}
	return result, true, nil
}

// groupContractReviewerLensTasks builds the fan-out task list grouped by
// normalized (engine, model, effort) for stagger-cache-warming, mirroring
// groupReviewerLensTasks for code review.
func groupContractReviewerLensTasks(reviewers []reviewLoopReviewer) []contractReviewerLensGroup {
	order := make([]reviewerStaggerKey, 0, len(reviewers))
	byKey := make(map[reviewerStaggerKey]*contractReviewerLensGroup, len(reviewers))
	for reviewerIndex, reviewer := range reviewers {
		key := reviewerStaggerKey{
			engine: reviewer.Agent.Name,
			model:  reviewer.ModelSpec.Model,
			effort: reviewer.ModelSpec.Effort,
		}
		group, ok := byKey[key]
		if !ok {
			order = append(order, key)
			group = &contractReviewerLensGroup{key: key, claude: reviewer.Agent.Name == agents.BuiltinClaude}
			byKey[key] = group
		}
		for lensIndex, lens := range contractReviewLenses {
			group.tasks = append(group.tasks, contractReviewerLensTask{
				reviewerIndex: reviewerIndex,
				lensIndex:     lensIndex,
				reviewer:      reviewer,
				lens:          lens,
			})
		}
	}
	groups := make([]contractReviewerLensGroup, 0, len(order))
	for _, key := range order {
		groups = append(groups, *byKey[key])
	}
	return groups
}

func (a App) runContractReviewFanOut(liveOutput io.Writer, runID string, context runContext, reviewers []reviewLoopReviewer, timeout time.Duration, wallClockCap time.Duration) ([][]contractReviewerResultDocument, [][]error) {
	memberResults := make([][]contractReviewerResultDocument, len(reviewers))
	errs := make([][]error, len(reviewers))
	for i := range reviewers {
		memberResults[i] = make([]contractReviewerResultDocument, len(contractReviewLenses))
		errs[i] = make([]error, len(contractReviewLenses))
	}

	var sharedLive io.Writer = liveOutput
	if liveOutput != nil {
		sharedLive = &synchronizedWriter{w: liveOutput}
	}

	groups := groupContractReviewerLensTasks(reviewers)
	var wg sync.WaitGroup
	for _, group := range groups {
		wg.Add(1)
		go func(g contractReviewerLensGroup) {
			defer wg.Done()
			a.runContractReviewerLensGroup(sharedLive, runID, context, g, timeout, wallClockCap, memberResults, errs)
		}(group)
	}
	wg.Wait()
	return memberResults, errs
}

func (a App) runContractReviewerLensGroup(sharedLive io.Writer, runID string, context runContext, group contractReviewerLensGroup, timeout time.Duration, wallClockCap time.Duration, memberResults [][]contractReviewerResultDocument, errs [][]error) {
	runTask := func(task contractReviewerLensTask, onFirstOutput func()) {
		result, err := a.runContractReviewerWithAgent(sharedLive, runID, context, task.reviewer, task.lens, timeout, wallClockCap, onFirstOutput)
		memberResults[task.reviewerIndex][task.lensIndex] = result
		errs[task.reviewerIndex][task.lensIndex] = err
	}

	if !group.staggered() {
		launchContractReviewerLensTasks(group.tasks, runTask)
		return
	}

	lead := group.tasks[0]
	held := group.tasks[1:]
	released := make(chan struct{})
	var releaseOnce sync.Once
	var releaseReason string
	release := func(reason string) {
		releaseOnce.Do(func() {
			releaseReason = reason
			close(released)
		})
	}

	if sharedLive != nil {
		fmt.Fprintf(sharedLive, "contract review stagger: holding %d %s %s attempt(s) until the lead warms the prompt cache\n", len(held), group.key.engine, reviewerStaggerModelLabel(group.key))
	}

	var leadWG sync.WaitGroup
	leadWG.Add(1)
	go func() {
		defer leadWG.Done()
		runTask(lead, func() { release("lead streamed first output") })
		release("lead finished before output")
	}()

	timer := time.NewTimer(a.reviewStaggerHoldTimeout())
	defer timer.Stop()
	select {
	case <-released:
	case <-timer.C:
		release("hold timeout elapsed")
	}

	if sharedLive != nil {
		fmt.Fprintf(sharedLive, "contract review stagger: releasing %d held %s attempt(s) (%s)\n", len(held), group.key.engine, releaseReason)
	}

	launchContractReviewerLensTasks(held, runTask)
	leadWG.Wait()
}

func launchContractReviewerLensTasks(tasks []contractReviewerLensTask, runTask func(contractReviewerLensTask, func())) {
	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(t contractReviewerLensTask) {
			defer wg.Done()
			runTask(t, nil)
		}(task)
	}
	wg.Wait()
}

func writeContractReviewLoopOutput(stdout io.Writer, response contractReviewLoopResponse) {
	fmt.Fprintln(stdout, "Contract review finished")
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Run: %s\n", response.RunID)
	fmt.Fprintf(stdout, "Reviewers: %s\n", strings.Join(response.Reviewers, ", "))
	fmt.Fprintf(stdout, "Lenses: %s\n", strings.Join(response.Lenses, ", "))
	fmt.Fprintf(stdout, "Terminal reason: %s\n", response.TerminalReason)
	if response.TerminalReason == "blockers_open" || response.TerminalReason == "fixer_no_progress" {
		fmt.Fprintf(stdout, "%d open blocking findings remain\n", response.OpenBlockingCount)
	}
	fmt.Fprintf(stdout, "Rounds: %d/%d\n", len(response.Rounds), response.MaxRounds)
	if len(response.Rounds) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Round results:")
		for _, round := range response.Rounds {
			fmt.Fprintf(stdout, "  - round %d: findings %d (blocking %d)", round.Round, len(round.Findings), round.BlockingFindings)
			if round.FixerAttemptID != "" {
				fmt.Fprintf(stdout, ", fixer %s, fixes applied=%d skipped=%d", round.FixerAttemptID, round.FixesApplied, round.FixesSkipped)
			}
			if round.CleanStreak > 0 {
				fmt.Fprintf(stdout, ", clean streak %d", round.CleanStreak)
			}
			if round.UnchangedVersionStreak > 0 {
				fmt.Fprintf(stdout, ", unchanged streak %d", round.UnchangedVersionStreak)
			}
			fmt.Fprintln(stdout)
			for _, s := range round.SkippedLenses {
				fmt.Fprintf(stdout, "    skipped: reviewer %s lens %s: %s\n", s.Reviewer, s.Lens, s.Reason)
			}
			for _, f := range round.Findings {
				blocking := ""
				if f.Blocking {
					blocking = " [blocking]"
				}
				fmt.Fprintf(stdout, "    [%s] [%s] [%s]%s %s\n", f.Reviewer, f.Lens, f.Severity, blocking, f.Message)
				if f.Evidence != "" {
					fmt.Fprintf(stdout, "      evidence: %s\n", f.Evidence)
				}
			}
		}
	}
	if len(response.Next) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Next:")
		for _, cmd := range response.Next {
			fmt.Fprintf(stdout, "  %s\n", cmd)
		}
	}
}
