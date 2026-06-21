package app

import (
	"bytes"
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/gitctx"
	"github.com/heurema/pactum/internal/ledger"
	"github.com/heurema/pactum/internal/loop"
)

const (
	reviewLoopSummarySchema   = "pactum.review_loop.v1alpha1"
	reviewLoopSummaryArtifact = "review/loop-summary.json"
)

// sentinel errors returned by the Step closure to signal early-exit conditions;
// loop.Run propagates them verbatim so the call site can detect them with errors.Is.
var (
	errFindingsOpen           = errors.New("findings_open")
	errReviewerUnparsed       = errors.New("reviewer_findings_unparsed")
	errGateFailed             = errors.New("gate_failed")
	errResolved               = errors.New("resolved")
	errBlockersOpen           = errors.New("blockers_open")
	errFixerNoProgress        = errors.New("fixer_no_progress")
	errPrecisionRejected      = errors.New("precision_rejected")
	errDebateNoConsensus      = errors.New("debate_no_consensus")
	errCriticVerdictsUnparsed = errors.New("critic_verdicts_unparsed")
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
	OpenBlockingCount   int                      `json:"open_blocking_count,omitempty"`
	NoProgressStreak    int                      `json:"no_progress_streak,omitempty"`
	NoProgressReason    string                   `json:"no_progress_reason,omitempty"`
	Rounds              []reviewLoopRoundSummary `json:"rounds"`
	Artifacts           reviewLoopArtifacts      `json:"artifacts"`
}

type reviewLoopArtifacts struct {
	Summary string `json:"summary"`
}

type reviewLoopRoundSummary struct {
	Round                      int                     `json:"round"`
	ReviewerAttemptID          string                  `json:"reviewer_attempt_id"`
	ReviewerAttemptIDs         []string                `json:"reviewer_attempt_ids,omitempty"`
	ReviewerAttempts           []reviewLoopAttemptRef  `json:"reviewer_attempts,omitempty"`
	ProposalsCreated           int                     `json:"proposals_created"`
	ProposalsAccepted          int                     `json:"proposals_accepted"`
	OpenFindings               int                     `json:"open_findings"`
	OpenBlockingFindings       int                     `json:"open_blocking_findings"`
	Warnings                   []string                `json:"warnings,omitempty"`
	SkippedLenses              []reviewLoopSkippedLens `json:"skipped_lenses,omitempty"`
	CleanStreak                int                     `json:"clean_streak"`
	UnchangedFingerprintStreak int                     `json:"unchanged_fingerprint_streak"`
	WorkingTreeFingerprint     string                  `json:"working_tree_fingerprint,omitempty"`
	FixerAttemptID             string                  `json:"fixer_attempt_id,omitempty"`
	FixOutcomesResolved        int                     `json:"fix_outcomes_resolved,omitempty"`
	FixOutcomesRebutted        int                     `json:"fix_outcomes_rebutted,omitempty"`
	FixOutcomesBlocked         int                     `json:"fix_outcomes_blocked,omitempty"`
	GateStatus                 string                  `json:"gate_status,omitempty"`
	GateReportArtifact         string                  `json:"gate_report_artifact,omitempty"`
	CriticAttemptID            string                  `json:"critic_attempt_id,omitempty"`
	CriticCorrectiveAttemptID  string                  `json:"critic_corrective_attempt_id,omitempty"`
	PrecisionCandidates        int                     `json:"precision_candidates"`
	PrecisionConfirmed         int                     `json:"precision_confirmed"`
	PrecisionRejected          int                     `json:"precision_rejected"`
	PrecisionUnresolved        int                     `json:"precision_unresolved"`
	CriticVerdictsArtifact     string                  `json:"critic_verdicts_artifact,omitempty"`
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
	Reviewers     []string
	AttemptIDs    []string
	Attempts      []reviewLoopAttemptRef
	Proposals     reviewLoopProposalBatch
	SkippedLenses []reviewLoopSkippedLens
}

// reviewLoopAttemptRef ties one lens attempt to the panel member (registry
// name) and lens it ran under, so the round summary surfaces the fan-out.
type reviewLoopAttemptRef struct {
	AttemptID string `json:"attempt_id"`
	Reviewer  string `json:"reviewer"`
	Lens      string `json:"lens"`
}

// reviewLoopSkippedLens records a reviewer/lens attempt that failed and was
// skipped so the round could continue with the remaining successful attempts.
type reviewLoopSkippedLens struct {
	Reviewer string `json:"reviewer"`
	Lens     string `json:"lens"`
	Reason   string `json:"reason"`
}

type reviewLoopProposalBatch struct {
	Created   []reviewProposalRecord
	Warnings  []string
	ParseMiss bool
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
	options.Timeout, err = resolveIdleTimeout(options.Timeout)
	if err != nil {
		return err
	}
	reviewConfig, err := readConfig(context.Paths.Config)
	if err != nil {
		return err
	}
	wallClockCap, err := resolveWallClockCap(reviewConfig.WallClockCap.Duration())
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
	critic, sameCriticEngineWarning, err := a.resolveCriticAgent(context, reviewConfig, reviewers)
	if err != nil {
		return err
	}
	reviewerNames := reviewLoopReviewerNames(reviewers)

	startedAt := a.nowUTC()
	summary := reviewLoopSummaryDocument{
		Schema:              reviewLoopSummarySchema,
		RunID:               runID,
		StartedAt:           startedAt.Format(time.RFC3339),
		Reviewer:            strings.Join(reviewerNames, ","),
		MaxRounds:           limits.MaxRounds,
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

	cleanStreak := 0
	unchangedFingerprintStreak := 0
	noProgressStreak := 0
	var prevBlockerKeys map[string]bool
	criticEngineWarningEmitted := false

	step := func(_ stdctx.Context, rc loop.RoundContext) (loop.RoundResult, error) {
		round := rc.Round

		reviewerResult, err := a.runReviewLoopReviewRound(context, liveOutput, runID, reviewers, options.Timeout, wallClockCap)
		if err != nil {
			return loop.RoundResult{}, err
		}
		proposals := reviewerResult.Proposals

		// ParseMiss on any lens prevents the round from being approvable — fail
		// before accepting any proposals so findings.jsonl stays consistent.
		if proposals.ParseMiss {
			cleanStreak = 0
			reviewSummaryForMiss, missErr := reviewLoopReviewSummary(context.RunPaths)
			roundSummary := reviewLoopRoundSummary{
				Round:                      round,
				ReviewerAttemptID:          firstString(reviewerResult.AttemptIDs),
				ProposalsCreated:           len(proposals.Created),
				Warnings:                   append([]string{}, proposals.Warnings...),
				SkippedLenses:              append([]reviewLoopSkippedLens{}, reviewerResult.SkippedLenses...),
				CleanStreak:                cleanStreak,
				UnchangedFingerprintStreak: unchangedFingerprintStreak,
			}
			if len(reviewerResult.AttemptIDs) > 1 {
				roundSummary.ReviewerAttemptIDs = append([]string{}, reviewerResult.AttemptIDs...)
			}
			roundSummary.ReviewerAttempts = append([]reviewLoopAttemptRef{}, reviewerResult.Attempts...)
			if missErr == nil {
				roundSummary.OpenFindings = reviewSummaryForMiss.Open
				roundSummary.OpenBlockingFindings = reviewSummaryForMiss.BlockingOpen
			}
			fingerprint, fpErr := a.reviewLoopWorkingTreeFingerprint(context)
			if fpErr == nil {
				roundSummary.WorkingTreeFingerprint = fingerprint
			}
			summary.Rounds = append(summary.Rounds, roundSummary)
			if missErr != nil {
				return loop.RoundResult{}, missErr
			}
			if fpErr != nil {
				return loop.RoundResult{}, fpErr
			}
			return loop.RoundResult{}, errReviewerUnparsed
		}

		openFindings, rebuttedFindings, err := reviewLoopDedupFindingFingerprints(context.RunPaths)
		if err != nil {
			return loop.RoundResult{}, err
		}

		// Dedup proposals: cross-batch against existing open/rebutted findings, and
		// intra-batch via batchSeen so the first occurrence of a fingerprint wins.
		//
		// Cross-batch duplicates are recorded immediately (the findingID is already known).
		// Intra-batch duplicates are deferred until after the accept loop: the "winner"
		// proposal's findingID is only known once the critic confirms and accepts it.
		type deferredIntraBatchDup struct {
			proposalID  string
			fingerprint reviewFindingFingerprint
			proposal    reviewProposalRecord
		}
		batchSeen := make(map[reviewFindingFingerprint]bool)
		var nonDuplicates []reviewProposalRecord
		var intraBatchDups []deferredIntraBatchDup
		accepted := 0
		duplicates := 0
		for _, proposal := range proposals.Created {
			fingerprint := fingerprintReviewFinding(proposal.findingCore)
			if existingFinding, ok := openFindings[fingerprint]; ok {
				if err := a.recordDuplicateReviewLoopProposal(context, proposal.ID, existingFinding.ID, "matches currently open finding"); err != nil {
					return loop.RoundResult{}, err
				}
				upgradedFinding, upgraded, err := a.upgradeDuplicateReviewFindingSeverity(context, existingFinding, proposal)
				if err != nil {
					return loop.RoundResult{}, err
				}
				if upgraded {
					openFindings[fingerprint] = upgradedFinding
				}
				duplicates++
				continue
			}
			if existingFinding, ok := rebuttedFindings[fingerprint]; ok {
				if err := a.recordDuplicateReviewLoopProposal(context, proposal.ID, existingFinding.ID, "matches rebutted finding"); err != nil {
					return loop.RoundResult{}, err
				}
				duplicates++
				continue
			}
			if batchSeen[fingerprint] {
				// Defer recording: the winning proposal for this fingerprint may not be
				// accepted yet (pending critic). Record after the accept loop so we can
				// reference the correct finding ID and maintain accepted-before-dup order.
				intraBatchDups = append(intraBatchDups, deferredIntraBatchDup{proposalID: proposal.ID, fingerprint: fingerprint, proposal: proposal})
				duplicates++
				continue
			}
			batchSeen[fingerprint] = true
			nonDuplicates = append(nonDuplicates, proposal)
		}

		// Partial roundSummary — fields that depend on post-accept state are filled below.
		roundSummary := reviewLoopRoundSummary{
			Round:             round,
			ReviewerAttemptID: firstString(reviewerResult.AttemptIDs),
			ProposalsCreated:  len(proposals.Created),
			Warnings:          append([]string{}, proposals.Warnings...),
			SkippedLenses:     append([]reviewLoopSkippedLens{}, reviewerResult.SkippedLenses...),
		}
		if len(reviewerResult.AttemptIDs) > 1 {
			roundSummary.ReviewerAttemptIDs = append([]string{}, reviewerResult.AttemptIDs...)
		}
		roundSummary.ReviewerAttempts = append([]reviewLoopAttemptRef{}, reviewerResult.Attempts...)

		// Critic pass: only when there are non-duplicate proposals to evaluate.
		if len(nonDuplicates) > 0 {
			if sameCriticEngineWarning != "" && !criticEngineWarningEmitted {
				roundSummary.Warnings = append(roundSummary.Warnings, sameCriticEngineWarning)
				criticEngineWarningEmitted = true
			}
			criticResult, err := a.runReviewCriticPass(context, liveOutput, runID, round, critic, nonDuplicates, options.Timeout, wallClockCap)
			if err != nil {
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{}, err
			}
			roundSummary.CriticAttemptID = criticResult.InitialAttemptID
			roundSummary.CriticCorrectiveAttemptID = criticResult.CorrectiveAttemptID
			roundSummary.PrecisionCandidates = criticResult.Candidates
			roundSummary.PrecisionConfirmed = criticResult.Confirmed
			roundSummary.PrecisionRejected = criticResult.Disputed
			roundSummary.PrecisionUnresolved = criticResult.Unresolved
			roundSummary.CriticVerdictsArtifact = criticResult.VerdictArtifactPath
			roundSummary.Warnings = append(roundSummary.Warnings, criticResult.Warnings...)

			if criticResult.ParseFailed {
				// Precedence: critic_verdicts_unparsed blocks approval; ensure OpenBlockingCount >= 1.
				preSummary, _ := reviewLoopReviewSummary(context.RunPaths)
				roundSummary.OpenFindings = preSummary.Open
				roundSummary.OpenBlockingFindings = preSummary.BlockingOpen
				openBlockingCount := preSummary.BlockingOpen
				if openBlockingCount < 1 {
					openBlockingCount = 1
				}
				summary.OpenBlockingCount = openBlockingCount
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{}, errCriticVerdictsUnparsed
			}

			// Pre-accept summary: reflects blocking state from prior rounds only,
			// used to determine whether critic terminal reasons apply.
			preAcceptSummary, err := reviewLoopReviewSummary(context.RunPaths)
			if err != nil {
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{}, err
			}

			// Classify blocking non-duplicate proposals by critic verdict.
			hasBlockingInsufficient := false
			hasBlockingDisputed := false
			hasBlockingConfirmed := false
			for _, proposal := range nonDuplicates {
				if !proposal.Blocking {
					continue
				}
				v := criticResult.Verdicts[proposal.ID]
				switch v.Verdict {
				case reviewCriticVerdictConfirmed:
					hasBlockingConfirmed = true
				case reviewCriticVerdictDisputed:
					hasBlockingDisputed = true
				default:
					hasBlockingInsufficient = true
				}
			}

			// Terminal reason checks — only when no prior open blocking findings exist.
			// Precedence: debate_no_consensus > precision_rejected.
			if preAcceptSummary.BlockingOpen == 0 {
				if hasBlockingInsufficient && !hasBlockingConfirmed {
					roundSummary.OpenFindings = preAcceptSummary.Open
					roundSummary.OpenBlockingFindings = 0
					summary.OpenBlockingCount = 1
					summary.Rounds = append(summary.Rounds, roundSummary)
					return loop.RoundResult{}, errDebateNoConsensus
				}
				if hasBlockingDisputed && !hasBlockingConfirmed {
					// All blocking candidates disputed, none confirmed — approvable.
					roundSummary.OpenFindings = preAcceptSummary.Open
					roundSummary.OpenBlockingFindings = 0
					summary.Rounds = append(summary.Rounds, roundSummary)
					return loop.RoundResult{}, errPrecisionRejected
				}
			}

			// Accept only critic-confirmed proposals.
			for _, proposal := range nonDuplicates {
				v := criticResult.Verdicts[proposal.ID]
				if v.Verdict != reviewCriticVerdictConfirmed {
					continue
				}
				finding, err := a.acceptReviewLoopProposal(context, proposal.ID)
				if err != nil {
					summary.Rounds = append(summary.Rounds, roundSummary)
					return loop.RoundResult{}, err
				}
				fingerprint := fingerprintReviewFinding(proposal.findingCore)
				openFindings[fingerprint] = finding
				accepted++
			}
		}

		// Write deferred intra-batch duplicate decisions now that openFindings is
		// populated with any findings accepted this round. This preserves the
		// accepted-before-duplicate decision ordering that tests rely on.
		// Also apply severity upgrades here for the same reason: in the old single-pass
		// loop the winner was accepted before the duplicate was processed, so openFindings
		// had the finding available for upgrade. We replicate that by doing it here.
		for _, dup := range intraBatchDups {
			findingID := ""
			if existingFinding, ok := openFindings[dup.fingerprint]; ok {
				findingID = existingFinding.ID
				upgradedFinding, upgraded, err := a.upgradeDuplicateReviewFindingSeverity(context, existingFinding, dup.proposal)
				if err != nil {
					return loop.RoundResult{}, err
				}
				if upgraded {
					openFindings[dup.fingerprint] = upgradedFinding
				}
			}
			if err := a.recordDuplicateReviewLoopProposal(context, dup.proposalID, findingID, "matches earlier proposal in same batch"); err != nil {
				return loop.RoundResult{}, err
			}
		}

		reviewSummaryAfterAccept, err := reviewLoopReviewSummary(context.RunPaths)
		if err != nil {
			return loop.RoundResult{}, err
		}

		roundSummary.ProposalsAccepted = accepted
		roundSummary.OpenFindings = reviewSummaryAfterAccept.Open
		roundSummary.OpenBlockingFindings = reviewSummaryAfterAccept.BlockingOpen

		// Path (3) and (4): no proposals from reviewer AND no critic-filtered proposals.
		// len(nonDuplicates) == 0 guards against the case where the critic filtered all
		// non-duplicate proposals — that round is NOT a clean round (criterion 22).
		if accepted == 0 && duplicates == 0 && len(nonDuplicates) == 0 {
			if len(proposals.Warnings) == 0 && reviewSummaryAfterAccept.BlockingOpen == 0 {
				// Path (4): clean round — no proposals, no warnings, no parse-miss, and no
				// open blocking findings from prior rounds.
				cleanStreak++
				roundSummary.CleanStreak = cleanStreak
				roundSummary.UnchangedFingerprintStreak = unchangedFingerprintStreak
				fingerprint, err := a.reviewLoopWorkingTreeFingerprint(context)
				if err != nil {
					summary.Rounds = append(summary.Rounds, roundSummary)
					return loop.RoundResult{}, err
				}
				roundSummary.WorkingTreeFingerprint = fingerprint
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{Clean: true}, nil
			}
			// Path (3b): warnings only, or no warnings but open blocking findings remain.
			// No blocking findings: no-progress round; let the loop engine handle patience.
			// Blocking findings from a prior round still open: fall through to fixer.
			cleanStreak = 0
			roundSummary.CleanStreak = cleanStreak
			if reviewSummaryAfterAccept.BlockingOpen == 0 {
				roundSummary.UnchangedFingerprintStreak = unchangedFingerprintStreak
				fingerprint, err := a.reviewLoopWorkingTreeFingerprint(context)
				if err != nil {
					summary.Rounds = append(summary.Rounds, roundSummary)
					return loop.RoundResult{}, err
				}
				roundSummary.WorkingTreeFingerprint = fingerprint
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{Clean: false, Progress: false}, nil
			}
			// Fall through to fixer path for pre-existing blocking findings.
		}

		// proposals.Created > 0 — path (1) or (2).
		cleanStreak = 0
		roundSummary.CleanStreak = cleanStreak

		// Path (2): proposals created but no blocking findings — resolve without fixer.
		if reviewSummaryAfterAccept.BlockingOpen == 0 {
			roundSummary.UnchangedFingerprintStreak = unchangedFingerprintStreak
			fingerprint, err := a.reviewLoopWorkingTreeFingerprint(context)
			if err != nil {
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{}, err
			}
			roundSummary.WorkingTreeFingerprint = fingerprint
			summary.Rounds = append(summary.Rounds, roundSummary)
			return loop.RoundResult{}, errResolved
		}

		// Path (1): blocking findings remain; try to fix unless --no-fix.
		if options.NoFix {
			roundSummary.UnchangedFingerprintStreak = unchangedFingerprintStreak
			fingerprint, err := a.reviewLoopWorkingTreeFingerprint(context)
			if err != nil {
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{}, err
			}
			roundSummary.WorkingTreeFingerprint = fingerprint
			summary.Rounds = append(summary.Rounds, roundSummary)
			return loop.RoundResult{}, errFindingsOpen
		}

		fingerprintBefore, err := a.reviewLoopWorkingTreeFingerprint(context)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			return loop.RoundResult{}, err
		}

		fixResult, err := a.runReviewLoopFixRound(liveOutput, runID, options.Agent, options.Timeout)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			return loop.RoundResult{}, err
		}
		if summary.Agent == "" {
			summary.Agent = fixResult.Fixer
		}
		roundSummary.FixerAttemptID = fixResult.AttemptID
		fixOutcomes, err := a.applyReviewLoopFixOutcomes(runID, fixResult.AttemptID)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			return loop.RoundResult{}, err
		}
		roundSummary.FixOutcomesResolved = fixOutcomes.Fixed
		roundSummary.FixOutcomesRebutted = fixOutcomes.Rebutted
		roundSummary.FixOutcomesBlocked = fixOutcomes.Blocked
		roundSummary.Warnings = append(roundSummary.Warnings, fixOutcomes.Warnings...)
		reviewSummaryAfterFix, err := reviewLoopReviewSummary(context.RunPaths)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			return loop.RoundResult{}, err
		}
		roundSummary.OpenFindings = reviewSummaryAfterFix.Open
		roundSummary.OpenBlockingFindings = reviewSummaryAfterFix.BlockingOpen
		fingerprintAfter, err := a.reviewLoopWorkingTreeFingerprint(context)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			return loop.RoundResult{}, err
		}
		roundSummary.WorkingTreeFingerprint = fingerprintAfter
		progress := fingerprintAfter != fingerprintBefore
		if progress {
			unchangedFingerprintStreak = 0
		} else {
			unchangedFingerprintStreak++
		}
		roundSummary.UnchangedFingerprintStreak = unchangedFingerprintStreak

		// Fixer no-progress escalation: if the canonical key set of open blocking
		// findings is unchanged for K=2 consecutive rounds, exit early.
		if reviewSummaryAfterFix.BlockingOpen > 0 {
			currBlockerKeys, keyErr := openBlockingFindingKeySet(context.RunPaths)
			if keyErr != nil {
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{}, keyErr
			}
			if prevBlockerKeys != nil && sameStringSet(currBlockerKeys, prevBlockerKeys) {
				noProgressStreak++
			} else {
				noProgressStreak = 0
			}
			prevBlockerKeys = currBlockerKeys
			if noProgressStreak >= 2 {
				summary.NoProgressStreak = noProgressStreak
				summary.NoProgressReason = formatStalledBlockerReason(currBlockerKeys)
				summary.OpenBlockingCount = reviewSummaryAfterFix.BlockingOpen
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{}, errFixerNoProgress
			}
		} else {
			noProgressStreak = 0
			prevBlockerKeys = nil
		}

		gateReport, err := a.runReviewLoopGate(runID)
		if err != nil {
			var gateErr gateProcessError
			if errors.As(err, &gateErr) {
				roundSummary.GateStatus = gateReport.Status
				roundSummary.GateReportArtifact = gateReportArtifact
				summary.Rounds = append(summary.Rounds, roundSummary)
				return loop.RoundResult{}, errGateFailed
			}
			summary.Rounds = append(summary.Rounds, roundSummary)
			return loop.RoundResult{}, err
		}
		roundSummary.GateStatus = gateReport.Status
		roundSummary.GateReportArtifact = gateReportArtifact
		summary.Rounds = append(summary.Rounds, roundSummary)

		// Primary success terminal: no open blocking findings after fixer outcomes.
		if reviewSummaryAfterFix.BlockingOpen == 0 {
			return loop.RoundResult{}, errResolved
		}

		return loop.RoundResult{Clean: false, Progress: progress}, nil
	}

	outcome, loopErr := loop.Run(stdctx.Background(), loop.Limits{
		Max:      limits.MaxRounds,
		Patience: limits.Patience,
		Settle:   limits.CleanRounds,
	}, step)

	switch {
	case errors.Is(loopErr, errFindingsOpen):
		summary.TerminalReason = "findings_open"
		loopErr = nil
	case errors.Is(loopErr, errReviewerUnparsed):
		summary.TerminalReason = "reviewer_findings_unparsed"
		loopErr = nil
	case errors.Is(loopErr, errGateFailed):
		summary.TerminalReason = "gate_failed"
		loopErr = nil
	case errors.Is(loopErr, errResolved):
		summary.TerminalReason = "resolved"
		loopErr = nil
	case errors.Is(loopErr, errFixerNoProgress):
		summary.TerminalReason = "fixer_no_progress"
		loopErr = nil
	case errors.Is(loopErr, errCriticVerdictsUnparsed):
		summary.TerminalReason = "critic_verdicts_unparsed"
		loopErr = nil
	case errors.Is(loopErr, errDebateNoConsensus):
		summary.TerminalReason = "debate_no_consensus"
		summary.OpenBlockingCount = 1
		loopErr = nil
	case errors.Is(loopErr, errPrecisionRejected):
		summary.TerminalReason = "precision_rejected"
		loopErr = nil
	case loopErr == nil:
		switch outcome.Reason {
		case "settled":
			summary.TerminalReason = "clean_round"
		case "stalemate", "max":
			// If blocking findings are still open, report blockers_open so callers
			// are never left with a silently non-approvable terminal reason.
			rs, rsErr := reviewLoopReviewSummary(context.RunPaths)
			if rsErr != nil {
				loopErr = rsErr
				break
			}
			if rs.BlockingOpen > 0 {
				summary.TerminalReason = "blockers_open"
				summary.OpenBlockingCount = rs.BlockingOpen
			} else if outcome.Reason == "stalemate" {
				summary.TerminalReason = "stalemate"
			} else {
				summary.TerminalReason = "max_rounds"
			}
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
	defaults := defaultConfigFile().Pipeline.CodeReview.Loop
	var configMax, configPatience, configSettle int
	if l := config.Pipeline.CodeReview.Loop; l != nil {
		configMax, configPatience, configSettle = l.Max, l.Patience, l.Settle
	}
	maxRounds, err := resolveReviewLoopLimit("max rounds", options.MaxRounds, configMax, defaults.Max)
	if err != nil {
		return reviewLimits{}, err
	}
	patience, err := resolveReviewLoopLimit("patience", options.Patience, configPatience, defaults.Patience)
	if err != nil {
		return reviewLimits{}, err
	}
	cleanRounds, err := resolveReviewLoopLimit("clean rounds", options.CleanRounds, configSettle, defaults.Settle)
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
	} else if len(config.Pipeline.CodeReview.By) > 0 {
		for _, name := range config.Pipeline.CodeReview.By {
			entry, err := findRegistryEntry(config, name)
			if err != nil {
				return nil, err
			}
			roster = append(roster, entry)
		}
	} else {
		// Empty panel: cross-model default — a single reviewer whose underlying
		// agent differs from the run executor's when the registry has one.
		entry, err := resolveReviewerEntry(config, nil, context, "")
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

func (a App) runReviewLoopReviewRound(context reviewContext, liveOutput io.Writer, runID string, reviewers []reviewLoopReviewer, timeout time.Duration, wallClockCap time.Duration) (reviewLoopReviewRoundResult, error) {
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
	errs := make([][]error, len(reviewers))
	for index := range reviewers {
		memberResults[index] = make([]reviewerResultDocument, len(reviewLenses))
		errs[index] = make([]error, len(reviewLenses))
	}
	var sharedLive io.Writer = liveOutput
	if liveOutput != nil {
		sharedLive = &synchronizedWriter{w: liveOutput}
	}

	a.runReviewerLensFanOut(sharedLive, runID, prep, reviewers, timeout, wallClockCap, memberResults, errs)

	// Collect failed lens attempts. If all fail, surface the first error. If some
	// fail, record them as skipped so the round continues with what succeeded.
	var skipped []reviewLoopSkippedLens
	var firstErr error
	var firstErrReviewer, firstErrLens string
	successCount := 0
	for reviewerIndex, reviewer := range reviewers {
		for lensIndex, err := range errs[reviewerIndex] {
			if err != nil {
				if firstErr == nil {
					firstErr = err
					firstErrReviewer = reviewer.Name
					firstErrLens = reviewLenses[lensIndex].Key
				}
				skipped = append(skipped, reviewLoopSkippedLens{
					Reviewer: reviewer.Name,
					Lens:     reviewLenses[lensIndex].Key,
					Reason:   err.Error(),
				})
			} else {
				successCount++
			}
		}
	}
	if successCount == 0 && firstErr != nil {
		return reviewLoopReviewRoundResult{}, fmt.Errorf("reviewer %s lens %s: %w", firstErrReviewer, firstErrLens, firstErr)
	}

	batch := reviewLoopProposalBatch{
		Created:  []reviewProposalRecord{},
		Warnings: []string{},
	}
	attemptIDs := make([]string, 0, len(reviewers)*len(reviewLenses))
	attempts := make([]reviewLoopAttemptRef, 0, len(reviewers)*len(reviewLenses))
	for index, reviewer := range reviewers {
		for lensIndex, result := range memberResults[index] {
			lens := reviewLenses[lensIndex]
			if errs[index][lensIndex] != nil {
				// Wall-clock-capped lens: already recorded in skipped_lenses above;
				// the attempt was killed before it could produce output so there is
				// nothing to parse — but it is NOT a parse miss (the transport made a
				// best-effort attempt and was hard-killed by the ceiling, not by a
				// protocol or format failure).
				if memberResults[index][lensIndex].WallClockTimeout {
					continue
				}
				// Hard failure: transport error. Not eligible for corrective retry.
				batch.ParseMiss = true
				batch.Warnings = append(batch.Warnings, fmt.Sprintf("lens=%s attempt=1: transport error — hard failure (parse miss): %v", lens.Key, errs[index][lensIndex]))
				continue
			}

			// Always record the attempt that ran.
			attemptIDs = append(attemptIDs, result.AttemptID)
			attempts = append(attempts, reviewLoopAttemptRef{
				AttemptID: result.AttemptID,
				Reviewer:  reviewer.Name,
				Lens:      result.Lens,
			})

			// Read the agent stdout to check for a valid findings block.
			stdoutBytes, err := activeStore.ReadBytes(reviewerAttemptPaths(prep.Context.RunPaths, result.AttemptID).StdoutLog)
			if err != nil {
				return reviewLoopReviewRoundResult{}, err
			}

			if len(stdoutBytes) == 0 {
				// Hard failure: empty stdout cannot be corrected.
				batch.ParseMiss = true
				batch.Warnings = append(batch.Warnings, fmt.Sprintf("lens=%s attempt=1: empty stdout — hard failure (parse miss)", lens.Key))
				continue
			}

			blocks, blockWarnings := parseReviewerFindingBlocks(string(stdoutBytes))
			if len(blocks) == 0 {
				// Soft failure: non-empty stdout with no valid block — one corrective retry.
				batch.Warnings = append(batch.Warnings, fmt.Sprintf("lens=%s attempt=1: no valid findings block — corrective retry", lens.Key))
				batch.Warnings = append(batch.Warnings, blockWarnings...)
				corrResult, corrOK, corrWarnings := a.runReviewerCorrectiveAttempt(liveOutput, runID, prep, reviewer, lens, timeout, wallClockCap)
				for _, w := range corrWarnings {
					batch.Warnings = append(batch.Warnings, fmt.Sprintf("lens=%s attempt=2: %s", lens.Key, w))
				}
				if corrOK {
					attemptIDs = append(attemptIDs, corrResult.AttemptID)
					attempts = append(attempts, reviewLoopAttemptRef{
						AttemptID: corrResult.AttemptID,
						Reviewer:  reviewer.Name,
						Lens:      lens.Key,
					})
					proposals, propErr := a.runReviewLoopProposeFindings(runID, corrResult.AttemptID)
					if propErr != nil {
						return reviewLoopReviewRoundResult{}, propErr
					}
					batch.Created = append(batch.Created, proposals.Created...)
					batch.Warnings = append(batch.Warnings, proposals.Warnings...)
				} else {
					batch.ParseMiss = true
				}
				continue
			}

			proposals, err := a.runReviewLoopProposeFindings(runID, result.AttemptID)
			if err != nil {
				return reviewLoopReviewRoundResult{}, err
			}
			batch.Created = append(batch.Created, proposals.Created...)
			batch.Warnings = append(batch.Warnings, proposals.Warnings...)
		}
	}
	return reviewLoopReviewRoundResult{
		Reviewers:     reviewLoopReviewerNames(reviewers),
		AttemptIDs:    attemptIDs,
		Attempts:      attempts,
		Proposals:     batch,
		SkippedLenses: skipped,
	}, nil
}

// reviewStaggerHoldTimeoutDefault is the production ceiling on how long the
// held attempts of a same-model Claude group wait for the lead to warm the
// prompt cache: a silent lead can never serialize the panel for longer.
const reviewStaggerHoldTimeoutDefault = 60 * time.Second

func (a App) reviewStaggerHoldTimeout() time.Duration {
	if a.reviewStaggerHold > 0 {
		return a.reviewStaggerHold
	}
	return reviewStaggerHoldTimeoutDefault
}

// reviewerLensTask is one (reviewer, lens) attempt of the round's fan-out,
// carrying the indices that place its result back into the per-reviewer,
// per-lens result/error grids so collection order stays independent of when the
// attempt actually ran.
type reviewerLensTask struct {
	reviewerIndex int
	lensIndex     int
	reviewer      reviewLoopReviewer
	lens          reviewLens
}

// reviewerStaggerKey groups lens attempts by the cache-relevant dimensions of
// their resolved reviewer: the inferred engine, the model, and the effort. Two
// registry names that resolve to the same Claude model and effort share one key
// (and thus one lead), because they share the same prompt-cache prefix.
type reviewerStaggerKey struct {
	engine string
	model  string
	effort string
}

// reviewerLensGroup is all the round's lens attempts that share one stagger
// key. claude marks a group whose inferred engine warms a write-premium cache;
// such a group with more than one attempt is staggered.
type reviewerLensGroup struct {
	key    reviewerStaggerKey
	claude bool
	tasks  []reviewerLensTask
}

func (g reviewerLensGroup) staggered() bool {
	return g.claude && len(g.tasks) > 1
}

// groupReviewerLensTasks builds the round's lens attempts and groups them by
// normalized (engine, model, effort). Tasks keep (reviewer index, lens index)
// order within a group, and groups keep first-appearance order, so the lead of
// a staggered group is deterministically the first reviewer's first lens.
func groupReviewerLensTasks(reviewers []reviewLoopReviewer) []reviewerLensGroup {
	order := make([]reviewerStaggerKey, 0, len(reviewers))
	byKey := make(map[reviewerStaggerKey]*reviewerLensGroup, len(reviewers))
	for reviewerIndex, reviewer := range reviewers {
		key := reviewerStaggerKey{
			engine: reviewer.Agent.Name,
			model:  reviewer.ModelSpec.Model,
			effort: reviewer.ModelSpec.Effort,
		}
		group, ok := byKey[key]
		if !ok {
			order = append(order, key)
			group = &reviewerLensGroup{key: key, claude: reviewer.Agent.Name == agents.BuiltinClaude}
			byKey[key] = group
		}
		for lensIndex, lens := range reviewLenses {
			group.tasks = append(group.tasks, reviewerLensTask{
				reviewerIndex: reviewerIndex,
				lensIndex:     lensIndex,
				reviewer:      reviewer,
				lens:          lens,
			})
		}
	}
	groups := make([]reviewerLensGroup, 0, len(order))
	for _, key := range order {
		groups = append(groups, *byKey[key])
	}
	return groups
}

// runReviewerLensFanOut launches the round's reviewer lens attempts grouped by
// normalized (engine, model, effort). Each group runs concurrently with the
// others; a staggered Claude group launches one lead first and holds the rest
// (see runReviewerLensGroup). Results and errors land in the caller's
// per-reviewer, per-lens grids, so the round's collection order is unchanged.
func (a App) runReviewerLensFanOut(sharedLive io.Writer, runID string, prep reviewerDryRunPreparation, reviewers []reviewLoopReviewer, timeout time.Duration, wallClockCap time.Duration, memberResults [][]reviewerResultDocument, errs [][]error) {
	groups := groupReviewerLensTasks(reviewers)
	var wg sync.WaitGroup
	for _, group := range groups {
		wg.Add(1)
		go func(group reviewerLensGroup) {
			defer wg.Done()
			a.runReviewerLensGroup(sharedLive, runID, prep, group, timeout, wallClockCap, memberResults, errs)
		}(group)
	}
	wg.Wait()
}

// runReviewerLensGroup runs one stagger group. A non-Claude group and a
// single-attempt group launch every attempt at once. A multi-attempt Claude
// group launches exactly one lead attempt and holds the rest until the lead
// streams its first output, finishes before producing any, or a hold timeout
// elapses — so the held attempts read the warmed prompt cache (0.1x) instead of
// each paying the 1.25x cold-write premium on the shared prefix.
func (a App) runReviewerLensGroup(sharedLive io.Writer, runID string, prep reviewerDryRunPreparation, group reviewerLensGroup, timeout time.Duration, wallClockCap time.Duration, memberResults [][]reviewerResultDocument, errs [][]error) {
	runTask := func(task reviewerLensTask, onFirstOutput func()) {
		result, err := a.runReviewLoopReviewerWithAgent(sharedLive, runID, prep, task.reviewer, task.lens, timeout, wallClockCap, onFirstOutput)
		memberResults[task.reviewerIndex][task.lensIndex] = result
		errs[task.reviewerIndex][task.lensIndex] = err
	}

	if !group.staggered() {
		launchReviewerLensTasks(group.tasks, runTask)
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
		fmt.Fprintf(sharedLive, "review stagger: holding %d %s %s attempt(s) until the lead warms the prompt cache\n", len(held), group.key.engine, reviewerStaggerModelLabel(group.key))
	}

	var leadWG sync.WaitGroup
	leadWG.Add(1)
	go func() {
		defer leadWG.Done()
		// First streamed output releases the held attempts; if the lead finishes
		// without producing any, completion releases them. release() is
		// idempotent, so both firing (output then completion) is safe.
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
		fmt.Fprintf(sharedLive, "review stagger: releasing %d held %s attempt(s) (%s)\n", len(held), group.key.engine, releaseReason)
	}

	launchReviewerLensTasks(held, runTask)
	leadWG.Wait()
}

// launchReviewerLensTasks runs every task concurrently and waits for all to
// finish; held attempts pass nil for the first-output callback.
func launchReviewerLensTasks(tasks []reviewerLensTask, runTask func(reviewerLensTask, func())) {
	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task reviewerLensTask) {
			defer wg.Done()
			runTask(task, nil)
		}(task)
	}
	wg.Wait()
}

// reviewerStaggerModelLabel renders a group's model (and effort, when pinned)
// for the held/released live-output lines.
func reviewerStaggerModelLabel(key reviewerStaggerKey) string {
	if key.effort != "" {
		return key.model + "/" + key.effort
	}
	return key.model
}

func (a App) runReviewLoopReviewerWithAgent(liveOutput io.Writer, runID string, prep reviewerDryRunPreparation, reviewer reviewLoopReviewer, lens reviewLens, timeout time.Duration, wallClockCap time.Duration, onFirstOutput func()) (reviewerResultDocument, error) {
	promptRepoPath := runArtifactRepoRel(runID, reviewerLensPromptArtifact(reviewer.Name, lens))
	return a.runReviewLoopReviewerWithPrompt(liveOutput, runID, prep, reviewer, lens, timeout, wallClockCap, onFirstOutput, promptRepoPath)
}

func (a App) runReviewLoopReviewerWithPrompt(liveOutput io.Writer, runID string, prep reviewerDryRunPreparation, reviewer reviewLoopReviewer, lens reviewLens, timeout time.Duration, wallClockCap time.Duration, onFirstOutput func(), promptRepoPath string) (reviewerResultDocument, error) {
	var stdout bytes.Buffer
	runErr := a.runReviewerAttempt(&stdout, liveOutput, runID, prep, reviewer, lens, timeout, wallClockCap, onFirstOutput, promptRepoPath)
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

func (a App) runReviewerAttempt(stdout io.Writer, liveOutput io.Writer, runID string, prep reviewerDryRunPreparation, reviewer reviewLoopReviewer, lens reviewLens, timeout time.Duration, wallClockCap time.Duration, onFirstOutput func(), promptRepoPath string) error {
	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[reviewerLensAttemptPlan, reviewerRequestDocument, reviewerResultDocument, struct{}]{
		Stdout:          stdout,
		LiveOutput:      liveOutput,
		OnFirstOutput:   onFirstOutput,
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
		PromptRepoPath:  promptRepoPath,
		ArtifactDir:     reviewerAttemptsArtifact,
		Timeout:         timeout,
		WallClockCap:    wallClockCap,
		ReadOnly:        true,
		StartedEvent:    "reviewer_attempt_started",
		FinishedEvent:   "reviewer_attempt_finished",
		ExitKind:        "reviewer",
		TimeoutMessage: func(timeout time.Duration) string {
			return fmt.Sprintf("reviewer process produced no output for %s", timeout)
		},
		Prepare: func(createdAt string) (reviewerLensAttemptPlan, error) {
			return buildReviewerLensPlan(runID, reviewer.Name, lens, reviewer.Agent, reviewer.ModelSpec)
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

// runReviewerCorrectiveAttempt writes a corrective prompt and runs one
// additional reviewer attempt. Returns (result, true, nil) when the corrective
// attempt emitted a valid block with at least one finding. Returns
// (result, false, warnings) for every failure mode: transport error, empty
// stdout, no valid block, or `"findings": []` (unverifiable clean).
func (a App) runReviewerCorrectiveAttempt(liveOutput io.Writer, runID string, prep reviewerDryRunPreparation, reviewer reviewLoopReviewer, lens reviewLens, timeout time.Duration, wallClockCap time.Duration) (reviewerResultDocument, bool, []string) {
	corrPath := filepath.Join(prep.Context.RunPaths.ReviewDir, fmt.Sprintf("reviewer-prompt-%s-%s-corrective.md", reviewer.Name, lens.Key))
	if err := activeStore.WriteBytes(corrPath, []byte(renderReviewerCorrectivePrompt(runID, lens)), 0o644); err != nil {
		return reviewerResultDocument{}, false, []string{fmt.Sprintf("corrective prompt write failed: %v", err)}
	}
	corrRepoPath := runArtifactRepoRel(runID, reviewerLensCorrectivePromptArtifact(reviewer.Name, lens))

	result, err := a.runReviewLoopReviewerWithPrompt(liveOutput, runID, prep, reviewer, lens, timeout, wallClockCap, nil, corrRepoPath)
	if err != nil {
		return reviewerResultDocument{}, false, []string{fmt.Sprintf("corrective attempt failed: %v", err)}
	}

	stdoutBytes, readErr := activeStore.ReadBytes(reviewerAttemptPaths(prep.Context.RunPaths, result.AttemptID).StdoutLog)
	if readErr != nil {
		return result, false, []string{fmt.Sprintf("corrective attempt stdout unreadable: %v", readErr)}
	}
	if len(stdoutBytes) == 0 {
		return result, false, []string{"corrective attempt produced no output — parse miss"}
	}

	blocks, warnings := parseReviewerFindingBlocks(string(stdoutBytes))
	if len(blocks) == 0 {
		return result, false, warnings
	}
	if len(*blocks[0].Findings) == 0 {
		return result, false, []string{"corrective attempt returned empty findings — unverifiable"}
	}
	return result, true, nil
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
	output, err := gitctx.Run(stdctx.Background(), root, "rev-parse", "--verify", "HEAD")
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
	if summary.TerminalReason == "blockers_open" || summary.TerminalReason == "fixer_no_progress" ||
		summary.TerminalReason == "critic_verdicts_unparsed" || summary.TerminalReason == "debate_no_consensus" {
		fmt.Fprintf(stdout, "  %d open blocking findings remain\n", summary.OpenBlockingCount)
	}
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
		for _, s := range round.SkippedLenses {
			fmt.Fprintf(stdout, "    skipped: reviewer %s lens %s: %s\n", s.Reviewer, s.Lens, s.Reason)
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

// canonicalBlockerKey normalises a finding's category and message into a
// stable key used ONLY for fixer-no-progress streak detection. The key is
// never stored or sent to agents.
func canonicalBlockerKey(category, message string) string {
	normalized := strings.ToLower(strings.TrimSpace(strings.Join(strings.Fields(message), " ")))
	return category + ":" + normalized
}

// openBlockingFindingKeySet returns the set of canonical keys for all
// currently open blocking findings (unresolved and blocking=true).
func openBlockingFindingKeySet(runPaths contractRunPathSet) (map[string]bool, error) {
	findings, resolutions, err := readReviewRecords(runPaths)
	if err != nil {
		return nil, err
	}
	resolved := latestReviewResolutions(resolutions)
	keys := map[string]bool{}
	for _, f := range findings {
		if _, ok := resolved[f.ID]; ok {
			continue
		}
		if !f.Blocking {
			continue
		}
		keys[canonicalBlockerKey(f.Category, f.Message)] = true
	}
	return keys, nil
}

// sameStringSet reports whether two string sets contain exactly the same keys.
func sameStringSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// formatStalledBlockerReason returns a human-readable reason string that
// identifies the stalled canonical blocker key set for fixer_no_progress.
func formatStalledBlockerReason(keys map[string]bool) string {
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	return "stalled canonical blocker key set: " + strings.Join(sorted, "; ")
}
