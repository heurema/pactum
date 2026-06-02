package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const (
	memoryCandidateSchema  = "pactum.memory_candidate.v1"
	memoryAcceptanceSchema = "pactum.memory_acceptance.v1"
	memoryItemSchema       = "pactum.memory_item.v1"

	memoryCandidateArtifact   = "memory/memory-candidate.json"
	memoryCandidateMDArtifact = "memory/memory-candidate.md"
	memoryAcceptanceArtifact  = "memory/memory-acceptance.json"
)

type memoryContext struct {
	Root     string
	Paths    artifacts.Paths
	RunDir   string
	RunPaths contractRunPathSet
	State    contractRunState
	Contract draftContract
	Approval approvalState
}

type memoryCandidateDocument struct {
	Schema         string                         `json:"schema"`
	RunID          string                         `json:"run_id"`
	CreatedAt      string                         `json:"created_at"`
	Source         string                         `json:"source"`
	Status         string                         `json:"status"`
	Contract       memoryCandidateContract        `json:"contract"`
	Outcome        memoryCandidateOutcome         `json:"outcome"`
	Changes        memoryCandidateChanges         `json:"changes"`
	Clarifications []memoryCandidateClarification `json:"clarifications"`
	Review         memoryCandidateReview          `json:"review"`
	Decisions      []memoryCandidateDecision      `json:"decisions"`
	Artifacts      memoryCandidateArtifacts       `json:"artifacts"`
}

type memoryCandidateContract struct {
	Goal               string   `json:"goal"`
	InScope            []string `json:"in_scope"`
	OutOfScope         []string `json:"out_of_scope"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	ValidationCommands []string `json:"validation_commands"`
}

type memoryCandidateOutcome struct {
	GateStatus        string `json:"gate_status"`
	ReviewStatus      string `json:"review_status"`
	ExecutionExitCode int    `json:"execution_exit_code"`
	ValidationPassed  bool   `json:"validation_passed"`
	ChangesNeedReview bool   `json:"changes_need_review"`
}

type memoryCandidateChanges struct {
	ChangedFiles []string `json:"changed_files"`
	NewFiles     []string `json:"new_files"`
	MissingFiles []string `json:"missing_files"`
}

type memoryCandidateClarification struct {
	QuestionID string `json:"question_id"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

type memoryCandidateReview struct {
	Findings        []memoryCandidateFinding       `json:"findings"`
	ProposalSummary memoryCandidateProposalSummary `json:"proposal_summary"`
}

type memoryCandidateFinding struct {
	ID         string `json:"id"`
	Message    string `json:"message"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Blocking   bool   `json:"blocking"`
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
}

type memoryCandidateProposalSummary struct {
	Pending  int `json:"pending"`
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
}

type memoryCandidateDecision struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type memoryCandidateArtifacts struct {
	Contract          string `json:"contract"`
	GateReport        string `json:"gate_report"`
	Review            string `json:"review"`
	Findings          string `json:"findings"`
	Resolutions       string `json:"resolutions"`
	Proposals         string `json:"proposals"`
	ProposalDecisions string `json:"proposal_decisions"`
}

type memoryAcceptanceDocument struct {
	Schema       string  `json:"schema"`
	Status       string  `json:"status"`
	AcceptedAt   *string `json:"accepted_at"`
	AcceptedBy   *string `json:"accepted_by"`
	MemoryItemID *string `json:"memory_item_id"`
}

type memoryItemRecord struct {
	Schema     string               `json:"schema"`
	ID         string               `json:"id"`
	RunID      string               `json:"run_id"`
	AcceptedAt string               `json:"accepted_at"`
	AcceptedBy string               `json:"accepted_by"`
	Source     string               `json:"source"`
	Title      string               `json:"title"`
	Summary    string               `json:"summary"`
	Files      []string             `json:"files"`
	Tags       []string             `json:"tags"`
	Candidate  string               `json:"candidate"`
	Freshness  *memoryItemFreshness `json:"freshness,omitempty"`
}

type memoryShowResponse struct {
	Candidate  memoryCandidateDocument  `json:"candidate"`
	Acceptance memoryAcceptanceDocument `json:"acceptance"`
}

type memoryAcceptResponse struct {
	Item       memoryItemRecord         `json:"item"`
	Acceptance memoryAcceptanceDocument `json:"acceptance"`
}

type preparedMemoryCandidate struct {
	Candidate        memoryCandidateDocument
	Acceptance       memoryAcceptanceDocument
	AcceptanceExists bool
	CandidateChanged bool
}

func (a App) MemoryPropose(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadMemoryContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	prepared, err := a.prepareMemoryCandidate(context)
	if err != nil {
		return err
	}
	if prepared.Acceptance.Status == "accepted" && prepared.CandidateChanged {
		return fmt.Errorf("cannot update accepted memory candidate")
	}

	if err := os.MkdirAll(context.RunPaths.MemoryDir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(context.RunPaths.MemoryCandidateJSON, prepared.Candidate); err != nil {
		return err
	}
	if err := os.WriteFile(context.RunPaths.MemoryCandidateMD, []byte(renderMemoryCandidateMD(prepared.Candidate)), 0o644); err != nil {
		return err
	}
	acceptance := prepared.Acceptance
	if acceptance.Status != "accepted" && (!prepared.AcceptanceExists || prepared.CandidateChanged) {
		acceptance = pendingMemoryAcceptance()
		if err := writeJSON(context.RunPaths.MemoryAcceptanceJSON, acceptance); err != nil {
			return err
		}
	}
	now := a.nowUTC()
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "memory_candidate_proposed", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := memoryShowResponse{Candidate: prepared.Candidate, Acceptance: acceptance}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeMemoryCandidateProposed(stdout, response)
	return nil
}

func (a App) MemoryShow(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadMemoryContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.MemoryCandidateJSON) {
		fmt.Fprintf(stdout, "Memory candidate has not been created. Run: pactum memory propose %s\n", runID)
		return nil
	}
	candidate, err := readMemoryCandidate(context.RunPaths.MemoryCandidateJSON)
	if err != nil {
		return err
	}
	acceptance, _, err := readMemoryAcceptance(context.RunPaths.MemoryAcceptanceJSON)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, memoryShowResponse{Candidate: candidate, Acceptance: acceptance})
	}
	if isRegularFile(context.RunPaths.MemoryCandidateMD) {
		data, err := os.ReadFile(context.RunPaths.MemoryCandidateMD)
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err
	}
	_, err = io.WriteString(stdout, renderMemoryCandidateMD(candidate))
	return err
}

func (a App) MemoryAccept(stdout io.Writer, runID string, acceptedBy string, jsonOutput bool) error {
	context, ok, err := a.loadMemoryContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.MemoryCandidateJSON) {
		return fmt.Errorf("memory candidate has not been created: %s", runID)
	}
	candidate, err := readMemoryCandidate(context.RunPaths.MemoryCandidateJSON)
	if err != nil {
		return err
	}
	acceptance, _, err := readMemoryAcceptance(context.RunPaths.MemoryAcceptanceJSON)
	if err != nil {
		return err
	}
	if acceptance.Status == "accepted" {
		return fmt.Errorf("memory candidate already accepted: %s", runID)
	}

	items, err := readMemoryItems(context.Paths.MemoryItems)
	if err != nil {
		return err
	}
	now := a.nowUTC()
	if strings.TrimSpace(acceptedBy) == "" {
		acceptedBy = "manual"
	}
	acceptedBy = sanitizeMemoryText(context.Root, acceptedBy)
	acceptedAt := now.Format(time.RFC3339)
	itemID := nextMemoryItemID(len(items) + 1)
	item := memoryItemFromCandidate(candidate, itemID, acceptedAt, strings.TrimSpace(acceptedBy))
	item.Freshness = buildAcceptedMemoryItemFreshness(context.Root, item.Files, acceptedAt)
	if err := appendJSONLine(context.Paths.MemoryItems, item); err != nil {
		return err
	}
	items = append(items, item)
	freshnessByID, err := readLatestMemoryFreshness(context.Paths, items)
	if err != nil {
		return err
	}
	if err := os.WriteFile(context.Paths.ProjectMemory, []byte(renderProjectMemoryMD(context.Root, items, freshnessByID)), 0o644); err != nil {
		return err
	}

	acceptance = memoryAcceptanceDocument{
		Schema:       memoryAcceptanceSchema,
		Status:       "accepted",
		AcceptedAt:   &acceptedAt,
		AcceptedBy:   &acceptedBy,
		MemoryItemID: &itemID,
	}
	if err := writeJSON(context.RunPaths.MemoryAcceptanceJSON, acceptance); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "memory_item_accepted", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := memoryAcceptResponse{Item: item, Acceptance: acceptance}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeMemoryAccepted(stdout, response)
	return nil
}

func (a App) loadMemoryContext(stdout io.Writer, runID string, jsonOutput bool) (memoryContext, bool, error) {
	root, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return memoryContext{}, false, err
	}

	runDir := filepath.Join(paths.RunsDir, runID)
	info, err := os.Stat(runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return memoryContext{}, false, fmt.Errorf("run not found: %s", runID)
		}
		return memoryContext{}, false, err
	}
	if !info.IsDir() {
		return memoryContext{}, false, fmt.Errorf("run not found: %s", runID)
	}

	runPaths := contractRunPaths(runDir)
	state, err := readContractRunState(runPaths.RunJSON)
	if err != nil {
		return memoryContext{}, false, err
	}
	contract, err := readDraftContract(runPaths.ContractJSON)
	if err != nil {
		return memoryContext{}, false, err
	}
	approval, err := readApprovalState(runPaths.ApprovalJSON)
	if err != nil {
		return memoryContext{}, false, err
	}
	return memoryContext{
		Root:     root,
		Paths:    paths,
		RunDir:   runDir,
		RunPaths: runPaths,
		State:    state,
		Contract: contract,
		Approval: approval,
	}, true, nil
}

func (a App) prepareMemoryCandidate(context memoryContext) (preparedMemoryCandidate, error) {
	if err := ensureMemoryContractApproved(context); err != nil {
		return preparedMemoryCandidate{}, err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return preparedMemoryCandidate{}, fmt.Errorf("cannot propose memory: gate report not found")
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return preparedMemoryCandidate{}, err
	}
	if !isRegularFile(context.RunPaths.ReviewJSON) {
		return preparedMemoryCandidate{}, fmt.Errorf("cannot propose memory: review is not prepared")
	}
	review, err := readReviewDocument(context.RunPaths.ReviewJSON)
	if err != nil {
		return preparedMemoryCandidate{}, err
	}
	if review.Status != "approved" {
		return preparedMemoryCandidate{}, fmt.Errorf("cannot propose memory: review is not approved")
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return preparedMemoryCandidate{}, err
	}
	proposals, proposalDecisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return preparedMemoryCandidate{}, err
	}
	reviewState := buildReviewStateWithProposals(review, findings, resolutions, proposals, proposalDecisions)
	if reviewState.ProposalSummary.Pending > 0 {
		return preparedMemoryCandidate{}, fmt.Errorf("cannot propose memory: pending review proposals remain")
	}

	createdAt := a.nowUTC().Format(time.RFC3339)
	existingBytes, existingErr := os.ReadFile(context.RunPaths.MemoryCandidateJSON)
	existingExists := existingErr == nil
	if existingErr != nil && !os.IsNotExist(existingErr) {
		return preparedMemoryCandidate{}, existingErr
	}
	if existingExists {
		existing, err := readMemoryCandidate(context.RunPaths.MemoryCandidateJSON)
		if err != nil {
			return preparedMemoryCandidate{}, err
		}
		if existing.CreatedAt != "" {
			createdAt = existing.CreatedAt
		}
	}

	candidate := buildMemoryCandidate(context, gateReport, reviewState, createdAt)
	nextBytes, err := indentedJSONBytes(candidate)
	if err != nil {
		return preparedMemoryCandidate{}, err
	}
	acceptance, acceptanceExists, err := readMemoryAcceptance(context.RunPaths.MemoryAcceptanceJSON)
	if err != nil {
		return preparedMemoryCandidate{}, err
	}
	return preparedMemoryCandidate{
		Candidate:        candidate,
		Acceptance:       acceptance,
		AcceptanceExists: acceptanceExists,
		CandidateChanged: !existingExists || !bytes.Equal(existingBytes, nextBytes),
	}, nil
}

func ensureMemoryContractApproved(context memoryContext) error {
	if context.Contract.Status != "approved" || context.Approval.Status != "approved" || context.Approval.ContractSHA256 == nil {
		return fmt.Errorf("cannot propose memory: contract is not approved")
	}
	hash, err := fileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return err
	}
	if hash != *context.Approval.ContractSHA256 {
		return fmt.Errorf("cannot propose memory: approved contract hash does not match current contract")
	}
	return nil
}

func buildMemoryCandidate(context memoryContext, gateReport gateReportDocument, reviewState reviewStateResponse, createdAt string) memoryCandidateDocument {
	contract := context.Contract
	clarifications := memoryClarificationsFromContract(context.Root, contract)
	findings := memoryFindingsFromReviewState(context.Root, reviewState)
	return memoryCandidateDocument{
		Schema:    memoryCandidateSchema,
		RunID:     context.State.RunID,
		CreatedAt: createdAt,
		Source:    "deterministic",
		Status:    "proposed",
		Contract: memoryCandidateContract{
			Goal:               sanitizeMemoryText(context.Root, contract.Goal),
			InScope:            sanitizeMemoryTexts(context.Root, contract.Scope.In),
			OutOfScope:         sanitizeMemoryTexts(context.Root, contract.Scope.Out),
			AcceptanceCriteria: sanitizeMemoryTexts(context.Root, contract.AcceptanceCriteria),
			ValidationCommands: sanitizeMemoryTexts(context.Root, contract.Validation.Commands),
		},
		Outcome: memoryCandidateOutcome{
			GateStatus:        gateReport.Status,
			ReviewStatus:      reviewState.Review.Status,
			ExecutionExitCode: gateReport.Execution.ExitCode,
			ValidationPassed:  gateReport.Summary.ValidationPassed,
			ChangesNeedReview: gateReport.Summary.ChangesNeedReview,
		},
		Changes: memoryCandidateChanges{
			ChangedFiles: sanitizeMemoryPaths(gateReport.Changes.ChangedFiles),
			NewFiles:     sanitizeMemoryPaths(gateReport.Changes.NewFiles),
			MissingFiles: sanitizeMemoryPaths(gateReport.Changes.MissingFiles),
		},
		Clarifications: clarifications,
		Review: memoryCandidateReview{
			Findings: findings,
			ProposalSummary: memoryCandidateProposalSummary{
				Pending:  reviewState.ProposalSummary.Pending,
				Accepted: reviewState.ProposalSummary.Accepted,
				Rejected: reviewState.ProposalSummary.Rejected,
			},
		},
		Decisions: memoryDecisionsFromArtifacts(context.Root, contract, clarifications, reviewState, gateReport),
		Artifacts: memoryCandidateArtifacts{
			Contract:          "contract/contract.json",
			GateReport:        gateReportArtifact,
			Review:            reviewArtifact,
			Findings:          reviewFindingsArtifact,
			Resolutions:       reviewResolutionsArtifact,
			Proposals:         reviewProposalsArtifact,
			ProposalDecisions: reviewProposalDecisionsArtifact,
		},
	}
}

func memoryClarificationsFromContract(root string, contract draftContract) []memoryCandidateClarification {
	clarifications := make([]memoryCandidateClarification, 0, len(contract.Clarifications.Questions))
	for _, question := range contract.Clarifications.Questions {
		if strings.TrimSpace(question.Answer) == "" {
			continue
		}
		clarifications = append(clarifications, memoryCandidateClarification{
			QuestionID: question.ID,
			Question:   sanitizeMemoryText(root, question.Question),
			Answer:     sanitizeMemoryText(root, question.Answer),
		})
	}
	return clarifications
}

func memoryFindingsFromReviewState(root string, state reviewStateResponse) []memoryCandidateFinding {
	findings := make([]memoryCandidateFinding, 0, len(state.Findings))
	for _, finding := range state.Findings {
		resolution := ""
		if finding.LatestResolution != nil {
			resolution = sanitizeMemoryText(root, finding.LatestResolution.Note)
		}
		findings = append(findings, memoryCandidateFinding{
			ID:         finding.ID,
			Message:    sanitizeMemoryText(root, finding.Message),
			Severity:   finding.Severity,
			Category:   finding.Category,
			File:       filepath.ToSlash(finding.File),
			Line:       finding.Line,
			Blocking:   finding.Blocking,
			Status:     finding.Status,
			Resolution: resolution,
		})
	}
	return findings
}

func memoryDecisionsFromArtifacts(root string, contract draftContract, clarifications []memoryCandidateClarification, state reviewStateResponse, gateReport gateReportDocument) []memoryCandidateDecision {
	decisions := []memoryCandidateDecision{}
	for _, item := range contract.Scope.In {
		decisions = append(decisions, memoryCandidateDecision{Kind: "scope", Text: "in scope: " + sanitizeMemoryText(root, item)})
	}
	for _, item := range contract.Scope.Out {
		decisions = append(decisions, memoryCandidateDecision{Kind: "scope", Text: "out of scope: " + sanitizeMemoryText(root, item)})
	}
	for _, clarification := range clarifications {
		decisions = append(decisions, memoryCandidateDecision{
			Kind: "clarification",
			Text: fmt.Sprintf("%s: %s Answer: %s", clarification.QuestionID, clarification.Question, clarification.Answer),
		})
	}
	for _, finding := range state.Findings {
		if finding.Status != "resolved" {
			continue
		}
		text := fmt.Sprintf("%s resolved: %s", finding.ID, sanitizeMemoryText(root, finding.Message))
		if finding.LatestResolution != nil && strings.TrimSpace(finding.LatestResolution.Note) != "" {
			text += "; resolution: " + sanitizeMemoryText(root, finding.LatestResolution.Note)
		}
		decisions = append(decisions, memoryCandidateDecision{Kind: "review_resolution", Text: text})
	}
	for _, decision := range state.ProposalDecisions {
		switch decision.Decision {
		case "accepted":
			text := fmt.Sprintf("proposal %s accepted", decision.ProposalID)
			if decision.FindingID != "" {
				text += " as " + decision.FindingID
			}
			decisions = append(decisions, memoryCandidateDecision{Kind: "review_resolution", Text: text})
		case "rejected":
			text := fmt.Sprintf("proposal %s rejected", decision.ProposalID)
			if decision.Reason != "" {
				text += ": " + sanitizeMemoryText(root, decision.Reason)
			}
			decisions = append(decisions, memoryCandidateDecision{Kind: "review_resolution", Text: text})
		}
	}
	for _, command := range gateReport.Validation.Commands {
		if command.ExitCode == 0 && !command.TimedOut {
			decisions = append(decisions, memoryCandidateDecision{Kind: "validation", Text: sanitizeMemoryText(root, command.Command) + " passed"})
		}
	}
	return decisions
}

func pendingMemoryAcceptance() memoryAcceptanceDocument {
	return memoryAcceptanceDocument{
		Schema:       memoryAcceptanceSchema,
		Status:       "pending",
		AcceptedAt:   nil,
		AcceptedBy:   nil,
		MemoryItemID: nil,
	}
}

func readMemoryCandidate(path string) (memoryCandidateDocument, error) {
	var candidate memoryCandidateDocument
	if err := readJSON(path, &candidate); err != nil {
		return memoryCandidateDocument{}, err
	}
	return candidate, nil
}

func readMemoryAcceptance(path string) (memoryAcceptanceDocument, bool, error) {
	if !isRegularFile(path) {
		return pendingMemoryAcceptance(), false, nil
	}
	var acceptance memoryAcceptanceDocument
	if err := readJSON(path, &acceptance); err != nil {
		return memoryAcceptanceDocument{}, false, err
	}
	if acceptance.Schema == "" {
		acceptance.Schema = memoryAcceptanceSchema
	}
	if acceptance.Status == "" {
		acceptance.Status = "pending"
	}
	return acceptance, true, nil
}

func readMemoryItems(path string) ([]memoryItemRecord, error) {
	return readJSONLines[memoryItemRecord](path)
}

func memoryItemFromCandidate(candidate memoryCandidateDocument, itemID string, acceptedAt string, acceptedBy string) memoryItemRecord {
	return memoryItemRecord{
		Schema:     memoryItemSchema,
		ID:         itemID,
		RunID:      candidate.RunID,
		AcceptedAt: acceptedAt,
		AcceptedBy: acceptedBy,
		Source:     "memory_candidate",
		Title:      memoryItemTitle(candidate),
		Summary:    memoryItemSummary(candidate),
		Files:      memoryItemFiles(candidate),
		Tags:       []string{"contract", "reviewed"},
		Candidate:  filepath.ToSlash(filepath.Join("runs", candidate.RunID, memoryCandidateArtifact)),
	}
}

func memoryItemTitle(candidate memoryCandidateDocument) string {
	goal := strings.TrimSpace(candidate.Contract.Goal)
	if goal == "" {
		return "Reviewed run " + candidate.RunID
	}
	return truncateMemoryText(goal, 80)
}

func memoryItemSummary(candidate memoryCandidateDocument) string {
	summary := fmt.Sprintf("Reviewed run %s with gate status %s and review status %s.", candidate.RunID, candidate.Outcome.GateStatus, candidate.Outcome.ReviewStatus)
	if strings.TrimSpace(candidate.Contract.Goal) != "" {
		summary += " Goal: " + strings.TrimSpace(candidate.Contract.Goal)
	}
	return truncateMemoryText(summary, 240)
}

func memoryItemFiles(candidate memoryCandidateDocument) []string {
	seen := map[string]struct{}{}
	files := []string{}
	add := func(values ...string) {
		for _, value := range values {
			value = filepath.ToSlash(strings.TrimSpace(value))
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			files = append(files, value)
		}
	}
	add(candidate.Changes.ChangedFiles...)
	add(candidate.Changes.NewFiles...)
	add(candidate.Changes.MissingFiles...)
	for _, finding := range candidate.Review.Findings {
		add(finding.File)
	}
	sort.Strings(files)
	return files
}

func nextMemoryItemID(index int) string {
	return fmt.Sprintf("mem_%03d", index)
}

func renderMemoryCandidateMD(candidate memoryCandidateDocument) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Memory Candidate")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Run")
	fmt.Fprintf(&b, "- Run id: %s\n", candidate.RunID)
	fmt.Fprintf(&b, "- Source: %s\n", candidate.Source)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Contract")
	fmt.Fprintf(&b, "- Goal: %s\n", valueOrNone(candidate.Contract.Goal))
	writeMarkdownList(&b, "In scope", candidate.Contract.InScope)
	writeMarkdownList(&b, "Out of scope", candidate.Contract.OutOfScope)
	writeMarkdownList(&b, "Acceptance criteria", candidate.Contract.AcceptanceCriteria)
	writeMarkdownList(&b, "Validation commands", candidate.Contract.ValidationCommands)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Outcome")
	fmt.Fprintf(&b, "- Gate status: %s\n", candidate.Outcome.GateStatus)
	fmt.Fprintf(&b, "- Review status: %s\n", candidate.Outcome.ReviewStatus)
	fmt.Fprintf(&b, "- Execution exit code: %d\n", candidate.Outcome.ExecutionExitCode)
	fmt.Fprintf(&b, "- Validation passed: %t\n", candidate.Outcome.ValidationPassed)
	fmt.Fprintf(&b, "- Changes need review: %t\n", candidate.Outcome.ChangesNeedReview)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Changes")
	writeMarkdownList(&b, "Changed files", candidate.Changes.ChangedFiles)
	writeMarkdownList(&b, "New files", candidate.Changes.NewFiles)
	writeMarkdownList(&b, "Missing files", candidate.Changes.MissingFiles)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Clarifications")
	if len(candidate.Clarifications) == 0 {
		fmt.Fprintln(&b, "- None")
	} else {
		for _, clarification := range candidate.Clarifications {
			fmt.Fprintf(&b, "- %s: %s\n", clarification.QuestionID, clarification.Question)
			fmt.Fprintf(&b, "  Answer: %s\n", clarification.Answer)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Review Decisions")
	if len(candidate.Review.Findings) == 0 {
		fmt.Fprintln(&b, "- Findings: none")
	} else {
		for _, finding := range candidate.Review.Findings {
			location := finding.File
			if finding.Line > 0 {
				location = fmt.Sprintf("%s:%d", location, finding.Line)
			}
			if location == "" {
				location = "no file"
			}
			fmt.Fprintf(&b, "- %s [%s] %s %s: %s\n", finding.ID, finding.Severity, finding.Status, location, finding.Message)
			if finding.Resolution != "" {
				fmt.Fprintf(&b, "  Resolution: %s\n", finding.Resolution)
			}
		}
	}
	fmt.Fprintf(&b, "- Proposal summary: pending=%d accepted=%d rejected=%d\n", candidate.Review.ProposalSummary.Pending, candidate.Review.ProposalSummary.Accepted, candidate.Review.ProposalSummary.Rejected)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Reusable Project Knowledge")
	if len(candidate.Decisions) == 0 {
		fmt.Fprintln(&b, "- None")
	} else {
		for _, decision := range candidate.Decisions {
			fmt.Fprintf(&b, "- %s: %s\n", decision.Kind, decision.Text)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Artifacts")
	fmt.Fprintf(&b, "- Contract: %s\n", candidate.Artifacts.Contract)
	fmt.Fprintf(&b, "- Gate report: %s\n", candidate.Artifacts.GateReport)
	fmt.Fprintf(&b, "- Review: %s\n", candidate.Artifacts.Review)
	fmt.Fprintf(&b, "- Findings: %s\n", candidate.Artifacts.Findings)
	fmt.Fprintf(&b, "- Resolutions: %s\n", candidate.Artifacts.Resolutions)
	fmt.Fprintf(&b, "- Proposals: %s\n", candidate.Artifacts.Proposals)
	fmt.Fprintf(&b, "- Proposal decisions: %s\n", candidate.Artifacts.ProposalDecisions)
	return b.String()
}

func renderProjectMemoryMD(root string, items []memoryItemRecord, freshnessByID map[string]memoryEffectiveFreshness) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Project Memory")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Accepted memory items")
	if len(items) == 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "No accepted memory items.")
		return b.String()
	}
	for _, item := range items {
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "### %s - %s\n", item.ID, valueOrNone(sanitizeMemoryText(root, item.Title)))
		fmt.Fprintf(&b, "- Run: %s\n", item.RunID)
		freshness := effectiveMemoryFreshnessForItem(item, freshnessByID)
		fmt.Fprintf(&b, "- Freshness: %s\n", freshness.Status)
		files := sanitizeSelectedMemoryPaths(root, item.Files)
		if len(files) == 0 {
			fmt.Fprintln(&b, "- Files: none")
		} else {
			fmt.Fprintf(&b, "- Files: %s\n", strings.Join(files, ", "))
		}
		fmt.Fprintf(&b, "- Summary: %s\n", valueOrNone(sanitizeMemoryText(root, item.Summary)))
		fmt.Fprintf(&b, "- Candidate: %s\n", sanitizeSelectedMemoryPath(root, item.Candidate))
	}
	return b.String()
}

func writeMarkdownList(b *strings.Builder, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(b, "- %s: none\n", label)
		return
	}
	fmt.Fprintf(b, "- %s:\n", label)
	for _, value := range values {
		fmt.Fprintf(b, "  - %s\n", value)
	}
}

func writeMemoryCandidateProposed(stdout io.Writer, response memoryShowResponse) {
	fmt.Fprintln(stdout, "Memory candidate proposed")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Candidate.RunID)
	fmt.Fprintf(stdout, "  source: %s\n", response.Candidate.Source)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  candidate: %s\n", runArtifactRepoRel(response.Candidate.RunID, memoryCandidateArtifact))
	fmt.Fprintf(stdout, "  preview: %s\n", runArtifactRepoRel(response.Candidate.RunID, memoryCandidateMDArtifact))
	fmt.Fprintf(stdout, "  acceptance: %s\n", runArtifactRepoRel(response.Candidate.RunID, memoryAcceptanceArtifact))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	fmt.Fprintf(stdout, "  pactum memory show %s\n", response.Candidate.RunID)
	fmt.Fprintf(stdout, "  pactum memory accept %s\n", response.Candidate.RunID)
}

func writeMemoryAccepted(stdout io.Writer, response memoryAcceptResponse) {
	fmt.Fprintln(stdout, "Memory item accepted")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Item:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Item.ID)
	fmt.Fprintf(stdout, "  run: %s\n", response.Item.RunID)
	fmt.Fprintf(stdout, "  accepted by: %s\n", response.Item.AcceptedBy)
	fmt.Fprintf(stdout, "  project memory: %s\n", artifacts.WorkspaceRel+"/memory/project-memory.md")
}

func sanitizeMemoryText(root string, value string) string {
	return sanitizeRepoRootInText(root, strings.TrimSpace(value))
}

func sanitizeMemoryTexts(root string, values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, sanitizeMemoryText(root, value))
	}
	return result
}

func sanitizeMemoryPaths(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, filepath.ToSlash(strings.TrimSpace(value)))
	}
	return result
}

func truncateMemoryText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

func indentedJSONBytes(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
