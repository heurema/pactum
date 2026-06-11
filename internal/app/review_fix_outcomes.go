package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/ledger"
)

type reviewFixOutcomesBlock struct {
	Schema   string            `json:"schema"`
	Outcomes []json.RawMessage `json:"outcomes"`
}

type reviewFixOutcomeRawInput struct {
	FindingID string `json:"finding_id"`
	Outcome   string `json:"outcome"`
	Note      string `json:"note"`
}

type reviewFixOutcomeInput struct {
	FindingID string
	Outcome   string
	Note      string
}

type reviewApplyFixOutcomesResponse struct {
	RunID          string                   `json:"run_id"`
	FixerAttemptID string                   `json:"fixer_attempt_id"`
	Fixed          int                      `json:"fixed"`
	Rebutted       int                      `json:"rebutted"`
	Blocked        int                      `json:"blocked"`
	Resolutions    []reviewResolutionRecord `json:"resolutions"`
	Warnings       []string                 `json:"warnings"`
	Next           []string                 `json:"next"`
}

func (a App) ReviewApplyFixOutcomes(stdout io.Writer, runID string, fixerAttemptID string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return gateReportMissingError("apply review fix outcomes", runID)
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}
	review, err := loadOrDeriveReviewDocument(context.RunPaths, runID, gateReport.Status)
	if err != nil {
		return err
	}

	attemptID, attemptPaths, err := resolveReviewFixAttemptForOutcomes(context.RunPaths, fixerAttemptID)
	if err != nil {
		return err
	}
	stdoutBytes, err := activeStore.ReadBytes(attemptPaths.StdoutLog)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("review fix attempt stdout not found: %s", attemptID)
		}
		return err
	}

	outcomes, warnings := parseReviewFixOutcomes(string(stdoutBytes))
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	latest := latestReviewResolutions(resolutions)
	findingsByID := make(map[string]reviewFindingRecord, len(findings))
	for _, finding := range findings {
		findingsByID[finding.ID] = finding
	}

	now := a.nowUTC()
	created := make([]reviewResolutionRecord, 0)
	response := reviewApplyFixOutcomesResponse{
		RunID:          runID,
		FixerAttemptID: attemptID,
		Warnings:       warnings,
		Resolutions:    []reviewResolutionRecord{},
	}
	for _, outcome := range outcomes {
		if _, ok := findingsByID[outcome.FindingID]; !ok {
			response.Warnings = append(response.Warnings, fmt.Sprintf("fix outcome skipped: finding not found: %s", outcome.FindingID))
			continue
		}
		if _, ok := latest[outcome.FindingID]; ok {
			response.Warnings = append(response.Warnings, fmt.Sprintf("fix outcome skipped: finding already resolved: %s", outcome.FindingID))
			continue
		}

		switch outcome.Outcome {
		case "blocked":
			response.Blocked++
			continue
		case "fixed", "rebutted":
			resolution := reviewResolutionRecord{
				Schema:    reviewResolutionSchema,
				ID:        nextReviewID("r", len(resolutions)+len(created)+1),
				RunID:     runID,
				FindingID: outcome.FindingID,
				Outcome:   outcome.Outcome,
				Note:      sanitizeRepoRootInText(context.Root, strings.TrimSpace(outcome.Note)),
				CreatedAt: now.Format(time.RFC3339),
				Source:    "review_fix",
			}
			if err := appendJSONLine(context.RunPaths.ReviewResolutionsJSONL, resolution); err != nil {
				return err
			}
			created = append(created, resolution)
			latest[outcome.FindingID] = resolution
			response.Resolutions = append(response.Resolutions, resolution)
			if outcome.Outcome == "fixed" {
				response.Fixed++
			} else {
				response.Rebutted++
			}
		}
	}

	if len(created) > 0 {
		resolutions = append(resolutions, created...)
		gateStatus := review.Gate.Status
		if isRegularFile(context.RunPaths.GateReportJSON) {
			gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
			if err != nil {
				return err
			}
			gateStatus = gateReport.Status
		}
		review = refreshReviewDocument(review, runID, gateStatus, findings, resolutions, now.Format(time.RFC3339))
		if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
			return err
		}
		if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_fix_outcomes_applied", Timestamp: now, RunID: runID}); err != nil {
			return err
		}
	}

	response.Next = nextCommandsForRun(context.Paths, runID)
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeReviewFixOutcomesApplied(stdout, response)
	return nil
}

func resolveReviewFixAttemptForOutcomes(runPaths contractRunPathSet, fixerAttemptID string) (string, attemptPathSet, error) {
	if strings.TrimSpace(fixerAttemptID) != "" {
		attemptID := strings.TrimSpace(fixerAttemptID)
		paths := agentAttemptPaths(runPaths.ReviewFixAttemptsDir, attemptID)
		dirExists, err := storeDirExists(paths.Dir)
		if err != nil {
			return "", attemptPathSet{}, err
		}
		if !dirExists {
			return "", attemptPathSet{}, fmt.Errorf("review fix attempt not found: %s", attemptID)
		}
		if !isRegularFile(paths.ResultJSON) {
			return "", attemptPathSet{}, fmt.Errorf("review fix attempt is not completed: %s", attemptID)
		}
		return attemptID, paths, nil
	}

	entries, err := activeStore.ReadDir(runPaths.ReviewFixAttemptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", attemptPathSet{}, fmt.Errorf("no completed review fix attempts found")
		}
		return "", attemptPathSet{}, err
	}
	attemptIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		var number int
		if _, err := fmt.Sscanf(name, "attempt_%03d", &number); err != nil {
			continue
		}
		paths := agentAttemptPaths(runPaths.ReviewFixAttemptsDir, name)
		if isRegularFile(paths.ResultJSON) {
			attemptIDs = append(attemptIDs, name)
		}
	}
	if len(attemptIDs) == 0 {
		return "", attemptPathSet{}, fmt.Errorf("no completed review fix attempts found")
	}
	sort.Sort(sort.Reverse(sort.StringSlice(attemptIDs)))
	attemptID := attemptIDs[0]
	return attemptID, agentAttemptPaths(runPaths.ReviewFixAttemptsDir, attemptID), nil
}

func parseReviewFixOutcomes(output string) ([]reviewFixOutcomeInput, []string) {
	jsonBlocks := extractFencedJSONBlocks(agentMessageText([]byte(output)))
	warnings := []string{}
	if len(jsonBlocks) == 0 {
		return nil, []string{"structured fix outcomes block not found"}
	}

	blocks := make([]reviewFixOutcomesBlock, 0, 1)
	for _, raw := range jsonBlocks {
		var block reviewFixOutcomesBlock
		if err := json.Unmarshal([]byte(raw), &block); err != nil {
			warnings = append(warnings, "structured fix outcomes block skipped: invalid JSON")
			continue
		}
		if block.Schema != reviewFixOutcomesSchema {
			continue
		}
		blocks = append(blocks, block)
	}
	if len(blocks) == 0 {
		warnings = append(warnings, "structured fix outcomes block not found")
		return nil, warnings
	}
	if len(blocks) > 1 {
		warnings = append(warnings, "multiple structured fix outcomes blocks found; using the first")
	}

	outcomes := make([]reviewFixOutcomeInput, 0, len(blocks[0].Outcomes))
	for _, raw := range blocks[0].Outcomes {
		var rawOutcome reviewFixOutcomeRawInput
		if err := json.Unmarshal(raw, &rawOutcome); err != nil {
			warnings = append(warnings, "fix outcome skipped: invalid outcome object")
			continue
		}
		findingID := strings.TrimSpace(rawOutcome.FindingID)
		if findingID == "" {
			warnings = append(warnings, "fix outcome skipped: finding_id is required")
			continue
		}
		outcome := strings.TrimSpace(rawOutcome.Outcome)
		if outcome != "fixed" && outcome != "rebutted" && outcome != "blocked" {
			warnings = append(warnings, "fix outcome skipped: outcome must be fixed, rebutted, or blocked")
			continue
		}
		outcomes = append(outcomes, reviewFixOutcomeInput{
			FindingID: findingID,
			Outcome:   outcome,
			Note:      rawOutcome.Note,
		})
	}
	if len(outcomes) == 0 {
		warnings = append(warnings, "structured fix outcomes block contained no valid outcomes")
	}
	return outcomes, warnings
}

func writeReviewFixOutcomesApplied(stdout io.Writer, response reviewApplyFixOutcomesResponse) {
	fmt.Fprintln(stdout, "Review fix outcomes applied")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Fix attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", response.FixerAttemptID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Outcomes:")
	fmt.Fprintf(stdout, "  fixed: %d\n", response.Fixed)
	fmt.Fprintf(stdout, "  rebutted: %d\n", response.Rebutted)
	fmt.Fprintf(stdout, "  blocked: %d\n", response.Blocked)
	writeReviewProposalWarnings(stdout, response.Warnings)
}
