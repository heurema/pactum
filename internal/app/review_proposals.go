package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/ledger"
)

const (
	reviewerFindingsSchema          = "pactum.reviewer_findings.v1"
	reviewProposalSchema            = "pactum.review_proposal.v1"
	reviewProposalDecisionSchema    = "pactum.review_proposal_decision.v1"
	reviewProposalsArtifact         = "review/proposals.jsonl"
	reviewProposalDecisionsArtifact = "review/proposal-decisions.jsonl"
)

type reviewProposalRecord struct {
	Schema            string `json:"schema"`
	ID                string `json:"id"`
	RunID             string `json:"run_id"`
	Source            string `json:"source"`
	ReviewerAttemptID string `json:"reviewer_attempt_id"`
	Message           string `json:"message"`
	Severity          string `json:"severity"`
	Category          string `json:"category"`
	File              string `json:"file,omitempty"`
	Line              int    `json:"line,omitempty"`
	Blocking          bool   `json:"blocking"`
	Evidence          string `json:"evidence,omitempty"`
	Status            string `json:"status"`
	CreatedAt         string `json:"created_at"`
}

type reviewProposalDecisionRecord struct {
	Schema     string `json:"schema"`
	ID         string `json:"id"`
	RunID      string `json:"run_id"`
	ProposalID string `json:"proposal_id"`
	Decision   string `json:"decision"`
	FindingID  string `json:"finding_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
	CreatedAt  string `json:"created_at"`
	Source     string `json:"source"`
}

type reviewProposalView struct {
	Schema            string                        `json:"schema"`
	ID                string                        `json:"id"`
	RunID             string                        `json:"run_id"`
	Source            string                        `json:"source"`
	ReviewerAttemptID string                        `json:"reviewer_attempt_id"`
	Message           string                        `json:"message"`
	Severity          string                        `json:"severity"`
	Category          string                        `json:"category"`
	File              string                        `json:"file,omitempty"`
	Line              int                           `json:"line,omitempty"`
	Blocking          bool                          `json:"blocking"`
	Evidence          string                        `json:"evidence,omitempty"`
	Status            string                        `json:"status"`
	CreatedAt         string                        `json:"created_at"`
	LatestDecision    *reviewProposalDecisionRecord `json:"latest_decision,omitempty"`
}

type reviewProposalSummary struct {
	Pending  int `json:"pending"`
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
}

type reviewerFindingBlock struct {
	Schema   string            `json:"schema"`
	Findings []json.RawMessage `json:"findings"`
}

type reviewerFindingProposalInput struct {
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	File     string `json:"file"`
	Line     *int   `json:"line"`
	Blocking *bool  `json:"blocking"`
	Evidence string `json:"evidence"`
}

type reviewProposeFindingsResponse struct {
	RunID             string                 `json:"run_id"`
	ReviewerAttemptID string                 `json:"reviewer_attempt_id"`
	Created           []reviewProposalRecord `json:"created"`
	Warnings          []string               `json:"warnings"`
}

type reviewAcceptProposalResponse struct {
	Proposal reviewProposalView           `json:"proposal"`
	Decision reviewProposalDecisionRecord `json:"decision"`
	Finding  reviewFindingRecord          `json:"finding"`
	State    reviewStateResponse          `json:"state"`
}

type reviewRejectProposalResponse struct {
	Proposal reviewProposalView           `json:"proposal"`
	Decision reviewProposalDecisionRecord `json:"decision"`
	State    reviewStateResponse          `json:"state"`
}

func (a App) ReviewProposeFindings(stdout io.Writer, runID string, reviewerAttemptID string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	if _, err := requireReviewPrepared(context.RunPaths, runID); err != nil {
		return err
	}

	attemptID, attemptPaths, err := resolveReviewerAttemptForProposals(context.RunPaths, reviewerAttemptID)
	if err != nil {
		return err
	}
	stdoutBytes, err := os.ReadFile(attemptPaths.StdoutLog)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("reviewer attempt stdout not found: %s", attemptID)
		}
		return err
	}

	proposals, _, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return err
	}
	blocks, warnings := parseReviewerFindingBlocks(string(stdoutBytes))
	now := a.nowUTC()
	created := make([]reviewProposalRecord, 0)
	for _, block := range blocks {
		for _, rawFinding := range block.Findings {
			var input reviewerFindingProposalInput
			if err := json.Unmarshal(rawFinding, &input); err != nil {
				warnings = append(warnings, "proposal skipped: invalid finding object")
				continue
			}
			proposal, warning := proposalRecordFromReviewerInput(context.Root, runID, attemptID, len(proposals)+len(created)+1, input, now)
			if warning != "" {
				warnings = append(warnings, warning)
				continue
			}
			created = append(created, proposal)
		}
	}

	response := reviewProposeFindingsResponse{
		RunID:             runID,
		ReviewerAttemptID: attemptID,
		Created:           created,
		Warnings:          warnings,
	}
	if len(created) == 0 {
		if jsonOutput {
			return writeJSONResponse(stdout, response)
		}
		writeNoReviewProposalsFound(stdout, response)
		return nil
	}

	for _, proposal := range created {
		if err := appendJSONLine(context.RunPaths.ReviewProposalsJSONL, proposal); err != nil {
			return err
		}
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_findings_proposed", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	proposals = append(proposals, created...)
	_, decisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return err
	}
	summary := summarizeReviewProposals(buildReviewProposalViews(proposals, decisions))
	writeReviewProposalsCreated(stdout, response, summary)
	return nil
}

func (a App) ReviewAcceptProposal(stdout io.Writer, runID string, proposalID string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	review, err := requireReviewPrepared(context.RunPaths, runID)
	if err != nil {
		return err
	}
	proposals, decisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return err
	}
	proposal, ok := findReviewProposal(proposals, proposalID)
	if !ok {
		return fmt.Errorf("review proposal not found: %s", proposalID)
	}
	if isProposalDecided(proposalID, decisions) {
		return fmt.Errorf("review proposal already decided: %s", proposalID)
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}

	now := a.nowUTC()
	finding := reviewFindingRecord{
		Schema:    reviewFindingSchema,
		ID:        nextReviewID("f", len(findings)+1),
		RunID:     runID,
		Message:   proposal.Message,
		Severity:  proposal.Severity,
		Category:  proposal.Category,
		File:      proposal.File,
		Line:      proposal.Line,
		Blocking:  proposal.Blocking,
		Evidence:  proposal.Evidence,
		Status:    "open",
		CreatedAt: now.Format(time.RFC3339),
		Source:    "reviewer_proposal",
	}
	decision := reviewProposalDecisionRecord{
		Schema:     reviewProposalDecisionSchema,
		ID:         nextReviewID("pd", len(decisions)+1),
		RunID:      runID,
		ProposalID: proposalID,
		Decision:   "accepted",
		FindingID:  finding.ID,
		CreatedAt:  now.Format(time.RFC3339),
		Source:     "manual",
	}

	if err := appendJSONLine(context.RunPaths.ReviewFindingsJSONL, finding); err != nil {
		return err
	}
	if err := appendJSONLine(context.RunPaths.ReviewProposalDecisionsJSONL, decision); err != nil {
		return err
	}
	findings = append(findings, finding)
	decisions = append(decisions, decision)

	resetApproval := review.Status == "approved" || review.Approval.ApprovedAt != nil || review.Approval.ApprovedBy != nil
	if resetApproval {
		review.Approval = reviewApproval{}
	}
	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, now.Format(time.RFC3339))
	if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
		return err
	}
	if resetApproval {
		if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_approval_reset", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
			return err
		}
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_proposal_accepted", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_finding_added", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	state := buildReviewStateWithProposals(review, findings, resolutions, proposals, decisions)
	view, _ := findReviewProposalView(state.Proposals, proposalID)
	response := reviewAcceptProposalResponse{Proposal: view, Decision: decision, Finding: finding, State: state}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeReviewProposalAccepted(stdout, response)
	return nil
}

func (a App) ReviewRejectProposal(stdout io.Writer, runID string, proposalID string, reason string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	review, err := requireReviewPrepared(context.RunPaths, runID)
	if err != nil {
		return err
	}
	proposals, decisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return err
	}
	if _, ok := findReviewProposal(proposals, proposalID); !ok {
		return fmt.Errorf("review proposal not found: %s", proposalID)
	}
	if isProposalDecided(proposalID, decisions) {
		return fmt.Errorf("review proposal already decided: %s", proposalID)
	}

	now := a.nowUTC()
	decision := reviewProposalDecisionRecord{
		Schema:     reviewProposalDecisionSchema,
		ID:         nextReviewID("pd", len(decisions)+1),
		RunID:      runID,
		ProposalID: proposalID,
		Decision:   "rejected",
		Reason:     sanitizeRepoRootInText(context.Root, strings.TrimSpace(reason)),
		CreatedAt:  now.Format(time.RFC3339),
		Source:     "manual",
	}
	if err := appendJSONLine(context.RunPaths.ReviewProposalDecisionsJSONL, decision); err != nil {
		return err
	}
	decisions = append(decisions, decision)
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_proposal_rejected", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	state := buildReviewStateWithProposals(review, findings, resolutions, proposals, decisions)
	view, _ := findReviewProposalView(state.Proposals, proposalID)
	response := reviewRejectProposalResponse{Proposal: view, Decision: decision, State: state}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeReviewProposalRejected(stdout, response)
	return nil
}

func readReviewProposalRecords(runPaths contractRunPathSet) ([]reviewProposalRecord, []reviewProposalDecisionRecord, error) {
	proposals, err := readJSONLines[reviewProposalRecord](runPaths.ReviewProposalsJSONL)
	if err != nil {
		return nil, nil, err
	}
	decisions, err := readJSONLines[reviewProposalDecisionRecord](runPaths.ReviewProposalDecisionsJSONL)
	if err != nil {
		return nil, nil, err
	}
	return proposals, decisions, nil
}

func resolveReviewerAttemptForProposals(runPaths contractRunPathSet, reviewerAttemptID string) (string, reviewerAttemptPathSet, error) {
	if strings.TrimSpace(reviewerAttemptID) != "" {
		attemptID := strings.TrimSpace(reviewerAttemptID)
		paths := reviewerAttemptPaths(runPaths, attemptID)
		if !isDir(paths.Dir) {
			return "", reviewerAttemptPathSet{}, fmt.Errorf("reviewer attempt not found: %s", attemptID)
		}
		if !isRegularFile(paths.ResultJSON) {
			return "", reviewerAttemptPathSet{}, fmt.Errorf("reviewer attempt is not completed: %s", attemptID)
		}
		return attemptID, paths, nil
	}

	entries, err := os.ReadDir(runPaths.ReviewAttemptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", reviewerAttemptPathSet{}, fmt.Errorf("no completed reviewer attempts found")
		}
		return "", reviewerAttemptPathSet{}, err
	}
	attemptIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		var number int
		if _, err := fmt.Sscanf(name, "reviewer_attempt_%03d", &number); err != nil {
			continue
		}
		paths := reviewerAttemptPaths(runPaths, name)
		if isRegularFile(paths.ResultJSON) {
			attemptIDs = append(attemptIDs, name)
		}
	}
	if len(attemptIDs) == 0 {
		return "", reviewerAttemptPathSet{}, fmt.Errorf("no completed reviewer attempts found")
	}
	sort.Sort(sort.Reverse(sort.StringSlice(attemptIDs)))
	attemptID := attemptIDs[0]
	return attemptID, reviewerAttemptPaths(runPaths, attemptID), nil
}

func parseReviewerFindingBlocks(output string) ([]reviewerFindingBlock, []string) {
	jsonBlocks := extractFencedJSONBlocks(output)
	blocks := make([]reviewerFindingBlock, 0, len(jsonBlocks))
	warnings := []string{}
	for _, raw := range jsonBlocks {
		var block reviewerFindingBlock
		if err := json.Unmarshal([]byte(raw), &block); err != nil {
			warnings = append(warnings, "structured proposal block skipped: invalid JSON")
			continue
		}
		if block.Schema != reviewerFindingsSchema {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks, warnings
}

func extractFencedJSONBlocks(output string) []string {
	lines := strings.Split(output, "\n")
	blocks := []string{}
	var b strings.Builder
	inJSON := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inJSON {
			if isJSONFenceStart(trimmed) {
				inJSON = true
				b.Reset()
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			blocks = append(blocks, b.String())
			inJSON = false
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return blocks
}

func isJSONFenceStart(line string) bool {
	if !strings.HasPrefix(line, "```") {
		return false
	}
	info := strings.TrimSpace(strings.TrimPrefix(line, "```"))
	if info == "" {
		return false
	}
	first, _, _ := strings.Cut(info, " ")
	return strings.EqualFold(first, "json")
}

func proposalRecordFromReviewerInput(root string, runID string, attemptID string, index int, input reviewerFindingProposalInput, now time.Time) (reviewProposalRecord, string) {
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return reviewProposalRecord{}, "proposal skipped: message is required"
	}
	severity := strings.TrimSpace(input.Severity)
	if !validReviewSeverity(severity) {
		return reviewProposalRecord{}, "proposal skipped: severity must be low, medium, high, or critical"
	}
	category := strings.TrimSpace(input.Category)
	if !validReviewCategory(category) {
		return reviewProposalRecord{}, "proposal skipped: category must be correctness, scope, quality, validation, process, or other"
	}
	file := strings.TrimSpace(input.File)
	if file != "" && !isRepoRelativeReviewFile(file) {
		return reviewProposalRecord{}, "proposal skipped: file must be repo-relative"
	}
	line := 0
	if input.Line != nil {
		if *input.Line < 0 {
			return reviewProposalRecord{}, "proposal skipped: line must be >= 0"
		}
		line = *input.Line
	}
	blocking := false
	if input.Blocking != nil {
		blocking = *input.Blocking
	}
	return reviewProposalRecord{
		Schema:            reviewProposalSchema,
		ID:                nextReviewID("p", index),
		RunID:             runID,
		Source:            "reviewer_attempt",
		ReviewerAttemptID: attemptID,
		Message:           sanitizeRepoRootInText(root, message),
		Severity:          severity,
		Category:          category,
		File:              filepath.ToSlash(file),
		Line:              line,
		Blocking:          blocking,
		Evidence:          sanitizeRepoRootInText(root, strings.TrimSpace(input.Evidence)),
		Status:            "pending",
		CreatedAt:         now.Format(time.RFC3339),
	}, ""
}

func validReviewSeverity(value string) bool {
	switch value {
	case "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func validReviewCategory(value string) bool {
	switch value {
	case "correctness", "scope", "quality", "validation", "process", "other":
		return true
	default:
		return false
	}
}

func isRepoRelativeReviewFile(file string) bool {
	if file == "" {
		return true
	}
	if isAbsolutePathLike(file) {
		return false
	}
	cleaned := path.Clean(filepath.ToSlash(file))
	return cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

func isAbsolutePathLike(file string) bool {
	if filepath.IsAbs(file) || strings.HasPrefix(filepath.ToSlash(file), "/") || strings.HasPrefix(file, `\\`) {
		return true
	}
	if len(file) >= 3 && file[1] == ':' && (file[2] == '\\' || file[2] == '/') {
		ch := file[0]
		return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
	}
	return false
}

func sanitizeRepoRootInText(root string, value string) string {
	if strings.TrimSpace(root) == "" || value == "" {
		return value
	}
	sanitized := strings.ReplaceAll(value, root, ".")
	slashRoot := filepath.ToSlash(root)
	if slashRoot != root {
		sanitized = strings.ReplaceAll(sanitized, slashRoot, ".")
	}
	return sanitized
}

func buildReviewProposalViews(proposals []reviewProposalRecord, decisions []reviewProposalDecisionRecord) []reviewProposalView {
	latest := latestReviewProposalDecisions(decisions)
	views := make([]reviewProposalView, 0, len(proposals))
	for _, proposal := range proposals {
		status := proposal.Status
		if status == "" {
			status = "pending"
		}
		view := reviewProposalView{
			Schema:            proposal.Schema,
			ID:                proposal.ID,
			RunID:             proposal.RunID,
			Source:            proposal.Source,
			ReviewerAttemptID: proposal.ReviewerAttemptID,
			Message:           proposal.Message,
			Severity:          proposal.Severity,
			Category:          proposal.Category,
			File:              proposal.File,
			Line:              proposal.Line,
			Blocking:          proposal.Blocking,
			Evidence:          proposal.Evidence,
			Status:            status,
			CreatedAt:         proposal.CreatedAt,
		}
		if decision, ok := latest[proposal.ID]; ok {
			view.Status = decision.Decision
			decisionCopy := decision
			view.LatestDecision = &decisionCopy
		}
		views = append(views, view)
	}
	return views
}

func latestReviewProposalDecisions(decisions []reviewProposalDecisionRecord) map[string]reviewProposalDecisionRecord {
	latest := make(map[string]reviewProposalDecisionRecord, len(decisions))
	for _, decision := range decisions {
		latest[decision.ProposalID] = decision
	}
	return latest
}

func summarizeReviewProposals(proposals []reviewProposalView) reviewProposalSummary {
	var summary reviewProposalSummary
	for _, proposal := range proposals {
		switch proposal.Status {
		case "accepted":
			summary.Accepted++
		case "rejected":
			summary.Rejected++
		default:
			summary.Pending++
		}
	}
	return summary
}

func isProposalDecided(proposalID string, decisions []reviewProposalDecisionRecord) bool {
	if decision, ok := latestReviewProposalDecisions(decisions)[proposalID]; ok {
		return decision.Decision == "accepted" || decision.Decision == "rejected"
	}
	return false
}

func findReviewProposal(proposals []reviewProposalRecord, proposalID string) (reviewProposalRecord, bool) {
	for _, proposal := range proposals {
		if proposal.ID == proposalID {
			return proposal, true
		}
	}
	return reviewProposalRecord{}, false
}

func findReviewProposalView(proposals []reviewProposalView, proposalID string) (reviewProposalView, bool) {
	for _, proposal := range proposals {
		if proposal.ID == proposalID {
			return proposal, true
		}
	}
	return reviewProposalView{}, false
}

func writeNoReviewProposalsFound(stdout io.Writer, response reviewProposeFindingsResponse) {
	fmt.Fprintln(stdout, "No structured reviewer finding proposals found.")
	writeReviewProposalWarnings(stdout, response.Warnings)
}

func writeReviewProposalsCreated(stdout io.Writer, response reviewProposeFindingsResponse, summary reviewProposalSummary) {
	fmt.Fprintln(stdout, "Review finding proposals created")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Reviewer attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", response.ReviewerAttemptID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Proposals:")
	fmt.Fprintf(stdout, "  created: %d\n", len(response.Created))
	fmt.Fprintf(stdout, "  pending: %d\n", summary.Pending)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  proposals: %s\n", runArtifactRepoRel(response.RunID, reviewProposalsArtifact))
	writeReviewProposalWarnings(stdout, response.Warnings)
}

func writeReviewProposalWarnings(stdout io.Writer, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Warnings:")
	for _, warning := range warnings {
		fmt.Fprintf(stdout, "  - %s\n", warning)
	}
}

func writeReviewPendingProposals(stdout io.Writer, proposals []reviewProposalView) {
	fmt.Fprintln(stdout, "Proposals:")
	wrote := false
	for _, proposal := range proposals {
		if proposal.Status != "pending" {
			continue
		}
		wrote = true
		blocking := ""
		if proposal.Blocking {
			blocking = " [blocking]"
		}
		fmt.Fprintf(stdout, "  - %s [%s]%s %s: %s\n", proposal.ID, proposal.Severity, blocking, proposal.Category, proposal.Message)
		if proposal.File != "" {
			location := proposal.File
			if proposal.Line > 0 {
				location = fmt.Sprintf("%s:%d", location, proposal.Line)
			}
			fmt.Fprintf(stdout, "    location: %s\n", location)
		}
		fmt.Fprintf(stdout, "    status: %s\n", proposal.Status)
		if proposal.Evidence != "" {
			fmt.Fprintf(stdout, "    evidence: %s\n", proposal.Evidence)
		}
	}
	if !wrote {
		fmt.Fprintln(stdout, "  none")
	}
}

func writeReviewProposalAccepted(stdout io.Writer, response reviewAcceptProposalResponse) {
	fmt.Fprintln(stdout, "Review proposal accepted")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Finding.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Proposal:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Proposal.ID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Finding:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Finding.ID)
	fmt.Fprintln(stdout, "  status: open")
}

func writeReviewProposalRejected(stdout io.Writer, response reviewRejectProposalResponse) {
	fmt.Fprintln(stdout, "Review proposal rejected")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Decision.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Proposal:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Proposal.ID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Reason:")
	fmt.Fprintf(stdout, "  %s\n", valueOrNone(response.Decision.Reason))
}
