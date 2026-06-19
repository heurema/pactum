package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/ledger"
)

const (
	clarifyLoopSummarySchema   = "pactum.clarify_loop_summary.v1alpha1"
	clarifyLoopSummaryArtifact = "clarify/loop-summary.json"

	clarifyLoopTerminalConverged  = "converged"
	clarifyLoopTerminalNeedsHuman = "needs_human"
	clarifyLoopTerminalMaxRounds  = "max_rounds"
)

type clarifyLoopOptions struct {
	Reviewer  string
	MaxRounds int
	// NoAuto disables auto-resolution: created questions stay open for the
	// human, so any open blocking question ends the loop as needs_human.
	NoAuto     bool
	Timeout    time.Duration
	JSONOutput bool
}

type clarifyLoopSummaryDocument struct {
	Schema         string `json:"schema"`
	RunID          string `json:"run_id"`
	RunStatus      string `json:"run_status"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	Clarifier      string `json:"clarifier,omitempty"`
	MaxRounds      int    `json:"max_rounds"`
	TerminalReason string `json:"terminal_reason"`
	Converged      bool   `json:"converged"`
	// ApprovalReset reports that some round of this loop regressed an
	// already-approved run back to clarifying (approval reset to pending) —
	// the M15.7 warning semantics flowing through the loop unchanged. It is
	// omitted (false) when the run was not approved.
	ApprovalReset bool                      `json:"approval_reset,omitempty"`
	Rounds        []clarifyLoopRoundSummary `json:"rounds"`
	Coverage      []clarifyKindCoverage     `json:"coverage"`
	Artifacts     clarifyLoopArtifacts      `json:"artifacts"`
}

type clarifyLoopArtifacts struct {
	Summary string `json:"summary"`
}

type clarifyLoopRoundSummary struct {
	Round              int      `json:"round"`
	ClarifierAttemptID string   `json:"clarifier_attempt_id"`
	QuestionsCreated   int      `json:"questions_created"`
	AutoResolved       int      `json:"auto_resolved"`
	OpenBlockingAfter  int      `json:"open_blocking_after"`
	Warnings           []string `json:"warnings,omitempty"`
}

// ClarifyLoop runs autonomous clarification rounds: a clarifier round proposes
// questions, auto-resolve (every open question whose confidence is high and
// whose recommended answer is non-empty gets that recommendation recorded as
// its answer), then the next round — until the clarification converges (no
// open blocking questions), a round makes no progress without a human, or the
// round cap is reached. With --no-auto, auto-resolution is skipped entirely.
// The loop automates only the question-and-answer churn; contract approval
// stays manual.
func (a App) ClarifyLoop(stdout io.Writer, liveOutput io.Writer, runID string, options clarifyLoopOptions) error {
	context, ok, err := a.loadClarifyContext(io.Discard, runID, options.JSONOutput)
	if err != nil || !ok {
		return err
	}
	maxRounds, err := a.resolveClarifyLoopMaxRounds(context.Paths.Config, options.MaxRounds)
	if err != nil {
		return err
	}
	options.Timeout, err = resolveIdleTimeout(options.Timeout)
	if err != nil {
		return err
	}
	clarifyConfig, err := readConfig(context.Paths.Config)
	if err != nil {
		return err
	}
	wallClockCap, err := resolveWallClockCap(clarifyConfig.WallClockCap.Duration())
	if err != nil {
		return err
	}

	startedAt := a.nowUTC()
	summary := clarifyLoopSummaryDocument{
		Schema:    clarifyLoopSummarySchema,
		RunID:     runID,
		RunStatus: context.State.Status,
		StartedAt: startedAt.Format(time.RFC3339),
		MaxRounds: maxRounds,
		Rounds:    []clarifyLoopRoundSummary{},
		Coverage:  []clarifyKindCoverage{},
		Artifacts: clarifyLoopArtifacts{
			Summary: clarifyLoopSummaryArtifact,
		},
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_loop_started", Timestamp: startedAt, RunID: runID}); err != nil {
		return err
	}

	var loopErr error
	for round := 1; round <= maxRounds; round++ {
		suggest, err := a.runClarifyLoopSuggestRound(liveOutput, runID, options.Reviewer, options.Timeout, wallClockCap)
		if err != nil {
			loopErr = err
			break
		}
		if summary.Clarifier == "" {
			summary.Clarifier = suggest.Clarifier
		}
		if suggest.ApprovalReset {
			summary.ApprovalReset = true
		}

		// Reload the run state: recording suggestions may have moved the run
		// (clarifying <-> contract_draft, approval reset).
		state, err := readContractRunState(context.RunPaths.RunJSON)
		if err != nil {
			loopErr = err
			break
		}
		context.State = state

		now := a.nowUTC()
		autoResolved := []clarificationAnswerRecord{}
		if !options.NoAuto {
			autoResolved, err = a.autoResolveClarifications(context, now)
			if err != nil {
				loopErr = err
				break
			}
		}
		// Refresh the clarification artifacts once after the round's
		// auto-resolves; a round that resolved nothing only recomputes the
		// status (suggest already refreshed when it recorded questions).
		var status clarifyStatusResponse
		if len(autoResolved) > 0 {
			refreshed, approvalReset, err := a.refreshClarificationArtifacts(context, now)
			if err != nil {
				loopErr = err
				break
			}
			if approvalReset {
				summary.ApprovalReset = true
			}
			status = refreshed
		} else {
			status, err = buildClarificationStatus(context.RunPaths, context.State)
			if err != nil {
				loopErr = err
				break
			}
			if context.State.Status == "contract_approved" && status.BlockingOpen == 0 {
				status.RunStatus = "contract_approved"
			}
		}

		summary.Rounds = append(summary.Rounds, clarifyLoopRoundSummary{
			Round:              round,
			ClarifierAttemptID: suggest.AttemptID,
			QuestionsCreated:   len(suggest.Created),
			AutoResolved:       len(autoResolved),
			OpenBlockingAfter:  status.BlockingOpen,
			Warnings:           append([]string{}, suggest.Warnings...),
		})
		summary.RunStatus = status.RunStatus
		summary.Converged = status.Converged
		summary.Coverage = status.Coverage

		if status.BlockingOpen == 0 {
			summary.TerminalReason = clarifyLoopTerminalConverged
			break
		}
		// With auto-resolution disabled, an open blocking question can only be
		// answered by the human, so further rounds cannot make progress.
		if options.NoAuto {
			summary.TerminalReason = clarifyLoopTerminalNeedsHuman
			break
		}
		// A round that created no new questions and auto-resolved nothing means
		// automation is out of moves: the open blocking questions await the human.
		if len(suggest.Created) == 0 && len(autoResolved) == 0 {
			summary.TerminalReason = clarifyLoopTerminalNeedsHuman
			break
		}
		if round == maxRounds {
			summary.TerminalReason = clarifyLoopTerminalMaxRounds
			break
		}
	}

	// Always finalize: write the summary artifact and emit the finished event even
	// when a round errored, so the run never leaves a dangling started event.
	if summary.TerminalReason == "" {
		if loopErr != nil {
			summary.TerminalReason = "error"
		} else {
			summary.TerminalReason = clarifyLoopTerminalMaxRounds
		}
	}
	finishedAt := a.nowUTC()
	summary.FinishedAt = finishedAt.Format(time.RFC3339)
	if err := writeJSON(context.RunPaths.ClarifyLoopSummaryJSON, summary); err != nil && loopErr == nil {
		loopErr = err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_loop_finished", Timestamp: finishedAt, RunID: runID}); err != nil && loopErr == nil {
		loopErr = err
	}
	if loopErr != nil {
		return loopErr
	}

	if options.JSONOutput {
		return writeJSONResponse(stdout, clarifyLoopResponse{clarifyLoopSummaryDocument: summary, Next: nextCommandsForRun(context.Paths, runID)})
	}
	writeClarifyLoopSummary(stdout, summary)
	return nil
}

// clarifyLoopResponse is the loop summary plus the next affordance; the
// summary artifact on disk stays unchanged.
type clarifyLoopResponse struct {
	clarifyLoopSummaryDocument
	Next []string `json:"next"`
}

func (a App) resolveClarifyLoopMaxRounds(configPath string, override int) (int, error) {
	config, err := readConfig(configPath)
	if err != nil {
		return 0, err
	}
	var configMax int
	if l := config.Pipeline.Clarify.Loop; l != nil {
		configMax = l.Max
	}
	return resolveReviewLoopLimit("max rounds", override, configMax, defaultConfigFile().Pipeline.Clarify.Loop.Max)
}

// runClarifyLoopSuggestRound runs one clarifier round (same prompt, artifacts,
// recording, and prompt-level dedupe via the existing-questions context) and
// parses its JSON response.
func (a App) runClarifyLoopSuggestRound(liveOutput io.Writer, runID string, reviewerName string, timeout time.Duration, wallClockCap time.Duration) (clarifierRoundResponse, error) {
	var stdout bytes.Buffer
	if err := a.runClarifierRound(&stdout, liveOutput, runID, reviewerName, timeout, wallClockCap); err != nil {
		return clarifierRoundResponse{}, err
	}
	var response clarifierRoundResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return clarifierRoundResponse{}, err
	}
	return response, nil
}

// autoResolveClarifications records the clarifier's own recommendation as the
// answer for every open question whose confidence is high and whose recommended
// answer is non-empty — answer source auto_recommended, decision source
// clarify_loop_auto. Questions with medium/low confidence or no recommendation
// stay open for the human. The safety story is downstream: contract approval
// remains manual, so the loop compresses the Q&A churn, not the human decision.
func (a App) autoResolveClarifications(context clarifyContext, now time.Time) ([]clarificationAnswerRecord, error) {
	questions, err := readClarificationQuestions(context.RunPaths.QuestionsJSONL)
	if err != nil {
		return nil, err
	}
	answers, err := readClarificationAnswers(context.RunPaths.AnswersJSONL)
	if err != nil {
		return nil, err
	}
	latestAnswers := latestAnswersByQuestion(answers)

	resolved := []clarificationAnswerRecord{}
	for _, question := range questions {
		if _, answered := latestAnswers[question.ID]; answered {
			continue
		}
		if question.Confidence != "high" || strings.TrimSpace(question.RecommendedAnswer) == "" {
			continue
		}
		// A question blocked on an unanswered prerequisite stays open: its
		// recommendation was formed without the answer depends_on says it needs,
		// so automation must not commit it (the human, or a later round after the
		// prerequisite resolves, decides). Questions are recorded foundational-
		// first, and latestAnswers gains this round's resolves as they happen, so
		// a prerequisite auto-resolved earlier in the same round unblocks its
		// dependents.
		blocked := false
		for _, prerequisiteID := range question.DependsOn {
			if _, prerequisiteAnswered := latestAnswers[prerequisiteID]; !prerequisiteAnswered {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		answerRecord, _, err := recordClarificationAnswer(context.RunPaths, context.State.RunID, question.ID, question.RecommendedAnswer, "auto_recommended", "clarify_loop_auto", "", now)
		if err != nil {
			return nil, err
		}
		if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_answer_recorded", Timestamp: now, RunID: context.State.RunID}); err != nil {
			return nil, err
		}
		if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_decision_recorded", Timestamp: now, RunID: context.State.RunID}); err != nil {
			return nil, err
		}
		latestAnswers[question.ID] = answerRecord
		resolved = append(resolved, answerRecord)
	}
	return resolved, nil
}

func writeClarifyLoopSummary(stdout io.Writer, summary clarifyLoopSummaryDocument) {
	fmt.Fprintln(stdout, "Clarify loop finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", summary.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", summary.RunStatus)
	fmt.Fprintf(stdout, "  terminal reason: %s\n", summary.TerminalReason)
	fmt.Fprintf(stdout, "  rounds: %d/%d\n", len(summary.Rounds), summary.MaxRounds)
	fmt.Fprintf(stdout, "  converged: %s\n", yesNo(summary.Converged))
	if summary.ApprovalReset {
		writeApprovalResetWarning(stdout, summary.RunID, summary.RunStatus)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Round results:")
	for _, round := range summary.Rounds {
		fmt.Fprintf(stdout, "  - round %d: questions created %d, auto-resolved %d, open blocking %d, clarifier %s\n", round.Round, round.QuestionsCreated, round.AutoResolved, round.OpenBlockingAfter, round.ClarifierAttemptID)
		for _, warning := range round.Warnings {
			fmt.Fprintf(stdout, "    warning: %s\n", warning)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Coverage by dimension:")
	for _, coverage := range summary.Coverage {
		fmt.Fprintf(stdout, "  - %s: total %d, answered %d, open %d, blocking open %d\n", coverage.Kind, coverage.Total, coverage.Answered, coverage.Open, coverage.BlockingOpen)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  summary: %s\n", runArtifactRepoRel(summary.RunID, clarifyLoopSummaryArtifact))
}
