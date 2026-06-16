package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/heurema/pactum/internal/agents"
)

const (
	contractReviewSchema          = "pactum.contract_review.v1alpha1"
	contractReviewerRequestSchema = "pactum.contract_reviewer_request.v1alpha1"
	contractReviewerResultSchema  = "pactum.contract_reviewer_result.v1alpha1"

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
	Reviewer string `json:"reviewer"`
	Lens     string `json:"lens"`
	Severity string `json:"severity"`
	Blocking bool   `json:"blocking"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
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
}

// contractReviewLoopRoundSummary records the outcome of one loop round, including
// findings from the reviewer panel and the fixer's result (when invoked).
type contractReviewLoopRoundSummary struct {
	Round                  int                     `json:"round"`
	Findings               []contractReviewFinding `json:"findings"`
	BlockingFindings       int                     `json:"blocking_findings"`
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
	Rounds              []contractReviewLoopRoundSummary `json:"rounds"`
	Next                []string                         `json:"next"`
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

	if len(config.Contract.Reviewers) == 0 {
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

	timeout, err = resolveIdleTimeout(context.Paths.Config, timeout)
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

	return a.runContractReviewLoop(stdout, liveOutput, runID, context, reviewers, limits, lensKeys, timeout, jsonOutput)
}

// runContractReviewLoop drives the reviewer/fixer convergence loop for contract
// review. Each round runs the configured reviewer panel; blocking findings drive
// a fixer attempt via contract revise --from -; the loop converges when a clean
// round is reached, patience is exhausted (stalemate), or max rounds is hit.
func (a App) runContractReviewLoop(
	stdout, liveOutput io.Writer,
	runID string,
	context runContext,
	reviewers []reviewLoopReviewer,
	limits reviewLimits,
	lensKeys []string,
	timeout time.Duration,
	jsonOutput bool,
) error {
	cleanStreak := 0
	unchangedVersionStreak := 0
	rounds := []contractReviewLoopRoundSummary{}
	terminalReason := ""

	for round := 1; round <= limits.MaxRounds; round++ {
		// Reload context: contract may have changed due to fixer in a prior round.
		roundCtx, ok, err := a.loadContractContext(io.Discard, runID, true)
		if err != nil || !ok {
			return err
		}

		if err := activeStore.MkdirAll(roundCtx.RunPaths.ContractReviewAttemptsDir); err != nil {
			return err
		}
		if err := writeContractReviewerPrompts(roundCtx.RunPaths, roundCtx.Contract, reviewers); err != nil {
			return err
		}

		roundResult, err := a.runContractReviewRound(liveOutput, runID, roundCtx, reviewers, timeout)
		if err != nil {
			return err
		}

		blockingCount := countContractReviewBlockingFindings(roundResult.Findings)
		roundSummary := contractReviewLoopRoundSummary{
			Round:            round,
			Findings:         roundResult.Findings,
			BlockingFindings: blockingCount,
			SkippedLenses:    roundResult.SkippedLenses,
			Warnings:         roundResult.Warnings,
		}

		if blockingCount == 0 {
			// Non-blocking (advisory) findings do not block convergence; only
			// blocking findings reset the clean streak.
			cleanStreak++
			roundSummary.CleanStreak = cleanStreak
			roundSummary.UnchangedVersionStreak = unchangedVersionStreak
			rounds = append(rounds, roundSummary)
			if cleanStreak >= limits.CleanRounds {
				terminalReason = "resolved"
				break
			}
			if round == limits.MaxRounds {
				terminalReason = "max_rounds"
				break
			}
			continue
		}

		cleanStreak = 0
		roundSummary.CleanStreak = 0

		versionBefore, err := storeFileSHA256(roundCtx.RunPaths.ContractJSON)
		if err != nil {
			return err
		}

		fixerAttemptID, fixesApplied, fixesSkipped, fixerWarnings, err := a.runContractReviewFixRound(liveOutput, runID, roundCtx, roundResult.Findings, versionBefore, timeout)
		if err != nil {
			return err
		}
		roundSummary.FixerAttemptID = fixerAttemptID
		roundSummary.FixesApplied = fixesApplied
		roundSummary.FixesSkipped = fixesSkipped
		roundSummary.Warnings = append(roundSummary.Warnings, fixerWarnings...)

		versionAfter, err := storeFileSHA256(roundCtx.RunPaths.ContractJSON)
		if err != nil {
			return err
		}

		if versionAfter == versionBefore {
			unchangedVersionStreak++
		} else {
			unchangedVersionStreak = 0
		}
		roundSummary.UnchangedVersionStreak = unchangedVersionStreak
		rounds = append(rounds, roundSummary)

		if unchangedVersionStreak >= limits.Patience {
			terminalReason = "stalemate"
			break
		}
		if round == limits.MaxRounds {
			terminalReason = "max_rounds"
			break
		}
	}

	if terminalReason == "" {
		terminalReason = "max_rounds"
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
		Rounds:              rounds,
		Next:                nextCommandsForRun(context.Paths, runID),
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
) (contractReviewRoundResult, error) {
	memberResults, errs := a.runContractReviewFanOut(liveOutput, runID, context, reviewers, timeout)

	var skipped []reviewLoopSkippedLens
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
	for reviewerIndex, reviewer := range reviewers {
		for lensIndex, result := range memberResults[reviewerIndex] {
			if errs[reviewerIndex][lensIndex] != nil || result.AttemptID == "" {
				continue
			}
			attemptPaths := agentAttemptPaths(context.RunPaths.ContractReviewAttemptsDir, result.AttemptID)
			stdoutBytes, readErr := activeStore.ReadBytes(attemptPaths.StdoutLog)
			if readErr != nil {
				continue
			}
			lensKey := contractReviewLenses[lensIndex].Key
			blocks, blockWarnings := parseReviewerFindingBlocks(string(stdoutBytes))
			for _, w := range blockWarnings {
				parseWarnings = append(parseWarnings, fmt.Sprintf("%s/%s: %s", reviewer.Name, lensKey, w))
			}
			for _, block := range blocks {
				for _, rawFinding := range block.Findings {
					var input reviewerFindingProposalInput
					if err := json.Unmarshal(rawFinding, &input); err != nil {
						parseWarnings = append(parseWarnings, fmt.Sprintf("%s/%s: finding skipped: invalid JSON", reviewer.Name, lensKey))
						continue
					}
					if strings.TrimSpace(input.Message) == "" {
						continue
					}
					findings = append(findings, contractFindingFromInput(reviewer.Name, lensKey, input))
				}
			}
		}
	}

	return contractReviewRoundResult{
		Findings:      findings,
		SkippedLenses: skipped,
		Warnings:      parseWarnings,
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
) (attemptID string, applied int, skipped int, warnings []string, err error) {
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return "", 0, 0, nil, err
	}

	entry, err := resolveExecutorEntry(config, "")
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
	runErr := a.runContractReviewFixerAttempt(&fixerOut, liveOutput, runID, context, resolved, timeout)

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

func (a App) runContractReviewFixerAttempt(stdout io.Writer, liveOutput io.Writer, runID string, context runContext, resolved resolvedAgent, timeout time.Duration) error {
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

// resolveContractReviewLoopLimits reads loop limits from the review config
// section. Contract review reuses the same limits as code review.
func resolveContractReviewLoopLimits(configPath string) (reviewLimits, error) {
	config, err := readConfig(configPath)
	if err != nil {
		return reviewLimits{}, err
	}
	defaults := defaultConfigFile().Review
	maxRounds, err := resolveReviewLoopLimit("max rounds", 0, config.Review.MaxRounds, defaults.MaxRounds)
	if err != nil {
		return reviewLimits{}, err
	}
	patience, err := resolveReviewLoopLimit("patience", 0, config.Review.Patience, defaults.Patience)
	if err != nil {
		return reviewLimits{}, err
	}
	cleanRounds, err := resolveReviewLoopLimit("clean rounds", 0, config.Review.CleanRounds, defaults.CleanRounds)
	if err != nil {
		return reviewLimits{}, err
	}
	return reviewLimits{MaxRounds: maxRounds, Patience: patience, CleanRounds: cleanRounds}, nil
}

func (a App) resolveContractReviewers(config configFile) ([]reviewLoopReviewer, error) {
	reviewers := make([]reviewLoopReviewer, 0, len(config.Contract.Reviewers))
	for _, name := range config.Contract.Reviewers {
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

func contractReviewBlockingFindings(findings []contractReviewFinding) []contractReviewFinding {
	var blocking []contractReviewFinding
	for _, f := range findings {
		if f.Blocking {
			blocking = append(blocking, f)
		}
	}
	return blocking
}

func contractFindingFromInput(reviewerName string, lensKey string, input reviewerFindingProposalInput) contractReviewFinding {
	blocking := false
	if input.Blocking != nil {
		blocking = *input.Blocking
	}
	return contractReviewFinding{
		Reviewer: reviewerName,
		Lens:     lensKey,
		Severity: strings.TrimSpace(input.Severity),
		Blocking: blocking,
		Message:  strings.TrimSpace(input.Message),
		Evidence: strings.TrimSpace(input.Evidence),
	}
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

func contractReviewerLensPromptPath(runPaths contractRunPathSet, member string, lens contractReviewLens) string {
	return filepath.Join(runPaths.ContractReviewDir, fmt.Sprintf("prompt-%s-%s.md", member, lens.Key))
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
	fmt.Fprintln(b, "State your analysis in prose. If you find issues, also include a structured block:")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "```json")
	fmt.Fprintln(b, "{")
	fmt.Fprintf(b, "  \"schema\": %q,\n", reviewerFindingsSchema)
	fmt.Fprintln(b, `  "findings": [`)
	fmt.Fprintln(b, "    {")
	fmt.Fprintln(b, `      "message": "Describe the contract issue clearly.",`)
	fmt.Fprintln(b, `      "severity": "medium",`)
	fmt.Fprintln(b, `      "category": "quality",`)
	fmt.Fprintln(b, `      "blocking": true,`)
	fmt.Fprintln(b, `      "evidence": "Quote or cite the contract field that shows the issue."`)
	fmt.Fprintln(b, "    }")
	fmt.Fprintln(b, "  ]")
	fmt.Fprintln(b, "}")
	fmt.Fprintln(b, "```")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "Rules:")
	fmt.Fprintf(b, "- Use severity: %s.\n", strings.Join(reviewSeverities, ", "))
	fmt.Fprintf(b, "- Use category: %s.\n", strings.Join(reviewCategories, ", "))
	fmt.Fprintln(b, "- Omit file and line (not applicable for contract review).")
	fmt.Fprintln(b, "- Set blocking=true for defects that should block approval: gaps that make the contract unexecutable or ungatable.")
	fmt.Fprintln(b, "- Set blocking=false for advisory issues.")
	fmt.Fprintln(b, "- If no issues, say so clearly. Do not include an empty findings block.")
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

func (a App) runContractReviewerAttempt(stdout io.Writer, liveOutput io.Writer, runID string, context runContext, reviewer reviewLoopReviewer, lens contractReviewLens, timeout time.Duration, onFirstOutput func()) error {
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
		PromptRepoPath:  runArtifactRepoRel(runID, contractReviewerLensPromptArtifact(reviewer.Name, lens)),
		ArtifactDir:     contractReviewerAttemptsArtifact,
		Timeout:         timeout,
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

func (a App) runContractReviewerWithAgent(liveOutput io.Writer, runID string, context runContext, reviewer reviewLoopReviewer, lens contractReviewLens, timeout time.Duration, onFirstOutput func()) (contractReviewerResultDocument, error) {
	var stdout bytes.Buffer
	runErr := a.runContractReviewerAttempt(&stdout, liveOutput, runID, context, reviewer, lens, timeout, onFirstOutput)
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

func (a App) runContractReviewFanOut(liveOutput io.Writer, runID string, context runContext, reviewers []reviewLoopReviewer, timeout time.Duration) ([][]contractReviewerResultDocument, [][]error) {
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
			a.runContractReviewerLensGroup(sharedLive, runID, context, g, timeout, memberResults, errs)
		}(group)
	}
	wg.Wait()
	return memberResults, errs
}

func (a App) runContractReviewerLensGroup(sharedLive io.Writer, runID string, context runContext, group contractReviewerLensGroup, timeout time.Duration, memberResults [][]contractReviewerResultDocument, errs [][]error) {
	runTask := func(task contractReviewerLensTask, onFirstOutput func()) {
		result, err := a.runContractReviewerWithAgent(sharedLive, runID, context, task.reviewer, task.lens, timeout, onFirstOutput)
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
