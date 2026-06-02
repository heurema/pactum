package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const (
	reviewSchema           = "pactum.review.v1"
	reviewFindingSchema    = "pactum.review_finding.v1"
	reviewResolutionSchema = "pactum.review_resolution.v1"

	reviewArtifact             = "review/review.json"
	reviewFindingsArtifact     = "review/findings.jsonl"
	reviewResolutionsArtifact  = "review/resolutions.jsonl"
	reviewerContextArtifact    = "review/reviewer-context.md"
	reviewerPromptArtifact     = "review/reviewer-prompt.md"
	reviewerDryRunArtifact     = "review/reviewer-dry-run.json"
	reviewerDryRunSchema       = "pactum.review_dry_run.v1"
	reviewerAttemptsArtifact   = "review/reviewer-attempts"
	reviewerLastResultArtifact = "review/reviewer-last-result.json"
	reviewerRequestSchema      = "pactum.reviewer_request.v1"
	reviewerResultSchema       = "pactum.reviewer_result.v1"
)

type reviewContext struct {
	Root     string
	Paths    artifacts.Paths
	RunDir   string
	RunPaths contractRunPathSet
	State    contractRunState
}

type reviewDocument struct {
	Schema    string          `json:"schema"`
	RunID     string          `json:"run_id"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Gate      reviewGate      `json:"gate"`
	Summary   reviewSummary   `json:"summary"`
	Approval  reviewApproval  `json:"approval"`
	Artifacts reviewArtifacts `json:"artifacts"`
}

type reviewGate struct {
	Status string `json:"status"`
	Report string `json:"report"`
}

type reviewSummary struct {
	Findings     int `json:"findings"`
	Open         int `json:"open"`
	Resolved     int `json:"resolved"`
	BlockingOpen int `json:"blocking_open"`
}

type reviewApproval struct {
	ApprovedAt *string `json:"approved_at"`
	ApprovedBy *string `json:"approved_by"`
}

type reviewArtifacts struct {
	Review            string `json:"review"`
	Findings          string `json:"findings"`
	Resolutions       string `json:"resolutions"`
	Proposals         string `json:"proposals"`
	ProposalDecisions string `json:"proposal_decisions"`
	GateReport        string `json:"gate_report"`
}

type reviewFindingInput struct {
	Message  string
	Severity string
	Category string
	File     string
	Line     int
	Blocking bool
}

type reviewFindingRecord struct {
	Schema    string `json:"schema"`
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	Message   string `json:"message"`
	Severity  string `json:"severity"`
	Category  string `json:"category"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Blocking  bool   `json:"blocking"`
	Evidence  string `json:"evidence,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	Source    string `json:"source"`
}

type reviewResolutionRecord struct {
	Schema    string `json:"schema"`
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	FindingID string `json:"finding_id"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
	Source    string `json:"source"`
}

type reviewFindingView struct {
	Schema           string                  `json:"schema"`
	ID               string                  `json:"id"`
	RunID            string                  `json:"run_id"`
	Message          string                  `json:"message"`
	Severity         string                  `json:"severity"`
	Category         string                  `json:"category"`
	File             string                  `json:"file,omitempty"`
	Line             int                     `json:"line,omitempty"`
	Blocking         bool                    `json:"blocking"`
	Evidence         string                  `json:"evidence,omitempty"`
	Status           string                  `json:"status"`
	CreatedAt        string                  `json:"created_at"`
	Source           string                  `json:"source"`
	LatestResolution *reviewResolutionRecord `json:"latest_resolution,omitempty"`
}

type reviewStateResponse struct {
	Review            reviewDocument                 `json:"review"`
	Findings          []reviewFindingView            `json:"findings"`
	Resolutions       []reviewResolutionRecord       `json:"resolutions"`
	Proposals         []reviewProposalView           `json:"proposals"`
	ProposalDecisions []reviewProposalDecisionRecord `json:"proposal_decisions"`
	ProposalSummary   reviewProposalSummary          `json:"proposal_summary"`
}

type reviewAddFindingResponse struct {
	Finding reviewFindingRecord `json:"finding"`
	State   reviewStateResponse `json:"state"`
}

type reviewResolveResponse struct {
	Resolution reviewResolutionRecord `json:"resolution"`
	State      reviewStateResponse    `json:"state"`
}

type reviewerDryRunPreparation struct {
	Context           reviewContext
	Contract          draftContract
	GateReport        gateReportDocument
	Review            reviewDocument
	Findings          []reviewFindingRecord
	Resolutions       []reviewResolutionRecord
	Proposals         []reviewProposalRecord
	ProposalDecisions []reviewProposalDecisionRecord
	Reviewer          agents.AgentDescriptor
}

type reviewerDryRunDocument struct {
	Schema    string                  `json:"schema"`
	RunID     string                  `json:"run_id"`
	CreatedAt string                  `json:"created_at"`
	Reviewer  reviewerDryRunAgent     `json:"reviewer"`
	Checks    reviewerDryRunChecks    `json:"checks"`
	Artifacts reviewerDryRunArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand    `json:"would_run"`
}

type reviewerDryRunAgent struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Input   string   `json:"input"`
}

type reviewerDryRunChecks struct {
	ReviewPrepared   bool `json:"review_prepared"`
	GateReportReady  bool `json:"gate_report_ready"`
	ContractApproved bool `json:"contract_approved"`
}

type reviewerDryRunArtifacts struct {
	ReviewerPrompt    string `json:"reviewer_prompt"`
	ReviewerContext   string `json:"reviewer_context"`
	Review            string `json:"review"`
	Findings          string `json:"findings"`
	Resolutions       string `json:"resolutions"`
	Proposals         string `json:"proposals"`
	ProposalDecisions string `json:"proposal_decisions"`
	GateReport        string `json:"gate_report"`
}

type reviewerRequestDocument struct {
	Schema    string                  `json:"schema"`
	RunID     string                  `json:"run_id"`
	AttemptID string                  `json:"attempt_id"`
	CreatedAt string                  `json:"created_at"`
	Reviewer  reviewerDryRunAgent     `json:"reviewer"`
	Artifacts reviewerDryRunArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand    `json:"would_run"`
}

type reviewerResultDocument struct {
	Schema         string `json:"schema"`
	RunID          string `json:"run_id"`
	AttemptID      string `json:"attempt_id"`
	Reviewer       string `json:"reviewer"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	DurationMillis int64  `json:"duration_ms"`
	ExitCode       int    `json:"exit_code"`
	TimedOut       bool   `json:"timed_out"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
}

type reviewerAttemptPathSet struct {
	Dir         string
	RequestJSON string
	StdoutLog   string
	StderrLog   string
	ResultJSON  string
}

type reviewerProcessError struct {
	ExitCode int
}

func (e reviewerProcessError) Error() string {
	return fmt.Sprintf("reviewer process exited with code %d", e.ExitCode)
}

func (a App) ReviewPrepare(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return fmt.Errorf("cannot prepare review: gate report not found")
	}

	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(context.RunPaths.ReviewDir, 0o755); err != nil {
		return err
	}
	if err := ensureAppendOnlyFile(context.RunPaths.ReviewFindingsJSONL); err != nil {
		return err
	}
	if err := ensureAppendOnlyFile(context.RunPaths.ReviewResolutionsJSONL); err != nil {
		return err
	}

	now := a.nowUTC()
	review := newReviewDocument(runID, gateReport.Status, now.Format(time.RFC3339))
	if isRegularFile(context.RunPaths.ReviewJSON) {
		existing, err := readReviewDocument(context.RunPaths.ReviewJSON)
		if err != nil {
			return err
		}
		review = existing
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	proposals, proposalDecisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return err
	}
	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, now.Format(time.RFC3339))
	if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_prepared", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	state := buildReviewStateWithProposals(review, findings, resolutions, proposals, proposalDecisions)
	if jsonOutput {
		return writeJSONResponse(stdout, state)
	}
	writeReviewPrepared(stdout, state)
	return nil
}

func (a App) ReviewStatus(stdout io.Writer, runID string, jsonOutput bool) error {
	state, ok, err := a.loadPreparedReviewState(stdout, runID)
	if err != nil || !ok {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, state)
	}
	writeReviewStatus(stdout, state)
	return nil
}

func (a App) ReviewShow(stdout io.Writer, runID string, jsonOutput bool) error {
	state, ok, err := a.loadPreparedReviewState(stdout, runID)
	if err != nil || !ok {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, state)
	}
	writeReviewShow(stdout, state)
	return nil
}

func (a App) ReviewAddFinding(stdout io.Writer, runID string, input reviewFindingInput, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	review, err := requireReviewPrepared(context.RunPaths, runID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.Message) == "" {
		return fmt.Errorf("review finding message is required")
	}
	if input.File != "" && filepath.IsAbs(input.File) {
		return fmt.Errorf("review finding file must be repo-relative")
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
		Message:   input.Message,
		Severity:  input.Severity,
		Category:  input.Category,
		File:      filepath.ToSlash(input.File),
		Line:      input.Line,
		Blocking:  input.Blocking,
		Status:    "open",
		CreatedAt: now.Format(time.RFC3339),
		Source:    "manual",
	}
	if err := appendJSONLine(context.RunPaths.ReviewFindingsJSONL, finding); err != nil {
		return err
	}
	findings = append(findings, finding)

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
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_finding_added", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := reviewAddFindingResponse{Finding: finding, State: buildReviewState(review, findings, resolutions)}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeReviewFindingAdded(stdout, response)
	return nil
}

func (a App) ReviewResolve(stdout io.Writer, runID string, findingID string, note string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	review, err := requireReviewPrepared(context.RunPaths, runID)
	if err != nil {
		return err
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	if !hasReviewFinding(findings, findingID) {
		return fmt.Errorf("review finding not found: %s", findingID)
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}

	now := a.nowUTC()
	resolution := reviewResolutionRecord{
		Schema:    reviewResolutionSchema,
		ID:        nextReviewID("r", len(resolutions)+1),
		RunID:     runID,
		FindingID: findingID,
		Note:      note,
		CreatedAt: now.Format(time.RFC3339),
		Source:    "manual",
	}
	if err := appendJSONLine(context.RunPaths.ReviewResolutionsJSONL, resolution); err != nil {
		return err
	}
	resolutions = append(resolutions, resolution)

	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, now.Format(time.RFC3339))
	if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_finding_resolved", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := reviewResolveResponse{Resolution: resolution, State: buildReviewState(review, findings, resolutions)}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeReviewResolved(stdout, response)
	return nil
}

func (a App) ReviewApprove(stdout io.Writer, runID string, approvedBy string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	review, err := requireReviewPrepared(context.RunPaths, runID)
	if err != nil {
		return err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return fmt.Errorf("cannot approve review: gate report not found")
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}
	if gateReport.Status == "failed" {
		return fmt.Errorf("cannot approve review: gate status is failed")
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	summary := summarizeReview(findings, resolutions)
	if summary.BlockingOpen > 0 {
		return fmt.Errorf("cannot approve review: blocking review findings remain")
	}

	now := a.nowUTC()
	if strings.TrimSpace(approvedBy) == "" {
		approvedBy = "manual"
	}
	approvedAt := now.Format(time.RFC3339)
	review.Gate.Status = gateReport.Status
	review.Summary = summary
	review.Status = "approved"
	review.UpdatedAt = approvedAt
	review.Approval = reviewApproval{ApprovedAt: &approvedAt, ApprovedBy: &approvedBy}
	review.Artifacts = defaultReviewArtifacts()
	if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_approved", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	state := buildReviewState(review, findings, resolutions)
	if jsonOutput {
		return writeJSONResponse(stdout, state)
	}
	writeReviewApproved(stdout, state)
	return nil
}

func (a App) ReviewDryRun(stdout io.Writer, runID string, reviewerName string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	prep, err := a.prepareReviewerDryRun(context, reviewerName)
	if err != nil {
		return err
	}

	now := a.nowUTC()
	createdAt := now.Format(time.RFC3339)
	plan, err := buildReviewerDryRunDocument(runID, createdAt, prep.Reviewer)
	if err != nil {
		return err
	}
	if err := writeReviewerDryRunArtifacts(prep, plan); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_dry_run_prepared", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	if jsonOutput {
		return writeJSONResponse(stdout, plan)
	}
	writeReviewDryRun(stdout, plan)
	return nil
}

func (a App) ReviewRun(stdout io.Writer, runID string, reviewerName string, timeout time.Duration, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	prep, err := a.prepareReviewerRun(context, reviewerName)
	if err != nil {
		return err
	}

	now := a.nowUTC()
	plan, err := ensureReviewerDryRunArtifacts(prep, now.Format(time.RFC3339))
	if err != nil {
		return err
	}

	attemptID, err := nextReviewerAttemptID(context.RunPaths.ReviewAttemptsDir)
	if err != nil {
		return err
	}
	attemptPaths := reviewerAttemptPaths(context.RunPaths, attemptID)
	if err := os.MkdirAll(attemptPaths.Dir, 0o755); err != nil {
		return err
	}

	request := reviewerRequestDocument{
		Schema:    reviewerRequestSchema,
		RunID:     runID,
		AttemptID: attemptID,
		CreatedAt: now.Format(time.RFC3339),
		Reviewer: reviewerDryRunAgent{
			Name:    prep.Reviewer.Name,
			Command: prep.Reviewer.Command,
			Args:    append([]string{}, prep.Reviewer.Args...),
			Input:   prep.Reviewer.Input,
		},
		Artifacts: plan.Artifacts,
		WouldRun:  plan.WouldRun,
	}
	if err := writeJSON(attemptPaths.RequestJSON, request); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "reviewer_attempt_started", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	runResult, runErr := agents.RunSubprocess(agents.RunRequest{
		RepoRoot:       context.Root,
		RunID:          runID,
		AttemptID:      attemptID,
		Agent:          prep.Reviewer,
		PromptRepoPath: reviewerPromptRepoPath(runID),
		ArtifactDir:    reviewerAttemptsArtifact,
		Timeout:        timeout,
	})
	if runErr != nil && runResult.StartedAt == "" {
		return runErr
	}
	result := reviewerResultFromRunResult(runID, attemptID, prep.Reviewer.Name, runResult)
	if err := writeJSON(attemptPaths.ResultJSON, result); err != nil {
		return err
	}
	if err := writeJSON(context.RunPaths.ReviewLastResultJSON, result); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "reviewer_attempt_finished", Timestamp: reviewerResultTimestamp(result, now), RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	if jsonOutput {
		if err := writeJSONResponse(stdout, result); err != nil {
			return err
		}
	} else {
		writeReviewRun(stdout, request, result)
	}
	if runErr != nil {
		if result.TimedOut {
			return fmt.Errorf("reviewer process timed out after %s", timeout)
		}
		return reviewerProcessError{ExitCode: result.ExitCode}
	}
	return nil
}

func (a App) loadPreparedReviewState(stdout io.Writer, runID string) (reviewStateResponse, bool, error) {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return reviewStateResponse{}, false, err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) || !isRegularFile(context.RunPaths.ReviewJSON) {
		writeReviewNotPrepared(stdout, runID)
		return reviewStateResponse{}, false, nil
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return reviewStateResponse{}, false, err
	}
	review, err := readReviewDocument(context.RunPaths.ReviewJSON)
	if err != nil {
		return reviewStateResponse{}, false, err
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return reviewStateResponse{}, false, err
	}
	proposals, proposalDecisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return reviewStateResponse{}, false, err
	}
	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, "")
	return buildReviewStateWithProposals(review, findings, resolutions, proposals, proposalDecisions), true, nil
}

func (a App) loadReviewContext(stdout io.Writer, runID string) (reviewContext, bool, error) {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return reviewContext{}, false, err
	}
	if workspace == "" {
		fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
		return reviewContext{}, false, nil
	}

	paths := artifacts.New(root)
	runDir := filepath.Join(paths.RunsDir, runID)
	info, err := os.Stat(runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return reviewContext{}, false, fmt.Errorf("run not found: %s", runID)
		}
		return reviewContext{}, false, err
	}
	if !info.IsDir() {
		return reviewContext{}, false, fmt.Errorf("run not found: %s", runID)
	}

	runPaths := contractRunPaths(runDir)
	state, err := readContractRunState(runPaths.RunJSON)
	if err != nil {
		return reviewContext{}, false, err
	}
	return reviewContext{
		Root:     root,
		Paths:    paths,
		RunDir:   runDir,
		RunPaths: runPaths,
		State:    state,
	}, true, nil
}

func (a App) prepareReviewerDryRun(context reviewContext, reviewerName string) (reviewerDryRunPreparation, error) {
	review, err := requireReviewPrepared(context.RunPaths, context.State.RunID)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return reviewerDryRunPreparation{}, fmt.Errorf("cannot prepare reviewer dry-run: gate report not found")
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	contract, err := readDraftContract(context.RunPaths.ContractJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	approval, err := readApprovalState(context.RunPaths.ApprovalJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if contract.Status != "approved" || approval.Status != "approved" || approval.ContractSHA256 == nil {
		return reviewerDryRunPreparation{}, fmt.Errorf("cannot prepare reviewer dry-run: contract is not approved")
	}
	hash, err := fileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if hash != *approval.ContractSHA256 {
		return reviewerDryRunPreparation{}, fmt.Errorf("cannot prepare reviewer dry-run: contract is not approved")
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	proposals, proposalDecisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	reviewer, err := a.agentRegistry().ResolveReviewer(reviewerName)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if reviewer.Input != agents.InputPromptFile {
		return reviewerDryRunPreparation{}, fmt.Errorf("unsupported agent input mode: %s", reviewer.Input)
	}

	review = refreshReviewDocument(review, context.State.RunID, gateReport.Status, findings, resolutions, "")
	return reviewerDryRunPreparation{
		Context:           context,
		Contract:          contract,
		GateReport:        gateReport,
		Review:            review,
		Findings:          findings,
		Resolutions:       resolutions,
		Proposals:         proposals,
		ProposalDecisions: proposalDecisions,
		Reviewer:          reviewer,
	}, nil
}

func (a App) prepareReviewerRun(context reviewContext, reviewerName string) (reviewerDryRunPreparation, error) {
	review, err := requireReviewPrepared(context.RunPaths, context.State.RunID)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return reviewerDryRunPreparation{}, fmt.Errorf("cannot run reviewer: gate report not found")
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	contract, err := readDraftContract(context.RunPaths.ContractJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	approval, err := readApprovalState(context.RunPaths.ApprovalJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if contract.Status != "approved" || approval.Status != "approved" || approval.ContractSHA256 == nil {
		return reviewerDryRunPreparation{}, fmt.Errorf("cannot run reviewer: contract is not approved")
	}
	hash, err := fileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if hash != *approval.ContractSHA256 {
		return reviewerDryRunPreparation{}, fmt.Errorf("cannot run reviewer: contract is not approved")
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	proposals, proposalDecisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	reviewer, err := a.agentRegistry().ResolveReviewer(reviewerName)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if reviewer.Input != agents.InputPromptFile {
		return reviewerDryRunPreparation{}, fmt.Errorf("unsupported agent input mode: %s", reviewer.Input)
	}

	review = refreshReviewDocument(review, context.State.RunID, gateReport.Status, findings, resolutions, "")
	return reviewerDryRunPreparation{
		Context:           context,
		Contract:          contract,
		GateReport:        gateReport,
		Review:            review,
		Findings:          findings,
		Resolutions:       resolutions,
		Proposals:         proposals,
		ProposalDecisions: proposalDecisions,
		Reviewer:          reviewer,
	}, nil
}

func writeReviewerDryRunArtifacts(prep reviewerDryRunPreparation, plan reviewerDryRunDocument) error {
	if err := os.MkdirAll(prep.Context.RunPaths.ReviewDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(prep.Context.RunPaths.ReviewContextMD, []byte(renderReviewerContext(prep)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(prep.Context.RunPaths.ReviewPromptMD, []byte(renderReviewerPrompt(prep.Context.State.RunID)), 0o644); err != nil {
		return err
	}
	return writeJSON(prep.Context.RunPaths.ReviewDryRunJSON, plan)
}

func ensureReviewerDryRunArtifacts(prep reviewerDryRunPreparation, createdAt string) (reviewerDryRunDocument, error) {
	expected, err := buildReviewerDryRunDocument(prep.Context.State.RunID, createdAt, prep.Reviewer)
	if err != nil {
		return reviewerDryRunDocument{}, err
	}
	if isRegularFile(prep.Context.RunPaths.ReviewContextMD) &&
		isRegularFile(prep.Context.RunPaths.ReviewPromptMD) &&
		isRegularFile(prep.Context.RunPaths.ReviewDryRunJSON) {
		var existing reviewerDryRunDocument
		if err := readJSON(prep.Context.RunPaths.ReviewDryRunJSON, &existing); err == nil && reviewerDryRunMatches(existing, expected) {
			return existing, nil
		}
	}
	if err := writeReviewerDryRunArtifacts(prep, expected); err != nil {
		return reviewerDryRunDocument{}, err
	}
	return expected, nil
}

func reviewerDryRunMatches(got reviewerDryRunDocument, want reviewerDryRunDocument) bool {
	return got.Schema == want.Schema &&
		got.RunID == want.RunID &&
		got.Reviewer.Name == want.Reviewer.Name &&
		got.Reviewer.Command == want.Reviewer.Command &&
		got.Reviewer.Input == want.Reviewer.Input &&
		sameStringSlice(got.Reviewer.Args, want.Reviewer.Args) &&
		got.Checks == want.Checks &&
		got.Artifacts == want.Artifacts &&
		got.WouldRun.Command == want.WouldRun.Command &&
		sameStringSlice(got.WouldRun.Args, want.WouldRun.Args) &&
		got.WouldRun.Stdin == want.WouldRun.Stdin
}

func readReviewGateReport(path string) (gateReportDocument, error) {
	var report gateReportDocument
	if err := readJSON(path, &report); err != nil {
		return gateReportDocument{}, err
	}
	return report, nil
}

func readReviewDocument(path string) (reviewDocument, error) {
	var review reviewDocument
	if err := readJSON(path, &review); err != nil {
		return reviewDocument{}, err
	}
	return review, nil
}

func requireReviewPrepared(runPaths contractRunPathSet, runID string) (reviewDocument, error) {
	if !isRegularFile(runPaths.ReviewJSON) {
		return reviewDocument{}, fmt.Errorf("review has not been prepared: %s", runID)
	}
	return readReviewDocument(runPaths.ReviewJSON)
}

func readReviewRecords(runPaths contractRunPathSet) ([]reviewFindingRecord, []reviewResolutionRecord, error) {
	findings, err := readJSONLines[reviewFindingRecord](runPaths.ReviewFindingsJSONL)
	if err != nil {
		return nil, nil, err
	}
	resolutions, err := readJSONLines[reviewResolutionRecord](runPaths.ReviewResolutionsJSONL)
	if err != nil {
		return nil, nil, err
	}
	return findings, resolutions, nil
}

func ensureAppendOnlyFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func newReviewDocument(runID string, gateStatus string, now string) reviewDocument {
	return reviewDocument{
		Schema:    reviewSchema,
		RunID:     runID,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
		Gate: reviewGate{
			Status: gateStatus,
			Report: gateReportArtifact,
		},
		Summary:   reviewSummary{},
		Approval:  reviewApproval{},
		Artifacts: defaultReviewArtifacts(),
	}
}

func refreshReviewDocument(review reviewDocument, runID string, gateStatus string, findings []reviewFindingRecord, resolutions []reviewResolutionRecord, updatedAt string) reviewDocument {
	if review.Schema == "" {
		review.Schema = reviewSchema
	}
	review.RunID = runID
	if review.CreatedAt == "" {
		if updatedAt != "" {
			review.CreatedAt = updatedAt
		} else {
			review.CreatedAt = time.Time{}.Format(time.RFC3339)
		}
	}
	if updatedAt != "" {
		review.UpdatedAt = updatedAt
	}
	review.Gate = reviewGate{
		Status: gateStatus,
		Report: gateReportArtifact,
	}
	review.Summary = summarizeReview(findings, resolutions)
	review.Artifacts = defaultReviewArtifacts()
	review.Status = reviewStatus(review.Approval, review.Summary)
	return review
}

func defaultReviewArtifacts() reviewArtifacts {
	return reviewArtifacts{
		Review:            reviewArtifact,
		Findings:          reviewFindingsArtifact,
		Resolutions:       reviewResolutionsArtifact,
		Proposals:         reviewProposalsArtifact,
		ProposalDecisions: reviewProposalDecisionsArtifact,
		GateReport:        gateReportArtifact,
	}
}

func summarizeReview(findings []reviewFindingRecord, resolutions []reviewResolutionRecord) reviewSummary {
	latest := latestReviewResolutions(resolutions)
	summary := reviewSummary{Findings: len(findings)}
	for _, finding := range findings {
		if _, ok := latest[finding.ID]; ok {
			summary.Resolved++
			continue
		}
		summary.Open++
		if finding.Blocking {
			summary.BlockingOpen++
		}
	}
	return summary
}

func reviewStatus(approval reviewApproval, summary reviewSummary) string {
	if approval.ApprovedAt != nil && approval.ApprovedBy != nil && summary.BlockingOpen == 0 {
		return "approved"
	}
	if summary.BlockingOpen > 0 {
		return "changes_requested"
	}
	return "pending"
}

func buildReviewState(review reviewDocument, findings []reviewFindingRecord, resolutions []reviewResolutionRecord) reviewStateResponse {
	return buildReviewStateWithProposals(review, findings, resolutions, nil, nil)
}

func buildReviewStateWithProposals(review reviewDocument, findings []reviewFindingRecord, resolutions []reviewResolutionRecord, proposals []reviewProposalRecord, proposalDecisions []reviewProposalDecisionRecord) reviewStateResponse {
	latest := latestReviewResolutions(resolutions)
	views := make([]reviewFindingView, 0, len(findings))
	for _, finding := range findings {
		view := reviewFindingView{
			Schema:    finding.Schema,
			ID:        finding.ID,
			RunID:     finding.RunID,
			Message:   finding.Message,
			Severity:  finding.Severity,
			Category:  finding.Category,
			File:      finding.File,
			Line:      finding.Line,
			Blocking:  finding.Blocking,
			Evidence:  finding.Evidence,
			Status:    "open",
			CreatedAt: finding.CreatedAt,
			Source:    finding.Source,
		}
		if resolution, ok := latest[finding.ID]; ok {
			view.Status = "resolved"
			resolutionCopy := resolution
			view.LatestResolution = &resolutionCopy
		}
		views = append(views, view)
	}
	if resolutions == nil {
		resolutions = []reviewResolutionRecord{}
	}
	if proposalDecisions == nil {
		proposalDecisions = []reviewProposalDecisionRecord{}
	}
	proposalViews := buildReviewProposalViews(proposals, proposalDecisions)
	return reviewStateResponse{
		Review:            review,
		Findings:          views,
		Resolutions:       resolutions,
		Proposals:         proposalViews,
		ProposalDecisions: proposalDecisions,
		ProposalSummary:   summarizeReviewProposals(proposalViews),
	}
}

func latestReviewResolutions(resolutions []reviewResolutionRecord) map[string]reviewResolutionRecord {
	latest := make(map[string]reviewResolutionRecord, len(resolutions))
	for _, resolution := range resolutions {
		latest[resolution.FindingID] = resolution
	}
	return latest
}

func hasReviewFinding(findings []reviewFindingRecord, findingID string) bool {
	for _, finding := range findings {
		if finding.ID == findingID {
			return true
		}
	}
	return false
}

func nextReviewID(prefix string, index int) string {
	return fmt.Sprintf("%s_%03d", prefix, index)
}

func buildReviewerDryRunDocument(runID string, createdAt string, reviewer agents.AgentDescriptor) (reviewerDryRunDocument, error) {
	agentArgs := append([]string{}, reviewer.Args...)
	reviewerPromptPath := runArtifactRepoRel(runID, reviewerPromptArtifact)
	wouldRun, err := agents.BuildCommand(reviewer, reviewerPromptPath)
	if err != nil {
		return reviewerDryRunDocument{}, err
	}
	return reviewerDryRunDocument{
		Schema:    reviewerDryRunSchema,
		RunID:     runID,
		CreatedAt: createdAt,
		Reviewer: reviewerDryRunAgent{
			Name:    reviewer.Name,
			Command: reviewer.Command,
			Args:    agentArgs,
			Input:   reviewer.Input,
		},
		Checks: reviewerDryRunChecks{
			ReviewPrepared:   true,
			GateReportReady:  true,
			ContractApproved: true,
		},
		Artifacts: reviewerDryRunArtifacts{
			ReviewerPrompt:    reviewerPromptArtifact,
			ReviewerContext:   reviewerContextArtifact,
			Review:            reviewArtifact,
			Findings:          reviewFindingsArtifact,
			Resolutions:       reviewResolutionsArtifact,
			Proposals:         reviewProposalsArtifact,
			ProposalDecisions: reviewProposalDecisionsArtifact,
			GateReport:        gateReportArtifact,
		},
		WouldRun: agents.DryRunCommand{
			Command: wouldRun.Command,
			Args:    append([]string{}, wouldRun.Args...),
			Stdin:   wouldRun.Stdin,
		},
	}, nil
}

func reviewerPromptRepoPath(runID string) string {
	return runArtifactRepoRel(runID, reviewerPromptArtifact)
}

func nextReviewerAttemptID(attemptsDir string) (string, error) {
	entries, err := os.ReadDir(attemptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "reviewer_attempt_001", nil
		}
		return "", err
	}
	maxAttempt := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var number int
		if _, err := fmt.Sscanf(entry.Name(), "reviewer_attempt_%03d", &number); err == nil && number > maxAttempt {
			maxAttempt = number
		}
	}
	return fmt.Sprintf("reviewer_attempt_%03d", maxAttempt+1), nil
}

func reviewerAttemptPaths(runPaths contractRunPathSet, attemptID string) reviewerAttemptPathSet {
	dir := filepath.Join(runPaths.ReviewAttemptsDir, attemptID)
	return reviewerAttemptPathSet{
		Dir:         dir,
		RequestJSON: filepath.Join(dir, "request.json"),
		StdoutLog:   filepath.Join(dir, "stdout.log"),
		StderrLog:   filepath.Join(dir, "stderr.log"),
		ResultJSON:  filepath.Join(dir, "result.json"),
	}
}

func reviewerResultFromRunResult(runID string, attemptID string, reviewer string, result agents.RunResult) reviewerResultDocument {
	return reviewerResultDocument{
		Schema:         reviewerResultSchema,
		RunID:          runID,
		AttemptID:      attemptID,
		Reviewer:       reviewer,
		StartedAt:      result.StartedAt,
		FinishedAt:     result.FinishedAt,
		DurationMillis: result.DurationMillis,
		ExitCode:       result.ExitCode,
		TimedOut:       result.TimedOut,
		Stdout:         result.StdoutPath,
		Stderr:         result.StderrPath,
	}
}

func reviewerResultTimestamp(result reviewerResultDocument, fallback time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, result.FinishedAt); err == nil {
		return parsed
	}
	return fallback
}

// writeReviewerMemorySection summarizes the accepted-memory prompt boundary for
// the reviewer. When the prompt manifest records memory metadata, the selected
// freshness counts are shown; otherwise the boundary is reported as not built.
func writeReviewerMemorySection(b *strings.Builder, runPaths contractRunPathSet) {
	memory, hasManifestMemory := promptManifestMemoryForReview(runPaths.PromptManifest)
	fmt.Fprintln(b, "## Accepted memory")
	if hasManifestMemory || isRegularFile(runPaths.MemoryContextMD) {
		fmt.Fprintln(b, "- Memory context: context/memory-context.md")
	}
	if hasManifestMemory {
		fmt.Fprintf(b, "- Selected items: %d\n", memory.Selected.Total)
		fmt.Fprintf(b, "- Fresh: %d\n", memory.Selected.Fresh)
		fmt.Fprintf(b, "- Stale: %d\n", memory.Selected.Stale)
		fmt.Fprintf(b, "- Unknown: %d\n", memory.Selected.Unknown)
		fmt.Fprintln(b, "- Stale memory may be outdated and must be verified.")
	} else {
		fmt.Fprintln(b, "- Memory prompt boundary: not built")
	}
	fmt.Fprintln(b)
}

func promptManifestMemoryForReview(path string) (promptManifestMemory, bool) {
	if !isRegularFile(path) {
		return promptManifestMemory{}, false
	}
	manifest, err := readPromptManifest(path)
	if err != nil || manifest.Memory == nil {
		return promptManifestMemory{}, false
	}
	return *manifest.Memory, true
}

func renderReviewerContext(prep reviewerDryRunPreparation) string {
	var b strings.Builder
	state := buildReviewStateWithProposals(prep.Review, prep.Findings, prep.Resolutions, prep.Proposals, prep.ProposalDecisions)

	fmt.Fprintln(&b, "# Reviewer Context")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Run")
	fmt.Fprintf(&b, "- Run id: %s\n", prep.Context.State.RunID)
	fmt.Fprintf(&b, "- Run status: %s\n", prep.Context.State.Status)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Contract")
	fmt.Fprintf(&b, "- Goal: %s\n", prep.Contract.Goal)
	writeMarkdownStringList(&b, "- In scope:", prep.Contract.Scope.In)
	writeMarkdownStringList(&b, "- Out of scope:", prep.Contract.Scope.Out)
	writeMarkdownStringList(&b, "- Acceptance criteria:", prep.Contract.AcceptanceCriteria)
	writeMarkdownStringList(&b, "- Validation commands:", prep.Contract.Validation.Commands)
	fmt.Fprintln(&b)
	writeReviewerMemorySection(&b, prep.Context.RunPaths)
	fmt.Fprintln(&b, "## Gate report")
	fmt.Fprintf(&b, "- Gate status: %s\n", prep.GateReport.Status)
	fmt.Fprintf(&b, "- Execution attempt id: %s\n", valueOrNone(prep.GateReport.Execution.AttemptID))
	fmt.Fprintf(&b, "- Execution exit code: %d\n", prep.GateReport.Execution.ExitCode)
	fmt.Fprintln(&b, "- Validation command results:")
	if len(prep.GateReport.Validation.Commands) == 0 {
		fmt.Fprintln(&b, "  - none")
	} else {
		for _, command := range prep.GateReport.Validation.Commands {
			fmt.Fprintf(&b, "  - %s: %s (exit %d, timed out: %t, result: %s)\n", command.ID, command.Command, command.ExitCode, command.TimedOut, command.Result)
		}
	}
	fmt.Fprintln(&b, "- Change summary:")
	writeMarkdownIndentedStringList(&b, "changed files", prep.GateReport.Changes.ChangedFiles)
	writeMarkdownIndentedStringList(&b, "new files", prep.GateReport.Changes.NewFiles)
	writeMarkdownIndentedStringList(&b, "missing files", prep.GateReport.Changes.MissingFiles)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Existing manual review")
	fmt.Fprintf(&b, "- Review status: %s\n", state.Review.Status)
	fmt.Fprintf(&b, "- Current findings summary: findings=%d open=%d resolved=%d blocking_open=%d\n", state.Review.Summary.Findings, state.Review.Summary.Open, state.Review.Summary.Resolved, state.Review.Summary.BlockingOpen)
	fmt.Fprintln(&b, "- Existing findings:")
	if len(state.Findings) == 0 {
		fmt.Fprintln(&b, "  - none")
	} else {
		for _, finding := range state.Findings {
			fmt.Fprintf(&b, "  - %s severity=%s category=%s blocking=%t status=%s: %s\n", finding.ID, finding.Severity, finding.Category, finding.Blocking, finding.Status, finding.Message)
		}
	}
	fmt.Fprintln(&b, "- Existing resolutions:")
	if len(state.Resolutions) == 0 {
		fmt.Fprintln(&b, "  - none")
	} else {
		for _, resolution := range state.Resolutions {
			note := valueOrNone(resolution.Note)
			fmt.Fprintf(&b, "  - %s finding=%s source=%s note=%s\n", resolution.ID, resolution.FindingID, resolution.Source, note)
		}
	}
	fmt.Fprintf(&b, "- Proposal summary: pending=%d accepted=%d rejected=%d\n", state.ProposalSummary.Pending, state.ProposalSummary.Accepted, state.ProposalSummary.Rejected)
	fmt.Fprintln(&b, "- Existing proposals:")
	if len(state.Proposals) == 0 {
		fmt.Fprintln(&b, "  - none")
	} else {
		for _, proposal := range state.Proposals {
			fmt.Fprintf(&b, "  - %s severity=%s category=%s blocking=%t status=%s source=%s attempt=%s: %s\n", proposal.ID, proposal.Severity, proposal.Category, proposal.Blocking, proposal.Status, proposal.Source, proposal.ReviewerAttemptID, proposal.Message)
			if proposal.File != "" {
				location := proposal.File
				if proposal.Line > 0 {
					location = fmt.Sprintf("%s:%d", location, proposal.Line)
				}
				fmt.Fprintf(&b, "    location: %s\n", location)
			}
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Artifacts")
	fmt.Fprintln(&b, "- Contract: contract/contract.json")
	fmt.Fprintln(&b, "- Gate report: gate/gate-report.json")
	fmt.Fprintln(&b, "- Review: review/review.json")
	fmt.Fprintln(&b, "- Findings: review/findings.jsonl")
	fmt.Fprintln(&b, "- Resolutions: review/resolutions.jsonl")
	fmt.Fprintln(&b, "- Proposals: review/proposals.jsonl")
	fmt.Fprintln(&b, "- Proposal decisions: review/proposal-decisions.jsonl")
	fmt.Fprintln(&b, "- Execution result: execute/last-result.json")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Reviewer guidance")
	fmt.Fprintln(&b, "- This context is not complete semantic truth.")
	fmt.Fprintln(&b, "- Use `pactum search \"<term>\"` and inspect files before proposing findings.")
	fmt.Fprintln(&b, "- Do not invent changes.")
	fmt.Fprintln(&b, "- Do not approve automatically.")
	fmt.Fprintln(&b, "- If uncertain, propose a blocking finding that asks for clarification.")
	return b.String()
}

func renderReviewerPrompt(runID string) string {
	reviewerContextPath := runArtifactRepoRel(runID, reviewerContextArtifact)
	contractPath := runArtifactRepoRel(runID, "contract/contract.json")
	gateReportPath := runArtifactRepoRel(runID, gateReportArtifact)
	reviewPath := runArtifactRepoRel(runID, reviewArtifact)
	findingsPath := runArtifactRepoRel(runID, reviewFindingsArtifact)
	resolutionsPath := runArtifactRepoRel(runID, reviewResolutionsArtifact)
	proposalsPath := runArtifactRepoRel(runID, reviewProposalsArtifact)
	proposalDecisionsPath := runArtifactRepoRel(runID, reviewProposalDecisionsArtifact)

	var b strings.Builder
	fmt.Fprintln(&b, "# Reviewer Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This prompt is prepared for a reviewer agent subprocess.")
	fmt.Fprintln(&b, "Pactum captures reviewer output as artifacts and may parse optional structured proposal blocks, but it does not trust reviewer output automatically.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Objective")
	fmt.Fprintln(&b, "Review the executed task against the approved Pactum contract and gate report.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Inputs")
	fmt.Fprintf(&b, "- Reviewer context: %s\n", reviewerContextPath)
	fmt.Fprintf(&b, "- Contract: %s\n", contractPath)
	fmt.Fprintf(&b, "- Gate report: %s\n", gateReportPath)
	fmt.Fprintf(&b, "- Review artifacts: %s, %s, %s, %s, %s\n", reviewPath, findingsPath, resolutionsPath, proposalsPath, proposalDecisionsPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Review boundaries")
	fmt.Fprintln(&b, "- Do not apply patches.")
	fmt.Fprintln(&b, "- Do not modify files.")
	fmt.Fprintln(&b, "- Do not approve the review.")
	fmt.Fprintln(&b, "- Do not claim semantic correctness without evidence.")
	fmt.Fprintln(&b, "- Prefer concrete findings with file/path evidence.")
	fmt.Fprintln(&b, "- Focus on real problems, not style preferences.")
	fmt.Fprintln(&b, "- Read the actual file and surrounding context before proposing a finding.")
	fmt.Fprintln(&b, "- Check whether the issue is already mitigated or already represented in existing findings/proposals.")
	fmt.Fprintln(&b, "- If uncertain, recommend a blocking manual finding.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Output shape")
	fmt.Fprintln(&b, "If you report findings in prose, make them easy for a human to convert manually:")
	fmt.Fprintln(&b, "- message")
	fmt.Fprintln(&b, "- severity")
	fmt.Fprintln(&b, "- category")
	fmt.Fprintln(&b, "- file")
	fmt.Fprintln(&b, "- line")
	fmt.Fprintln(&b, "- blocking")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Optional structured finding proposals")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "If you propose findings, include a fenced JSON block exactly like:")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintln(&b, `  "schema": "pactum.reviewer_findings.v1",`)
	fmt.Fprintln(&b, `  "findings": [`)
	fmt.Fprintln(&b, "    {")
	fmt.Fprintln(&b, `      "message": "Explain the issue clearly.",`)
	fmt.Fprintln(&b, `      "severity": "medium",`)
	fmt.Fprintln(&b, `      "category": "quality",`)
	fmt.Fprintln(&b, `      "file": "internal/app/example.go",`)
	fmt.Fprintln(&b, `      "line": 42,`)
	fmt.Fprintln(&b, `      "blocking": true,`)
	fmt.Fprintln(&b, `      "evidence": "Short evidence from reviewed artifacts."`)
	fmt.Fprintln(&b, "    }")
	fmt.Fprintln(&b, "  ]")
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Rules:")
	fmt.Fprintln(&b, "- Use repo-relative file paths only.")
	fmt.Fprintln(&b, "- Do not include absolute paths.")
	fmt.Fprintln(&b, "- Use severity: low, medium, high, critical.")
	fmt.Fprintln(&b, "- Use category: correctness, scope, quality, validation, process, other.")
	fmt.Fprintln(&b, "- If uncertain, set blocking=true and explain uncertainty in evidence.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Important: Pactum does not trust this output automatically. A human must accept proposals.")
	return b.String()
}

func writeMarkdownStringList(b *strings.Builder, heading string, values []string) {
	fmt.Fprintln(b, heading)
	if len(values) == 0 {
		fmt.Fprintln(b, "  - none")
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "  - %s\n", value)
	}
}

func writeMarkdownIndentedStringList(b *strings.Builder, label string, values []string) {
	fmt.Fprintf(b, "  - %s:\n", label)
	if len(values) == 0 {
		fmt.Fprintln(b, "    - none")
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "    - %s\n", value)
	}
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

func writeReviewNotPrepared(stdout io.Writer, runID string) {
	fmt.Fprintf(stdout, "Review has not been prepared. Run: pactum review prepare %s\n", runID)
}

func writeReviewPrepared(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Review prepared")
	fmt.Fprintln(stdout)
	writeReviewRunAndGate(stdout, state)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Review:")
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Status)
	fmt.Fprintf(stdout, "  findings: %d\n", state.Review.Summary.Findings)
	fmt.Fprintf(stdout, "  blocking open: %d\n", state.Review.Summary.BlockingOpen)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  review: %s\n", runArtifactRepoRel(state.Review.RunID, reviewArtifact))
}

func writeReviewStatus(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Review status")
	fmt.Fprintln(stdout)
	writeReviewRunAndGate(stdout, state)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Review:")
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Status)
	fmt.Fprintf(stdout, "  findings: %d\n", state.Review.Summary.Findings)
	fmt.Fprintf(stdout, "  open: %d\n", state.Review.Summary.Open)
	fmt.Fprintf(stdout, "  resolved: %d\n", state.Review.Summary.Resolved)
	fmt.Fprintf(stdout, "  blocking open: %d\n", state.Review.Summary.BlockingOpen)
	if state.Review.Status == "approved" {
		if state.Review.Approval.ApprovedAt != nil {
			fmt.Fprintf(stdout, "  approved at: %s\n", *state.Review.Approval.ApprovedAt)
		}
		if state.Review.Approval.ApprovedBy != nil {
			fmt.Fprintf(stdout, "  approved by: %s\n", *state.Review.Approval.ApprovedBy)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Proposals:")
	fmt.Fprintf(stdout, "  pending: %d\n", state.ProposalSummary.Pending)
	fmt.Fprintf(stdout, "  accepted: %d\n", state.ProposalSummary.Accepted)
	fmt.Fprintf(stdout, "  rejected: %d\n", state.ProposalSummary.Rejected)
}

func writeReviewShow(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Review")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", state.Review.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Review:")
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Findings:")
	if len(state.Findings) == 0 {
		fmt.Fprintln(stdout, "  none")
	} else {
		for _, finding := range state.Findings {
			blocking := ""
			if finding.Blocking {
				blocking = " [blocking]"
			}
			fmt.Fprintf(stdout, "  - %s [%s]%s %s: %s\n", finding.ID, finding.Severity, blocking, finding.Category, finding.Message)
			if finding.File != "" {
				location := finding.File
				if finding.Line > 0 {
					location = fmt.Sprintf("%s:%d", location, finding.Line)
				}
				fmt.Fprintf(stdout, "    location: %s\n", location)
			}
			fmt.Fprintf(stdout, "    status: %s\n", finding.Status)
			if finding.LatestResolution != nil && finding.LatestResolution.Note != "" {
				fmt.Fprintf(stdout, "    resolution: %s\n", finding.LatestResolution.Note)
			}
		}
	}
	fmt.Fprintln(stdout)
	writeReviewPendingProposals(stdout, state.Proposals)
}

func writeReviewFindingAdded(stdout io.Writer, response reviewAddFindingResponse) {
	fmt.Fprintln(stdout, "Review finding added")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Finding.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.State.Review.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Finding:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Finding.ID)
	fmt.Fprintf(stdout, "  severity: %s\n", response.Finding.Severity)
	fmt.Fprintf(stdout, "  category: %s\n", response.Finding.Category)
	fmt.Fprintf(stdout, "  blocking: %t\n", response.Finding.Blocking)
	fmt.Fprintf(stdout, "  status: open\n")
}

func writeReviewResolved(stdout io.Writer, response reviewResolveResponse) {
	fmt.Fprintln(stdout, "Review finding resolved")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Resolution.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.State.Review.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Resolution:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Resolution.ID)
	fmt.Fprintf(stdout, "  finding: %s\n", response.Resolution.FindingID)
}

func writeReviewApproved(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Review approved")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", state.Review.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Status)
	if state.Review.Approval.ApprovedBy != nil {
		fmt.Fprintf(stdout, "  approved by: %s\n", *state.Review.Approval.ApprovedBy)
	}
}

func writeReviewDryRun(stdout io.Writer, plan reviewerDryRunDocument) {
	fmt.Fprintln(stdout, "Reviewer dry-run prepared")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", plan.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Reviewer:")
	fmt.Fprintf(stdout, "  name: %s\n", plan.Reviewer.Name)
	fmt.Fprintf(stdout, "  command: %s\n", plan.Reviewer.Command)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Checks:")
	fmt.Fprintln(stdout, "  review prepared: yes")
	fmt.Fprintln(stdout, "  gate report: ready")
	fmt.Fprintln(stdout, "  contract approved: yes")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Would run:")
	fmt.Fprintf(stdout, "  %s\n", formatAgentCommand(plan.WouldRun))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  reviewer prompt: %s\n", runArtifactRepoRel(plan.RunID, reviewerPromptArtifact))
	fmt.Fprintf(stdout, "  reviewer context: %s\n", runArtifactRepoRel(plan.RunID, reviewerContextArtifact))
	fmt.Fprintf(stdout, "  dry run: %s\n", runArtifactRepoRel(plan.RunID, reviewerDryRunArtifact))
}

func writeReviewRun(stdout io.Writer, request reviewerRequestDocument, result reviewerResultDocument) {
	fmt.Fprintln(stdout, "Reviewer attempt finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", result.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Reviewer:")
	fmt.Fprintf(stdout, "  name: %s\n", request.Reviewer.Name)
	fmt.Fprintf(stdout, "  command: %s\n", request.Reviewer.Command)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", result.AttemptID)
	fmt.Fprintf(stdout, "  exit code: %d\n", result.ExitCode)
	fmt.Fprintf(stdout, "  timed out: %t\n", result.TimedOut)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  request: %s\n", runArtifactRepoRel(result.RunID, filepath.ToSlash(filepath.Join(reviewerAttemptsArtifact, result.AttemptID, "request.json"))))
	fmt.Fprintf(stdout, "  result: %s\n", runArtifactRepoRel(result.RunID, filepath.ToSlash(filepath.Join(reviewerAttemptsArtifact, result.AttemptID, "result.json"))))
	fmt.Fprintf(stdout, "  stdout: %s\n", runArtifactRepoRel(result.RunID, result.Stdout))
	fmt.Fprintf(stdout, "  stderr: %s\n", runArtifactRepoRel(result.RunID, result.Stderr))
	fmt.Fprintf(stdout, "  last result: %s\n", runArtifactRepoRel(result.RunID, reviewerLastResultArtifact))
}

func writeReviewRunAndGate(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", state.Review.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Gate:")
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Gate.Status)
}
