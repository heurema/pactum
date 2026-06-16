package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/ledger"
)

const (
	reviewerFindingsSchema          = "pactum.reviewer_findings.v1alpha1"
	reviewProposalSchema            = "pactum.review_proposal.v1alpha1"
	reviewProposalDecisionSchema    = "pactum.review_proposal_decision.v1alpha1"
	reviewProposalsArtifact         = "review/proposals.jsonl"
	reviewProposalDecisionsArtifact = "review/proposal-decisions.jsonl"
)

type reviewProposalRecord struct {
	Schema            string `json:"schema"`
	ID                string `json:"id"`
	RunID             string `json:"run_id"`
	Source            string `json:"source"`
	ReviewerAttemptID string `json:"reviewer_attempt_id"`
	findingCore
	Evidence  string `json:"evidence,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
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
	// DecidedBy is the explicit CLI principal (--by). Automatic loop decisions
	// record only Source as their provenance and omit it.
	DecidedBy string `json:"decided_by,omitempty"`
}

type reviewProposalView struct {
	reviewProposalRecord
	LatestDecision *reviewProposalDecisionRecord `json:"latest_decision,omitempty"`
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
	Message    string `json:"message"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	File       string `json:"file"`
	Line       *int   `json:"line"`
	Blocking   *bool  `json:"blocking"`
	Confidence string `json:"confidence"`
	Evidence   string `json:"evidence"`
}

type reviewProposeFindingsResponse struct {
	RunID string `json:"run_id"`
	// ReviewerAttemptIDs lists every attempt whose stdout was parsed: the
	// explicit --attempt when given, otherwise ALL completed reviewer attempts —
	// a review fans out into five lens attempts per reviewer, so the default
	// must cover the whole review, not just the latest attempt.
	ReviewerAttemptIDs []string               `json:"reviewer_attempt_ids"`
	Created            []reviewProposalRecord `json:"created"`
	Warnings           []string               `json:"warnings"`
	Next               []string               `json:"next"`
}

type reviewAcceptProposalResponse struct {
	Proposal reviewProposalView           `json:"proposal"`
	Decision reviewProposalDecisionRecord `json:"decision"`
	Finding  reviewFindingRecord          `json:"finding"`
	State    reviewStateResponse          `json:"state"`
	Next     []string                     `json:"next"`
}

type reviewRejectProposalResponse struct {
	Proposal reviewProposalView           `json:"proposal"`
	Decision reviewProposalDecisionRecord `json:"decision"`
	State    reviewStateResponse          `json:"state"`
	Next     []string                     `json:"next"`
}

func (a App) ReviewProposeFindings(stdout io.Writer, runID string, reviewerAttemptID string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}

	attemptIDs, err := resolveReviewerAttemptsForProposals(context.RunPaths, reviewerAttemptID)
	if err != nil {
		return err
	}

	proposals, _, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return err
	}
	now := a.nowUTC()
	created := make([]reviewProposalRecord, 0)
	warnings := []string{}
	for _, attemptID := range attemptIDs {
		attemptPaths := reviewerAttemptPaths(context.RunPaths, attemptID)
		stdoutBytes, err := activeStore.ReadBytes(attemptPaths.StdoutLog)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("reviewer attempt stdout not found: %s", attemptID)
			}
			return err
		}
		blocks, attemptWarnings := parseReviewerFindingBlocks(string(stdoutBytes))
		for _, warning := range attemptWarnings {
			warnings = append(warnings, prefixAttemptWarning(attemptIDs, attemptID, warning))
		}
		for _, block := range blocks {
			for _, rawFinding := range block.Findings {
				var input reviewerFindingProposalInput
				if err := json.Unmarshal(rawFinding, &input); err != nil {
					warnings = append(warnings, prefixAttemptWarning(attemptIDs, attemptID, "proposal skipped: invalid finding object"))
					continue
				}
				proposal, warning := proposalRecordFromReviewerInput(context.Root, runID, attemptID, len(proposals)+len(created)+1, input, now)
				if warning != "" {
					warnings = append(warnings, prefixAttemptWarning(attemptIDs, attemptID, warning))
					continue
				}
				created = append(created, proposal)
			}
		}
	}

	response := reviewProposeFindingsResponse{
		RunID:              runID,
		ReviewerAttemptIDs: attemptIDs,
		Created:            created,
		Warnings:           warnings,
	}
	if len(created) == 0 {
		response.Next = nextCommandsForRun(context.Paths, runID)
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
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_findings_proposed", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	response.Next = nextCommandsForRun(context.Paths, runID)
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

// ReviewAcceptProposal accepts a pending proposal as a review finding,
// recording the explicit CLI principal (--by) per the uniform rule.
func (a App) ReviewAcceptProposal(stdout io.Writer, runID string, proposalID string, decidedBy string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	return a.acceptReviewProposal(stdout, context, runID, proposalID, "manual", normalizePrincipal(context.Root, decidedBy), jsonOutput)
}

// acceptReviewProposal is the shared accept write path. Provenance is explicit:
// the CLI verb passes source "manual" with the normalized principal, the
// review run's auto-accept passes source "review_loop" with no principal —
// automatic decisions carry only their source.
func (a App) acceptReviewProposal(stdout io.Writer, context reviewContext, runID string, proposalID string, source string, decidedBy string, jsonOutput bool) error {
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
	review, err := loadOrDeriveReviewDocument(context.RunPaths, runID, gateReport.Status)
	if err != nil {
		return err
	}

	now := a.nowUTC()
	finding := reviewFindingRecord{
		Schema:      reviewFindingSchema,
		ID:          nextReviewID("f", len(findings)+1),
		RunID:       runID,
		findingCore: proposal.findingCore,
		Status:      "open",
		CreatedAt:   now.Format(time.RFC3339),
		Source:      "reviewer_proposal",
	}
	decision := reviewProposalDecisionRecord{
		Schema:     reviewProposalDecisionSchema,
		ID:         nextReviewID("pd", len(decisions)+1),
		RunID:      runID,
		ProposalID: proposalID,
		Decision:   "accepted",
		FindingID:  finding.ID,
		CreatedAt:  now.Format(time.RFC3339),
		Source:     source,
		DecidedBy:  decidedBy,
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
		if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_approval_reset", Timestamp: now, RunID: runID}); err != nil {
			return err
		}
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_proposal_accepted", Timestamp: now, RunID: runID}); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_finding_added", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	state := buildReviewStateWithProposals(review, findings, resolutions, proposals, decisions)
	view, _ := findReviewProposalView(state.Proposals, proposalID)
	response := reviewAcceptProposalResponse{Proposal: view, Decision: decision, Finding: finding, State: state, Next: nextCommandsForRun(context.Paths, runID)}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeReviewProposalAccepted(stdout, response)
	return nil
}

func (a App) ReviewRejectProposal(stdout io.Writer, runID string, proposalID string, reason string, decidedBy string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
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
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}
	review, err := loadOrDeriveReviewDocument(context.RunPaths, runID, gateReport.Status)
	if err != nil {
		return err
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
		DecidedBy:  normalizePrincipal(context.Root, decidedBy),
	}
	if err := appendJSONLine(context.RunPaths.ReviewProposalDecisionsJSONL, decision); err != nil {
		return err
	}
	decisions = append(decisions, decision)
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_proposal_rejected", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	state := buildReviewStateWithProposals(review, findings, resolutions, proposals, decisions)
	view, _ := findReviewProposalView(state.Proposals, proposalID)
	response := reviewRejectProposalResponse{Proposal: view, Decision: decision, State: state, Next: nextCommandsForRun(context.Paths, runID)}
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

// resolveReviewerAttemptsForProposals resolves which reviewer attempts to
// parse: the explicit attempt when given, otherwise EVERY completed attempt in
// ascending order — a review fans out into five lens attempts per reviewer, so
// defaulting to one attempt would silently cover a fraction of the review.
func resolveReviewerAttemptsForProposals(runPaths contractRunPathSet, reviewerAttemptID string) ([]string, error) {
	if strings.TrimSpace(reviewerAttemptID) != "" {
		attemptID := strings.TrimSpace(reviewerAttemptID)
		paths := reviewerAttemptPaths(runPaths, attemptID)
		dirExists, err := storeDirExists(paths.Dir)
		if err != nil {
			return nil, err
		}
		if !dirExists {
			return nil, fmt.Errorf("reviewer attempt not found: %s", attemptID)
		}
		if !isRegularFile(paths.ResultJSON) {
			return nil, fmt.Errorf("reviewer attempt is not completed: %s", attemptID)
		}
		return []string{attemptID}, nil
	}

	entries, err := activeStore.ReadDir(runPaths.ReviewAttemptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no completed reviewer attempts found")
		}
		return nil, err
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
		return nil, fmt.Errorf("no completed reviewer attempts found")
	}
	sort.Strings(attemptIDs)
	return attemptIDs, nil
}

// prefixAttemptWarning prefixes a parse warning with its attempt id when more
// than one attempt is being parsed, so the operator can tell which lens
// attempt produced it; a single explicit attempt keeps bare warnings.
func prefixAttemptWarning(attemptIDs []string, attemptID string, warning string) string {
	if len(attemptIDs) <= 1 {
		return warning
	}
	return attemptID + ": " + warning
}

func parseReviewerFindingBlocks(output string) ([]reviewerFindingBlock, []string) {
	text := agentMessageText([]byte(output))
	jsonBlocks := extractFencedJSONBlocks(text)
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
	// The schema marker without a single parsed block means the reviewer did
	// emit findings that this parse missed (e.g. a stream cut before the
	// closing fence) — zero proposals must read as a parse miss, not a clean
	// review.
	if len(blocks) == 0 && strings.Contains(text, reviewerFindingsSchema) {
		warnings = append(warnings, "findings schema marker present but no findings block parsed: zero proposals is a parse miss, not a clean review (inspect the attempt stdout)")
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
			if isJSONFenceStart(trimmed) || hasGluedJSONFenceStart(trimmed) {
				inJSON = true
				b.Reset()
			}
			continue
		}
		if isJSONFenceStart(trimmed) {
			// An opening fence cannot close a block: reaching one mid-block
			// means a stray glued opener swallowed prose — the real block
			// starts fresh here.
			b.Reset()
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

// hasGluedJSONFenceStart reports whether a line ends with an opening ```json
// fence glued to prose — the shape an ACP attempt log gets when an agent ends
// one message without a trailing newline and opens the next with a fence, and
// the transport cannot separate them (adapters streaming raw token deltas
// carry no messageId to mark the boundary).
func hasGluedJSONFenceStart(line string) bool {
	idx := strings.LastIndex(line, "```")
	if idx <= 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(line[idx+3:]), "json")
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
	confidence := strings.TrimSpace(input.Confidence)
	if confidence == "" {
		// A missing confidence defaults to medium, preserving v1 compatibility
		// for producers that predate the confidence field. A non-empty value
		// outside the allowed set is still rejected.
		confidence = "medium"
	}
	if !validReviewConfidence(confidence) {
		return reviewProposalRecord{}, "proposal skipped: confidence must be one of high, medium, low"
	}
	return reviewProposalRecord{
		Schema:            reviewProposalSchema,
		ID:                nextReviewID("p", index),
		RunID:             runID,
		Source:            "reviewer_attempt",
		ReviewerAttemptID: attemptID,
		findingCore: findingCore{
			Message:    sanitizeRepoRootInText(root, message),
			Severity:   severity,
			Category:   category,
			File:       filepath.ToSlash(file),
			Line:       line,
			Blocking:   blocking,
			Confidence: confidence,
		},
		Evidence:  sanitizeRepoRootInText(root, strings.TrimSpace(input.Evidence)),
		Status:    "pending",
		CreatedAt: now.Format(time.RFC3339),
	}, ""
}

// reviewSeverities, reviewCategories, and reviewConfidences are the canonical
// enum value sets for review findings, shared by the proposal validators and
// the reviewer prompt.
// (The CLI kong `enum:` tags in app.go must stay literal struct tags.)
var (
	reviewSeverities  = []string{"low", "medium", "high", "critical"}
	reviewCategories  = []string{"correctness", "scope", "quality", "validation", "process", "other"}
	reviewConfidences = []string{"high", "medium", "low"}
)

func validReviewSeverity(value string) bool {
	return slices.Contains(reviewSeverities, value)
}

func validReviewCategory(value string) bool {
	return slices.Contains(reviewCategories, value)
}

func validReviewConfidence(value string) bool {
	return slices.Contains(reviewConfidences, value)
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
		view := reviewProposalView{reviewProposalRecord: proposal}
		if view.Status == "" {
			view.Status = "pending"
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
		case "duplicate":
			// Duplicate is a terminal autonomous-loop decision, but the summary
			// schema only distinguishes accepted/rejected/pending.
		default:
			summary.Pending++
		}
	}
	return summary
}

func isProposalDecided(proposalID string, decisions []reviewProposalDecisionRecord) bool {
	if decision, ok := latestReviewProposalDecisions(decisions)[proposalID]; ok {
		return decision.Decision == "accepted" || decision.Decision == "rejected" || decision.Decision == "duplicate"
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
	fmt.Fprintln(stdout, "Reviewer attempts:")
	fmt.Fprintf(stdout, "  ids: %s\n", strings.Join(response.ReviewerAttemptIDs, ", "))
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
		if proposal.Confidence != "" {
			fmt.Fprintf(stdout, "    confidence: %s\n", proposal.Confidence)
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
