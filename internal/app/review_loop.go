package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/heurema/pactum/internal/ledger"
)

const (
	reviewLoopSummarySchema   = "pactum.review_loop.v1"
	reviewLoopSummaryArtifact = "review/loop-summary.json"
)

type reviewLoopOptions struct {
	Reviewer   string
	Agent      string
	MaxRounds  int
	Timeout    time.Duration
	Yes        bool
	JSONOutput bool
}

type reviewLoopSummaryDocument struct {
	Schema         string                   `json:"schema"`
	RunID          string                   `json:"run_id"`
	StartedAt      string                   `json:"started_at"`
	FinishedAt     string                   `json:"finished_at"`
	Reviewer       string                   `json:"reviewer,omitempty"`
	Agent          string                   `json:"agent,omitempty"`
	MaxRounds      int                      `json:"max_rounds"`
	TerminalReason string                   `json:"terminal_reason"`
	Rounds         []reviewLoopRoundSummary `json:"rounds"`
	Artifacts      reviewLoopArtifacts      `json:"artifacts"`
}

type reviewLoopArtifacts struct {
	Summary string `json:"summary"`
}

type reviewLoopRoundSummary struct {
	Round              int      `json:"round"`
	ReviewerAttemptID  string   `json:"reviewer_attempt_id"`
	ProposalsCreated   int      `json:"proposals_created"`
	ProposalsAccepted  int      `json:"proposals_accepted"`
	OpenFindings       int      `json:"open_findings"`
	TotalOpenFindings  int      `json:"total_open_findings"`
	Warnings           []string `json:"warnings,omitempty"`
	FixerAttemptID     string   `json:"fixer_attempt_id,omitempty"`
	GateStatus         string   `json:"gate_status,omitempty"`
	GateReportArtifact string   `json:"gate_report_artifact,omitempty"`
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
	maxRounds, err := a.resolveReviewLoopMaxRounds(context, options.MaxRounds)
	if err != nil {
		return err
	}

	startedAt := a.nowUTC()
	summary := reviewLoopSummaryDocument{
		Schema:    reviewLoopSummarySchema,
		RunID:     runID,
		StartedAt: startedAt.Format(time.RFC3339),
		MaxRounds: maxRounds,
		Rounds:    []reviewLoopRoundSummary{},
		Artifacts: reviewLoopArtifacts{
			Summary: reviewLoopSummaryArtifact,
		},
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_loop_started", Timestamp: startedAt, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	var loopErr error
	for round := 1; round <= maxRounds; round++ {
		reviewerResult, proposals, err := a.runReviewLoopReviewRound(liveOutput, runID, options.Reviewer, options.Timeout)
		if err != nil {
			loopErr = err
			break
		}
		if summary.Reviewer == "" {
			summary.Reviewer = reviewerResult.Reviewer
		}

		accepted := 0
		for _, proposal := range proposals.Created {
			if err := a.acceptReviewLoopProposal(runID, proposal.ID); err != nil {
				loopErr = err
				break
			}
			accepted++
		}
		if loopErr != nil {
			break
		}
		totalOpen, err := reviewLoopTotalOpenFindings(context.RunPaths)
		if err != nil {
			loopErr = err
			break
		}

		roundSummary := reviewLoopRoundSummary{
			Round:             round,
			ReviewerAttemptID: reviewerResult.AttemptID,
			ProposalsCreated:  len(proposals.Created),
			ProposalsAccepted: accepted,
			OpenFindings:      accepted,
			TotalOpenFindings: totalOpen,
			Warnings:          append([]string{}, proposals.Warnings...),
		}
		// A round that accepts no proposals ends the loop, but only a round that
		// reported NOTHING is a clean pass. If the reviewer reported findings that
		// could not be parsed into proposals (warnings), the code was not actually
		// cleared — terminate with a distinct, non-pass reason so real findings are
		// never silently dropped.
		if accepted == 0 {
			summary.Rounds = append(summary.Rounds, roundSummary)
			if len(proposals.Warnings) > 0 {
				summary.TerminalReason = "reviewer_findings_unparsed"
			} else {
				summary.TerminalReason = "clean_round"
			}
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

		gateReport, err := a.runReviewLoopGate(runID)
		if err != nil {
			summary.Rounds = append(summary.Rounds, roundSummary)
			loopErr = err
			break
		}
		roundSummary.GateStatus = gateReport.Status
		roundSummary.GateReportArtifact = gateReportArtifact
		summary.Rounds = append(summary.Rounds, roundSummary)

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
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_loop_finished", Timestamp: finishedAt, RunID: runID, RepoRoot: context.Root}); err != nil && loopErr == nil {
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

func (a App) resolveReviewLoopMaxRounds(context reviewContext, override int) (int, error) {
	if override < 0 {
		return 0, fmt.Errorf("max rounds must be positive")
	}
	if override > 0 {
		return override, nil
	}
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return 0, err
	}
	maxRounds := config.Limits.Review.MaxRounds
	if maxRounds <= 0 {
		maxRounds = defaultConfigFile().Limits.Review.MaxRounds
	}
	if maxRounds <= 0 {
		return 0, fmt.Errorf("review max rounds must be positive")
	}
	return maxRounds, nil
}

func (a App) runReviewLoopReviewRound(liveOutput io.Writer, runID string, reviewer string, timeout time.Duration) (reviewerResultDocument, reviewProposeFindingsResponse, error) {
	reviewerResult, err := a.runReviewLoopReviewer(liveOutput, runID, reviewer, timeout)
	if err != nil {
		return reviewerResultDocument{}, reviewProposeFindingsResponse{}, err
	}
	proposals, err := a.runReviewLoopProposeFindings(runID, reviewerResult.AttemptID)
	if err != nil {
		return reviewerResultDocument{}, reviewProposeFindingsResponse{}, err
	}
	return reviewerResult, proposals, nil
}

func (a App) runReviewLoopReviewer(liveOutput io.Writer, runID string, reviewer string, timeout time.Duration) (reviewerResultDocument, error) {
	var stdout bytes.Buffer
	if err := a.ReviewRun(&stdout, liveOutput, runID, reviewer, timeout, true, true); err != nil {
		return reviewerResultDocument{}, err
	}
	var result reviewerResultDocument
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return reviewerResultDocument{}, err
	}
	return result, nil
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

func (a App) acceptReviewLoopProposal(runID string, proposalID string) error {
	return a.ReviewAcceptProposal(io.Discard, runID, proposalID, false)
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

func (a App) runReviewLoopGate(runID string) (gateReportDocument, error) {
	var stdout bytes.Buffer
	if err := a.GateRun(&stdout, runID, true, true); err != nil {
		return gateReportDocument{}, err
	}
	var report gateReportDocument
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		return gateReportDocument{}, err
	}
	return report, nil
}

func reviewLoopTotalOpenFindings(runPaths contractRunPathSet) (int, error) {
	findings, resolutions, err := readReviewRecords(runPaths)
	if err != nil {
		return 0, err
	}
	return summarizeReview(findings, resolutions).Open, nil
}

func writeReviewLoopSummary(stdout io.Writer, summary reviewLoopSummaryDocument) {
	fmt.Fprintln(stdout, "Review loop finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", summary.RunID)
	fmt.Fprintf(stdout, "  terminal reason: %s\n", summary.TerminalReason)
	fmt.Fprintf(stdout, "  rounds: %d/%d\n", len(summary.Rounds), summary.MaxRounds)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Round results:")
	for _, round := range summary.Rounds {
		fmt.Fprintf(stdout, "  - round %d: open findings %d, proposals accepted %d, reviewer %s", round.Round, round.OpenFindings, round.ProposalsAccepted, round.ReviewerAttemptID)
		if round.FixerAttemptID != "" {
			fmt.Fprintf(stdout, ", fixer %s", round.FixerAttemptID)
		}
		if round.GateStatus != "" {
			fmt.Fprintf(stdout, ", gate %s", round.GateStatus)
		}
		fmt.Fprintln(stdout)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  summary: %s\n", runArtifactRepoRel(summary.RunID, reviewLoopSummaryArtifact))
}
